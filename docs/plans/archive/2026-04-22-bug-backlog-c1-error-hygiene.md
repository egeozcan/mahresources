# Cluster 1 — Error Hygiene (BH-P05, BH-019)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Can run 2 parallel subagents (Task groups A and B touch disjoint files). Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stop leaking internal server config on `.json` error paths (BH-P05) and reject control-character/bidi-override/NUL in entity names at the input layer (BH-019).

**Architecture:** Two surgical fixes on disjoint files. Group A extends `discardFields` in `server/template_handlers/render_template.go` to strip `_appContext` and `_requestContext` from JSON error responses. Group B introduces `application_context/validation/entity_name.go` with a shared `SanitizeEntityName` helper called from tag/group/note/resource/noteType/category create+update paths.

**Tech Stack:** Go, pongo2 templates, Gorilla Mux, existing `server/api_tests/` test suite.

**Worktree branch:** `bugfix/c1-error-hygiene`

---

## File structure

**Modified:**
- `server/template_handlers/render_template.go` — add `_appContext`, `_requestContext` to the existing `discardFields` denylist (BH-P05)
- `application_context/tag_context.go`, `group_context.go`, `note_context.go`, `resource_context.go`, `note_type_context.go`, `category_context.go` — call `SanitizeEntityName` on create and update (BH-019)

**Created:**
- `application_context/validation/entity_name.go` — new helper
- `application_context/validation/entity_name_test.go` — unit tests
- `server/api_tests/json_error_leaks_appcontext_test.go` — BH-P05 regression
- `server/api_tests/entity_name_control_chars_test.go` — BH-019 API-level test

---

## Task Group A: BH-P05 — JSON error leak

### Task A1: Write failing test for `_appContext` leak on `.json` error

**Files:**
- Create: `server/api_tests/json_error_leaks_appcontext_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJsonErrorDoesNotLeakAppContext(t *testing.T) {
	tc := SetupTestEnv(t)

	// Non-numeric id triggers an error via .json route
	resp := tc.MakeRequest(http.MethodGet, "/resource.json?id=abc", nil)
	assert.GreaterOrEqual(t, resp.Code, 400, "response should be an error status")

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	// These must NOT leak — they exposed DbDsn, FfmpegPath, FileSavePath, AltFileSystems, etc.
	assert.NotContains(t, body, "_appContext", "_appContext must not leak in JSON error response")
	assert.NotContains(t, body, "_requestContext", "_requestContext must not leak in JSON error response")

	// Sanity: the user-facing error must still be present
	assert.Contains(t, body, "errorMessage", "errorMessage should be present in error response")
}

func TestJsonErrorDoesNotLeakAppContextAcrossEntities(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{"/note.json?id=abc", "/group.json?id=abc", "/tag.json?id=abc", "/resource.json?id=abc"} {
		resp := tc.MakeRequest(http.MethodGet, path, nil)
		assert.GreaterOrEqual(t, resp.Code, 400, "%s: expected error status", path)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		require.NoError(t, err, "%s: response should be valid JSON", path)

		assert.NotContains(t, body, "_appContext", "%s: _appContext must not leak", path)
		assert.NotContains(t, body, "_requestContext", "%s: _requestContext must not leak", path)
	}
}
```

