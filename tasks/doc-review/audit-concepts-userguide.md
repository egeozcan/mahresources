# Audit: Concepts and User Guide Docs

Audited against: `inventory-entities.md`, `inventory-features.md`, `inventory-api.md`, `style-guide.md`

---

## concepts/overview.md

**Verdict:** PATCH
**Reason:** Entity table is missing Series and LogEntry. Deletion table has a wrong claim about Category cascade behavior, and the search syntax section omits fuzzy and exact modes.

### Missing Content
- Entity table omits **Series** (groups Resources with shared metadata) and **LogEntry** (activity log)
- Search syntax: missing `~word` (fuzzy mode) and `=word` / `"word"` (exact mode); inventory says terms >= 3 chars default to prefix mode, not just `term*`
- No mention of the `HAS_KEYS` MetaQuery operator (nine operators total, not eight)
- No mention of the Dual Response `.body` suffix (returns HTML body without layout wrapper)
- Bulk operations list missing `replaceTags` (resources) and `addGroups` (resources)

### Wrong Content
- Line ~86: "supports eight comparison operators (EQ, LI, NE, NL, GT, GE, LT, LE)" -- WRONG, there are nine: the inventory also lists `HAS_KEYS` for key existence checks
- Line ~97-100: Search syntax claims `term` "matches words containing term" -- WRONG per inventory; the query parser defaults terms >= 3 chars to prefix mode (`term*`), not substring/contains
- Line ~144: "**Category** | Cascades: **deletes all Groups** in the category" -- WRONG per inventory; Category uses `ON DELETE CASCADE` which does cascade, but the overview.md's own line ~33 and groups.md line ~75 say it is SET NULL. The entity inventory says `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"` which is CASCADE. The conflict is between this file and groups.md (groups.md says SET NULL). The entity inventory confirms CASCADE, so this line is correct but groups.md line ~75 is wrong. However, the groups.md explicitly states SET NULL. Both files must be reconciled.

### Stale Content
- None identified

### Style Issues
- Line ~1: opener is two sentences ("Mahresources has seven entity types. This page describes how they connect.") -- style guide says 1 sentence max for page opener; also the count "seven" is wrong since the table lists nine entity types
- Line ~99: "Multiple words are AND-ed together" -- this is undocumented behavior; the inventory does not confirm this for the global search parser

---

## concepts/resources.md

**Verdict:** PATCH
**Reason:** Missing several Resource fields (Category legacy field, OwnerId in properties table), thumbnail details are vague, and image thumbnail claim is inaccurate (generated on-demand, not on upload).

### Missing Content
- Properties table missing: `category` (legacy category string), `ownerId` (FK to owner Group)
- Properties table missing: `location` (storage path) -- mentioned in prose but not in the Properties table
- No mention of the `RotateResourceQuery` endpoint for image rotation
- No mention of `ResourceFromLocalCreator` (adding files from server filesystem)
- Thumbnail section does not mention SVG thumbnail support (built-in via oksvg/rasterx)
- Thumbnail section does not mention HEIC/AVIF support via ImageMagick fallback
- No mention of the ThumbnailWorker background pre-generation for video resources
- Deletion section does not mention the backup naming format (`{hash}__{id}__{ownerId}___{basename}`)

### Wrong Content
- Line ~51: "Generated on upload for all image types" -- WRONG; per inventory, thumbnails are generated on-demand via `LoadOrCreateThumbnailForResource`, not on upload. The ThumbnailWorker does background pre-generation only for videos.
- Line ~52: "Multiple sizes available for different UI contexts" -- misleading; thumbnails are generated for the requested dimensions, not pre-generated in multiple sizes

### Stale Content
- None identified

### Style Issues
- Line ~7: Page opener is two sentences, not one
- Line ~41-44: "Configure multiple storage locations for:" followed by vague bullets ("Separating different types of content") -- style guide says every claim must have an example; no configuration example shown for alt filesystems

---

## concepts/notes.md

**Verdict:** OK
**Reason:** Comprehensive coverage of Note fields, relationships, deletion behavior, and query parameters. Minor style nit but content is accurate.

### Missing Content
- Note description field: docs say "syncs with first text block" which is correct but does not mention it also syncs from Description to first text block (bidirectional is mentioned later, but the Properties table description is one-directional)
- NoteType: does not mention that the NoteType `description` field exists per entity inventory

### Wrong Content
- None identified

### Stale Content
- None identified

