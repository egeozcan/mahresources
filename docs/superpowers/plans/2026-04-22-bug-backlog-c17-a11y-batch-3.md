# Cluster 17 — A11y batch 3 (BH-029, BH-030)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development. Two independent a11y fixes on disjoint components — parallel subagents safe. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Apply the WAI-ARIA Tree View pattern to the group hierarchy tree (BH-029) and fix the resource-compare view's color-only diff signal + radiogroup roving tabindex (BH-030).

**Architecture:**

- **Group A (BH-029):** `src/components/groupTree.js`'s `render()` and `renderNode()` produce a flat `<ul>/<li>` tree with expand buttons. Add `role="tree"` on the outer `<ul>`, `role="treeitem"` on each `<li>`, `aria-level`, `aria-setsize`, `aria-posinset`. Implement roving tabindex (only one treeitem has `tabindex=0`). Implement arrow-key navigation: Up/Down move focus between treeitems, Right expand (or move to first child), Left collapse (or move to parent), Home/End jump to first/last treeitem. Leave the existing `aria-expanded` + `aria-label` on the expand button — those are already correct.
- **Group B (BH-030):** In `templates/compare.tpl`, add `aria-label="Changed: <field>"` to every `.compare-meta-card--diff` element. In `templates/partials/compareImage.tpl` (and any other `role="radiogroup"` with `role="radio"` buttons — confirm via grep), implement roving tabindex: `tabindex=0` on the selected radio, `tabindex=-1` on the others; `ArrowRight`/`ArrowLeft` handlers advance selection.

**Tech Stack:** Plain JS (groupTree), Pongo2 (compare.tpl), Alpine.js (compareImage), axe-core via existing a11y E2E fixture.

**Worktree branch:** `bugfix/c17-a11y-batch-3`

**Top-level spec:** `docs/superpowers/specs/2026-04-22-bughunt-batch-c9-c18-design.md` — Cluster 17.

---

## File structure

**Modified:**
- `src/components/groupTree.js` — `render()` + `renderNode()` for ARIA + roving tabindex; `handleClick` reused for keyboard; new `handleKeyDown` method
- `templates/compare.tpl` — `aria-label="Changed: <field>"` on all `.compare-meta-card--diff` elements (approx 4 cards at lines 133-169)
- `templates/partials/compareImage.tpl` — roving tabindex + ArrowLeft/Right key handlers
- `templates/partials/compareInlineText.tpl` — same treatment if it has its own radiogroup
- `templates/partials/compareText.tpl` — same

**Created:**
- `e2e/tests/accessibility/c17-bh029-group-tree-a11y.spec.ts`
- `e2e/tests/accessibility/c17-bh030-compare-view-a11y.spec.ts`

---

## Task 0: Worktree + baseline

- [ ] **Step 1: Worktree**

```bash
cd /Users/egecan/Code/mahresources
git worktree add -b bugfix/c17-a11y-batch-3 ../mahresources-c17 master
cd ../mahresources-c17
```

- [ ] **Step 2: Baseline a11y E2E**

```bash
cd e2e && npm run test:with-server:a11y 2>&1 | tail -15
```

Confirm current pass count. New tests should add to PASS without reducing.

---

## Task Group A: BH-029 — Group tree ARIA

### Task A1: Write failing a11y test for tree semantics

