package jobs

import (
	"encoding/json"
	"fmt"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// InstallLegacyReplay upgrades an existing non-replayable Job with the
// encrypted input recovered from its authoritative legacy source. It is only
// for the startup migration: ordinary acceptance never changes the meaning of
// an existing Job, and an existing envelope or purge marker is never replaced.
func (s *Service) InstallLegacyReplay(deps Deps, jobID string, replay ReplayInput) error {
	if deps.DB == nil || jobID == "" || replay.NonReplayable || len(replay.Input) == 0 {
		return fmt.Errorf("jobs: legacy replay installation needs a Job and replayable input")
	}
	now := deps.now().UTC()
	return deps.DB.Transaction(func(tx *gorm.DB) error {
		txDeps := deps
		txDeps.DB = tx
		txDeps.Now = func() time.Time { return now }
		var job models.Job
		if err := tx.Where("id = ?", jobID).First(&job).Error; err != nil {
			return fmt.Errorf("jobs: legacy replay target is unavailable")
		}
		if ReplayClass(job.ReplayClass) != ReplayClassNonReplayable {
			return fmt.Errorf("jobs: legacy replay target is not explicitly non-replayable")
		}
		var existing models.JobReplayEnvelope
		err := tx.Where("job_id = ?", jobID).First(&existing).Error
		if err == nil {
			return fmt.Errorf("jobs: legacy replay target already has an envelope")
		}
		if !isNotFound(err) {
			return fmt.Errorf("jobs: legacy replay envelope lookup failed")
		}
		sealed, summary, err := s.sealReplay(txDeps, job, replay, now)
		if err != nil {
			return err
		}
		if err := tx.Create(&sealed).Error; err != nil {
			return fmt.Errorf("jobs: legacy replay envelope could not be stored")
		}
		if err := tx.Model(&models.Job{}).Where("id = ? AND replay_class = ?", jobID, string(ReplayClassNonReplayable)).
			Updates(map[string]any{"replay_class": string(ReplayClassReplayable), "summary": summary}).Error; err != nil {
			return fmt.Errorf("jobs: legacy replay target could not be upgraded")
		}
		if job.FinishedAt != nil {
			if err := stampReplayExpiry(tx, job, deps.replayRetention(), now); err != nil {
				return err
			}
		}
		return nil
	})
}

// LegacyImport is the evidence needed to project one historical source record
// into the Job ledger. It accepts only the facts the source can prove; it does
// not synthesize claims, retries, links, or intermediate lifecycle events.
type LegacyImport struct {
	Acceptance        Acceptance
	State             State
	AcceptedAt        time.Time
	StartedAt         *time.Time
	FinishedAt        *time.Time
	Failure           *Failure
	MigrationNote     json.RawMessage
	PurgeReplayReason string
}

// ImportLegacy writes one previously completed or still-scheduled unit of
// legacy work. The accepted fact is stamped at the source submission time, and
// the start and outcome facts are written only when their source timestamps are
// present. The caller's transaction contains the Job, encrypted replay input,
// compatibility handles, and its source mapping.
func (s *Service) ImportLegacy(deps Deps, legacy LegacyImport) (Snapshot, error) {
	if deps.DB == nil {
		return Snapshot{}, fmt.Errorf("jobs: legacy import needs a database")
	}
	if legacy.AcceptedAt.IsZero() {
		return Snapshot{}, fmt.Errorf("jobs: legacy import needs a proven accepted timestamp")
	}
	if !legacy.State.Valid() || legacy.State == StateRunning || legacy.State == StatePaused || legacy.State == StateBlocked {
		return Snapshot{}, fmt.Errorf("jobs: legacy import cannot safely represent state %q", legacy.State)
	}
	if legacy.State.Terminal() && legacy.FinishedAt == nil {
		return Snapshot{}, fmt.Errorf("jobs: terminal legacy import needs a proven finished timestamp")
	}
	if legacy.StartedAt != nil && legacy.StartedAt.Before(legacy.AcceptedAt) {
		return Snapshot{}, fmt.Errorf("jobs: legacy start precedes acceptance")
	}
	if legacy.FinishedAt != nil && (legacy.FinishedAt.Before(legacy.AcceptedAt) || (legacy.StartedAt != nil && legacy.FinishedAt.Before(*legacy.StartedAt))) {
		return Snapshot{}, fmt.Errorf("jobs: legacy finish precedes its proven lifecycle timestamps")
	}
	if legacy.State == StateFailed && legacy.Failure == nil {
		return Snapshot{}, fmt.Errorf("jobs: failed legacy import needs a failure classification")
	}

	acceptance := legacy.Acceptance
	acceptance.State = StateQueued
	if legacy.State == StateScheduled {
		acceptance.State = StateScheduled
	}
	fixed := legacy.AcceptedAt.UTC()
	importDeps := deps
	importDeps.Now = func() time.Time { return fixed }

	var imported Snapshot
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		txDeps := importDeps
		txDeps.DB = tx
		created, _, err := s.accept(nil, txDeps, acceptance, nil)
		if err != nil {
			return err
		}
		imported = created
		if legacy.PurgeReplayReason != "" {
			if legacy.PurgeReplayReason != models.JobReplayPurgeExpired && legacy.PurgeReplayReason != models.JobReplayPurgeForgotten {
				return fmt.Errorf("jobs: legacy import has an unknown replay purge reason")
			}
			purgedAt := legacy.AcceptedAt.UTC()
			if legacy.FinishedAt != nil {
				purgedAt = legacy.FinishedAt.UTC()
			}
			result := tx.Model(&models.JobReplayEnvelope{}).Where("job_id = ? AND purged_at IS NULL", created.ID).
				Updates(map[string]any{"nonce": nil, "ciphertext": nil, "purged_at": purgedAt, "purge_reason": legacy.PurgeReplayReason, "updated_at": purgedAt})
			if result.Error != nil {
				return fmt.Errorf("jobs: mark imported replay as purged: %w", result.Error)
			}
			if result.RowsAffected != 1 {
				return fmt.Errorf("jobs: imported replay envelope could not be purged")
			}
		}
		var row models.Job
		if err := tx.Where("id = ?", created.ID).First(&row).Error; err != nil {
			return err
		}

		updates := map[string]any{
			"accepted_at": legacy.AcceptedAt.UTC(),
			"queued_at":   nil,
			"started_at":  nil,
			"finished_at": nil,
			"updated_at":  legacy.AcceptedAt.UTC(),
		}
		if legacy.State == StateQueued {
			accepted := legacy.AcceptedAt.UTC()
			updates["queued_at"] = &accepted
		}
		if legacy.StartedAt != nil {
			started := legacy.StartedAt.UTC()
			updates["started_at"] = &started
			if err := insertLegacyEvent(tx, row.ID, 2, 2, EventStarted, started, nil); err != nil {
				return err
			}
			row.Version = 2
		}
		if legacy.State.Terminal() {
			finished := legacy.FinishedAt.UTC()
			updates["finished_at"] = &finished
			updates["state_entered_at"] = &finished
			updates["control_intent"] = ""
			updates["control_requested_at"] = nil
			updates["updated_at"] = finished
			version := row.Version + 1
			updates["version"] = version
			updates["state"] = string(legacy.State)
			expires := finished.Add(deps.retention().windowFor(legacy.State)).UTC()
			updates["expires_at"] = &expires
			eventType := eventTypeFor(Transition{To: legacy.State}, legacy.StartedAt != nil)
			var detail json.RawMessage
			if legacy.State == StateFailed {
				failure := legacy.Failure
				if failure == nil || failure.Code == "" || !knownFailureClass(failure.Class) || len(failure.Message) > MaxFailureMessageBytes {
					return fmt.Errorf("jobs: invalid legacy failure classification")
				}
				updates["failure_code"] = failure.Code
				updates["failure_class"] = failure.Class
				updates["failure_message"] = failure.Message
				updates["failure_diagnostic_ref"] = failure.DiagnosticRef
				detail, _ = json.Marshal(map[string]string{"code": failure.Code, "class": failure.Class})
			}
			if err := insertLegacyEvent(tx, row.ID, uint64(version), uint64(version), eventType, finished, detail); err != nil {
				return err
			}
			row.FinishedAt = &finished
			if err := stampReplayExpiry(tx, row, deps.replayRetention(), finished); err != nil {
				return err
			}
			row.Version = version
		}
		if len(legacy.MigrationNote) > 0 {
			if !json.Valid(legacy.MigrationNote) || len(legacy.MigrationNote) > MaxEventDetailBytes {
				return fmt.Errorf("jobs: invalid legacy migration note")
			}
			sequence, err := nextEventSequence(tx, row.ID)
			if err != nil {
				return err
			}
			if err := insertLegacyEvent(tx, row.ID, sequence, row.Version, "migration-note", legacy.AcceptedAt.UTC(), legacy.MigrationNote); err != nil {
				return err
			}
		}
		if err := tx.Model(&models.Job{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("jobs: store legacy lifecycle facts: %w", err)
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	return s.Get(deps, Access{Administrator: true}, imported.ID)
}

func insertLegacyEvent(tx *gorm.DB, jobID string, sequence uint64, version uint64, eventType string, at time.Time, detail json.RawMessage) error {
	row := newEvent(jobID, sequence, version, eventType, detail, true, at.UTC())
	if err := tx.Create(&row).Error; err != nil {
		return fmt.Errorf("jobs: store imported %s event: %w", eventType, err)
	}
	return nil
}
