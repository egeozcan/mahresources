package application_context

import (
	"encoding/json"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
)

// ScrubResourceFromBlocks removes resourceID from every gallery.resourceIds[]
// and every calendar.calendars[].source.resourceId in the note_blocks table.
//
// Called synchronously from the resource DELETE handler, BH-020.
func ScrubResourceFromBlocks(db *gorm.DB, resourceID uint) error {
	var blocks []struct {
		ID      uint
		Content string
	}
	if err := db.Raw(
		`SELECT id, CAST(content AS TEXT) AS content FROM note_blocks
         WHERE type IN ('gallery','calendar')`,
	).Scan(&blocks).Error; err != nil {
		return err
	}

	for _, b := range blocks {
		updated, changed, err := scrubResourceFromBlockContent(b.Content, resourceID)
		if err != nil {
			return fmt.Errorf("block %d: %w", b.ID, err)
		}
		if !changed {
			continue
		}
		if err := db.Exec(
			`UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
		).Error; err != nil {
			return err
		}
	}

	return nil
}

// ScrubGroupFromBlocks removes groupID from references.groupIds[].
func ScrubGroupFromBlocks(db *gorm.DB, groupID uint) error {
	var blocks []struct {
		ID      uint
		Content string
	}
	if err := db.Raw(
		`SELECT id, CAST(content AS TEXT) AS content FROM note_blocks WHERE type = 'references'`,
	).Scan(&blocks).Error; err != nil {
		return err
	}

	for _, b := range blocks {
		updated, changed, err := scrubGroupFromBlockContent(b.Content, groupID)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := db.Exec(
			`UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// ScrubQueryFromBlocks nulls queryId in every table-block whose queryId matches.
func ScrubQueryFromBlocks(db *gorm.DB, queryID uint) error {
	var blocks []struct {
		ID      uint
		Content string
	}
	if err := db.Raw(
		`SELECT id, CAST(content AS TEXT) AS content FROM note_blocks WHERE type = 'table'`,
	).Scan(&blocks).Error; err != nil {
		return err
	}
	for _, b := range blocks {
		updated, changed, err := scrubQueryFromBlockContent(b.Content, queryID)
		if err != nil {
			return err
		}
		if !changed {
			continue
		}
		if err := db.Exec(
			`UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// scrubResourceFromBlockContent removes the resourceID from gallery.resourceIds[]
// and calendar.calendars[].source.resourceId in the given JSON content string.
// Returns the updated JSON, whether anything changed, and any error.
func scrubResourceFromBlockContent(content string, resourceID uint) (string, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false, err
	}
	changed := false

	// Gallery: content.resourceIds = [id, id, ...]
	if ids, ok := raw["resourceIds"].([]any); ok {
		filtered := make([]any, 0, len(ids))
		for _, v := range ids {
			if toUint(v) != resourceID {
				filtered = append(filtered, v)
			} else {
				changed = true
			}
		}
		if changed {
			raw["resourceIds"] = filtered
		}
	}

	// Calendar: content.calendars[].source.resourceId
	if cals, ok := raw["calendars"].([]any); ok {
		calChanged := false
		for i, c := range cals {
			cmap, ok := c.(map[string]any)
			if !ok {
				continue
			}
			source, ok := cmap["source"].(map[string]any)
			if !ok {
				continue
			}
			if rid, ok := source["resourceId"]; ok && toUint(rid) == resourceID {
				delete(source, "resourceId")
				cmap["source"] = source
				cals[i] = cmap
				calChanged = true
			}
		}
		if calChanged {
			raw["calendars"] = cals
			changed = true
		}
	}

	if !changed {
		return content, false, nil
	}
	out, err := json.Marshal(raw)
	if err != nil {
		return content, false, err
	}
	return string(out), true, nil
}

// scrubGroupFromBlockContent removes the groupID from references.groupIds[].
func scrubGroupFromBlockContent(content string, groupID uint) (string, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false, err
	}
	ids, ok := raw["groupIds"].([]any)
	if !ok {
		return content, false, nil
	}
	filtered := make([]any, 0, len(ids))
	changed := false
	for _, v := range ids {
		if toUint(v) != groupID {
			filtered = append(filtered, v)
		} else {
			changed = true
		}
	}
	if !changed {
		return content, false, nil
	}
	raw["groupIds"] = filtered
	out, err := json.Marshal(raw)
	if err != nil {
		return content, false, err
	}
	return string(out), true, nil
}

// scrubQueryFromBlockContent removes the queryId from table block content if it matches.
func scrubQueryFromBlockContent(content string, queryID uint) (string, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false, err
	}
	if qid, ok := raw["queryId"]; ok && toUint(qid) == queryID {
		delete(raw, "queryId")
		out, err := json.Marshal(raw)
		if err != nil {
			return content, false, err
		}
		return string(out), true, nil
	}
	return content, false, nil
}

// MigrateBlockReferencesOnce scans all note_blocks for dangling IDs and removes them.
// It is a one-shot operation: completion is recorded in the plugin_kvs table so it
// does not re-run on subsequent boots.
//
// To skip the migration (e.g., on large deployments), set SKIP_BLOCK_REF_CLEANUP=1
// or pass -skip-block-ref-cleanup to the server binary.
func MigrateBlockReferencesOnce(db *gorm.DB) error {
	const markerKey = "block_ref_cleanup_v1"

	// Check if already completed
	var completed struct{ Value string }
	db.Raw(`SELECT value FROM plugin_kvs WHERE plugin_name = '_system' AND key = ?`, markerKey).Scan(&completed)
	if completed.Value == "done" {
		return nil
	}

	// Pre-fetch existing IDs
	existingResources := map[uint]bool{}
	existingGroups := map[uint]bool{}
	existingQueries := map[uint]bool{}
	{
		var rows []uint
		db.Raw(`SELECT id FROM resources`).Scan(&rows)
		for _, id := range rows {
			existingResources[id] = true
		}
	}
	{
		var rows []uint
		db.Raw(`SELECT id FROM groups`).Scan(&rows)
		for _, id := range rows {
			existingGroups[id] = true
		}
	}
	{
		var rows []uint
		db.Raw(`SELECT id FROM queries`).Scan(&rows)
		for _, id := range rows {
			existingQueries[id] = true
		}
	}

	// Load all relevant blocks
	var blocks []struct {
		ID      uint
		Type    string
		Content string
	}
	if err := db.Raw(
		`SELECT id, type, CAST(content AS TEXT) AS content FROM note_blocks
         WHERE type IN ('gallery','references','calendar','table')`,
	).Scan(&blocks).Error; err != nil {
		return err
	}

	for _, b := range blocks {
		updated := b.Content
		anyChanged := false

		switch b.Type {
		case "gallery":
			if u, c := scrubMissingIdsFromArrayField(updated, "resourceIds", existingResources); c {
				updated = u
				anyChanged = true
			}
			if u, c := scrubMissingCalendarResources(updated, existingResources); c {
				updated = u
				anyChanged = true
			}
		case "references":
			if u, c := scrubMissingIdsFromArrayField(updated, "groupIds", existingGroups); c {
				updated = u
				anyChanged = true
			}
		case "calendar":
			if u, c := scrubMissingCalendarResources(updated, existingResources); c {
				updated = u
				anyChanged = true
			}
		case "table":
			if u, c := scrubMissingQueryIdField(updated, existingQueries); c {
				updated = u
				anyChanged = true
			}
		}

		if anyChanged {
			if err := db.Exec(
				`UPDATE note_blocks SET content = ? WHERE id = ?`, updated, b.ID,
			).Error; err != nil {
				return err
			}
		}
	}

	// Record completion using GORM's upsert (portable SQLite + Postgres)
	marker := models.PluginKV{
		PluginName: "_system",
		Key:        markerKey,
		Value:      "done",
	}
	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "plugin_name"}, {Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value", "updated_at"}),
	}).Create(&marker).Error
}

// scrubMissingIdsFromArrayField removes IDs from a JSON array field that are not in existing.
func scrubMissingIdsFromArrayField(content, field string, existing map[uint]bool) (string, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false
	}
	ids, ok := raw[field].([]any)
	if !ok {
		return content, false
	}
	filtered := make([]any, 0, len(ids))
	changed := false
	for _, v := range ids {
		if existing[toUint(v)] {
			filtered = append(filtered, v)
		} else {
			changed = true
		}
	}
	if !changed {
		return content, false
	}
	raw[field] = filtered
	out, _ := json.Marshal(raw)
	return string(out), true
}

// scrubMissingCalendarResources removes resourceId from calendar sources when the resource no longer exists.
func scrubMissingCalendarResources(content string, existing map[uint]bool) (string, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false
	}
	cals, ok := raw["calendars"].([]any)
	if !ok {
		return content, false
	}
	changed := false
	for i, c := range cals {
		cmap, ok := c.(map[string]any)
		if !ok {
			continue
		}
		source, ok := cmap["source"].(map[string]any)
		if !ok {
			continue
		}
		if rid, ok := source["resourceId"]; ok && !existing[toUint(rid)] {
			delete(source, "resourceId")
			cmap["source"] = source
			cals[i] = cmap
			changed = true
		}
	}
	if !changed {
		return content, false
	}
	raw["calendars"] = cals
	out, _ := json.Marshal(raw)
	return string(out), true
}

// scrubMissingQueryIdField removes queryId from table block content when the query no longer exists.
func scrubMissingQueryIdField(content string, existing map[uint]bool) (string, bool) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(content), &raw); err != nil {
		return content, false
	}
	if qid, ok := raw["queryId"]; ok && !existing[toUint(qid)] {
		delete(raw, "queryId")
		out, _ := json.Marshal(raw)
		return string(out), true
	}
	return content, false
}

// toUint converts a JSON-decoded number or string to uint; returns 0 if not convertible.
func toUint(v any) uint {
	switch x := v.(type) {
	case float64:
		return uint(x)
	case int:
		return uint(x)
	case uint:
		return x
	case json.Number:
		n, _ := x.Int64()
		return uint(n)
	case string:
		var n uint
		fmt.Sscanf(x, "%d", &n)
		return n
	}
	return 0
}
