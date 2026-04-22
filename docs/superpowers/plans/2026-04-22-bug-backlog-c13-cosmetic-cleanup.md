# Cluster 13 — Cosmetic Cleanup (BH-001, BH-002, BH-007)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Can run 3 parallel subagents — task groups A, B, C touch disjoint files. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Three surgical cosmetic fixes: stop double-rendering the "Meta Data" heading on tag + note-text pages (BH-001), make `renderJsonTable(null)` safe (BH-002), and stop the version-compare action bar from wrapping the "Upload New Version" button to three lines (BH-007).

**Architecture:** All three bugs are visible-but-trivial and share nothing beyond "cosmetic". Group A drops one `{% include %}` line from two templates. Group B changes `renderJsonTable` to return a `DocumentFragment` for `null`/`undefined` instead of a primitive string (also fixes downstream `appendChild(string)` throw). Group C restructures the version-compare flex container to wrap the upload form onto a second row when three children are visible.

**Tech Stack:** Pongo2 templates, Playwright E2E (`page.evaluate` used for the JS-unit-style BH-002 test — the repo has no dedicated JS unit runner), Tailwind utility classes.

**Worktree branch:** `bugfix/c13-cosmetic-cleanup`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 13 section. c13 is small enough that a dedicated cluster spec would be redundant; the top-level spec section is sufficient.

---

## File structure

**Modified:**
- `templates/displayTag.tpl:28` — drop duplicate `sideTitle.tpl` include (BH-001)
- `templates/displayNoteText.tpl:54` — drop duplicate `sideTitle.tpl` include (BH-001)
- `src/tableMaker.js:3-64` — `renderJsonTable` returns an empty `DocumentFragment` for null/undefined; the recursion paths in `generateObjectTable` / `generateArrayTable` accept that correctly (BH-002)
- `templates/partials/versionPanel.tpl:54-78` — responsive-stack the action bar (BH-007)

**Created:**
- `e2e/tests/c13-bh001-dup-meta-heading.spec.ts` — asserts exactly one `<h2>Meta Data</h2>` on tag + note-text pages
- `e2e/tests/c13-bh002-json-table-null.spec.ts` — calls `renderJsonTable(null)` in `page.evaluate` and asserts the result is a `Node` that `appendChild` accepts
- `e2e/tests/c13-bh007-version-panel-layout.spec.ts` — asserts the "Upload New Version" button is on a single visual line at ≥ `sm` breakpoint

**No backend changes.** No Go code changes. No migrations.

---

## Task 0: Create worktree

**Files:**
- N/A

