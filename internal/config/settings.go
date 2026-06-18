package config

import (
	"path/filepath"
)

// Settings holds vibemux's user-level preferences. Stored separately from
// projects.json because it is a single object, not a list.
type Settings struct {
	Multiplexer string `json:"multiplexer"`
}

// SettingsFile is the path to the settings JSON inside the config dir.
func SettingsFile() string {
	return filepath.Join(Dir(), "settings.json")
}

// LoadSettings reads settings.json. A missing file is not an error: it
// returns the zero Settings (no multiplexer chosen yet).
func LoadSettings() (Settings, error) {
	return readJSON[Settings](SettingsFile())
}

// SaveSettings writes settings.json, creating the config dir if needed.
func SaveSettings(s Settings) error {
	return writeJSON(SettingsFile(), s)
}
