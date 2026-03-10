# Design Language Migration: mahpastes → mahresources

**Date:** 2026-03-10
**Status:** Approved

## Goal

Adopt mahpastes' aesthetic DNA (stone palette, IBM Plex Mono, minimalist mood) across every component in mahresources, while preserving mahresources' layout structure (horizontal nav, sidebar, grid).

## Design Tokens

### Color Palette — Stone Base

| Token | Value | Usage |
|-------|-------|-------|
| `--stone-50` | `#fafaf9` | Page backgrounds |
| `--stone-100` | `#f5f5f4` | Card backgrounds, hover states |
| `--stone-200` | `#e7e5e4` | Borders, dividers |
| `--stone-300` | `#d6d3d1` | Disabled states, subtle borders |
| `--stone-400` | `#a8a29e` | Muted text, placeholders |
| `--stone-500` | `#78716c` | Secondary text |
| `--stone-600` | `#57534e` | Body text |
| `--stone-700` | `#44403c` | Headings, form elements |
| `--stone-800` | `#292524` | Primary text, dark accents |
| `--stone-900` | `#1c1917` | Darkest — modals, overlays |

### Warm Accent (interactive elements)

| Token | Value | Usage |
|-------|-------|-------|
| `--accent` | `#b45309` | Primary buttons, active nav (amber-700) |
| `--accent-hover` | `#92400e` | Hover state (amber-800) |
| `--accent-light` | `#fef3c7` | Accent backgrounds (amber-100) |
| `--accent-focus` | `#d97706` | Focus rings (amber-600) |

### Muted Semantic Colors (WCAG AA — 4.5:1+ contrast)

| Entity | Background | Text | Border |
|--------|-----------|------|--------|
| Category | `#f5f5f4` (stone-100) | `#78716c` (stone-500) | `#d6d3d1` (stone-300) |
| Tag | `#fef9ee` (warm cream) | `#92400e` (amber-800) | `#fde68a` (amber-200) |
| Relation | `#f5f3ff` (cool lavender) | `#5b21b6` (violet-800) | `#ddd6fe` (violet-200) |
| Note Type | `#f0fdf4` (sage tint) | `#166534` (green-800) | `#bbf7d0` (green-200) |

### Typography

```
Imports:
  IBM Plex Mono: 400, 500, 600
  IBM Plex Sans: 400, 500, 600

--font-ui: 'IBM Plex Mono', monospace    → nav, labels, metadata, buttons, badges
--font-prose: 'IBM Plex Sans', sans-serif → descriptions, notes, long text, .prose
```

Size scale compact: `text-xs` and `text-sm` dominant for UI, standard sizes for prose.

### Shadows, Radii, Transitions

```
--shadow-sm: 0 1px 2px rgba(0, 0, 0, 0.04)
--shadow-md: 0 4px 12px rgba(0, 0, 0, 0.08)
--shadow-lg: 0 8px 24px rgba(0, 0, 0, 0.12)

--radius-sm: 6px
--radius-md: 8px
--radius-lg: 12px

--transition-fast: 120ms ease
--transition-normal: 200ms ease
```

## Component Patterns

### Navigation Bar
- Background: `stone-50`, subtle bottom border (`stone-200`), no gradient
- Font: IBM Plex Mono, `text-xs`, uppercase tracking
- Active link: warm accent text (`--accent`) with `accent-light` underline
- Hover: `stone-700` text, `stone-100` bg
- Dropdowns: `stone-50` bg, `stone-200` border, `shadow-md`
- Global search: `stone-100` input bg, `stone-300` border, mono font

### Cards
- Background: white, `stone-200` border, `radius-md`
- Hover: `shadow-md`, border shifts to `stone-300`
- Title: IBM Plex Mono, `text-sm`, `font-semibold`, `stone-800`
- Meta: IBM Plex Mono, `text-xs`, `stone-400`
- Description: IBM Plex Sans, `text-sm`, `stone-600`, line-clamped
- Tags row: muted semantic badges
- Actions row: top border `stone-100`, icon buttons `stone-500`, hover `stone-800`
- Selection checkbox: accent color `stone-700`

### Buttons

