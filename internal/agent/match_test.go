package agent

import (
	"testing"
	"time"

	"github.com/misaelabanto/vibemux/internal/model"
)

// helpers

func proj(id, path string) model.Project {
	return model.Project{ID: id, Name: id, Path: path}
}

func status(cwd string, state State, updatedAt time.Time) Status {
	return Status{Cwd: cwd, SessionID: cwd, State: state, UpdatedAt: updatedAt}
}

var (
	t0 = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t1 = t0.Add(1 * time.Minute)
	t2 = t0.Add(2 * time.Minute)
	t3 = t0.Add(3 * time.Minute)
)

// OwningProjectID

func TestOwningProjectID_ExactMatch(t *testing.T) {
	projects := []model.Project{proj("p1", "/code/app")}
	id, ok := OwningProjectID("/code/app", projects)
	if !ok || id != "p1" {
		t.Errorf("expected (p1, true), got (%q, %v)", id, ok)
	}
}

func TestOwningProjectID_NestedPicksLongest(t *testing.T) {
	// /code/mono/api/src -> should match /code/mono/api, not /code/mono
	projects := []model.Project{
		proj("mono", "/code/mono"),
		proj("api", "/code/mono/api"),
	}
	id, ok := OwningProjectID("/code/mono/api/src", projects)
	if !ok || id != "api" {
		t.Errorf("expected (api, true), got (%q, %v)", id, ok)
	}
}

func TestOwningProjectID_SiblingPrefixNotAncestor(t *testing.T) {
	// /a/bc is NOT an ancestor of /a/b
	projects := []model.Project{proj("bc", "/a/bc")}
	_, ok := OwningProjectID("/a/b", projects)
	if ok {
		t.Error("expected no match for sibling prefix /a/bc vs cwd /a/b")
	}
}

func TestOwningProjectID_NoMatch(t *testing.T) {
	projects := []model.Project{proj("p1", "/code/app")}
	_, ok := OwningProjectID("/other/thing", projects)
	if ok {
		t.Error("expected no match")
	}
}

// GroupByProject

func TestGroupByProject_BucketsCorrectly(t *testing.T) {
	projects := []model.Project{
		proj("p1", "/a"),
		proj("p2", "/b"),
	}
	statuses := []Status{
		status("/a/x", Working, t0),
		status("/a/y", Done, t1),
		status("/b/z", Blocked, t2),
		status("/unknown", Working, t3),
	}
	m := GroupByProject(statuses, projects)

	if len(m["p1"]) != 2 {
		t.Errorf("expected 2 statuses in p1, got %d", len(m["p1"]))
	}
	if len(m["p2"]) != 1 {
		t.Errorf("expected 1 status in p2, got %d", len(m["p2"]))
	}
	if len(m[""]) != 1 {
		t.Errorf("expected 1 unmatched status (key \"\"), got %d", len(m[""]))
	}
}

// DerivedState

func TestDerivedState_Working_BelowThreshold_StaysWorking(t *testing.T) {
	s := status("/a", Working, t0)
	got := DerivedState(s, 10*time.Minute, t0.Add(5*time.Minute))
	if got != Working {
		t.Errorf("expected Working, got %v", got)
	}
}

func TestDerivedState_Working_PastThreshold_BecomesStale(t *testing.T) {
	s := status("/a", Working, t0)
	got := DerivedState(s, 10*time.Minute, t0.Add(11*time.Minute))
	if got != Stale {
		t.Errorf("expected Stale, got %v", got)
	}
}

func TestDerivedState_Done_NeverStale(t *testing.T) {
	s := status("/a", Done, t0)
	got := DerivedState(s, 10*time.Minute, t0.Add(100*time.Minute))
	if got != Done {
		t.Errorf("expected Done, got %v", got)
	}
}

func TestDerivedState_Blocked_NeverStale(t *testing.T) {
	s := status("/a", Blocked, t0)
	got := DerivedState(s, 10*time.Minute, t0.Add(100*time.Minute))
	if got != Blocked {
		t.Errorf("expected Blocked, got %v", got)
	}
}

// SortByUrgency

func TestSortByUrgency_Order(t *testing.T) {
	threshold := 10 * time.Minute
	now := t0.Add(20 * time.Minute)

	// stale = Working updated at t0 (20 min ago > 10 min threshold)
	staleS := status("/stale", Working, t0)
	// working = Working updated at now (fresh)
	workingS := status("/working", Working, now)
	// blocked
	blockedS := status("/blocked", Blocked, t0)
	// done
	doneS := status("/done", Done, t0)

	input := []Status{doneS, workingS, staleS, blockedS}
	got := SortByUrgency(input, threshold, now)

	wantOrder := []State{Blocked, Stale, Working, Done}
	for i, want := range wantOrder {
		got_state := DerivedState(got[i], threshold, now)
		if got_state != want {
			t.Errorf("position %d: expected %v, got %v", i, want, got_state)
		}
	}
}

func TestSortByUrgency_RecencyTiebreak(t *testing.T) {
	threshold := 10 * time.Minute
	now := t0.Add(1 * time.Hour)

	// two blocked statuses - more recent should come first
	older := Status{Cwd: "/a", State: Blocked, UpdatedAt: t0}
	newer := Status{Cwd: "/b", State: Blocked, UpdatedAt: t0.Add(5 * time.Minute)}

	got := SortByUrgency([]Status{older, newer}, threshold, now)
	if got[0].Cwd != "/b" {
		t.Errorf("expected newer (/b) first, got %s", got[0].Cwd)
	}
}

func TestSortByUrgency_DoesNotMutateInput(t *testing.T) {
	threshold := 10 * time.Minute
	now := t0.Add(1 * time.Hour)

	input := []Status{
		status("/done", Done, t0),
		status("/blocked", Blocked, t1),
	}
	orig0 := input[0]
	_ = SortByUrgency(input, threshold, now)
	if input[0] != orig0 {
		t.Error("SortByUrgency mutated the input slice")
	}
}
