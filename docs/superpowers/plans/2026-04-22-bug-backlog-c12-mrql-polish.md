# Cluster 12 — MRQL Polish (BH-012, BH-013)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Task groups A and B touch disjoint code (frontend Save/Update UX vs backend LIMIT handling). Can run as parallel subagents. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Give the MRQL editor a proper Update path for saved queries (BH-012 — stop forcing delete-and-recreate), and reduce the MRQL default LIMIT from the too-permissive 1000 to a configurable 500 while surfacing a banner when the default is applied (BH-013).

**Architecture:**
- **Group A (BH-012):** `mrqlEditor` tracks `loadedSavedQueryId` state. `saveQuery()` routes to `PUT /v1/mrql/saved?id={id}` when loaded-id is present and name unchanged; otherwise `POST /v1/mrql/saved` (current behavior). UI shows an "Update" button label next to the "Save as new" label.
- **Group B (BH-013):** Rename the hardcoded `defaultMRQLLimit = 1000` const in `application_context/mrql_context.go` to a config field. New flag `--mrql-default-limit` / `MRQL_DEFAULT_LIMIT` (default 500). When the default is applied (parsed.Limit < 0), set a flag on the MRQL response so the UI can render a banner. Banner text: "Default limit applied (N rows) — add `LIMIT` / `OFFSET` to the query to paginate."

**Tech Stack:** Go (config wiring, GORM), Alpine.js, Pongo2, Playwright E2E.

**Worktree branch:** `bugfix/c12-mrql-polish`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 12.

---

## File structure

**Modified:**
- `src/components/mrqlEditor.js` — track `loadedSavedQueryId`; add `updateQuery()` path; split save UI into "Update" + "Save as new"
- `templates/mrqlEditor.tpl` (or `templates/mrql.tpl`) — render the two save options + the default-limit banner
- `application_context/mrql_context.go:100-123` — replace const with config-backed value; flag the response when default was applied
- `application_context/context.go` — add `MRQLDefaultLimit int` to `Config` struct, wire it
- `cmd/mahresources/main.go` or wherever flags are defined — add `--mrql-default-limit` flag, default 500
- `server/api_handlers/mrql_api_handlers.go` — include `default_limit_applied` in the JSON response
- `CLAUDE.md` — document the new flag in the config table

**Created:**
- `application_context/mrql_context_test.go` (extend if exists, else create) — unit test for default-limit flag in response
- `e2e/tests/c12-bh012-mrql-update-vs-save.spec.ts`
- `e2e/tests/c12-bh013-mrql-default-limit-banner.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c12-mrql-polish ../mahresources-c12 master
cd ../mahresources-c12
```

- [ ] **Step 2: Baseline**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS (baseline 0 failed / ≥1452 passed per c10 merge).

---

## Task Group A: BH-012 — Update vs Save-as-new

### Task A1: Write failing E2E for Update path

**Files:**
- Create: `e2e/tests/c12-bh012-mrql-update-vs-save.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-012: Saved MRQL queries cannot be updated in place — only create.
 *
 * Repro (pre-fix): load a saved query, edit, click Save → dialog opens with
 * empty Name field, treating this as a new save. PUT /v1/mrql/saved is
 * wired on the backend but the UI never calls it.
 *
 * Fix: mrqlEditor tracks loadedSavedQueryId. Save button splits into
 * "Update" (PUT) when loaded + name unchanged, and "Save as new" (POST).
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-012: MRQL editor Update path', () => {
  test('loading a saved query then saving edits routes to PUT and preserves id', async ({ page, apiClient }) => {
    // Arrange: create a saved query via the backend
    const name = `BH012-${Date.now()}`;
    const originalQuery = 'type = resource';
    const createResp = await apiClient.request.post('/v1/mrql/saved', {
      data: { name, query: originalQuery, description: 'original' },
    });
    const created = await createResp.json();
    const savedId = created.ID || created.id;
    expect(savedId).toBeTruthy();

    // Act: navigate to /mrql, load the saved query, edit it
    await page.goto('/mrql');
    await page.getByTestId('mrql-saved-panel').locator(`[data-saved-id="${savedId}"]`).click();
    const editor = page.locator('[data-testid="mrql-input"]');
    await editor.fill('type = note');

    // Click the "Update" affordance (must exist after the fix)
    const updateBtn = page.getByTestId('mrql-update-button');
    await expect(updateBtn).toBeVisible();
    await updateBtn.click();

    // Assert: the saved query's query text is updated server-side under the same ID
    const updatedResp = await apiClient.request.get(`/v1/mrql/saved?id=${savedId}`);
    const updated = await updatedResp.json();
    expect(updated.Query || updated.query).toBe('type = note');
  });

  test('renaming a loaded saved query then saving creates a new row (POST)', async ({ page, apiClient }) => {
    const name = `BH012-rename-${Date.now()}`;
    const createResp = await apiClient.request.post('/v1/mrql/saved', {
      data: { name, query: 'type = tag', description: '' },
    });
    const created = await createResp.json();
    const originalId = created.ID || created.id;

    await page.goto('/mrql');
    await page.getByTestId('mrql-saved-panel').locator(`[data-saved-id="${originalId}"]`).click();

    // Open the Save-as-new dialog
    const saveAsNewBtn = page.getByTestId('mrql-save-as-new-button');
    await saveAsNewBtn.click();
    await page.getByTestId('mrql-save-name-input').fill(`${name}-copy`);
    await page.getByTestId('mrql-save-confirm-button').click();

    // Assert: original still exists, and a new row with the copy name was created
    const listResp = await apiClient.request.get('/v1/mrql/saved?all=1');
    const list = await listResp.json();
    const names = list.map((r: any) => r.Name || r.name);
    expect(names).toContain(name);
    expect(names).toContain(`${name}-copy`);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c12-bh012-mrql-update-vs-save --reporter=line --repeat-each=3
```

