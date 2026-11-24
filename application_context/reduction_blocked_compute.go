package application_context

import (
	"context"
	"encoding/json"
	"fmt"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

// resumeBlockedReductionComputes returns to the queue the clustering runs held for a
// Reduction whose migration has now published (a compute that outwaited the
// migration is blocked "migration-pending", design D12, and nothing else would ever
// start it again). Best effort: a run that cannot be resumed here stays held with
// its Resume command, and a failure is logged, not returned.
func (ctx *MahresourcesContext) resumeBlockedReductionComputes(reductionID uint) {
	service := ctx.JobService()
	if service == nil || ctx.db == nil {
		return
	}
	var rows []struct {
		ID      string
		Summary types.JSON
	}
	if err := ctx.db.Raw(`SELECT id, summary FROM jobs WHERE kind = ? AND state = ?`, JobKindReductionCompute, string(jobs.StateBlocked)).Scan(&rows).Error; err != nil {
		ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d: reading its held clustering runs: %v", reductionID, err), nil)
		return
	}
	for _, row := range rows {
		var summary reductionComputeSummary
		if len(row.Summary) == 0 || json.Unmarshal(row.Summary, &summary) != nil || summary.ReductionID != reductionID {
			continue
		}
		current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, row.ID)
		if err == nil {
			_, err = service.ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
				JobID: row.ID, Key: jobs.CommandResume, Actor: jobs.Access{Administrator: true},
				ExpectedVersion: current.Version,
				IdempotencyKey:  fmt.Sprintf("reduction-%d-published-v%d", reductionID, current.Version),
			})
		}
		if err != nil {
			ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d: resuming held clustering run %s: %v", reductionID, row.ID, err), nil)
		}
	}
}
