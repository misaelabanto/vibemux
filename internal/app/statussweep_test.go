package app

import (
	"runtime"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/misaelabanto/vibemux/internal/gitstatus"
)

// TestSweepGroupCoalescesConcurrentCalls covers the burst that arrives when a
// multiplexer session is left: several refresh triggers are dequeued together
// and must cost one sweep, not one each.
func TestSweepGroupCoalescesConcurrentCalls(t *testing.T) {
	var group sweepGroup
	var computed atomic.Int32

	release := make(chan struct{})
	started := make(chan struct{})

	compute := func() StatusComputedMsg {
		if computed.Add(1) == 1 {
			close(started)
		}
		<-release
		return StatusComputedMsg{Active: map[string]bool{"p1": true}}
	}

	const callers = 3
	results := make([]StatusComputedMsg, callers)
	var wg sync.WaitGroup

	// First caller starts the sweep and blocks inside compute.
	wg.Add(1)
	go func() {
		defer wg.Done()
		results[0] = group.do(compute)
	}()
	<-started

	// The rest arrive while it is in flight and must join it. The sweep is held
	// open until every one of them has actually joined, so the assertion below
	// measures coalescing rather than goroutine scheduling luck.
	for i := 1; i < callers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = group.do(compute)
		}(i)
	}
	for group.joinedCount() < callers-1 {
		runtime.Gosched()
	}

	close(release)
	wg.Wait()

	if got := computed.Load(); got != 1 {
		t.Errorf("compute ran %d times for %d concurrent callers, want 1", got, callers)
	}
	for i, r := range results {
		if !r.Active["p1"] {
			t.Errorf("caller %d got %+v, want the shared sweep result", i, r)
		}
	}
}

// TestSweepGroupRunsAgainAfterCompletion verifies coalescing does not latch:
// a sweep requested after the previous one finished must actually run.
func TestSweepGroupRunsAgainAfterCompletion(t *testing.T) {
	var group sweepGroup
	var computed atomic.Int32

	compute := func() StatusComputedMsg {
		computed.Add(1)
		return StatusComputedMsg{}
	}

	for i := 0; i < 3; i++ {
		group.do(compute)
	}

	if got := computed.Load(); got != 3 {
		t.Errorf("compute ran %d times for 3 sequential calls, want 3", got)
	}
}

// TestSweepGroupPropagatesResult guards that the coalesced result carries every
// field, not just the one the burst test checks.
func TestSweepGroupPropagatesResult(t *testing.T) {
	var group sweepGroup

	want := StatusComputedMsg{
		Active: map[string]bool{"p1": true},
		Git:    map[string]gitstatus.Status{"p1": {IsRepo: true, Clean: true}},
	}
	got := group.do(func() StatusComputedMsg { return want })

	if !got.Active["p1"] || !got.Git["p1"].Clean {
		t.Errorf("do() = %+v, want %+v", got, want)
	}
}
