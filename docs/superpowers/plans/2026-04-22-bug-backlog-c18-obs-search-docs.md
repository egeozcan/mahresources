# Cluster 18 — Observability, Search, Docs (BH-005a, BH-022, BH-037)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Three bugs in three disjoint subsystems (search · OpenAPI · hash observability). Parallel subagents safe. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Make global search case-insensitive on the SQLite LIKE paths (BH-005a — fuzzy deferred as BH-005b), register the 11 missing MRQL + editMeta + plugin routes in the OpenAPI spec (BH-022), and surface DHash/AHash values on the resource detail page + an admin drill-down for DHash=0 resources (BH-037).

**Architecture:**

- **Group A (BH-005a):** SQLite's FTS5 default tokenizer (`unicode61`) is already case-insensitive via Unicode case folding, so the FTS exact/prefix path likely already handles `Pasta` vs `pasta` correctly — verify with a test first. The real gap is on the LIKE-based paths: `searchEntitiesLike` (`application_context/search_context.go:377-414`) uses `getLikeOperator()` → `LIKE` on SQLite (case-sensitive). Same for `fuzzyFallback` in `fts/sqlite.go:173-219`. Fix both paths to use `LOWER(col) LIKE LOWER(?)` on SQLite (Postgres already uses `ILIKE`, which is case-insensitive). File BH-005b as a new active-bug entry pointing at the fuzzy-search brainstorm (out of scope this batch).
- **Group B (BH-022):** Add 11 route registrations to `server/routes_openapi.go`: `/v1/mrql` (POST), `/v1/mrql/validate` (POST), `/v1/mrql/complete` (POST), `/v1/mrql/saved` (GET, POST, PUT), `/v1/mrql/saved/delete` (POST), `/v1/mrql/saved/run` (POST), `/v1/note/editMeta` (POST), `/v1/group/editMeta` (POST), `/v1/resource/editMeta` (POST). Write a drift-check Go unit test that enumerates live `/v1/` routes from the mux and fails when any aren't registered — with an explicit exclusion list for `PathPrefix("/v1/plugins/")` (dynamic handler, per-plugin).
- **Group C (BH-037):** Extend `resource_crud_context.go::GetResourceByID` to also load `ImageHash` (resource has a 1:1 relationship). Update `templates/displayResource.tpl` (find the "Technical Details" section) to render `Perceptual hash (DHash): 0x... (AHash: 0x...)` when present. Add an admin-overview drill-down linking to a filter `/resources?hash.dhash_int=0` that lists resources with DHash=0 (the BH-018 solid-color class).

**Tech Stack:** Go (GORM `Preload`, openapi registry, table-driven tests), Pongo2, Playwright E2E.

**Worktree branch:** `bugfix/c18-obs-search-docs`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 18 (BH-005 split into 005a in-batch + 005b deferred).

---

## File structure

**Modified:**
- `application_context/search_context.go:279-284, 377-414` — SQLite LIKE case-insensitive
- `fts/sqlite.go:173-219` — `fuzzyFallback` LIKE case-insensitive
- `server/routes_openapi.go` — add 11 route registrations
- `application_context/resource_crud_context.go` — `Preload("ImageHash")` on `GetResourceByID`
- `templates/displayResource.tpl` — DHash/AHash row in Technical Details
- `templates/adminOverview.tpl` — DHash=0 drill-down
- `tasks/bug-hunt-log.md` — BH-005 "split into 005a (fixed) + 005b (deferred — see new entry)", BH-022 FIXED, BH-037 FIXED, NEW entry for BH-005b

**Created:**
- `server/api_tests/global_search_case_insensitive_test.go` — BH-005a API test
- `server/openapi/drift_test.go` — BH-022 drift-check
- `e2e/tests/c18-bh005a-search-case-insensitive.spec.ts`
- `e2e/tests/c18-bh037-perceptual-hash-visible.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c18-obs-search-docs ../mahresources-c18 master
cd ../mahresources-c18
```

- [ ] **Step 2: Baseline**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS.

---

## Task Group A: BH-005a — Case-insensitive search on LIKE paths

### Task A1: Verify FTS5 unicode61 already handles case

**Files:**
- Create: `server/api_tests/global_search_case_insensitive_test.go`

- [ ] **Step 1: Write the failing test (FTS5 expected-pass, LIKE expected-fail)**

