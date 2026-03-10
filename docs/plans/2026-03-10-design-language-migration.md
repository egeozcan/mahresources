# Design Language Migration Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Migrate mahresources' visual design to match mahpastes' aesthetic DNA — stone palette, IBM Plex Mono/Sans typography, warm amber accent, minimalist mood — across every template and CSS file.

**Architecture:** CSS-first transformation. Rewrite design tokens and component styles in `index.css`, then sweep all ~76 templates to replace inline Tailwind color classes. No JS, layout, or backend changes.

**Tech Stack:** Tailwind CSS v4, Pongo2 templates, CSS custom properties, IBM Plex fonts via Google Fonts.

---

## Class Mapping Reference

All engineers MUST use this mapping when updating inline Tailwind classes in templates. This is the single source of truth.

### Gray → Stone (direct replacement everywhere)

| Old | New |
|-----|-----|
| `text-gray-50` | `text-stone-50` |
| `text-gray-100` | `text-stone-100` |
| `text-gray-200` | `text-stone-200` |
| `text-gray-300` | `text-stone-300` |
| `text-gray-400` | `text-stone-400` |
| `text-gray-500` | `text-stone-500` |
| `text-gray-600` | `text-stone-600` |
| `text-gray-700` | `text-stone-700` |
| `text-gray-800` | `text-stone-800` |
| `text-gray-900` | `text-stone-900` |
| `bg-gray-*` | `bg-stone-*` (same shade numbers) |
| `border-gray-*` | `border-stone-*` (same shade numbers) |
| `ring-gray-*` | `ring-stone-*` (same shade numbers) |
| `divide-gray-*` | `divide-stone-*` (same shade numbers) |
| `placeholder-gray-*` | `placeholder-stone-*` (same shade numbers) |

### Primary Actions (indigo/blue → amber accent)

| Old | New | Context |
|-----|-----|---------|
| `bg-indigo-600` | `bg-amber-700` | Primary buttons |
| `bg-indigo-700` | `bg-amber-800` | Primary button hover |
| `hover:bg-indigo-700` | `hover:bg-amber-800` | Button hover |
| `bg-blue-500` | `bg-amber-700` | Action buttons |
| `bg-blue-600` | `bg-amber-700` | Action buttons |
| `hover:bg-blue-600` | `hover:bg-amber-800` | Action hover |
| `hover:bg-blue-700` | `hover:bg-amber-800` | Action hover |
| `text-indigo-600` | `text-amber-700` | Links, active states |
| `text-indigo-700` | `text-amber-800` | Link hover |
| `text-blue-600` | `text-amber-700` | Links |
| `text-blue-500` | `text-amber-700` | Links |
| `hover:text-indigo-800` | `hover:text-amber-900` | Link hover |
| `hover:text-blue-700` | `hover:text-amber-800` | Link hover |
| `border-indigo-500` | `border-amber-600` | Active borders |
| `border-blue-500` | `border-amber-600` | Active borders |
| `ring-indigo-500` | `ring-amber-600` | Focus rings |
| `ring-blue-500` | `ring-amber-600` | Focus rings |
| `focus:ring-indigo-500` | `focus:ring-amber-600` | Focus rings |
| `focus:ring-blue-500` | `focus:ring-amber-600` | Focus rings |
| `focus:border-indigo-500` | `focus:border-amber-600` | Focus borders |
| `focus:border-blue-500` | `focus:border-amber-600` | Focus borders |

### Success/Confirm Actions (green → amber accent)

| Old | New | Context |
|-----|-----|---------|
| `bg-green-700` | `bg-amber-700` | Confirm/save buttons |
| `bg-green-800` | `bg-amber-800` | Confirm hover |
| `hover:bg-green-800` | `hover:bg-amber-800` | Confirm hover |
| `hover:bg-green-900` | `hover:bg-amber-900` | Confirm hover |
| `text-green-800` | `text-amber-700` | Success links |
| `text-green-700` | `text-amber-700` | Success links |
| `ring-green-500` | `ring-amber-600` | Focus rings |
| `focus:ring-green-500` | `focus:ring-amber-600` | Focus rings |

### Danger Actions (red — keep but adjust)

| Old | New | Context |
|-----|-----|---------|
| `text-red-600` | `text-red-700` | Danger text (keep red, darken for contrast) |
| `text-red-700` | `text-red-700` | Keep |
| `bg-red-50` | `bg-red-50` | Keep |
| `bg-red-600` | `bg-red-700` | Danger buttons (darken slightly) |
| `hover:bg-red-700` | `hover:bg-red-800` | Danger hover |
| `border-red-200` | `border-red-200` | Keep |
| `ring-red-500` | `ring-red-600` | Focus rings |

### Typography Classes to Add

Where labels, metadata, nav links, buttons, badges appear, add `font-mono`. Where prose/descriptions appear, add `font-sans`. This is contextual — engineers must judge based on content type.

**font-mono targets:** nav links, table headers, labels, metadata, badge text, button text, form labels, breadcrumbs, pagination, timestamps, file sizes, counts
**font-sans targets:** descriptions, note content, prose blocks, long text, error messages, help text

