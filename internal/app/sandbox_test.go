package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
)

// TestTempXDGDirSandboxesConfigDir guards the helper the rest of these tests
// rely on for safety.
//
// config.Dir() and the agent store read two different environment variables, so
// redirecting one does not redirect the other. When the config dir escapes the
// sandbox it resolves to the developer's real ~/.config/vibemux, and the first
// test to call config.SaveProjects overwrites their actual project list with
// fixtures. That is silent: the test still passes.
//
// Paths are only inspected here, never written, so this test cannot itself
// damage a real config dir if the helper regresses.
func TestTempXDGDirSandboxesConfigDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home dir: %v", err)
	}
	realConfigDir := filepath.Join(home, ".config", "vibemux")

	tempXDGDir(t)

	got := config.Dir()
	if got == realConfigDir {
		t.Fatalf("config.Dir() = %s, the real config dir; tests would overwrite the user's projects.json", got)
	}
	if strings.HasPrefix(got, home+string(filepath.Separator)) {
		t.Errorf("config.Dir() = %s, still inside the home dir; want a temp dir", got)
	}
	if !strings.HasPrefix(config.ProjectsFile(), got) {
		t.Errorf("ProjectsFile() = %s, want it under the sandboxed config dir %s", config.ProjectsFile(), got)
	}
}

// TestTempXDGDirSandboxesAgentDir covers the other half of the helper, so a
// future edit cannot fix one redirect by dropping the other.
func TestTempXDGDirSandboxesAgentDir(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot resolve home dir: %v", err)
	}

	runtimeDir := tempXDGDir(t)

	got := agent.AgentsDir()
	if !strings.HasPrefix(got, runtimeDir) {
		t.Errorf("AgentsDir() = %s, want it under the sandboxed runtime dir %s", got, runtimeDir)
	}
	// The unsandboxed fallback is $HOME/.local/state/vibemux/agents.
	if strings.HasPrefix(got, filepath.Join(home, ".local", "state")) {
		t.Errorf("AgentsDir() = %s, still the real state dir", got)
	}
}
