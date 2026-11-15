package api_tests

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
)

// The MRQL tools give a model what it needs to write a query that runs here:
// the language, this deployment's facts, and the caller's local names.
func TestMCP_MRQLReferenceStatesThisServer(t *testing.T) {
	env := newMCPEnv(t, false)
	cs := env.connect(t, clientOpts{})

	res := callTool(t, cs, "mrql_reference", map[string]any{})
	text := resultText(res)
	if res.IsError {
		t.Fatalf("mrql_reference: %s", text)
	}
	for _, want := range []string{
		"# MRQL on this server",
		"Database: SQLite",
		"runs with LIMIT",
		"# Rules",
		"category and noteType require numeric IDs",
		"## Operators",
		"## Fields (by entity type)",
		"## Parameters -- `$name`",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("mrql_reference is missing %q", want)
		}
	}
	for _, gone := range []string{"## CLI Reference", "## CLI Invocation", "## List-Page Filter Bar", "GENERATED FILE"} {
		if strings.Contains(text, gone) {
			t.Errorf("mrql_reference still carries %q", gone)
		}
	}
	// Regex is taught only where it runs.
	if strings.Contains(text, "- Use field ~*") {
		t.Errorf("mrql_reference teaches the regex rule on SQLite")
	}

	recipes := callTool(t, cs, "mrql_reference", map[string]any{"topic": "recipes"})
	if recipes.IsError || !strings.Contains(resultText(recipes), "# MRQL Recipes") {
		t.Fatalf("recipes = %v %s", recipes.IsError, clipForLog(resultText(recipes)))
	}
	if bad := callTool(t, cs, "mrql_reference", map[string]any{"topic": "nope"}); !bad.IsError {
		t.Fatalf("unknown topic accepted")
	}
}

func TestMCP_MRQLVocabularyListsLocalNames(t *testing.T) {
	env := newMCPEnv(t, false)
	nt := env.tc.CreateNoteType(t, "Meeting Minutes")
	note := &models.Note{Name: "standup", Meta: types.JSON(`{"attendees": 4}`), NoteTypeId: &nt.ID}
	if err := env.tc.DB.Create(note).Error; err != nil {
		t.Fatal(err)
	}
	tag := &models.Tag{Name: "vocab-tag"}
	if err := env.tc.DB.Create(tag).Error; err != nil {
		t.Fatal(err)
	}
	cs := env.connect(t, clientOpts{})

	saved := callTool(t, cs, "save_mrql_query", map[string]any{"name": "minutes", "query": `type = note AND noteType = $nt`})
	if saved.IsError {
		t.Fatalf("save_mrql_query: %s", resultText(saved))
	}

	res := callTool(t, cs, "mrql_vocabulary", map[string]any{})
	if res.IsError {
		t.Fatalf("mrql_vocabulary: %s", resultText(res))
	}
	var v struct {
		NoteTypes    []map[string]any  `json:"noteTypes"`
		Tags         []map[string]any  `json:"tags"`
		NoteMetaKeys []string          `json:"noteMetaKeys"`
		SavedQueries []map[string]any  `json:"savedQueries"`
		Notes        map[string]string `json:"notes"`
	}
	if err := json.Unmarshal([]byte(resultText(res)), &v); err != nil {
		t.Fatalf("decode: %v\n%s", err, resultText(res))
	}
	hasRow := func(rows []map[string]any, id uint, name string) bool {
		for _, r := range rows {
			if fmt.Sprint(r["id"]) == fmt.Sprint(id) && r["name"] == name {
				return true
			}
		}
		return false
	}
	if !hasRow(v.NoteTypes, nt.ID, "Meeting Minutes") {
		t.Errorf("noteTypes = %v", v.NoteTypes)
	}
	if !hasRow(v.Tags, tag.ID, "vocab-tag") {
		t.Errorf("tags = %v", v.Tags)
	}
	found := false
	for _, k := range v.NoteMetaKeys {
		found = found || k == "attendees"
	}
	if !found {
		t.Errorf("noteMetaKeys = %v", v.NoteMetaKeys)
	}
	if len(v.SavedQueries) != 1 || v.SavedQueries[0]["query"] != `type = note AND noteType = $nt` {
		t.Errorf("savedQueries = %v", v.SavedQueries)
	}
	if !strings.Contains(v.Notes["noteTypes"], "noteType = <id>") {
		t.Errorf("notes = %v", v.Notes)
	}

	only := callTool(t, cs, "mrql_vocabulary", map[string]any{"sections": []any{"tags"}, "tagName": "vocab"})
	if only.IsError || strings.Contains(resultText(only), "noteTypes") || !strings.Contains(resultText(only), "vocab-tag") {
		t.Fatalf("sections/tagName = %v %s", only.IsError, resultText(only))
	}
	if bad := callTool(t, cs, "mrql_vocabulary", map[string]any{"sections": []any{"users"}}); !bad.IsError {
		t.Fatalf("unknown section accepted")
	}

	// The saved query runs by name with its placeholder bound.
	run := callTool(t, cs, "run_saved_query", map[string]any{"name": "minutes", "params": map[string]any{"nt": nt.ID}})
	if run.IsError || !strings.Contains(resultText(run), "standup") {
		t.Fatalf("run_saved_query = %v %s", run.IsError, clipForLog(resultText(run)))
	}
	list := callTool(t, cs, "list_saved_queries", map[string]any{})
	if list.IsError || !strings.Contains(resultText(list), `"minutes"`) {
		t.Fatalf("list_saved_queries = %v %s", list.IsError, resultText(list))
	}
}

