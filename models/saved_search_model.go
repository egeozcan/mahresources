package models

import "time"

// SavedSearch is personal navigation state, independent of saved SQL/MRQL queries.
type SavedSearch struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	UserId    uint      `gorm:"not null;index:idx_saved_search_owner_family" json:"-"`
	Name      string    `gorm:"not null" json:"name"`
	Family    string    `gorm:"not null;index:idx_saved_search_owner_family" json:"family"`
	URL       string    `gorm:"type:text;not null" json:"url"`
	Layout    string    `gorm:"-" json:"layout"`
}
