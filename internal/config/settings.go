package config

import (
	"encoding/json"
	"os"
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
	data, err := os.ReadFile(SettingsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return Settings{}, nil
		}
		return Settings{}, err
	}

	var s Settings
	if err := json.Unmarshal(data, &s); err != nil {
		return Settings{}, err
	}
	return s, nil
}

// SaveSettings writes settings.json, creating the config dir if needed.
func SaveSettings(s Settings) error {
	if err := EnsureDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(SettingsFile(), data, 0o644)
}
