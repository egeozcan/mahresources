package contracts

import "time"

// MetadataIndexStatus describes background reconciliation. Ready means all
// requested indexes exist and obsolete managed indexes have been removed.
// Failed builds retain the category declarations for retry.
type MetadataIndexStatus struct {
	Configuration string    `json:"configuration"`
	State         string    `json:"state"`
	Detail        string    `json:"detail,omitempty"`
	Error         string    `json:"error,omitempty"`
	CheckedAt     time.Time `json:"checkedAt,omitempty"`
}
