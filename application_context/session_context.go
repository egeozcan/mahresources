package application_context

import (
	"errors"
	"time"

	"mahresources/auth"
	"mahresources/models"

	"gorm.io/gorm"
)

// Session errors.
var (
	ErrSessionInvalid = errors.New("session invalid or expired")
)

// sessionTouchInterval throttles LastSeenAt writes so validating a session on
// every request does not produce a database write each time.
const sessionTouchInterval = time.Minute

// CreateSession mints a new login session for a user and returns the raw token
// (to be placed in the cookie) plus the stored session record.
func (ctx *MahresourcesContext) CreateSession(userID uint, ttl time.Duration, userAgent, ip string) (string, *models.Session, error) {
	raw, err := auth.GenerateToken()
	if err != nil {
		return "", nil, err
	}
	csrf, err := auth.GenerateToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	session := &models.Session{
		UserId:     userID,
		TokenHash:  auth.HashToken(raw),
		CsrfToken:  csrf,
		ExpiresAt:  now.Add(ttl),
		LastSeenAt: now,
		UserAgent:  userAgent,
		IP:         ip,
	}
	if err := ctx.db.Create(session).Error; err != nil {
		return "", nil, err
	}
	return raw, session, nil
}

// ValidateSession resolves a raw cookie token to its user. It rejects expired
// sessions and disabled accounts, and refreshes LastSeenAt at most once per
// sessionTouchInterval.
func (ctx *MahresourcesContext) ValidateSession(rawToken string) (*models.User, *models.Session, error) {
	if rawToken == "" {
		return nil, nil, ErrSessionInvalid
	}
	var session models.Session
	err := ctx.db.Where("token_hash = ?", auth.HashToken(rawToken)).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrSessionInvalid
		}
		return nil, nil, err
	}
	if time.Now().After(session.ExpiresAt) {
		// Best-effort cleanup of the expired row.
		ctx.db.Delete(&models.Session{}, session.ID)
		return nil, nil, ErrSessionInvalid
	}

	user, err := ctx.GetUser(session.UserId)
	if err != nil {
		return nil, nil, ErrSessionInvalid
	}
	if user.Disabled {
		return nil, nil, ErrUserDisabled
	}

	if time.Since(session.LastSeenAt) > sessionTouchInterval {
		now := time.Now()
		ctx.db.Model(&models.Session{}).Where("id = ?", session.ID).Update("last_seen_at", now)
		session.LastSeenAt = now
	}
	return user, &session, nil
}

// RevokeSession deletes the session identified by a raw cookie token (logout).
func (ctx *MahresourcesContext) RevokeSession(rawToken string) error {
	if rawToken == "" {
		return nil
	}
	return ctx.db.Where("token_hash = ?", auth.HashToken(rawToken)).Delete(&models.Session{}).Error
}

// RevokeUserSessions deletes every session for a user (e.g. on password change
// or account disable).
func (ctx *MahresourcesContext) RevokeUserSessions(userID uint) error {
	return ctx.db.Where("user_id = ?", userID).Delete(&models.Session{}).Error
}

// GetSessionsForUser lists a user's active (non-expired) sessions, newest first.
func (ctx *MahresourcesContext) GetSessionsForUser(userID uint) ([]models.Session, error) {
	var sessions []models.Session
	err := ctx.db.Where("user_id = ? AND expires_at > ?", userID, time.Now()).
		Order("last_seen_at desc").Find(&sessions).Error
	return sessions, err
}

// DeleteExpiredSessions purges expired sessions and returns how many were removed.
func (ctx *MahresourcesContext) DeleteExpiredSessions() (int64, error) {
	res := ctx.db.Where("expires_at <= ?", time.Now()).Delete(&models.Session{})
	return res.RowsAffected, res.Error
}
