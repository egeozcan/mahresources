package application_context

import (
	"errors"
	"sync/atomic"
)

// This file holds the one test-only seam the Job control plane's failure paths
// need.
//
// Every durable write here happens on a goroutine the caller does not own — a
// queue-backed execution's waiter, the plugin manager's settle path, a heartbeat —
// so "the database refused this write" and "the Job row could not be read" cannot
// be produced from outside the process by any means short of a real outage. The
// behaviour those failures must have is nevertheless a contract: a refused
// progress write is telemetry rather than completion, and an unacknowledged
// terminal outcome is retried rather than assumed published.
//
// So the faults are injectable, in the shape `queueClaimLease` and
// `scopedPluginAccess.ttl` already established for the same reason: nil is the
// production value, every use is a nil check, and only tests set it. The type is
// guarded by atomics rather than by a mutex because the goroutines under test read
// it while the test writes it, and the race detector is part of the verification.

// errJobFaultInjected is what an injected failure reports. It is deliberately not
// a jobs-module error: nothing classifies it, and every path that sees it treats
// it as an unexpected write failure.
var errJobFaultInjected = errors.New("job durability fault injected by a test")

// jobDurabilityFaults is the set of failures a test may inject.
type jobDurabilityFaults struct {
	// failProgressWrite refuses the next progress mirror the queue bridge makes
	// for a Job, once. Once, because the failure under test is transient: the
	// executor keeps running, and the next tick has to be able to record where it
	// has got to.
	failProgressWrite atomic.Bool
	// failSettlementRead refuses every read the plugin-action sink makes to ask
	// whether its Job is already finished, until the test clears it. A read that
	// fails is not a Job that finished, and the retry this drives is only
	// observable while the failure lasts.
	failSettlementRead atomic.Bool
	// failSettlementWrite refuses every terminal write the plugin-action sink
	// makes, until the test clears it, for the same reason.
	failSettlementWrite atomic.Bool
}

// progressWrite refuses one progress mirror, once.
func (f *jobDurabilityFaults) progressWrite() error {
	if f == nil || !f.failProgressWrite.CompareAndSwap(true, false) {
		return nil
	}
	return errJobFaultInjected
}

// settlementRead reports a refusal of every read the sink makes while the flag is
// set.
func (f *jobDurabilityFaults) settlementRead() error {
	if f == nil || !f.failSettlementRead.Load() {
		return nil
	}
	return errJobFaultInjected
}

// settlementWrite is the same for the sink's terminal write.
func (f *jobDurabilityFaults) settlementWrite() error {
	if f == nil || !f.failSettlementWrite.Load() {
		return nil
	}
	return errJobFaultInjected
}
