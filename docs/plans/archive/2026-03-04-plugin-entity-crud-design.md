# Plugin Entity CRUD API Design

**Date:** 2026-03-04
**Status:** Approved

## Goal

Extend the plugin system so Lua plugins can perform full CRUD operations on all entities (except Queries), manage relationships between entities, and create/manage all category types.

## Scope

### Entities with Full CRUD

| Entity | Create | Update | Delete | Notes |
|--------|--------|--------|--------|-------|
| Group | Yes | Yes | Yes | |
| Note | Yes | Yes | Yes | |
| Resource | Existing | No (immutable files) | Yes (new) | `create_resource_from_url` / `create_resource_from_data` kept |
| Tag | Yes | Yes | Yes | |
| Category (Group) | Yes | Yes | Yes | |
| ResourceCategory | Yes | Yes | Yes | |
| NoteType | Yes | Yes | Yes | |
| GroupRelation | Yes | Yes | Yes | |
| RelationType | Yes | Yes | Yes | |

### Relationship Management

- Add/remove **tags** on groups, notes, resources
- Add/remove **groups** on notes, resources
- Add/remove **resources** on notes

## Approach

**Approach A: Per-entity explicit methods** — A new `EntityWriter` interface with explicit methods for each entity, matching the codebase's existing Reader/Writer/Deleter pattern. Implemented in `plugin_db_adapter.go`, exposed via `mah.db.*` Lua functions.

## EntityWriter Interface

```go
type EntityWriter interface {
    // Groups
    CreateGroup(opts map[string]any) (map[string]any, error)
    UpdateGroup(id uint, opts map[string]any) (map[string]any, error)
    DeleteGroup(id uint) error

    // Notes
    CreateNote(opts map[string]any) (map[string]any, error)
    UpdateNote(id uint, opts map[string]any) (map[string]any, error)
    DeleteNote(id uint) error

    // Tags
    CreateTag(opts map[string]any) (map[string]any, error)
    UpdateTag(id uint, opts map[string]any) (map[string]any, error)
    DeleteTag(id uint) error

    // Categories (for Groups)
    CreateCategory(opts map[string]any) (map[string]any, error)
    UpdateCategory(id uint, opts map[string]any) (map[string]any, error)
    DeleteCategory(id uint) error

    // Resource Categories
    CreateResourceCategory(opts map[string]any) (map[string]any, error)
    UpdateResourceCategory(id uint, opts map[string]any) (map[string]any, error)
    DeleteResourceCategory(id uint) error

    // Note Types
    CreateNoteType(opts map[string]any) (map[string]any, error)
    UpdateNoteType(id uint, opts map[string]any) (map[string]any, error)
    DeleteNoteType(id uint) error

    // Group Relations
    CreateGroupRelation(opts map[string]any) (map[string]any, error)
    UpdateGroupRelation(opts map[string]any) (map[string]any, error)
    DeleteGroupRelation(id uint) error

    // Relation Types
    CreateRelationType(opts map[string]any) (map[string]any, error)
    UpdateRelationType(opts map[string]any) (map[string]any, error)
    DeleteRelationType(id uint) error

    // Relationship management
    AddTagsToEntity(entityType string, id uint, tagIds []uint) error
    RemoveTagsFromEntity(entityType string, id uint, tagIds []uint) error
    AddGroupsToEntity(entityType string, id uint, groupIds []uint) error
    RemoveGroupsFromEntity(entityType string, id uint, groupIds []uint) error
    AddResourcesToNote(noteId uint, resourceIds []uint) error
    RemoveResourcesFromNote(noteId uint, resourceIds []uint) error

    // Resource delete
    DeleteResource(id uint) error
}
```

## Lua API Surface

All functions under `mah.db.*`. Create/update return entity tables. Delete returns nothing on success, `(nil, error)` on failure.

### Groups

```lua
local group = mah.db.create_group({
    name = "My Group",
    description = "...",
    category_id = 1,
    owner_id = 2,
    tags = {1, 2},
    groups = {3},
    meta = "{}",
    url = "https://..."
})

mah.db.update_group(group.id, {name = "Updated", description = "..."})
mah.db.delete_group(group.id)
```

### Notes

```lua
local note = mah.db.create_note({
    name = "My Note",
    description = "...",
    note_type_id = 1,
    owner_id = 2,
    tags = {1},
    groups = {2},
    resources = {3},
    meta = "{}",
    start_date = "2024-01-01",
    end_date = "2024-12-31"
})

mah.db.update_note(note.id, {name = "Updated"})
mah.db.delete_note(note.id)
```

