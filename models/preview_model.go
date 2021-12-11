package models

import (
	"time"
)

type Preview struct {
	ID          uint `gorm:"primarykey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Data        []byte `json:"-"`
	Width       uint
	Height      uint
	ContentType string
	Resource    *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ResourceId  *uint
}