**Files:**
- Create: `e2e/tests/accessibility/c17-bh029-group-tree-a11y.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-029: Group hierarchy tree missing ARIA tree semantics.
 *
 * Current state: tree is tab-navigable but the container is <ul> with no
 * role="tree" / role="treeitem", no aria-level/setsize/posinset, no
 * arrow-key WAI-ARIA Tree View pattern. Screen readers perceive it as a
 * flat list of links and buttons.
 */
import { a11yTest as test, expect } from '../../fixtures/a11y.fixture';

test.describe('BH-029: group tree ARIA semantics', () => {
  test('outer ul has role=tree and children have role=treeitem', async ({ page, apiClient }) => {
    // Seed a 2-level tree: root → 2 children
    const parent = await apiClient.createGroup({ name: `BH029-parent-${Date.now()}` });
    await apiClient.createGroup({ name: `BH029-child-1-${Date.now()}`, ownerGroupId: parent.ID });
    await apiClient.createGroup({ name: `BH029-child-2-${Date.now()}`, ownerGroupId: parent.ID });

    await page.goto('/groups/tree');

    const treeUl = page.locator('ul[role="tree"]').first();
    await expect(treeUl).toBeVisible();

    const treeitems = treeUl.locator('li[role="treeitem"]');
    const count = await treeitems.count();
    expect(count).toBeGreaterThan(0);

    // Each treeitem has aria-level, aria-posinset, aria-setsize
    const first = treeitems.first();
    await expect(first).toHaveAttribute('aria-level', /\d+/);
    await expect(first).toHaveAttribute('aria-posinset', /\d+/);
    await expect(first).toHaveAttribute('aria-setsize', /\d+/);
  });

  test('exactly one treeitem has tabindex=0 (roving)', async ({ page }) => {
    await page.goto('/groups/tree');
    const tabStops = page.locator('li[role="treeitem"][tabindex="0"]');
    await expect(tabStops).toHaveCount(1);
    const minusOnes = page.locator('li[role="treeitem"][tabindex="-1"]');
    const total = page.locator('li[role="treeitem"]');
    const totalCount = await total.count();
    const minusCount = await minusOnes.count();
    expect(totalCount - minusCount).toBe(1); // only the tabindex=0 treeitem remains
  });

  test('ArrowDown moves focus to next treeitem', async ({ page }) => {
    await page.goto('/groups/tree');
    const tree = page.locator('ul[role="tree"]');
    await tree.locator('li[role="treeitem"][tabindex="0"]').first().focus();
    const before = await page.evaluate(() => document.activeElement?.getAttribute('data-group-id'));
    await page.keyboard.press('ArrowDown');
    const after = await page.evaluate(() => document.activeElement?.getAttribute('data-group-id'));
    expect(after).not.toBe(before);
  });

  test('no axe violations in the tree surface', async ({ page, axeBuilder }) => {
    await page.goto('/groups/tree');
    const results = await axeBuilder().include('ul[role="tree"]').analyze();
    expect(results.violations).toEqual([]);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c17-bh029-group-tree-a11y --reporter=line --repeat-each=3
```

Expected: all runs FAIL — `role="tree"` selector returns 0.

### Task A2: Implement ARIA tree pattern in `groupTree.js`

**Files:**
- Modify: `src/components/groupTree.js:38-156`

- [ ] **Step 1: Attach ARIA attributes + roving tabindex**

Modify `render()`:

```javascript
render() {
  const container = this.$refs.treeContainer;
  if (!container) return;

  const rootNodes = this.tree['root'] || this.tree[0] || [];

  if (rootNodes.length === 0) {
    const p = document.createElement('p');
    p.className = 'text-gray-500 p-4';
    p.textContent = 'No groups found.';
    container.replaceChildren(p);
    return;
  }

  const ul = document.createElement('ul');
  ul.className = 'tree-chart-list';
  ul.setAttribute('role', 'tree');
  ul.setAttribute('aria-label', 'Group hierarchy');

  rootNodes.forEach((node, idx) => {
    ul.appendChild(this.renderNode(node, true, {
      level: 1,
      posinset: idx + 1,
      setsize: rootNodes.length,
    }));
  });

  container.replaceChildren(ul);

  // Roving tabindex: first treeitem gets tabindex=0 on initial render if no
  // current tab stop exists. All others get tabindex=-1.
  this._applyRovingTabindex(container);
},

_applyRovingTabindex(container) {
  const treeitems = container.querySelectorAll('li[role="treeitem"]');
  if (treeitems.length === 0) return;
  const currentStop = container.querySelector('li[role="treeitem"][tabindex="0"]');
  treeitems.forEach(li => li.setAttribute('tabindex', '-1'));
  if (currentStop) {
    currentStop.setAttribute('tabindex', '0');
  } else {
    treeitems[0].setAttribute('tabindex', '0');
  }
},
```

Modify `renderNode(node, isRoot, { level, posinset, setsize })` to accept and attach ARIA:

```javascript
renderNode(node, isRoot, { level = 1, posinset = 1, setsize = 1 } = {}) {
  // ... existing isHighlighted / isFocused / isExpanded / isLoading / children / hasChildren setup ...

  const li = document.createElement('li');
  if (isRoot) li.className = 'tree-root-node';

  // BH-029: ARIA tree semantics
  li.setAttribute('role', 'treeitem');
  li.setAttribute('aria-level', String(level));
  li.setAttribute('aria-posinset', String(posinset));
  li.setAttribute('aria-setsize', String(setsize));
  li.setAttribute('data-group-id', String(node.id));
  if (hasChildren) li.setAttribute('aria-expanded', isExpanded ? 'true' : 'false');

  // ... existing link (<a>) + expand button + children <ul> construction ...

  // When rendering children ul, pass level+1 and sibling index:
  if (hasChildren && isExpanded && children.length > 0) {
    const childUl = document.createElement('ul');
    childUl.className = 'tree-chart-list';
    childUl.setAttribute('role', 'group');   // per WAI-ARIA pattern: nested groups
    children.forEach((child, idx) => {
      childUl.appendChild(this.renderNode(child, false, {
        level: level + 1,
        posinset: idx + 1,
        setsize: children.length,
      }));
    });
    // ... existing "+N more" appending unchanged ...
    li.appendChild(childUl);
  }

  return li;
},
```

