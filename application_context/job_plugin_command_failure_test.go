package application_context

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/jobs"
	"mahresources/plugin_commands"
)

// finishClaimedCommandRun runs one command's canonical Job to the given terminal
// state through the same claimed path the dispatcher uses.
func finishClaimedCommandRun(t *testing.T, runID string, finish plugin_commands.RunFinish) (*MahresourcesContext, *jobs.Service, string) {
	t.Helper()
	return finishClaimedCommandRunAfter(t, runID, finish, nil)
}

// finishClaimedCommandRunAfter runs before between the run starting and its
// finish, for a test that needs the Job in another state first.
func finishClaimedCommandRunAfter(t *testing.T, runID string, finish plugin_commands.RunFinish, before func(*MahresourcesContext)) (*MahresourcesContext, *jobs.Service, string) {
	t.Helper()
	ctx := newPluginCommandStoreTestContext(t)
	service := jobs.NewService()
	ctx.SetJobService(service)
	require.NoError(t, ctx.StartPluginCommands(context.Background(), testPluginCommandSettings{root: t.TempDir(), commandPath: t.TempDir()}))
	t.Cleanup(func() { _ = ctx.StopPluginCommands() })
	now := time.Now().UTC()
	record := testRun(runID, nil, true, now)
	require.NoError(t, ctx.CreateRun(record, testOutput(record.ID, now)))
	stored, _, err := ctx.Run(record.ID)
	require.NoError(t, err)
	_, claimed, err := ctx.claimPluginCommandJob(stored.JobID, JobKindPluginCommand, record.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	won, err := ctx.MarkRunRunning(record.ID, now.Add(time.Second))
	require.NoError(t, err)
	require.True(t, won)
	if before != nil {
		before(ctx)
	}
	finish.FinishedAt = now.Add(2 * time.Second)
	won, err = ctx.FinishRun(record.ID, finish)
	require.NoError(t, err)
	require.True(t, won)
	return ctx, service, stored.JobID
}

// TestAFailedPluginCommandJobNamesWhyItFailed pins that a command's Job says
// why it failed, in the words its run history records, and classes a timeout
// as one: the Job is where a person looks first, and "did not complete
// successfully" sent them to an admin page nothing linked to. The Job also
// keeps its command history output, as a successful one does.
func TestAFailedPluginCommandJobNamesWhyItFailed(t *testing.T) {
	three := 3
	for _, tc := range []struct {
		name    string
		finish  plugin_commands.RunFinish
		code    string
		class   string
		message string
	}{
		{"exit status", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed, ExitCode: &three,
			Error: "command exited with status 3", Cause: plugin_commands.RunCauseExitStatus},
			"plugin-command-exit-status", jobs.FailureClassDependency, "command exited with status 3"},
		{"timeout", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed,
			Error: "command timeout exceeded (45s)", Cause: plugin_commands.RunCauseTimeout},
			"plugin-command-timeout", jobs.FailureClassTimeout, "command timeout exceeded (45s)"},
		{"quota", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed,
			Error: "per-run quota exceeded: 10 bytes used, limit 5", Cause: plugin_commands.RunCauseQuota},
			"plugin-command-quota", jobs.FailureClassCapacity, "per-run quota exceeded: 10 bytes used, limit 5"},
		{"another reason", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed,
			Error: "start command: permission denied"},
			"plugin-command-failed", jobs.FailureClassInternal, "start command: permission denied"},
		{"no reason recorded", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed},
			"plugin-command-failed", jobs.FailureClassInternal, "the plugin command did not complete successfully"},
		{"a reason longer than a failure message", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed,
			Error: strings.Repeat("é", jobs.MaxFailureMessageBytes)},
			"plugin-command-failed", jobs.FailureClassInternal, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, service, jobID := finishClaimedCommandRun(t, "failed-"+strings.ReplaceAll(tc.name, " ", "-"), tc.finish)
			snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
			require.NoError(t, err)
			require.Equal(t, jobs.StateFailed, snapshot.State)
			require.NotNil(t, snapshot.Failure)
			require.Equal(t, tc.code, snapshot.Failure.Code)
			require.Equal(t, tc.class, snapshot.Failure.Class)
			if tc.message != "" {
				require.Equal(t, tc.message, snapshot.Failure.Message)
			} else {
				require.LessOrEqual(t, len(snapshot.Failure.Message), jobs.MaxFailureMessageBytes)
				require.True(t, strings.HasPrefix(tc.finish.Error, strings.TrimSuffix(snapshot.Failure.Message, "…")))
			}
			outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			require.Equal(t, "command-history", outputs[0].Key)
		})
	}
}

