package selfupdate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"digitdns/internal/version"
)

// InContainer reports whether this process runs in Docker/Podman. The update
// method setting is only meaningful there.
func (u *Updater) InContainer() bool { return u.container }

// SetContainerUpdateMode applies the operator's saved choice. It is a no-op
// outside a container: a systemd install always uses the signed binary path, and
// a stale posted value must not weaken that.
func (u *Updater) SetContainerUpdateMode(mode string) {
	u.containerBinary.Store(u.container && mode == "binary")
}

// ContainerUpdateMode returns "image" or "binary" for a container install, and
// "" elsewhere.
func (u *Updater) ContainerUpdateMode() string {
	if !u.container {
		return ""
	}
	if u.containerBinary.Load() {
		return "binary"
	}
	return "image"
}

// containerMarker lives beside the pre-update backups under the data volume. It
// answers one question the process cannot otherwise answer: did this container
// start from its image, or is it running a binary that was swapped in after it
// started?
type containerMarker struct {
	ContainerID    string    `json:"container_id"`
	ImageVersion   string    `json:"image_version"`
	SwappedVersion string    `json:"swapped_version,omitempty"`
	SwappedAt      time.Time `json:"swapped_at,omitempty"`
}

// reconcileContainerMarker runs once at startup.
//
// Docker gives every container a fresh hostname (its ID). If the marker names a
// different ID, this is a new container: whatever binary is running came from
// the image, so the image version is the current version and no swap is active.
// If the ID matches, the marker's swapped version is compared with the running
// version. A mismatch means the swapped binary is gone — a manual replacement or
// a recreate that kept the same hostname — and the marker is cleared.
func (u *Updater) reconcileContainerMarker() {
	if u.markerPath == "" {
		return
	}
	if u.containerID == "" {
		u.containerID = "unknown"
	}
	running := version.Get().Version

	var m containerMarker
	if data, err := os.ReadFile(u.markerPath); err == nil {
		_ = json.Unmarshal(data, &m)
	}

	if m.ContainerID != u.containerID {
		u.imageVersion = running
		u.swapActive.Store(false)
		u.writeMarker(containerMarker{ContainerID: u.containerID, ImageVersion: running})
		return
	}

	u.imageVersion = m.ImageVersion
	if u.imageVersion == "" {
		u.imageVersion = running
	}
	active := m.SwappedVersion != "" && m.SwappedVersion == running
	u.swapActive.Store(active)
	if m.SwappedVersion != "" && !active {
		m.SwappedVersion = ""
		m.SwappedAt = time.Time{}
		u.writeMarker(m)
	}
}

// recordContainerSwap is called after a verified binary has been swapped in.
func (u *Updater) recordContainerSwap(tag string) {
	if !u.container || u.markerPath == "" {
		return
	}
	swapped := strings.TrimPrefix(strings.TrimSpace(tag), "v")
	if swapped == "" {
		return
	}
	u.swapActive.Store(true)
	u.writeMarker(containerMarker{
		ContainerID:    u.containerID,
		ImageVersion:   u.imageVersion,
		SwappedVersion: swapped,
		SwappedAt:      time.Now().UTC(),
	})
}

// writeMarker persists the marker atomically. A failure is not fatal: the next
// start then treats the container as running its image, which is the conservative
// reading (no false "temporary update" badge).
func (u *Updater) writeMarker(m containerMarker) {
	data, err := json.Marshal(m)
	if err != nil {
		return
	}
	if dir := filepath.Dir(u.markerPath); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	tmp := u.markerPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, u.markerPath); err != nil {
		_ = os.Remove(tmp)
	}
}
