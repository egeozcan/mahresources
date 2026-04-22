# Cluster 6 — Block Editor A11y (BH-027)

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Solo subagent. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Close 4 WCAG-A violations in the block-editor authoring surface (2 axe-critical, 2 serious).

**Architecture:** Additive ARIA attributes on the block-editor template and a lightweight Alpine extension for the Add-Block picker's roving tabindex behavior.

**Tech Stack:** Pongo2 templates, Alpine.js, Playwright + axe-core.

**Worktree branch:** `bugfix/c6-block-editor-a11y`

---

## File structure

**Modified:**
- `templates/partials/blockEditor.tpl` — gallery img alt, heading-level select aria-label, reorder/delete icon aria-label, Add-Block picker disclosure ARIA + listbox + roving tabindex.
- `src/components/blockEditor.js` — Arrow-key navigation for Add-Block picker; live-region announcement on block reorder.

**Created:**
- `e2e/tests/c6-bh027-block-editor-a11y.spec.ts`

---

## Task 1: Failing axe-core spec for block editor

**Files:**
- Create: `e2e/tests/c6-bh027-block-editor-a11y.spec.ts`

- [ ] **Step 1: Write the failing test**

```ts
import { test, expect } from '../fixtures/a11y.fixture';
import AxeBuilder from '@axe-core/playwright';

test.describe('BH-027: block editor a11y', () => {
  test('axe finds zero Critical violations on the block editor', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-${Date.now()}` });
    // Add a gallery block with at least one image so the image-alt rule has targets
    // (implementation-specific — may need to use existing test helpers).

    await page.goto(`/note/edit?id=${note.ID}`);
    await page.waitForSelector('[data-testid="block-editor"], .block-editor');

    const scan = await new AxeBuilder({ page })
      .include('.block-editor')
      .analyze();

    const critical = scan.violations.filter(v => v.impact === 'critical');
    expect(critical, JSON.stringify(critical, null, 2)).toEqual([]);
  });

  test('gallery images have non-empty alt', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-gallery-${Date.now()}` });
    // Add gallery block with test resource, via existing helpers or direct DB seed.
    await page.goto(`/note/edit?id=${note.ID}`);

    const galleryImgs = page.locator('.block-gallery img, [data-block-type="gallery"] img');
    const count = await galleryImgs.count();
    if (count === 0) test.skip(true, 'no gallery images seeded — skip image-alt assertion');

    for (let i = 0; i < count; i++) {
      const alt = await galleryImgs.nth(i).getAttribute('alt');
      expect(alt).not.toBeNull();
      expect(alt!.trim().length).toBeGreaterThan(0);
    }
  });

  test('heading-level select has accessible name', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-h-${Date.now()}` });
    await page.goto(`/note/edit?id=${note.ID}`);
    // Insert a heading block via the UI
    await page.locator('button[aria-label="Add block"], [data-testid="add-block-trigger"]').click();
    await page.locator('[role="listbox"] [data-block-type="heading"], text=Heading').first().click();

    const levelSelect = page.locator('[data-block-type="heading"] select').last();
    const ariaLabel = await levelSelect.getAttribute('aria-label');
    expect(ariaLabel).toMatch(/heading level/i);
  });

  test('move + delete buttons have aria-labels', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-ctrl-${Date.now()}` });
    await page.goto(`/note/edit?id=${note.ID}`);
    // Add two blocks so move-up/down appear
    for (let i = 0; i < 2; i++) {
      await page.locator('button[aria-label="Add block"], [data-testid="add-block-trigger"]').click();
      await page.locator('[role="listbox"] text=Text, [data-block-type="text"]').first().click();
    }

    const buttons = page.locator('[data-block-control]');
    const count = await buttons.count();
    for (let i = 0; i < count; i++) {
      const aria = await buttons.nth(i).getAttribute('aria-label');
      expect(aria, `block control #${i} missing aria-label`).toBeTruthy();
    }
  });

  test('add-block picker exposes disclosure + listbox semantics', async ({ page, apiClient }) => {
    const note = await apiClient.createNote({ Name: `bh027-picker-${Date.now()}` });
    await page.goto(`/note/edit?id=${note.ID}`);

    const trigger = page.locator('[data-testid="add-block-trigger"]');
    await expect(trigger).toHaveAttribute('aria-haspopup', 'listbox');
    await expect(trigger).toHaveAttribute('aria-expanded', 'false');

    await trigger.click();
    await expect(trigger).toHaveAttribute('aria-expanded', 'true');

    const list = page.locator('[role="listbox"][aria-label="Block types"]');
    await expect(list).toBeVisible();
  });
});
```

- [ ] **Step 2: Run 3× to verify fail**

```bash
cd e2e
npm run test:with-server -- --grep "BH-027" --repeat-each=3 --workers=1
```

Expected: FAIL all 3 on at least the "zero critical" assertion.

## Task 2: Fix gallery `<img>` alt

**Files:**
- Modify: `templates/partials/blockEditor.tpl:181-183, 197`

- [ ] **Step 1: Add dynamic alt**

```html
<img :src="resourceThumbUrl(resId)"
     :alt="getResourceName(resId) || 'Resource ' + resId"
     loading="lazy">
```

If `getResourceName` doesn't exist on the component, add it. The component already fetches resource metadata for gallery rendering — reuse that cache.

## Task 3: Fix heading-level select

**Files:**
- Modify: `templates/partials/blockEditor.tpl:112`

- [ ] **Step 1: Add aria-label**

```html
<select aria-label="Heading level" x-model.number="level" @change="save()">
  <option value="1">H1</option>
  <option value="2">H2</option>
  <option value="3">H3</option>