- [ ] **Step 2: Add arrow-key handler**

Add a new method on the component:

```javascript
handleKeyDown(e) {
  const target = e.target.closest('li[role="treeitem"]');
  if (!target) return;

  const container = this.$refs.treeContainer;
  const all = Array.from(container.querySelectorAll('li[role="treeitem"]'));
  const idx = all.indexOf(target);
  if (idx < 0) return;

  let next = null;
  switch (e.key) {
    case 'ArrowDown':
      next = all[idx + 1] || target;
      break;
    case 'ArrowUp':
      next = all[idx - 1] || target;
      break;
    case 'Home':
      next = all[0];
      break;
    case 'End':
      next = all[all.length - 1];
      break;
    case 'ArrowRight': {
      const expanded = target.getAttribute('aria-expanded');
      if (expanded === 'false') {
        // Expand this node
        const nodeId = parseInt(target.dataset.groupId, 10);
        if (!Number.isNaN(nodeId)) this.expandNode(nodeId);
        e.preventDefault();
        return;
      }
      if (expanded === 'true') {
        // Move to first child
        const firstChild = target.querySelector(':scope > ul > li[role="treeitem"]');
        if (firstChild) next = firstChild;
      }
      break;
    }
    case 'ArrowLeft': {
      const expanded = target.getAttribute('aria-expanded');
      if (expanded === 'true') {
        const nodeId = parseInt(target.dataset.groupId, 10);
        if (!Number.isNaN(nodeId)) {
          this.expandedNodes.delete(nodeId);
          this.render();
          // Refocus the same node after re-render
          const reFocused = this.$refs.treeContainer.querySelector(`li[role="treeitem"][data-group-id="${nodeId}"]`);
          reFocused?.focus();
        }
        e.preventDefault();
        return;
      }
      // Move to parent treeitem
      const parent = target.parentElement?.closest('li[role="treeitem"]');
      if (parent) next = parent;
      break;
    }
    default:
      return;
  }

  if (next && next !== target) {
    e.preventDefault();
    // Update roving tabindex
    all.forEach(li => li.setAttribute('tabindex', '-1'));
    next.setAttribute('tabindex', '0');
    next.focus();
  }
},
```

- [ ] **Step 3: Wire the handler in the template**

Find where groupTree is mounted — typically `templates/displayGroupTree.tpl` or inline in a page template:

```bash
grep -rn "groupTree(" templates/ | head
```

On the tree container's Alpine root, add:

```pongo2
x-on:keydown="handleKeyDown($event)"
```

- [ ] **Step 4: Run the E2E to verify pass**

```bash
npm run build
cd e2e && npx playwright test c17-bh029-group-tree-a11y --reporter=line
```

Expected: PASS all 4 tests.

### Task A3: Commit

```bash
git add src/components/groupTree.js templates/ public/dist/ \
  e2e/tests/accessibility/c17-bh029-group-tree-a11y.spec.ts
git commit -m "fix(a11y): BH-029 — group tree adopts WAI-ARIA Tree View pattern

Outer <ul> gains role=tree + aria-label. Each <li> gets role=treeitem,
aria-level, aria-posinset, aria-setsize, and aria-expanded when it has
children. Nested child <ul>s get role=group.

Roving tabindex: exactly one treeitem carries tabindex=0. Arrow keys
navigate per WAI-ARIA Tree View:
- Up/Down between treeitems
- Right expand (or move to first child)
- Left collapse (or move to parent)
- Home/End jump to first/last

The existing aria-expanded + aria-label on the expand button is preserved.

E2E: e2e/tests/accessibility/c17-bh029-group-tree-a11y.spec.ts."
```

---

## Task Group B: BH-030 — Compare view a11y

### Task B1: Write failing a11y test for compare view

