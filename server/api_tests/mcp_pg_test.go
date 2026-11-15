//go:build postgres

package api_tests

import (
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func newMCPEnvPG(t *testing.T, authOn bool) *mcpEnv {
	t.Helper()
	return newMCPEnvOn(t, func(t *testing.T, mutate func(*application_context.MahresourcesConfig)) *TestContext {
		return setupPostgresTestEnvWith(t, mutate, nil)
	}, authOn)
}

func TestMCP_PG_ConfirmedDeleteAndScopedPreview(t *testing.T) {
	env := newMCPEnvPG(t, true)

	note := env.tc.CreateDummyNote("pg doomed")
	var seen []string
	cs := env.connect(t, clientOpts{bearer: env.bearer, answer: accept, seen: &seen})
	if res := callTool(t, cs, "delete", map[string]any{"entity": "note", "id": note.ID}); res.IsError {
		t.Fatalf("confirmed delete: %s", resultText(res))
	}
	if env.noteExists(t, note.ID) {
		t.Fatal("note survived a confirmed delete")
	}

	root := &models.Group{Name: "pg-mcp-root"}
	env.tc.DB.Create(root)
	outside := &models.Group{Name: "pg-mcp-outside"}
	env.tc.DB.Create(outside)
	inNote := &models.Note{Name: "pg-visible", OwnerId: &root.ID}
	env.tc.DB.Create(inNote)
	outNote := &models.Note{Name: "pg-secret", OwnerId: &outside.ID}
	env.tc.DB.Create(outNote)
	var scopedSeen []string
	scoped := env.connect(t, clientOpts{bearer: scopedUserBearer(t, env.tc, root.ID), answer: accept, seen: &scopedSeen})
	callTool(t, scoped, "call_operation", map[string]any{"operationId": "bulkDeleteNotes",
		"arguments": map[string]any{"ID": []any{inNote.ID, outNote.ID}}})
	if len(scopedSeen) != 1 || strings.Contains(scopedSeen[0], "pg-secret") || !strings.Contains(scopedSeen[0], "pg-visible") {
		t.Fatalf("scoped preview = %q", scopedSeen)
	}
	if !env.noteExists(t, outNote.ID) {
		t.Fatal("scoped bulk delete removed a note outside the scope")
	}
}

func TestMCP_PG_ReadsAndGuestWrites(t *testing.T) {
	env := newMCPEnvPG(t, true)
	note := env.tc.CreateDummyNote("pg read")
	cs := env.connect(t, clientOpts{bearer: env.bearer, protocol: mcpLegacy})
	res := callTool(t, cs, "get_note", map[string]any{"id": note.ID})
	if res.IsError || !strings.Contains(resultText(res), "pg read") {
		t.Fatalf("get_note on PG = %s", resultText(res))
	}
	refused := callTool(t, cs, "delete", map[string]any{"entity": "note", "id": note.ID})
	if !refused.IsError || !env.noteExists(t, note.ID) {
		t.Fatalf("legacy delete on PG = %v %s", refused.IsError, resultText(refused))
	}
}

// On PostgreSQL the reference says so and teaches the regex operators.
func TestMCP_PG_MRQLReferenceTeachesRegex(t *testing.T) {
	env := newMCPEnvPG(t, false)
	cs := env.connect(t, clientOpts{})
	text := resultText(callTool(t, cs, "mrql_reference", map[string]any{}))
	for _, want := range []string{"Database: PostgreSQL", "- Use field ~*", `name ~* "<regex>"`} {
		if !strings.Contains(text, want) {
			t.Errorf("PG mrql_reference is missing %q", want)
		}
	}
	env.tc.CreateDummyNote("IMG_2024 notes")
	res := callTool(t, cs, "mrql_query", map[string]any{"query": `type = note AND name ~* $re`, "params": map[string]any{"re": "^IMG_[0-9]{4}"}})
	if res.IsError || !strings.Contains(resultText(res), "IMG_2024 notes") {
		t.Fatalf("regex query = %v %s", res.IsError, clipForLog(resultText(res)))
	}
}
