package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
)

// jobDownloadHoldBatch bounds one page of the reclassification's scan.
const jobDownloadHoldBatch = 200

// ReclassifyDownloadHolds moves the download Jobs an earlier release held as
// blocked into the paused state a pause records now, and reports how many it moved.
//
// That release recorded a person's pause as blocked with the reason "paused", so a
// held download read as work that needed attention, and a filter for paused work
// never found it. The Job whose latest blocked event gives that reason is the hold;
// one whose latest block is a refusal stays blocked, whatever held it before. Each
// start runs it, so a rolling upgrade's last start moves what the older processes
// it replaced recorded.
func (ctx *MahresourcesContext) ReclassifyDownloadHolds() (int, error) {
	service := ctx.JobService()
	if service == nil || ctx.db == nil {
		return 0, nil
	}
	moved := 0
	cursor := ""
	for {
		var candidates []models.Job
		query := ctx.db.Select("id", "version").
			Where("kind IN ? AND state = ? AND COALESCE(execution_token, '') = ''",
				[]string{JobKindRemoteDownload, JobKindDeferredDownload}, string(jobs.StateBlocked)).
			Order("id ASC").Limit(jobDownloadHoldBatch)
		if cursor != "" {
			query = query.Where("id > ?", cursor)
		}
		if err := query.Find(&candidates).Error; err != nil {
			return moved, fmt.Errorf("read blocked downloads: %w", err)
		}
		for _, candidate := range candidates {
			held, err := ctx.downloadBlockIsHold(candidate.ID)
			if err != nil {
				return moved, err
			}
			if !held {
				continue
			}
			snap, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, candidate.ID)
			if err != nil {
				return moved, err
			}
			progress := snap.Progress
			progress.Phase = ""
			progress.Message = jobDownloadPausedMessage
			progress.ETA = nil
			_, err = service.PauseBlockedHold(ctx.jobDeps(), candidate.ID, snap.Version, jobDownloadPausedDetail, &progress)
			switch {
			case err == nil:
				moved++
			case errors.Is(err, jobs.ErrVersionConflict), errors.Is(err, jobs.ErrIllegalTransition),
				errors.Is(err, jobs.ErrControlIntentWon):
				// Moved on since it was read, or cancelled while held: it is no longer
				// the hold this pass was asked to record.
			default:
				return moved, err
			}
		}
		if len(candidates) < jobDownloadHoldBatch {
			return moved, nil
		}
		cursor = candidates[len(candidates)-1].ID
	}
}

// downloadBlockIsHold reports whether a blocked download's latest block recorded a
// person's pause.
func (ctx *MahresourcesContext) downloadBlockIsHold(jobID string) (bool, error) {
	var event models.JobEvent
	err := ctx.db.Select("detail").
		Where("job_id = ? AND type = ?", jobID, jobs.EventBlocked).
		Order("sequence DESC").Limit(1).Take(&event).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("read the block of download %s: %w", jobID, err)
	}
	var detail struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal(event.Detail, &detail); err != nil {
		return false, nil
	}
	return detail.Reason == "paused", nil
}
