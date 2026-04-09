# Note Type Feature Parity: Schema, Shortcodes & Section Config

**Date:** 2026-04-09
**Status:** Approved

## Summary

Bring NoteType to feature parity with Category (groups) and ResourceCategory (resources) by adding:
1. **MetaSchema** — JSON schema support for note metadata
2. **Shortcode support** — already works via reflection; adding MetaSchema to the model enables it automatically
3. **SectionConfig** — per-note-type control over which sections appear on note detail pages

## Approach

Direct mirror of the existing Category/ResourceCategory pattern. No new architectural patterns introduced.

---

## 1. Model Layer

Add two fields to `NoteType` in `models/note_type_model.go`:

```go
MetaSchema    string     `gorm:"type:text"`
SectionConfig types.JSON `gorm:"type:json"`
```

GORM AutoMigrate handles column addition. No manual migration needed.

## 2. NoteSectionConfig

New struct in `models/section_config.go` with `ResolveNoteSectionConfig()` resolver. All bools default to `true`.

### Toggleable Sections

| Field | Controls | Area |
|-------|----------|------|
| `content` | Description AND block editor (combined toggle) | Body |
| `groups` | Groups association list | Body |
| `resources` | Resources association list | Body |
| `timestamps` | Start/end dates in meta strip | Body |
| `tags` | Tag list | Sidebar |
| `metaJson` | Raw JSON metadata display | Sidebar |
| `metaSchemaDisplay` | Schema-rendered metadata display | Sidebar |
| `owner` | Owner display | Sidebar |
| `noteTypeLink` | Note type link | Sidebar |
| `share` | Share & plugin actions | Sidebar |

No collapsible sections — notes don't use `<details>` wrappers.

### Struct Definition

```go
type NoteSectionConfig struct {
    Content           bool `json:"content"`
    Groups            bool `json:"groups"`
    Resources         bool `json:"resources"`
    Timestamps        bool `json:"timestamps"`
    Tags              bool `json:"tags"`
    MetaJson          bool `json:"metaJson"`
    MetaSchemaDisplay bool `json:"metaSchemaDisplay"`
    Owner             bool `json:"owner"`
    NoteTypeLink      bool `json:"noteTypeLink"`
    Share             bool `json:"share"`
}
```

Plus corresponding `rawNoteSectionConfig` with pointer fields and `ResolveNoteSectionConfig()` following the exact pattern of the existing resolvers.

## 3. Shortcode Support

No code changes needed. The shortcode tag parser (`server/template_handlers/template_filters/shortcode_tag.go:98-100`) already handles notes:

```go
case "Note":
    entityType = "note"
    metaSchema = extractCategorySchema(v, "NoteType")
```

`extractCategorySchema` uses reflection to read `MetaSchema` from the related type. Once the field exists on `NoteType`, shortcodes in CustomHeader/CustomSidebar automatically get schema access.

## 4. Query Model

Update `NoteTypeEditor` in `models/query_models/note_query.go`:

```go
type NoteTypeEditor struct {
    ID            uint
    Name          string
    Description   string
    CustomHeader  string
    CustomSidebar string
    CustomSummary string
    CustomAvatar  string
    MetaSchema    string   // NEW
    SectionConfig string   // NEW
}
```

## 5. API Handler

Update `GetAddNoteTypeHandler` in `server/api_handlers/note_api_handlers.go`:

- Add `MetaSchema` and `SectionConfig` to the partial-update pre-fill logic for both JSON and form-encoded request paths
- For SectionConfig: preserve existing value when field is not sent (same pattern as category handlers)

## 6. Application Context

Update `CreateOrUpdateNoteType` in the note type context file to:
- Pass `MetaSchema` field through to the model
- Convert and pass `SectionConfig` string to `types.JSON` (same pattern as `category_context.go`)
- Preserve SectionConfig on updates when the field is not explicitly sent

## 7. Templates

### `createNoteType.tpl`

Mirror `createCategory.tpl`:
- Wrap custom template fields (CustomHeader, CustomSidebar, CustomSummary, CustomAvatar) in a fieldset with collapsible reference docs
- Add MetaSchema textarea with schema editor modal
- Include `sectionConfigForm.tpl` with `sectionConfigType="note"`

### `createNote.tpl`

Add schema-aware meta editor, mirroring `createGroup.tpl` lines 57-110:
- When a NoteType with MetaSchema is selected, show `<schema-form-mode>` instead of freeFields
- Listen for the NoteType autocompleter's `multiple-input` event to get MetaSchema from the selected type
- Fall back to freeFields when no schema is available
- This replaces the current raw `freeFields.tpl` include on line 81

### `displayNote.tpl`

Wrap each section in `{% if sc.X %}` conditionals:
- `sc.Content` — description/block editor
- `sc.Timestamps` — meta strip dates
- `sc.Groups` — groups seeAll
- `sc.Resources` — resources seeAll
- `sc.Owner` — owner sidebar
- `sc.NoteTypeLink` — note type sidebar link
- `sc.Tags` — tags sidebar
- `sc.MetaJson` — meta JSON sidebar
- `sc.MetaSchemaDisplay` — schema display sidebar (new section)
- `sc.Share` — share/actions sidebar

