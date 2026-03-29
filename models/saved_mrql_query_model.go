package models

import "time"

// SavedMRQLQuery stores a named MRQL query for quick retrieval and reuse.
type SavedMRQLQuery struct {
	ID          uint      `gorm:"primarykey" json:"id"`
	CreatedAt   time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"index" json:"updatedAt"`
	Name        string    `gorm:"uniqueIndex:unique_mrql_query_name" json:"name"`
	Query       string    `json:"query"`
	Description string    `json:"description"`
}

func (q SavedMRQLQuery) GetId() uint          { return q.ID }
func (q SavedMRQLQuery) GetName() string      { return q.Name }
func (q SavedMRQLQuery) GetDescription() string { return q.Description }
