package plugin_commands

import (
	"errors"
	"fmt"
	"time"
)

// ErrCommandRuntimeQuarantined reports that command and exchange operations are
// temporarily unavailable while the host retries safe runtime activation.
var ErrCommandRuntimeQuarantined = errors.New("plugin command runtime is quarantined")

// ErrRuntimeFenceLost reports that this process no longer owns the command
// runtime's database fence: it released it at shutdown, or another runtime took
// it. A durable write refused for it can never succeed from this process, so it
// is not retried; the runtime that owns the fence recovers the row.
var ErrRuntimeFenceLost = errors.New("plugin command runtime fence is not owned")

// RuntimeQuarantinedError carries the current safety refusal and, when known,
// how long remains before the next automatic activation attempt.
type RuntimeQuarantinedError struct {
	Reason             string
	RetryAfterDuration time.Duration
}

func (e *RuntimeQuarantinedError) Error() string {
	if e == nil || e.Reason == "" {
		return ErrCommandRuntimeQuarantined.Error()
	}
	return fmt.Sprintf("%s: %s", ErrCommandRuntimeQuarantined, e.Reason)
}

func (e *RuntimeQuarantinedError) Unwrap() error {
	return ErrCommandRuntimeQuarantined
}

func (e *RuntimeQuarantinedError) RetryAfter() time.Duration {
	if e == nil || e.RetryAfterDuration < 0 {
		return 0
	}
	return e.RetryAfterDuration
}
