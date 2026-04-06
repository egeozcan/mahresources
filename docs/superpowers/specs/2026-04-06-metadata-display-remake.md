# Metadata Display Remake

Redesign the unstructured metadata display on entity detail pages (groups, resources, notes) to use type-aware rendering, informative collapsed states, and styling consistent with the app's amber/stone design language.

## Files to modify

- `src/tableMaker.js` — Core rendering engine, add type detection and new element generation
- `src/webcomponents/expandabletext.js` — Restyle and improve UX
- `public/jsonTable.css` — Complete restyle to match app theme
- `templates/partials/json.tpl` — Replace fullscreen button with icon toggle, restructure header

## 1. Type-Aware Value Rendering

Add type detection to `renderJsonTable` in `tableMaker.js`. Each type gets distinct visual treatment:

### Detection rules (checked in order)

| Type | Detection | Rendering |
|---|---|---|
| Date/Timestamp | Number where `val > 1e9 && val < 1e13`, or valid ISO 8601 string. Values > 1e11 are milliseconds (divide by 1000), values between 1e9 and 1e11 are seconds. | Formatted date string (e.g. `Jun 14, 2021`) in `stone-500` color |
| Boolean | `typeof === "boolean"` | Colored dot (green `#10b981` for true, gray `#d6d3d1` for false) + "yes"/"no" text |
| Boolean-like | Value is `0` or `1` AND key matches boolean pattern | Same as boolean |
| URL | String starting with `http://` or `https://` | Truncated clickable link in amber (`#b45309`), shows domain + path |
| ID | Key is `id`, `parent`, or ends with `_id` | Muted monospace in `stone-400`, `0.75rem` |
| Empty array | `Array.isArray(val) && val.length === 0` | Collapse button: `empty — show` |
| Non-empty array | `Array.isArray(val) && val.length > 0` | Collapse button: `N items — show` |
| Empty object | `typeof === "object" && Object.keys(val).length === 0` | Collapse button: `empty — show` |
| Non-empty object | `typeof === "object" && Object.keys(val).length > 0` | Collapse button: `N keys — show` |
| Long string | `typeof === "string" && val.length > 30` | `<expandable-text>` component |
| data: URI image | String starting with `data:image` | `<img>` element (existing behavior) |
| Plain string/number | Everything else | Plain text, as-is |

### Boolean-like key patterns

Only convert `0`/`1` to boolean rendering when the key matches one of these patterns:
`active`, `enabled`, `disabled`, `visible`, `hidden`, `deleted`, `verified`, `published`, `is_*`, `has_*`, `can_*`, `show_*`

### Date formatting

Use `toLocaleDateString()` with `{ year: 'numeric', month: 'short', day: 'numeric' }` options. For timestamps that include time information (detected by being within the last 24 hours or having non-zero hours/minutes), also show time: `Jun 14, 2021, 3:45 PM`.

### URL truncation

Display format: strip protocol, show domain + first path segment. Full URL in `title` attribute and `href`. Example: `https://www.example.com/users/profile?id=5` renders as `example.com/users/profile...`

## 2. Expandable Text Redesign

Restyle `src/webcomponents/expandabletext.js`:

### Font and styling
- Font family: `'IBM Plex Mono', monospace` (replaces Arial)
- Text color: `#292524` (stone-800)

### Truncation
- First 30 characters + `...` ellipsis in `stone-400` color
- "show more" toggle link: `0.6875rem`, color `#b45309` (amber-600), no border/background, `font-weight: 500`
- Hover: underline
- When expanded: full text shown, toggle reads "show less"

### Copy button
- Small clipboard SVG icon (14x14), `stone-400` color
- Appears on hover (opacity 0 → 1, `120ms` transition)
- Hover: `stone-600`
- Positioned after the toggle link with `0.375rem` left margin

### Focus states
- All interactive elements: `outline: 2px solid #b45309; outline-offset: 2px`
- `:focus:not(:focus-visible)` removes outline for mouse clicks

## 3. Collapsed State Buttons

Replace emoji togglers (`➕`/`➖`) with styled text buttons:

### Button styling
- Font: IBM Plex Mono, `0.6875rem`
- Color: `stone-500` (`#78716c`)
- Background: `stone-100` (`#f5f5f4`)
- Border: `1px solid stone-200` (`#e7e5e4`)
- Border-radius: `6px`
- Padding: `0.1875rem 0.5rem`
- Cursor: pointer

### States
- Hover: border `stone-300`, background `stone-50`, color `stone-600`
- Expanded: background white, border `amber-600` (`#b45309`), color `amber-600`
- Focus-visible: `outline: 2px solid #b45309; outline-offset: 2px`

### Text content
- Empty array/object: `empty — show` / `empty — hide`
- Non-empty array: `N items — show` / `N items — hide`
- Non-empty object: `N keys — show` / `N keys — hide`

