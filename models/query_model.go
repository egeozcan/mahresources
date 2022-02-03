package models

import (
	"time"
)

type Query struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name string `gorm:"uniqueIndex:unique_query_name"`
	Text string `gorm:"index"`
}
