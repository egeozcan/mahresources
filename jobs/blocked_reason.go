package jobs

import (
	"encoding/json"
	"fmt"

	"mahresources/models/types"
)

// BlockedReasons answers why each of the given blocked Jobs is blocked: the
// "reason" its latest blocked event recorded, which is the one that blocked it
// now. The state lives on the row and the reason on the timeline, so a list
// that shows both reads them together here, with one query for the page
// however many of its Jobs are blocked. Each Job contributes one row, its latest
// blocked event, found by walking its timeline index back from the end, so a
// Job blocked and resumed many times costs no more than one blocked once. The
// caller has already decided the viewer may see these Jobs; a Job whose event
// carries no reason is left out.
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
	var rows []struct {
		JobID  string
		Detail types.JSON
	}
	if err := deps.DB.Table("jobs").
		Select("jobs.id AS job_id, (SELECT e.detail FROM job_events e WHERE e.job_id = jobs.id AND e.type = ? ORDER BY e.sequence DESC LIMIT 1) AS detail", EventBlocked).
		Where("jobs.id IN ?", ids).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("jobs: read blocked reasons: %w", err)
	}
	for _, row := range rows {
		var detail struct {
			Reason string `json:"reason"`
		}
		if json.Unmarshal(row.Detail, &detail) == nil && detail.Reason != "" {
			reasons[row.JobID] = detail.Reason
		}
	}
	return reasons, nil
}
