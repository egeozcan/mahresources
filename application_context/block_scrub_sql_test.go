package application_context

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// refShapes are the ways a block can name a Resource id. Each is a shape
// toUint decides on its own terms, so the SQL scrub is differentially tested
// against it shape by shape (design D13).
func refShapes(rng *rand.Rand, id uint) any {
	switch rng.Intn(25) {
	case 0:
		return id
	case 1:
		return float64(id) + 0.5 // truncated
	case 2:
		return fmt.Sprintf("%d", id)
	case 3:
		return fmt.Sprintf(" %dabc", id) // leading space, trailing garbage
	case 4:
		return fmt.Sprintf("+%d", id) // rejected, preserved
	case 5:
		return fmt.Sprintf("\t%d", id)
	case 6:
		return fmt.Sprintf("00%d", id)
	case 7:
		return fmt.Sprintf("%d.9", id)
	case 8:
		return json.RawMessage(fmt.Sprintf("%de0", id)) // exponent form
	case 9:
		return json.RawMessage(fmt.Sprintf("%d.0", id))
	case 10:
		return -float64(id)
	case 11:
		return fmt.Sprintf("-%d", id)
	case 12:
		return fmt.Sprintf("%d 7", id)
	case 13:
		return fmt.Sprintf("7%d", id) // a different id that merely ends alike
	case 14:
		return fmt.Sprintf("%dx", id)
	case 15:
		// A float that float64 parsing rounds up to the next integer: Go reads id+1.
		return json.RawMessage(fmt.Sprintf("%d.9999999999999999", id))
	case 16:
		// Rounds up to 1 under float64 parsing.
		return json.RawMessage("0.99999999999999999")
	case 17:
		return json.RawMessage(fmt.Sprintf("%d.99999", id)) // truncated, not rounded
	case 18:
		return "\u00a0" + fmt.Sprintf("%d", id) // a Unicode space Sscanf skips
	case 19:
		return "\u3000\u2003" + fmt.Sprintf("%dz", id)
	case 20:
		return "\u200b" + fmt.Sprintf("%d", id) // zero-width space: not a space
	case 21:
		return "\n" + fmt.Sprintf("%d", id) // a newline is an error, not a space
	case 22:
		return "\r" + fmt.Sprintf("%d", id)
	case 23:
		return "\u0085" + fmt.Sprintf("%d", id)
	default:
		return float64(id)
	}
}

func junkRef(rng *rand.Rand) any {
	switch rng.Intn(8) {
	case 0:
		return nil
	case 1:
		return true
	case 2:
		return "abc"
	case 3:
		return map[string]any{"id": 3}
	case 4:
		return []any{3}
	case 5:
		return 0
	case 6:
		return ""
	default:
		return 1.5
	}
}

func randRef(rng *rand.Rand, universe uint) any {
	if rng.Intn(5) == 0 {
		return junkRef(rng)
	}
	return refShapes(rng, uint(rng.Intn(int(universe)))+1)
}

func randBlockContent(rng *rand.Rand, universe uint) (string, string) {
	typ := "gallery"
	if rng.Intn(2) == 0 {
		typ = "calendar"
	}
	doc := map[string]any{}
	if rng.Intn(4) != 0 {
		var arr []any
		for i, n := 0, rng.Intn(7); i < n; i++ {
			arr = append(arr, randRef(rng, universe))
		}
		if arr == nil {
			arr = []any{}
		}
		switch rng.Intn(12) {
		case 0:
			doc["resourceIds"] = "notanarray"
		case 1:
			doc["resourceIds"] = nil
		case 2:
			doc["resourceIds"] = map[string]any{"a": 1}
		default:
			doc["resourceIds"] = arr
		}
	}
	if rng.Intn(3) != 0 {
		var cals []any
		for i, n := 0, rng.Intn(5); i < n; i++ {
			switch rng.Intn(8) {
			case 0:
				cals = append(cals, "str")
			case 1:
				cals = append(cals, map[string]any{"name": "x"})
			case 2:
				cals = append(cals, map[string]any{"source": "notobject"})
			case 3:
				cals = append(cals, map[string]any{"source": map[string]any{"type": "ics"}})
			default:
				cals = append(cals, map[string]any{"name": fmt.Sprintf("c%d", i), "color": "<&>", "source": map[string]any{"type": "resource", "resourceId": randRef(rng, universe), "keep": []any{1, "a", nil, true}}})
			}
		}
		if cals == nil {
			cals = []any{}
		}
		if rng.Intn(10) == 0 {
			doc["calendars"] = "x"
		} else {
			doc["calendars"] = cals
		}
	}
	if rng.Intn(3) == 0 {
		doc["caption"] = "keep é <b>"
		doc["nested"] = map[string]any{"a": []any{1, 2.5, "x", nil, false}}
	}
	b, err := json.Marshal(doc)
	if err != nil {
		panic(err)
	}
	return typ, string(b)
}

