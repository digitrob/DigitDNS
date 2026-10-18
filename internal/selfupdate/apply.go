package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ghRelease is the slice of the GitHub Releases API we consume.
type ghRelease struct {
	TagName    string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// tagMatchesTarget reports whether a release tag is precisely the target
// version, not merely a superstring of it. strings.Contains(tag, "2.6.0")
// accepts v2.6.0-beta.9, v2.6.01 and v12.6.0 — an operator who asked for 2.6.0
// can silently receive a beta (or, on a differently-segmented tag line, an
// unintended build). A tag is v<semver>[-channel[.n]]: the numeric core must
// equal the target exactly, and only the channel/prerelease suffix may follow.
func tagMatchesTarget(tag, target string) bool {
	tag = strings.TrimPrefix(tag, "v")
	target = strings.TrimPrefix(target, "v")
	core, suffix := tag, ""
	if i := strings.IndexByte(tag, '-'); i >= 0 {
		core, suffix = tag[:i], tag[i+1:]
	}
	if core != targetNumericCore(target) {
		return false
	}
	// The channel-suffix rule:
	//   - a final tag (no suffix) may satisfy any target: the operator's
	//     intent is "at least this version", and the final build is the release
	//     the target names;
	//   - a channel tag satisfies a target only when that target names the SAME
	//     channel (beta → beta), never a final request — a stray beta must not
	//     masquerade as the final build an operator asked for.
	if suffix == "" {
		return true
	}
	return targetHasChannel(target) && strings.HasPrefix(suffix, targetChannel(target))
}

func targetNumericCore(v string) string {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i]
	}
	return v
}

func targetChannel(v string) string {
	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[i+1:]
	}
	return ""
}

// targetHasChannel reports whether the target itself names a channel suffix —
// the "beta" in 2.6.0-beta. A channel tag can satisfy only such a target.
func targetHasChannel(v string) bool {
	return strings.IndexByte(v, '-') >= 0
}

// apply runs the full update pipeline. Every step updates progress so the modal
// can narrate it; any error is returned and surfaced by StartApply.
func (u *Updater) apply(ctx context.Context) error {
	binName := "digitdns-linux-" + runtime.GOARCH // digitdns-linux-amd64 / -arm64

	// 1. Resolve the newest release and the two assets we need from it.
	u.setProgress("resolve", "Finding the latest release…", 5)
	binURL, sumURL, tag, err := u.resolveAssets(ctx, binName)
	if err != nil {
		return err
	}

	// 2. Fetch checksums.txt AND its detached signature (a release asset — the
	//    release host passes no custom headers), verify the signature over the
	//    raw bytes, and only then parse the hash. Both files come from the same
	//    origin, so without the signature the hash comparison is an
	//    unauthenticated self-consistency test, not origin authentication.
	u.setProgress("verify", "Fetching and verifying signed checksums…", 15)
	wantSum, err := u.expectedSum(ctx, sumURL, binName)
	if err != nil {
		return err
	}

	u.setProgress("download", "Downloading "+tag+"…", 25)
	tmpPath, gotSum, err := u.downloadTo(ctx, binURL)
	if tmpPath != "" {
		defer os.Remove(tmpPath)
	}
	if err != nil {
		return err
	}
	u.setProgress("verify", "Verifying binary digest and signature…", 70)
	if !strings.EqualFold(gotSum, wantSum) {
		return fmt.Errorf("checksum mismatch: the downloaded binary does not match the release's signed checksums.txt (refusing to install)")
	}

	// 3. Back up the data files (never touched by the swap, but a pre-update
	//    snapshot is what makes a bad release recoverable).
	u.setProgress("backup", "Backing up data…", 78)
	if err := u.backupData(); err != nil {
		return fmt.Errorf("pre-update backup failed, aborting before any change: %w", err)
	}

	// 4. Swap the binary atomically (re-hashing first), keeping the old one as
	//    .bak for rollback.
	u.setProgress("swap", "Installing the new binary…", 88)
	if err := u.swapBinary(tmpPath, gotSum); err != nil {
		return err
	}

	// 5. Ask the daemon to restart onto the new binary.
	u.setProgress("restart", "Update installed — restarting…", 100)
	u.mu.Lock()
	u.progress.Restarting = true
	u.progress.Phase = "done"
	u.mu.Unlock()
	go func() {
		// Let the modal poll the "restarting" state once before the process goes.
		time.Sleep(1500 * time.Millisecond)
		_ = signalRestart()
	}()
	return nil
}

// resolveAssets finds the newest release whose tag carries the main-branch
// version and returns the download URLs for the arch binary and checksums.txt.
func (u *Updater) resolveAssets(ctx context.Context, binName string) (binURL, sumURL, tag string, err error) {
	st, err := u.Check(ctx)
	if err != nil {
		return "", "", "", err
	}
	if !st.UpdateAvailable {
		return "", "", "", fmt.Errorf("already on the latest version (%s)", st.Current)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, releasesURL, nil)
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := u.client.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("list releases: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("releases API returned HTTP %d", resp.StatusCode)
	}
	var releases []ghRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return "", "", "", fmt.Errorf("parse releases: %w", err)
	}

	// The list is newest-first; take the newest NON-draft release whose tag is
	// exactly the target version (a beta channel suffix is accepted only when
	// the target itself names that channel), and which carries the binary and
	// the signed checksums pair.
	for _, rel := range releases {
		if rel.Draft {
			continue
		}
		if !tagMatchesTarget(rel.TagName, st.Latest) {
			continue
		}
		binURL, sumURL = "", ""
		for _, a := range rel.Assets {
			switch a.Name {
			case binName:
				binURL = a.URL
			case "checksums.txt":
				sumURL = a.URL
			}
		}
		if binURL != "" && sumURL != "" {
			return binURL, sumURL, rel.TagName, nil
		}
	}
	return "", "", "", fmt.Errorf("no release asset %q with a checksums.txt found for version %s", binName, st.Latest)
}

