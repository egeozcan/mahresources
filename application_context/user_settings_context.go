package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// MaxUserSettingValueSize bounds a single setting's JSON blob. The lightbox
	// quick-tag payload (4×9 slots + 9 recents) is a few KB; 256KB leaves generous
	// headroom while stopping a buggy/hostile client from storing megabytes per key.
	MaxUserSettingValueSize = 256 * 1024
	// maxUserSettingKeyLength matches the column size:128.
	maxUserSettingKeyLength = 128
	// MaxUserSettingKeysPerUser bounds the table so one account cannot exhaust it.
	// The known consumers use a handful of keys; 200 is far above real usage.
	MaxUserSettingKeysPerUser = 200
)

var (
	ErrNoSettingsOwner  = errors.New("no user to store settings for")
	ErrUserSettingKey   = errors.New("invalid setting key")
	ErrUserSettingValue = errors.New("invalid setting value")
	ErrTooManySettings  = errors.New("too many settings for this user")
)

// GetUserSettings returns all settings for the acting user as key → raw JSON value.
// The owner is resolved internally from the request principal (auth on) or the root
// admin (auth off); a 0 owner yields an empty map so a read never fails on identity.
func (ctx *MahresourcesContext) GetUserSettings() (map[string]json.RawMessage, error) {
	userID := ctx.actingUserID()
	out := map[string]json.RawMessage{}
	if userID == 0 {
		return out, nil
	}

	var rows []models.UserSetting
	if err := ctx.db.Where("user_id = ?", userID).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.Key] = json.RawMessage(row.Value)
	}
	return out, nil
}

// SetUserSetting upserts a single setting for the acting user. value must be a valid
// JSON document. Writes are rejected when there is no resolvable owner (0), the key is
// empty/oversize, the value is invalid/oversize, or the per-user key cap would be
// exceeded by a brand-new key.
func (ctx *MahresourcesContext) SetUserSetting(key string, value json.RawMessage) error {
	userID := ctx.actingUserID()
	if userID == 0 {
		return ErrNoSettingsOwner
	}
	if key == "" || len(key) > maxUserSettingKeyLength {
		return fmt.Errorf("%w: key must be 1-%d chars", ErrUserSettingKey, maxUserSettingKeyLength)
	}
	if len(value) == 0 {
		return fmt.Errorf("%w: empty value", ErrUserSettingValue)
	}
	if len(value) > MaxUserSettingValueSize {
		return fmt.Errorf("%w: value size %d exceeds maximum of %d bytes", ErrUserSettingValue, len(value), MaxUserSettingValueSize)
	}
	if !json.Valid(value) {
		return fmt.Errorf("%w: not valid JSON", ErrUserSettingValue)
	}

	// Upsert with an atomic per-user key cap. The cap must be enforced inside the write
	// itself: a check-then-insert lets two concurrent new-key requests both observe
	// total < cap and both succeed, bypassing the cap this code exists to enforce.
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		// Serialize concurrent writes for this user so the cap check is atomic. Postgres:
		// lock the user's existing rows FOR UPDATE (a no-op branch on SQLite, which
		// serializes writers within the transaction's write lock and where the
		// conditional INSERT below is a single write statement). Mirrors lockEnabledAdmins.
		if ctx.Config.DbType == constants.DbTypePosgres {
			var ids []uint
			if err := tx.Model(&models.UserSetting{}).
				Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("user_id = ?", userID).
				Order("id").
				Pluck("id", &ids).Error; err != nil {
				return err
			}
		}

		// Insert only when the key already exists (EXISTS → always an update) or the user
		// is under the cap. The COUNT is evaluated inside the single INSERT statement, so
		// with the serialization above it is atomic. SQLite requires a WHERE in the SELECT
		// when combined with ON CONFLICT, which we have. RowsAffected == 0 means a genuinely
		// new key was rejected by the cap (an existing key always matches the EXISTS branch).
		now := time.Now()
		res := tx.Exec(
			`INSERT INTO user_settings (user_id, key, value, created_at, updated_at)
			 SELECT ?, ?, ?, ?, ?
			 WHERE EXISTS (SELECT 1 FROM user_settings WHERE user_id = ? AND key = ?)
			    OR (SELECT COUNT(*) FROM user_settings WHERE user_id = ?) < ?
			 ON CONFLICT (user_id, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			userID, key, string(value), now, now,
			userID, key, userID, MaxUserSettingKeysPerUser,
		)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("%w: limit is %d", ErrTooManySettings, MaxUserSettingKeysPerUser)
		}
		return nil
	})
}

// DeleteUserSetting removes a single setting for the acting user. Deleting a missing
// key is a no-op (no error), matching the idempotent reset semantics of the other KV
// stores.
func (ctx *MahresourcesContext) DeleteUserSetting(key string) error {
	userID := ctx.actingUserID()
	if userID == 0 {
		return ErrNoSettingsOwner
	}
	err := ctx.db.Where("user_id = ? AND key = ?", userID, key).
		Delete(&models.UserSetting{}).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	return err
}