```go
package api_tests

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// BH-005a: global search case-insensitivity.
// Two tests — one runs with FTS enabled (default), one with SKIP_FTS=1 to
// force the LIKE fallback path.
func TestGlobalSearch_CaseInsensitive_FTS(t *testing.T) {
	tc := SetupTestEnv(t) // FTS enabled
	tc.MakeRequest(http.MethodPost, "/v1/tag",
		strings.NewReader(url.Values{"name": {"Pasta"}}.Encode()),
		withHeader("Content-Type", "application/x-www-form-urlencoded"))

	for _, query := range []string{"Pasta", "pasta", "PASTA"} {
		resp := tc.MakeRequest(http.MethodGet,
			"/v1/search?query="+url.QueryEscape(query),
			nil)
		assertStatus(t, resp, 200)
		if !strings.Contains(resp.Body.String(), "Pasta") {
			t.Errorf("query %q: expected tag 'Pasta' in results; got %s", query, resp.Body.String())
		}
	}
}

func TestGlobalSearch_CaseInsensitive_LIKEFallback(t *testing.T) {
	tc := SetupTestEnvWithConfig(t, TestEnvConfig{SkipFTS: true})
	tc.MakeRequest(http.MethodPost, "/v1/tag",
		strings.NewReader(url.Values{"name": {"Pasta"}}.Encode()),
		withHeader("Content-Type", "application/x-www-form-urlencoded"))

	for _, query := range []string{"Pasta", "pasta", "PASTA"} {
		resp := tc.MakeRequest(http.MethodGet,
			"/v1/search?query="+url.QueryEscape(query),
			nil)
		assertStatus(t, resp, 200)
		if !strings.Contains(resp.Body.String(), "Pasta") {
			t.Errorf("query %q: expected 'Pasta' in LIKE-fallback results; got %s", query, resp.Body.String())
		}
	}
}
```

`SetupTestEnvWithConfig` may need adding to the harness — or use a test tag to toggle FTS. If the harness doesn't support per-test FTS toggling, the second test can be `t.Skip`-ed with a comment and the LIKE path validated via unit test on `searchEntitiesLike` directly.

- [ ] **Step 2: Run 3× to observe which tests fail**

```bash
go test --tags 'json1 fts5' ./server/api_tests/ -run TestGlobalSearch_CaseInsensitive -v -count=3
```

Expected:
- FTS test may PASS (unicode61 case-folds by default) — document this finding in the cluster's commit.
- LIKE-fallback test FAILS — the real fix target.

If BOTH tests pass, BH-005a is already fixed at the DB-collation level; write that up and close BH-005a without a code change. If ONLY the FTS test passes, proceed with the LIKE fix below.

### Task A2: Make SQLite LIKE case-insensitive

**Files:**
- Modify: `application_context/search_context.go:279-284` (`getLikeOperator`) and `:377-414` (`searchEntitiesLike`)

- [ ] **Step 1: Change the SQLite LIKE pattern to `LOWER(col) LIKE LOWER(?)`**

Two approaches — pick one based on simplicity vs index-friendliness:

**Approach 1 (simpler, no index needed — good for prototyping):**

In `searchEntitiesLike`:

```go
func searchEntitiesLike[T searchable](ctx *MahresourcesContext, entityType, searchTerm string, limit int) []query_models.SearchResultItem {
    info := entitySearchInfo[entityType]
    likeOp := ctx.getLikeOperator()
    escaped := escapeLikeWildcards(searchTerm)
    pattern := "%" + escaped + "%"
    likeEscape := " ESCAPE '\\'"

    // BH-005a: on SQLite, LIKE is case-sensitive by default. Wrap both sides
    // in LOWER() for parity with Postgres's ILIKE.
    usingSQLite := ctx.Config.DbType != constants.DbTypePosgres
    var whereParts []string
    var args []any
    buildLike := func(col string) string {
        if usingSQLite {
            return "LOWER(" + col + ") " + likeOp + " LOWER(?)" + likeEscape
        }
        return col + " " + likeOp + " ?" + likeEscape
    }
    whereParts = append(whereParts, buildLike("name"))
    whereParts = append(whereParts, buildLike("description"))
    args = append(args, pattern, pattern)
    for _, col := range info.extraLikeCols {
        whereParts = append(whereParts, buildLike(col))
        args = append(args, pattern)
    }

    var entities []T
    ctx.db.
        Where(strings.Join(whereParts, " OR "), args...).
        Limit(limit).
        Find(&entities)
    // ... rest unchanged ...
}
```