func TestMCP_MRQLQueryParamsObjectAndValidate(t *testing.T) {
	env := newMCPEnv(t, false)
	env.tc.CreateDummyNote("quarterly review")
	cs := env.connect(t, clientOpts{})

	res := callTool(t, cs, "mrql_query", map[string]any{"query": `type = note AND name ~ $needle`, "params": map[string]any{"needle": "quarterly"}})
	if res.IsError || !strings.Contains(resultText(res), "quarterly review") {
		t.Fatalf("params object = %v %s", res.IsError, clipForLog(resultText(res)))
	}

	ok := callTool(t, cs, "mrql_validate", map[string]any{"query": `type = note AND name ~ $needle`})
	if ok.IsError || !strings.Contains(resultText(ok), `"valid":true`) || !strings.Contains(resultText(ok), "needle") {
		t.Fatalf("validate valid = %v %s", ok.IsError, resultText(ok))
	}
	bad := callTool(t, cs, "mrql_validate", map[string]any{"query": `type = note AND name LIKE "x"`})
	if strings.Contains(resultText(bad), `"valid":true`) {
		t.Fatalf("validate accepted LIKE: %s", resultText(bad))
	}
}

// A guest confined to a subtree gets the vocabulary through the same scoped
// REST reads, and a section it may not read is reported, not fatal.
func TestMCP_MRQLVocabularyAsScopedGuest(t *testing.T) {
	env := newMCPEnv(t, true)
	cs := env.connect(t, clientOpts{bearer: guestBearer(t, env)})
	res := callTool(t, cs, "mrql_vocabulary", map[string]any{})
	if res.IsError {
		t.Fatalf("guest mrql_vocabulary: %s", resultText(res))
	}
	if !strings.Contains(resultText(res), `"notes"`) {
		t.Fatalf("guest vocabulary = %s", resultText(res))
	}
}

func guestBearer(t *testing.T, env *mcpEnv) string {
	t.Helper()
	scope := env.tc.CreateDummyGroup("vocab scope")
	u, err := env.tc.AppCtx.CreateUser(&application_context.UserInput{Username: "vocabguest", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &scope.ID})
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := env.tc.AppCtx.CreateApiToken(u.ID, "g", nil)
	if err != nil {
		t.Fatal(err)
	}
	return "Bearer " + raw
}