---

## Phase 1: Foundation

### Task 1: Add IBM Plex Font Imports to Base Layout

**Agent:** Frontend Lead
**Files:**
- Modify: `templates/layouts/base.tpl`

**Step 1: Read the current base.tpl**

Read: `templates/layouts/base.tpl`

**Step 2: Add Google Fonts import**

Add before the existing CSS `<link>` tags in `<head>`:

```html
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=IBM+Plex+Mono:wght@400;500;600&family=IBM+Plex+Sans:wght@400;500;600&display=swap" rel="stylesheet">
```

**Step 3: Commit**

```bash
git add templates/layouts/base.tpl
git commit -m "feat: add IBM Plex Mono and Sans font imports"
```

---

### Task 2: Configure Tailwind Font Families

**Agent:** Frontend Lead
**Files:**
- Modify: `index.css` (Tailwind source config, root level)

**Step 1: Read current index.css**

Read: `index.css`

**Step 2: Add Tailwind v4 theme configuration**

Add after the `@source` directives:

```css
@theme {
  --font-sans: 'IBM Plex Sans', sans-serif;
  --font-mono: 'IBM Plex Mono', monospace;
}
```

This makes `font-sans` = IBM Plex Sans and `font-mono` = IBM Plex Mono globally via Tailwind's utility classes.

**Step 3: Commit**

```bash
git add index.css
git commit -m "feat: configure IBM Plex font families in Tailwind theme"
```

---

### Task 3: Rewrite CSS Custom Properties

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (lines 1-33, the `:root` block and site-level vars)

**Step 1: Read public/index.css lines 1-50**

Read: `public/index.css` (lines 1-50)

**Step 2: Replace the `:root` custom properties block**

Replace the existing `:root` block with:

```css
:root {
    --bg-accent: #fafaf9;
    --bg-accent-dark: #f5f5f4;
    --nav-text: #44403c;
    --nav-text-muted: #78716c;
    --nav-hover: #b45309;
    --nav-active: #92400e;
    --nav-bg: rgba(250, 250, 249, 0.85);
    --nav-border: rgba(0, 0, 0, 0.06);

    --site-spacing: 0.5rem;

    --shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.04);
    --shadow-md: 0 4px 12px rgba(0, 0, 0, 0.08);
    --shadow-lg: 0 8px 24px rgba(0, 0, 0, 0.12);

    --radius-sm: 6px;
    --radius-md: 8px;
    --radius-lg: 12px;

    --transition-fast: 120ms ease;
    --transition-normal: 200ms ease;
}
```

Key changes:
- `--bg-accent`: teal `#e8fafa` → stone-50 `#fafaf9`
- `--bg-accent-dark`: teal `#c5f0f0` → stone-100 `#f5f5f4`
- `--nav-text`: dark gray `#2d3748` → stone-700 `#44403c`
- `--nav-text-muted`: slate `#64748b` → stone-500 `#78716c`
- `--nav-hover`: teal `#0d9488` → amber-700 `#b45309`
- `--nav-active`: dark teal `#0f766e` → amber-800 `#92400e`
- `--nav-bg`: semi-transparent white → semi-transparent stone-50

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite CSS custom properties to stone palette with amber accent"
```

---

### Task 4: Rewrite Header Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (header/site-header section, ~lines 35-47)

**Step 1: Read public/index.css lines 30-50**

Read: `public/index.css` (lines 30-50)

**Step 2: Update header background**

The header currently uses a gradient from `--bg-accent` to `--bg-accent-dark`. Since we've updated those vars to stone-50/stone-100, the gradient will automatically become a subtle warm neutral gradient. Verify the gradient still looks intentional. If there are any hardcoded teal/cyan values in this section, replace them with stone equivalents.

Also update any `font-family` declarations in the header to use `'IBM Plex Mono', monospace`.

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: update header styles for stone palette"
```

---

### Task 5: Rewrite Navbar Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (navbar section, ~lines 49-216)

**Step 1: Read public/index.css lines 49-216**

Read: `public/index.css` (lines 49-216)

**Step 2: Replace all hardcoded teal values**

Search for and replace ALL instances of:
- `rgba(13, 148, 136, ...)` (teal with opacity) → `rgba(180, 83, 9, ...)` (amber-700 with same opacity)
- Any remaining `#0d9488` → `#b45309`
- Any remaining `#0f766e` → `#92400e`
- Any `#14b8a6` or similar teal variants → amber equivalents

Add `font-family: 'IBM Plex Mono', monospace;` to the navbar base styles.
Add `text-transform: uppercase;` and `letter-spacing: 0.05em;` to navbar link styles.
Ensure `font-size` is set to `0.75rem` (text-xs) for nav links.

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite navbar styles for stone/amber theme with mono typography"
```

---

### Task 6: Rewrite Card Badge Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (card-badge section, ~lines 439-511)

**Step 1: Read public/index.css lines 430-520**

Read: `public/index.css` (lines 430-520)

**Step 2: Replace badge color definitions**

```css
.card-badge--category {
    background: #f5f5f4;  /* stone-100 */
    color: #78716c;       /* stone-500 */
    border: 1px solid #d6d3d1;  /* stone-300 */
}

