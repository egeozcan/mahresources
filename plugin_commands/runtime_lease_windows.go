//go:build windows

package plugin_commands

// RuntimeLease is inert on Windows because plugin command execution is refused
// there; the type keeps manifest loading and host construction portable. An
// inert lease cannot report ErrRuntimeLeaseBusy.
type RuntimeLease struct{}

func AcquireRuntimeLease(string) (*RuntimeLease, error) { return &RuntimeLease{}, nil }
func (*RuntimeLease) Close() error                      { return nil }