// TestACancelledOrInterruptedCommandJobKeepsItsHistory covers the other ends a
// started run can reach: its Job keeps the command history output that says
// where the run's record is, and carries no failure.
func TestACancelledOrInterruptedCommandJobKeepsItsHistory(t *testing.T) {
	for _, status := range []string{plugin_commands.RunStatusCancelled, plugin_commands.RunStatusInterrupted} {
		t.Run(status, func(t *testing.T) {
			ctx, service, jobID := finishClaimedCommandRun(t, "ended-"+status, plugin_commands.RunFinish{Status: status, Error: "operator cancelled"})
			snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
			require.NoError(t, err)
			require.Nil(t, snapshot.Failure)
			outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
			require.NoError(t, err)
			require.Len(t, outputs, 1)
			require.Equal(t, "command-history", outputs[0].Key)
		})
	}
}

// TestARecoveredCommandJobKeepsItsHistory covers a run whose Job recovery
// blocked while its process group could not be settled: once the group is
// gone the run is interrupted, and the blocked Job ends with its history
// output under the execution token it kept while blocked.
func TestARecoveredCommandJobKeepsItsHistory(t *testing.T) {
	ctx, service, jobID := finishClaimedCommandRunAfter(t, "recovered-run",
		plugin_commands.RunFinish{Status: plugin_commands.RunStatusInterrupted, Error: "server interrupted while command was running"},
		func(ctx *MahresourcesContext) {
			require.NoError(t, ctx.QuarantineRun(plugin_commands.RecoveryBlocker{RunID: "recovered-run", ProcessGroupID: 4242, Reason: "alive"}))
			stored, _, err := ctx.Run("recovered-run")
			require.NoError(t, err)
			blocked, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, stored.JobID)
			require.NoError(t, err)
			require.Equal(t, jobs.StateBlocked, blocked.State)
		})
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	require.NoError(t, err)
	require.Equal(t, jobs.StateInterrupted, snapshot.State)
	outputs, err := service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	require.NoError(t, err)
	require.Len(t, outputs, 1)
	require.Equal(t, "command-history", outputs[0].Key)
}

// TestInspectingACommandJobOpensItsRunHistory pins that "Inspect command
// history" takes the reader somewhere: its outcome names the run's page, which
// the Job Center follows, for a command Job and for an import Job alike.
func TestInspectingACommandJobOpensItsRunHistory(t *testing.T) {
	ctx, service, jobID := finishClaimedCommandRun(t, "inspected-run", plugin_commands.RunFinish{Status: plugin_commands.RunStatusFailed,
		Error: "command exited with status 3"})
	snapshot, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	require.NoError(t, err)
	adapter := &pluginCommandJobAdapter{ctx: ctx, kind: JobKindPluginCommand}
	outcome, err := adapter.ExecuteCommand(context.Background(), jobs.CommandExecution{JobID: snapshot.ID, Key: "inspect"})
	require.NoError(t, err)
	require.Equal(t, jobs.CommandStatusSucceeded, outcome.Status)
	var detail map[string]string
	require.NoError(t, json.Unmarshal(outcome.Detail, &detail))
	require.Equal(t, "inspected-run", detail["runId"])
	require.Equal(t, "/admin/plugin-command-runs?id=inspected-run", detail["location"])
}

// TestPresentJobIDsReportsOnlyJobsThatExist backs the command history page's
// Job links: a Job retention deleted is absent, and an empty request reads
// nothing.
func TestPresentJobIDsReportsOnlyJobsThatExist(t *testing.T) {
	ctx, _, jobID := finishClaimedCommandRun(t, "present-job", plugin_commands.RunFinish{Status: plugin_commands.RunStatusSucceeded})
	present, err := ctx.PresentJobIDs([]string{jobID, "01a0dda5-0000-7000-8000-000000000000", ""})
	require.NoError(t, err)
	require.Equal(t, map[string]bool{jobID: true}, present)
	present, err = ctx.PresentJobIDs(nil)
	require.NoError(t, err)
	require.Empty(t, present)
}
