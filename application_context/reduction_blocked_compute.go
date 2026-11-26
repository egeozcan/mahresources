package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

// blockedComputeChunk is how many held clustering runs one read loads.
var blockedComputeChunk = 100

// resumeBlockedReductionComputes returns to the queue the clustering runs held for a
// Reduction whose migration has now published (a compute that outwaited the
// migration is blocked "migration-pending", design D12, and nothing else would ever
// start it again). Best effort: a run that cannot be resumed here stays held with
// its Resume command, and a failure is logged, not returned.
func (ctx *MahresourcesContext) resumeBlockedReductionComputes(reductionID uint) {
	ctx.resumeHeldReductionComputes(reductionID, "")
}

// ResumeIndexBlockedReductionComputes returns to the queue every clustering run held
// as "indexes-building", once the similarity distance indexes read valid. Called
// when the index build finishes.
func (ctx *MahresourcesContext) ResumeIndexBlockedReductionComputes() {
	ctx.resumeHeldReductionComputes(0, reductionHoldIndexesBuilding)
}

// resumeIfHoldCleared is the other half of a hold: a run held for a condition that
// cleared between the check that refused it and the block that recorded the hold was
// missed by the resumer, which looked before the hold existed. It asks the condition
// once more and resumes at once when it already holds.
func (ctx *MahresourcesContext) resumeIfHoldCleared(reason string, reductionID uint) {
	switch reason {
	case reductionHoldMigrationPending:
		reduction, err := ctx.loadReductionForUpdate(reductionID, nil, false)
		if err == nil && reduction.PlanSchema != 0 {
			ctx.resumeBlockedReductionComputes(reductionID)
		}
	case reductionHoldIndexesBuilding:
		if ctx.requireSimilarityIndexes() == nil {
			ctx.ResumeIndexBlockedReductionComputes()
		}
	}
}

// resumeHeldReductionComputes resumes the held clustering runs of one Reduction
// (reductionID non-zero), or of every Reduction, and only those held for reason
// when one is given. The held runs are read in keyset chunks, and for one
// Reduction the database filters on the summary's reductionId, so a deployment's
// held runs for other Reductions are never loaded.
func (ctx *MahresourcesContext) resumeHeldReductionComputes(reductionID uint, reason string) {
	service := ctx.JobService()
	if service == nil || ctx.db == nil {
		return
	}
	where, args := `kind = ? AND state = ?`, []any{JobKindReductionCompute, string(jobs.StateBlocked)}
	if reductionID != 0 {
		where += ` AND CAST(` + jobSummaryTextExpr(ctx.db, "reductionId") + ` AS TEXT) = ?`
		args = append(args, strconv.FormatUint(uint64(reductionID), 10))
	}
	after := ""
	for {
		var rows []struct {
			ID      string
			Summary types.JSON
		}
		if err := ctx.db.Raw(`SELECT id, summary FROM jobs WHERE `+where+` AND id > ? ORDER BY id LIMIT ?`,
			append(append([]any{}, args...), after, blockedComputeChunk)...).Scan(&rows).Error; err != nil {
			ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d: reading its held clustering runs: %v", reductionID, err), nil)
			return
		}
		for _, row := range rows {
			after = row.ID
			var summary reductionComputeSummary
			if len(row.Summary) == 0 || json.Unmarshal(row.Summary, &summary) != nil || (reductionID != 0 && summary.ReductionID != reductionID) {
				continue
			}
			current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, row.ID)
			if err == nil && reason != "" {
				reasons, rerr := service.BlockedReasons(ctx.jobDeps(), []jobs.Snapshot{current})
				if rerr == nil && reasons[row.ID] != reason {
					continue
				}
			}
			if err == nil {
				_, err = service.ExecuteCommand(context.Background(), ctx.jobDeps(), jobs.CommandRequest{
					JobID: row.ID, Key: jobs.CommandResume, Actor: jobs.Access{Administrator: true},
					ExpectedVersion: current.Version,
					IdempotencyKey:  fmt.Sprintf("reduction-%d-%s-v%d", summary.ReductionID, holdKey(reason), current.Version),
				})
			}
			if err != nil {
				ctx.Logger().Warning(models.LogActionSystem, "", nil, "", fmt.Sprintf("reduction %d: resuming held clustering run %s: %v", summary.ReductionID, row.ID, err), nil)
			}
		}
		if len(rows) < blockedComputeChunk {
			return
		}
	}
}

// holdKey names what a resume was for in its idempotency key, keeping the key the
// migration's publication has always used.
func holdKey(reason string) string {
	if reason == "" {
		return "published"
	}
	return reason
}
