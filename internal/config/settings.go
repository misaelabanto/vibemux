package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings holds vibemux's user-level preferences. Stored as a single object
// in settings.json (not a list, unlike projects.json). It carries both the
// chosen multiplexer backend and the project-status display configuration.
type Settings struct {
	Multiplexer       string            `json:"multiplexer"`
	Icons             map[string]string `json:"icons"`
	LocalRefreshMS    int               `json:"local_refresh_ms"`
	StaleThresholdSec int               `json:"stale_threshold_sec"`
	FetchOnEnter      bool              `json:"fetch_on_enter"`
}

// DefaultSettings returns the built-in defaults. Multiplexer is intentionally
// empty: an unset multiplexer triggers onboarding.
func DefaultSettings() Settings {
	return Settings{
		Icons: map[string]string{
			"working": "🦾",
			"done":    "✅",
			"blocked": "❗",
			"stale":   "🫠",
			"active":  "⚪",
			"no_git":  "⊘",
		},
		LocalRefreshMS:    3000,
		StaleThresholdSec: 600,
		FetchOnEnter:      true,
	}
}

// SettingsFile is the path to settings.json inside the config dir.
func SettingsFile() string {
	return filepath.Join(Dir(), "settings.json")
}

// LoadSettings reads settings.json and merges it over defaults. A missing
// file is not an error: it yields the defaults with an empty Multiplexer.
// Icon keys are merged individually so unset keys keep their default glyph.
func LoadSettings() (Settings, error) {
	s := DefaultSettings()

	data, err := os.ReadFile(SettingsFile())
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return s, err
	}

	// Pointers distinguish "absent" from "explicit zero" for the numeric and
	// bool fields, so an absent field keeps its default.
	var raw struct {
		Multiplexer       string            `json:"multiplexer"`
		Icons             map[string]string `json:"icons"`
		LocalRefreshMS    *int              `json:"local_refresh_ms"`
		StaleThresholdSec *int              `json:"stale_threshold_sec"`
		FetchOnEnter      *bool             `json:"fetch_on_enter"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return s, err
	}

	s.Multiplexer = raw.Multiplexer
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

	// Guard against a settings.json that persisted zero values for these
	// (for example an early save before the status fields existed): a zero
	// refresh or stale window would break the tick loop and stale detection.
	if s.LocalRefreshMS <= 0 {
		s.LocalRefreshMS = 3000
	}
	if s.StaleThresholdSec <= 0 {
		s.StaleThresholdSec = 600
	}

	return s, nil
}

// SaveSettings writes settings.json, creating the config dir if needed.
// Callers should load, modify, and save so existing fields are preserved.
func SaveSettings(s Settings) error {
	return writeJSON(SettingsFile(), s)
}
