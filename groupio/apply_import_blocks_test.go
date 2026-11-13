package groupio

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"mahresources/archive"
	"mahresources/models"
	"mahresources/models/block_types"
	"mahresources/models/types"
)

type strictBlockType struct{}

func (strictBlockType) Type() string { return "test:strict" }
func (strictBlockType) ValidateContent(c json.RawMessage) error {
	var v struct {
		N *int `json:"n"`
	}
	if err := json.Unmarshal(c, &v); err != nil || v.N == nil {
		return fmt.Errorf("n must be an integer")
	}
	return nil
}
func (strictBlockType) ValidateState(s json.RawMessage) error {
	var v map[string]any
	if err := json.Unmarshal(s, &v); err != nil || len(v) != 0 {
		return fmt.Errorf("state must be empty")
	}
	return nil
}
func (strictBlockType) DefaultContent() json.RawMessage { return json.RawMessage(`{"n":0}`) }
func (strictBlockType) DefaultState() json.RawMessage   { return json.RawMessage(`{}`) }

// blockFixture is a group Root with one note carrying rows, exported.
type blockFixture struct {
	ctx  *opCtx
	note *models.Note
	tar  []byte
}

func newBlockFixture(t *testing.T, name string, rows []models.NoteBlock) blockFixture {
	t.Helper()
	ctx := createGUIDIsolatedContext(t, name)
	root := mustCreateGroup(t, ctx, "Root", nil)
	note := mustCreateNote(t, ctx, "Blocky", &root.ID)
	for i := range rows {
		rows[i].NoteID = note.ID
		if err := ctx.db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	var tarBuf bytes.Buffer
	if err := ctx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedNotes: true},
	}, &tarBuf, func(ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return blockFixture{ctx: ctx, note: note, tar: tarBuf.Bytes()}
}

// importBlockArchive parses and applies tar into dst with policy ("" is the
// default merge).
func importBlockArchive(t *testing.T, dst *opCtx, tar []byte, policy, jobID string) (*ImportApplyResult, error) {
	t.Helper()
	dst.fs.MkdirAll("_imports", 0755)
	tarPath := filepath.Join("_imports", jobID+".tar")
	afero.WriteFile(dst.fs, tarPath, tar, 0644)
	plan, err := dst.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("%s parse: %v", jobID, err)
	}
	decisions := buildDefaultDecisions(plan)
	if policy != "" {
		decisions.GUIDCollisionPolicy = policy
	}
	return dst.ApplyImport(context.Background(), jobID, importPlanPath(jobID), decisions, noopSink{})
}

func blocksOf(t *testing.T, ctx *opCtx) []models.NoteBlock {
	t.Helper()
	var got []models.NoteBlock
	if err := ctx.db.Order("position").Find(&got).Error; err != nil {
		t.Fatal(err)
	}
	return got
}

// A block whose state fails validation keeps its content and gets the type's
// default state: state is UI state, and the default is always valid.
func TestApplyImport_ResetsAnInvalidBlockState(t *testing.T) {
	block_types.RegisterBlockType(strictBlockType{})
	t.Cleanup(func() { block_types.UnregisterBlockType("test:strict") })

	f := newBlockFixture(t, "blocks_state_src", []models.NoteBlock{
		{Type: "text", Position: "a", Content: types.JSON(`{"text":"ok"}`), State: types.JSON(`{}`)},
		{Type: "test:strict", Position: "b", Content: types.JSON(`{"n":2}`), State: types.JSON(`{"x":1}`)},
	})

	check := func(label string, dst *opCtx, policy string) {
		result, err := importBlockArchive(t, dst, f.tar, policy, label)
		if err != nil {
			t.Fatalf("%s apply: %v", label, err)
		}
		got := blocksOf(t, dst)
		if len(got) != 2 || got[1].Type != "test:strict" {
			t.Fatalf("%s: want both blocks, got %+v", label, got)
		}
		if !strings.Contains(string(got[1].Content), `"n":2`) || string(got[1].State) != `{}` {
			t.Errorf("%s: want content kept and state reset, got content %s state %s", label, got[1].Content, got[1].State)
		}
		reset := 0
		for _, w := range result.Warnings {
			if strings.Contains(w, "reset the state of block") {
				reset++
			}
		}
		if reset != 1 {
			t.Errorf("%s: want 1 state reset warning, got %v", label, result.Warnings)
		}
	}
	check("blocks-state-create", createGUIDIsolatedContext(t, "blocks_state_dst"), "")
	check("blocks-state-replace", f.ctx, "replace")
}

