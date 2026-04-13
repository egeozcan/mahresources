# Block MRQL Shortcode Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow `[mrql]...[/mrql]` block syntax where the inner content becomes the per-item template for query results, overriding `CustomMRQLResult` and `format`.

**Architecture:** Single function change in `RenderMRQLShortcode`. When `sc.IsBlock` with non-empty trimmed inner content, stamp every result item's `CustomMRQLResult` with the block template and force custom rendering. Existing `renderFlatWithCustom` handles the rest.

**Tech Stack:** Go, Playwright (E2E), Tailwind CSS

---

### Task 1: Unit tests for block MRQL shortcode

**Files:**
- Modify: `shortcodes/mrql_handler_test.go`

- [ ] **Step 1: Write test — block flat results use inner content as template**

Add to `shortcodes/mrql_handler_test.go`:

```go
func TestMRQLBlockFlatUsesInnerTemplate(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Alpha"}, Meta: []byte(`{}`)},
			{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "Beta"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<span class="item">[property path="Name"]</span>[/mrql]`,
		InnerContent: `<span class="item">[property path="Name"]</span>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<span class="item">Alpha</span>`)
	assert.Contains(t, html, `<span class="item">Beta</span>`)
}
```

- [ ] **Step 2: Write test — block overrides CustomMRQLResult**

```go
func TestMRQLBlockOverridesCustomMRQLResult(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType:       "resource",
				EntityID:         1,
				Entity:           testEntity{ID: 1, Name: "Item"},
				Meta:             []byte(`{}`),
				CustomMRQLResult: `<div class="category-template">CATEGORY</div>`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<p class="block-tpl">[property path="Name"]</p>[/mrql]`,
		InnerContent: `<p class="block-tpl">[property path="Name"]</p>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<p class="block-tpl">Item</p>`)
	assert.NotContains(t, html, "CATEGORY")
}
```

- [ ] **Step 3: Write test — block with format="table" still uses inner content**

```go
func TestMRQLBlockOverridesFormatAttr(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Row"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test", "format": "table"},
		Raw:          `[mrql query="test" format="table"]<b>[property path="Name"]</b>[/mrql]`,
		InnerContent: `<b>[property path="Name"]</b>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<b>Row</b>")
	assert.NotContains(t, html, "<table")
}
```

- [ ] **Step 4: Write test — block with bucketed results applies template per item**

```go
func TestMRQLBlockBucketedAppliesTemplate(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "bucketed",
		Groups: []QueryResultGroup{
			{
				Key: map[string]any{"type": "photo"},
				Items: []QueryResultItem{
					{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Sunset"}, Meta: []byte(`{}`)},
					{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "Mountain"}, Meta: []byte(`{}`)},
				},
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<em>[property path="Name"]</em>[/mrql]`,
		InnerContent: `<em>[property path="Name"]</em>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<em>Sunset</em>")
	assert.Contains(t, html, "<em>Mountain</em>")
	// Bucket header still rendered
	assert.Contains(t, html, "photo")
}
```

- [ ] **Step 5: Write test — block with aggregated results ignores inner content**

```go
func TestMRQLBlockAggregatedIgnoresInnerContent(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "aggregated",
		Rows: []map[string]any{
			{"category": "photo", "count": float64(10)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]SHOULD NOT APPEAR[/mrql]`,
		InnerContent: `SHOULD NOT APPEAR`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.NotContains(t, html, "SHOULD NOT APPEAR")
	assert.Contains(t, html, "<table")
	assert.Contains(t, html, "photo")
}
```

- [ ] **Step 6: Write test — whitespace-only block falls back to normal rendering**

```go
func TestMRQLBlockWhitespaceOnlyFallsBack(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Fallback"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          "[mrql query=\"test\"]\n  \n[/mrql]",
		InnerContent: "\n  \n",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	// Falls back to default card rendering (has a link with entity name)
	assert.Contains(t, html, "Fallback")
	assert.Contains(t, html, `href="/resource?id=1"`)
}
```

- [ ] **Step 7: Write test — block with empty results shows "No results."**

```go
func TestMRQLBlockEmptyResultsShowsDefault(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items:      []QueryResultItem{},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<b>[property path="Name"]</b>[/mrql]`,
		InnerContent: `<b>[property path="Name"]</b>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "No results.")
}
```

- [ ] **Step 8: Write test — block body with [meta] renders item-specific values**

```go
func TestMRQLBlockChildContextWithMeta(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType: "resource",
				EntityID:   1,
				Entity:     testEntity{ID: 1, Name: "Item A"},
				Meta:       []byte(`{"rating": 5}`),
				MetaSchema: `{"type":"object","properties":{"rating":{"type":"integer"}}}`,
			},
			{
				EntityType: "resource",
				EntityID:   2,
				Entity:     testEntity{ID: 2, Name: "Item B"},
				Meta:       []byte(`{"rating": 3}`),
				MetaSchema: `{"type":"object","properties":{"rating":{"type":"integer"}}}`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"][property path="Name"] [meta path="rating"][/mrql]`,
		InnerContent: `[property path="Name"] [meta path="rating"]`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	// [property] should render per-item names
	assert.Contains(t, html, "Item A")
	assert.Contains(t, html, "Item B")
	// [meta] should render as meta-shortcode web components with per-item entity IDs
	assert.Contains(t, html, `data-path="rating"`)
	assert.Contains(t, html, `data-entity-id="1"`)
	assert.Contains(t, html, `data-entity-id="2"`)
}
```

- [ ] **Step 9: Run all tests — verify the core ones fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run "TestMRQLBlock" -v`

Expected: 5 tests FAIL (the ones that assert block template rendering):
- `TestMRQLBlockFlatUsesInnerTemplate` — FAIL (block content ignored)
- `TestMRQLBlockOverridesCustomMRQLResult` — FAIL (category template still used)
- `TestMRQLBlockOverridesFormatAttr` — FAIL (table format still used)
- `TestMRQLBlockBucketedAppliesTemplate` — FAIL (block content ignored)
- `TestMRQLBlockChildContextWithMeta` — FAIL (block content ignored)

3 tests PASS already (they test fallback/ignore behavior that matches current code):
- `TestMRQLBlockAggregatedIgnoresInnerContent` — PASS (aggregated already ignores block)
- `TestMRQLBlockWhitespaceOnlyFallsBack` — PASS (whitespace block is a no-op today)
- `TestMRQLBlockEmptyResultsShowsDefault` — PASS (empty results already show default)

- [ ] **Step 10: Commit failing tests**

```bash
git add shortcodes/mrql_handler_test.go
git commit -m "test: add failing unit tests for block mrql shortcode"
```

---

### Task 2: Implement block template logic in RenderMRQLShortcode

**Files:**
- Modify: `shortcodes/mrql_handler.go:19-55`

- [ ] **Step 1: Add block template override logic**

In `shortcodes/mrql_handler.go`, replace the `RenderMRQLShortcode` function body. The change is: after executor returns, check for block template, and if present, stamp items and force custom rendering.

Replace the entire `RenderMRQLShortcode` function (lines 19-55) with:

```go
func RenderMRQLShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	query := sc.Attrs["query"]
	saved := sc.Attrs["saved"]
	if query == "" && saved == "" {
		return ""
	}

	limit := parseIntAttr(sc.Attrs["limit"], defaultMRQLShortcodeLimit)
	buckets := parseIntAttr(sc.Attrs["buckets"], defaultMRQLShortcodeBuckets)
	format := sc.Attrs["format"] // "" means auto-resolve
	scopeGroupID := resolveScopeKeyword(sc.Attrs["scope"], ctx)

	result, err := executor(reqCtx, query, saved, limit, buckets, scopeGroupID)
	if err != nil {
		return fmt.Sprintf(
			`<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3 font-mono">%s</div>`,
			html.EscapeString(err.Error()),
		)
	}

	if result == nil {
		return ""
	}

	// Block template: trim and check. Non-empty trimmed content overrides
	// CustomMRQLResult on every item and forces custom rendering.
	blockTemplate := ""
	if sc.IsBlock {
		blockTemplate = strings.TrimSpace(sc.InnerContent)
	}

	if blockTemplate != "" {
		applyBlockTemplate(result, blockTemplate)
		format = "custom"
	}

	var inner string

	switch result.Mode {
	case "aggregated":
		inner = renderAggregatedTable(result.Rows)
	case "bucketed":
		inner = renderBucketed(reqCtx, result.Groups, format, ctx, renderer, executor, depth)
	default: // "flat" or empty
		inner = renderFlat(reqCtx, result.Items, format, ctx, renderer, executor, depth)
	}

	return fmt.Sprintf(`<div class="mrql-results">%s</div>`, inner)
}
```

- [ ] **Step 2: Add the applyBlockTemplate helper**

Add this function below `RenderMRQLShortcode` (before `renderFlat`):

```go
// applyBlockTemplate stamps every entity item in the result with the block
// template, overriding any category-level CustomMRQLResult. Aggregated results
// have no items so they are unaffected.
func applyBlockTemplate(result *QueryResult, tpl string) {
	for i := range result.Items {
		result.Items[i].CustomMRQLResult = tpl
	}
	for i := range result.Groups {
		for j := range result.Groups[i].Items {
			result.Groups[i].Items[j].CustomMRQLResult = tpl
		}
	}
}
```

- [ ] **Step 3: Run the new tests — verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run "TestMRQLBlock" -v`

