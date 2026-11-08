// Package selfupdate implements the dashboard's "check for and apply an update"
// flow (v2.6): it reads the version published on the project's main branch,
// compares it with the running build, and — on Linux under systemd — downloads
// the matching release binary, verifies it, backs up the data files, swaps the
// binary atomically, and asks the daemon to restart onto it.
//
// Security posture (v2.8, audit finding #1): the download host is hard-coded
// (no operator-supplied URL), the client never honours a proxy, and a binary is
// NEVER swapped in unless BOTH conditions hold — its SHA-256 matches the
// release's checksums.txt, AND that checksums.txt carries a valid ed25519
// signature from the release signing key. A bare hash compare alone is an
// unauthenticated self-consistency test: both artifacts come from the same
// origin, so whoever can serve one can serve the other. The signature is what
// makes the expected hash trustworthy, and it is verified over the raw file
// bytes before any parsing happens. The private half of that key lives only in
// the GitHub Actions release-signing secret, so nothing in this repository can
// sign a release bundle.
//
// The data key/db/config are backed up before anything is replaced and are
// otherwise never touched, so an update cannot lose a subscriber list or an
// admin credential.
package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/version"
)

// ReleasePubKey is the ed25519 public key baked into the binary, pinning the
// only identity whose signatures this daemon will accept for a release bundle
// (checksums.txt). The private half lives only in the GitHub Actions secret
// RELEASE_SIGNING_KEY; nothing in this repository may sign a release.
//
// To rotate the release identity: generate a fresh keypair, put the private half
// in the RELEASE_SIGNING_KEY secret, and replace the base64 below — the public
// key is the 32-byte raw ed25519 key, not a PEM/SPKI wrapper.
var ReleasePubKey = mustDecodeKey("gicsCE+bVHVCr198m4mmWsG7FS3M6nOAHU/3/xrJJSU=")

func mustDecodeKey(b64 string) ed25519.PublicKey {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		panic("selfupdate: invalid embedded release public key")
	}
	return ed25519.PublicKey(raw)
}

// checksumsSignatureAsset is the release asset that carries the detached
// signature over checksums.txt's bytes. It is a SEPARATE FILE, not a response
// header: GitHub Releases serves assets from its CDN and passes through only its
// own headers, so any X-DigitDNS-* we sent would simply be dropped on the floor
// and every update would be refused. A file rides alongside checksums.txt on the
// same release and is fetched the same way.
const checksumsSignatureAsset = "checksums.sig"

// checksumsSignatureHeader is the response-header name a future self-hosted
// release host could use to carry the same signature (kept so a mirror or an
// intranet origin does not have to publish two files). It is only consulted when
// the asset itself is absent.
const checksumsSignatureHeader = "X-DigitDNS-Checksums-Signature"

// verifyChecksumsSignatureWith is the parameterised core of the gate, so a test
// can pin the behaviour against keys it controls without touching the embedded
// ReleasePubKey.
func verifyChecksumsSignatureWith(body []byte, header string, pub ed25519.PublicKey) error {
	if header == "" {
		return fmt.Errorf("the release carries no checksums signature (refusing an unverified update)")
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(header))
	if err != nil {
		return fmt.Errorf("the checksums signature is not valid base64: %w", err)
	}
	if !ed25519.Verify(pub, body, sig) {
		return fmt.Errorf("checksums signature verification failed (the bundle was not signed by the release key)")
	}
	return nil
}

// verifyChecksumsSignature checks the detached signature over the raw
// checksums.txt bytes against the pinned release key. It is the gate that
// turns the same-origin hash comparison into an authenticated one.
func verifyChecksumsSignature(body []byte, header string) error {
	return verifyChecksumsSignatureWith(body, header, ReleasePubKey)
}

// VerifyChecksumsSignatureForTest exposes the exact gate to the repo-only
// release-signing tool (tools/releasesign), so a signature that verifies there
// is bit-for-bit the one the daemon will accept. Not used by the daemon path.
func VerifyChecksumsSignatureForTest(body []byte, header string) error {
	return verifyChecksumsSignature(body, header)
}

// This fork publishes its own releases from digitrob/DigitDNS on the `main`
// branch, and signs every bundle with the fork's own pinned RELEASE_SIGNING_KEY
// (see ReleasePubKey above). The updater only ever reads from these values, so
// repointing them here is what makes a fork build update from the fork and not
// from upstream.
const (
	ghOwner  = "digitrob"
	ghRepo   = "DigitDNS"
	ghBranch = "main"

	rawVersionURL = "https://raw.githubusercontent.com/" + ghOwner + "/" + ghRepo + "/" + ghBranch + "/version.json"
	releasesURL   = "https://api.github.com/repos/" + ghOwner + "/" + ghRepo + "/releases?per_page=30"

	// maxBinaryBytes caps a downloaded release binary. The linux build is ~20 MB;
	// 128 MB is generous headroom that still refuses a decompression-bomb-sized body.
	maxBinaryBytes = 128 << 20
)

