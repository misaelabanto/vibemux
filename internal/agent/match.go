package agent

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/misaelabanto/vibemux/internal/model"
)

// Stale is the derived state for a Working agent that has not updated within the stale threshold.
const Stale State = "stale"

// urgencyRank maps a derived State to a sort priority (lower = higher urgency).
func urgencyRank(s State) int {
	switch s {
	case Blocked:
		return 0
	case Stale:
		return 1
	case Working:
		return 2
	case Done:
		return 3
	default:
		return 4
	}
}

// OwningProjectID returns the ID of the project whose Path is the longest
// ancestor (or exact match) of cwd, after filepath.Clean.
// A path p is an ancestor of cwd only when cwd == p OR cwd starts with
// p + string(os.PathSeparator), so /a/bc is NOT an ancestor of /a/b.
func OwningProjectID(cwd string, projects []model.Project) (string, bool) {
	cleanCwd := filepath.Clean(cwd)
	bestID := ""
	bestLen := -1

	for _, p := range projects {
		cleanPath := filepath.Clean(p.Path)
		if cleanCwd == cleanPath {
			if len(cleanPath) > bestLen {
				bestLen = len(cleanPath)
				bestID = p.ID
			}
			continue
		}
		prefix := cleanPath + string(os.PathSeparator)
		if len(prefix) <= len(cleanCwd) && cleanCwd[:len(prefix)] == prefix {
			if len(cleanPath) > bestLen {
				bestLen = len(cleanPath)
				bestID = p.ID
			}
		}
	}

	if bestLen == -1 {
		return "", false
	}
	return bestID, true
}

// GroupByProject groups statuses by owning project ID.
// Statuses with no matching project are placed under the empty string key.
func GroupByProject(statuses []Status, projects []model.Project) map[string][]Status {
	result := make(map[string][]Status)
	for _, s := range statuses {
		id, _ := OwningProjectID(s.Cwd, projects)
		result[id] = append(result[id], s)
	}
	return result
}

// DerivedState returns Stale when s.State == Working and the status has not
// been updated within staleThreshold, otherwise returns s.State unchanged.
func DerivedState(s Status, staleThreshold time.Duration, now time.Time) State {
	if s.State == Working && now.Sub(s.UpdatedAt) > staleThreshold {
		return Stale
	}
	return s.State
}

// SortByUrgency returns a new slice sorted by urgency: Blocked > Stale > Working > Done.
// Within equal urgency, more-recent UpdatedAt comes first.
// The input slice is not mutated.
func SortByUrgency(ss []Status, staleThreshold time.Duration, now time.Time) []Status {
	out := make([]Status, len(ss))
	copy(out, ss)

	sort.SliceStable(out, func(i, j int) bool {
		ri := urgencyRank(DerivedState(out[i], staleThreshold, now))
		rj := urgencyRank(DerivedState(out[j], staleThreshold, now))
		if ri != rj {
			return ri < rj
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	return out
}