Expected: All 8 new tests PASS.

- [ ] **Step 4: Run the full shortcodes test suite — verify no regressions**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`

Expected: All existing tests PASS alongside the new ones.

- [ ] **Step 5: Commit**

```bash
git add shortcodes/mrql_handler.go
git commit -m "feat: block mrql shortcode — inner content becomes per-item template"
```

---

### Task 3: E2E test for block MRQL shortcode

**Files:**
- Modify: `e2e/tests/shortcodes.spec.ts`

The E2E test creates a category with a `CustomHeader` containing a block `[mrql]` shortcode, creates child groups, then visits the parent group page and asserts the block template rendered per-item values.

- [ ] **Step 1: Add E2E test describe block**

Append to the end of `e2e/tests/shortcodes.spec.ts`:

```typescript
test.describe('Block MRQL shortcode', () => {
  let parentCategoryId: number;
  let parentGroupId: number;
  let childGroupIds: number[] = [];

  let childCategoryId: number;

  test.beforeAll(async ({ apiClient }) => {
    // Category for child groups — gives them a name we can assert
    const childCat = await apiClient.createCategory(
      `BlockMRQL Child ${Date.now()}`,
      'Child category',
    );
    childCategoryId = childCat.ID;

    // Parent category with block [mrql] in CustomHeader.
    // Query uses type = note to avoid scope including the parent group itself.
    const parentCat = await apiClient.createCategory(
      `BlockMRQL Parent ${Date.now()}`,
      'Parent with block mrql header',
      {
        CustomHeader: [
          '<div class="block-mrql-test">',
          '[mrql query=\'type = group\' scope="global" limit="10"]',
          '<div class="block-mrql-item"><span class="item-name">[property path="Name"]</span></div>',
          '[/mrql]',
          '</div>',
        ].join('\n'),
      },
    );
    parentCategoryId = parentCat.ID;

    // Parent group that owns child groups
    const parent = await apiClient.createGroup({
      name: `BlockMRQL Parent Group ${Date.now()}`,
      categoryId: parentCat.ID,
    });
    parentGroupId = parent.ID;

    // Create two child groups owned by the parent
    for (const name of ['Apple', 'Banana']) {
      const child = await apiClient.createGroup({
        name: `BlockMRQL ${name} ${Date.now()}`,
        categoryId: childCat.ID,
        ownerId: parentGroupId,
      });
      childGroupIds.push(child.ID);
    }
  });

  test.afterAll(async ({ apiClient }) => {
    for (const id of childGroupIds) await apiClient.deleteGroup(id);
    if (parentGroupId) await apiClient.deleteGroup(parentGroupId);
    if (parentCategoryId) await apiClient.deleteCategory(parentCategoryId);
    if (childCategoryId) await apiClient.deleteCategory(childCategoryId);
  });

  test('block [mrql] renders per-item template with property shortcodes', async ({ page }) => {
    await page.goto(`/group?id=${parentGroupId}`);
    await page.waitForLoadState('load');

    const container = page.locator('.block-mrql-test');
    await expect(container).toBeVisible({ timeout: 5000 });

    // Block template should render items using .block-mrql-item divs (not default cards)
    const items = container.locator('.block-mrql-item');
    await expect(items.first()).toBeVisible({ timeout: 5000 });

    // At least the two child groups should appear with their names via [property path="Name"]
    const containerText = await container.textContent();
    expect(containerText).toContain('BlockMRQL Apple');
    expect(containerText).toContain('BlockMRQL Banana');

    // Should NOT have the default card layout (no href links from default renderer)
    const defaultCards = container.locator('a[href*="/group?id="]');
    await expect(defaultCards).toHaveCount(0);
  });
});
```

- [ ] **Step 2: Build the application**

Run: `npm run build`

Expected: Build succeeds (CSS + JS + Go binary).

- [ ] **Step 3: Run the E2E test**

Run: `cd e2e && npm run test:with-server -- --grep "Block MRQL"`

Expected: The new test PASSES. The block template renders per-item `[property path="Name"]` with each child group's name.

- [ ] **Step 4: Run the full E2E suite to check for regressions**

Run: `cd e2e && npm run test:with-server`

Expected: All existing tests PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/tests/shortcodes.spec.ts
git commit -m "test(e2e): add block mrql shortcode test"
```

