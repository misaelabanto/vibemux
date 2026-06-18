package projectlist

import (
	"testing"

	"github.com/misaelabanto/vibemux/internal/agent"
	"github.com/misaelabanto/vibemux/internal/config"
	"github.com/misaelabanto/vibemux/internal/gitstatus"
)

var defaultSettings = config.DefaultSettings()

// AgentIcon tests

func TestAgentIcon_Working(t *testing.T) {
	got := AgentIcon(agent.Working, defaultSettings)
	want := defaultSettings.Icons["working"]
	if got != want {
		t.Errorf("AgentIcon(Working) = %q, want %q", got, want)
	}
}

func TestAgentIcon_Done(t *testing.T) {
	got := AgentIcon(agent.Done, defaultSettings)
	want := defaultSettings.Icons["done"]
	if got != want {
		t.Errorf("AgentIcon(Done) = %q, want %q", got, want)
	}
}

func TestAgentIcon_Blocked(t *testing.T) {
	got := AgentIcon(agent.Blocked, defaultSettings)
	want := defaultSettings.Icons["blocked"]
	if got != want {
		t.Errorf("AgentIcon(Blocked) = %q, want %q", got, want)
	}
}

func TestAgentIcon_Stale(t *testing.T) {
	got := AgentIcon(agent.Stale, defaultSettings)
	want := defaultSettings.Icons["stale"]
	if got != want {
		t.Errorf("AgentIcon(Stale) = %q, want %q", got, want)
	}
}

func TestAgentIcon_Unknown(t *testing.T) {
	got := AgentIcon(agent.State("unknown"), defaultSettings)
	if got != "" {
		t.Errorf("AgentIcon(unknown) = %q, want empty string", got)
	}
}

func TestAgentIcon_Empty(t *testing.T) {
	got := AgentIcon(agent.State(""), defaultSettings)
	if got != "" {
		t.Errorf("AgentIcon(empty) = %q, want empty string", got)
	}
}

// GitBadge tests

func TestGitBadge_NoRepo(t *testing.T) {
	g := gitstatus.Status{IsRepo: false}
	got := GitBadge(g, defaultSettings)
	want := defaultSettings.Icons["no_git"]
	if got != want {
		t.Errorf("GitBadge(no-repo) = %q, want %q", got, want)
	}
}

func TestGitBadge_CleanInSync(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       true,
		HasUpstream: true,
		Ahead:       0,
		Behind:      0,
	}
	got := GitBadge(g, defaultSettings)
	want := "✔ ="
	if got != want {
		t.Errorf("GitBadge(clean+insync) = %q, want %q", got, want)
	}
}

func TestGitBadge_CleanNoUpstream(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       true,
		HasUpstream: false,
	}
	got := GitBadge(g, defaultSettings)
	want := "✔"
	if got != want {
		t.Errorf("GitBadge(clean+no-upstream) = %q, want %q", got, want)
	}
}

func TestGitBadge_CleanAhead(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       true,
		HasUpstream: true,
		Ahead:       2,
		Behind:      0,
	}
	got := GitBadge(g, defaultSettings)
	want := "✔ ↑"
	if got != want {
		t.Errorf("GitBadge(clean+ahead) = %q, want %q", got, want)
	}
}

func TestGitBadge_CleanBehind(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       true,
		HasUpstream: true,
		Ahead:       0,
		Behind:      3,
	}
	got := GitBadge(g, defaultSettings)
	want := "✔ ↓"
	if got != want {
		t.Errorf("GitBadge(clean+behind) = %q, want %q", got, want)
	}
}

func TestGitBadge_CleanDiverged(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       true,
		HasUpstream: true,
		Ahead:       1,
		Behind:      2,
	}
	got := GitBadge(g, defaultSettings)
	want := "✔ <>"
	if got != want {
		t.Errorf("GitBadge(clean+diverged) = %q, want %q", got, want)
	}
}

func TestGitBadge_StagedUntrackedAhead(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       false,
		Staged:      true,
		Untracked:   true,
		HasUpstream: true,
		Ahead:       2,
		Behind:      0,
	}
	got := GitBadge(g, defaultSettings)
	want := "● … ↑"
	if got != want {
		t.Errorf("GitBadge(staged+untracked+ahead) = %q, want %q", got, want)
	}
}

