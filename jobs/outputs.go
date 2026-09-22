package jobs

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// This file holds typed outputs: publication, the bounded view of what was
// published, and the verification success depends on. Policy about which
// reference shapes are safe belongs to the Kind adapters; what the Service
// guarantees here is that a reference is bounded and valid, that one key names
// one output, that availability never rewrites an outcome, and that success is
// refused while a required output cannot be opened.

// PublishOutput records one typed output of a Job an execution owns.
//
// Publishing is a replacement, not an append: (job_id, key) is unique, so an
// at-least-once executor that crashed after writing its row and runs again
// rewrites the same output and advances that output's own version instead of
// growing a second row. The Job's lifecycle version is untouched — an output is
// not a lifecycle fact, and moving the version would invalidate the transition
// an executor is about to make.
//
// An output whose planned expiry has already passed is stored expired rather
// than available: nothing can fetch it, and advertising it as available would
// make every reader check the clock for themselves. For an optional output that
// becomes a bounded warning; a required one is not refused here, because §7
// makes availability a property of success rather than of publication, and
// Finish is where that is enforced.
func (s *Service) PublishOutput(deps Deps, ref ExecutionRef, input OutputInput) (Output, error) {
	if err := validateExecutionRef(ref); err != nil {
		return Output{}, err
	}
	if err := validateOutputInput(input); err != nil {
		return Output{}, err
	}

	job, err := loadJob(deps.DB, ref.JobID)
	if err != nil {
		return Output{}, err
	}
	if err := requireExecutionToken(job, ref.ExecutionToken); err != nil {
		return Output{}, err
	}
	if err := requireNonterminal(job); err != nil {
		return Output{}, err
	}

	now := deps.now()
	expiresAt := utcPtr(input.ExpiresAt)
	availability := OutputAvailable
	if expiresAt != nil && !expiresAt.After(now) {
		availability = OutputExpired
	}

	var published Output
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		// The first statement is the write: it takes the writer lock before
		// anything is read (SQLite), locks the Job row (PostgreSQL), and is what
		// serialises two publications of one key against each other.
		result := tx.Model(&models.Job{}).
			Where("id = ? AND execution_token = ? AND state = ?", job.ID, job.ExecutionToken, job.State).
			Update("updated_at", now)
		if result.Error != nil {
			return fmt.Errorf("jobs: touch job: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("%w: job %s changed while the output was being published", ErrVersionConflict, job.ID)
		}

		row, err := upsertOutput(tx, job, input, availability, expiresAt, now)
		if err != nil {
			return err
		}
		published = outputView(row)

		if availability != OutputAvailable && !input.Required {
			detail, err := json.Marshal(map[string]any{
				"reason": "an optional output is not available",
				"key":    input.Key,
				"state":  string(availability),
			})
			if err != nil {
				return fmt.Errorf("jobs: encode output warning: %w", err)
			}
			return appendEventTx(tx, job, EventInput{Type: EventWarning, Detail: detail}, now)
		}
		return nil
	})
	if err != nil {
		return Output{}, err
	}
	return published, nil
}

