package config

import "testing"

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

	if err := SaveSettings(Settings{Multiplexer: "zellij"}); err != nil {
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