// decodedEqual compares two JSON documents after decoding: the SQL scrub does
// not keep key order or HTML escaping, by design.
func decodedEqual(t *testing.T, want, got string, msg string) {
	t.Helper()
	var w, g any
	require.NoError(t, json.Unmarshal([]byte(want), &w), msg)
	require.NoError(t, json.Unmarshal([]byte(got), &g), msg+" (got "+got+")")
	require.True(t, reflect.DeepEqual(w, g), "%s\nwant %s\n got %s", msg, want, got)
}

// testSQLScrubMatchesGoScrub runs today's Go scrub and the SQL scrub over the
// same generated blocks and requires the documents equal after decoding.
func testSQLScrubMatchesGoScrub(t *testing.T, ctx *MahresourcesContext) {
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	rng := rand.New(rand.NewSource(7))
	const universe = 40
	deleted := map[uint]bool{}
	var ids []uint
	for len(ids) < 12 {
		id := uint(rng.Intn(universe)) + 1
		if !deleted[id] {
			deleted[id] = true
			ids = append(ids, id)
		}
	}

	type blk struct {
		id      uint
		content string
	}
	var blocks []blk
	// More blocks than one keyset chunk (500).
	for i := 0; i < 1300; i++ {
		typ, content := randBlockContent(rng, universe)
		b := &models.NoteBlock{NoteID: note.ID, Type: typ, Position: fmt.Sprintf("p%05d", i), Content: []byte(content)}
		require.NoError(t, ctx.db.Create(b).Error)
		blocks = append(blocks, blk{b.ID, content})
	}
	// A text block that merely looks like a gallery must never be touched.
	text := &models.NoteBlock{NoteID: note.ID, Type: "text", Position: "z", Content: []byte(`{"resourceIds":[` + fmt.Sprint(ids[0]) + `]}`)}
	require.NoError(t, ctx.db.Create(text).Error)

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, ids))
	require.NoError(t, scrubDeletedResourcesFromBlocks(ctx.db, op))

	changed := 0
	for _, b := range blocks {
		want, ch, err := scrubResourcesFromBlockContent(b.content, deleted)
		require.NoError(t, err)
		if ch {
			changed++
		}
		var got struct{ Content string }
		require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) AS content FROM note_blocks WHERE id = ?", b.id).Scan(&got).Error)
		decodedEqual(t, want, got.Content, fmt.Sprintf("block %d: %s", b.id, b.content))
	}
	require.Greater(t, changed, 100, "the generator must exercise the removal")

	var untouched struct{ Content string }
	require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) AS content FROM note_blocks WHERE id = ?", text.ID).Scan(&untouched).Error)
	decodedEqual(t, string(text.Content), untouched.Content, "text block")
}

func TestSQLScrubMatchesGoScrubSQLite(t *testing.T) {
	testSQLScrubMatchesGoScrub(t, newReductionSQLiteContext(t))
}

// An emptied array stays [], an array keeps its order, a non-object block is
// skipped instead of failing the delete, and unstaged ids leave a block alone.
func testSQLScrubEdges(t *testing.T, ctx *MahresourcesContext) {
	note := &models.Note{Name: "n"}
	require.NoError(t, ctx.db.Create(note).Error)
	n := 0
	mk := func(typ, content string) uint {
		n++
		b := &models.NoteBlock{NoteID: note.ID, Type: typ, Position: fmt.Sprintf("p%d", n), Content: []byte(content)}
		require.NoError(t, ctx.db.Create(b).Error)
		return b.ID
	}
	emptied := mk("gallery", `{"resourceIds":[5]}`)
	ordered := mk("gallery", `{"resourceIds":[9,5,1,5,2]}`)
	array := mk("gallery", `[5]`)
	untouched := mk("gallery", `{"resourceIds":[6]}`)

	op := newDeleteOp()
	require.NoError(t, stageDeletedIDs(ctx.db, op, []uint{5}))
	require.NoError(t, scrubDeletedResourcesFromBlocks(ctx.db, op))

	get := func(id uint) string {
		var got struct{ Content string }
		require.NoError(t, ctx.db.Raw("SELECT CAST(content AS TEXT) AS content FROM note_blocks WHERE id = ?", id).Scan(&got).Error)
		return got.Content
	}
	decodedEqual(t, `{"resourceIds":[]}`, get(emptied), "emptied")
	decodedEqual(t, `{"resourceIds":[9,1,2]}`, get(ordered), "ordered")
	decodedEqual(t, `[5]`, get(array), "non-object")
	decodedEqual(t, `{"resourceIds":[6]}`, get(untouched), "untouched")

	// Staged ids are the hook's to keep: the scrub never deletes them.
	var cnt int64
	require.NoError(t, ctx.db.Model(&models.ReductionDeletedID{}).Where("op = ?", op).Count(&cnt).Error)
	require.EqualValues(t, 1, cnt)
}

func TestSQLScrubEdgesSQLite(t *testing.T) {
	testSQLScrubEdges(t, newReductionSQLiteContext(t))
}
