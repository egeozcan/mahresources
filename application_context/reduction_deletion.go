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
// Blocks are visited by keyset (id > ?, 500 per page) in ascending id order, and a
// block that references a removed id is rewritten by ONE statement that updates its
// gallery and calendar references together. Every transaction therefore takes its
// block row locks in id order, which two concurrent deletes cannot turn into a cycle
// (a gallery pass followed by a calendar pass locked different blocks in each pass
// and did: 40P01).
//
// What counts as a reference is what toUint accepts (see block_ref_cleanup.go): an
// integer, a float taken as float64 and truncated, and a string read the way
// Sscanf("%d") reads it (leading Unicode space, then digits, then anything); "+12" is
// not one. A block whose content is not a JSON object, or whose field has another
// shape, is left alone.
//
// Go reads the last of a repeated object key and SQLite's JSON functions the first,
// so a SQLite block that repeats a key this scrub reads (resourceIds, calendars, a
// calendar's source or its source's resourceId) is scrubbed by the Go version, that
// one block at a time. Postgres's jsonb has dropped repeated keys on write.
func scrubDeletedResourcesFromBlocks(db *gorm.DB, op string) error {
	var s blockScrubSQL
	switch db.Dialector.Name() {
	case "postgres":
		s = postgresBlockScrub(opLiteral(op))
	default:
		s = sqliteBlockScrub(opLiteral(op))
	}
	var staged map[uint]bool
	var last uint
	for {
		var blocks []struct {
			ID  uint
			Dup int
		}
		if err := db.Raw(s.candidates, last, deleteStageChunk).Scan(&blocks).Error; err != nil {
			return err
		}
		if len(blocks) == 0 {
			return nil
		}
		for _, b := range blocks {
			if b.Dup == 0 {
				if err := db.Exec(s.update, b.ID).Error; err != nil {
					return err
				}
				continue
			}
			if staged == nil {
				var ids []uint
				if err := db.Raw("SELECT resource_id FROM reduction_deleted_ids WHERE op = ?", op).Scan(&ids).Error; err != nil {
					return err
				}
				staged = make(map[uint]bool, len(ids))
				for _, id := range ids {
					staged[id] = true
				}
			}
			if err := scrubBlockInGo(db, b.ID, staged); err != nil {
				return err
			}
		}
		last = blocks[len(blocks)-1].ID
	}
}

// scrubBlockInGo is the Go scrub of one block, for a document SQL cannot read the way
// Go does.
func scrubBlockInGo(db *gorm.DB, id uint, staged map[uint]bool) error {
	var row struct{ Content string }
	if err := db.Raw("SELECT CAST(content AS TEXT) AS content FROM note_blocks WHERE id = ?", id).Scan(&row).Error; err != nil {
		return err
	}
	updated, changed, err := scrubResourcesFromBlockContent(row.Content, staged)
	if err != nil {
		return fmt.Errorf("block %d: %w", id, err)
	}
	if !changed {
		return nil
	}
	return db.Exec("UPDATE note_blocks SET content = ? WHERE id = ?", updated, id).Error
}

// blockScrubSQL is one dialect's two statements. candidates takes (id > ?, LIMIT ?)
// and returns the next page of blocks to look at, each with dup set when only the Go
// scrub reads it right. update takes one block id and rewrites that block's gallery
// and calendar references in a single statement.
type blockScrubSQL struct{ candidates, update string }

// blockScrubWhitespace is what Sscanf("%d") skips before a number: the Unicode space
// set of fmt's isSpace except the newline (a newline is an error, which no digit
// follows, so leaving it out reads the same).
const blockScrubWhitespace = " \t\f\v\r\u0085                 　"

