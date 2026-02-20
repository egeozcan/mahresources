package models

import (
	"time"
)

type ResourceVersion struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"createdAt"`
	ResourceID      uint      `gorm:"index;not null" json:"resourceId"`
	VersionNumber   int       `gorm:"not null" json:"versionNumber"`
	Hash            string    `gorm:"index;not null" json:"hash"`
	HashType        string    `gorm:"not null;default:'SHA1'" json:"hashType"`
	FileSize        int64     `gorm:"not null" json:"fileSize"`
	ContentType     string    `json:"contentType"`
	Width           uint      `json:"width"`
	Height          uint      `json:"height"`
	Location        string    `gorm:"not null" json:"location"`
	StorageLocation *string   `json:"storageLocation"`
	Comment         string    `json:"comment"`
}

func (v ResourceVersion) GetId() uint {
	return v.ID
}

// VersionComparison holds comparison data between two versions
type VersionComparison struct {
	Version1       *ResourceVersion `json:"version1"`
	Version2       *ResourceVersion `json:"version2"`
	Resource1      *Resource        `json:"resource1,omitempty"`
	Resource2      *Resource        `json:"resource2,omitempty"`
	SizeDelta      int64            `json:"sizeDelta"`
	SameHash       bool             `json:"sameHash"`
	SameType       bool             `json:"sameType"`
	DimensionsDiff bool             `json:"dimensionsDiff"`
	CrossResource  bool             `json:"crossResource"`
}