- [ ] **Step 1: Create the worktree from master**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c13-cosmetic-cleanup ../mahresources-c13 master
cd ../mahresources-c13
```

Expected: worktree at `../mahresources-c13` checked out on `bugfix/c13-cosmetic-cleanup`.

- [ ] **Step 2: Verify baseline tests pass before any changes**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS. If FAIL, stop and fix the baseline first — do not start the cluster on a red trunk (per user memory `feedback_dont_rerun_tests_to_investigate.md`: read the code, don't just rerun).

---

## Task Group A: BH-001 — Duplicate "Meta Data" heading

### Task A1: Write failing E2E test for duplicate heading

**Files:**
- Create: `e2e/tests/c13-bh001-dup-meta-heading.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-001: Duplicate "META DATA" heading on tag and note-text pages.
 *
 * Both displayTag.tpl and displayNoteText.tpl include sideTitle.tpl with
 * title="Meta Data" AND json.tpl — and json.tpl already renders its own
 * <h2 class="sidebar-group-title">Meta Data</h2>. Result: two stacked
 * headings. The fix drops the redundant sideTitle include from both
 * templates.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-001: single Meta Data heading', () => {
  test('tag display page shows exactly one "Meta Data" heading', async ({
    page,
    apiClient,
  }) => {
    const tagName = `BH001-tag-${Date.now()}`;
    const tag = await apiClient.createTag(tagName);

    await page.goto(`/tag?id=${tag.ID}`);

    // Count sidebar-group-title h2s whose text is exactly "Meta Data"
    const count = await page.locator('h2.sidebar-group-title', { hasText: /^Meta Data$/ }).count();
    expect(count).toBe(1);
  });

  test('note-text display page shows exactly one "Meta Data" heading', async ({
    page,
    apiClient,
  }) => {
    // Create a note-type with a non-empty Meta so json.tpl has something to render.
    const nt = await apiClient.createNoteType({ name: `BH001-nt-${Date.now()}` });
    const note = await apiClient.createNote({
      name: `BH001-note-${Date.now()}`,
      noteTypeId: nt.ID,
      meta: { k: 'v' },
    });

    await page.goto(`/noteText?id=${note.ID}`);

    const count = await page.locator('h2.sidebar-group-title', { hasText: /^Meta Data$/ }).count();
    expect(count).toBe(1);
  });
});
```

- [ ] **Step 2: Run test to verify it fails with exactly the BH-001 symptom**

```bash
cd e2e && npx playwright test c13-bh001-dup-meta-heading --reporter=line
```

Expected: both tests FAIL with `Expected: 1, Received: 2`. If the failure is anything else (404, selector missed), fix the test before proceeding — the failure must match the real symptom.

### Task A2: Remove duplicate sideTitle includes from both templates

**Files:**
- Modify: `templates/displayTag.tpl:28` — drop one line
- Modify: `templates/displayNoteText.tpl:54` — drop one line

- [ ] **Step 1: Remove the duplicate include in `displayTag.tpl`**

Find the block at lines 26–30:

```pongo2
    <div class="sidebar-group">
        {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
        {% include "/partials/json.tpl" with jsonData=tag.Meta %}
    </div>
```

Replace with (drop line 28):

```pongo2
    <div class="sidebar-group">
        {% include "/partials/json.tpl" with jsonData=tag.Meta %}
    </div>
```

- [ ] **Step 2: Remove the duplicate include in `displayNoteText.tpl`**

Find the block at lines 53–56:

```pongo2
    {% if sc.MetaJson %}
    {% include "/partials/sideTitle.tpl" with title="Meta Data" %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
    {% endif %}
```

Replace with (drop line 54):

```pongo2
    {% if sc.MetaJson %}
    {% include "/partials/json.tpl" with jsonData=note.Meta %}
    {% endif %}
```

- [ ] **Step 3: Run the BH-001 test to verify it passes**

```bash
cd e2e && npx playwright test c13-bh001-dup-meta-heading --reporter=line
```

Expected: both tests PASS.

- [ ] **Step 4: Commit**

```bash
git add templates/displayTag.tpl templates/displayNoteText.tpl e2e/tests/c13-bh001-dup-meta-heading.spec.ts
git commit -m "$(cat <<'EOF'
fix(templates): BH-001 — drop duplicate Meta Data heading

displayTag.tpl and displayNoteText.tpl both included sideTitle.tpl with
title="Meta Data" AND json.tpl — but json.tpl already renders its own
<h2>Meta Data</h2>, so the heading stacked twice. Drop the redundant
sideTitle include; json.tpl owns the heading.

E2E: e2e/tests/c13-bh001-dup-meta-heading.spec.ts asserts exactly one
heading on both pages.
EOF
)"
```

---

## Task Group B: BH-002 — `renderJsonTable(null)` throws

### Task B1: Write failing E2E test that exercises `renderJsonTable(null)` via `page.evaluate`

**Files:**
- Create: `e2e/tests/c13-bh002-json-table-null.spec.ts`

Rationale for E2E-over-unit: the repo has no dedicated JS unit test runner (no `vitest.config`, no `jest.config`). `tableMaker.js` is bundled by Vite and only runs in a browser context. Testing via `page.evaluate()` on a tag page with no Meta is the most faithful reproduction of BH-002's observed symptom ("renderJsonTable(null) throws on entities with no Meta").

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-002: renderJsonTable(null) throws on entities with no Meta.
 *
 * templates/partials/json.tpl:33 calls appendChild(renderJsonTable(jsonData))
 * without guarding for null. tableMaker.js:3 (renderJsonTable) falls through
 * the Array/Date/object branches on null and returns a primitive string from
 * the final ternary, which makes appendChild throw TypeError.
 *
 * Fix: renderJsonTable returns an empty DocumentFragment for null/undefined.
 * appendChild(fragment) is a no-op — no throw, no DOM pollution.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-002: renderJsonTable handles null/undefined', () => {
  test('renderJsonTable(null) returns a Node that appendChild accepts', async ({ page }) => {
    // Load any page that bundles main.js so renderJsonTable is globally importable.
    await page.goto('/tags');

    const result = await page.evaluate(async () => {
      // tableMaker.js is an ES module; import it from the bundled assets.
      // The bundler emits renderJsonTable as part of main.js via src/main.js.
      // We expose it for this test via a dynamic import of a known module path
      // using the window.__tableMaker global we'll add on the page — but since
      // we can't add that, evaluate the actual DOM behaviour: simulate the call
      // and capture whether it throws.
      //
      // Approach: locate the json.tpl x-init path. We mount a small host element
      // and call the exported module directly from the bundle. In practice we
      // grab the function via the already-imported module registry exposed
      // through the main entry.
      const mod = await import('/dist/main.js' as any);
      // main.js imports tableMaker; it may re-export or not. If renderJsonTable
      // is not on mod, the test author must expose it via src/main.js. The fix
      // task (B2) will ensure renderJsonTable is exported on window for testability.

      const fn = (window as any).renderJsonTable;
      if (typeof fn !== 'function') {
        return { error: 'window.renderJsonTable not found — fix task B2 must expose it' };
      }

      const host = document.createElement('div');
      try {
        const out = fn(null);
        host.appendChild(out); // Must NOT throw
        return {
          ok: true,
          isNode: out instanceof Node,
          isFragment: out instanceof DocumentFragment,
          childCount: host.childNodes.length,
        };
      } catch (err: any) {
        return { error: String(err?.message ?? err) };
      }
    });

    expect(result).toEqual({ ok: true, isNode: true, isFragment: true, childCount: 0 });
  });

  test('renderJsonTable(undefined) returns a Node that appendChild accepts', async ({ page }) => {
    await page.goto('/tags');

    const result = await page.evaluate(async () => {
      const fn = (window as any).renderJsonTable;
      if (typeof fn !== 'function') {
        return { error: 'window.renderJsonTable not found' };
      }
      const host = document.createElement('div');
      try {
        const out = fn(undefined);
        host.appendChild(out);
        return { ok: true, isFragment: out instanceof DocumentFragment };
      } catch (err: any) {
        return { error: String(err?.message ?? err) };
      }
    });

    expect(result).toEqual({ ok: true, isFragment: true });
  });

  test('tag detail page with no Meta produces no Alpine/console errors', async ({
    page,
    apiClient,
  }) => {
    const tagName = `BH002-tag-${Date.now()}`;
    const tag = await apiClient.createTag(tagName); // No .Meta assigned

    const consoleErrors: string[] = [];
    page.on('pageerror', (err) => consoleErrors.push(String(err)));
    page.on('console', (msg) => {
      if (msg.type() === 'error') consoleErrors.push(msg.text());
    });

    await page.goto(`/tag?id=${tag.ID}`);
    await page.waitForLoadState('networkidle');

    const offending = consoleErrors.filter((m) => /renderJsonTable|appendChild|parameter 1 is not of type 'Node'/i.test(m));
    expect(offending, `unexpected errors: ${offending.join('\n')}`).toEqual([]);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd e2e && npx playwright test c13-bh002-json-table-null --reporter=line
```

Expected: all three tests FAIL. The first two fail with `error: 'window.renderJsonTable not found'` (B2 will expose it). The third fails with `parameter 1 is not of type 'Node'` or a similar `appendChild` error on the tag page — that's the exact BH-002 symptom.

### Task B2: Fix `renderJsonTable` to return `DocumentFragment` for null/undefined; expose for tests

**Files:**
- Modify: `src/tableMaker.js:3-64` (adjust the `renderJsonTable` entry function)
- Modify: `src/main.js` (add a test-only `window.renderJsonTable = renderJsonTable` export)

- [ ] **Step 1: Fix `renderJsonTable` in `src/tableMaker.js`**

Find the existing function entry at lines 3–64:

```javascript
export function renderJsonTable(data, path = ["$"], key = "") {
    if (Array.isArray(data)) {
        return generateArrayTable(data, path);
    }

    if (data instanceof Date) {
        return createDateElement(data.getTime());
    }

    if (typeof data === "object" && data !== undefined && data !== null) {
        return generateObjectTable(data, path);
    }
    // ... primitive branches ...
}
```

Add an early-return for `null`/`undefined` as the very first check — before any other branch — so the caller always receives a `Node`:

```javascript
export function renderJsonTable(data, path = ["$"], key = "") {
    // BH-002: null/undefined must return a Node so appendChild() is safe.
    // An empty DocumentFragment appends no children and is the least-surprising
    // "nothing to render" signal.
    if (data === null || data === undefined) {
        return document.createDocumentFragment();
    }

    if (Array.isArray(data)) {
        return generateArrayTable(data, path);
    }

    if (data instanceof Date) {
        return createDateElement(data.getTime());
    }

    if (typeof data === "object") {
        return generateObjectTable(data, path);
    }
    // ... primitive branches unchanged ...
}
```

Note the second change: the `typeof data === "object" && data !== undefined && data !== null` guard simplifies to `typeof data === "object"` because the null/undefined case is already handled above. `typeof null === "object"` would have been a bug; the early-return preempts it.

Leave the rest of the function (primitive branches lines 16–63) unchanged.

- [ ] **Step 2: Expose `renderJsonTable` on `window` for test harness**

Find `src/main.js` and check for existing exports. Add (idempotent — only if not already present):

```javascript
import { renderJsonTable } from './tableMaker.js';

// Expose for E2E tests. The function is otherwise only reachable via the
// x-init in templates/partials/json.tpl.
window.renderJsonTable = renderJsonTable;
```

Placement: near the top of `src/main.js` alongside existing imports. Do NOT wrap in `if (DEV)` — the test runs against the production bundle.

- [ ] **Step 3: Rebuild the JS bundle**

```bash
npm run build-js
```

Expected: Vite builds cleanly, `public/dist/main.js` + `public/dist/assets/*.js` updated.

- [ ] **Step 4: Run the BH-002 test to verify it passes**

```bash
cd e2e && npx playwright test c13-bh002-json-table-null --reporter=line
```

Expected: all three tests PASS.

- [ ] **Step 5: Commit**

```bash
git add src/tableMaker.js src/main.js public/dist/ e2e/tests/c13-bh002-json-table-null.spec.ts
git commit -m "$(cat <<'EOF'
fix(frontend): BH-002 — renderJsonTable(null) returns DocumentFragment

Previously renderJsonTable(null) fell through the Array/Date/object
branches and returned a primitive string from the final ternary, which
made appendChild(string) throw TypeError in the json.tpl x-init.

Return an empty DocumentFragment for null/undefined up front — appendChild
of a fragment is a no-op, no throw, no DOM pollution. Also simplify the
object guard now that null is handled above.

Expose renderJsonTable on window for the E2E test harness.

E2E: e2e/tests/c13-bh002-json-table-null.spec.ts asserts (a) the pure
function returns a Node for null/undefined, and (b) a tag page with no
Meta produces no appendChild errors.
EOF
)"
```

---

## Task Group C: BH-007 — Version-compare action bar layout

### Task C1: Write failing E2E test for the three-line button wrap

**Files:**
- Create: `e2e/tests/c13-bh007-version-panel-layout.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-007: Version-compare action bar wraps "Upload New Version" to 3 lines
 * when Compare Selected is visible.
 *
 * versionPanel.tpl:54-78 is a flex row with 2 children by default; a third
 * appears when compareMode && selected.length === 2. The upload form then
 * gets squeezed, and the "Upload New Version" button label wraps over three
 * lines — looks broken.
 *
 * Fix: wrap the upload form onto a second row on small viewports and use
 * whitespace-nowrap so the button label never splits.
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-007: version-compare action bar layout', () => {
  test('upload button label does not wrap when compare mode is active with 2 selected', async ({
    page,
    apiClient,
  }) => {
    // Create a resource with 3 versions so compare mode is reachable.
    const resource = await apiClient.createResource({ name: `BH007-${Date.now()}`, content: 'v1' });
    await apiClient.uploadResourceVersion(resource.ID, 'v2', 'version 2');
    await apiClient.uploadResourceVersion(resource.ID, 'v3', 'version 3');

    await page.setViewportSize({ width: 1024, height: 800 });
    await page.goto(`/resource?id=${resource.ID}`);

    // Open the Versions <details> panel and toggle Compare mode
    await page.locator('details:has-text("Versions")').locator('summary').click();
    await page.getByRole('button', { name: /^Compare$/ }).click();

    // Select 2 versions (click their list rows' checkboxes)
    const versionCheckboxes = page.locator('details:has-text("Versions") input[type="checkbox"]');
    await versionCheckboxes.nth(0).check();
    await versionCheckboxes.nth(1).check();

    // The upload button should be visible and on a single visual line
    const uploadBtn = page.getByRole('button', { name: /Upload New Version/ });
    await expect(uploadBtn).toBeVisible();

    const box = await uploadBtn.boundingBox();
    expect(box, 'upload button boundingBox').not.toBeNull();

    // Compute the button's line-height and assert its height is ≤ ~1.4× lh
    // (single line, not two or three lines).
    const { height, lineHeight } = await uploadBtn.evaluate((el) => {
      const cs = window.getComputedStyle(el);
      return { height: el.getBoundingClientRect().height, lineHeight: parseFloat(cs.lineHeight) };
    });

    expect(height, `button height ${height}px should stay within a single line (lh=${lineHeight}px)`)
      .toBeLessThan(lineHeight * 1.8);
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd e2e && npx playwright test c13-bh007-version-panel-layout --reporter=line
```

Expected: FAIL with the button height exceeding `1.8 × lineHeight` (the three-line wrap). If the failure is something else (selector missed, resource creation error), fix the test first.

### Task C2: Fix the action bar layout in `versionPanel.tpl`

**Files:**
- Modify: `templates/partials/versionPanel.tpl:54-78`

- [ ] **Step 1: Restructure the flex container to stack on narrow rows and give the upload button nowrap**

Find the block at lines 54–78:

```pongo2
        <div class="p-4 bg-stone-50">
            <div class="flex items-center justify-between">
                <button @click="compareMode = !compareMode; selected = []"
                        class="px-3 py-1 text-sm border rounded hover:bg-stone-100"
                        :class="{ 'bg-amber-100 border-amber-300': compareMode }">
                    <span x-text="compareMode ? 'Cancel Compare' : 'Compare'"></span>
                </button>

                <template x-if="compareMode && selected.length === 2">
                    <a :href="'/resource/compare?r1={{ resourceId }}&v1=' + selected[0] + '&v2=' + selected[1]"
                       class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800">
                        Compare Selected
                    </a>
                </template>

                <form action="/v1/resource/versions?resourceId={{ resourceId }}" method="post" enctype="multipart/form-data"
                      class="flex items-center space-x-2">
                    <input type="file" name="file" required class="text-sm" aria-label="Upload file for new version">
                    <input type="text" name="comment" placeholder="Comment (optional)"
                           class="px-2 py-1 text-sm border rounded" aria-label="Version comment">
                    <button type="submit" class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800">
                        Upload New Version
                    </button>
                </form>
            </div>
        </div>
```

Replace with (stack the upload form on a second row via `flex-wrap`; constrain the compare toggle + "Compare Selected" to a single left group; add `whitespace-nowrap` on the submit button so the label never splits regardless of width):

```pongo2
        <div class="p-4 bg-stone-50">
            <div class="flex flex-wrap items-center gap-y-2 gap-x-4 justify-between">
                <div class="flex items-center gap-2">
                    <button @click="compareMode = !compareMode; selected = []"
                            class="px-3 py-1 text-sm border rounded hover:bg-stone-100"
                            :class="{ 'bg-amber-100 border-amber-300': compareMode }">
                        <span x-text="compareMode ? 'Cancel Compare' : 'Compare'"></span>
                    </button>

                    <template x-if="compareMode && selected.length === 2">
                        <a :href="'/resource/compare?r1={{ resourceId }}&v1=' + selected[0] + '&v2=' + selected[1]"
                           class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800 whitespace-nowrap">
                            Compare Selected
                        </a>
                    </template>
                </div>

                <form action="/v1/resource/versions?resourceId={{ resourceId }}" method="post" enctype="multipart/form-data"
                      class="flex items-center gap-2 flex-wrap">
                    <input type="file" name="file" required class="text-sm" aria-label="Upload file for new version">
                    <input type="text" name="comment" placeholder="Comment (optional)"
                           class="px-2 py-1 text-sm border rounded" aria-label="Version comment">
                    <button type="submit" class="px-3 py-1 text-sm bg-amber-700 text-white rounded hover:bg-amber-800 whitespace-nowrap">
                        Upload New Version
                    </button>
                </form>
            </div>
        </div>
```

Key changes:
- Outer row gets `flex-wrap` + `gap-y-2 gap-x-4` — two rows on narrow widths, clean row-gap vertical spacing, horizontal gap stays tight.
- Compare-toggle + Compare-Selected grouped in a `<div>` so they stay left-aligned together.
- Upload form gets `gap-2 flex-wrap` — inputs can stack if really narrow.
- Both action buttons get `whitespace-nowrap` — the "Upload New Version" and "Compare Selected" labels never split mid-label.

- [ ] **Step 2: Rebuild Tailwind CSS**

```bash
npm run build-css
```

Expected: clean Tailwind build; `public/tailwind.css` updated.

- [ ] **Step 3: Run the BH-007 test to verify it passes**

```bash
cd e2e && npx playwright test c13-bh007-version-panel-layout --reporter=line
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add templates/partials/versionPanel.tpl public/tailwind.css e2e/tests/c13-bh007-version-panel-layout.spec.ts
git commit -m "$(cat <<'EOF'
fix(templates): BH-007 — version-compare action bar no longer wraps upload label

The action bar was a single flex row designed for 2 children; when the
third child (Compare Selected) appears, the upload form gets squeezed
and the "Upload New Version" button label wraps to three lines.

Wrap the outer row with flex-wrap + gap-y-2 so the upload form drops
to a second row on narrow widths. Whitespace-nowrap on both action
buttons so their labels never split mid-label regardless of width.

E2E: e2e/tests/c13-bh007-version-panel-layout.spec.ts asserts the
button height stays within 1.8× line-height with Compare Selected
visible at 1024px.
EOF
)"
```

---

## Task D: Update `tasks/bug-hunt-log.md`

**Files:**
- Modify: `tasks/bug-hunt-log.md` — move BH-001, BH-002, BH-007 to the Fixed/closed table with PR + commit references

- [ ] **Step 1: Update each of the three active entries to show FIXED + link the PR**

For BH-001, change the **Status** line from `verified` to:

```markdown
- **Status:** **FIXED** (2026-04-22, c13-cosmetic-cleanup, PR #XX merged <sha>)
- **Original status (pre-fix):** verified
```

Repeat for BH-002 and BH-007.

- [ ] **Step 2: Add the three rows to the Fixed / closed table**

Append to the `## Fixed / closed pre-existing` table in `tasks/bug-hunt-log.md`:

```markdown
| BH-001 | **fixed** (2026-04-22, c13-cosmetic-cleanup, PR #XX merged <sha>) | Dropped the duplicate `{% include "/partials/sideTitle.tpl" ... %}` from `displayTag.tpl` and `displayNoteText.tpl`; `json.tpl` already owns the `<h2>Meta Data</h2>`. E2E: `e2e/tests/c13-bh001-dup-meta-heading.spec.ts`. |
| BH-002 | **fixed** (2026-04-22, c13-cosmetic-cleanup, PR #XX merged <sha>) | `renderJsonTable(null)` and `renderJsonTable(undefined)` now return an empty `DocumentFragment` up front in `src/tableMaker.js`, so the `appendChild` call in `partials/json.tpl` no longer throws `TypeError: parameter 1 is not of type 'Node'`. E2E: `e2e/tests/c13-bh002-json-table-null.spec.ts`. |
| BH-007 | **fixed** (2026-04-22, c13-cosmetic-cleanup, PR #XX merged <sha>) | `templates/partials/versionPanel.tpl` action bar now uses `flex flex-wrap gap-y-2` so the upload form drops to a second row on narrow widths; both action buttons get `whitespace-nowrap` so labels never split. E2E: `e2e/tests/c13-bh007-version-panel-layout.spec.ts`. |
```

Leave the `#XX` and `<sha>` placeholders literal for now — they're filled in after the PR opens/merges.

- [ ] **Step 3: Commit the log update**

```bash
git add tasks/bug-hunt-log.md
git commit -m "chore(bughunt): close BH-001/002/007 — c13 cosmetic cleanup"
```

---

## Task E: Full test matrix verification

- [ ] **Step 1: Run Go unit tests (SQLite)**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS. c13 is frontend-only but the Go suite must remain green.

- [ ] **Step 2: Run full browser + CLI E2E in parallel against ephemeral servers**

```bash
cd e2e && npm run test:with-server:all
```

Expected: PASS. If any non-c13 test fails, per user memory `feedback_fix_all_confirmed_bugs.md` that's in-scope to investigate — read the failure, find the root cause, fix it or call it out explicitly before opening the PR.

- [ ] **Step 3: Run accessibility E2E**

```bash
cd e2e && npm run test:with-server:a11y
```

Expected: PASS. c13 doesn't touch a11y surface but the axe-core suite must stay green.

- [ ] **Step 4: Run Postgres tests**

```bash
go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1
cd e2e && npm run test:with-server:postgres
```

Expected: both PASS. Docker must be running. c13 has no DB-layer changes so parity is expected.

---

## Task F: Open the PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin bugfix/c13-cosmetic-cleanup
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "fix(bughunt c13): BH-001/002/007 cosmetic cleanup" --body "$(cat <<'EOF'
## Summary
- **BH-001** — dropped the duplicate `sideTitle.tpl` include from `displayTag.tpl` and `displayNoteText.tpl`; `json.tpl` already owns the "Meta Data" heading.
- **BH-002** — `renderJsonTable(null/undefined)` now returns an empty `DocumentFragment`; the `appendChild` in `json.tpl` no longer throws for entities with no Meta.
- **BH-007** — version-compare action bar wraps onto a second row on narrow widths; both action buttons get `whitespace-nowrap` so labels never split.

Spec: `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 13 section.
Plan: `docs/superpowers/plans/2026-04-22-bug-backlog-c13-cosmetic-cleanup.md`.

## Test plan
- [x] Go unit tests pass (`go test --tags 'json1 fts5' ./...`)
- [x] E2E browser + CLI pass (`cd e2e && npm run test:with-server:all`)
- [x] E2E a11y pass (`cd e2e && npm run test:with-server:a11y`)
- [x] Postgres parity pass (Go + E2E)
- [x] `tasks/bug-hunt-log.md` updated
EOF
)"
```

- [ ] **Step 3: After PR is merged, update the log with the real PR # + sha**

```bash
git checkout master && git pull
# Edit tasks/bug-hunt-log.md: replace "#XX" and "<sha>" with actual values
git add tasks/bug-hunt-log.md
git commit -m "chore(bughunt): backfill c13 PR # and merge sha"
git push origin master
```

- [ ] **Step 4: Clean up the worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree remove ../mahresources-c13
git branch -d bugfix/c13-cosmetic-cleanup
```

---

## Self-review checklist

Before calling the cluster done, verify:

- [ ] All three BH-IDs moved to Fixed/closed with real PR + sha (not `#XX`/`<sha>`)
- [ ] Three new `c13-*.spec.ts` files exist and pass in the main branch after merge
- [ ] No test file from any other cluster is failing
- [ ] `tasks/bug-hunt-log.md` active-bug count dropped by exactly 3
- [ ] Worktree cleaned up, branch deleted
