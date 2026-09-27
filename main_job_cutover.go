package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
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
	{Kind: application_context.JobKindSummaryExport, Version: 1},
}

func validateJobCenterCutover(service *jobs.Service, readiness application_context.JobMigrationReadiness) error {
	if !readiness.Ready || readiness.WriterEpoch < models.JobWriterEpochRetiredPlaintext || readiness.Phase != models.JobMigrationPhaseComplete {
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

// warnJobPrincipalReviewCandidates says, at every start while any remain, which
// unfinished Jobs may be a deleted account's work that would now run as the host
// (application_context.jobPrincipalReviewCandidates). It is a warning rather than
// a refusal to start: the rows cannot be told apart from legitimate actorless work.
func warnJobPrincipalReviewCandidates(review application_context.JobReviewCandidates) {
	if review.Count == 0 {
		return
	}
	ids := make([]string, 0, len(review.Jobs))
	for _, job := range review.Jobs {
		ids = append(ids, job.ID+" ("+job.Kind+", "+job.State+")")
	}
	more := ""
	if review.Count > int64(len(review.Jobs)) {
		more = fmt.Sprintf(" and %d more", review.Count-int64(len(review.Jobs)))
	}
	log.Printf("[jobs] WARNING: %d unfinished Job(s) record no owner, no actor and no deleted account, and run as the server itself. "+
		"An earlier release may have cleared the account that submitted them. Review them before admitting traffic "+
		"(\"Unfinished Jobs of accounts deleted before this release\" in the advanced configuration docs, "+
		"or reviewCandidates in GET /v1/admin/jobs/migration-readiness): %s%s",
		review.Count, strings.Join(ids, ", "), more)
}
