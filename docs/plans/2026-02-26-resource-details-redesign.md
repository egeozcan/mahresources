# Resource Details Metadata Redesign

## Goal

Replace the flat key-value table on the resource detail page with a card-based metadata grid. Improve scannability by surfacing primary fields and hiding technical details behind a collapsible section.

## Field Grouping

**Primary fields (always visible):**
- Name + Original Name (side by side if both exist)
- Dimensions (Width x Height combined)
- Created (human-readable date)
- Updated (human-readable date)

**Technical details (collapsed `<details>`):**
- ID
- Hash + HashType (combined, e.g. "SHA1: fdaaa47cc3f...")
- Location
- Original Location
- Storage Location
- Description (omitted if empty)

Empty fields are omitted entirely in both sections.

## Card Visual Design

Each card:
- Label: small muted text above (`text-xs text-gray-500`)
- Value: regular weight text below (`text-sm`)
- Copy icon: clipboard icon, visible on hover, top-right corner. Copies raw value.
- Background: `bg-gray-50`, border `border border-gray-200`, `rounded-lg`, `px-4 py-3`
- Hover: border darkens to `border-gray-300`

Grid layout:
- `grid grid-cols-2 md:grid-cols-3 gap-3`
- Cards stretch to fill grid cells

Collapsed technical section:
- `<details>` with styled `<summary>`: "Technical Details" + chevron
- Same card grid inside
- Empty fields omitted

## Implementation

**Files changed:**
- `templates/displayResource.tpl` — Replace `{% include "partials/json.tpl" %}` for main metadata with card markup using Pongo2 template logic and direct `resource` field access.

**Files NOT changed:**
- `templates/partials/json.tpl` — Still used by other pages and the Meta Data sidebar section
- `src/tableMaker.js` — Untouched
- `public/jsonTable.css` — Untouched
- Sidebar, description, notes/groups, versions sections — Unchanged

**JavaScript:**
- No new JS files. Copy-on-hover uses existing clipboard utility from `src/index.js`.
- Minimal Alpine.js for copy feedback tooltip.

**Accessibility:**
- Semantic markup: `<dl>`, `<dt>`, `<dd>`
- Copy button: `aria-label="Copy value"`
- `<details>`/`<summary>` natively accessible
- Sufficient color contrast on all text

## Decisions

- Fullscreen toggle removed for this section (cards are already scannable)
- Card style over definition list or hybrid bar (cleaner departure from table)
- Hover-to-reveal copy icons (discoverable but uncluttered)
- Tailwind-only styling (consistent with project conventions)
