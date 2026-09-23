package models

import "time"

// JobResourceReceipt records the Resource a canonical download Job created.
// It is committed with the Resource so retrying after a process crash between
// resource creation and queue acknowledgement returns the original row.
type JobResourceReceipt struct {
	JobID       string    `gorm:"primaryKey;size:36" json:"jobId"`
	ResourceID  uint      `gorm:"not null;index" json:"resourceId"`
	Hash        string    `gorm:"size:40;not null" json:"hash"`
	ActorUserID *uint     `gorm:"index" json:"actorUserId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`

	Job      *Job      `gorm:"foreignKey:JobID;references:ID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Resource *Resource `gorm:"foreignKey:ResourceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
}