.card-badge--relation {
    background: #f5f3ff;  /* cool lavender */
    color: #5b21b6;       /* violet-800 */
    border: 1px solid #ddd6fe;  /* violet-200 */
}

.card-badge--tag {
    background: #fef9ee;  /* warm cream */
    color: #92400e;       /* amber-800 */
    border: 1px solid #fde68a;  /* amber-200 */
}

.card-badge--tag-active {
    background: #fef3c7;  /* amber-100 */
    color: #92400e;       /* amber-800 */
    border: 1px solid #f59e0b;  /* amber-500 */
}

.card-badge--note-type {
    background: #f0fdf4;  /* sage tint */
    color: #166534;       /* green-800 */
    border: 1px solid #bbf7d0;  /* green-200 */
}

.card-badge--action {
    background: var(--nav-active);  /* amber-800 */
    color: white;
}
```

Add `font-family: 'IBM Plex Mono', monospace;` and `font-size: 0.75rem;` to the base `.card-badge` class.

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite badge colors to muted semantic palette"
```

---

### Task 7: Rewrite Card Component Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (card section)

**Step 1: Read public/index.css card styles**

Search for `.card` styles in `public/index.css` and read the relevant sections.

**Step 2: Update card styles**

Ensure cards use:
- `border-color: #e7e5e4` (stone-200)
- `border-radius: var(--radius-md)` (8px)
- Hover: `box-shadow: var(--shadow-md)` and `border-color: #d6d3d1` (stone-300)
- `.card-title`: add `font-family: 'IBM Plex Mono', monospace; font-weight: 600; color: #292524;` (stone-800)
- `.card-meta`: add `font-family: 'IBM Plex Mono', monospace; font-size: 0.75rem; color: #a8a29e;` (stone-400)
- `.card-description`: add `font-family: 'IBM Plex Sans', sans-serif; color: #57534e;` (stone-600)
- `.card-actions` border: `border-color: #f5f5f4` (stone-100)
- Action icons: `color: #78716c` (stone-500), hover `color: #44403c` (stone-700)

Replace any remaining teal/indigo/blue references in card styles.

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite card component styles for stone palette"
```

---

### Task 8: Rewrite Form and Input Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (form section)
- Modify: `index.css` (Tailwind source, base layer form overrides)

**Step 1: Read form styles in both files**

Read the form-related styles in `public/index.css` and the `@layer base` section in `index.css`.

**Step 2: Update form styles**

In `index.css` (Tailwind source), update the `@layer base` form overrides:
- `border-color: #e7e5e4` (stone-200 instead of gray #6b7280)

In `public/index.css`, update any form-specific styles:
- Label text: `font-family: 'IBM Plex Mono', monospace; text-transform: uppercase; letter-spacing: 0.05em; font-size: 0.75rem; font-weight: 600; color: #78716c;` (stone-500)
- Input focus ring: replace any indigo/blue focus with amber-600 `#d97706`
- Input background: `#fafaf9` (stone-50)
- Checkbox accent: `#44403c` (stone-700)

**Step 3: Commit**

```bash
git add public/index.css index.css
git commit -m "feat: rewrite form and input styles for stone palette"
```

---

### Task 9: Rewrite Tooltip, Scrollbar, and Focus Ring Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (tooltip, scrollbar, focus sections)

**Step 1: Read relevant sections**

Search `public/index.css` for tooltip, scrollbar, and focus-related styles.

**Step 2: Update styles**

**Tooltips:**
- Background: `#1c1917` (stone-900)
- Text: white, `font-family: 'IBM Plex Mono', monospace; font-size: 0.75rem;`
- Keep arrow indicator and 300ms delay

**Scrollbars:**
- Webkit scrollbar width: 6px
- Track: transparent
- Thumb: `#d6d3d1` (stone-300), hover `#a8a29e` (stone-400)

**Focus rings:**
- Replace any teal/indigo/blue focus outlines with `#a8a29e` (stone-400) for general focus
- Interactive element focus: `#d97706` (amber-600) ring

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite tooltip, scrollbar, and focus styles"
```

---

### Task 10: Rewrite Lightbox Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (lightbox section)

**Step 1: Read lightbox styles**

Search for lightbox-related styles in `public/index.css`.

**Step 2: Update styles**

- Backdrop: `rgba(28, 25, 23, 0.95)` (stone-900 at 95% — likely already this from mahpastes influence)
- Controls bar background: `#292524` (stone-800)
- Icon buttons: white, hover with stone-400 background
- Focus rings: `#a8a29e` (stone-400) with 2px offset
- Replace any teal/indigo accent colors in lightbox controls with amber or stone equivalents

