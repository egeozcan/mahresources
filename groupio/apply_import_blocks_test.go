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

func TestApplyImport_ValidatesBlocks(t *testing.T) {
	block_types.RegisterBlockType(strictBlockType{})
	t.Cleanup(func() { block_types.UnregisterBlockType("test:strict") })

	srcCtx := createGUIDIsolatedContext(t, "blocks_src")
	root := mustCreateGroup(t, srcCtx, "Root", nil)
	note := mustCreateNote(t, srcCtx, "Blocky", &root.ID)
	rows := []models.NoteBlock{
		{Type: "text", Position: "a", Content: types.JSON(`{"text":"ok"}`), State: types.JSON(`{}`)},
		{Type: "test:strict", Position: "b", Content: types.JSON(`{"n":1}`), State: types.JSON(`{}`)},
		{Type: "test:strict", Position: "c", Content: types.JSON(`{"n":"<img>"}`), State: types.JSON(`{}`)},
		{Type: "test:strict", Position: "d", Content: types.JSON(`{"n":2}`), State: types.JSON(`{"x":1}`)},
		{Type: "plugin:missing:thing", Position: "e", Content: types.JSON(`{}`), State: types.JSON(`{}`)},
	}
	for i := range rows {
		rows[i].NoteID = note.ID
		if err := srcCtx.db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	var tarBuf bytes.Buffer
	if err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedNotes: true},
	}, &tarBuf, func(ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}

	dstCtx := createGUIDIsolatedContext(t, "blocks_dst")
	check := func(label, policy, jobID string) {
		dstCtx.fs.MkdirAll("_imports", 0755)
		tarPath := filepath.Join("_imports", jobID+".tar")
		afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644)
		plan, err := dstCtx.ParseImport(context.Background(), jobID, tarPath)
		if err != nil {
			t.Fatalf("%s parse: %v", label, err)
		}
		decisions := buildDefaultDecisions(plan)
		if policy != "" {
			decisions.GUIDCollisionPolicy = policy
		}
		result, err := dstCtx.ApplyImport(context.Background(), jobID, importPlanPath(jobID), decisions, noopSink{})
		if err != nil {
			t.Fatalf("%s apply: %v", label, err)
		}
		var got []models.NoteBlock
		dstCtx.db.Order("position").Find(&got)
		if len(got) != 2 || got[0].Type != "text" || got[1].Type != "test:strict" {
			t.Fatalf("%s: want exactly the two valid blocks, got %+v", label, got)
		}
		if !strings.Contains(string(got[1].Content), `"n":1`) {
			t.Errorf("%s: wrong content kept: %s", label, got[1].Content)
		}
		skipped := 0
		for _, w := range result.Warnings {
			if strings.Contains(w, "skipped block") {
				skipped++
			}
		}
		if skipped != 3 {
			t.Errorf("%s: want 3 skip warnings, got %d: %v", label, skipped, result.Warnings)
		}
	}
	check("create", "", "blocks-create")
	check("replace", "replace", "blocks-replace")
}