Expected: FAIL — `mrql-update-button` testid does not exist pre-fix.

### Task A2: Extend `mrqlEditor.js` state + save logic

**Files:**
- Modify: `src/components/mrqlEditor.js`

- [ ] **Step 1: Track loaded-saved-query state**

In the component's return object (where state fields are declared), add:

```javascript
// BH-012: track the saved query that was loaded into the editor, so Save
// can branch between PUT (update) and POST (create).
loadedSavedQueryId: null,
loadedSavedQueryName: '',
```

- [ ] **Step 2: Set it on `loadSavedQuery`**

Find `loadSavedQuery(q)` (around line 353):

```javascript
loadSavedQuery(q) {
    this.setQuery(q.query);
    this.execute();
},
```

Change to:

```javascript
loadSavedQuery(q) {
    this.setQuery(q.query);
    this.loadedSavedQueryId = q.ID ?? q.id ?? null;
    this.loadedSavedQueryName = q.Name ?? q.name ?? '';
    this.execute();
},
```

- [ ] **Step 3: Add `updateQuery()` method**

Add after `saveQuery()`:

```javascript
// BH-012: PUT branch — reuses the loaded saved-query id. Does NOT prompt
// for a name; the name stays the same.
async updateQuery() {
    if (!this.loadedSavedQueryId) return;
    const query = this.getQuery().trim();
    if (!query) return;
    if (this.validationError) {
        this.saveError = 'Fix syntax errors before updating';
        return;
    }
    this.saveError = '';

    try {
        const resp = await fetch('/v1/mrql/saved?id=' + encodeURIComponent(this.loadedSavedQueryId), {
            method: 'PUT',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({
                name: this.loadedSavedQueryName,
                query: query,
            }),
        });
        if (!resp.ok) {
            const errData = await resp.json().catch(() => null);
            this.saveError = errData?.error || errData?.Error || `Update failed (${resp.status})`;
            return;
        }
        await this.fetchSavedQueries();
    } catch (err) {
        this.saveError = err.message || 'Network error';
    }
},

// BH-012: reset the loaded-id state so Save acts as a fresh create.
clearLoadedSaved() {
    this.loadedSavedQueryId = null;
    this.loadedSavedQueryName = '';
},
```

- [ ] **Step 4: Expose `canUpdate` getter**

Add alongside other getters:

```javascript
get canUpdate() {
    return !!this.loadedSavedQueryId && !this.validationError && this.getQuery().trim().length > 0;
},
```

### Task A3: Render Update + Save-as-new in the template

**Files:**
- Locate via `grep -rn "mrqlEditor" templates/ | head` — typically `templates/mrqlEditor.tpl` or `templates/mrql.tpl`
- Modify: that file

- [ ] **Step 1: Find the existing Save button**

Look for the section where the Save button is rendered, e.g.:

```pongo2
<button @click="showSaveDialog = true" ...>Save</button>
```

- [ ] **Step 2: Replace with dual-affordance**

```pongo2
<button x-show="canUpdate"
        @click="updateQuery()"
        data-testid="mrql-update-button"
        class="...existing-button-classes...">
    Update "<span x-text="loadedSavedQueryName"></span>"
</button>

<button @click="showSaveDialog = true"
        data-testid="mrql-save-as-new-button"
        class="...existing-button-classes...">
    <span x-text="canUpdate ? 'Save as new' : 'Save'"></span>
</button>
```

Inside the save-dialog form, add `data-testid="mrql-save-name-input"` to the name input and `data-testid="mrql-save-confirm-button"` to the confirm button.

### Task A4: Build + run the E2E

```bash
npm run build
cd e2e && npx playwright test c12-bh012-mrql-update-vs-save --reporter=line
```

Expected: PASS.