// upsertOutput stores one output row, replacing the Job's existing row for the
// same key — which is the same output produced again — rather than adding a
// second one, and refuses a new key once the Job holds its ceiling of them.
func upsertOutput(tx *gorm.DB, job models.Job, input OutputInput, availability OutputAvailability, expiresAt *time.Time, now time.Time) (models.JobOutput, error) {
	var existing models.JobOutput
	err := tx.Where("job_id = ? AND key = ?", job.ID, input.Key).First(&existing).Error
	switch {
	case err == nil:
		// The publication is the current truth about this key, so the removal
		// instant is cleared with it: an output produced again is not still
		// recorded as gone, and an availability of "available" beside a removal
		// instant would be a row that contradicts itself.
		updates := map[string]any{
			"type":         input.Type,
			"label":        input.Label,
			"reference":    types.JSON(input.Reference),
			"required":     input.Required,
			"availability": string(availability),
			"expires_at":   expiresAt,
			"removed_at":   nil,
			"version":      existing.Version + 1,
			"updated_at":   now,
		}
		if err := tx.Model(&models.JobOutput{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return models.JobOutput{}, fmt.Errorf("jobs: replace output: %w", err)
		}
		existing.Type = input.Type
		existing.Label = input.Label
		existing.Reference = types.JSON(input.Reference)
		existing.Required = input.Required
		existing.Availability = string(availability)
		existing.ExpiresAt = expiresAt
		existing.RemovedAt = nil
		existing.Version++
		existing.UpdatedAt = now
		return existing, nil

	case !isNotFound(err):
		return models.JobOutput{}, fmt.Errorf("jobs: read output: %w", err)
	}

	var stored int64
	if err := tx.Model(&models.JobOutput{}).Where("job_id = ?", job.ID).Count(&stored).Error; err != nil {
		return models.JobOutput{}, fmt.Errorf("jobs: count outputs: %w", err)
	}
	if stored >= MaxOutputsPerJob {
		return models.JobOutput{}, fmt.Errorf("%w: job %s already holds %d outputs",
			ErrOutputCapacityExhausted, job.ID, stored)
	}

	row := models.JobOutput{
		ID:           types.NewUUIDv7(),
		JobID:        job.ID,
		Key:          input.Key,
		Type:         input.Type,
		Label:        input.Label,
		Reference:    types.JSON(input.Reference),
		Required:     input.Required,
		Availability: string(availability),
		Version:      1,
		ExpiresAt:    expiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := tx.Create(&row).Error; err != nil {
		return models.JobOutput{}, fmt.Errorf("jobs: store output: %w", err)
	}
	return row, nil
}

// validateOutputInput checks and normalizes one publication request.
func validateOutputInput(input OutputInput) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidOutput, fmt.Sprintf(format, args...))
	}

	if strings.TrimSpace(input.Key) == "" {
		return invalid("an output needs a key")
	}
	if len(input.Key) > MaxOutputKeyBytes {
		return invalid("output key is %d bytes, over the %d-byte ceiling", len(input.Key), MaxOutputKeyBytes)
	}
	if strings.TrimSpace(input.Type) == "" {
		return invalid("an output needs a type")
	}
	if len(input.Type) > MaxOutputTypeBytes {
		return invalid("output type is %d bytes, over the %d-byte ceiling", len(input.Type), MaxOutputTypeBytes)
	}
	if !knownOutputType(input.Type) {
		return invalid("output type %q is not one of %s", input.Type, strings.Join(OutputTypes, ", "))
	}
	if len(input.Label) > MaxOutputLabelBytes {
		return invalid("output label is %d bytes, over the %d-byte ceiling", len(input.Label), MaxOutputLabelBytes)
	}
	if len(input.Reference) == 0 {
		return invalid("an output needs a reference to what it published")
	}
	if len(input.Reference) > MaxOutputReferenceBytes {
		return invalid("output reference is %d bytes, over the %d-byte ceiling", len(input.Reference), MaxOutputReferenceBytes)
	}
	if !json.Valid(input.Reference) {
		return invalid("output reference is not valid JSON")
	}
	input.ExpiresAt = utcPtr(input.ExpiresAt)
	return nil
}

// knownOutputType reports whether t is in the closed vocabulary.
func knownOutputType(t string) bool {
	for _, candidate := range OutputTypes {
		if t == candidate {
			return true
		}
	}
	return false
}

// outputView projects a stored row onto the bounded view callers receive. The
// reference is copied so a returned view never shares a buffer with a caller's
// request or with a row another goroutine is holding.
func outputView(row models.JobOutput) Output {
	return Output{
		ID:           row.ID,
		JobID:        row.JobID,
		Key:          row.Key,
		Type:         row.Type,
		Label:        row.Label,
		Reference:    json.RawMessage(copyJSON(row.Reference)),
		Required:     row.Required,
		Availability: OutputAvailability(row.Availability),
		Version:      row.Version,
		ExpiresAt:    row.ExpiresAt,
		RemovedAt:    row.RemovedAt,
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}
}

// copyJSON copies a bounded reference, returning nil for an absent one so an
// empty reference stays absent rather than becoming JSON null.
func copyJSON(raw types.JSON) types.JSON {
	if len(raw) == 0 {
		return nil
	}
	copied := make([]byte, len(raw))
	copy(copied, raw)
	return types.JSON(copied)
}

// verifyRequiredOutputs refuses a success whose required outputs are not durable
// and available. It runs inside the terminal transaction, so a Job never commits
// a success alongside an output it cannot honestly claim.
//
// Both halves matter. A key the caller named but never published is missing —
// absence is invisible to a scan of the rows, which is why the adapter's own
// declaration is checked too — and every stored required row is verified because
// the row is the durable authority, whatever the caller remembered to pass.
// Optional outputs are deliberately not verified: their inability to be opened
// is a warning, not a reason for finished work to be reported as unfinished.
func verifyRequiredOutputs(tx *gorm.DB, jobID string, named []string) error {
	var rows []models.JobOutput
	if err := tx.Where("job_id = ?", jobID).Find(&rows).Error; err != nil {
		return fmt.Errorf("jobs: read outputs for verification: %w", err)
	}

	byKey := make(map[string]models.JobOutput, len(rows))
	for _, row := range rows {
		byKey[row.Key] = row
	}
	for _, key := range named {
		row, published := byKey[key]
		if !published {
			return fmt.Errorf("%w: job %s never published its %q output", ErrRequiredOutputUnavailable, jobID, key)
		}
		if row.Availability != string(OutputAvailable) {
			return fmt.Errorf("%w: job %s output %q is %s", ErrRequiredOutputUnavailable, jobID, key, row.Availability)
		}
	}
	for _, row := range rows {
		if row.Required && row.Availability != string(OutputAvailable) {
			return fmt.Errorf("%w: required output %q of job %s is %s",
				ErrRequiredOutputUnavailable, row.Key, jobID, row.Availability)
		}
	}
	return nil
}
