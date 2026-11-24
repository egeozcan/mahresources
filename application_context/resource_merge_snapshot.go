package application_context

import (
	"fmt"
	"strings"
)

// The merge's Loser backup snapshot, built entirely in SQL (design D13).
//
// Today the merge loads each Loser whole, marshals it with json.Marshal and
// writes the result into the Winner's meta.backups.resource_<id>. The snapshot
// is now one SQL expression per dialect, generated from resourceSnapshotFields,
// which lists every models.Resource field with the JSON key json.Marshal gives it
// and how the value is produced. A reflection test over models.Resource keeps the
// list complete: a new field fails until it is mapped here.

// snapKind says how a Resource field becomes a JSON value.
type snapKind int

const (
	snapUint     snapKind = iota // a number column
	snapString                   // a text column, always present
	snapNullable                 // a pointer column: null or its value
	snapOmitNil                  // a pointer column with omitempty: the key is absent when NULL
	snapTime                     // a time column, RFC3339Nano
	snapMeta                     // meta: embedded JSON, backups stripped when an object
	snapJSON                     // own_meta: embedded JSON
	snapNull                     // a relation json.Marshal writes as null on an unloaded row
	snapAbsent                   // omitempty and empty on an unloaded row: no key
)

type snapField struct {
	Field  string // the Go field name on models.Resource
	Key    string // the JSON key json.Marshal writes
	Column string // the column, "" for snapNull and snapAbsent
	Kind   snapKind
}

// resourceSnapshotFields is in models.Resource's field order.
var resourceSnapshotFields = []snapField{
	{"ID", "ID", "id", snapUint},
	{"CreatedAt", "CreatedAt", "created_at", snapTime},
	{"UpdatedAt", "UpdatedAt", "updated_at", snapTime},
	{"CreatedByUserId", "createdByUserId", "created_by_user_id", snapOmitNil},
	{"GUID", "guid", "guid", snapOmitNil},
	{"Name", "Name", "name", snapString},
	{"OriginalName", "OriginalName", "original_name", snapString},
	{"OriginalLocation", "OriginalLocation", "original_location", snapString},
	{"Hash", "Hash", "hash", snapString},
	{"HashType", "HashType", "hash_type", snapString},
	{"Location", "Location", "location", snapString},
	{"StorageLocation", "StorageLocation", "storage_location", snapNullable},
	{"Description", "Description", "description", snapString},
	{"Meta", "Meta", "meta", snapMeta},
	{"Width", "Width", "width", snapUint},
	{"Height", "Height", "height", snapUint},
	{"FileSize", "FileSize", "file_size", snapUint},
	{"Category", "Category", "category", snapString},
	{"ContentType", "ContentType", "content_type", snapString},
	{"ContentCategory", "ContentCategory", "content_category", snapString},
	{"ResourceCategoryId", "resourceCategoryId", "resource_category_id", snapUint},
	{"ResourceCategory", "resourceCategory", "", snapAbsent},
	{"SeriesID", "seriesId", "series_id", snapNullable},
	{"Series", "series", "", snapAbsent},
	{"OwnMeta", "ownMeta", "own_meta", snapJSON},
	{"Tags", "Tags", "", snapNull},
	{"Notes", "Notes", "", snapNull},
	{"Groups", "Groups", "", snapNull},
	{"Owner", "Owner", "", snapNull},
	{"OwnerId", "OwnerId", "owner_id", snapNullable},
	{"Previews", "Previews", "", snapNull},
	{"CurrentVersionID", "currentVersionId", "current_version_id", snapNullable},
	{"CurrentVersion", "currentVersion", "", snapAbsent},
	{"Versions", "versions", "", snapAbsent},
	{"ImageHash", "imageHash", "", snapAbsent},
	{"RenderedHTML", "renderedHTML", "", snapAbsent},
	{"SimilarityDistance", "similarityDistance", "", snapAbsent},
}

// snapshotCol qualifies a column with the alias of the row being snapshotted.
func snapshotCol(alias, col string) string { return alias + "." + col }

// sqliteTime is the RFC3339Nano rendering json.Marshal gives the time the driver loads
// from a stored timestamp: the stored wall clock and offset, not the instant in UTC. The
// driver stores "2006-01-02 15:04:05.999999999-07:00" and parses it back with that offset;
// the fraction keeps its digits with trailing zeros dropped, and a zero offset is "Z", as
// Go writes it. A stored value without an offset reads as UTC. A form this does not
// recognise falls back to the UTC rendering of the same instant.
func sqliteTime(col string) string {
	// The driver also reads a date alone and a time without seconds (with a space or a
	// T between them); those are completed to the full form the offsets below rely on.
	c := "(CASE length(" + col + ") WHEN 10 THEN " + col + " || ' 00:00:00' WHEN 16 THEN " + col + " || ':00' ELSE " + col + " END)"
	rest := "substr(" + c + ", 21)"
	end := "min(instr(" + rest + " || '+', '+'), instr(" + rest + " || '-', '-'), instr(" + rest + " || 'Z', 'Z'), instr(" + rest + " || ' ', ' '))"
	hasFrac := "substr(" + c + ", 20, 1) = '.'"
	frac := "rtrim(CASE WHEN " + hasFrac + " THEN substr(" + rest + ", 1, " + end + " - 1) ELSE '' END, '0')"
	fracText := "CASE WHEN length(" + frac + ") > 0 THEN '.' || " + frac + " ELSE '' END"
	off := "CASE WHEN " + hasFrac + " THEN substr(" + c + ", 20 + " + end + ") ELSE substr(" + c + ", 20) END"
	offText := "CASE WHEN " + off + " IN ('', 'Z', '+00:00', '-00:00') THEN 'Z' ELSE " + off + " END"
	offOK := "(" + off + " IN ('', 'Z') OR (substr(" + off + ", 1, 1) IN ('+', '-') AND length(" + off + ") = 6 AND substr(" + off + ", 4, 1) = ':'))"
	wall := "substr(" + c + ", 1, 10) || 'T' || substr(" + c + ", 12, 8)"
	utc := "strftime('%Y-%m-%dT%H:%M:%S', " + c + ")"
	return "(CASE WHEN " + offOK + " THEN " + wall + " || " + fracText + " || " + offText +
		" ELSE " + utc + " || " + fracText + " || 'Z' END)"
}

