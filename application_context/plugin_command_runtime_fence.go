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
	"mahresources/plugin_commands"
	"mahresources/plugin_system"
)

const pluginCommandRuntimeFenceKey = "plugin-command"

var (
	errPluginCommandFenceBoundToOtherRoot = errors.New("plugin command database fence is bound to a different staging root")
	// errPluginCommandFenceRootIsDurable explains a binding no retry can move:
	// a durable root keeps the database's retained command outputs and import
	// sources, so the binding outlives every restart.
	errPluginCommandFenceRootIsDurable = errors.New("that root keeps this database's retained command outputs and import sources, so the binding outlives restarts; restart with -plugin-command-staging-path set to it")
	// errPluginCommandFenceOwnerNotStopped explains a binding that moves once its
	// owner stops: the bound root is private to a process that is still running
	// or runs where this process cannot inspect it.
	errPluginCommandFenceOwnerNotStopped = errors.New("that root is private to the process holding the fence, which is still running or cannot be inspected from here (another host or an earlier boot); the binding ends when it stops, or once a server started with -plugin-command-staging-path set to that root stops cleanly")
)

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
		return plugin_commands.ErrRuntimeFenceLost
	}
	result := tx.Model(&models.JobRuntimeFence{}).
		Where("key = ? AND token = ?", pluginCommandRuntimeFenceKey, token).
		Update("acquired_at", gorm.Expr("acquired_at"))
	if result.Error != nil {
		return fmt.Errorf("lock plugin command runtime fence: %w", result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("%w: its token is stale", plugin_commands.ErrRuntimeFenceLost)
	}
	return nil
}

// acquirePluginCommandDBFence takes the fence for a durable staging root.
func (ctx *MahresourcesContext) acquirePluginCommandDBFence(stagingRoot string) (string, error) {
	return ctx.acquirePluginCommandDBFenceFor(stagingRoot, false)
}

// acquirePluginCommandDBFenceFor binds the database's command runtime to one
// staging root and takes its fence token. The caller already holds that root's
// runtime lease, which is what proves no other process on this host is using
// the same root, so the token of a fence bound to it is simply replaced.
//
// A fence bound to another root is taken over only when that root was private
// to its process (temporary): such a root is deleted with its process, so the
// binding ends once the process has released the fence or is proved gone by its
// recorded runtime identity. A durable root keeps the database's retained
// outputs and import sources, and its binding survives every restart.
func (ctx *MahresourcesContext) acquirePluginCommandDBFenceFor(stagingRoot string, temporary bool) (string, error) {
	if ctx == nil || ctx.db == nil {
		return "", fmt.Errorf("plugin command database fence has no database")
	}
	var tokenBytes [16]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return "", fmt.Errorf("create plugin command fence token: %w", err)
	}
	token := hex.EncodeToString(tokenBytes[:])
	owner := plugin_system.CurrentRuntimeIdentity().String()
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		seed := models.JobRuntimeFence{Key: pluginCommandRuntimeFenceKey, StagingRoot: stagingRoot, StagingTemporary: temporary}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&seed).Error; err != nil {
			return err
		}
		var current models.JobRuntimeFence
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key = ?", pluginCommandRuntimeFenceKey).First(&current).Error; err != nil {
			return err
		}
		if current.StagingRoot != stagingRoot {
			if !current.StagingTemporary {
				return fmt.Errorf("%w %q; %w", errPluginCommandFenceBoundToOtherRoot, current.StagingRoot, errPluginCommandFenceRootIsDurable)
			}
			if !pluginCommandFenceOwnerStopped(current) {
				return fmt.Errorf("%w %q; %w", errPluginCommandFenceBoundToOtherRoot, current.StagingRoot, errPluginCommandFenceOwnerNotStopped)
			}
		} else if current.StagingTemporary {
			// A private root started on by name stays private: it is how an
			// operator frees a binding whose owner cannot be proved gone (after a
			// reboot that skipped the clean stop), and its clean stop then leaves
			// the binding to the next private root.
			temporary = true
		}
		result := tx.Model(&models.JobRuntimeFence{}).
			Where("key = ? AND staging_root = ? AND token = ?", pluginCommandRuntimeFenceKey, current.StagingRoot, current.Token).
			Updates(map[string]any{
				"token": token, "acquired_at": time.Now().UTC(), "owner": owner,
				"staging_root": stagingRoot, "staging_temporary": temporary,
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("plugin command database fence changed while it was being acquired")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// pluginCommandFenceOwnerStopped reports whether the process that holds a fence
// has provably stopped holding it: it released the token, or its recorded
// identity names a process that cannot still exist. An owner that was never
// recorded, one on another host, and one whose pid is taken all prove nothing.
func pluginCommandFenceOwnerStopped(fence models.JobRuntimeFence) bool {
	if fence.Token == "" {
		return true
	}
	identity, ok := plugin_system.ParseRuntimeIdentity(fence.Owner)
	return ok && identity.Liveness() == plugin_system.RuntimeGone
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