// Status is the answer to "is there a newer version?" for the dashboard.
type Status struct {
	Current         string `json:"current"`
	Latest          string `json:"latest"`
	Codename        string `json:"codename,omitempty"`
	Channel         string `json:"channel,omitempty"`
	UpdateAvailable bool   `json:"update_available"`
	Supported       bool   `json:"supported"` // false when auto-apply cannot run here (non-Linux or image-mode container)
	Note            string `json:"note,omitempty"`

	// Container reports whether this process runs in Docker/Podman. The panel
	// uses it to decide whether the update-method selector applies at all.
	Container bool `json:"container,omitempty"`
	// UpdateMode is "image" or "binary" on a container install, and empty
	// elsewhere. It reflects the operator's saved choice, not whether a swap is
	// currently live.
	UpdateMode string `json:"update_mode,omitempty"`
	// ContainerSwapActive is true when the running binary was swapped into this
	// container after it started. A recreate restores ContainerImageVersion.
	ContainerSwapActive bool `json:"container_swap_active,omitempty"`
	// ContainerImageVersion is the version the container started with, which is
	// what a recreate would restore.
	ContainerImageVersion string `json:"container_image_version,omitempty"`
}

// Progress is the live state of an in-flight apply, polled by the modal.
type Progress struct {
	Running    bool      `json:"running"`
	Phase      string    `json:"phase"` // idle|resolve|download|verify|backup|swap|restart|done|error
	Percent    int       `json:"percent"`
	Message    string    `json:"message"`
	Restarting bool      `json:"restarting"`
	StartedAt  time.Time `json:"started_at,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// updateStaleAfter is how long a Running apply may sit without the process
// having exited before a retry is allowed. The apply context is five minutes;
// this leaves a margin for cleanup. Without it, a failed restart or a hung
// goroutine made every later click answer "an update is already in progress".
const updateStaleAfter = 6 * time.Minute

// applyBusy reports whether an in-flight apply is still recent enough to refuse
// a second one. A zero StartedAt (a state written by an older build) is stale.
func applyBusy(progress Progress, now time.Time) bool {
	return progress.Running && now.Sub(progress.StartedAt) < updateStaleAfter
}

// runningInContainer reports whether this process is inside a container. The
// binary-swap updater is correct for a systemd install, but inside Docker it
// replaces the container's writable-layer binary and never pulls the image the
// operator actually deploys. Container installs are therefore told to pull and
// recreate instead.
func runningInContainer() bool {
	if os.Getenv("DIGITDNS_CONTAINER") == "1" {
		return true
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	if _, err := os.Stat("/run/.containerenv"); err == nil {
		return true
	}
	if data, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		for _, marker := range [][]byte{[]byte("docker"), []byte("kubepods"), []byte("containerd"), []byte("lxc")} {
			if bytes.Contains(data, marker) {
				return true
			}
		}
	}
	return false
}

// Updater owns the paths it may touch and the progress of the current apply.
type Updater struct {
	binPath   string   // the running executable (os.Executable)
	dataPaths []string // files to back up before swapping (db, key, config)
	backupDir string   // where timestamped pre-update backups are written
	container bool     // true inside Docker/Podman; binary swap is not the deployed artefact
	client    *http.Client

	// Container update state. containerBinary is the operator's opt-in to the
	// in-container swap; it is only honored when container is true. The marker
	// lets the panel tell "running a temporary swapped binary" from "running the
	// image's binary", which a recreate silently switches back to.
	containerBinary atomic.Bool
	swapActive      atomic.Bool
	containerID     string
	imageVersion    string
	markerPath      string

	mu       sync.Mutex
	progress Progress
}

// New builds an Updater. binPath is the running binary to replace; dataPaths are
// the files copied into backupDir before any swap.
func New(binPath string, dataPaths []string, backupDir string) *Updater {
	u := &Updater{
		binPath:   binPath,
		dataPaths: dataPaths,
		backupDir: backupDir,
		container: runningInContainer(),
		client: &http.Client{
			Timeout: 3 * time.Minute, // a whole binary download, not a metadata call
			Transport: &http.Transport{
				// Never honour a proxy: an update mechanism that follows
				// HTTP(S)_PROXY can be walked straight into a TLS-intercepting
				// middlebox that is trusted locally (CA store), which defeats the
				// whole origin-authentication chain. Direct is the only safe
				// default for something whose output is executed as root.
				Proxy: nil,
			},
		},
	}
	if u.container {
		u.containerID, _ = os.Hostname()
		u.markerPath = filepath.Join(backupDir, "container-swap.json")
		u.reconcileContainerMarker()
	}
	return u
}

// Progress returns a copy of the current apply state. A Running flag that has
// outlived the apply deadline is cleared here as well as in StartApply: the
// progress modal polls this endpoint, so without it a goroutine that never
// reached fail() would leave the panel spinning on "in progress" forever even
// though every later click is allowed.
func (u *Updater) Progress() Progress {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.progress.Running && !applyBusy(u.progress, time.Now()) {
		u.progress.Running = false
		u.progress.Restarting = false
		u.progress.Phase = "error"
		u.progress.Error = "the previous update stalled; check the service log and retry"
	}
	return u.progress
}

func (u *Updater) setProgress(phase, msg string, percent int) {
	u.mu.Lock()
	u.progress.Running = true
	u.progress.Phase = phase
	u.progress.Message = msg
	u.progress.Percent = percent
	u.progress.Error = ""
	u.mu.Unlock()
}

func (u *Updater) fail(err error) {
	u.mu.Lock()
	u.progress.Running = false
	u.progress.Restarting = false
	u.progress.Phase = "error"
	u.progress.Error = err.Error()
	u.mu.Unlock()
}

// Check reads the version published on the main branch and compares it with the
// running build. Network failures are returned as errors; a well-formed "no
// update" answer is not an error.
func (u *Updater) Check(ctx context.Context) (Status, error) {
	cur := version.Get()
	st := Status{Current: cur.Version, Container: u.container}
	if u.container {
		st.UpdateMode = u.ContainerUpdateMode()
		st.ContainerSwapActive = u.swapActive.Load()
		st.ContainerImageVersion = u.imageVersion
	}
	switch {
	case runtime.GOOS != "linux":
		st.Note = "Automatic update is available on Linux/systemd installs only."
	case u.container && !u.containerBinary.Load():
		// The binary-swap updater would replace the container's writable-layer
		// binary, not the image the operator actually deploys. Refuse it and
		// give the command that does update the deployment.
		st.Note = "Update the image: docker compose pull && docker compose up -d"
	case u.container:
		// The operator opted into the temporary path explicitly. Never present it
		// as an image update: a recreate restores the image's binary.
		st.Supported = true
		st.Note = "Temporary binary update: a recreate restores the image's binary."
	default:
		st.Supported = true
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawVersionURL, nil)
	if err != nil {
		return st, err
	}
	resp, err := u.client.Do(req)
	if err != nil {
		return st, fmt.Errorf("reach GitHub: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return st, fmt.Errorf("version.json fetch returned HTTP %d", resp.StatusCode)
	}
	var remote struct {
		Version  string `json:"version"`
		Channel  string `json:"channel"`
		Codename string `json:"codename"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&remote); err != nil {
		return st, fmt.Errorf("parse remote version.json: %w", err)
	}
	if remote.Version == "" {
		return st, fmt.Errorf("remote version.json carried no version")
	}
	st.Latest = remote.Version
	st.Channel = remote.Channel
	st.Codename = remote.Codename
	st.UpdateAvailable = compareSemver(remote.Version, cur.Version) > 0
	return st, nil
}

