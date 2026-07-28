package app

import "sync"

// sweepCall is one in-flight status sweep and the result it will produce.
type sweepCall struct {
	done   chan struct{}
	result StatusComputedMsg
}

// sweepGroup collapses concurrent status sweeps into a single one.
//
// Returning from a multiplexer session delivers a burst of refresh triggers at
// once: bubbletea's event loop is blocked for the whole time the session is
// attached, so the pending tick, the fetch-on-enter completion, and the return
// itself are all dequeued together. Each one asks for the same status, and
// without coalescing each would spawn a full set of git processes to compute an
// answer the others are already computing, tripling the work on exactly the
// path where the user is waiting to see the dashboard.
//
// A caller arriving while a sweep is in flight waits for that sweep and shares
// its result, so no refresh is dropped.
type sweepGroup struct {
	mu     sync.Mutex
	active *sweepCall
	// joined counts callers that shared an in-flight sweep instead of starting
	// their own. It exists so tests can observe the coalescing.
	joined int
}

// joinedCount reports how many callers have shared an in-flight sweep.
func (g *sweepGroup) joinedCount() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.joined
}

// do runs compute, unless a sweep is already in flight, in which case it waits
// for that one and returns its result.
func (g *sweepGroup) do(compute func() StatusComputedMsg) StatusComputedMsg {
	g.mu.Lock()
	if call := g.active; call != nil {
		g.joined++
		g.mu.Unlock()
		<-call.done
		return call.result
	}
	call := &sweepCall{done: make(chan struct{})}
	g.active = call
	g.mu.Unlock()

	// Written before done is closed, so waiters reading it after the receive
	// are ordered behind this write.
	call.result = compute()

	g.mu.Lock()
	g.active = nil
	g.mu.Unlock()
	close(call.done)

	return call.result
}

// statusSweeps is the process-wide group for computeStatus. The sweep reads
// only external state (the multiplexer, the agent store, the git worktrees), so
// one group for the whole program is enough.
var statusSweeps sweepGroup