// A block the destination cannot store as it is (an unknown type, or content
// its type rejects) fails the import rather than being dropped. Dropping it
// silently loses it: on a replace the note's existing blocks are deleted
// first, so a self-restore would destroy them for good.
func TestApplyImport_RefusesABlockItCannotStore(t *testing.T) {
	block_types.RegisterBlockType(strictBlockType{})
	t.Cleanup(func() { block_types.UnregisterBlockType("test:strict") })

	cases := map[string]struct {
		bad  models.NoteBlock
		want string
	}{
		"invalid content": {models.NoteBlock{Type: "test:strict", Position: "b", Content: types.JSON(`{"n":"<img>"}`), State: types.JSON(`{}`)}, "invalid content"},
		"unknown type":    {models.NoteBlock{Type: "plugin:missing:thing", Position: "b", Content: types.JSON(`{}`), State: types.JSON(`{}`)}, "unknown block type"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			src := strings.ReplaceAll(name, " ", "_")
			f := newBlockFixture(t, "blocks_refuse_"+src, []models.NoteBlock{
				{Type: "text", Position: "a", Content: types.JSON(`{"text":"keep"}`), State: types.JSON(`{}`)},
				c.bad,
			})
			before := blocksOf(t, f.ctx)

			// Replace into the instance the archive came from: nothing is lost.
			if _, err := importBlockArchive(t, f.ctx, f.tar, "replace", "refuse-replace-"+src); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("replace: want an error mentioning %q, got %v", c.want, err)
			}
			after := blocksOf(t, f.ctx)
			if len(after) != len(before) {
				t.Fatalf("replace lost blocks: before %d, after %d (%+v)", len(before), len(after), after)
			}
			for i := range before {
				if after[i].ID != before[i].ID || string(after[i].Content) != string(before[i].Content) {
					t.Fatalf("replace changed block %d: before %+v, after %+v", i, before[i], after[i])
				}
			}

			// Create into a fresh instance: refused, not silently short a block.
			dst := createGUIDIsolatedContext(t, "blocks_refuse_dst_"+src)
			if _, err := importBlockArchive(t, dst, f.tar, "", "refuse-create-"+src); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("create: want an error mentioning %q, got %v", c.want, err)
			}
			if got := blocksOf(t, dst); len(got) != 0 {
				t.Fatalf("create stored blocks of a refused note: %+v", got)
			}
		})
	}
}

// The merge policy writes no blocks, so an archive block the destination
// could not store does not stop a merge into an existing note.
func TestApplyImport_MergeIgnoresBlocksItWouldNotWrite(t *testing.T) {
	f := newBlockFixture(t, "blocks_merge_src", []models.NoteBlock{
		{Type: "plugin:missing:thing", Position: "a", Content: types.JSON(`{}`), State: types.JSON(`{}`)},
	})
	if _, err := importBlockArchive(t, f.ctx, f.tar, "merge", "blocks-merge"); err != nil {
		t.Fatalf("merge apply: %v", err)
	}
	if got := blocksOf(t, f.ctx); len(got) != 1 {
		t.Fatalf("merge changed blocks: %+v", got)
	}
}

// The finding's case: calendar blocks saved before the http(s)-only URL rule
// and the month/agenda view rule. A self-restore with the replace policy must
// not destroy them.
func TestApplyImport_ReplaceKeepsLegacyCalendarBlocks(t *testing.T) {
	t.Run("webcal url", func(t *testing.T) {
		f := newBlockFixture(t, "blocks_webcal", []models.NoteBlock{
			{Type: "calendar", Position: "a", Content: types.JSON(`{"calendars":[{"id":"c1","name":"Work","source":{"type":"url","url":"webcal://example.com/x.ics"}}]}`), State: types.JSON(`{"view":"month"}`)},
			{Type: "text", Position: "b", Content: types.JSON(`{"text":"keep"}`), State: types.JSON(`{}`)},
		})
		if _, err := importBlockArchive(t, f.ctx, f.tar, "replace", "webcal-replace"); err == nil {
			t.Fatal("replace stored a calendar block its type rejects")
		}
		got := blocksOf(t, f.ctx)
		if len(got) != 2 || !strings.Contains(string(got[0].Content), "webcal://") {
			t.Fatalf("replace lost the legacy calendar block: %+v", got)
		}
	})
	t.Run("week view", func(t *testing.T) {
		f := newBlockFixture(t, "blocks_week", []models.NoteBlock{
			{Type: "calendar", Position: "a", Content: types.JSON(`{"calendars":[{"id":"c1","name":"Work","source":{"type":"url","url":"https://example.com/x.ics"}}]}`), State: types.JSON(`{"view":"week"}`)},
			{Type: "text", Position: "b", Content: types.JSON(`{"text":"keep"}`), State: types.JSON(`{}`)},
		})
		if _, err := importBlockArchive(t, f.ctx, f.tar, "replace", "week-replace"); err != nil {
			t.Fatalf("replace apply: %v", err)
		}
		got := blocksOf(t, f.ctx)
		if len(got) != 2 || !strings.Contains(string(got[0].Content), "https://example.com/x.ics") {
			t.Fatalf("replace lost the calendar block: %+v", got)
		}
		if string(got[0].State) != string(block_types.GetBlockType("calendar").DefaultState()) {
			t.Errorf("calendar state = %s, want the default", got[0].State)
		}
	})
}

// A plugin's block type is unregistered while the plugin is disabled. A
// self-restore with the replace policy in that window must not delete the
// plugin's blocks.
func TestApplyImport_ReplaceKeepsBlocksOfADisabledPlugin(t *testing.T) {
	block_types.RegisterBlockType(strictBlockType{})
	f := newBlockFixture(t, "blocks_disabled", []models.NoteBlock{
		{Type: "test:strict", Position: "a", Content: types.JSON(`{"n":1}`), State: types.JSON(`{}`)},
	})
	block_types.UnregisterBlockType("test:strict")

	if _, err := importBlockArchive(t, f.ctx, f.tar, "replace", "disabled-replace"); err == nil {
		t.Fatal("replace stored a block of an unregistered type")
	}
	if got := blocksOf(t, f.ctx); len(got) != 1 || got[0].Type != "test:strict" {
		t.Fatalf("replace deleted the disabled plugin's block: %+v", got)
	}
}