func sqliteBlockScrub(op string) blockScrubSQL {
	// match reports whether a JSON value of type t (json_each's type names) held in v
	// names a staged id. The id v names is computed first and then probed by the
	// primary key (op, resource_id), so a block costs one probe per reference, not a
	// scan of every staged id. A real is cast the way uint(float64) truncates it; a
	// text value is read as Sscanf("%d") does: whitespace, zeros, then the digit run,
	// which CAST ... AS INTEGER reads as its leading integer prefix. A run too long for
	// 18 digits saturates and names no id.
	match := func(t, v string) string {
		trimmed := "ltrim(ltrim(" + v + ", '" + blockScrubWhitespace + "'), '0')"
		rid := "CASE " + t +
			" WHEN 'integer' THEN " + v +
			" WHEN 'real' THEN CASE WHEN " + v + " >= 1 AND " + v + " < 9e18 THEN CAST(" + v + " AS INTEGER) END" +
			" WHEN 'text' THEN CASE WHEN substr(" + trimmed + ", 1, 1) BETWEEN '0' AND '9' THEN CAST(" + trimmed + " AS INTEGER) END END"
		return "EXISTS (SELECT 1 FROM reduction_deleted_ids d WHERE d.op = " + op + " AND d.resource_id = " + rid + ")"
	}
	elem := func(j string) string {
		return "CASE " + j + ".type WHEN 'object' THEN " + j + ".value WHEN 'array' THEN " + j + ".value" +
			" WHEN 'null' THEN 'null' WHEN 'true' THEN 'true' WHEN 'false' THEN 'false'" +
			" ELSE json_quote(" + j + ".value) END"
	}
	const c = "CAST(content AS TEXT)"
	object := "CASE WHEN json_valid(" + c + ") THEN json_type(" + c + ") = 'object' ELSE 0 END"

	galleryHas := "json_type(" + c + ", '$.resourceIds') = 'array' AND EXISTS (SELECT 1 FROM json_each(" + c + ", '$.resourceIds') j WHERE " + match("j.type", "j.value") + ")"
	galleryArr := "json((SELECT '[' || COALESCE(group_concat(x.txt, ','), '') || ']' FROM (" +
		"SELECT " + elem("j") + " AS txt FROM json_each(" + c + ", '$.resourceIds') j WHERE NOT " + match("j.type", "j.value") + " ORDER BY j.key) x))"

	// A calendar's source is walked member by member and the rewrite drops its
	// resourceId. A repeated key is the Go scrub's (dupHas), so exactly one resourceId
	// member exists wherever this runs.
	calMatch := "k.type = 'object' AND json_type(k.value, '$.source') = 'object' AND EXISTS (SELECT 1 FROM json_each(k.value, '$.source') s WHERE s.key = 'resourceId' AND " + match("s.type", "s.value") + ")"
	calRemove := "json_set(k.value, '$.source', json((SELECT '{' || COALESCE(group_concat(json_quote(s.key) || ':' || " + elem("s") + ", ','), '') || '}' FROM json_each(k.value, '$.source') s WHERE s.key <> 'resourceId')))"
	calendarHas := "json_type(" + c + ", '$.calendars') = 'array' AND EXISTS (SELECT 1 FROM json_each(" + c + ", '$.calendars') k WHERE " + calMatch + ")"
	calendarArr := "json((SELECT '[' || COALESCE(group_concat(x.txt, ','), '') || ']' FROM (" +
		"SELECT CASE WHEN " + calMatch + " THEN " + calRemove + " ELSE " + elem("k") + " END AS txt" +
		" FROM json_each(" + c + ", '$.calendars') k ORDER BY k.key) x))"

	// A repeated key the scrub reads: Go keeps the last, json_each walks them all.
	dupHas := "EXISTS (SELECT 1 FROM json_each(" + c + ") t WHERE t.key IN ('resourceIds', 'calendars') GROUP BY t.key HAVING count(*) > 1)" +
		" OR (json_type(" + c + ", '$.calendars') = 'array' AND EXISTS (SELECT 1 FROM json_each(" + c + ", '$.calendars') k WHERE k.type = 'object' AND (" +
		"EXISTS (SELECT 1 FROM json_each(k.value) m WHERE m.key = 'source' GROUP BY m.key HAVING count(*) > 1)" +
		" OR EXISTS (SELECT 1 FROM json_each(k.value, '$.source') s WHERE s.key = 'resourceId' GROUP BY s.key HAVING count(*) > 1))))"

	after := "CASE WHEN " + galleryHas + " THEN json_set(" + c + ", '$.resourceIds', " + galleryArr + ") ELSE " + c + " END"
	update := "UPDATE note_blocks SET content = CASE WHEN " + calendarHas +
		" THEN json_set(" + after + ", '$.calendars', " + calendarArr + ") ELSE " + after + " END" +
		" WHERE id = ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + galleryHas + ") OR (" + calendarHas + ") ELSE 0 END"
	candidates := "SELECT id, CASE WHEN " + dupHas + " THEN 1 ELSE 0 END AS dup FROM note_blocks" +
		" WHERE id > ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + dupHas + ") OR (" + galleryHas + ") OR (" + calendarHas + ") ELSE 0 END" +
		" ORDER BY id LIMIT ?"
	return blockScrubSQL{candidates: candidates, update: update}
}