### Task A5: Commit

```bash
git add src/components/mrqlEditor.js templates/ public/dist/ public/tailwind.css \
  e2e/tests/c12-bh012-mrql-update-vs-save.spec.ts
git commit -m "feat(mrql): BH-012 — editable saved queries via Update/Save-as-new

mrqlEditor now tracks loadedSavedQueryId + loadedSavedQueryName. The Save
button splits into two:
- 'Update \"{name}\"' routes to PUT /v1/mrql/saved?id={id} when a saved
  query is loaded. No dialog; the name is preserved.
- 'Save as new' still routes to POST /v1/mrql/saved with a prompted name.

Previously the only path was POST-with-empty-name, forcing users into a
delete-and-recreate workflow and leaving PUT /v1/mrql/saved unused.

E2E: e2e/tests/c12-bh012-mrql-update-vs-save.spec.ts."
```

---

## Task Group B: BH-013 — Configurable default LIMIT + banner

### Task B1: Write failing test for banner flag on response

**Files:**
- Create/extend: `server/api_tests/mrql_default_limit_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// BH-013: when an MRQL query has no LIMIT clause, the server must apply a
// default AND flag the response so the UI can show "Default limit applied".
func TestMRQLResponseSignalsDefaultLimitApplied(t *testing.T) {
	tc := SetupTestEnv(t)

	// Seed a few entities so the default kicks in
	for i := 0; i < 3; i++ {
		tc.MakeRequest(http.MethodPost, "/v1/tag",
			strings.NewReader(url.Values{"name": {"BH013-default-" + string(rune('A'+i))}}.Encode()),
			withHeader("Content-Type", "application/x-www-form-urlencoded"))
	}

	// Query without LIMIT
	body := url.Values{"query": {"type = tag"}}.Encode()
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", strings.NewReader(body),
		withHeader("Content-Type", "application/x-www-form-urlencoded"))
	assertStatus(t, resp, 200)

	var got map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("parse response: %v", err)
	}

	flag, ok := got["default_limit_applied"].(bool)
	if !ok {
		t.Fatalf("response missing default_limit_applied bool field; got keys: %v", mapKeys(got))
	}
	if !flag {
		t.Errorf("expected default_limit_applied=true for query without LIMIT")
	}

	applied, ok := got["applied_limit"].(float64)
	if !ok {
		t.Fatalf("response missing applied_limit numeric field")
	}
	if applied <= 0 {
		t.Errorf("expected applied_limit > 0, got %v", applied)
	}
}

// Query WITH explicit LIMIT must NOT set the flag.
func TestMRQLResponseDoesNotSignalWhenLimitExplicit(t *testing.T) {
	tc := SetupTestEnv(t)
	body := url.Values{"query": {"type = tag LIMIT 5"}}.Encode()
	resp := tc.MakeRequest(http.MethodPost, "/v1/mrql", strings.NewReader(body),
		withHeader("Content-Type", "application/x-www-form-urlencoded"))
	assertStatus(t, resp, 200)

	var got map[string]any
	_ = json.Unmarshal(resp.Body.Bytes(), &got)
	if flag, _ := got["default_limit_applied"].(bool); flag {
		t.Error("expected default_limit_applied=false for query with explicit LIMIT")
	}
}

func mapKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
```

Helpers `withHeader` and `assertStatus` should already exist in the test package — check neighbouring test files and either reuse or declare.

- [ ] **Step 2: Run 3× to verify fails**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestMRQLResponseSignalsDefaultLimitApplied -v -count=3
```

Expected: FAIL with `default_limit_applied` missing.

### Task B2: Replace const with config-backed value + flag response

**Files:**
- Modify: `application_context/mrql_context.go:100-123`
- Modify: `application_context/context.go` — `Config` struct field
- Modify: wherever config is loaded from flags/env (search `ExportRetention` for a pattern reference)
- Modify: `server/api_handlers/mrql_api_handlers.go` — include flag in response

- [ ] **Step 1: Add `MRQLDefaultLimit int` to `Config` struct in `application_context/context.go`**

Follow the same pattern as `ExportRetention`. Default to 500.

- [ ] **Step 2: Wire the flag in `cmd/mahresources/main.go` (or whatever main file does flag parsing)**

```go
mrqlDefaultLimit := flag.Int("mrql-default-limit", envIntDefault("MRQL_DEFAULT_LIMIT", 500),
    "Default LIMIT applied to MRQL queries without an explicit LIMIT clause")