---

### Task 4: Update docs-site shortcodes page

**Files:**
- Modify: `docs-site/docs/features/shortcodes.md:156-174`

- [ ] **Step 1: Fix stale nesting limit**

In `docs-site/docs/features/shortcodes.md`, replace the Nesting subsection (lines 156-158):

Old text:
```markdown
### Nesting

`[mrql]` shortcodes can nest up to 2 levels deep. This allows CustomMRQLResult templates to contain their own `[mrql]` shortcodes. Beyond the depth limit, nested shortcodes are left as-is.
```

New text:
```markdown
### Nesting

Shortcodes can nest up to 10 levels deep (the processing recursion limit). This allows CustomMRQLResult templates and block templates to contain their own shortcodes, including nested `[mrql]` queries. Beyond the depth limit, unprocessed shortcodes are left as literal text.
```

- [ ] **Step 2: Add Block Syntax subsection after Examples**

Insert a new subsection after the existing Examples block (after line 174, before the `## [conditional]` heading). Add:

```markdown
### Block Syntax

`[mrql]` supports block mode, where the inner content becomes a per-item template:

```
[mrql query='type = resource AND tags = "recipe"' limit="5"]
  <div class="recipe-card">
    <h3>[property path="Name"]</h3>
    <p>Cook time: [meta path="cooking.time"] min</p>
  </div>