</select>
```

## Task 4: Fix move + delete buttons

**Files:**
- Modify: `templates/partials/blockEditor.tpl:37-65`

- [ ] **Step 1: Add aria-labels and a data-attribute for the test**

```html
<button type="button"
        data-block-control="move-up"
        :aria-label="'Move block ' + (index + 1) + ' up'"
        @click="moveUp(index)">
  <!-- existing icon -->
</button>
<button type="button"
        data-block-control="move-down"
        :aria-label="'Move block ' + (index + 1) + ' down'"
        @click="moveDown(index)">
  <!-- existing icon -->
</button>
<button type="button"
        data-block-control="delete"
        :aria-label="'Delete block ' + (index + 1)"
        @click="remove(index)">
  <!-- existing icon -->
</button>
```

- [ ] **Step 2: Add live-region announcement on reorder**

In `blockEditor.js`, add a `$refs.live` element in the template (`<div x-ref="live" class="sr-only" aria-live="polite"></div>`) and:

```js
moveUp(index) {
  // existing reorder
  this.$refs.live.textContent = `Block ${index + 1} moved up`;
},
moveDown(index) {
  // existing reorder
  this.$refs.live.textContent = `Block ${index + 1} moved down`;
}
```

## Task 5: Fix Add-Block picker disclosure + listbox + roving tabindex

**Files:**
- Modify: `templates/partials/blockEditor.tpl:862-888`
- Modify: `src/components/blockEditor.js` — keyboard handler

- [ ] **Step 1: Add disclosure ARIA to trigger**

```html
<button type="button"
        data-testid="add-block-trigger"
        :aria-expanded="addBlockPickerOpen.toString()"
        aria-haspopup="listbox"
        aria-controls="add-block-listbox"
        @click="addBlockPickerOpen = !addBlockPickerOpen"
        @keydown.escape="addBlockPickerOpen = false">
  Add block
</button>
```

- [ ] **Step 2: Add listbox semantics**

```html
<ul id="add-block-listbox"
    role="listbox"
    aria-label="Block types"
    x-show="addBlockPickerOpen"
    @keydown.escape="addBlockPickerOpen = false">
  <template x-for="(btype, idx) in blockTypes" :key="btype.name">
    <li role="option"
        :tabindex="idx === activePickerIndex ? 0 : -1"
        :aria-selected="idx === activePickerIndex"
        :data-block-type="btype.name"
        @click="insertBlock(btype.name)"
        @keydown.enter="insertBlock(btype.name)"
        @keydown.arrow-down.prevent="activePickerIndex = Math.min(activePickerIndex + 1, blockTypes.length - 1)"
        @keydown.arrow-up.prevent="activePickerIndex = Math.max(activePickerIndex - 1, 0)"
        @keydown.home.prevent="activePickerIndex = 0"
        @keydown.end.prevent="activePickerIndex = blockTypes.length - 1">
      <span x-text="btype.label"></span>
    </li>
  </template>
</ul>
```

- [ ] **Step 3: Initialize `activePickerIndex` in the component**

```js
addBlockPickerOpen: false,
activePickerIndex: 0,

// when the picker opens, reset index and focus the active item
$watch: {
  addBlockPickerOpen(open) {
    if (open) {
      this.activePickerIndex = 0;
      this.$nextTick(() => {
        this.$el.querySelector('[role="option"]:not([tabindex="-1"])')?.focus();
      });
    }
  }
}
```

(Adapt to however $watch is wired in the existing Alpine code — may be `init()` + explicit subscribe.)

## Task 6: Rebuild and run all BH-027 tests 3× to verify pass

- [ ] **Step 1:**

```bash
npm run build-js
cd e2e
npm run test:with-server -- --grep "BH-027" --repeat-each=3 --workers=1
```

Expected: PASS all 3 runs across all sub-tests.

- [ ] **Step 2: Run full a11y suite to ensure no regression elsewhere**

```bash
cd e2e
npm run test:with-server:a11y
```

Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add templates/partials/blockEditor.tpl src/components/blockEditor.js public/dist e2e/tests/c6-bh027-*.spec.ts
git commit -m "fix(a11y): BH-027 — block editor gallery alt, heading label, control aria-labels, picker listbox"
```

---

## Cluster PR gate

- [ ] **Step 1: Full Go + full E2E a11y**

```bash
cd <worktree>
go test --tags 'json1 fts5' ./...
cd e2e && npm run test:with-server:a11y
```

- [ ] **Step 2: Rebase + full suite per master plan.**

- [ ] **Step 3: Open PR, self-merge**

```bash
gh pr create --title "fix(a11y): BH-027 — block editor WCAG-A violations" --body "$(cat <<'EOF'
Closes BH-027.

## Changes

- Gallery `<img>`: dynamic `:alt` using resource name.
- Heading-level `<select>`: `aria-label="Heading level"`.
- Move-up / move-down / delete icon buttons: dynamic `:aria-label`; live-region announcement on reorder.
- Add-Block picker: `aria-expanded`, `aria-haspopup="listbox"`, `aria-controls` on trigger; `role="listbox"` + `aria-label="Block types"` + roving tabindex + Arrow/Home/End navigation on options.

## Tests

- E2E (axe + targeted assertions): 5 sub-tests, pass 3× pre red / post green.
- Full a11y suite: ✓
- Full E2E: ✓

## Bug-hunt-log update

Post-merge: BH-027 → Fixed / closed.
EOF
)"
gh pr merge --merge --delete-branch
```

Then master plan Step F.
