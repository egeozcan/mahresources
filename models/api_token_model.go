package models

import "time"

// ApiToken is a long-lived bearer credential for non-browser clients (the mr
// CLI, automation). The raw token is shown once at creation; the database stores
// its SHA-256 hash. The Prefix is a short, non-secret display fragment so users
// can recognize a token in a list without revealing it.
type ApiToken struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time

	UserId uint  `gorm:"index;not null" json:"userId"`
	User   *User `gorm:"foreignKey:UserId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`

	Name string `json:"name"`
	// TokenHash is the hex SHA-256 of the raw token (64 chars).
	TokenHash string `gorm:"uniqueIndex;size:64;not null" json:"-"`
	// Prefix is a short non-secret fragment of the raw token for display.
	Prefix string `gorm:"index;size:24" json:"prefix"`

	// ExpiresAt is optional; nil means the token never expires.
	ExpiresAt  *time.Time `gorm:"index" json:"expiresAt,omitempty"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	Disabled   bool       `gorm:"index" json:"disabled"`
}