### Behavior
- Click to toggle expand/collapse (same as current)
- Shift-click to recursively expand/collapse all subtables (same as current)
- Keyboard: Enter/Space to toggle (same as current)

## 4. Table Styling

Replace `jsonTable.css` content with styles matching the app's design language:

### Table structure
- Font: IBM Plex Mono (no change)
- Border-collapse: collapse
- Width: 100%
- No outer border (table sits inside sidebar-group which provides context)

### Header cells (keys)
- Background: `stone-100` (`#f5f5f4`)
- Color: `stone-600` (`#57534e`)
- Font-size: `0.75rem`
- Font-weight: 500
- Padding: `0.5rem 0.75rem`
- Width: 35%
- Vertical-align: top
- Border-bottom: `1px solid stone-100`

### Value cells
- Background: white
- Color: `stone-900` (`#292524`)
- Font-size: `0.8125rem`
- Same padding as headers
- Vertical-align: top
- Border-bottom: `1px solid stone-100` (subtler than current `stone-200`)
- Position: relative (for copy tooltip positioning)

### Row hover
- Left border: `2px solid transparent` → `amber-600` (`#b45309`) on hover
- Background: both th and td shift to `stone-50` on row hover
- Transition: `120ms ease`

### Last row
- No bottom border on th/td

### Nested subtables
- Wrapped in a div with `0.375rem` top margin
- Subtable gets `1px solid stone-200` border and `4px` border-radius
- Subtable header cells use `stone-50` background (slightly lighter than parent)

### Array table alternating rows
- Even: white, Odd: `stone-50` (`#fafaf9`)

### Empty table
- "No data" message: `0.75rem`, monospace, uppercase, `letter-spacing: 0.025em`, `stone-400`, centered, `1rem` vertical padding

## 5. Fullscreen Toggle

### Template change (`json.tpl`)
Replace the full-width amber "Fullscreen" button with a header row containing the section title and an icon toggle:

```html
<div class="sidebar-header">
  <span class="sidebar-title">Meta Data</span>  <!-- from sideTitle -->
  <button class="expand-btn">
    <svg><!-- expand icon --></svg>
    Expand
  </button>
</div>
```

This means `json.tpl` will render its own header instead of relying on the separate `sideTitle.tpl` include. The parent templates (`displayGroup.tpl`, `displayResource.tpl`, `displayNote.tpl`) should be updated to remove the separate `sideTitle.tpl` include for the metadata section.

### Button styling
- Inline-flex, centered items, `0.375rem` gap
- Font: IBM Plex Mono, `0.75rem`
- Color: `stone-500`
- Background: transparent
- Border: `1px solid stone-300`
- Border-radius: `6px`
- Padding: `0.25rem 0.5rem`
- Hover: border and text turn `amber-600`
- Focus-visible: amber outline

### Icons
- Expand: four-corner outward arrows SVG (14x14)
- Minimize: four-corner inward arrows SVG (14x14)

### Fullscreen display
- Same `position: fixed` overlay, `z-index: 100` (current behavior)
- Background: white
- Padding: `1.5rem`
- Content centered with `max-width: 1200px; margin: 0 auto`
- Top bar with "Meta Data" title (left) and "Minimize" button (right)
- Border-bottom separator on the top bar: `1px solid stone-100`
- Body overflow hidden when expanded (existing behavior via Alpine `x-effect`)

## 6. Copy-on-Click Feedback

### Flash animation
- On click (existing event delegation), add class `copy-flash` to the clicked cell
- CSS animation: background flashes `amber-100` (`#fef3c7`) and fades to transparent over `300ms`
- Remove class after animation ends (via `animationend` event)

### Tooltip
- Absolutely positioned `div` inside the cell: `"Copied!"`
- Styled: `stone-700` background, white text, `0.6875rem` monospace, `4px` border-radius, `0.1875rem 0.5rem` padding
- Appears near top-right of the cell
- Fades in over `150ms`, stays for `1.5s`, then fades out
- `pointer-events: none`, `z-index: 10`

### Guards
- No copy trigger on button clicks or expandable-text interaction (existing behavior preserved)

## Non-goals

- No data grouping or reordering — fields display in the order they appear in the JSON
- No schema-driven rendering — this is for unstructured metadata only (schema-editor handles structured metadata separately)
- No changes to the metadata editing flow (`editMeta` handler)
- No changes to the Go backend or data model

## Accessibility

- All interactive elements (collapse buttons, expand toggle, expandable text toggle/copy) must be keyboard accessible
- Focus-visible outlines on all buttons (amber ring)
- `aria-expanded` on collapse buttons and expandable text toggle
- `aria-label` on icon-only buttons (copy, expand/minimize)
- Copy tooltip uses `role="status"` and `aria-live="polite"` for screen reader announcement
- Color is never the only indicator — boolean dots always paired with "yes"/"no" text