func postgresBlockScrub(op string) blockScrubSQL {
	staged := "SELECT 1 FROM reduction_deleted_ids d WHERE d.op = " + op
	ws := "'^[" + blockScrubWhitespace + "]*([0-9]+)'"
	// refid is the id a JSON value v names, or NULL. A number is taken as float64 and
	// truncated, as uint(float64) does, so 11.9999999999999999 names 12; the numeric is
	// range-checked before the float cast so an out-of-range literal cannot raise.
	refid := func(v string) string {
		digits := "ltrim(substring(" + v + " #>> '{}' from " + ws + "), '0')"
		f := "(" + v + ")::numeric::float8"
		return "CASE jsonb_typeof(" + v + ")" +
			" WHEN 'number' THEN (CASE WHEN (" + v + ")::numeric > 0 AND (" + v + ")::numeric < 1e18 THEN (CASE WHEN " + f + " >= 1 THEN trunc(" + f + ")::bigint END) END)" +
			" WHEN 'string' THEN (CASE WHEN length(" + digits + ") BETWEEN 1 AND 18 THEN (" + digits + ")::bigint END)" +
			" END"
	}
	match := func(v string) string {
		return "EXISTS (" + staged + " AND d.resource_id = " + refid(v) + ")"
	}
	object := "jsonb_typeof(content) = 'object'"
	galleryHas := "jsonb_typeof(content->'resourceIds') = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements(content->'resourceIds') e(v) WHERE " + match("e.v") + ")"
	galleryArr := "COALESCE((SELECT jsonb_agg(e.v ORDER BY e.ord) FROM jsonb_array_elements(content->'resourceIds') WITH ORDINALITY e(v, ord) WHERE NOT " + match("e.v") + "), '[]'::jsonb)"

	calMatch := "jsonb_typeof(e.v) = 'object' AND jsonb_typeof(e.v->'source') = 'object' AND " + match("(e.v->'source'->'resourceId')")
	calendarHas := "jsonb_typeof(content->'calendars') = 'array' AND EXISTS (SELECT 1 FROM jsonb_array_elements(content->'calendars') e(v) WHERE " + calMatch + ")"
	calendarArr := "COALESCE((SELECT jsonb_agg(CASE WHEN " + calMatch + " THEN jsonb_set(e.v, '{source}', (e.v->'source') - 'resourceId') ELSE e.v END ORDER BY e.ord)" +
		" FROM jsonb_array_elements(content->'calendars') WITH ORDINALITY e(v, ord)), '[]'::jsonb)"

	after := "CASE WHEN " + galleryHas + " THEN jsonb_set(content, '{resourceIds}', " + galleryArr + ") ELSE content END"
	update := "UPDATE note_blocks SET content = CASE WHEN " + calendarHas +
		" THEN jsonb_set(" + after + ", '{calendars}', " + calendarArr + ") ELSE " + after + " END" +
		" WHERE id = ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + galleryHas + ") OR (" + calendarHas + ") ELSE false END"
	candidates := "SELECT id, 0 AS dup FROM note_blocks" +
		" WHERE id > ? AND type IN ('gallery','calendar') AND CASE WHEN " + object + " THEN (" + galleryHas + ") OR (" + calendarHas + ") ELSE false END" +
		" ORDER BY id LIMIT ?"
	return blockScrubSQL{candidates: candidates, update: update}
}

