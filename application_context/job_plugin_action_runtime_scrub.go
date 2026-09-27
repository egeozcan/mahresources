package application_context

import (
	"encoding/json"
	"fmt"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"
)

// pluginActionRuntimeScrubBatch bounds one read of the scrub below.
const pluginActionRuntimeScrubBatch = 500

// ScrubPluginActionRuntimeFromSummaries moves the runtime identity out of every
// stored plugin-action summary that still carries it, onto the Job's
// OriginRuntime, and reports how many Jobs it rewrote.
//
// Earlier releases wrote the identity (host, boot session and pid) into the
// sanitized summary, which every owner is shown, every listing returns and the
// search box matches. A release that stops writing it still serves those rows
// until they are rewritten, so startup rewrites them. Reconciliation reads the
// column, so a queued closure accepted by an earlier release is still judged by
// the process that started it.
//
// A summary is written once, at acceptance, and never updated, so each row is
// rewritten by id without a guard; the Job's version, timestamps and events are
// untouched, because nothing about the Job changed. Running it again finds
// nothing. A summary that is not a JSON object is left alone: it is not one this
// Kind wrote.
func (ctx *MahresourcesContext) ScrubPluginActionRuntimeFromSummaries() (int, error) {
	if ctx == nil || ctx.db == nil {
		return 0, nil
	}
	scrubbed := 0
	after := ""
	for {
		var rows []models.Job
		if err := ctx.db.Model(&models.Job{}).Select("id", "summary", "origin_runtime").
			Where("kind = ? AND id > ? AND CAST(summary AS TEXT) LIKE ?", JobKindPluginAction, after, `%"runtime"%`).
			Order("id").Limit(pluginActionRuntimeScrubBatch).Find(&rows).Error; err != nil {
			return scrubbed, fmt.Errorf("read plugin-action summaries: %w", err)
		}
		for _, row := range rows {
			after = row.ID
			var summary map[string]json.RawMessage
			if err := json.Unmarshal(row.Summary, &summary); err != nil {
				continue
			}
			raw, present := summary["runtime"]
			if !present {
				continue
			}
			delete(summary, "runtime")
			cleaned, err := json.Marshal(summary)
			if err != nil {
				return scrubbed, fmt.Errorf("encode the summary of %s: %w", row.ID, err)
			}
			updates := map[string]any{"summary": types.JSON(cleaned)}
			var runtime string
			if json.Unmarshal(raw, &runtime) == nil && row.OriginRuntime == "" &&
				runtime != "" && len(runtime) <= jobs.MaxOriginRuntimeBytes {
				updates["origin_runtime"] = runtime
			}
			if err := ctx.db.Model(&models.Job{}).Where("id = ?", row.ID).UpdateColumns(updates).Error; err != nil {
				return scrubbed, fmt.Errorf("rewrite the summary of %s: %w", row.ID, err)
			}
			scrubbed++
		}
		if len(rows) < pluginActionRuntimeScrubBatch {
			return scrubbed, nil
		}
	}
}

// legacySummaryRuntime reads the runtime identity from a summary written before
// the identity moved to the Job's OriginRuntime, for a row the scrub has not
// rewritten yet: one written by an older process still running beside this one.
// An unreadable summary yields an empty identity, which proves nothing.
func legacySummaryRuntime(summary json.RawMessage) string {
	var legacy struct {
		Runtime string `json:"runtime"`
	}
	if len(summary) == 0 || json.Unmarshal(summary, &legacy) != nil {
		return ""
	}
	return legacy.Runtime
}