### Style Issues
- Line ~8: Page opener is two sentences (joined by comma but still long). Acceptable but could be tighter.

---

## concepts/note-blocks.md

**Verdict:** PATCH
**Reason:** References block schema is wrong (shows items with type/id but code uses groupIds array). Table block schema is wrong (shows queryName/params but code uses queryId/queryParams/isStatic/columns/rows). Todos block shows "text" field but code uses "label". Several missing details.

### Missing Content
- Gallery block state schema (`layout`: "grid" or "list") not documented
- Table block: missing documentation of dual-mode (manual columns/rows vs query mode), `isStatic` flag, `queryParams` object
- Table block state: missing `sortColumn` and `sortDir` fields
- Calendar block: missing validation rule that each calendar must have non-empty `id`
- Calendar block state: missing `view` allowed values documentation (must be "month", "week", or "agenda")
- Calendar block: missing max custom events limit (500 per `MaxCustomEvents`)
- Calendar block: missing requirement that each custom event needs `id`, `title`, `start`, `end` and `calendarId` must be `"custom"`
- No mention of `ICS files are capped at 10MB` -- wait, line ~154 does mention this. Disregard.
- Sub-endpoints table line ~203: `?id={id}` for table query endpoint -- WRONG; the API inventory says the param is `blockId`, not `id`

### Wrong Content
- Line ~65-75: References block content schema shows `{"items": [{"type": "group", "id": 10}, ...]}` -- WRONG; the entity inventory shows the actual schema is `{"groupIds": [<uint>, ...]}`. The references block only references groups, not arbitrary entity types.
- Line ~86-88: Todos block content shows `{"id": "a1b2", "text": "First task"}` -- WRONG; the entity inventory shows the field is `label`, not `text`: `{"items": [{"id": "<string>", "label": "<string>"}]}`
- Line ~98-108: Table block content shows `{"queryName": "resource-stats", "params": {"minSize": "1000000"}}` -- WRONG; the entity inventory shows the actual fields are `queryId` (uint), `queryParams` (object), `isStatic` (bool), `columns` (array), `rows` (array). There is no `queryName` field; it uses `queryId`.
- Line ~152: "state.view: month, week, or agenda" -- "week" is listed but the entity inventory says the valid values are `"month"`, `"week"`, or `"agenda"` -- this is correct. No issue.
- Line ~203: Table block query endpoint shown as `?id={id}` -- WRONG; the API inventory says the parameter is `blockId`
- Line ~204: Calendar events endpoint shown as `?id={id}&start={date}&end={date}` -- WRONG; the API inventory says the parameter is `blockId`, not `id`

### Stale Content
- None identified

### Style Issues
- None identified

---

## concepts/groups.md

**Verdict:** PATCH
**Reason:** Category deletion behavior claim contradicts the entity inventory. Missing Tags in Group properties table.

### Missing Content
- Group properties table missing: `tags` relationship (M2M via `group_tags`) -- mentioned later but not in properties
- Missing mention of Group `Name` search with `SearchParentsForName` and `SearchChildrenForName` behavior details (how CTEs work)
- Missing mention of the `group_related_groups` M2M self-relationship in the properties/relationships section

### Wrong Content
- Line ~74-78: ":::warning Deleting a Category" says "Deleting a Category sets `categoryId` to NULL on affected Groups (ON DELETE SET NULL)" -- WRONG per entity inventory; the Category model has `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"` which means CASCADE delete, not SET NULL. This contradicts overview.md line ~144 which correctly says "deletes all Groups in the category". The concepts/tags-categories.md line ~305-308 also says CASCADE. This file's claim of SET NULL is wrong.

### Stale Content
- None identified

### Style Issues
- None identified

---

## concepts/tags-categories.md

**Verdict:** PATCH
**Reason:** Category deletion behavior is correctly stated as CASCADE but the admonition says "deletes all Groups" which matches entity inventory. However, the Tag delete section uses "danger" level which is appropriate. Minor missing fields.