func TestGitBadge_BothAheadAndBehind(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       false,
		HasUpstream: true,
		Ahead:       1,
		Behind:      3,
	}
	got := GitBadge(g, defaultSettings)
	want := "<>"
	if got != want {
		t.Errorf("GitBadge(ahead+behind) = %q, want %q", got, want)
	}
}

func TestGitBadge_AllDirtyFlags(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       false,
		Staged:      true,
		Modified:    true,
		Untracked:   true,
		Stashed:     true,
		Conflict:    true,
		HasUpstream: true,
		Ahead:       0,
		Behind:      0,
	}
	got := GitBadge(g, defaultSettings)
	want := "● ✚ … ⚑ ✖ ="
	if got != want {
		t.Errorf("GitBadge(all-dirty+insync) = %q, want %q", got, want)
	}
}

func TestGitBadge_ModifiedBehind(t *testing.T) {
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       false,
		Modified:    true,
		HasUpstream: true,
		Ahead:       0,
		Behind:      1,
	}
	got := GitBadge(g, defaultSettings)
	want := "✚ ↓"
	if got != want {
		t.Errorf("GitBadge(modified+behind) = %q, want %q", got, want)
	}
}

// StatusLine tests

func TestStatusLine_NoAgentActive(t *testing.T) {
	g := gitstatus.Status{IsRepo: false}
	got := StatusLine(agent.Status{}, agent.State(""), 0, g, true, defaultSettings)
	// no git badge (no_git) and active icon
	wantGit := defaultSettings.Icons["no_git"]
	wantActive := defaultSettings.Icons["active"]
	want := wantGit + "  " + wantActive
	if got != want {
		t.Errorf("StatusLine(no-agent+active) = %q, want %q", got, want)
	}
}

func TestStatusLine_NoAgentNotActive(t *testing.T) {
	g := gitstatus.Status{IsRepo: false}
	got := StatusLine(agent.Status{}, agent.State(""), 0, g, false, defaultSettings)
	// no git badge only
	want := defaultSettings.Icons["no_git"]
	if got != want {
		t.Errorf("StatusLine(no-agent+not-active) = %q, want %q", got, want)
	}
}

func TestStatusLine_AgentWorking_OtherCount2(t *testing.T) {
	g := gitstatus.Status{IsRepo: false}
	got := StatusLine(agent.Status{State: agent.Working}, agent.Working, 2, g, true, defaultSettings)
	wantGit := defaultSettings.Icons["no_git"]
	wantAgent := defaultSettings.Icons["working"] + "+2"
	want := wantGit + "  " + wantAgent
	if got != want {
		t.Errorf("StatusLine(working+otherCount=2) = %q, want %q", got, want)
	}
}

func TestStatusLine_AgentBlocked_OtherCount0(t *testing.T) {
	g := gitstatus.Status{IsRepo: false}
	got := StatusLine(agent.Status{State: agent.Blocked}, agent.Blocked, 0, g, true, defaultSettings)
	wantGit := defaultSettings.Icons["no_git"]
	wantAgent := defaultSettings.Icons["blocked"]
	want := wantGit + "  " + wantAgent
	if got != want {
		t.Errorf("StatusLine(blocked+otherCount=0) = %q, want %q", got, want)
	}
}

func TestStatusLine_EmptyGitWithAgent(t *testing.T) {
	// Clean repo with upstream in sync, working agent
	g := gitstatus.Status{
		IsRepo:      true,
		Clean:       true,
		HasUpstream: true,
		Ahead:       0,
		Behind:      0,
	}
	got := StatusLine(agent.Status{State: agent.Working}, agent.Working, 0, g, true, defaultSettings)
	wantGit := "✔ ="
	wantAgent := defaultSettings.Icons["working"]
	want := wantGit + "  " + wantAgent
	if got != want {
		t.Errorf("StatusLine(clean-git+agent) = %q, want %q", got, want)
	}
}

func TestStatusLine_GitOnlyNoBothParts(t *testing.T) {
	// Active git, no agent, not active session - just git badge
	g := gitstatus.Status{
		IsRepo: true,
		Clean:  true,
	}
	got := StatusLine(agent.Status{}, agent.State(""), 0, g, false, defaultSettings)
	want := "✔"
	if got != want {
		t.Errorf("StatusLine(git-only) = %q, want %q", got, want)
	}
}
