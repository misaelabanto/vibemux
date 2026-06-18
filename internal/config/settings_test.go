package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettings_IconKeys(t *testing.T) {
	s := DefaultSettings()

	expectedKeys := []string{"working", "done", "blocked", "stale", "active", "no_git"}
	for _, key := range expectedKeys {
		if _, ok := s.Icons[key]; !ok {
			t.Errorf("DefaultSettings().Icons missing key %q", key)
		}
	}
}

func TestDefaultSettings_IconValues(t *testing.T) {
	s := DefaultSettings()

	cases := map[string]string{
		"working": "🦾",
		"done":    "✅",
		"blocked": "‼️",
		"stale":   "🫠",
		"active":  "⚪",
		"no_git":  "⊘",
	}
	for key, want := range cases {
		if got := s.Icons[key]; got != want {
			t.Errorf("DefaultSettings().Icons[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestDefaultSettings_Numerics(t *testing.T) {
	s := DefaultSettings()

	if s.LocalRefreshMS != 3000 {
		t.Errorf("LocalRefreshMS = %d, want 3000", s.LocalRefreshMS)
	}
	if s.StaleThresholdSec != 600 {
		t.Errorf("StaleThresholdSec = %d, want 600", s.StaleThresholdSec)
	}
	if !s.FetchOnEnter {
		t.Errorf("FetchOnEnter = false, want true")
	}
}

func TestLoadSettings_NoFile_ReturnsDefaults(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	def := DefaultSettings()

	if s.LocalRefreshMS != def.LocalRefreshMS {
		t.Errorf("LocalRefreshMS = %d, want %d", s.LocalRefreshMS, def.LocalRefreshMS)
	}
	if s.StaleThresholdSec != def.StaleThresholdSec {
		t.Errorf("StaleThresholdSec = %d, want %d", s.StaleThresholdSec, def.StaleThresholdSec)
	}
	if s.FetchOnEnter != def.FetchOnEnter {
		t.Errorf("FetchOnEnter = %v, want %v", s.FetchOnEnter, def.FetchOnEnter)
	}
	for key, want := range def.Icons {
		if got := s.Icons[key]; got != want {
			t.Errorf("Icons[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestLoadSettings_PartialFile_MergesOverDefaults(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	vibemuxDir := filepath.Join(tmp, "vibemux")
	if err := os.MkdirAll(vibemuxDir, 0o755); err != nil {
		t.Fatal(err)
	}

	partial := map[string]any{
		"icons": map[string]string{
			"working": "W",
		},
	}
	data, err := json.Marshal(partial)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vibemuxDir, "settings.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	def := DefaultSettings()

	if s.Icons["working"] != "W" {
		t.Errorf("Icons[\"working\"] = %q, want \"W\"", s.Icons["working"])
	}
	for key, want := range def.Icons {
		if key == "working" {
			continue
		}
		if got := s.Icons[key]; got != want {
			t.Errorf("Icons[%q] = %q, want default %q", key, got, want)
		}
	}
	if s.LocalRefreshMS != def.LocalRefreshMS {
		t.Errorf("LocalRefreshMS = %d, want %d", s.LocalRefreshMS, def.LocalRefreshMS)
	}
	if s.StaleThresholdSec != def.StaleThresholdSec {
		t.Errorf("StaleThresholdSec = %d, want %d", s.StaleThresholdSec, def.StaleThresholdSec)
	}
	if s.FetchOnEnter != def.FetchOnEnter {
		t.Errorf("FetchOnEnter = %v, want %v", s.FetchOnEnter, def.FetchOnEnter)
	}
}

func TestSettingsFile(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)

	got := SettingsFile()
	want := filepath.Join(tmp, "vibemux", "settings.json")
	if got != want {
		t.Errorf("SettingsFile() = %q, want %q", got, want)
	}
}

func TestLoadSettingsMissing(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if s.Multiplexer != "" {
		t.Errorf("Multiplexer = %q, want empty for missing file", s.Multiplexer)
	}
}

func TestSaveLoadSettingsRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	save := DefaultSettings()
	save.Multiplexer = "zellij"
	if err := SaveSettings(save); err != nil {
		t.Fatalf("SaveSettings() error: %v", err)
	}
	s, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error: %v", err)
	}
	if s.Multiplexer != "zellij" {
		t.Errorf("Multiplexer = %q, want %q", s.Multiplexer, "zellij")
	}
}