**Approach 2 (COLLATE NOCASE — requires index for large tables):**

Add `COLLATE NOCASE` to the LIKE clause. Discuss with the orchestrator if this turns out to be substantially faster at scale; for now the LOWER() approach is correct and portable.

### Task A3: Make `fuzzyFallback` case-insensitive

**Files:**
- Modify: `fts/sqlite.go:173-219`

- [ ] **Step 1: Same treatment — LOWER both sides**

```go
func (s *SQLiteFTS) fuzzyFallback(db *gorm.DB, tableName, term string) *gorm.DB {
    escaped := escapeLikeWildcards(term)
    likeEscape := " ESCAPE '\\'"

    searchCols := []string{tableName + ".name", tableName + ".description"}
    if tableName == "resources" {
        searchCols = append(searchCols, tableName+".original_name")
    }

    if len(term) <= 2 {
        var conditions []string
        var args []interface{}
        for _, col := range searchCols {
            // BH-005a: LOWER() for case-insensitive LIKE on SQLite
            conditions = append(conditions, "LOWER("+col+") LIKE LOWER(?)"+likeEscape)
            args = append(args, "%"+escaped+"%")
        }
        return db.Where(strings.Join(conditions, " OR "), args...)
    }

    runes := []rune(term)
    var conditions []string
    var args []interface{}

    for i := range runes {
        before := escapeLikeWildcards(string(runes[:i]))
        after := escapeLikeWildcards(string(runes[i+1:]))
        pattern := before + "_" + after
        for _, col := range searchCols {
            conditions = append(conditions, "LOWER("+col+") LIKE LOWER(?)"+likeEscape)
            args = append(args, "%"+pattern+"%")
        }
    }
    for _, col := range searchCols {
        conditions = append(conditions, "LOWER("+col+") LIKE LOWER(?)"+likeEscape)
        args = append(args, "%"+escaped+"%")
    }

    return db.Where(strings.Join(conditions, " OR "), args...)
}
```

### Task A4: Write failing E2E

**Files:**
- Create: `e2e/tests/c18-bh005a-search-case-insensitive.spec.ts`

- [ ] **Step 1: Write the test**

```typescript
/**
 * BH-005a: global search is case-insensitive.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-005a: global search case-insensitive', () => {
  test('search "pasta" matches tag "Pasta"', async ({ page, apiClient }) => {
    const tagName = `Pasta-BH005a-${Date.now()}`;
    await apiClient.createTag(tagName);

    await page.goto('/');
    await page.keyboard.press('Meta+k');
    const searchInput = page.getByRole('textbox', { name: /search/i }).first();
    await searchInput.fill(tagName.toLowerCase().replace(/bh005a.*/, 'bh005a'));
    // Wait for results
    const results = page.locator('[role="option"], [data-search-result]');
    await expect(results.first()).toBeVisible({ timeout: 3000 });
    const text = await results.first().textContent();
    expect(text?.toLowerCase()).toContain(tagName.toLowerCase().substring(0, 8));
  });
});
```

Simplify selectors to match the actual globalSearch component — check `src/components/globalSearch.js` for the result container.

### Task A5: Build + run + commit

```bash
npm run build
go test --tags 'json1 fts5' ./server/api_tests/ -run TestGlobalSearch_CaseInsensitive -v -count=1
cd e2e && npx playwright test c18-bh005a-search-case-insensitive --reporter=line
```

Expected: PASS.

### Task A6: File BH-005b in the bug-hunt-log

- [ ] **Step 1: Add a new active entry**

In `tasks/bug-hunt-log.md`, add at the top of the Active section:

```markdown
### BH-005b · Global search has no fuzzy/typo tolerance (split from BH-005)
- **Status:** deferred (filed 2026-04-22 during c18 plan)
- **Severity:** feature-gap
- **Workflow:** discovery / search
- **Background:** BH-005a fixed case-sensitivity on the SQLite LIKE fallback paths. This remaining half covers typo-tolerance (e.g., "Weeknight" matches, "Weeknite" doesn't).
- **Design scope:** trigram vs Levenshtein vs FTS5 tokenizer change vs sqlean extension. Has perf implications on "millions of resources" deployments. Needs its own brainstorm + design doc — do NOT bolt on without evaluation.
- **Blocked on:** separate brainstorm; not scheduled.
```