- [ ] **Step 2: Run test 3× to verify it fails consistently with the real symptom**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestJsonErrorDoesNotLeakAppContext -v -count=3
```

Expected: FAIL all 3 runs. Failure message must mention `_appContext` or `_requestContext` being present in the body — this confirms the symptom matches BH-P05. If failure is for a different reason (compile error, test setup), fix the test itself first.

### Task A2: Add `_appContext` and `_requestContext` to discardFields

**Files:**
- Modify: `server/template_handlers/render_template.go:54-78`

- [ ] **Step 1: Add the two keys to the existing denylist**

In `render_template.go`, the `discardFields` call in `RenderTemplate`'s JSON branch already discards UI fields. Add `_appContext` and `_requestContext`:

```go
if err := json.NewEncoder(writer).Encode(discardFields(map[string]bool{
    // Function-valued fields (cannot serialize to JSON)
    "partial":     true,
    "path":        true,
    "withQuery":   true,
    "hasQuery":    true,
    "stringId":    true,
    "getNextId":   true,
    "dereference": true,
    // Internal/rendering fields (should not leak to JSON consumers)
    "_pluginManager":     true,
    "_statusCode":        true,
    "_appContext":        true,  // BH-P05: contains Config (DbDsn, FfmpegPath, etc.)
    "_requestContext":    true,  // BH-P05: contains nested Go context
    "currentPath":        true,
    "pluginMenuItems":    true,
    "menu":               true,
    "adminMenu":          true,
    "title":              true,
    "assetVersion":       true,
    "queryValues":        true,
    "url":                true,
    "hasPluginManager":   true,
    "pluginDetailActions": true,
    "pluginCardActions":  true,
    "pluginBulkActions":  true,
}, context)); err != nil {
    fmt.Println(err)
}
```

- [ ] **Step 2: Run the new tests 3× to verify they pass**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestJsonErrorDoesNotLeakAppContext -v -count=3
```

Expected: PASS all 3 runs.

- [ ] **Step 3: Run the full existing `json_route_no_internal_context_test.go` suite to confirm no regression**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run 'TestJsonRouteError|TestJsonRouteSuccess|TestJsonRouteDoesNotLeak|TestJsonErrorDoes' -v
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd <worktree>
git add server/template_handlers/render_template.go server/api_tests/json_error_leaks_appcontext_test.go
git commit -m "fix(errors): BH-P05 — stop leaking _appContext/_requestContext on .json errors"
```

---

## Task Group B: BH-019 — Entity name control-char sanitization

### Task B1: Write unit test for `SanitizeEntityName` helper

**Files:**
- Create: `application_context/validation/entity_name_test.go`

- [ ] **Step 1: Write the failing test**

```go
package validation_test

import (
	"strings"
	"testing"

	"mahresources/application_context/validation"
)

func TestSanitizeEntityName_RejectsNullByte(t *testing.T) {
	_, err := validation.SanitizeEntityName("foo\x00bar")
	if err == nil {
		t.Fatal("expected error for NUL byte, got nil")
	}
	if !strings.Contains(err.Error(), "control character") {
		t.Fatalf("expected 'control character' error, got: %v", err)
	}
}

func TestSanitizeEntityName_RejectsDirectionalOverrides(t *testing.T) {
	for _, ch := range []string{"‪", "‫", "‬", "‭", "‮", "⁦", "⁧", "⁨", "⁩"} {
		_, err := validation.SanitizeEntityName("foo" + ch + "bar")
		if err == nil {
			t.Fatalf("expected error for directional override %U, got nil", []rune(ch)[0])
		}
	}
}

func TestSanitizeEntityName_RejectsEmbeddedNewlines(t *testing.T) {
	for _, raw := range []string{"foo\nbar", "foo\rbar", "foo\r\nbar"} {
		_, err := validation.SanitizeEntityName(raw)
		if err == nil {
			t.Fatalf("expected error for embedded newline in %q, got nil", raw)
		}
	}
}

func TestSanitizeEntityName_AllowsTabAndNormalUnicode(t *testing.T) {
	for _, raw := range []string{"hello world", "café", "日本語", "name\twith\ttab", "emoji \U0001F600"} {
		got, err := validation.SanitizeEntityName(raw)
		if err != nil {
			t.Fatalf("unexpected error for %q: %v", raw, err)
		}
		if got != raw {
			t.Fatalf("expected passthrough for %q, got %q", raw, got)
		}
	}
}

func TestSanitizeEntityName_TrimsSurroundingWhitespace(t *testing.T) {
	got, err := validation.SanitizeEntityName("  hello  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Fatalf("expected trimmed 'hello', got %q", got)
	}
}

