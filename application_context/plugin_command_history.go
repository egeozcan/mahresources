package application_context

import (
	"mahresources/models"
	"mahresources/plugin_commands"
)

// GetPluginCommandRuns returns one bounded, newest-first administrator page.
// Output and import rows are deliberately excluded from the list query; detail
// is the only surface that loads them.
func (ctx *MahresourcesContext) GetPluginCommandRuns(offset, limit int) ([]plugin_commands.RunRecord, int64, error) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var count int64
	if err := ctx.db.Model(&models.PluginCommandRun{}).Count(&count).Error; err != nil {
		return nil, 0, err
	}
	var rows []models.PluginCommandRun
	if err := ctx.db.Order("created_at desc, id desc").Offset(offset).Limit(limit).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	runs := make([]plugin_commands.RunRecord, len(rows))
	for i, row := range rows {
		// The writer epoch retires ParamsJSON and InputsJSON in this source
		// table. Show the safe input names and byte counts from canonical replay
		// while it is available. A forgotten or unreadable envelope leaves this
		// history row visible with its input metadata empty.
		if err := ctx.hydratePluginCommandRun(&row, ctx.db); err != nil {
			row.ParamsJSON, row.InputsJSON = "", ""
		}
		runs[i] = runRecord(row)
	}
	return runs, count, nil
}

// GetPluginCommandRun returns durable detail and whether the independently
// prunable output row still exists.
func (ctx *MahresourcesContext) GetPluginCommandRun(id string) (plugin_commands.RunView, bool, error) {
	record, output, err := ctx.Run(id)
	if err != nil {
		return plugin_commands.RunView{}, false, err
	}
	var imports []models.PluginCommandImportMap
	if err := ctx.db.Where("run_id = ?", id).Order("id asc").Find(&imports).Error; err != nil {
		return plugin_commands.RunView{}, false, err
	}
	mapped := make([]plugin_commands.ImportMapEntry, len(imports))
	for i, row := range imports {
		mapped[i] = importMapEntry(row)
	}
	return plugin_commands.RunView{RunRecord: record, Output: output, Imports: mapped}, output.RunID != "", nil
}

// PluginCommandRuntimeAvailability reports whether this process can enforce a
// command mutation. The reason is intentionally the same actionable, generic
// quarantine explanation returned to command callers; private blocker details
// remain in /logs.
func (ctx *MahresourcesContext) PluginCommandRuntimeAvailability() (available bool, reason string) {
	if _, err := ctx.pluginCommandActive(); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// CancelPluginCommandRun is the single administrator seam. It first loads the
// active controller snapshot, so a quarantined process never writes a durable
// cancellation label it cannot enforce. Once active it delegates to the
// dispatcher so history and live-cockpit cancellation share the durable latch
// and pre-fork/process-group handling.
func (ctx *MahresourcesContext) CancelPluginCommandRun(id string) error {
	if _, _, err := ctx.Run(id); err != nil {
		return err
	}
	active, err := ctx.pluginCommandActive()
	if err != nil {
		return err
	}
	return active.dispatcher.Cancel(id, "operator cancelled")
}