### Task A7: Commit

```bash
git add application_context/search_context.go fts/sqlite.go \
  server/api_tests/global_search_case_insensitive_test.go \
  e2e/tests/c18-bh005a-search-case-insensitive.spec.ts \
  tasks/bug-hunt-log.md
git commit -m "feat(search): BH-005a — case-insensitive global search on SQLite LIKE paths

SQLite's LIKE is case-sensitive by default. The FTS5 default tokenizer
(unicode61) already case-folds, but both fallback paths — searchEntities
Like and fuzzyFallback — used raw LIKE. Users querying 'pasta' got zero
hits for a tag named 'Pasta'.

Wrap col + argument in LOWER() on SQLite only (Postgres already uses
ILIKE via getLikeOperator). No behavior change on Postgres.

BH-005b (fuzzy/typo tolerance) filed as a separate deferred entry —
needs its own brainstorm.

API test: server/api_tests/global_search_case_insensitive_test.go.
E2E: e2e/tests/c18-bh005a-search-case-insensitive.spec.ts."
```

---

## Task Group B: BH-022 — OpenAPI registration

### Task B1: Write failing drift-check test

**Files:**
- Create: `server/openapi/drift_test.go`

- [ ] **Step 1: Write the failing test**

```go
package openapi_test

import (
	"net/http"
	"sort"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/server"
)

// routesExcludedFromOpenAPI enumerates endpoints intentionally omitted from
// the OpenAPI spec. Keys are method + space + path. Each entry MUST have a
// comment explaining why it's excluded.
var routesExcludedFromOpenAPI = map[string]string{
	// PathPrefix handler — routes depend on installed plugins and cannot be
	// enumerated statically. Documented in the spec description instead.
	"ANY /v1/plugins/": "dynamic plugin-specific API; routes vary per install",
}

// BH-022: make sure every live /v1/ route is registered in the OpenAPI spec
// (or explicitly excluded).
func TestOpenAPI_RouteRegistrationCoverage(t *testing.T) {
	// Build the router with the real route set
	ctx := application_context.NewTestContext(t)
	router := server.BuildRouter(ctx)

	// Walk the mux to collect live /v1/ routes
	type liveRoute struct{ Method, Path string }
	var live []liveRoute
	err := router.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, _ := route.GetPathTemplate()
		if path == "" || !strings.HasPrefix(path, "/v1/") {
			return nil
		}
		methods, _ := route.GetMethods()
		if len(methods) == 0 {
			live = append(live, liveRoute{Method: "ANY", Path: path})
			return nil
		}
		for _, m := range methods {
			live = append(live, liveRoute{Method: m, Path: path})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("router walk: %v", err)
	}

	// Build the spec and extract its operations
	spec := server.BuildOpenAPISpec(ctx)
	inSpec := map[string]bool{}
	for path, pathItem := range spec.Paths {
		for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			if getOp(pathItem, method) != nil {
				inSpec[method+" "+path] = true
			}
		}
	}

	var missing []string
	for _, r := range live {
		key := r.Method + " " + r.Path
		if inSpec[key] {
			continue
		}
		if _, excluded := routesExcludedFromOpenAPI[key]; excluded {
			continue
		}
		// Also accept the PathPrefix catch-all exclusion key
		if r.Method == "ANY" {
			prefixKey := "ANY " + r.Path
			if _, excluded := routesExcludedFromOpenAPI[prefixKey]; excluded {
				continue
			}
		}
		missing = append(missing, key)
	}

	sort.Strings(missing)
	if len(missing) > 0 {
		t.Fatalf("OpenAPI spec missing %d routes (add to server/routes_openapi.go or routesExcludedFromOpenAPI with a reason):\n  %s",
			len(missing), strings.Join(missing, "\n  "))
	}
}

func getOp(p *openapi3.PathItem, method string) *openapi3.Operation {
	switch method {
	case http.MethodGet:
		return p.Get
	case http.MethodPost:
		return p.Post
	case http.MethodPut:
		return p.Put
	case http.MethodDelete:
		return p.Delete
	case http.MethodPatch:
		return p.Patch
	}
	return nil
}
```