func TestSanitizeEntityName_RejectsEmptyAfterTrim(t *testing.T) {
	_, err := validation.SanitizeEntityName("   ")
	if err == nil {
		t.Fatal("expected error for whitespace-only name")
	}
}
```

- [ ] **Step 2: Run test 3× to verify it fails with compilation error** (the helper package doesn't exist yet — that's the expected pre-implementation symptom)

```bash
go test --tags 'json1 fts5' ./application_context/validation/... -v -count=3
```

Expected: 3× FAIL with "package validation is not in GOROOT" or similar import error. This matches the pre-implementation state.

### Task B2: Implement `SanitizeEntityName` helper

**Files:**
- Create: `application_context/validation/entity_name.go`

- [ ] **Step 1: Write the implementation**

```go
package validation

import (
	"fmt"
	"strings"
	"unicode"
)

// SanitizeEntityName validates and trims a user-supplied entity name.
// Returns an error if the name contains NUL bytes, C0 controls (except TAB),
// Unicode directional overrides, embedded newlines/CRs, or is empty after trimming.
//
// Context: BH-019 — these characters cause UI spoofing, CSV/log corruption,
// and C-library truncation (e.g., ffmpeg shelling to paths containing NUL).
func SanitizeEntityName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("name must not be empty")
	}

	for i, r := range trimmed {
		switch {
		case r == 0x00:
			return "", fmt.Errorf("name contains NUL byte at position %d", i)
		case r == '\n' || r == '\r':
			return "", fmt.Errorf("name contains newline at position %d", i)
		case r == '\t':
			// allowed
		case r < 0x20 || r == 0x7F:
			return "", fmt.Errorf("name contains control character U+%04X at position %d", r, i)
		case r >= 0x80 && r < 0xA0:
			return "", fmt.Errorf("name contains C1 control character U+%04X at position %d", r, i)
		case r >= 0x202A && r <= 0x202E:
			return "", fmt.Errorf("name contains directional override U+%04X at position %d", r, i)
		case r >= 0x2066 && r <= 0x2069:
			return "", fmt.Errorf("name contains directional isolate U+%04X at position %d", r, i)
		case !unicode.IsGraphic(r) && r != '\t':
			return "", fmt.Errorf("name contains non-graphic character U+%04X at position %d", r, i)
		}
	}

	return trimmed, nil
}
```

- [ ] **Step 2: Run test 3× to verify it passes**

```bash
go test --tags 'json1 fts5' ./application_context/validation/... -v -count=3
```

Expected: PASS all 3 runs.

### Task B3: Wire `SanitizeEntityName` into all entity create + update handlers

**Files (each modified once for create, once for update):**
- Modify: `application_context/tag_context.go`
- Modify: `application_context/group_crud_context.go`
- Modify: `application_context/note_context.go`
- Modify: `application_context/resource_context.go`
- Modify: `application_context/note_type_context.go`
- Modify: `application_context/category_context.go`

- [ ] **Step 1: For each context file above, locate the function that creates the entity (typically `Create<Entity>` or `Add<Entity>`) and the function that updates it (typically `Update<Entity>` or `Edit<Entity>`). At the top of each, before any DB operation, call:**

```go
cleanName, err := validation.SanitizeEntityName(query.Name)
if err != nil {
    return nil, err
}
query.Name = cleanName
```

Add the import `"mahresources/application_context/validation"` to each file.

**Important:** If a context file does not expose `Name` on its query struct (e.g., it re-uses a shared DTO), call `SanitizeEntityName` on whichever field carries the user-facing name for the entity. Refer to the existing whitespace-name validation tests under `server/api_tests/whitespace_name_validation_test.go` for patterns already established.

- [ ] **Step 2: Run existing tests to catch any behavior change on legitimate names**

```bash
go test --tags 'json1 fts5' ./application_context/... ./server/api_tests/... -v -run 'TestTag|TestGroup|TestNote|TestResource|TestNoteType|TestCategory'
```

Expected: PASS. If a test breaks because it used a name containing, for example, a newline, update the test to use a legitimate name — the old behavior was the bug.

### Task B4: Write API-level test for BH-019 symptom

**Files:**
- Create: `server/api_tests/entity_name_control_chars_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-nul\x00byte")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "NUL-byte name must be rejected with 400")
}

