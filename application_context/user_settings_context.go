package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	// maxUserSettingValueSize bounds a single setting's JSON blob. The lightbox
	// quick-tag payload (4×9 slots + 9 recents) is a few KB; 256KB leaves generous
	// headroom while stopping a buggy/hostile client from storing megabytes per key.
	maxUserSettingValueSize = 256 * 1024
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
	if len(value) > maxUserSettingValueSize {
		return fmt.Errorf("%w: value size %d exceeds maximum of %d bytes", ErrUserSettingValue, len(value), maxUserSettingValueSize)
	}
	if !json.Valid(value) {
		return fmt.Errorf("%w: not valid JSON", ErrUserSettingValue)
	}

	// Enforce the per-user key cap, but only for a genuinely new key — updating an
	// existing key must always be allowed even at the cap.
	var existing int64
	if err := ctx.db.Model(&models.UserSetting{}).
		Where("user_id = ? AND key = ?", userID, key).Count(&existing).Error; err != nil {
		return err
	}
	if existing == 0 {
		var total int64
		if err := ctx.db.Model(&models.UserSetting{}).
			Where("user_id = ?", userID).Count(&total).Error; err != nil {
			return err
		}
		if total >= MaxUserSettingKeysPerUser {
			return fmt.Errorf("%w: limit is %d", ErrTooManySettings, MaxUserSettingKeysPerUser)
		}
	}

	row := models.UserSetting{UserId: userID, Key: key, Value: string(value)}
	return ctx.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&row).Error
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
