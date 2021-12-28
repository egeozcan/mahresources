package models

import (
	"time"
)

type Category struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name        string   `gorm:"uniqueIndex:unique_category_name"`
	Description string   `gorm:"index"`
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