### `displayNoteText.tpl`

The wide-display route for notes. Must also respect SectionConfig:
- `sc.Content` — description/block editor (the main body)
- `sc.Owner` — owner sidebar
- `sc.NoteTypeLink` — note type sidebar link
- `sc.Tags` — tags sidebar
- `sc.MetaJson` — meta JSON sidebar
- `sc.MetaSchemaDisplay` — schema-rendered metadata (sidebar)

This ensures SectionConfig behaves consistently across both note detail routes.

### `listNotes.tpl` and `listNotesTimeline.tpl`

Add `schemaSearchFields` support, mirroring `listGroups.tpl` and `listGroupsTimeline.tpl`:
- When a NoteType is selected in the search sidebar, extract its MetaSchema
- Render schema-driven search fields alongside the existing freeFields
- The schema-driven search feature (2026-03-31 spec) explicitly excluded notes because NoteType had no MetaSchema; this lifts that restriction

### Note Template Context

Add a note template context provider (following the pattern in `server/template_handlers/group_template_context.go`) that:
1. Resolves `NoteSectionConfig` from `note.NoteType.SectionConfig`
2. Passes it as `sc` to the template
3. Used by both `displayNote.tpl` and `displayNoteText.tpl`

**Nil NoteType fallback:** Notes without a NoteType are valid in the current app. When `note.NoteType == nil`, call `ResolveNoteSectionConfig(nil)` which returns all-defaults (everything enabled). This matches how `ResolveGroupSectionConfig(nil)` and `ResolveResourceSectionConfig(nil)` already handle nil categories.

## 8. Frontend

### `src/components/sectionConfigForm.js`

Add note defaults:

```js
const noteDefaults = {
    content: true, groups: true, resources: true, timestamps: true,
    tags: true, metaJson: true, metaSchemaDisplay: true,
    owner: true, noteTypeLink: true, share: true,
};
```

Update the defaults selection:
```js
const defaults = type === 'group' ? groupDefaults : type === 'note' ? noteDefaults : resourceDefaults;
```

### `templates/partials/sectionConfigForm.tpl`

- Add `type === 'note'` to the description paragraph
- Add note-specific body section with `content` toggle
- Note associations section: groups, resources
- Note-specific sidebar items: `noteTypeLink`, `share`
- Shared sidebar items (tags, metaJson, owner) already in the common sidebar section
- In the shared "Main Content" section: for notes, show `content` (description + blocks) instead of `description`, show `metaSchemaDisplay` and `timestamps` as normal, hide `breadcrumb` (notes don't have breadcrumbs)

## 9. Plugin DB Adapter

Update `application_context/plugin_db_adapter.go`:

### `noteTypeToMap` (line 722)
Add `meta_schema` and `section_config` fields to the map output, matching `categoryToMap` and `resourceCategoryToMap`.

### `CreateNoteType` / `UpdateNoteType` / `PatchNoteType` (lines 1121-1177)
Pass `MetaSchema` and `SectionConfig` through to the `NoteTypeEditor`, matching how category CRUD handles these fields.

### `plugin_system/db_api.go`
No interface changes needed — the EntityWriter methods already accept `map[string]any`, so the new fields flow through naturally.

## 10. Files Changed

| File | Change |
|------|--------|
| `models/note_type_model.go` | Add MetaSchema, SectionConfig fields |
| `models/section_config.go` | Add NoteSectionConfig, rawNoteSectionConfig, ResolveNoteSectionConfig |
| `models/query_models/note_query.go` | Add MetaSchema, SectionConfig to NoteTypeEditor |
| `server/api_handlers/note_api_handlers.go` | Partial-update pre-fill for new fields |
| `application_context/note_type_context.go` (or equivalent) | Pass new fields in create/update |
| `application_context/plugin_db_adapter.go` | noteTypeToMap + Create/Update/Patch for new fields |
| `server/template_handlers/note_template_context.go` (new or existing) | Resolve and pass sc |
| `templates/createNoteType.tpl` | Schema editor, section config form, reference docs |
| `templates/createNote.tpl` | Schema-aware meta editor (schema-form-mode / freeFields toggle) |
| `templates/displayNote.tpl` | Section config conditionals |
| `templates/displayNoteText.tpl` | Section config conditionals |
| `templates/listNotes.tpl` | schemaSearchFields for NoteType MetaSchema |
| `templates/listNotesTimeline.tpl` | schemaSearchFields for NoteType MetaSchema |
| `src/components/sectionConfigForm.js` | Note defaults |
| `templates/partials/sectionConfigForm.tpl` | Note-specific form sections |

## 11. Testing

- E2E tests for section config toggles on note detail pages (mirroring `e2e/tests/75-section-config.spec.ts`)
- E2E tests for section config on the wide-display route (`displayNoteText.tpl`)
- E2E tests for MetaSchema on note types (schema editor in create/edit form)
- E2E tests for schema-aware meta editor on note create/edit form
- E2E tests for schemaSearchFields on note list pages
- Go unit tests for `ResolveNoteSectionConfig` with various JSON inputs
- Plugin adapter tests for noteType CRUD with meta_schema and section_config