// expectedSum downloads checksums.txt, verifies the detached release signature
// over its raw bytes, and returns the hex SHA-256 it records for binName. The
// signature gate runs BEFORE the parse: an unsigned or wrongly-signed bundle is
// refused whatever it says, so a same-origin attacker who can replace both
// artifacts still cannot produce a bundle this daemon will accept. The file is
// `sha256sum *` output: "<hex>  <filename>" per line.
func (u *Updater) expectedSum(ctx context.Context, sumURL, binName string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sumURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch checksums.txt: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("checksums.txt returned HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	// The signature travels as a release ASSET next to checksums.txt, because a
	// release host (GitHub's CDN) passes no custom response headers through. A
	// header is still honoured when present, so a self-hosted mirror can carry
	// it that way instead. Both routes mean an unsigned bundle is refused.
	sigValue := resp.Header.Get(checksumsSignatureHeader)
	if sigValue == "" {
		sigAssetURL := strings.Replace(sumURL, "checksums.txt", checksumsSignatureAsset, 1)
		if sigAssetURL == sumURL {
			return "", fmt.Errorf("the release carries no checksums signature (refusing an unverified update)")
		}
		sigReq, err := http.NewRequestWithContext(ctx, http.MethodGet, sigAssetURL, nil)
		if err != nil {
			return "", err
		}
		sigResp, err := u.client.Do(sigReq)
		if err != nil {
			return "", fmt.Errorf("fetch %s: %w", checksumsSignatureAsset, err)
		}
		if sigResp.StatusCode != http.StatusOK {
			sigResp.Body.Close()
			return "", fmt.Errorf("the release carries no %s asset (HTTP %d; refusing an unverified update)", checksumsSignatureAsset, sigResp.StatusCode)
		}
		rawSig, err := io.ReadAll(io.LimitReader(sigResp.Body, 8<<10))
		sigResp.Body.Close()
		if err != nil {
			return "", err
		}
		sigValue = strings.TrimSpace(string(rawSig))
	}
	if err := verifyChecksumsSignature(body, sigValue); err != nil {
		return "", err
	}
	for _, line := range strings.Split(string(body), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == binName {
			return fields[0], nil
		}
	}
	return "", fmt.Errorf("checksums.txt has no entry for %q", binName)
}

// downloadTo streams a URL to a temp file next to the target binary (same
// filesystem, so the later rename is atomic) and returns the file's SHA-256.
func (u *Updater) downloadTo(ctx context.Context, url string) (path, sum string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", "", err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(u.binPath), ".digitdns-update-*")
	if err != nil {
		return "", "", err
	}
	path = tmp.Name()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(resp.Body, maxBinaryBytes)); err != nil {
		tmp.Close()
		return path, "", fmt.Errorf("write download: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return path, "", err
	}
	return path, hex.EncodeToString(h.Sum(nil)), nil
}

// backupData copies each data file into a timestamped directory. Missing files
// are skipped (a fresh install may have no config yet); an unreadable/unwritable
// file is a hard error so the swap does not proceed without a snapshot.
func (u *Updater) backupData() error {
	stamp := time.Now().Format("20060102-150405")
	dst := filepath.Join(u.backupDir, "pre-update-"+stamp)
	if err := os.MkdirAll(dst, 0o700); err != nil {
		return err
	}
	for _, src := range u.dataPaths {
		if src == "" {
			continue
		}
		if _, err := os.Stat(src); err != nil {
			continue // not present yet
		}
		if err := copyFile(src, filepath.Join(dst, filepath.Base(src)), 0o600); err != nil {
			return err
		}
	}
	return nil
}

// swapBinary re-reads and re-hashes the verified download (binding the bytes
// that will be renamed to the bytes that were verified — a chmod'd file on a
// writable directory could otherwise be swapped between the check and the
// rename), keeps the current binary as <bin>.bak for rollback, and renames the
// new one into place. The rename is atomic within one filesystem, so a crash
// cannot leave a half-written binary.
func (u *Updater) swapBinary(tmpPath string, wantSum string) error {
	// Re-hash the file as it sits on disk right before it becomes the target.
	raw, err := os.ReadFile(tmpPath)
	if err != nil {
		return fmt.Errorf("re-read the downloaded binary: %w", err)
	}
	if got := sha256.Sum256(raw); !strings.EqualFold(hex.EncodeToString(got[:]), wantSum) {
		return fmt.Errorf("the downloaded file changed between download and install (refusing to install)")
	}
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return err
	}
	// Best-effort rollback copy of the running binary.
	_ = copyFile(u.binPath, u.binPath+".bak", 0o755)
	if err := os.Rename(tmpPath, u.binPath); err != nil {
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
