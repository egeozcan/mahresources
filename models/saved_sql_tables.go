package models

import (
	"sort"
	"strings"
)

// savedSQLTables classifies every table the server migrates: true means saved
// SQL may read it, false means it may not. It is the single definition both
// databases use. On SQLite the read-only driver's authorizer refuses a read of
// any table that is not allowed here; on PostgreSQL VerifyPostgresReadOnlyRole
// refuses a role that can SELECT one, and PostgresReadOnlyRoleSQL grants SELECT
// on exactly the allowed ones.
//
// Saved SQL is authored by editors and run by every unscoped principal, so a
// table is allowed only when it is content those principals can already read
// through the API: entities, their join tables and their full-text indexes.
// Credentials, secrets, the audit log, per-account data, job and plugin
// internals, and anything owner-restricted are not.
//
// A table the server migrates must appear here. TestSavedSQLTablesClassifyEveryMigratedTable
// (main package, SQLite and PostgreSQL) fails on one that does not, so a new
// table is unreadable until someone decides otherwise.
var savedSQLTables = map[string]bool{
	// Entities.
	"resources":             true,
	"resource_versions":     true,
	"previews":              true,
	"image_hashes":          true,
	"resource_similarities": true,
	"notes":                 true,
	"note_blocks":           true,
	"note_types":            true,
	"groups":                true,
	"group_relations":       true,
	"group_relation_types":  true,
	"tags":                  true,
	"categories":            true,
	"resource_categories":   true,
	"series":                true,
	"queries":               true,
	"saved_mrql_queries":    true,
	"template_partials":     true,
	// Their join tables.
	"resource_tags":            true,
	"resource_notes":           true,
	"note_tags":                true,
	"group_tags":               true,
	"groups_related_resources": true,
	"groups_related_notes":     true,
	"group_related_groups":     true,

	// Credentials and sessions.
	"users":      false, // password hashes, roles
	"sessions":   false, // session token hashes, CSRF tokens
	"api_tokens": false, // API token hashes
	// Admin-only system state.
	"runtime_settings": false,
	"log_entries":      false, // the audit log
	// Another account's private data.
	"user_settings":       false,
	"saved_searches":      false,
	"resource_reductions": false, // visible to the owner and admins only
	// Template history: snapshots the API does not expose.
	"template_revisions": false,
	// Plugin state: settings hold secrets in plaintext.
	"plugin_states":                             false,
	"plugin_kvs":                                false,
	"plugin_schedules":                          false,
	"plugin_command_runs":                       false,
	"plugin_command_run_outputs":                false,
	"plugin_command_imports":                    false,
	"plugin_command_import_maps":                false,
	"plugin_command_import_command_facts":       false,
	"plugin_command_import_command_fact_groups": false,
	// Downloads and Jobs: per-owner, and their inputs can carry URLs with
	// credentials in them.
	"download_history_entries":  false,
	"scheduled_downloads":       false,
	"jobs":                      false,
	"job_resource_receipts":     false,
	"job_events":                false,
	"job_event_sequences":       false,
	"job_links":                 false,
	"job_outputs":               false,
	"job_replay_envelopes":      false, // sealed job inputs
	"job_claims":                false,
	"job_capacity_leases":       false,
	"job_preferences":           false,
	"job_pin_guards":            false,
	"job_command_requests":      false,
	"job_legacy_handles":        false,
	"job_import_command_facts":  false,
	"job_writer_epochs":         false,
	"job_source_mappings":       false,
	"job_migration_checkpoints": false,
	"job_runtime_fences":        false,
}

// ftsShadowSuffixes are the tables FTS5 keeps beside a full-text index named
// <table>_fts (fts/sqlite.go): they hold the same data as the index.
var ftsShadowSuffixes = []string{"", "_data", "_idx", "_content", "_docsize", "_config"}

// savedSQLBaseTable maps a full-text index table, or one of its FTS5 shadow
// tables, to the table it indexes; any other name maps to itself.
func savedSQLBaseTable(table string) string {
	i := strings.LastIndex(table, "_fts")
	if i <= 0 {
		return table
	}
	for _, suffix := range ftsShadowSuffixes {
		if table[i+len("_fts"):] == suffix {
			return table[:i]
		}
	}
	return table
}

// IsSavedSQLAllowedTable reports whether saved SQL may read table, and whether
// the schema listing offered to query authors includes it. A full-text index
// over an allowed table and its FTS5 shadow tables are allowed with it. Any
// other name, including one savedSQLTables does not mention, is not.
func IsSavedSQLAllowedTable(table string) bool {
	return savedSQLTables[savedSQLBaseTable(strings.ToLower(table))]
}

// IsSavedSQLClassifiedTable reports whether savedSQLTables decides table, as
// opposed to refusing it only because it is unknown.
func IsSavedSQLClassifiedTable(table string) bool {
	_, ok := savedSQLTables[savedSQLBaseTable(strings.ToLower(table))]
	return ok
}

// SavedSQLAllowedTables lists the tables saved SQL may read, sorted, leaving
// out full-text index tables (which exist on SQLite only).
func SavedSQLAllowedTables() []string {
	var tables []string
	for table, allowed := range savedSQLTables {
		if allowed {
			tables = append(tables, table)
		}
	}
	sort.Strings(tables)
	return tables
}

// SavedSQLTableClassifications returns a copy of savedSQLTables, for the test
// that checks it against the migrated schema.
func SavedSQLTableClassifications() map[string]bool {
	out := make(map[string]bool, len(savedSQLTables))
	for table, allowed := range savedSQLTables {
		out[table] = allowed
	}
	return out
}
