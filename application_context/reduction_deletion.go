package application_context

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/models"
)

// Resource deletion, staged (design D9, D13).
//
// A deleting transaction stages the ids it removes in reduction_deleted_ids
// under one operation id, then walks the affected Clusters (the hook), scrubs the
// note blocks, and only then deletes the staged ids. Everything that needs "the
// ids this transaction deletes" reads them from the staging table, so nothing
// holds the whole set in Go.

// deleteStageChunk is the most staged ids one statement inserts or reads.
const deleteStageChunk = 500

// newDeleteOp is a fresh operation id for one deleting transaction.
func newDeleteOp() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// stageDeletedIDs records ids under op, in chunks. Staging an id twice is fine.
func stageDeletedIDs(db *gorm.DB, op string, ids []uint) error {
	for start := 0; start < len(ids); start += deleteStageChunk {
		end := start + deleteStageChunk
		if end > len(ids) {
			end = len(ids)
		}
		rows := make([]models.ReductionDeletedID, 0, end-start)
		for _, id := range ids[start:end] {
			rows = append(rows, models.ReductionDeletedID{Op: op, ResourceID: id})
		}
		if err := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error; err != nil {
			return err
		}
	}
	return nil
}

// purgeDeletedIDs removes op's staged ids. The outer transaction calls it last,
// after the note-block scrub.
func purgeDeletedIDs(db *gorm.DB, op string) error {
	return db.Exec("DELETE FROM reduction_deleted_ids WHERE op = ?", op).Error
}

// opLiteral quotes an operation id for inlining into the scrub's SQL. Operation
// ids are generated here, so anything but hex and dashes is a bug, not input.
func opLiteral(op string) string {
	for _, r := range op {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r == '-') {
			panic(fmt.Sprintf("malformed delete op id %q", op))
		}
	}
	return "'" + op + "'"
}

// scrubDeletedResourcesFromBlocks removes the staged ids of op from every
// gallery `resourceIds` array and calendar `calendars[].source.resourceId` in
// note_blocks, in SQL, once per transaction.
//
// Blocks are visited by keyset (id > ?, 500 per statement); each UPDATE's WHERE is
// an exact, type-guarded membership test, so only blocks that reference a removed
// id are written. What counts as a reference is what toUint accepts (see
// block_ref_cleanup.go): integers, floats truncated, and decimal strings with
// leading space and trailing garbage; "+12" is not one. A block whose content is
// not a JSON object, or whose field has another shape, is left alone.
func scrubDeletedResourcesFromBlocks(db *gorm.DB, op string) error {
	var s blockScrubSQL
	switch db.Dialector.Name() {
	case "postgres":
		s = postgresBlockScrub(opLiteral(op))
	default:
		s = sqliteBlockScrub(opLiteral(op))
	}
	var last uint
	for {
		var ids []uint
		if err := db.Raw(
			"SELECT id FROM note_blocks WHERE id > ? AND type IN ('gallery','calendar') ORDER BY id LIMIT ?",
			last, deleteStageChunk,
		).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		lo, hi := ids[0], ids[len(ids)-1]
		for _, stmt := range []string{s.gallery, s.calendar} {
			if err := db.Exec(stmt, lo, hi).Error; err != nil {
				return err
			}
		}
		last = hi
	}
}

// blockScrubSQL is the two UPDATE statements of one dialect, each taking the
// chunk's lowest and highest block id.
type blockScrubSQL struct{ gallery, calendar string }

const blockScrubWhitespace = " \t\f\v\r"