**Files:**
- Create: `e2e/tests/accessibility/c17-bh030-compare-view-a11y.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
/**
 * BH-030: compare view diff cards communicate change via color only
 * (WCAG 1.4.1), and the image-compare mode radiogroup lacks roving
 * tabindex (WCAG 2.1.1).
 */
import { a11yTest as test, expect } from '../../fixtures/a11y.fixture';

test.describe('BH-030: compare view a11y', () => {
  test('each diff card carries aria-label="Changed: <field>"', async ({ page, apiClient }) => {
    // Set up two versions of a resource so /resource/compare produces diff cards
    const r = await apiClient.createImageResource({ name: `BH030-r-${Date.now()}` });
    const v1 = await apiClient.uploadResourceVersion(r.ID, 'v1');
    const v2 = await apiClient.uploadResourceVersion(r.ID, 'v2-different');

    await page.goto(`/resource/compare?r1=${r.ID}&v1=${v1.ID}&v2=${v2.ID}`);

    const diffCards = page.locator('.compare-meta-card--diff');
    const count = await diffCards.count();
    expect(count).toBeGreaterThan(0);

    for (let i = 0; i < count; i++) {
      const ariaLabel = await diffCards.nth(i).getAttribute('aria-label');
      expect(ariaLabel, `diff card #${i} should have aria-label`).not.toBeNull();
      expect(ariaLabel).toMatch(/^Changed:/i);
    }
  });

  test('image-compare radiogroup has exactly one radio with tabindex=0', async ({ page, apiClient }) => {
    const r = await apiClient.createImageResource({ name: `BH030-img-${Date.now()}` });
    const v1 = await apiClient.uploadResourceVersion(r.ID, 'v1');
    const v2 = await apiClient.uploadResourceVersion(r.ID, 'v2');

    await page.goto(`/resource/compare?r1=${r.ID}&v1=${v1.ID}&v2=${v2.ID}`);

    const rg = page.locator('[role="radiogroup"]').first();
    await expect(rg).toBeVisible();

    const tabStops = rg.locator('[role="radio"][tabindex="0"]');
    await expect(tabStops).toHaveCount(1);

    const minusOnes = rg.locator('[role="radio"][tabindex="-1"]');
    const radios = rg.locator('[role="radio"]');
    const total = await radios.count();
    const minus = await minusOnes.count();
    expect(total - minus).toBe(1);
  });

  test('ArrowRight moves selection to the next radio', async ({ page, apiClient }) => {
    const r = await apiClient.createImageResource({ name: `BH030-arrow-${Date.now()}` });
    const v1 = await apiClient.uploadResourceVersion(r.ID, 'v1');
    const v2 = await apiClient.uploadResourceVersion(r.ID, 'v2');
    await page.goto(`/resource/compare?r1=${r.ID}&v1=${v1.ID}&v2=${v2.ID}`);

    const radios = page.locator('[role="radiogroup"] [role="radio"]');
    const initiallyChecked = page.locator('[role="radiogroup"] [role="radio"][aria-checked="true"]').first();
    await initiallyChecked.focus();
    await page.keyboard.press('ArrowRight');

    const checkedAfter = page.locator('[role="radiogroup"] [role="radio"][aria-checked="true"]').first();
    await expect(checkedAfter).not.toHaveAttribute('id', (await initiallyChecked.getAttribute('id')) || '');
  });

  test('no axe violations on compare view', async ({ page, axeBuilder, apiClient }) => {
    const r = await apiClient.createImageResource({ name: `BH030-axe-${Date.now()}` });
    const v1 = await apiClient.uploadResourceVersion(r.ID, 'v1');
    const v2 = await apiClient.uploadResourceVersion(r.ID, 'v2');
    await page.goto(`/resource/compare?r1=${r.ID}&v1=${v1.ID}&v2=${v2.ID}`);

    const results = await axeBuilder().analyze();
    expect(results.violations).toEqual([]);
  });
});
```

- [ ] **Step 2: Run 3× to verify fails**

```bash
cd e2e && npx playwright test c17-bh030-compare-view-a11y --reporter=line --repeat-each=3
```

Expected: FAIL — diff cards have no aria-label; all radio buttons are independently tab-stoppable.

### Task B2: Add aria-label to each diff card in `compare.tpl`

**Files:**
- Modify: `templates/compare.tpl:133-180`

- [ ] **Step 1: Decorate each `.compare-meta-card--diff`**

For every `<div class="compare-meta-card{% if <condition> %} compare-meta-card--diff{% endif %}">` block, add a conditional `aria-label`:

Example for Content Type (lines 133-135):

```pongo2
<div class="compare-meta-card{% if not comparison.SameType %} compare-meta-card--diff{% endif %}"
     {% if not comparison.SameType %}aria-label="Changed: Content Type"{% endif %}>
    <div class="compare-meta-card-label">Content Type</div>
