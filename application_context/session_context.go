package application_context

import (
	"errors"
	"strings"
	"time"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Session errors.
var (
	ErrSessionInvalid = errors.New("session invalid or expired")
)

// sessionTouchInterval throttles LastSeenAt writes so validating a session on
// every request does not produce a database write each time.
const sessionTouchInterval = time.Minute

// CreateSession mints a new login session for a user and returns the raw token
// (to be placed in the cookie) plus the stored session record. Login handlers
// must use AuthenticateAndCreateSession so password verification and insertion
// serialize with password reset and account deletion.
func (ctx *MahresourcesContext) CreateSession(userID uint, ttl time.Duration, userAgent, ip string) (string, *models.Session, error) {
	raw, session, err := newSession(userID, ttl, userAgent, ip)
	if err != nil {
		return "", nil, err
	}
	if err := ctx.db.Create(session).Error; err != nil {
		return "", nil, err
	}
	return raw, session, nil
}

// AuthenticateAndCreateSession verifies credentials and inserts the resulting
// browser session under the same user-management transaction lock. A password
// reset that commits after this transaction therefore deletes the new session;
// one that commits first causes the old password check to fail.
func (ctx *MahresourcesContext) AuthenticateAndCreateSession(username, password string, ttl time.Duration, userAgent, ip string) (*models.User, string, *models.Session, error) {
	raw, session, err := newSession(0, ttl, userAgent, ip)
	if err != nil {
		return nil, "", nil, err
	}
	var user models.User
	err = ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.lockUserManagementMutation(tx); err != nil {
			return err
		}
		query := tx
		if ctx.Config.DbType == constants.DbTypePosgres {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.Where("username = ?", strings.TrimSpace(username)).First(&user).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				auth.CheckPassword(dummyHash, password)
				return ErrInvalidCredentials
			}
			return err
		}
		if !auth.CheckPassword(user.PasswordHash, password) {
			return ErrInvalidCredentials
		}
		if user.Disabled {
			return ErrUserDisabled
		}
		session.UserId = user.ID
		return tx.Create(session).Error
	})
	if err != nil {
		return nil, "", nil, err
	}
	return &user, raw, session, nil
}

func newSession(userID uint, ttl time.Duration, userAgent, ip string) (string, *models.Session, error) {
	raw, err := auth.GenerateToken()
	if err != nil {
		return "", nil, err
	}
	csrf, err := auth.GenerateToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now()
	return raw, &models.Session{
		UserId:     userID,
		TokenHash:  auth.HashToken(raw),
		CsrfToken:  csrf,
		ExpiresAt:  now.Add(ttl),
		LastSeenAt: now,
		UserAgent:  userAgent,
		IP:         ip,
	}, nil
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

	// Backfill a CSRF token for sessions created before the CsrfToken column
	// existed (in-place upgrade across the unreleased branch). Without this an
	// otherwise-valid session would be 403'd on every state-changing request
	// because the synchronizer-token check sees an empty expected token.
	if session.CsrfToken == "" {
		if tok, genErr := auth.GenerateToken(); genErr == nil {
			session.CsrfToken = tok
			ctx.db.Model(&models.Session{}).Where("id = ?", session.ID).Update("csrf_token", tok)
		}
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
	return deleteUserSessions(ctx.db, userID, nil)
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
