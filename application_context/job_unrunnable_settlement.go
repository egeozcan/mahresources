package application_context

import (
	"mahresources/jobs"
)

// settleUnrunnablePluginActionJob records, under the claim that found it, what
// the control plane would have recorded for a plugin-action Job that can never
// run when its own write did not land in the claim's bound: the failure it
// carries, or otherwise its block.
func (ctx *MahresourcesContext) settleUnrunnablePluginActionJob(execution jobs.Execution, unrunnable *jobs.UnrunnableClaimError) error {
	if unrunnable.Failure == nil {
		return ctx.blockPluginActionJob(execution, unrunnable.Reason)
	}
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	failure := *unrunnable.Failure
	if _, err := service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: current.Version,
		Outcome:         jobs.StateFailed,
		Failure:         &failure,
	}); err != nil && !mirrorRefusalIsSilent(err) {
		return err
	}
	return nil
}