| Variant | Background | Text | Border | Hover |
|---------|-----------|------|--------|-------|
| Primary | `--accent` | white | none | `--accent-hover` |
| Secondary | white | `stone-700` | `stone-200` | `stone-100` bg |
| Danger | white | `red-700` | `red-200` | `red-50` bg |
| Ghost | transparent | `stone-500` | none | `stone-100` bg |

All: IBM Plex Mono, `text-xs`, `font-medium`, `radius-sm`, `transition-fast`.

### Forms
- Labels: IBM Plex Mono, `text-xs`, `font-semibold`, `stone-500`, uppercase tracking
- Inputs: `stone-200` border, `stone-50` bg, `radius-sm`, focus ring `--accent-focus`
- Textareas: same, IBM Plex Sans for content
- Selects: custom arrow SVG in `stone-400`
- Checkboxes: accent color `stone-700`, focus ring `--accent-focus`
- Autocompleter: white bg, `shadow-lg`, `stone-200` border, hover `stone-100`

### Badges
- Rounded-full pill, `text-xs`, IBM Plex Mono, `font-medium`
- Per-entity colors from semantic token table
- Padding: `px-2 py-0.5`

### Tooltips
- `stone-900` bg, white text, `text-xs`, mono
- Arrow indicator, 300ms delay

### Pagination
- `stone-200` borders, `stone-500` text
- Active page: `--accent` bg, white text
- Hover: `stone-100` bg

### Bulk Editor Toolbar
- `stone-100` bg, `stone-200` top/bottom border
- Toggle buttons: `stone-700` text, active uses `--accent-light` bg + `--accent` text
- Sticky positioning preserved

### Lightbox
- Backdrop: `stone-900` at 95% opacity
- Controls bar: `stone-800` bg, white icon buttons
- Focus rings: `stone-400` with offset

### Sidebar
- `stone-50` bg sections, `stone-200` borders
- Section titles: IBM Plex Mono, `text-xs`, uppercase, `stone-400`
- Content: `text-sm`, `stone-600`

### Scrollbars
- Minimal webkit scrollbars (6px width, transparent track)
- Thumb: `stone-300`, hover `stone-400`

## Multi-Agent Team Structure

| Role | Responsibility |
|------|---------------|
| UX Lead | Defines tokens, validates WCAG, reviews every page for visual consistency and a11y |
| Frontend Lead | Token infrastructure (Tailwind config, CSS vars, fonts), builds reference pages, code reviews |
| Engineer 1 | Create/edit pages (9 templates) |
| Engineer 2 | Display pages (11 templates) |
| Engineer 3 | List pages (11 templates) |
| Engineer 4 | Shared/partials — menu, breadcrumb, pagination, form partials, bulk editors, global search |
| Engineer 5 | Complex components — lightbox, plugin modal, block editor, compare views, download cockpit, paste upload, entity picker |
| Engineer 6 | Layout + CSS foundation — base.tpl, gallery.tpl, index.css, jsonTable.css, card/badge/button base styles |

### Workflow Phases

1. **Foundation** (sequential): Frontend Lead builds token infrastructure → UX Lead validates
2. **Reference Pages** (sequential): Frontend Lead builds Dashboard + List Resources → UX Lead reviews
3. **Propagation** (parallel): Engineers 1-6 work simultaneously → Leads review
4. **Polish** (sequential): UX Lead full sweep → Frontend Lead final cleanup

### Agent Rules
- Engineers use `/frontend-design` skill
- Engineers ask leads on ambiguity
- UX Lead has final say on visual/a11y
- Frontend Lead has final say on code quality

## Scope

### In Scope
- All CSS custom properties and Tailwind config
- Font imports (IBM Plex Mono + Sans)
- Every `.tpl` template file — inline Tailwind classes updated
- `index.css` — custom component styles rewritten
- `jsonTable.css` — restyled
- Scrollbar, tooltip, focus ring styling

### Out of Scope
- No JavaScript changes
- No layout restructuring
- No template structure changes (only classes)
- No new components
- No Vite/build config changes
- No Go backend changes

## Success Criteria

1. Every page uses the stone palette — no stray teal/cyan
2. IBM Plex Mono on all UI chrome, IBM Plex Sans on all prose
3. All interactive elements use the warm accent
4. Semantic badges muted but WCAG AA compliant (4.5:1+)
5. Visual consistency across all pages — no "seams" between engineers
6. All existing E2E and a11y tests pass
7. Overall mood: calm, professional, monochrome — same family as mahpastes