// compareSemver returns >0 when a is a newer version than b, 0 when equal, <0
// otherwise. It compares the dotted numeric fields only (2.6.0 vs 2.5.0); the
// channel/prerelease suffix is deliberately ignored — the version.json on main
// carries no build suffix to compare against.
func compareSemver(a, b string) int {
	as := splitVersion(a)
	bs := splitVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var av, bv int
		if i < len(as) {
			av = as[i]
		}
		if i < len(bs) {
			bv = bs[i]
		}
		if av != bv {
			if av > bv {
				return 1
			}
			return -1
		}
	}
	return 0
}

func splitVersion(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	// Drop any -beta / +build suffix; compare the numeric core only.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil {
			break // stop at the first non-numeric field
		}
		out = append(out, n)
	}
	return out
}

// StartApply launches the update in the background if one is not already
// running. It returns immediately; the dashboard polls Progress(). The target
// version string is informational (the release is resolved from GitHub).
func (u *Updater) StartApply() error {
	if runtime.GOOS != "linux" {
		return fmt.Errorf("automatic update runs on Linux/systemd installs only")
	}
	if u.container && !u.containerBinary.Load() {
		return fmt.Errorf("this install runs in a container; update the image instead: docker compose pull && docker compose up -d")
	}
	u.mu.Lock()
	if applyBusy(u.progress, time.Now()) {
		phase, message := u.progress.Phase, u.progress.Message
		u.mu.Unlock()
		return fmt.Errorf("an update is already in progress (%s: %s)", phase, message)
	}
	u.progress = Progress{
		Running:   true,
		Phase:     "resolve",
		Message:   "Starting update…",
		Percent:   1,
		StartedAt: time.Now(),
	}
	u.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if err := u.apply(ctx); err != nil {
			u.fail(err)
		}
	}()
	return nil
}