func sqliteBlockScrub(op string) blockScrubSQL {
	staged := "SELECT 1 FROM reduction_deleted_ids d WHERE d.op = " + op
	// match reports whether a JSON value of type t (json_each's type names) held in
	// v names a staged id. A text value is decoded as Sscanf("%d") does: leading
	// whitespace, then digits, then anything. The digits are compared as text with
	// the id, so an overlong digit run cannot overflow.
	match := func(t, v string) string {
		trimmed := "ltrim(ltrim(" + v + ", '" + blockScrubWhitespace + "'), '0')"
		return "EXISTS (" + staged + " AND (" +
			"(" + t + " = 'integer' AND " + v + " = d.resource_id) OR " +
			"(" + t + " = 'real' AND " + v + " >= 1 AND " + v + " < 9e18 AND CAST(" + v + " AS INTEGER) = d.resource_id) OR " +
			"(" + t + " = 'text' AND substr(" + trimmed + ", 1, length(d.resource_id)) = CAST(d.resource_id AS TEXT)" +
			" AND substr(" + trimmed + ", length(d.resource_id) + 1, 1) NOT BETWEEN '0' AND '9')))"
	}
	elem := func(j string) string {
		return "CASE " + j + ".type WHEN 'object' THEN " + j + ".value WHEN 'array' THEN " + j + ".value" +
			" WHEN 'null' THEN 'null' WHEN 'true' THEN 'true' WHEN 'false' THEN 'false'" +
			" ELSE json_quote(" + j + ".value) END"
	}
	const c = "CAST(content AS TEXT)"
	object := "CASE WHEN json_valid(" + c + ") THEN json_type(" + c + ") = 'object' ELSE 0 END"

	galleryHas := "json_type(" + c + ", '$.resourceIds') = 'array' AND EXISTS (SELECT 1 FROM json_each(" + c + ", '$.resourceIds') j WHERE " + match("j.type", "j.value") + ")"
	gallery := "UPDATE note_blocks SET content = json_set(" + c + ", '$.resourceIds', json((" +
		"SELECT '[' || COALESCE(group_concat(x.txt, ','), '') || ']' FROM (" +
		"SELECT " + elem("j") + " AS txt FROM json_each(" + c + ", '$.resourceIds') j WHERE NOT " + match("j.type", "j.value") + " ORDER BY j.key) x)))" +
		" WHERE id BETWEEN ? AND ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + galleryHas + ") ELSE 0 END"

	calType := "json_type(k.value, '$.source.resourceId')"
	calVal := "json_extract(k.value, '$.source.resourceId')"
	calMatch := "k.type = 'object' AND json_type(k.value, '$.source') = 'object' AND " + match(calType, calVal)
	calendarHas := "json_type(" + c + ", '$.calendars') = 'array' AND EXISTS (SELECT 1 FROM json_each(" + c + ", '$.calendars') k WHERE " + calMatch + ")"
	calendar := "UPDATE note_blocks SET content = json_set(" + c + ", '$.calendars', json((" +
		"SELECT '[' || COALESCE(group_concat(x.txt, ','), '') || ']' FROM (" +
		"SELECT CASE WHEN " + calMatch + " THEN json_remove(k.value, '$.source.resourceId') ELSE " + elem("k") + " END AS txt" +
		" FROM json_each(" + c + ", '$.calendars') k ORDER BY k.key) x)))" +
		" WHERE id BETWEEN ? AND ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + calendarHas + ") ELSE 0 END"
	return blockScrubSQL{gallery: gallery, calendar: calendar}
}

func postgresBlockScrub(op string) blockScrubSQL {
	staged := "SELECT 1 FROM reduction_deleted_ids d WHERE d.op = " + op
	ws := "'^[" + blockScrubWhitespace + "]*([0-9]+)'"
	// refid is the id a JSON value v names, or NULL.
	refid := func(v string) string {
		digits := "ltrim(substring(" + v + " #>> '{}' from " + ws + "), '0')"
		return "CASE jsonb_typeof(" + v + ")" +
			" WHEN 'number' THEN (CASE WHEN (" + v + ")::numeric >= 1 AND (" + v + ")::numeric < 1e18 THEN trunc((" + v + ")::numeric)::bigint END)" +
			" WHEN 'string' THEN (CASE WHEN length(" + digits + ") BETWEEN 1 AND 18 THEN (" + digits + ")::bigint END)" +
			" END"
	}
	match := func(v string) string {
		return "EXISTS (" + staged + " AND d.resource_id = " + refid(v) + ")"
	}
	object := "jsonb_typeof(content) = 'object'"
	galleryHas := "jsonb_typeof(content->'resourceIds') = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements(content->'resourceIds') e(v) WHERE " + match("e.v") + ")"
	gallery := "UPDATE note_blocks SET content = jsonb_set(content, '{resourceIds}', COALESCE((" +
		"SELECT jsonb_agg(e.v ORDER BY e.ord) FROM jsonb_array_elements(content->'resourceIds') WITH ORDINALITY e(v, ord) WHERE NOT " + match("e.v") + "), '[]'::jsonb))" +
		" WHERE id BETWEEN ? AND ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + galleryHas + ") ELSE false END"

	calMatch := "jsonb_typeof(e.v) = 'object' AND jsonb_typeof(e.v->'source') = 'object' AND " + match("(e.v->'source'->'resourceId')")
	calendarHas := "jsonb_typeof(content->'calendars') = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements(content->'calendars') e(v) WHERE " + calMatch + ")"
	calendar := "UPDATE note_blocks SET content = jsonb_set(content, '{calendars}', COALESCE((" +
		"SELECT jsonb_agg(CASE WHEN " + calMatch + " THEN jsonb_set(e.v, '{source}', (e.v->'source') - 'resourceId') ELSE e.v END ORDER BY e.ord)" +
		" FROM jsonb_array_elements(content->'calendars') WITH ORDINALITY e(v, ord)), '[]'::jsonb))" +
		" WHERE id BETWEEN ? AND ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + calendarHas + ") ELSE false END"
	return blockScrubSQL{gallery: gallery, calendar: calendar}
}

