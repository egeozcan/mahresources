package jobs

import (
	"encoding/json"
	"fmt"

	"mahresources/models"
)

// BlockedReasons answers why each of the given blocked Jobs is blocked: the
// "reason" its latest blocked event recorded, which is the one that blocked it
// now. The state lives on the row and the reason on the timeline, so a list
// that shows both reads them together here, with one query for the page
// however many of its Jobs are blocked. The caller has already decided the
// viewer may see these Jobs; a Job whose event carries no reason is left out.
func (s *Service) BlockedReasons(deps Deps, snapshots []Snapshot) (map[string]string, error) {
	ids := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.State == StateBlocked {
			ids = append(ids, snapshot.ID)
		}
	}
	reasons := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return reasons, nil
	}
	var events []models.JobEvent
	if err := deps.DB.Model(&models.JobEvent{}).
		Select("job_id", "sequence", "detail").
		Where("job_id IN ? AND type = ?", ids, EventBlocked).
		Order("job_id").Order("sequence DESC").
		Find(&events).Error; err != nil {
		return nil, fmt.Errorf("jobs: read blocked reasons: %w", err)
	}
	seen := make(map[string]bool, len(ids))
	for _, event := range events {
		if seen[event.JobID] {
			continue
		}
		seen[event.JobID] = true
		var detail struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(event.Detail, &detail) == nil && detail.Reason != "" {
			reasons[event.JobID] = detail.Reason
		}
	}
	return reasons, nil
}
