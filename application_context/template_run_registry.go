package application_context

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// MaxLiveTemplateRunsPerAccount bounds the generation runs one account has in
// flight (plan section 3, "Registry").
const MaxLiveTemplateRunsPerAccount = 2

// ErrTemplateRunLimit is a run refused because its account already has
// MaxLiveTemplateRunsPerAccount live runs.
var ErrTemplateRunLimit = errors.New("too many generation runs are in progress for this account; finish or cancel one first")

// templateRunRegistry maps a random run ID to its account, its Finish now flag
// and its cancel function. A run is registered before it starts and released,
// by a deferred call, on every exit path.
type templateRunRegistry struct {
	mu   sync.Mutex
	runs map[string]*TemplateRunHandle
}

func newTemplateRunRegistry() *templateRunRegistry {
	return &templateRunRegistry{runs: map[string]*TemplateRunHandle{}}
}

// TemplateRunHandle is one registered run.
type TemplateRunHandle struct {
	ID       string
	account  string
	cancel   context.CancelFunc
	finish   atomic.Bool
	registry *templateRunRegistry
	once     sync.Once
}

// FinishRequested reports whether Finish now was requested. It is the run
// environment's Finish function.
func (h *TemplateRunHandle) FinishRequested() bool { return h.finish.Load() }

// Cancel cancels the run's context.
func (h *TemplateRunHandle) Cancel() {
	if h.cancel != nil {
		h.cancel()
	}
}

// Release frees the run's entry. It is safe to call more than once; defer it
// right after Start.
func (h *TemplateRunHandle) Release() {
	h.once.Do(func() {
		h.registry.mu.Lock()
		delete(h.registry.runs, h.ID)
		h.registry.mu.Unlock()
	})
}

// start registers a run for an account, or returns ErrTemplateRunLimit.
func (r *templateRunRegistry) start(account string, cancel context.CancelFunc) (*TemplateRunHandle, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return nil, fmt.Errorf("could not create a run ID: %w", err)
	}
	h := &TemplateRunHandle{ID: hex.EncodeToString(buf), account: account, cancel: cancel, registry: r}
	r.mu.Lock()
	defer r.mu.Unlock()
	live := 0
	for _, other := range r.runs {
		if other.account == account {
			live++
		}
	}
	if live >= MaxLiveTemplateRunsPerAccount {
		return nil, ErrTemplateRunLimit
	}
	r.runs[h.ID] = h
	return h, nil
}

// finish sets the Finish now flag of a run when the account matches. A run
// that is unknown or belongs to another account is not found, so run IDs
// cannot be probed.
func (r *templateRunRegistry) finish(runID, account string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	h, ok := r.runs[runID]
	if !ok || h.account != account {
		return false
	}
	h.finish.Store(true)
	return true
}

func (r *templateRunRegistry) live(account string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, h := range r.runs {
		if h.account == account {
			n++
		}
	}
	return n
}

var templateRuns = newTemplateRunRegistry()

// TemplateRunAccountKey is the key runs are limited and finished by: the user ID
// when accounts are enabled, else the client address.
func TemplateRunAccountKey(userID uint, clientIP string) string {
	if userID != 0 {
		return fmt.Sprintf("user:%d", userID)
	}
	return "ip:" + clientIP
}

// StartTemplateRun registers a run on the process-wide registry. The caller
// defers Release on the handle, passes handle.FinishRequested as the run
// environment's Finish and handle.ID to the client.
func StartTemplateRun(accountKey string, cancel context.CancelFunc) (*TemplateRunHandle, error) {
	return templateRuns.start(accountKey, cancel)
}

// FinishTemplateRun is the finish route's lookup: it asks the run to make its
// next Turn the final one, if the account owns it.
func FinishTemplateRun(runID, accountKey string) bool {
	return templateRuns.finish(runID, accountKey)
}