### Missing Content
- Tag properties table missing: `createdAt`, `updatedAt` fields (present in entity inventory)
- Category properties table missing: `createdAt`, `updatedAt` fields
- Resource Category properties table missing: `createdAt`, `updatedAt` fields
- No mention of Tag `meta` being searchable via MetaQuery (Tags have `meta` field per inventory but TagQuery doesn't have MetaQuery support -- correct to omit)
- Comparison table at bottom: no mention of Note Types (they are covered in the Note Types section above but not in the comparison)
- Missing `NoteType.description` field in the Note Types section (it exists in the entity inventory)
- Tag deletion: the admonition says ":::danger Cascade delete" but per entity inventory, tag M2M relationships use ON DELETE CASCADE on the join table, which is a join-table cascade, not a delete of the entities themselves. The wording "removes it from all associated Resources, Notes, and Groups" is correct.

### Wrong Content
- None identified -- the Category deletion cascade claim on line ~305-308 correctly says "deletes all Groups" matching the entity inventory

### Stale Content
- None identified

### Style Issues
- Line ~7: Page opener is two sentences ("Tags and Categories organize content differently. Tags are flat labels that apply across Resources, Notes, and Groups.") -- style guide says 1 sentence max
- Line ~141: "Categories define types of Groups with custom presentation and optional metadata schemas." -- repeats the opener phrasing nearly verbatim

---

## concepts/relationships.md

**Verdict:** OK
**Reason:** Accurately documents RelationTypes and Relations with correct field names, constraints, and back-relation mechanics. Content matches entity inventory.

### Missing Content
- No mention of the `uniqueIndex:unique_rel_type` constraint on RelationType (combination of name, fromCategoryId, toCategoryId must be unique)
- No mention that RelationType `Name` is part of the unique constraint (the doc says "Unique name" in the properties table but the actual uniqueness is a composite of name + fromCategoryId + toCategoryId)

### Wrong Content
- Line ~17: "Unique name for the relationship type" -- MISLEADING; the name is unique only within the combination of (name, fromCategoryId, toCategoryId), not globally unique. Two RelationTypes can share the same name if they have different category pairs.

### Stale Content
- None identified

### Style Issues
- None identified

---

## concepts/series.md

**Verdict:** OK
**Reason:** Accurate documentation of Series metadata mechanics, concurrent safety, and API endpoints. Matches entity inventory and features inventory closely.

### Missing Content
- Series properties table missing: `createdAt`, `updatedAt` fields
- No mention of the `GET /v1/series` also accepting `slug` query parameter for lookup (line ~76 says "Also accepts `slug` as a query parameter" which is actually present -- disregard)
- SeriesQuery in entity inventory supports `CreatedBefore`, `CreatedAfter`, `SortBy` -- these are documented in the List Series table

### Wrong Content
- Line ~73: "`GET /v1/series?ID={id}`" -- the parameter capitalization may not match; the API inventory says `id` (lowercase) based on `EntityIdQuery` which has field `ID` mapped from form. This is acceptable as the form binding is case-insensitive.

### Stale Content
- None identified

### Style Issues
- None identified

---

## user-guide/navigation.md

**Verdict:** PATCH
**Reason:** Missing Plugins menu item, missing Quick Tag Panel description in lightbox section, and several keyboard shortcuts omitted.

### Missing Content
- Main menu links missing: **Plugins** / **Manage Plugins** -- per the API inventory, there is a `/plugins/manage` template route and plugin pages
- Admin dropdown missing: **Series** (there is a `/series` template route in the API inventory)
- Settings dropdown: no mention of other settings beyond "Show Descriptions" (the `savedSetting` store persists multiple UI settings to localStorage/sessionStorage)
- Lightbox section missing: **Quick Tag Panel** -- the inventory documents a side panel with 9 configurable tag slots (1-9 keys), not mentioned in this doc
- Lightbox section missing: **Zoom presets** keyboard info (Ctrl+Scroll for zoom toward cursor)
- Keyboard shortcuts table missing: `1-9` keys for quick tag slots in lightbox
- Global search: docs say "limited to 20 items per search (max 50)" but the API inventory says `limit` default is 20, max is 200
- Download Cockpit section missing -- the floating button and Cmd/Ctrl+Shift+D shortcut are mentioned in keyboard shortcuts but not described in the body text (the Download Cockpit is described in managing-resources.md, but navigation.md should at least mention it)
- Missing mention of the paste-upload feature (global paste intercept, modal for uploading pasted content)

### Wrong Content
- Line ~56: "Results cache for 60 seconds and are limited to 20 items per search (max 50)." -- WRONG; per the API inventory, the max limit is 200, not 50. The client-side cache is 30s TTL per the frontend inventory (globalSearch component), and the server-side cache is 60s TTL.
- Line ~158: "Right-click | Select range of items (alternative)" -- per the frontend inventory, right-click triggers range select/deselect from last-selected to right-clicked item, which is correct but the description "alternative" is vague

### Stale Content
- None identified

### Style Issues
- Line ~1: No page opener sentence -- the file jumps straight to "## Top Navigation Bar". Style guide template says "One sentence: what this is and when you need it."

---

## user-guide/managing-resources.md

**Verdict:** PATCH
**Reason:** Missing paste-upload workflow, missing version upload workflow, thumbnail generation claim ("at upload time") is inaccurate.

### Missing Content
- No mention of **paste upload** workflow -- the `pasteUpload` store/component allows pasting images and files from clipboard to create resources
- No mention of **resource versioning** workflow (how to upload a new version from the UI) -- the doc says "upload a new version and use the versioning system" but doesn't explain how
- No mention of **Series assignment** during upload (`SeriesSlug` or `SeriesId` fields)
- No mention of `ContentCategory` field during upload
- Download Cockpit: missing mention of **pause/resume** functionality (the inventory documents pause/resume/cancel/retry)
- Download Cockpit: missing mention that it also shows **plugin action jobs** (merged SSE stream per frontend inventory)
- Inline editing: no mention of **inline description editing** (the `inline-edit` web component supports both name and description)
- Resource detail page: missing mention of **version history** section
- Resource detail page: missing mention of the **activity log** / entity history section
- Image operations: missing mention of `POST /v1/resources/setDimensions` for manually setting dimensions

### Wrong Content
- Line ~205: "Thumbnails are generated at upload time and cached for fast display." -- WRONG; per features inventory, thumbnails are generated on-demand via `LoadOrCreateThumbnailForResource`, not at upload time. Only the video `ThumbnailWorker` does background pre-generation.

### Stale Content
- None identified

### Style Issues
- None identified

---

## user-guide/managing-notes.md

**Verdict:** PATCH
**Reason:** Missing sharing workflow, missing block editor entity picker usage for gallery/references, incorrect claim about custom templates using JavaScript.

### Missing Content
- No mention of **note sharing** workflow (generating share tokens, the share server, accessing shared notes)
- Block editor: Gallery blocks section says "Enter comma-separated resource IDs" but the actual UI uses an entity picker modal (the `blockGallery` component uses `$store.entityPicker` per frontend inventory)
- Block editor: References blocks section says "Enter comma-separated group IDs" but the actual UI uses an entity picker modal (the `blockReferences` component uses `$store.entityPicker`)
- Block editor: Table blocks section missing mention of **query mode** (selecting a saved Query, query parameters, static/dynamic refresh)
- Block editor: Calendar blocks not documented in the user guide at all
- No mention of **note blocks on shared notes** (interactive todos on shared notes, calendar on shared notes)
- Note detail page: missing mention of the **activity log** / entity history

### Wrong Content
- Line ~112-121: "Note type templates have access to the note data through JavaScript: `<div x-data>...<p x-text='entity.Name'>...</p>`" -- MISLEADING; per the features inventory and entity inventory, NoteType custom templates are rendered server-side with Pongo2 (Django-like syntax), not client-side JavaScript/Alpine.js. The template syntax should show `{{ note.Name }}` not `x-text="entity.Name"`. This may work in practice if the templates are rendered in an Alpine context, but the canonical template system is Pongo2.
- Line ~266-268: Gallery blocks "Enter comma-separated resource IDs (e.g., '1, 2, 3')" -- WRONG per frontend inventory; the `blockGallery` component uses the entity picker (`$store.entityPicker.open`) to browse and add resources, not manual ID entry
- Line ~268: References blocks "Enter comma-separated group IDs (e.g., '1, 2, 3')" -- WRONG per frontend inventory; the `blockReferences` component uses the entity picker to browse and add groups

### Stale Content
- None identified

### Style Issues
- None identified

---

## user-guide/organizing-with-groups.md

**Verdict:** OK
**Reason:** Comprehensive and accurate coverage of group workflows including hierarchy, relations, merging, cloning, and metadata. Content matches inventories.

### Missing Content
- No mention of the **group tree** view (`/group/tree` template route, `groupTree` Alpine component for interactive hierarchy visualization)
- No mention of the **text view** for groups (`/groups/text` template route)
- Missing mention that merge also backs up loser metadata into the winner's meta field
- Relation types: missing mention of `ReverseName` parameter for creating inverse relation types

### Wrong Content
- None identified

### Stale Content
- None identified

### Style Issues
- Line ~17-19: "**Category** - The type of group (required in the create form; the API allows it to be empty)" -- the parenthetical is useful context but reads as hedging

---

## user-guide/search.md

**Verdict:** PATCH
**Reason:** Missing fuzzy search mode, missing exact search mode syntax, wrong max result limit.

### Missing Content
- Full-text search syntax: missing `~word` fuzzy search mode (uses trigram matching in PostgreSQL, LIKE fallback in SQLite)
- Full-text search syntax: missing `=word` and `"word"` exact search modes
- Full-text search: missing detail that terms >= 3 characters default to prefix mode automatically (no `*` required)
- Global search "What Gets Searched" table: missing that Resource `OriginalName` is also searched (per FTS setup)
- Saved Queries: missing mention of the `Template` field for custom result rendering (line ~187 mentions it but no explanation of what templates can do)
- Saved Queries: missing mention that queries use a read-only DB connection when `DB_READONLY_DSN` is configured; without it, the main connection is used
- MetaQuery: missing the `HAS_KEYS` operator for checking key existence
- No mention of the `MaxResults` parameter for resource searches
- Group-specific filters missing: `SearchParentsForName`, `SearchChildrenForName`, `SearchParentsForTags`, `SearchChildrenForTags`
- Note-specific filters missing: `StartDateBefore`, `StartDateAfter`, `EndDateBefore`, `EndDateAfter`, `NoteTypeId`, `Shared`

### Wrong Content
- Line ~49-50: "Caches up to 50 results per query" and "Default result limit: 20 (max: 50)" -- WRONG; per the API inventory, `limit` max is 200, not 50. The LRU cache stores query results (not per-result entries); the cache is an in-memory map with 60s TTL.
- Line ~88-89: MetaQuery operators table shows default as "LI" (LIKE) when no operator specified -- per the entity inventory `ParseMeta`, when format is `key:value` (no operator), the default operator is `EQ` (equals), not `LI`. Actually, re-reading the inventory: `ParseMeta` parses `key:value` or `key:operation:value`. For `key:value` format, let me verify... The inventory says operations are "EQ, LI, NE, NL, GT, GE, LT, LE" and format is `key:value` or `key:operation:value`. The default when no operation is specified is not explicitly stated in the inventory excerpt. The doc claims "LI" which may or may not be accurate -- flagging as potentially wrong.

### Stale Content
- None identified

### Style Issues
- Line ~89: "LI | LIKE (default when no operator specified)" -- if this is wrong, it's a content issue, not style

---

## user-guide/bulk-operations.md

**Verdict:** PATCH
**Reason:** Missing tag merge from bulk selection, missing Notes bulk operations, operations table incomplete.

### Missing Content
- Operations table missing: **Notes** column -- per API inventory, notes do not have dedicated bulk endpoints, but the UI may support bulk deletion; the table only shows Resources and Groups
- Operations table missing: **Tags** -- per API inventory, tags support `POST /v1/tags/merge` and `POST /v1/tags/delete` (bulk delete)
- Missing mention of **merge from bulk selection** for resources (the `POST /v1/resources/merge` endpoint exists)
- Missing mention of **merge from bulk selection** for tags (the `POST /v1/tags/merge` endpoint exists)
- Comparing Resources: missing detail that the comparison view supports **image comparison** modes (side-by-side, slider, overlay, toggle via `imageCompare` component) and **text diff** (via `textDiff` component)
- Comparing Resources: missing mention that comparison also supports **cross-resource version comparison** (per API inventory `CrossVersionCompareQuery`)
- Missing mention that bulk operations submit via AJAX and morph the list container on success (per `bulkSelectionForms` component)

### Wrong Content
- None identified

### Stale Content
- None identified

### Style Issues
- None identified

---

# Summary

| File | Verdict |
|------|---------|
| concepts/overview.md | PATCH |
| concepts/resources.md | PATCH |
| concepts/notes.md | OK |
| concepts/note-blocks.md | PATCH |
| concepts/groups.md | PATCH |
| concepts/tags-categories.md | PATCH |
| concepts/relationships.md | OK |
| concepts/series.md | OK |
| user-guide/navigation.md | PATCH |
| user-guide/managing-resources.md | PATCH |
| user-guide/managing-notes.md | PATCH |
| user-guide/organizing-with-groups.md | OK |
| user-guide/search.md | PATCH |
| user-guide/bulk-operations.md | PATCH |

**OK:** 4 files
**PATCH:** 10 files
**REWRITE:** 0 files
