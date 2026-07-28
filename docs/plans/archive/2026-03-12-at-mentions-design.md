# @-Mentions in Description Fields

## Overview

Add @-mention autocomplete to description/text fields. Users type `@` followed by a search query, pick an entity from a dropdown, and a marker is inserted. On save, mentioned entities are added as relations. On render, markers become links (with thumbnails for resources).

## Marker Syntax

Format: `@[Type:ID:Display Name]`

Examples:
- `@[resource:42:photo.jpg]`
- `@[note:7:Meeting Notes]`
- `@[group:15:Project Alpha]`
- `@[tag:3:important]`

Rules:
- Type is lowercase entity type name (matches search API `type` field)
- ID is the numeric entity ID
- Display Name captured at mention time (ID is authoritative)
- Parsing splits on first two colons only (colons in names are fine)
- Stored as plain text in Description fields and NoteBlock Content JSON

No schema changes needed.

## Scope: Which Fields

- Description textareas on Notes, Groups, Resources
- NoteBlock text content

## Mentionable Types (Contextual)

Only entity types that have a relationship field on the edited entity:
- **Note form**: resource, group, tag
- **Group form**: resource, note, group, tag
- **Resource form**: note, group, tag

## Frontend: Autocomplete Component

New Alpine.js component `mentionTextarea`:

1. On `@` keypress, start capturing query string
2. After 2+ characters, call `/v1/search?q={query}&types={allowedTypes}` with debounce
3. Floating dropdown anchored near textarea caret position
4. Results show type badge + name + truncated description
5. Arrow keys navigate, Enter/click selects, Escape dismisses
6. On selection: insert `@[type:id:name]` replacing `@query`
7. On dismiss: leave `@query` text as-is

Reuses search API caching/debounce patterns from `globalSearch.js` and styling from existing autocompleter.

## Server-side Rendering

A pongo2 template filter `render_mentions` parses markers and outputs HTML.

### Inline mentions (surrounded by other text on the line)
- **Resources**: small inline thumbnail (~1-2rem) + name link
- **Other types**: styled badge link

### Standalone mentions (only content on the line)
- **Resources**: card-like block preview with larger thumbnail + name link
- **Other types**: same as inline (badge link)

### Deleted/missing entities
Rendered as `<span class="mention-missing">Name</span>` with muted/strikethrough style and `aria-label="deleted entity: Name"`.

### Inline vs standalone detection
Filter checks if the marker is the only non-whitespace content on its line.

### Entity existence checking
Batch lookup of all referenced IDs grouped by type. Briefly cached to avoid repeated DB hits.

## Relation Syncing on Save

On create/update of Note/Group/Resource:

1. Parse all `@[Type:ID:Name]` markers from description (and NoteBlock content for Notes)
2. Add each mentioned entity as a relation via existing bulk logic
3. Additive only — mentions never remove relations
4. Idempotent — existing relations silently succeed

Shared helper in application context layer, called after entity save.

Shared parser: `ParseMentions(text string) []Mention` used by both relation syncer and template renderer.

## Accessibility

- Combobox ARIA pattern (role, aria-expanded, aria-activedescendant)
- Live region announces result count
- Full keyboard navigation: @ triggers, arrows navigate, Enter selects, Escape dismisses
- Rendered mentions are standard `<a>` tags
- Sufficient color contrast on badges
- Missing mentions get descriptive aria-label

## What Changes

| Layer | Files | Change |
|-------|-------|--------|
| Frontend | New `src/components/mentionTextarea.js` | Alpine component for @-autocomplete |
| Frontend | `src/main.js` | Import and register component |
| Templates | `createNote.tpl`, `createGroup.tpl`, `createResource.tpl` | Apply to description textareas with allowed types |
| Templates | NoteBlock text editor template | Apply to text block inputs |
| Templates | `description.tpl` / display templates | Pipe through `render_mentions` filter |
| Backend | New `utils/mentions.go` or similar | `ParseMentions()` shared parser |
| Backend | New template filter registration | `render_mentions` pongo2 filter |
| Backend | `note_context.go`, `group_context.go`, `resource_context.go` | Call relation syncer after save |
| CSS | `public/index.css` or Tailwind classes | `.mention-badge`, `.mention-card`, `.mention-missing` styles |

No database schema changes. No new API endpoints. No new dependencies.