**Step 3: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite lightbox styles for stone palette"
```

---

### Task 11: Rewrite Dashboard Activity and Compare Styles

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (dashboard activity ~lines 1099-1117, compare ~lines 1275-1505)

**Step 1: Read both sections**

Read: `public/index.css` (lines 1090-1130 and lines 1270-1510)

**Step 2: Update dashboard activity indicator colors**

Replace entity-type colors with stone-adjacent muted variants:
```css
--resource: #b45309;  /* amber-700 (was blue) */
--note: #78716c;      /* stone-500 (was purple) */
--group: #a8a29e;     /* stone-400 (was amber) */
--tag: #57534e;       /* stone-600 (was emerald) */
```

**Step 3: Update compare page styles**

The compare page uses red for "old" and green for "new" — this is semantic and functional. Keep the red/green scheme but use slightly more muted values:
- Old: bg `#fef2f2`, text `#991b1b`, border `#fca5a5` — keep as-is (already appropriate)
- New: bg `#f0fdf4`, text `#166534`, border `#86efac` — keep as-is
- Replace any teal/indigo accent colors in compare controls with amber/stone

**Step 4: Commit**

```bash
git add public/index.css
git commit -m "feat: rewrite dashboard activity and compare styles"
```

---

### Task 12: Rewrite jsonTable.css

**Agent:** Frontend Lead
**Files:**
- Modify: `public/jsonTable.css`

**Step 1: Read jsonTable.css**

Read: `public/jsonTable.css`

**Step 2: Replace colors**

| Old | New | Element |
|-----|-----|---------|
| `#a7a7a7` (border gray) | `#e7e5e4` (stone-200) | Table borders |
| `#ececec` (header bg) | `#f5f5f4` (stone-100) | Table headers |
| `#fff` (cell bg) | `#ffffff` | Keep white |
| `#fbfde5` (odd row yellow) | `#fafaf9` (stone-50) | Odd row stripe |
| `coral` (toggler border) | `#b45309` (amber-700) | Toggler border |
| `#fddea2` (hover amber) | `#f5f5f4` (stone-100) | Row hover |

Add `font-family: 'IBM Plex Mono', monospace;` to the table base styles.

**Step 3: Commit**

```bash
git add public/jsonTable.css
git commit -m "feat: rewrite JSON table styles for stone palette"
```

---

### Task 13: Rewrite Remaining Component Styles in index.css

**Agent:** Frontend Lead
**Files:**
- Modify: `public/index.css` (bulk editor, pagination, sidebar, plugin modal, download cockpit, paste upload, entity picker, and any other remaining sections)

**Step 1: Search for ALL remaining teal/indigo/blue color values**

Run a search across `public/index.css` for: `#0d9488`, `#0f766e`, `#14b8a6`, `#2dd4bf`, `teal`, `#4338ca`, `#4f46e5`, `#6366f1`, `indigo`, `#3b82f6`, `#2563eb`, `#1d4ed8`, `blue`, `#10b981`, `#059669`, `emerald`, any `rgba(13, 148, 136` patterns.

**Step 2: Replace each occurrence**

For each remaining occurrence, apply the mapping:
- Teal/cyan → amber accent or stone (depending on context: interactive = amber, decorative = stone)
- Indigo → amber accent
- Blue → amber accent or stone
- Emerald/green → amber accent (for actions) or keep green (for semantic success indicators)

**Step 3: Run CSS build to verify no errors**

```bash
npm run build-css
```

**Step 4: Commit**

```bash
git add public/index.css
git commit -m "feat: eliminate all remaining teal/indigo/blue color references"
```

---

### Task 14: Build and Visual Smoke Test

**Agent:** Frontend Lead
**Files:** None (verification only)

**Step 1: Full build**

```bash
npm run build
```

**Step 2: Start ephemeral server and verify**

```bash
./mahresources -ephemeral -bind-address=:8181
```

Open in browser. Verify:
- Header background is warm neutral (not teal)
- Nav links use IBM Plex Mono, uppercase
- Hover/active states are amber, not teal
- Cards have stone borders
- Badges use muted semantic colors
- No visible teal/cyan anywhere

**Step 3: UX Lead Review**

UX Lead reviews the foundation CSS changes for:
- WCAG AA contrast compliance on all badge text/background combinations
- Focus ring visibility
- Tooltip readability
- Overall mood matches mahpastes aesthetic

**Step 4: Commit any fixes from review**

---

## Phase 2: Reference Pages

### Task 15: Update Dashboard Template

**Agent:** Frontend Lead
**Files:**
- Modify: `templates/dashboard.tpl`

**Step 1: Read dashboard.tpl**

Read: `templates/dashboard.tpl`

**Step 2: Apply class mapping**

