package models

import (
	"time"
)

type Category struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name        string   `gorm:"uniqueIndex:unique_category_name"`
	Description string   `gorm:"index"`
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
