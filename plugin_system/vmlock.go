package plugin_system

import (
	"context"
	"errors"
	"fmt"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// vmMutex is a plugin VM's exclusive lock.
//
// A channel rather than a sync.Mutex, because acquiring one has to be
// abandonable. sync.Mutex.Lock takes no context and cannot be interrupted, and
// building a cancellable wait out of TryLock in a poll loop is worse than it
// looks. A waiter that has been queued longer than a millisecond flips the
// mutex into starvation mode — not on a timer, but when it is next woken and
// finds the lock taken again — and in that mode ownership is handed off
// directly to the head of the wait queue while arriving goroutines are made to
// queue rather than barge (internal/sync/mutex.go: starvationThresholdNs, and
// the "new arriving goroutines must queue" branch). A poller never joins that
// queue, so no handoff can ever reach it, and
// TryLock refuses outright while the starving bit is set — so for as long as
// the mutex stays in that mode it cannot acquire at all. Light contention never
// reaches it and a poller does fine there; sustained contention is the case
// that starves it, and it is the same case under which giving up matters.
//
// A buffered channel of capacity one is instead a lock whose acquisition is an
// ordinary channel send: selectable against a context. Blocked senders are also
// woken in arrival order, since the runtime's sendq is a FIFO list — an
// implementation property of the gc runtime rather than a language guarantee,
// so nothing here is built on it.
type vmMutex struct {
	ch chan struct{}
}

func newVMMutex() *vmMutex { return &vmMutex{ch: make(chan struct{}, 1)} }

// Lock waits indefinitely, as sync.Mutex.Lock does.
func (m *vmMutex) Lock() { m.ch <- struct{}{} }

// TryLock acquires the lock if it is free, reporting whether it did.
func (m *vmMutex) TryLock() bool {
	select {
	case m.ch <- struct{}{}:
		return true
	default:
		return false
	}
}

// LockWithin waits for the lock until ctx ends or wait elapses, whichever comes
// first, reporting whether it was acquired. A zero wait means "no deadline of
// its own", so only ctx can stop it; a nil or never-cancelled ctx with a zero
// wait is Lock.
func (m *vmMutex) LockWithin(ctx context.Context, wait time.Duration) bool {
	// Fast path, and also the one that keeps an already-cancelled caller
	// working when the lock happens to be free. Refusing on ctx.Err() first
	// would fail a call that had nothing to wait for.
	if m.TryLock() {
		return true
	}

	var timeout <-chan time.Time
	if wait > 0 {
		t := time.NewTimer(wait)
		defer t.Stop()
		timeout = t.C
	}
	var abandoned <-chan struct{}
	if ctx != nil {
		abandoned = ctx.Done()
	}

	select {
	case m.ch <- struct{}{}:
		return true
	case <-abandoned:
		return false
	case <-timeout:
		return false
	}
}

// Unlock releases the lock, panicking on an unlocked one.
//
// Deliberately a panic and not sync.Mutex's answer, which is fatal("sync:
// unlock of unlocked mutex") — an abort no recover can catch. Taking the whole
// process down over one plugin's bookkeeping is the wrong trade in a host whose
// entire job is running untrusted code, so this stays recoverable. The cost is
// that a recover somewhere up the stack can swallow it, which is why the
// message names the package: a swallowed one still has to be findable in a log.
//
// The guard is not decoration. Draining an empty channel blocks forever, so a
// bare receive would turn a double unlock from a loud crash into a silent wedge
// of the whole plugin — the failure this package has the most history with.
//
// It cannot fire spuriously. Only the holder unlocks, and nothing else removes
// the buffered value, so the buffer is non-empty for as long as this caller
// holds the lock. A sender blocked at the same instant does not open a window
// either: on a full buffer the runtime hands the head slot to the receiver and
// refills that same slot from the sender in one step under the channel's lock
// (runtime/chan.go, recv), so the count never transiently reads zero.
func (m *vmMutex) Unlock() {
	select {
	case <-m.ch:
	default:
		panic("plugin_system: Unlock of an unlocked VM lock")
	}
}

// errVMWaitAbandoned is what a caller gets when its own context ended while it
// was queued behind another call into the same plugin.
//
// It is deliberately distinct from the "plugin is no longer available" answer a
// disabled plugin produces. Those are different events with different
// remedies, and only one of them is about the plugin at all.
var errVMWaitAbandoned = errors.New("plugin was busy and the request was abandoned before its VM became free")

// lockVMForRequest is how every request-serving surface acquires a plugin's VM.
//
// One helper rather than the same six lines at each site, because the two
// failures have to stay distinguishable and a site that collapses them reads
// perfectly well: "plugin is no longer available" is a plausible-looking answer
// for an abandoned request, and wrong.
func (pm *PluginManager) lockVMForRequest(ctx context.Context, L *lua.LState, pluginName string) (*vmMutex, error) {
	mu, err := pm.LockVMWithContext(ctx, L)
	if err != nil {
		return nil, fmt.Errorf("plugin %q: %w", pluginName, err)
	}
	if mu == nil {
		// Disabled between the registry lookup and here, so its state is
		// closing and must not be touched.
		return nil, fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	return mu, nil
}
