package models

import "time"

// Session is a server-side browser login session. The raw token is held only in
// the user's cookie; the database stores its SHA-256 hash (hex) so a database
// disclosure does not yield usable credentials.
type Session struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time

	UserId uint  `gorm:"index;not null" json:"userId"`
	User   *User `gorm:"foreignKey:UserId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`

	// TokenHash is the hex SHA-256 of the raw cookie token (64 chars).
	TokenHash string `gorm:"uniqueIndex;size:64;not null" json:"-"`

	// CsrfToken is a random per-session synchronizer token for CSRF protection.
	// Unlike the session token it is safe to expose to the page (meta tag / form
	// field); a forged cross-site request cannot read it, so it cannot be
	// replayed. Defense-in-depth atop the SameSite=Lax session cookie.
	CsrfToken string `gorm:"size:64" json:"-"`

	ExpiresAt  time.Time `gorm:"index" json:"expiresAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`

	UserAgent string `json:"-"`
	IP        string `json:"-"`
}
