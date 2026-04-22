# Cluster 16 — Group UX (BH-014)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (single bug — no parallelism benefit). Steps use checkbox (`- [ ]`) syntax.

**Goal:** Stop silent orphaning of child groups when a parent group is deleted (BH-014). Show a confirm dialog with live counts: "Delete N groups? This will orphan X child groups and M notes/resources (they'll move to top level)."

**Architecture:** The group bulk-delete form in `templates/partials/bulkEditorGroup.tpl:28-40` currently uses the generic `confirmAction({ message: "..." })` component which triggers `window.confirm(message)` on submit. Replace the confirm-action usage with a new `confirmGroupDelete` Alpine component that:
1. Intercepts submit.
2. Queries each selected group via the existing `/v1/group?id=<id>` endpoint (parallel fetches) to get `.SubGroups.length`, `.Notes.length`, `.Resources.length`.
3. Aggregates counts, builds a clear dialog message, and calls `confirm()` with it.
4. If confirmed, lets the form submit natively.

**Tech Stack:** Alpine.js (new component), existing group API, Playwright E2E.

**Worktree branch:** `bugfix/c16-group-ux`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 16.

---

## File structure

**Modified:**
- `templates/partials/bulkEditorGroup.tpl:28-40` — swap `confirmAction` for new `confirmGroupDelete`
- `src/main.js` — register the new Alpine data component

**Created:**
- `src/components/confirmGroupDelete.js` — new component
- `e2e/tests/c16-bh014-group-delete-orphan-warning.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c16-group-ux ../mahresources-c16 master
cd ../mahresources-c16
```

- [ ] **Step 2: Baseline**

```bash
go test --tags 'json1 fts5' ./... -count=1
```

Expected: PASS.

---

## Task 1: Write failing E2E test for the counted dialog

**Files:**
- Create: `e2e/tests/c16-bh014-group-delete-orphan-warning.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-014: deleting a parent group silently orphans its children.
 *
 * Fix: bulk-delete form uses confirmGroupDelete which fetches each
 * selected group's child/note/resource counts, aggregates them, and
 * shows "Delete N groups? This will orphan X child groups and M
 * notes/resources (they'll move to top level)."
 */
import { test, expect } from '../fixtures/base.fixture';

test.describe('BH-014: group delete orphan-warning dialog', () => {
  test('deleting a parent with 2 children + 1 note shows counts in the confirm', async ({ page, apiClient }) => {
    const parent = await apiClient.createGroup({ name: `BH014-parent-${Date.now()}` });
    await apiClient.createGroup({ name: `BH014-child1-${Date.now()}`, ownerGroupId: parent.ID });
    await apiClient.createGroup({ name: `BH014-child2-${Date.now()}`, ownerGroupId: parent.ID });
    await apiClient.createNote({ name: `BH014-note-${Date.now()}`, ownerGroupId: parent.ID });

    // Observe the confirm() invocation
    const confirmMessages: string[] = [];
    page.on('dialog', async (dialog) => {
      confirmMessages.push(dialog.message());
      await dialog.dismiss(); // Cancel — we're only checking the message
    });

    // Navigate to groups list, select the parent, click Delete
    await page.goto(`/groups?name=${encodeURIComponent(parent.Name ?? parent.name)}`);
    await page.locator(`[data-resource-id="${parent.ID}"], [data-group-id="${parent.ID}"]`).first()
      .locator('input[type="checkbox"]').first().check();

    // Click the Delete button in the bulk editor
    await page.getByRole('button', { name: /^Delete$/ }).click();

    // Assert the dialog message captured the counts
    expect(confirmMessages.length).toBeGreaterThan(0);
    const msg = confirmMessages[0];
    expect(msg).toMatch(/2\s*child group/i);
    expect(msg).toMatch(/1\s*(note|note\/resource)/i);
    expect(msg).toMatch(/orphan|move to top level/i);
  });

  test('deleting a leaf group shows a simple "no orphans" confirm', async ({ page, apiClient }) => {
    const leaf = await apiClient.createGroup({ name: `BH014-leaf-${Date.now()}` });

    const confirmMessages: string[] = [];
    page.on('dialog', async (dialog) => {
      confirmMessages.push(dialog.message());
      await dialog.dismiss();
    });

    await page.goto(`/groups?name=${encodeURIComponent(leaf.Name ?? leaf.name)}`);
    await page.locator('input[type="checkbox"]').first().check();
    await page.getByRole('button', { name: /^Delete$/ }).click();

    expect(confirmMessages.length).toBe(1);
    // Leaf group: no children → dialog should NOT mention orphaning
    expect(confirmMessages[0]).not.toMatch(/orphan|child group/i);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c16-bh014-group-delete-orphan-warning --reporter=line --repeat-each=3
```

Expected: FAIL — current dialog reads "Are you sure you want to delete the selected groups?" with no counts.

---

## Task 2: Create the `confirmGroupDelete` Alpine component

**Files:**
- Create: `src/components/confirmGroupDelete.js`

- [ ] **Step 1: Write the component**

