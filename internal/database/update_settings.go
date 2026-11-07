package database

// UpdateSettings holds this install's self-update preferences. They are stored
// as their own encrypted settings record rather than folded into the large
// server record: the only consumer is the updater, and the value is meaningless
// outside a container.
type UpdateSettings struct {
	// ContainerMode selects how a container install applies an update:
	//
	//	"image"  — the panel refuses the in-app swap and shows the compose
	//	           pull/recreate command (the default).
	//	"binary" — the panel downloads, verifies and swaps the binary inside
	//	           the running container. A container recreate restores the
	//	           image's binary, and the image itself is not updated.
	//
	// It has no effect on a systemd install, which always uses the signed binary
	// path.
	ContainerMode string `json:"container_mode,omitempty"`
}

// Normalize collapses anything unrecognised to the safe default.
func (s UpdateSettings) Normalize() UpdateSettings {
	if s.ContainerMode != "binary" {
		s.ContainerMode = "image"
	}
	return s
}

// GetUpdateSettings returns the stored preferences, normalised. A missing record
// reads as the default ("image"), which is what every install written before
// this setting existed should get.
func (db *DB) GetUpdateSettings() UpdateSettings {
	var s UpdateSettings
	if db == nil {
		return s.Normalize()
	}
	_ = db.GetSetting("update", &s)
	return s.Normalize()
}

// SaveUpdateSettings persists the preferences. The caller is responsible for
// applying the change to the live updater.
func (db *DB) SaveUpdateSettings(s UpdateSettings) error {
	if db == nil {
		return ErrSettingsNil
	}
	return db.SetSetting("update", s.Normalize())
}
