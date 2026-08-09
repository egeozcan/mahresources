package application_context

import (
	"errors"
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
	// ErrLoginUnavailable means the credential was never judged: the login could
	// not claim the user-management lock in time. It is deliberately distinct
	// from ErrInvalidCredentials so a database stall is reported as a transient
	// outage instead of being blamed on the password and charged to the login
	// rate limiter.
	ErrLoginUnavailable = errors.New("login temporarily unavailable")
)

// sessionTouchInterval throttles LastSeenAt writes so validating a session on
// every request does not produce a database write each time.
const sessionTouchInterval = time.Minute

// loginLockWait bounds how long minting a session waits for the user-management
// lock. Long enough to ride out ordinary user-management mutations, short enough
// that a bulk group operation degrades logins to a fast retryable error rather
// than an unbounded hang.
const loginLockWait = 2 * time.Second

// CreateSession mints a new login session for an already-identified user and
// returns the raw token (to be placed in the cookie) plus the stored session
// record. It performs no credential check and takes no lock, so login handlers
// must use AuthenticateAndCreateSession instead: composing this with
// AuthenticateUser leaves a window in which a concurrent password reset, disable
// or delete commits between the two and the session outlives it.
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
// browser session, re-checking the credential under the user-management lock so
// the two cannot interleave. A password reset, disable or delete that commits
// first replaces or removes the hash that was verified, so the insert is
// refused; one that commits afterwards deletes the new session.
//
// The bcrypt compare deliberately runs before the transaction opens. Holding the
// process-wide mutation lock (on SQLite, the database writer lock) across tens of
// milliseconds of hashing lets unauthenticated login traffic serialize every
// writer in the process. SetUserPassword hoists its own hashing out of its
// transaction for the same reason. What the lock has to cover is not the compare
// itself but the window between the compare and the insert, and comparing the
// stored hash to the one that was verified closes exactly that window at the cost
// of two index lookups.
func (ctx *MahresourcesContext) AuthenticateAndCreateSession(username, password string, ttl time.Duration, userAgent, ip string) (*models.User, string, *models.Session, error) {
	verified, err := ctx.AuthenticateUser(username, password)
	if err != nil {
		return nil, "", nil, err
	}

	raw, session, err := newSession(verified.ID, ttl, userAgent, ip)
	if err != nil {
		return nil, "", nil, err
	}

	var user models.User
	err = ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := ctx.lockUserManagementMutationBounded(tx, loginLockWait); err != nil {
			return err
		}
		query := tx
		if ctx.Config.DbType == constants.DbTypePosgres {
			query = query.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if err := query.First(&user, verified.ID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				// The account was deleted between the compare and the lock.
				return ErrInvalidCredentials
			}
			return err
		}
		// Not a secret comparison: both operands are server-side values, so a
		// constant-time compare would buy nothing.
		if user.PasswordHash != verified.PasswordHash {
			return ErrInvalidCredentials
		}
		if user.Disabled {
			return ErrUserDisabled
		}
		return tx.Create(session).Error
	})
	if err != nil {
		if isLockContentionError(err) {
			return nil, "", nil, ErrLoginUnavailable
		}
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