### Tags

```lua
local tag = mah.db.create_tag({name = "my-tag", description = "..."})
mah.db.update_tag(tag.id, {name = "renamed-tag"})
mah.db.delete_tag(tag.id)
```

### Categories (Group Categories)

```lua
local cat = mah.db.create_category({
    name = "My Category",
    description = "...",
    custom_header = "<h1>...</h1>",
    custom_sidebar = "...",
    custom_summary = "...",
    custom_avatar = "...",
    meta_schema = "{}"
})

mah.db.update_category(cat.id, {name = "Updated"})
mah.db.delete_category(cat.id)
```

### Resource Categories

```lua
local rc = mah.db.create_resource_category({
    name = "Photos",
    description = "...",
    custom_header = "...",
    meta_schema = "{}"
})

mah.db.update_resource_category(rc.id, {name = "Images"})
mah.db.delete_resource_category(rc.id)
```

### Note Types

```lua
local nt = mah.db.create_note_type({
    name = "Meeting Notes",
    description = "...",
    custom_header = "...",
    custom_sidebar = "...",
    custom_summary = "...",
    custom_avatar = "..."
})

mah.db.update_note_type(nt.id, {name = "Updated"})
mah.db.delete_note_type(nt.id)
```

### Group Relations

```lua
local rel = mah.db.create_group_relation({
    from_group_id = 1,
    to_group_id = 2,
    relation_type_id = 3,
    name = "...",
    description = "..."
})

mah.db.update_group_relation({id = rel.id, name = "Updated"})
mah.db.delete_group_relation(rel.id)
```

### Relation Types

```lua
local rt = mah.db.create_relation_type({
    name = "Parent Of",
    description = "...",
    reverse_name = "Child Of",
    from_category = 1,
    to_category = 2
})

mah.db.update_relation_type({id = rt.id, name = "Contains"})
mah.db.delete_relation_type(rt.id)
```

### Relationship Management

```lua
-- Tags on any entity
mah.db.add_tags("group", group_id, {1, 2, 3})
mah.db.remove_tags("group", group_id, {1})
mah.db.add_tags("resource", resource_id, {4, 5})
mah.db.add_tags("note", note_id, {6})

-- Groups on notes/resources
mah.db.add_groups("note", note_id, {1, 2})
mah.db.remove_groups("resource", resource_id, {3})

-- Resources on notes
mah.db.add_resources_to_note(note_id, {1, 2, 3})
mah.db.remove_resources_from_note(note_id, {1})

-- Resource delete
mah.db.delete_resource(resource_id)
```

## Implementation Files

| File | Changes |
|------|---------|
| `plugin_system/db_api.go` | Add `EntityWriter` interface; register new Lua functions in `registerDBAPI` |
| `application_context/plugin_db_adapter.go` | Implement `EntityWriter` — bridge `map[string]any` to query model structs to app context calls |
| `plugin_system/manager.go` | Accept `EntityWriter` in constructor alongside `EntityQuerier` |

### Adapter Pattern

Each adapter method:
1. Extracts fields from `map[string]any` opts using helper functions (`getStringOpt`, `getUintOpt`, `getUintSliceOpt`)
2. Builds the appropriate query model struct
3. Calls the corresponding application context method
4. Converts the resulting model to `map[string]any` for Lua

### Relationship Management Implementation

Uses existing bulk operation methods internally:
- `BulkAddTagsToGroups`, `BulkRemoveTagsFromGroups` for group tags
- `BulkAddTagsToResources`, `BulkRemoveTagsFromResources` for resource tags
- Note tag/group/resource management via `CreateOrUpdateNote` (append associations)

### Hook Behavior

Entity creation/update/delete through the plugin API fires before/after hooks normally. This means:
- Plugin A creating a group triggers Plugin B's `before_group_create` hook
- Hooks can modify or abort plugin-initiated operations
- This is intentional — hooks are a system-wide contract

### Error Handling

- All errors returned to Lua as `(nil, error_string)` pairs
- Validation happens in the application context layer (same as HTTP API)
- Before-hook aborts surface as errors to the calling plugin

## Testing

- Unit tests in `application_context/plugin_db_adapter_test.go` for each adapter method
- Extend example plugin to exercise new APIs
- Existing E2E tests unaffected (additive change)

## Backwards Compatibility

- Existing `create_resource_from_url` and `create_resource_from_data` kept as-is
- No changes to existing Lua API surface
- `EntityQuerier` interface unchanged — `EntityWriter` is a new, separate interface