Replace ALL inline Tailwind color classes using the Class Mapping Reference above:
- `text-gray-*` → `text-stone-*`
- `bg-gray-*` → `bg-stone-*`
- `border-gray-*` → `border-stone-*`
- `text-indigo-*` → `text-amber-*` (per mapping)
- `bg-indigo-*` → `bg-amber-*` (per mapping)
- `bg-blue-*` → `bg-amber-*` (per mapping)
- `text-blue-*` → `text-amber-*` (per mapping)
- `ring-indigo-*` → `ring-amber-*` (per mapping)
- `ring-blue-*` → `ring-amber-*` (per mapping)
- `bg-green-*` → `bg-amber-*` (for action buttons, per mapping)
- `focus:ring-indigo-*` → `focus:ring-amber-*`
- `focus:border-indigo-*` → `focus:border-amber-*`

Add `font-mono` to: labels, metadata, counts, timestamps, section headings
Add `font-sans` to: description text, prose content

**Step 3: Commit**

```bash
git add templates/dashboard.tpl
git commit -m "feat: migrate dashboard template to stone/amber design"
```

---

### Task 16: Update List Resources Template + Resource Card Partial

**Agent:** Frontend Lead
**Files:**
- Modify: `templates/listResources.tpl`
- Modify: `templates/partials/resource.tpl`

**Step 1: Read both files**

Read: `templates/listResources.tpl` and `templates/partials/resource.tpl`

**Step 2: Apply class mapping to listResources.tpl**

Same class mapping as Task 15. Pay special attention to:
- Bulk editor toolbar classes
- Filter form classes
- Grid container classes

**Step 3: Apply class mapping to resource.tpl**

This is the card component used everywhere. Update:
- Card border/bg/shadow classes
- Title text classes → add `font-mono`
- Meta text classes → add `font-mono`
- Description → add `font-sans`
- Badge classes (should defer to CSS classes, but replace any inline color overrides)
- Action button classes
- Selection checkbox accent

**Step 4: Commit**

```bash
git add templates/listResources.tpl templates/partials/resource.tpl
git commit -m "feat: migrate list resources and resource card to stone/amber design"
```

---

### Task 17: Visual Verification of Reference Pages

**Agent:** Frontend Lead + UX Lead
**Files:** None (verification only)

**Step 1: Build and start server**

```bash
npm run build && ./mahresources -ephemeral -bind-address=:8181
```

**Step 2: Verify dashboard**

- All sections use stone palette
- Activity feed colors are muted
- Card components render correctly
- Typography is correct (mono for UI, sans for prose)
- No stray teal/indigo/blue

**Step 3: Verify list resources**

- Grid cards render with stone borders
- Badges use correct semantic colors
- Bulk editor toolbar uses stone/amber
- Pagination uses stone/amber
- Search form uses stone inputs with amber focus

**Step 4: UX Lead accessibility check**

- Run axe DevTools or similar on both pages
- Verify all contrast ratios meet WCAG AA
- Verify focus rings are visible
- Verify keyboard navigation works

**Step 5: Fix any issues found, commit**

---

## Phase 3: Propagation (Parallel)

> All engineers work simultaneously on their assigned files. Each follows the Class Mapping Reference above exactly. Each uses the `/frontend-design` skill. When uncertain, ask the Frontend Lead or UX Lead before proceeding.

### Task 18: Engineer 1 — Create/Edit Page Templates

**Agent:** Engineer 1
**Files:**
- Modify: `templates/createResource.tpl`
- Modify: `templates/createNote.tpl`
- Modify: `templates/createGroup.tpl`
- Modify: `templates/createTag.tpl`
- Modify: `templates/createCategory.tpl`
- Modify: `templates/createResourceCategory.tpl`
- Modify: `templates/createQuery.tpl`
- Modify: `templates/createRelation.tpl`
- Modify: `templates/createRelationType.tpl`
- Modify: `templates/createNoteType.tpl`

**Step 1: Read each file**

Read all 10 templates.

**Step 2: Apply class mapping to each**

For each template, apply the full Class Mapping Reference:
- All `gray` → `stone`
- All `indigo`/`blue` interactive → `amber`
- All `green` actions → `amber`
- Add `font-mono` to labels, form field labels, button text
- Add `font-sans` to textarea content, description fields, help text
- Red danger states: keep red, adjust per mapping

These are form-heavy pages. Key areas:
- Form labels and field groups
- Submit/cancel buttons
- Autocompleter dropdowns
- Free fields (dynamic metadata)
- Error message styling
- File upload areas

**Step 3: Commit**

```bash
git add templates/create*.tpl
git commit -m "feat: migrate all create/edit page templates to stone/amber design"
```

---

### Task 19: Engineer 2 — Display Page Templates

**Agent:** Engineer 2
**Files:**
- Modify: `templates/displayResource.tpl`
- Modify: `templates/displayNote.tpl`
- Modify: `templates/displayNoteText.tpl`
- Modify: `templates/displayGroup.tpl`
- Modify: `templates/displayGroupTree.tpl`
- Modify: `templates/displayTag.tpl`
- Modify: `templates/displayCategory.tpl`
- Modify: `templates/displayResourceCategory.tpl`
- Modify: `templates/displayQuery.tpl`
- Modify: `templates/displayRelation.tpl`
- Modify: `templates/displayRelationType.tpl`
- Modify: `templates/displayNoteType.tpl`
- Modify: `templates/displaySeries.tpl`
- Modify: `templates/displayLog.tpl`

