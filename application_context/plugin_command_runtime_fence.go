package application_context

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/models"
)

const pluginCommandRuntimeFenceKey = "plugin-command"

var errPluginCommandFenceBoundToOtherRoot = errors.New("plugin command database fence is bound to a different staging root")

func (ctx *MahresourcesContext) pluginCommandFenceOwned() bool {
	if ctx == nil || ctx.pluginCommandController == nil {
		return false
	}
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	defer controller.mu.Unlock()
	return controller.dbFence != "" && controller.lease != nil && controller.state == pluginCommandRuntimeActive
}

func (ctx *MahresourcesContext) requirePluginCommandFenceTx(tx *gorm.DB) error {
	if ctx == nil || ctx.pluginCommandController == nil {
		return nil // Embedders and legacy tests without the runtime controller.
	}
	controller := ctx.pluginCommandController
	controller.mu.Lock()
	token := controller.dbFence
	lease := controller.lease
	state := controller.state
	controller.mu.Unlock()
	if token == "" && lease == nil && state == pluginCommandRuntimeIdle {
		return nil // This context has never started the plugin command runtime.
	}
	if token == "" && lease == nil && state == pluginCommandRuntimeActive {
		return nil // Lease-less active runtimes are installed only by legacy unit fixtures.
	}
	if token == "" || lease == nil {
		return fmt.Errorf("plugin command runtime fence is not owned")
	}
	result := tx.Model(&models.JobRuntimeFence{}).
		Where("key = ? AND token = ?", pluginCommandRuntimeFenceKey, token).
		Update("acquired_at", gorm.Expr("acquired_at"))
	if result.Error != nil {
		return fmt.Errorf("lock plugin command runtime fence: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("plugin command runtime fence token is stale")
	}
	return nil
}

func (ctx *MahresourcesContext) acquirePluginCommandDBFence(stagingRoot string) (string, error) {
	if ctx == nil || ctx.db == nil {
		return "", fmt.Errorf("plugin command database fence has no database")
	}
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", fmt.Errorf("create plugin command fence token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes[:])
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		seed := models.JobRuntimeFence{Key: pluginCommandRuntimeFenceKey, StagingRoot: stagingRoot}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var current models.JobRuntimeFence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key = ?", pluginCommandRuntimeFenceKey).First(&current).Error; err != nil {
			return err
		}
		if current.StagingRoot != stagingRoot {
			return errPluginCommandFenceBoundToOtherRoot
		}
		return tx.Model(&models.JobRuntimeFence{}).Where("key = ? AND staging_root = ?", pluginCommandRuntimeFenceKey, stagingRoot).
			Updates(map[string]any{"token": token, "acquired_at": time.Now().UTC()}).Error
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

func (ctx *MahresourcesContext) releasePluginCommandDBFence(token string) error {
	if ctx == nil || ctx.db == nil || token == "" {
		return nil
	}
	result := ctx.db.Model(&models.JobRuntimeFence{}).
		Where("key = ? AND token = ?", pluginCommandRuntimeFenceKey, token).
		Updates(map[string]any{"token": "", "acquired_at": time.Time{}})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("plugin command database fence token is no longer owned")
	}
	return nil
}
