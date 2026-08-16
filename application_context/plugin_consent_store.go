package application_context

import (
	"errors"
	"fmt"

	"mahresources/models"
	"mahresources/plugin_system"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// pluginConsentStore is plugin_system.ConsentStore over PluginState.GrantsJSON.
//
// It is the seam that keeps the plugin manager out of the layer that owns the
// database, like the entity querier and the KV store beside it: the manager asks
// what an operator agreed to, and this answers from the row.
type pluginConsentStore struct {
	ctx *MahresourcesContext
}

// ConsentFor reads the stored record for one plugin.
//
// A missing row is "no record", which the manager grandfathers — every row that
// predates the GrantsJSON column reads that way. A failed read is *not*: an
// error here refuses the load, because reporting a transient database failure as
// an absence would grandfather whatever the file now asks for, once per failure.
func (s *pluginConsentStore) ConsentFor(pluginName string) (plugin_system.Grants, bool, error) {
	var state models.PluginState
	if err := s.ctx.db.Where("plugin_name = ?", pluginName).First(&state).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plugin_system.Grants{}, false, nil
		}
		return plugin_system.Grants{}, false, fmt.Errorf("reading stored consent for plugin %q: %w", pluginName, err)
	}
	// Verbatim: ParseGrants already draws the line between "no record" and "a
	// record granting nothing", and between an empty column and a corrupt one.
	return plugin_system.ParseGrants(state.GrantsJSON)
}

// RecordConsent writes what the operator agreed to.
//
// Column-scoped, like every other mutation on this table: a full-struct write
// would carry a zero SettingsJSON along with it and wipe the plugin's settings.
func (s *pluginConsentStore) RecordConsent(pluginName string, granted plugin_system.Grants) error {
	value, err := granted.Marshal()
	if err != nil {
		return fmt.Errorf("encoding consent for plugin %q: %w", pluginName, err)
	}

	// An upsert, not an UPDATE, because a plugin can be loaded before its state
	// row exists: EnsurePluginStates runs before ActivateEnabledPlugins at boot,
	// but nothing makes that true for a manager driven directly — which is how
	// the hook tests, and any embedder, use it. An UPDATE there matches no rows,
	// and refusing the load over a bookkeeping row absent for a legitimate
	// reason turns consent into a availability problem.
	//
	// One statement rather than update-then-insert: the two-step version races
	// a concurrent enable of the same plugin, and the loser's FirstOrCreate
	// finds the winner's row and silently declines to write the consent — the
	// exact silent-drop this must not have.
	//
	// DoUpdates names grants_json alone, so an existing row keeps its enabled
	// flag and its settings; a newly inserted one takes Enabled's zero value,
	// which is what EnsurePluginStates would have created anyway.
	result := s.ctx.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_name"}},
		DoUpdates: clause.AssignmentColumns([]string{"grants_json"}),
	}).Create(&models.PluginState{PluginName: pluginName, GrantsJSON: value})
	if result.Error != nil {
		return fmt.Errorf("recording consent for plugin %q: %w", pluginName, result.Error)
	}
	// A write that changed nothing is not a success: the next load would find no
	// record, grandfather again, and accept a widening without asking anyone —
	// the one thing storing consent exists to prevent.
	if result.RowsAffected == 0 {
		return fmt.Errorf("recording consent for plugin %q: the write affected no rows, so the consent was not stored", pluginName)
	}
	return nil
}