**Step 1: Read each file**

Read all 14 templates.

**Step 2: Apply class mapping to each**

Same mapping. Display pages typically have:
- Detail header with title and metadata
- Sidebar with related entities, timestamps, actions
- Description/content area (use `font-sans` for prose)
- Related entity lists (cards, badges)
- Action buttons (edit, delete, merge)
- Tag/group/relation displays

**Step 3: Commit**

```bash
git add templates/display*.tpl
git commit -m "feat: migrate all display page templates to stone/amber design"
```

---

### Task 20: Engineer 3 — List Page Templates

**Agent:** Engineer 3
**Files:**
- Modify: `templates/listNotes.tpl`
- Modify: `templates/listGroups.tpl`
- Modify: `templates/listGroupsText.tpl`
- Modify: `templates/listTags.tpl`
- Modify: `templates/listCategories.tpl`
- Modify: `templates/listResourceCategories.tpl`
- Modify: `templates/listQueries.tpl`
- Modify: `templates/listRelations.tpl`
- Modify: `templates/listRelationTypes.tpl`
- Modify: `templates/listNoteTypes.tpl`
- Modify: `templates/listLogs.tpl`
- Modify: `templates/listResourcesSimple.tpl`
- Modify: `templates/listResourcesDetails.tpl`

**Step 1: Read each file**

Read all 13 templates.

**Step 2: Apply class mapping to each**

Same mapping. List pages typically have:
- Search/filter forms
- Grid or list layouts
- Card components (which inherit from partials)
- Pagination
- Bulk editor toolbars (some pages)
- Sort controls

**Step 3: Commit**

```bash
git add templates/list*.tpl
git commit -m "feat: migrate all list page templates to stone/amber design"
```

---

### Task 21: Engineer 4 — Shared Partials and Form Components

**Agent:** Engineer 4
**Files:**
- Modify: `templates/partials/menu.tpl`
- Modify: `templates/partials/breadcrumb.tpl`
- Modify: `templates/partials/pagination.tpl`
- Modify: `templates/partials/title.tpl`
- Modify: `templates/partials/subtitle.tpl`
- Modify: `templates/partials/sideTitle.tpl`
- Modify: `templates/partials/globalSearch.tpl`
- Modify: `templates/partials/description.tpl`
- Modify: `templates/partials/bulkEditorResource.tpl`
- Modify: `templates/partials/bulkEditorNote.tpl`
- Modify: `templates/partials/bulkEditorTag.tpl`
- Modify: `templates/partials/bulkEditorGroup.tpl`
- Modify: `templates/partials/form/textInput.tpl`
- Modify: `templates/partials/form/checkboxInput.tpl`
- Modify: `templates/partials/form/selectInput.tpl`
- Modify: `templates/partials/form/dateInput.tpl`
- Modify: `templates/partials/form/autocompleter.tpl`
- Modify: `templates/partials/form/createFormTextInput.tpl`
- Modify: `templates/partials/form/createFormTextareaInput.tpl`
- Modify: `templates/partials/form/createFormCodeEditorInput.tpl`
- Modify: `templates/partials/form/createFormSubmit.tpl`
- Modify: `templates/partials/form/searchFormResource.tpl`
- Modify: `templates/partials/form/searchButton.tpl`
- Modify: `templates/partials/form/addButton.tpl`
- Modify: `templates/partials/form/deleteButton.tpl`
- Modify: `templates/partials/form/multiSortInput.tpl`
- Modify: `templates/partials/form/freeFields.tpl`
- Modify: `templates/partials/form/formParts/errorMessage.tpl`
- Modify: `templates/partials/form/formParts/dropDownResults.tpl`
- Modify: `templates/partials/form/formParts/dropDownSelectedResults.tpl`
- Modify: `templates/partials/form/formParts/connected/selectAllButton.tpl`
- Modify: `templates/partials/form/formParts/connected/deselectButton.tpl`
- Modify: `templates/partials/form/formParts/connected/selectedIds.tpl`
- Modify: `templates/partials/seeAll.tpl`
- Modify: `templates/partials/json.tpl`
- Modify: `templates/partials/ownerDisplay.tpl`
- Modify: `templates/partials/tagList.tpl`
- Modify: `templates/partials/avatar.tpl`
- Modify: `templates/partials/noteShare.tpl`

**Step 1: Read each file**

Read all files. These are the most impactful files — they're included in nearly every page.

**Step 2: Apply class mapping to each**

Same mapping. Pay particular attention to:
- `menu.tpl`: Nav links need `font-mono`, uppercase tracking. Active states use amber.
- `breadcrumb.tpl`: `font-mono`, stone text colors
- `pagination.tpl`: stone borders, amber active page
- `globalSearch.tpl`: stone input, amber focus, `font-mono`
- `description.tpl`: `font-sans` for prose content
- `bulkEditor*.tpl`: stone bg, amber active toggles
- All form partials: stone borders, amber focus, `font-mono` labels
- `searchButton.tpl` / `addButton.tpl`: amber primary styling
- `deleteButton.tpl`: keep red danger styling
- `errorMessage.tpl`: keep red error styling

