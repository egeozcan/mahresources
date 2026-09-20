package plugin_commands

import (
	"errors"
	"fmt"
	"time"
)

// ErrCommandRuntimeQuarantined reports that command and exchange operations are
// temporarily unavailable while the host retries safe runtime activation.
var ErrCommandRuntimeQuarantined = errors.New("plugin command runtime is quarantined")

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