Adjust imports to match the actual OpenAPI types in the repo (probably `github.com/getkin/kin-openapi/openapi3`).

- [ ] **Step 2: Run 3× to verify fails**

```bash
go test --tags 'json1 fts5' ./server/openapi/ -run TestOpenAPI_RouteRegistrationCoverage -v -count=3
```

Expected: FAIL with a list of 11 missing routes (MRQL + editMeta).

### Task B2: Register the 11 missing routes

**Files:**
- Modify: `server/routes_openapi.go` — find the existing `RegisterAPIRoutesWithOpenAPI` function

- [ ] **Step 1: Add route blocks for each missing endpoint**

For each of the 11 routes, follow the existing style in `routes_openapi.go` (find a similar existing block like `/v1/search` at line 1390 and replicate). Schemas for MRQL request/response may need new types — add them to the schemas section.

Example for one route (POST /v1/mrql):

```go
{
    Method:      http.MethodPost,
    Path:        "/v1/mrql",
    Tags:        []string{"mrql"},
    Summary:     "Execute an MRQL query",
    Description: "Parses, validates, translates, and executes a Mahresources Query Language query.",
    RequestBody: &openapi3.RequestBodyRef{
        Value: openapi3.NewRequestBody().
            WithDescription("MRQL query").
            WithContent(openapi3.NewContentWithFormDataSchema(openapi3.NewObjectSchema().
                WithProperty("query", openapi3.NewStringSchema()).
                WithProperty("limit", openapi3.NewIntegerSchema()).
                WithProperty("page", openapi3.NewIntegerSchema()))),
    },
    Responses: openapi3.NewResponses(
        openapi3.WithStatus(200, openapi3.NewResponseRef().Value(openapi3.NewResponse().
            WithDescription("MRQL result"))),
        openapi3.WithStatus(400, openapi3.NewResponseRef().Value(openapi3.NewResponse().
            WithDescription("Query syntax error"))),
    ),
},
```

Repeat for the other 10 routes. Skim `server/api_handlers/mrql_api_handlers.go` for each handler's actual input/output shape.

### Task B3: Run drift-check to verify pass

```bash
go test --tags 'json1 fts5' ./server/openapi/ -v -count=1
```

Expected: PASS with 0 missing (or only `/v1/plugins/` in exclusion list).

### Task B4: Run `go run ./cmd/openapi-gen` and check path count

```bash
go run ./cmd/openapi-gen -output /tmp/spec.yaml
grep -c "  /v1/" /tmp/spec.yaml || true
```

Expected: path count increased from 156 (pre-fix baseline) to ~167.

### Task B5: Commit

```bash
git add server/routes_openapi.go server/openapi/drift_test.go
git commit -m "docs(openapi): BH-022 — register 11 missing routes + add drift-check

server/routes_openapi.go now registers the MRQL subsystem
(/v1/mrql, /v1/mrql/validate, /v1/mrql/complete, /v1/mrql/saved × 3,
/v1/mrql/saved/delete, /v1/mrql/saved/run) and per-entity editMeta
routes (/v1/{note,group,resource}/editMeta).

Drift-check test walks the live mux, compares against registered ops,
and fails when a /v1/ route isn't either in the spec or in the explicit
exclusion list. /v1/plugins/ is excluded with a rationale (dynamic
PathPrefix handler).

Generated spec path count: 156 → ~167."
```

---

## Task Group C: BH-037 — Perceptual hash observability

### Task C1: Load ImageHash on resource detail fetch

**Files:**
- Modify: `application_context/resource_crud_context.go:22` (`GetResourceByID`)

- [ ] **Step 1: Add `Preload("ImageHash")` (or whatever the relation is named)**

First confirm the relation — check `models/resource_model.go` for the field. Typically:

```go
func (ctx *MahresourcesContext) GetResourceByID(id uint) (*models.Resource, error) {
    var resource models.Resource
    result := ctx.db.
        Preload("ImageHash").   // BH-037
        First(&resource, id)
    if result.Error != nil {
        return nil, result.Error
    }
    return &resource, nil
}
```

If `Resource` doesn't have an `ImageHash` field but instead uses a foreign key lookup, add a helper `GetResourceWithImageHash(id)` or query the hash table separately in the template context provider.

### Task C2: Render DHash/AHash row in `displayResource.tpl`

**Files:**
- Modify: `templates/displayResource.tpl` — find the "Technical Details" section

