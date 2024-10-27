package models

import (
	"time"
)

type Query struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name        string `gorm:"uniqueIndex:unique_query_name"`
	Text        string `gorm:"index"`
	Template    string
	Description string
}

func (q Query) GetId() uint {
	return q.ID
}

func (q Query) GetName() string {
	return q.Name
}

func (q Query) GetDescription() string {
	return q.Description
}
