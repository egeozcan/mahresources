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