- [ ] **Step 1: Add the row**

```pongo2
{% if resource.ImageHash %}
<tr>
    <td class="technical-details-label">Perceptual hash</td>
    <td class="technical-details-value">
        <div class="font-mono text-xs break-all">
            {% if resource.ImageHash.DHashInt %}DHash: 0x{{ resource.ImageHash.DHash }}{% endif %}
            {% if resource.ImageHash.AHashInt %}<br>AHash: 0x{{ resource.ImageHash.AHash }}{% endif %}
        </div>
        <p class="mt-1 text-xs text-stone-500">Used by the perceptual similarity engine. DHash=0 indicates a uniform/solid-color image — see BH-018.</p>
    </td>
</tr>
{% endif %}
```

### Task C3: Admin overview drill-down

**Files:**
- Modify: `templates/adminOverview.tpl` — find the "Hashing" stats block

- [ ] **Step 1: Add a link alongside existing counts**

```pongo2
<a href="/resources?hashDhashInt=0"
   class="block text-xs text-amber-700 hover:underline"
   data-testid="admin-dhash-zero-drilldown">
    View N resources with DHash=0 →
</a>
```

(If the `/resources` filter doesn't yet support `hashDhashInt=0`, either add the filter to the resource-query layer — it's BH-037 scope — or link to a saved-MRQL page demonstrating how to find them. Simpler to add the filter.)

### Task C4: Write failing E2E

**Files:**
- Create: `e2e/tests/c18-bh037-perceptual-hash-visible.spec.ts`

- [ ] **Step 1: Write the test**

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-037: perceptual hash visible on resource detail', () => {
  test('resource with ImageHash row shows DHash/AHash in Technical Details', async ({ page, apiClient }) => {
    // Upload a resource that gets hashed
    const r = await apiClient.createImageResource({ name: `BH037-${Date.now()}` });

    // Poll until hashing completes (background worker)
    await expect.poll(async () => {
      const resp = await apiClient.request.get(`/v1/resource?id=${r.ID}`);
      const data = await resp.json();
      return !!(data.ImageHash && (data.ImageHash.DHashInt || data.ImageHash.AHashInt));
    }, { timeout: 15_000 }).toBe(true);

    await page.goto(`/resource?id=${r.ID}`);
    const tech = page.getByText(/technical details/i).first();
    await expect(tech).toBeVisible();

    await expect(page.getByText(/DHash: 0x/i)).toBeVisible();
  });
});
```

### Task C5: Build + run + commit

```bash
npm run build
cd e2e && npx playwright test c18-bh037-perceptual-hash-visible --reporter=line
```

Expected: PASS.

```bash
git add application_context/resource_crud_context.go templates/displayResource.tpl templates/adminOverview.tpl \
  public/dist/ public/tailwind.css \
  e2e/tests/c18-bh037-perceptual-hash-visible.spec.ts
git commit -m "feat(observability): BH-037 — surface perceptual hashes in resource UI

Resource detail 'Technical Details' section now shows DHash/AHash values
for images that have been hashed by the background worker. Admin
overview links to a filter for resources where DHash=0 — the BH-018
solid-color false-positive class.

Helps operators understand why two unrelated solids show as 'similar'
without running SQL against resource_hashes.

E2E: e2e/tests/c18-bh037-perceptual-hash-visible.spec.ts."
```

---

## Task D: Log updates + PR + merge + backfill + cleanup

Mark BH-022 and BH-037 FIXED; mark BH-005 as "split — 005a fixed (this PR), 005b deferred (new entry)"; ensure BH-005b appears as a new active entry. PR title: `fix(bughunt c18): BH-005a/022/037 observability + search + docs`.

---

## Self-review checklist

- [ ] `Pasta` and `pasta` yield identical results on SQLite
- [ ] OpenAPI drift test covers all /v1/ routes with an explicit exclusion list
- [ ] Spec path count: 156 → ≥167 (verify by running `go run ./cmd/openapi-gen`)
- [ ] DHash/AHash visible on resource detail when `ImageHash` row exists
- [ ] Admin overview has DHash=0 drill-down link
- [ ] BH-005b filed as a new active entry in bug-hunt-log.md
- [ ] BH-005 marked split (not fully fixed) — honest accounting
- [ ] Postgres tests still pass (ILIKE path untouched)