```

Wire into the `Config` struct.

- [ ] **Step 3: Replace the const in `mrql_context.go`**

Delete:

```go
const defaultMRQLLimit = 1000
```

Replace every read (line 76, 122, 159) with `ctx.Config.MRQLDefaultLimit`.

- [ ] **Step 4: Return a flag on the result**

Extend `MRQLResult` in `application_context/mrql_context.go` (find the struct at the top of the file):

```go
type MRQLResult struct {
    // ... existing fields ...
    DefaultLimitApplied bool `json:"default_limit_applied"`
    AppliedLimit        int  `json:"applied_limit"`
}
```

In `ExecuteMRQL`, after the effective limit is computed:

```go
result.DefaultLimitApplied = parsed.Limit < 0
if parsed.Limit < 0 {
    result.AppliedLimit = ctx.Config.MRQLDefaultLimit
} else {
    result.AppliedLimit = parsed.Limit
}
```

Same treatment in `ExecuteMRQLGrouped` for consistency.

- [ ] **Step 5: Ensure api_handlers pass `DefaultLimitApplied` through**

Search `server/api_handlers/mrql_api_handlers.go` for how `MRQLResult` is marshaled — if it uses direct JSON encoding, the new field flows through automatically. If it maps to a DTO, add the field to the DTO.

### Task B3: Add the banner in `mrqlEditor`

**Files:**
- Modify: `src/components/mrqlEditor.js` (store the flag from the response)
- Modify: `templates/mrqlEditor.tpl` (or equivalent — render the banner)

- [ ] **Step 1: Capture the flag when results come in**

In the response-handling code in `execute()`:

```javascript
this.defaultLimitApplied = !!(data?.default_limit_applied);
this.appliedLimit = data?.applied_limit ?? 0;
```

- [ ] **Step 2: Render the banner**

Add to the results panel template:

```pongo2
<div x-show="defaultLimitApplied"
     data-testid="mrql-default-limit-banner"
     class="mt-2 p-2 text-sm bg-amber-50 border border-amber-200 rounded text-amber-800">
    Default limit applied (<span x-text="appliedLimit"></span> rows) — add
    <code>LIMIT</code>&nbsp;/&nbsp;<code>OFFSET</code> to the query to paginate.
</div>
```

### Task B4: Write failing E2E for the banner

**Files:**
- Create: `e2e/tests/c12-bh013-mrql-default-limit-banner.spec.ts`

Assert that a LIMIT-less query shows the banner, a WITH-LIMIT query does not. Cap the assertion at banner visibility, not specific text.

### Task B5: Build + run tests

```bash
npm run build
go test --tags 'json1 fts5' ./server/api_tests/ -run TestMRQLResponseSignalsDefaultLimitApplied -v -count=1
cd e2e && npx playwright test c12-bh013-mrql-default-limit-banner --reporter=line
```

Expected: PASS.

### Task B6: Document the flag in `CLAUDE.md`

Add a row to the config table:

```markdown
| `-mrql-default-limit` | `MRQL_DEFAULT_LIMIT` | Default `LIMIT` applied to MRQL queries without an explicit LIMIT clause (default: 500) |
```

### Task B7: Commit

```bash
git add application_context/mrql_context.go application_context/context.go \
  cmd/ server/api_handlers/mrql_api_handlers.go \
  src/components/mrqlEditor.js templates/ public/dist/ public/tailwind.css \
  server/api_tests/mrql_default_limit_test.go \
  e2e/tests/c12-bh013-mrql-default-limit-banner.spec.ts \
  CLAUDE.md
git commit -m "feat(mrql): BH-013 — configurable default LIMIT + banner when applied

Replaces the hardcoded defaultMRQLLimit = 1000 with a config-backed value.
New flag --mrql-default-limit / MRQL_DEFAULT_LIMIT, default 500.

MRQLResult gains default_limit_applied + applied_limit fields. The mrql
editor shows a banner reading 'Default limit applied (N rows) — add
LIMIT / OFFSET to paginate' whenever the default kicks in. Users who
supply an explicit LIMIT see no banner.

Rationale: 1000 is too permissive on million-row deployments (per
CLAUDE.md target profile); 500 is a safer default while still fitting
most exploratory queries without pagination.

API test: server/api_tests/mrql_default_limit_test.go.
E2E: e2e/tests/c12-bh013-mrql-default-limit-banner.spec.ts.
Docs: CLAUDE.md config table."
```

---

## Task C: Update `tasks/bug-hunt-log.md`

Mark BH-012 and BH-013 as FIXED; add rows to Fixed/closed table.

---

## Task D: Full test matrix + PR + merge + log backfill + cleanup

Standard pattern. PR title: `fix(bughunt c12): BH-012/013 mrql polish`.

---

## Self-review checklist

- [ ] Update button + Save-as-new button both visible in mrqlEditor
- [ ] PUT /v1/mrql/saved endpoint actually exercised (not just reachable)
- [ ] `MRQL_DEFAULT_LIMIT` flag documented in CLAUDE.md
- [ ] Default-limit banner appears on queries without LIMIT, disappears with explicit LIMIT
- [ ] Existing MRQL tests (unit + e2e) still pass
