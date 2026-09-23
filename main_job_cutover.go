package main

import (
	"errors"
	"fmt"

	"mahresources/application_context"
	"mahresources/jobs"
)

// Keep this independent of the runtime's own Registrations list. A missing
// Kind must block the cutover rather than silently shrink the inventory.
var jobCenterCutoverKinds = []jobs.CommandFilterKind{
	{Kind: application_context.JobKindRemoteDownload, Version: 1},
	{Kind: application_context.JobKindDeferredDownload, Version: 1},
	{Kind: application_context.JobKindGroupExport, Version: 1},
	{Kind: application_context.JobKindGroupImportParse, Version: 1},
	{Kind: application_context.JobKindGroupImportApply, Version: 1},
	{Kind: application_context.JobKindReductionCompute, Version: 1},
	{Kind: application_context.JobKindSimilarityRecompute, Version: 1},
	{Kind: application_context.JobKindPluginAction, Version: 1},
	{Kind: application_context.JobKindPluginCommand, Version: 1},
	{Kind: application_context.JobKindPluginCommandImport, Version: 1},
}

func validateJobCenterCutover(service *jobs.Service, readiness application_context.JobMigrationReadiness) error {
	if !readiness.Ready {
		return fmt.Errorf("Job Center cutover blocked: migration phase %s, writer epoch %d, blockers %v",
			readiness.Phase, readiness.WriterEpoch, readiness.Blockers)
	}
	if service == nil {
		return errors.New("Job Center cutover blocked: control plane is unavailable")
	}
	if err := service.ValidateCommandFilterCoverage(jobCenterCutoverKinds); err != nil {
		return fmt.Errorf("Job Center cutover blocked: %w", err)
	}
	return nil
}