**Step 3: Commit**

```bash
git add templates/partials/menu.tpl templates/partials/breadcrumb.tpl templates/partials/pagination.tpl templates/partials/title.tpl templates/partials/subtitle.tpl templates/partials/sideTitle.tpl templates/partials/globalSearch.tpl templates/partials/description.tpl templates/partials/bulkEditor*.tpl templates/partials/form/ templates/partials/seeAll.tpl templates/partials/json.tpl templates/partials/ownerDisplay.tpl templates/partials/tagList.tpl templates/partials/avatar.tpl templates/partials/noteShare.tpl
git commit -m "feat: migrate shared partials and form components to stone/amber design"
```

---

### Task 22: Engineer 5 — Complex Component Partials

**Agent:** Engineer 5
**Files:**
- Modify: `templates/partials/lightbox.tpl`
- Modify: `templates/partials/pluginActionModal.tpl`
- Modify: `templates/partials/pluginActionsCard.tpl`
- Modify: `templates/partials/pluginActionsBulk.tpl`
- Modify: `templates/partials/pluginActionsSidebar.tpl`
- Modify: `templates/partials/blockEditor.tpl`
- Modify: `templates/partials/blocks/sharedBlock.tpl`
- Modify: `templates/partials/compareImage.tpl`
- Modify: `templates/partials/compareText.tpl`
- Modify: `templates/partials/comparePdf.tpl`
- Modify: `templates/partials/compareBinary.tpl`
- Modify: `templates/partials/versionPanel.tpl`
- Modify: `templates/partials/downloadCockpit.tpl`
- Modify: `templates/partials/pasteUpload.tpl`
- Modify: `templates/partials/entityPicker.tpl`

**Step 1: Read each file**

Read all 15 templates.

**Step 2: Apply class mapping to each**

Same mapping. Special considerations:
- `lightbox.tpl`: Dark UI — use stone-800/900 backgrounds, white text, stone-400 focus rings
- `pluginActionModal.tpl`: Modal overlay, stone backdrop, amber action buttons
- `blockEditor.tpl`: Editor UI needs `font-mono` for controls, `font-sans` for content editing
- `compare*.tpl`: Keep red/green semantic colors for old/new, but update any accent/action buttons
- `downloadCockpit.tpl`: Progress indicators, stone palette
- `pasteUpload.tpl`: Drop zone styling, stone borders, amber accent for active state
- `entityPicker.tpl`: Modal with search, stone/amber styling

**Step 3: Commit**

```bash
git add templates/partials/lightbox.tpl templates/partials/pluginAction*.tpl templates/partials/blockEditor.tpl templates/partials/blocks/ templates/partials/compare*.tpl templates/partials/versionPanel.tpl templates/partials/downloadCockpit.tpl templates/partials/pasteUpload.tpl templates/partials/entityPicker.tpl
git commit -m "feat: migrate complex component partials to stone/amber design"
```

---

### Task 23: Engineer 6 — Layout Templates, Entity Card Partials, and Remaining Files

**Agent:** Engineer 6
**Files:**
- Modify: `templates/layouts/base.tpl` (inline classes only — font import already added in Task 1)
- Modify: `templates/layouts/gallery.tpl`
- Modify: `templates/layouts/bodyOnly.tpl`
- Modify: `templates/shared/base.tpl`
- Modify: `templates/shared/displayNote.tpl`
- Modify: `templates/partials/note.tpl`
- Modify: `templates/partials/group.tpl`
- Modify: `templates/partials/tag.tpl`
- Modify: `templates/partials/category.tpl`
- Modify: `templates/partials/relation.tpl`
- Modify: `templates/partials/relation_reverse.tpl`
- Modify: `templates/partials/relationType.tpl`
- Modify: `templates/partials/query.tpl`
- Modify: `templates/error.tpl`
- Modify: `templates/compare.tpl`
- Modify: `templates/managePlugins.tpl`
- Modify: `templates/pluginPage.tpl`
- Modify: `templates/partials/svg/home.tpl`
- Modify: `templates/partials/svg/arrow.tpl`

**Step 1: Read each file**

Read all 19 files.

**Step 2: Apply class mapping to each**

Same mapping. Key notes:
- `base.tpl`: Update skip-to-content link (`text-indigo-600` → `text-amber-700`), timestamp styling, body background, any remaining color classes
- `gallery.tpl`: Grid container styling
- Entity card partials (`note.tpl`, `group.tpl`, etc.): Same card patterns as `resource.tpl` — `font-mono` for titles/meta, `font-sans` for descriptions, stone colors
- `error.tpl`: Keep red for error state, but use stone for surrounding chrome
- SVG partials: May have `fill` or `stroke` color classes — update to stone/amber

**Step 3: Commit**

