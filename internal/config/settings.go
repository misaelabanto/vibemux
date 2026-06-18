package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings holds global vibemux configuration loaded from config.json.
type Settings struct {
	Icons             map[string]string `json:"icons"`
	LocalRefreshMS    int               `json:"local_refresh_ms"`
	StaleThresholdSec int               `json:"stale_threshold_sec"`
	FetchOnEnter      bool              `json:"fetch_on_enter"`
}

// DefaultSettings returns the built-in defaults for all settings.
func DefaultSettings() Settings {
	return Settings{
		Icons: map[string]string{
			"working": "🦾",
			"done":    "✅",
			"blocked": "‼️",
			"stale":   "🫠",
			"active":  "⚪",
			"no_git":  "⊘",
		},
		LocalRefreshMS:    3000,
		StaleThresholdSec: 600,
		FetchOnEnter:      true,
	}
}

// ConfigFile returns the path to config.json inside the vibemux config dir.
func ConfigFile() string {
	return filepath.Join(Dir(), "config.json")
}

// LoadSettings reads config.json and merges it over defaults.
// Missing fields keep their default values. Any read or parse error returns defaults.
func LoadSettings() Settings {
	s := DefaultSettings()

	data, err := os.ReadFile(ConfigFile())
	if err != nil {
		// File missing or unreadable - return defaults.
		return s
	}

	// Unmarshal into a temporary struct so we can merge icon keys individually.
	var raw struct {
		Icons             map[string]string `json:"icons"`
		LocalRefreshMS    *int              `json:"local_refresh_ms"`
		StaleThresholdSec *int              `json:"stale_threshold_sec"`
		FetchOnEnter      *bool             `json:"fetch_on_enter"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		// Malformed JSON - return defaults.
		return s
	}

	// Merge icon keys individually so unset keys keep defaults.
	for key, val := range raw.Icons {
		s.Icons[key] = val
	}

	if raw.LocalRefreshMS != nil {
		s.LocalRefreshMS = *raw.LocalRefreshMS
	}
	if raw.StaleThresholdSec != nil {
		s.StaleThresholdSec = *raw.StaleThresholdSec
	}
	if raw.FetchOnEnter != nil {
		s.FetchOnEnter = *raw.FetchOnEnter
	}

	return s
}