[/mrql]
```

Each result entity gets its own shortcode context, so `[meta]`, `[property]`, `[conditional]`, nested `[mrql]`, and plugin shortcodes all work inside the block body.

**Precedence rules:**

- Block template overrides any `customMRQLResult` set on the entity's category
- Block template overrides the `format` attribute (the block body is the format)
- Empty or whitespace-only blocks (`[mrql query="..."][/mrql]`) fall back to normal rendering

**Result modes:**

- **Flat queries:** Block template applied per entity
- **Bucketed GROUP BY:** Block template applied per entity within each bucket; bucket headers render normally
- **Aggregated GROUP BY:** Block template ignored; aggregated table renders as usual
```

- [ ] **Step 3: Commit**

```bash
git add docs-site/docs/features/shortcodes.md
git commit -m "docs: add block mrql syntax, fix stale nesting limit"
```

---

### Task 5: Update docs-site custom-templates page

**Files:**
- Modify: `docs-site/docs/features/custom-templates.md:367-404`

- [ ] **Step 1: Revise "How It Works" and "Format Auto-Resolution" subsections**

In `docs-site/docs/features/custom-templates.md`, replace the "Custom MRQL Result Templates" section content (lines 367-404) with:

```markdown
## Custom MRQL Result Templates

Categories, Resource Categories, and Note Types can define a `customMRQLResult` field containing a shortcode template that controls how entities of that type render in `[mrql]` shortcode results. The template is processed by the shortcode engine (not Pongo2), so `[meta]`, `[property]`, and nested `[mrql]` shortcodes work, but `{{ }}` expressions do not.

### How It Works

1. Set the `customMRQLResult` field on a Category, Resource Category, or Note Type
2. When an `[mrql]` shortcode query returns entities of that type, the custom template is used instead of the default card layout
3. The template has access to the entity context, so shortcodes like `[meta]` and `[property]` work inside it
4. If the `[mrql]` shortcode uses [block syntax](./shortcodes.md#block-syntax), the block template takes precedence over `customMRQLResult` for all items

### Setting via the UI

1. Navigate to **Categories**, **Resource Categories**, or **Note Types**
2. Create or edit an entry
3. Enter a template in the **Custom MRQL Result** textarea
4. Save

### Example

A Category with this `customMRQLResult`:

```html
<div class="flex items-center gap-2 p-2 border rounded">
  <strong>[property path="Name"]</strong>
  <span class="text-sm text-stone-500">[meta path="status"]</span>
</div>
```

When an `[mrql]` query returns groups in this category, each result renders using this template instead of the default link card.

### Format and Template Precedence

Template selection follows this priority:

1. **Block template** -- if the `[mrql]` shortcode uses block syntax with non-empty content, that block body is the per-item template. `customMRQLResult` and `format` are both ignored.
2. **Explicit `format`** -- `format="table"`, `format="list"`, or `format="compact"` override custom template rendering.
3. **`customMRQLResult`** -- when `format` is empty (auto) or `"custom"`, entities with a `customMRQLResult` use it; entities without one fall back to the default card layout.
4. **Default card layout** -- used when none of the above apply.
```

Preserve the existing content after line 404 (`## Styling Tips` and beyond) unchanged.

- [ ] **Step 2: Commit**

```bash
git add docs-site/docs/features/custom-templates.md
git commit -m "docs: revise custom MRQL result template precedence rules"
```

---

### Task 6: Full test suite verification

**Files:** None (verification only)

- [ ] **Step 1: Run all Go unit tests**

Run: `go test --tags 'json1 fts5' ./... -count=1`

Expected: All tests PASS.

- [ ] **Step 2: Run all E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`

Expected: All tests PASS.

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`

Expected: All tests PASS.
