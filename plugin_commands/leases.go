package plugin_commands

import (
	"fmt"
	"sync"
)

// LeaseManager serializes per-run file operations against the sweep decision.
// A sweep holds the run in the sweeping state from its skip check through its
// delete, so no operation can acquire a lease in that gap.
type LeaseManager struct {
	mu   sync.Mutex
	runs map[string]*runLeaseState
}

type runLeaseState struct {
	active   int
	sweeping bool
}

func NewLeaseManager() *LeaseManager {
	return &LeaseManager{runs: make(map[string]*runLeaseState)}
}

// Acquire pins a run until the returned release function is called. Import
// submission must acquire this pin when its durable claim is admitted, before
// waiting for a worker slot.
func (m *LeaseManager) Acquire(runID string) (func(), error) {
	if m == nil {
		return nil, fmt.Errorf("run swept: lease manager is unavailable")
	}
	m.mu.Lock()
	state := m.stateLocked(runID)
	if state.sweeping {
		m.mu.Unlock()
		return nil, fmt.Errorf("run swept: exchange folder is being removed")
	}
	state.active++
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			state := m.stateLocked(runID)
			if state.active > 0 {
				state.active--
			}
			if state.active == 0 && !state.sweeping {
				delete(m.runs, runID)
			}
			m.mu.Unlock()
		})
	}, nil
}

// BeginSweep atomically checks for active operations and reserves the delete
// decision. The caller must invoke the returned function after either deleting
// or abandoning the sweep. New acquisitions are refused until then.
func (m *LeaseManager) BeginSweep(runID string) (func(), bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	state := m.stateLocked(runID)
	if state.active != 0 || state.sweeping {
		m.mu.Unlock()
		return nil, false
	}
	state.sweeping = true
	m.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			m.mu.Lock()
			state := m.stateLocked(runID)
			state.sweeping = false
			if state.active == 0 {
				delete(m.runs, runID)
			}
			m.mu.Unlock()
		})
	}, true
}

func (m *LeaseManager) stateLocked(runID string) *runLeaseState {
	state := m.runs[runID]
	if state == nil {
		state = &runLeaseState{}
		m.runs[runID] = state
	}
	return state
}