// reductionHookStep is how many Clusters one step of the deletion hook's walk
// locks and updates. A var so a test can force several steps from a small fixture.
var reductionHookStep = 500

// reductionDeletionHook is the one hook every Resource delete path calls in its
// deleting transaction (design D9), after the rows it deletes are locked and
// deleted and its ids staged under op. It marks the staged Resources gone in
// every Reduction Cluster that holds them, in the live generation (the plan), the
// computing one, or a migration's, and re-derives each affected Cluster's
// gone_members.
//
// One keyset walk over the affected Cluster ids, ascending, reductionHookStep per
// step: a bulk delete never restarts the order, so it cannot lock Cluster 20 before
// Cluster 10. Within a step, the Clusters are locked first (Postgres), then their
// matching member rows are updated by primary-key page, then gone_members is
// recomputed. It never deletes the staged ids and is idempotent. No Reduction row
// is touched and no version bumps.
func reductionDeletionHook(db *gorm.DB, op string) error {
	live := `(EXISTS (SELECT 1 FROM resource_reductions r WHERE r.id = m.reduction_id AND (r.plan_generation = m.generation OR r.compute_generation = m.generation))` +
		` OR EXISTS (SELECT 1 FROM reduction_migrations g WHERE g.reduction_id = m.reduction_id AND g.generation = m.generation))`
	var last uint
	for {
		var clusterIDs []uint
		if err := db.Raw(
			`SELECT DISTINCT m.cluster_id FROM reduction_members m JOIN reduction_deleted_ids d ON d.op = ? AND d.resource_id = m.resource_id`+
				` WHERE m.cluster_id > ? AND `+live+` ORDER BY m.cluster_id LIMIT ?`,
			op, last, reductionHookStep,
		).Scan(&clusterIDs).Error; err != nil {
			return err
		}
		if len(clusterIDs) == 0 {
			return nil
		}
		last = clusterIDs[len(clusterIDs)-1]

		if db.Dialector.Name() == "postgres" {
			var locked []uint
			if err := db.Raw("SELECT id FROM reduction_clusters WHERE id IN ? ORDER BY id FOR UPDATE", clusterIDs).Scan(&locked).Error; err != nil {
				return err
			}
		}
		var after uint
		for {
			var memberIDs []uint
			if err := db.Raw(
				`SELECT m.id FROM reduction_members m JOIN reduction_deleted_ids d ON d.op = ? AND d.resource_id = m.resource_id`+
					` WHERE m.cluster_id IN ? AND m.gone = ? AND m.id > ? ORDER BY m.id LIMIT ?`,
				op, clusterIDs, false, after, reductionRowChunk,
			).Scan(&memberIDs).Error; err != nil {
				return err
			}
			if len(memberIDs) == 0 {
				break
			}
			if err := db.Exec("UPDATE reduction_members SET gone = ? WHERE id IN ?", true, memberIDs).Error; err != nil {
				return err
			}
			after = memberIDs[len(memberIDs)-1]
		}
		if err := db.Exec(
			`UPDATE reduction_clusters SET gone_members = (SELECT COUNT(*) FROM reduction_members m WHERE m.cluster_id = reduction_clusters.id AND m.gone = ? AND m.ejected = ?) WHERE id IN ?`,
			true, false, clusterIDs,
		).Error; err != nil {
			return err
		}
	}
}