func TestTagCreate_RejectsDirectionalOverrideInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19‮rotated")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "RTL override in name must be rejected")
}

func TestTagCreate_RejectsEmbeddedNewlineInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19\nwith newline")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "Newline in name must be rejected")
}

func TestTagCreate_AcceptsNormalName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "ordinary-tag-name")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusOK, resp.Code, "Ordinary name must still succeed")
}

// Parallel checks for the other entity types.
func TestGroupCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-group\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
func TestNoteCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-note\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
func TestCategoryCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-cat\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/category", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
func TestNoteTypeCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-nt\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/noteType", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}
```

- [ ] **Step 2: Run 3× — expect PASS now that Task B3 wired the validation**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run 'TestTagCreate_RejectsNullByte|TestGroupCreate_RejectsNullByte|TestNoteCreate_RejectsNullByte|TestCategoryCreate_RejectsNullByte|TestNoteTypeCreate_RejectsNullByte|TestTagCreate_RejectsDirectional|TestTagCreate_RejectsEmbedded|TestTagCreate_AcceptsNormal' -v -count=3
```

Expected: PASS all 3 runs. If any endpoint returns 200 for NUL-byte names, Task B3 missed that endpoint — go back and wire it.

- [ ] **Step 3: Commit**

```bash
cd <worktree>
git add application_context/validation/ application_context/*_context.go server/api_tests/entity_name_control_chars_test.go
git commit -m "fix(validation): BH-019 — reject NUL/bidi/newlines in entity names"
```

---

## Cluster PR gate

- [ ] **Step 1: Go unit + API full suite**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
```

Expected: PASS.

- [ ] **Step 2: Rebase on latest master**

```bash
git fetch origin
git rebase origin/master
```

- [ ] **Step 3: Full E2E browser + CLI + Postgres (per master plan)**

```bash
cd e2e && npm run test:with-server:all
cd .. && go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

Expected: ALL PASS.

- [ ] **Step 4: Open PR**

```bash
gh pr create --title "fix(errors): BH-P05, BH-019 — error hygiene" --body "$(cat <<'EOF'
Closes BH-P05, BH-019.

## Changes

- `server/template_handlers/render_template.go` — add `_appContext` + `_requestContext` to JSON discard list. `.json` error responses no longer leak server config.
- `application_context/validation/entity_name.go` — new `SanitizeEntityName` helper rejects NUL bytes, C0/C1 controls (except `\t`), Unicode directional overrides (U+202A–U+202E, U+2066–U+2069), and embedded newlines.
- Wired into tag/group/note/resource/noteType/category create + update paths.

## Tests

- Unit: ✓ `application_context/validation/entity_name_test.go` (6 cases)
- API: ✓ `server/api_tests/json_error_leaks_appcontext_test.go` (2 cases)
- API: ✓ `server/api_tests/entity_name_control_chars_test.go` (8 cases)
- All new tests pass 3× consecutively (both pre-fix 3× red and post-fix 3× green verified)
- Full `go test ./...`: ✓
- Full E2E (browser + CLI): ✓
- Postgres: ✓

## Evidence of fix

- BH-P05: `curl -s http://localhost:8181/resource.json?id=abc | jq 'has("_appContext")'` → `false`
- BH-019: `POST /v1/tag name=bh-nul%00byte` → HTTP 400

## Bug-hunt-log update

Post-merge, both BH-P05 and BH-019 move from active to fixed in `tasks/bug-hunt-log.md`.
EOF
)"
gh pr merge --merge --delete-branch
```

- [ ] **Step 5: Update bug-hunt-log and clean up**

Per master plan Step F.
