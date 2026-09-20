package plugin_commands

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeQuarantinedErrorContract(t *testing.T) {
	err := &RuntimeQuarantinedError{Reason: "lease busy", RetryAfterDuration: 2 * time.Second}
	require.ErrorIs(t, err, ErrCommandRuntimeQuarantined)
	require.Equal(t, 2*time.Second, err.RetryAfter())
	require.Contains(t, err.Error(), "lease busy")
}