```

Do the same for:
- File Size: `aria-label="Changed: File Size"` when `comparison.SizeDelta != 0`
- Dimensions: `aria-label="Changed: Dimensions"` when `comparison.DimensionsDiff`
- Hash: `aria-label="Changed: Hash"` when `not comparison.SameHash`
- Any other `--diff` variant in the template

### Task B3: Implement roving tabindex in `compareImage.tpl` radiogroup

**Files:**
- Modify: `templates/partials/compareImage.tpl:9-30`

- [ ] **Step 1: Add `:tabindex` bindings and keyboard handler**

Wrap the radiogroup with `@keydown` and add `:tabindex` to each radio:

```pongo2
<div class="compare-segmented-control" role="radiogroup" aria-label="Comparison mode"
     @keydown="onRadiogroupKeydown($event, 'mode', ['side-by-side', 'slider', 'onion', 'toggle'])">
    <button @click="mode = 'side-by-side'" role="radio" :aria-checked="mode === 'side-by-side'"
            :tabindex="mode === 'side-by-side' ? 0 : -1"
            class="compare-seg-btn">
        ...
    </button>
    <button @click="mode = 'slider'" role="radio" :aria-checked="mode === 'slider'"
            :tabindex="mode === 'slider' ? 0 : -1"
            class="compare-seg-btn">
        ...
    </button>
    <button @click="mode = 'onion'" role="radio" :aria-checked="mode === 'onion'"
            :tabindex="mode === 'onion' ? 0 : -1"
            class="compare-seg-btn">
        ...
    </button>
    <button @click="mode = 'toggle'" role="radio" :aria-checked="mode === 'toggle'"
            :tabindex="mode === 'toggle' ? 0 : -1"
            class="compare-seg-btn">
        ...
    </button>
</div>
```

- [ ] **Step 2: Implement `onRadiogroupKeydown` in the Alpine data component**

Find `imageCompare` in `src/components/compareView.js` (or wherever its factory lives). Add:

```javascript
onRadiogroupKeydown(e, stateKey, values) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft' && e.key !== 'Home' && e.key !== 'End') return;
    e.preventDefault();
    const idx = values.indexOf(this[stateKey]);
    let nextIdx = idx;
    if (e.key === 'ArrowRight') nextIdx = (idx + 1) % values.length;
    else if (e.key === 'ArrowLeft') nextIdx = (idx - 1 + values.length) % values.length;
    else if (e.key === 'Home') nextIdx = 0;
    else if (e.key === 'End') nextIdx = values.length - 1;
    this[stateKey] = values[nextIdx];
    // After Alpine re-renders, focus the now-checked radio
    this.$nextTick(() => {
        const checked = e.currentTarget.querySelector('[role="radio"][aria-checked="true"]');
        checked?.focus();
    });
},
```

(Check whether `compareView.js` already defines similar utilities — if so, match style.)

### Task B4: Repeat for `compareInlineText.tpl` + `compareText.tpl` radiogroups

Same treatment. Document in the PR body which sub-templates received the change.

### Task B5: Run + commit

```bash
npm run build
cd e2e && npx playwright test c17-bh030-compare-view-a11y --reporter=line
```

Expected: PASS.

```bash
git add templates/ src/components/compareView.js public/dist/ public/tailwind.css \
  e2e/tests/accessibility/c17-bh030-compare-view-a11y.spec.ts
git commit -m "fix(a11y): BH-030 — compare view aria-label on diff cards + radiogroup roving tabindex

- Each .compare-meta-card--diff now carries aria-label='Changed: <field>'
  so screen-reader and color-blind users get the diff signal without
  relying on border color (WCAG 1.4.1).
- Image-compare mode radiogroup (and sibling compareText / compareInlineText
  variants) implement roving tabindex: only the checked radio is
  tab-stoppable; ArrowLeft/Right/Home/End change selection via a new
  onRadiogroupKeydown helper on the Alpine data component (WCAG 2.1.1).

E2E: e2e/tests/accessibility/c17-bh030-compare-view-a11y.spec.ts."
```

---

## Task C: Update `tasks/bug-hunt-log.md`, open PR, merge, backfill, cleanup

Standard pattern. PR title: `fix(bughunt c17): BH-029/030 a11y batch 3`.

---

## Self-review checklist

- [ ] Group tree: role=tree / role=treeitem / aria-level/setsize/posinset on every item
- [ ] Group tree: exactly one treeitem has tabindex=0, arrow keys navigate + expand/collapse
- [ ] Diff cards: all `--diff` cards have aria-label="Changed: <field>"
- [ ] Radiogroups: exactly one radio has tabindex=0; arrow keys change selection
- [ ] No axe violations introduced (existing a11y suite + new specs green)
- [ ] Existing keyboard behaviour (Tab between interactive elements) unchanged