// resourceSnapshotExpr is the SQL expression for one row's snapshot object, over
// the row aliased as alias, for the dialect.
func resourceSnapshotExpr(dialect, alias string) string {
	postgres := dialect == "postgres"
	var pairs []string
	var optional []string
	for _, f := range resourceSnapshotFields {
		key := "'" + f.Key + "'"
		col := ""
		if f.Column != "" {
			col = snapshotCol(alias, f.Column)
		}
		var val string
		switch f.Kind {
		case snapAbsent:
			continue
		case snapNull:
			val = "NULL"
		case snapNullable:
			val = col
		case snapUint:
			// A NULL scans into a Go zero value, which json.Marshal writes as 0.
			val = "COALESCE(" + col + ", 0)"
		case snapString:
			val = "COALESCE(" + col + ", '')"
		case snapOmitNil:
			if postgres {
				optional = append(optional, "CASE WHEN "+col+" IS NOT NULL THEN jsonb_build_object("+key+", "+col+") ELSE '{}'::jsonb END")
			} else {
				optional = append(optional, "CASE WHEN "+col+" IS NOT NULL THEN json_object("+key+", "+col+") ELSE '{}' END")
			}
			continue
		case snapTime:
			if postgres {
				val = "COALESCE(rtrim(rtrim(to_char(" + col + " AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.US'), '0'), '.') || 'Z', '0001-01-01T00:00:00Z')"
			} else {
				val = "COALESCE(" + sqliteTime(col) + ", '0001-01-01T00:00:00Z')"
			}
		case snapMeta:
			if postgres {
				val = "COALESCE(CASE WHEN jsonb_typeof(" + col + ") = 'object' THEN " + col + " - '" + metaBackupsKey + "' ELSE " + col + " END, 'null'::jsonb)"
			} else {
				// json_remove drops the first of a repeated key and Go's decode keeps the last, so
				// a repeated backups key is removed once per occurrence. json_remove keeps every
				// other member's text as stored, which a rebuild through json_each would not (it
				// hands back a real as a double: 9.1234567891234549 would come out ...555).
				occurrences := "(SELECT count(*) FROM json_each(" + col + ") m WHERE m.key = '" + metaBackupsKey + "')"
				stripped := "(WITH RECURSIVE r(doc, n) AS (SELECT " + col + ", " + occurrences + " UNION ALL SELECT json_remove(doc, '$." + metaBackupsKey + "'), n - 1 FROM r WHERE n > 0) SELECT doc FROM r WHERE n = 0)"
				val = "json(COALESCE(CASE WHEN json_type(" + col + ") = 'object' THEN " + stripped + " ELSE " + col + " END, 'null'))"
			}
		case snapJSON:
			if postgres {
				val = "COALESCE(" + col + ", 'null'::jsonb)"
			} else {
				val = "json(COALESCE(" + col + ", 'null'))"
			}
		}
		pairs = append(pairs, key+", "+val)
	}
	if postgres {
		expr := "jsonb_build_object(" + strings.Join(pairs, ", ") + ")"
		for _, o := range optional {
			expr += " || " + o
		}
		return expr
	}
	expr := "json_object(" + strings.Join(pairs, ", ") + ")"
	if len(optional) > 0 {
		// json_patch merges the optional keys in; their values are never null.
		for _, o := range optional {
			expr = "json_patch(" + expr + ", " + o + ")"
		}
	}
	return expr
}

// mergeBackupsUpdateSQL is the one UPDATE of a merge that appends every Loser's
// snapshot to the Winner's meta.backups, keeping what is there. The guards are the
// ones the Go-built version used: a NULL or JSON null meta, and a backups of null,
// both read as empty. Binds: the Loser ids, then the Winner id.
func mergeBackupsUpdateSQL(dialect string) string {
	if dialect == "postgres" {
		agg := "(SELECT jsonb_object_agg('resource_' || l.id, " + resourceSnapshotExpr(dialect, "l") + ") FROM resources l WHERE l.id IN ?)"
		return fmt.Sprintf("UPDATE resources SET meta = jsonb_set(COALESCE(nullif(meta, 'null'::jsonb), '{}'::jsonb), '{%s}', COALESCE(nullif(meta->'%s', 'null'::jsonb), '{}'::jsonb) || %s) WHERE id = ?", metaBackupsKey, metaBackupsKey, agg)
	}
	agg := "json((SELECT json_group_object(k, json(v)) FROM (SELECT 'resource_' || l.id AS k, " + resourceSnapshotExpr(dialect, "l") + " AS v FROM resources l WHERE l.id IN ?)))"
	return fmt.Sprintf("UPDATE resources SET meta = json_patch(COALESCE(nullif(meta, 'null'), '{}'), json_object('%s', %s)) WHERE id = ?", metaBackupsKey, agg)
}