```javascript
/**
 * BH-014: Alpine data component used by the group bulk-delete form.
 *
 * On submit:
 *   1. Reads selected group IDs from $store.bulkSelection.selectedIds.
 *   2. Fetches each group's metadata in parallel via GET /v1/group?id=<id>.
 *   3. Aggregates child-group + note + resource counts.
 *   4. Shows a confirm() dialog with the counts.
 *   5. Lets the form submit natively if confirmed.
 *
 * Fallback: on fetch error, falls back to a generic confirm — the delete
 * still works, just without counts.
 */
export function confirmGroupDelete() {
    return {
        _shiftHeld: false,
        init() {
            this._onKeyDown = (e) => { if (e.key === 'Shift') this._shiftHeld = true; };
            this._onKeyUp   = (e) => { if (e.key === 'Shift') this._shiftHeld = false; };
            document.addEventListener('keydown', this._onKeyDown);
            document.addEventListener('keyup', this._onKeyUp);
        },
        destroy() {
            document.removeEventListener('keydown', this._onKeyDown);
            document.removeEventListener('keyup', this._onKeyUp);
        },
        events: {
            async ["@submit"](e) {
                if (this._shiftHeld) return;

                // Prevent the native submit while we fetch counts.
                e.preventDefault();

                const ids = [...(Alpine.store('bulkSelection')?.selectedIds || [])];
                if (ids.length === 0) return;

                let childGroups = 0;
                let items = 0; // notes + resources combined for a cleaner message
                let fetchFailed = false;
                try {
                    const results = await Promise.all(ids.map(id =>
                        fetch('/v1/group?id=' + encodeURIComponent(id)).then(r => r.ok ? r.json() : null)
                    ));
                    for (const g of results) {
                        if (!g) { fetchFailed = true; continue; }
                        childGroups += (g.SubGroups?.length || g.subGroups?.length || 0);
                        items += (g.Notes?.length || g.notes?.length || 0);
                        items += (g.Resources?.length || g.resources?.length || 0);
                    }
                } catch (_) {
                    fetchFailed = true;
                }

                let message;
                if (fetchFailed) {
                    message = `Delete ${ids.length} group${ids.length !== 1 ? 's' : ''}? (counts unavailable — you'll see the effect after delete)`;
                } else if (childGroups === 0 && items === 0) {
                    message = `Delete ${ids.length} group${ids.length !== 1 ? 's' : ''}?`;
                } else {
                    const parts = [];
                    if (childGroups > 0) parts.push(`${childGroups} child group${childGroups !== 1 ? 's' : ''}`);
                    if (items > 0)       parts.push(`${items} note${items !== 1 ? 's/resources' : '/resource'}`);
                    message = `Delete ${ids.length} group${ids.length !== 1 ? 's' : ''}? This will orphan ${parts.join(' and ')} (they'll move to top level).`;
                }

                if (confirm(message)) {
                    // User confirmed — submit natively now.
                    e.target.submit();
                }
            },
        },
    };
}
```

- [ ] **Step 2: Register the component in `src/main.js`**

Find the block near line 51-123 that registers other `Alpine.data(...)` entries:

```javascript
import { confirmAction } from './components/confirmAction.js';
// ...
Alpine.data('confirmAction', confirmAction);
```

Add alongside:

```javascript
import { confirmGroupDelete } from './components/confirmGroupDelete.js';
// ...
Alpine.data('confirmGroupDelete', confirmGroupDelete);
```

---

## Task 3: Swap the confirm-action usage in `bulkEditorGroup.tpl`

**Files:**
- Modify: `templates/partials/bulkEditorGroup.tpl:28-40`

- [ ] **Step 1: Change the Alpine data binding**

Find:

```pongo2
    <form
            class="px-4 no-ajax"
            method="post"
            :action="'/v1/groups/delete?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            x-data="confirmAction('Are you sure you want to delete the selected groups?')"
            x-bind="events"
    >
```

Replace with:

```pongo2
    <form
            class="px-4 no-ajax"
            method="post"
            :action="'/v1/groups/delete?redirect=' + encodeURIComponent(window.location.pathname + window.location.search)"
            x-data="confirmGroupDelete"
            x-bind="events"
            data-testid="bulk-delete-groups-form"
    >
```

Keep the rest of the form body unchanged.

---

## Task 4: Build + verify

- [ ] **Step 1: Build**

```bash
npm run build
```

- [ ] **Step 2: Run the E2E**

```bash
cd e2e && npx playwright test c16-bh014-group-delete-orphan-warning --reporter=line
```

Expected: PASS both tests.

---

## Task 5: Commit

```bash
git add src/components/confirmGroupDelete.js src/main.js \
  templates/partials/bulkEditorGroup.tpl \
  public/dist/ public/tailwind.css \
  e2e/tests/c16-bh014-group-delete-orphan-warning.spec.ts
git commit -m "feat(groups): BH-014 — delete confirms with child + note/resource counts

Previously the bulk-delete form fired a generic 'Are you sure?' confirm
then silently orphaned child groups (and moved their notes/resources to
top level) with no feedback. Users had to discover the orphans by
browsing /groups with the right filter.

New confirmGroupDelete Alpine component intercepts the form submit,
fetches each selected group via GET /v1/group?id=<id> in parallel,
aggregates childGroups + notes + resources counts, and calls confirm()
with 'Delete N groups? This will orphan X child groups and M
notes/resources (they'll move to top level).'

Leaf-only selections see a simpler 'Delete N groups?' — no orphan
language when there are no orphans.

Fetch failures fall back to a generic confirm — delete still works.

E2E: e2e/tests/c16-bh014-group-delete-orphan-warning.spec.ts."
```

---

## Task 6: Update `tasks/bug-hunt-log.md`, open PR, merge, backfill, cleanup

Standard pattern. PR title: `fix(bughunt c16): BH-014 group delete orphan warning`.

---

## Self-review checklist

- [ ] BH-014 entry moved to Fixed/closed with real PR + sha
- [ ] Dialog message contains live counts for parent-with-children scenarios
- [ ] Dialog does NOT mention orphaning when selection is all leaves
- [ ] Shift-click bypass still works (the existing escape hatch from `confirmAction` is preserved)
- [ ] Fetch failure falls back gracefully — delete works even if the API can't be reached
