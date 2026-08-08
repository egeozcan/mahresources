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

// API token errors.
var (
	ErrApiTokenInvalid      = errors.New("api token invalid or expired")
	ErrApiTokenNotFound     = errors.New("api token not found")
	ErrApiTokenLimitReached = errors.New("api token limit reached; revoke an existing token first")
)

// apiTokenTouchInterval throttles LastUsedAt writes for the same reason sessions
// throttle LastSeenAt.
const apiTokenTouchInterval = time.Minute

// CreateApiToken mints a new bearer token for a user and returns the raw token
// (shown once) plus the stored record. A nil expiresAt means the token never
// expires.
func (ctx *MahresourcesContext) CreateApiToken(userID uint, name string, expiresAt *time.Time) (string, *models.ApiToken, error) {
	// Unlimited mode keeps the direct create path, but still validates ownership
	// explicitly so SQLite and Postgres return the same typed error.
	if ctx.Config.MaxUserTokens <= 0 {
		if err := apiTokenOwnerExists(ctx.db, userID); err != nil {
			return "", nil, err
		}
		return createApiToken(ctx.db, userID, name, expiresAt)
	}

	// Serialize capped creation per owner. PostgreSQL locks the owning row. On
	// SQLite the no-op UPDATE is intentionally the transaction's first statement:
	// it acquires the write lock before the count, preventing a read-before-write
	// window while leaving the user record unchanged.
	var raw string
	var token *models.ApiToken
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if err := lockApiTokenOwner(ctx, tx, userID); err != nil {
			return err
		}

		var count int64
		if err := tx.Model(&models.ApiToken{}).Where("user_id = ?", userID).Count(&count).Error; err != nil {
			return err
		}
		if count >= int64(ctx.Config.MaxUserTokens) {
			return ErrApiTokenLimitReached
		}

		var err error
		raw, token, err = createApiToken(tx, userID, name, expiresAt)
		return err
	})
	if err != nil {
		return "", nil, err
	}
	return raw, token, nil
}

func apiTokenOwnerExists(db *gorm.DB, userID uint) error {
	var owner models.User
	if err := db.Select("id").First(&owner, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUserNotFound
		}
		return err
	}
	return nil
}

func lockApiTokenOwner(ctx *MahresourcesContext, tx *gorm.DB, userID uint) error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		var owner models.User
		if err := tx.Select("id").Clauses(clause.Locking{Strength: "UPDATE"}).First(&owner, userID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUserNotFound
			}
			return err
		}
		return nil
	}

	result := tx.Exec("UPDATE users SET id = id WHERE id = ?", userID)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func createApiToken(db *gorm.DB, userID uint, name string, expiresAt *time.Time) (string, *models.ApiToken, error) {
	raw, err := auth.GenerateToken()
	if err != nil {
		return "", nil, err
	}
	token := &models.ApiToken{
		UserId:    userID,
		Name:      name,
		TokenHash: auth.HashToken(raw),
		Prefix:    auth.TokenPrefix(raw),
		ExpiresAt: expiresAt,
	}
	if err := db.Create(token).Error; err != nil {
		return "", nil, err
	}
	return raw, token, nil
}

// ValidateApiToken resolves a raw bearer token to its user, rejecting disabled
// or expired tokens and disabled accounts.
func (ctx *MahresourcesContext) ValidateApiToken(rawToken string) (*models.User, *models.ApiToken, error) {
	if rawToken == "" {
		return nil, nil, ErrApiTokenInvalid
	}
	var token models.ApiToken
	err := ctx.db.Where("token_hash = ?", auth.HashToken(rawToken)).First(&token).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrApiTokenInvalid
		}
		return nil, nil, err
	}
	if token.Disabled {
		return nil, nil, ErrApiTokenInvalid
	}
	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, nil, ErrApiTokenInvalid
	}

	user, err := ctx.GetUser(token.UserId)
	if err != nil {
		return nil, nil, ErrApiTokenInvalid
	}
	if user.Disabled {
		return nil, nil, ErrUserDisabled
	}

	if token.LastUsedAt == nil || time.Since(*token.LastUsedAt) > apiTokenTouchInterval {
		now := time.Now()
		ctx.db.Model(&models.ApiToken{}).Where("id = ?", token.ID).Update("last_used_at", now)
		token.LastUsedAt = &now
	}
	return user, &token, nil
}

// ListApiTokens returns a user's tokens, newest first.
func (ctx *MahresourcesContext) ListApiTokens(userID uint) ([]models.ApiToken, error) {
	var tokens []models.ApiToken
	err := ctx.db.Where("user_id = ?", userID).Order("created_at desc").Find(&tokens).Error
	return tokens, err
}

// RevokeApiToken deletes a token owned by the given user. Scoping the delete to
// userID prevents a user from revoking another user's token by ID.
func (ctx *MahresourcesContext) RevokeApiToken(id, userID uint) error {
	res := ctx.db.Where("id = ? AND user_id = ?", id, userID).Delete(&models.ApiToken{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrApiTokenNotFound
	}
	return nil
}

// RevokeUserApiTokens deletes every token for a user (e.g. on account disable).
func (ctx *MahresourcesContext) RevokeUserApiTokens(userID uint) error {
	return ctx.db.Where("user_id = ?", userID).Delete(&models.ApiToken{}).Error
}