```bash
git add templates/layouts/ templates/shared/ templates/partials/note.tpl templates/partials/group.tpl templates/partials/tag.tpl templates/partials/category.tpl templates/partials/relation.tpl templates/partials/relation_reverse.tpl templates/partials/relationType.tpl templates/partials/query.tpl templates/error.tpl templates/compare.tpl templates/managePlugins.tpl templates/pluginPage.tpl templates/partials/svg/
git commit -m "feat: migrate layouts, entity cards, and remaining templates to stone/amber design"
```

---

## Phase 4: Polish

### Task 24: Full Build and E2E Test Run

**Agent:** Frontend Lead
**Files:** None (verification only)

**Step 1: Full build**

```bash
npm run build
```

Verify no build errors.

**Step 2: Run Go unit tests**

```bash
go test ./...
```

Verify all pass (CSS changes shouldn't affect Go tests, but verify).

**Step 3: Run E2E tests**

```bash
cd e2e && npm run test:with-server
```

Fix any failures. Common issues:
- Tests that assert on specific CSS colors (unlikely but possible)
- Tests that match text content that changed (shouldn't happen — we're only changing classes)
- Selector changes if any class names were used as selectors

**Step 4: Run accessibility tests**

```bash
cd e2e && npm run test:with-server:a11y
```

Fix any a11y violations. Common issues:
- Contrast ratio failures on badge text/background
- Missing focus indicators
- Color-only information differentiation

**Step 5: Commit fixes**

```bash
git add -A
git commit -m "fix: resolve test and accessibility issues from design migration"
```

---

### Task 25: UX Lead Full Visual Sweep

**Agent:** UX Lead
**Files:** None (review only) — or fixes as needed

**Step 1: Start server with seed data**

```bash
npm run build && ./mahresources -ephemeral -bind-address=:8181
```

**Step 2: Check every page type**

Visit each page type and verify:
- [ ] Dashboard — cards, activity feed, sidebar
- [ ] List Resources — grid, bulk editor, pagination
- [ ] List Resources Simple — gallery view
- [ ] List Resources Details — detail list view
- [ ] Display Resource — detail, sidebar, tags, lightbox
- [ ] Create Resource — form, file upload, autocompleter
- [ ] List Notes — note cards
- [ ] Display Note — note content, prose styling
- [ ] Create Note — form with code editor
- [ ] List Groups — group cards
- [ ] Display Group — hierarchical tree
- [ ] Create Group — form
- [ ] List Tags — tag list
- [ ] Display Tag — tag detail
- [ ] Create Tag — form
- [ ] List Categories — category list
- [ ] List Queries — saved searches
- [ ] Display Query — query detail
- [ ] List Relations — relation cards
- [ ] Compare page — side-by-side comparison
- [ ] Plugin management — plugin list
- [ ] Error page — error display
- [ ] Global search (Cmd+K)
- [ ] Lightbox — media viewer
- [ ] Plugin action modal

For each page verify:
1. No stray teal/cyan/indigo colors
2. Typography is correct (mono UI, sans prose)
3. Badges use correct semantic colors
4. Buttons use correct variants
5. Forms have correct focus rings
6. Overall mood is calm, professional, monochrome

**Step 3: Log issues**

Document any inconsistencies with file path and line number.

**Step 4: Fix issues and commit**

---

### Task 26: Frontend Lead Final Cleanup

**Agent:** Frontend Lead
**Files:** Various (from UX Lead review)

**Step 1: Fix all issues from UX Lead review**

Address each documented issue.

**Step 2: Search for ANY remaining old color references**

Run across ALL template and CSS files:
- Search for: `teal`, `cyan`, `indigo` (in class names)
- Search for: `#0d9488`, `#0f766e`, `#14b8a6`, `#e8fafa`, `#c5f0f0` (old CSS values)
- Search for: `#4338ca`, `#4f46e5`, `#6366f1` (old indigo values)
- Search for: `#3b82f6`, `#2563eb` (old blue values)

Replace any remaining occurrences.

**Step 3: Final build and test**

```bash
npm run build
cd e2e && npm run test:with-server
cd e2e && npm run test:with-server:a11y
```

All must pass.

**Step 4: Commit**

```bash
git add -A
git commit -m "chore: final cleanup — eliminate all legacy color references"
```

---

## File Summary

### Files Modified (count by agent)

| Agent | Files | Type |
|-------|-------|------|
| Frontend Lead | 5 | CSS + config + reference templates |
| Engineer 1 | 10 | Create/edit templates |
| Engineer 2 | 14 | Display templates |
| Engineer 3 | 13 | List templates |
| Engineer 4 | 39 | Shared partials + form components |
| Engineer 5 | 15 | Complex component partials |
| Engineer 6 | 19 | Layouts, entity cards, remaining |
| **Total** | **~115** | |

### Files NOT Modified (out of scope)

- `src/*.js` — No JavaScript changes
- `vite.config.js` — No build config changes
- `postcss.config.js` — No PostCSS changes
- `*.go` — No backend changes
- `e2e/` — Tests run but not modified (unless selectors break)
