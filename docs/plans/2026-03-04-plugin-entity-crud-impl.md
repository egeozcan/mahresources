# Plugin Entity CRUD API — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend the Lua plugin system with full CRUD for groups, notes, tags, categories, resource categories, note types, group relations, relation types, plus relationship management and resource deletion.

**Architecture:** Add `EntityWriter` interface in `plugin_system/db_api.go`, implement it in `application_context/plugin_db_adapter.go`, wire it via `PluginManager.SetEntityWriter()`, and register all new Lua functions in `registerDbModule()`. Each adapter method converts `map[string]any` options to query model structs and delegates to existing application context methods.

**Tech Stack:** Go, gopher-lua, GORM, existing application_context layer

---

### Task 1: Add EntityWriter interface and SetEntityWriter to plugin_system

**Files:**
- Modify: `plugin_system/db_api.go`

**Step 1: Add the EntityWriter interface and setter**

Add after the existing `EntityQuerier` interface (line 26) in `plugin_system/db_api.go`:

```go
// EntityWriter provides write access to entities for plugins.
// All create/update methods accept map[string]any options and return map[string]any results.
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

	// Categories (Group categories)
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

Add a new `atomic.Value` field, setter, and getter to the PluginManager. In `plugin_system/manager.go`, add a `dbWriter atomic.Value` field next to `dbProvider` (line 79), then add in `db_api.go`:

```go
// SetEntityWriter sets the write provider for plugin DB access.
func (pm *PluginManager) SetEntityWriter(ew EntityWriter) {
	pm.dbWriter.Store(ew)
}

// getDbWriter returns the current EntityWriter, or nil if not yet set.
func (pm *PluginManager) getDbWriter() EntityWriter {
	v := pm.dbWriter.Load()
	if v == nil {
		return nil
	}
	return v.(EntityWriter)
}
```

**Step 2: Add dbWriter field to PluginManager**

In `plugin_system/manager.go`, add after `dbProvider atomic.Value` (line 79):

```go
dbWriter   atomic.Value
```

**Step 3: Build and verify compilation**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles with no errors

**Step 4: Commit**

```bash
git add plugin_system/db_api.go plugin_system/manager.go
git commit -m "feat(plugins): add EntityWriter interface and SetEntityWriter"
```

---

### Task 2: Implement adapter helpers and Group CRUD in plugin_db_adapter.go

**Files:**
- Modify: `application_context/plugin_db_adapter.go`

**Step 1: Add the EntityWriter adapter return type and helpers**

Change `NewPluginDBAdapter` to return both interfaces. Add a new constructor that returns the writer, and add option extraction helpers:

```go
// NewPluginDBWriteAdapter creates a write adapter for plugin DB access.
func NewPluginDBWriteAdapter(ctx *MahresourcesContext) plugin_system.EntityWriter {
	return &pluginDBAdapter{ctx: ctx}
}

// getStringOpt extracts a string from opts, returning "" if missing.
func getStringOpt(opts map[string]any, key string) string {
	if v, ok := opts[key].(string); ok {
		return v
	}
	return ""
}

// getUintOpt extracts a uint from opts (Lua numbers are float64), returning 0 if missing.
func getUintOpt(opts map[string]any, key string) uint {
	if v, ok := opts[key].(float64); ok && v > 0 {
		return uint(v)
	}
	return 0
}

// getUintSliceOpt extracts a []uint from opts (Lua arrays are []any of float64).
func getUintSliceOpt(opts map[string]any, key string) []uint {
	arr, ok := opts[key].([]any)
	if !ok {
		// Also try map[string]any (Lua tables with integer keys get parsed this way)
		if m, ok := opts[key].(map[string]any); ok {
			var result []uint
			for _, v := range m {
				if id, ok := v.(float64); ok && id > 0 {
					result = append(result, uint(id))
				}
			}
			return result
		}
		return nil
	}
	var result []uint
	for _, v := range arr {
		if id, ok := v.(float64); ok && id > 0 {
			result = append(result, uint(id))
		}
	}
	return result
}
```

**Step 2: Implement Group CRUD**

```go
func (a *pluginDBAdapter) CreateGroup(opts map[string]any) (map[string]any, error) {
	creator := &query_models.GroupCreator{
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
		Meta:        getStringOpt(opts, "meta"),
		URL:         getStringOpt(opts, "url"),
		CategoryId:  getUintOpt(opts, "category_id"),
		OwnerId:     getUintOpt(opts, "owner_id"),
		Tags:        getUintSliceOpt(opts, "tags"),
		Groups:      getUintSliceOpt(opts, "groups"),
	}
	group, err := a.ctx.CreateGroup(creator)
	if err != nil {
		return nil, err
	}
	return groupToMap(group), nil
}

func (a *pluginDBAdapter) UpdateGroup(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.GroupEditor{
		GroupCreator: query_models.GroupCreator{
			Name:        getStringOpt(opts, "name"),
			Description: getStringOpt(opts, "description"),
			Meta:        getStringOpt(opts, "meta"),
			URL:         getStringOpt(opts, "url"),
			CategoryId:  getUintOpt(opts, "category_id"),
			OwnerId:     getUintOpt(opts, "owner_id"),
			Tags:        getUintSliceOpt(opts, "tags"),
			Groups:      getUintSliceOpt(opts, "groups"),
		},
		ID: id,
	}
	group, err := a.ctx.UpdateGroup(editor)
	if err != nil {
		return nil, err
	}
	return groupToMap(group), nil
}

func (a *pluginDBAdapter) DeleteGroup(id uint) error {
	return a.ctx.DeleteGroup(id)
}

// groupToMap converts a Group model to a map suitable for Lua.
func groupToMap(g *models.Group) map[string]any {
	result := map[string]any{
		"id":          float64(g.ID),
		"name":        g.Name,
		"description": g.Description,
		"meta":        string(g.Meta),
	}
	if g.OwnerId != nil {
		result["owner_id"] = float64(*g.OwnerId)
	}
	if g.CategoryId != nil {
		result["category_id"] = float64(*g.CategoryId)
	}
	return result
}
```

**Step 3: Build and verify**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles (EntityWriter not fully implemented yet — that's OK, Go doesn't check interface compliance until assignment)

**Step 4: Commit**

```bash
git add application_context/plugin_db_adapter.go
git commit -m "feat(plugins): add adapter helpers and Group CRUD for EntityWriter"
```

---

### Task 3: Implement Note CRUD in adapter

**Files:**
- Modify: `application_context/plugin_db_adapter.go`

**Step 1: Add Note CRUD methods**

```go
func (a *pluginDBAdapter) CreateNote(opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteEditor{}
	editor.Name = getStringOpt(opts, "name")
	editor.Description = getStringOpt(opts, "description")
	editor.Meta = getStringOpt(opts, "meta")
	editor.NoteTypeId = getUintOpt(opts, "note_type_id")
	editor.OwnerId = getUintOpt(opts, "owner_id")
	editor.StartDate = getStringOpt(opts, "start_date")
	editor.EndDate = getStringOpt(opts, "end_date")
	editor.Tags = getUintSliceOpt(opts, "tags")
	editor.Groups = getUintSliceOpt(opts, "groups")
	editor.Resources = getUintSliceOpt(opts, "resources")
	// ID = 0 means create
	note, err := a.ctx.CreateOrUpdateNote(editor)
	if err != nil {
		return nil, err
	}
	return noteToMap(note), nil
}

func (a *pluginDBAdapter) UpdateNote(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteEditor{}
	editor.ID = id
	editor.Name = getStringOpt(opts, "name")
	editor.Description = getStringOpt(opts, "description")
	editor.Meta = getStringOpt(opts, "meta")
	editor.NoteTypeId = getUintOpt(opts, "note_type_id")
	editor.OwnerId = getUintOpt(opts, "owner_id")
	editor.StartDate = getStringOpt(opts, "start_date")
	editor.EndDate = getStringOpt(opts, "end_date")
	editor.Tags = getUintSliceOpt(opts, "tags")
	editor.Groups = getUintSliceOpt(opts, "groups")
	editor.Resources = getUintSliceOpt(opts, "resources")
	note, err := a.ctx.CreateOrUpdateNote(editor)
	if err != nil {
		return nil, err
	}
	return noteToMap(note), nil
}

func (a *pluginDBAdapter) DeleteNote(id uint) error {
	return a.ctx.DeleteNote(id)
}

func noteToMap(n *models.Note) map[string]any {
	result := map[string]any{
		"id":          float64(n.ID),
		"name":        n.Name,
		"description": n.Description,
		"meta":        string(n.Meta),
	}
	if n.OwnerId != nil {
		result["owner_id"] = float64(*n.OwnerId)
	}
	if n.NoteTypeId != nil {
		result["note_type_id"] = float64(*n.NoteTypeId)
	}
	return result
}
```

**Step 2: Build and verify**

Run: `go build --tags 'json1 fts5'`

**Step 3: Commit**

```bash
git add application_context/plugin_db_adapter.go
git commit -m "feat(plugins): add Note CRUD adapter methods"
```

---

### Task 4: Implement Tag, Category, ResourceCategory, NoteType CRUD in adapter

**Files:**
- Modify: `application_context/plugin_db_adapter.go`

**Step 1: Add Tag CRUD**

```go
func (a *pluginDBAdapter) CreateTag(opts map[string]any) (map[string]any, error) {
	creator := &query_models.TagCreator{
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	tag, err := a.ctx.CreateTag(creator)
	if err != nil {
		return nil, err
	}
	return tagToMap(tag), nil
}

func (a *pluginDBAdapter) UpdateTag(id uint, opts map[string]any) (map[string]any, error) {
	creator := &query_models.TagCreator{
		ID:          id,
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	tag, err := a.ctx.UpdateTag(creator)
	if err != nil {
		return nil, err
	}
	return tagToMap(tag), nil
}

func (a *pluginDBAdapter) DeleteTag(id uint) error {
	return a.ctx.DeleteTag(id)
}

func tagToMap(t *models.Tag) map[string]any {
	return map[string]any{
		"id":          float64(t.ID),
		"name":        t.Name,
		"description": t.Description,
	}
}
```

**Step 2: Add Category CRUD**

```go
func (a *pluginDBAdapter) CreateCategory(opts map[string]any) (map[string]any, error) {
	creator := &query_models.CategoryCreator{
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
		MetaSchema:    getStringOpt(opts, "meta_schema"),
	}
	cat, err := a.ctx.CreateCategory(creator)
	if err != nil {
		return nil, err
	}
	return categoryToMap(cat), nil
}

func (a *pluginDBAdapter) UpdateCategory(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.CategoryEditor{
		CategoryCreator: query_models.CategoryCreator{
			Name:          getStringOpt(opts, "name"),
			Description:   getStringOpt(opts, "description"),
			CustomHeader:  getStringOpt(opts, "custom_header"),
			CustomSidebar: getStringOpt(opts, "custom_sidebar"),
			CustomSummary: getStringOpt(opts, "custom_summary"),
			CustomAvatar:  getStringOpt(opts, "custom_avatar"),
			MetaSchema:    getStringOpt(opts, "meta_schema"),
		},
		ID: id,
	}
	cat, err := a.ctx.UpdateCategory(editor)
	if err != nil {
		return nil, err
	}
	return categoryToMap(cat), nil
}

func (a *pluginDBAdapter) DeleteCategory(id uint) error {
	return a.ctx.DeleteCategory(id)
}

func categoryToMap(c *models.Category) map[string]any {
	return map[string]any{
		"id":          float64(c.ID),
		"name":        c.Name,
		"description": c.Description,
	}
}
```

**Step 3: Add ResourceCategory CRUD**

```go
func (a *pluginDBAdapter) CreateResourceCategory(opts map[string]any) (map[string]any, error) {
	creator := &query_models.ResourceCategoryCreator{
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
		MetaSchema:    getStringOpt(opts, "meta_schema"),
	}
	rc, err := a.ctx.CreateResourceCategory(creator)
	if err != nil {
		return nil, err
	}
	return resourceCategoryToMap(rc), nil
}

func (a *pluginDBAdapter) UpdateResourceCategory(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.ResourceCategoryEditor{
		ResourceCategoryCreator: query_models.ResourceCategoryCreator{
			Name:          getStringOpt(opts, "name"),
			Description:   getStringOpt(opts, "description"),
			CustomHeader:  getStringOpt(opts, "custom_header"),
			CustomSidebar: getStringOpt(opts, "custom_sidebar"),
			CustomSummary: getStringOpt(opts, "custom_summary"),
			CustomAvatar:  getStringOpt(opts, "custom_avatar"),
			MetaSchema:    getStringOpt(opts, "meta_schema"),
		},
		ID: id,
	}
	rc, err := a.ctx.UpdateResourceCategory(editor)
	if err != nil {
		return nil, err
	}
	return resourceCategoryToMap(rc), nil
}

func (a *pluginDBAdapter) DeleteResourceCategory(id uint) error {
	return a.ctx.DeleteResourceCategory(id)
}

func resourceCategoryToMap(rc *models.ResourceCategory) map[string]any {
	return map[string]any{
		"id":          float64(rc.ID),
		"name":        rc.Name,
		"description": rc.Description,
	}
}
```

**Step 4: Add NoteType CRUD**

```go
func (a *pluginDBAdapter) CreateNoteType(opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteTypeEditor{
		// ID = 0 means create
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
	}
	nt, err := a.ctx.CreateOrUpdateNoteType(editor)
	if err != nil {
		return nil, err
	}
	return noteTypeToMap(nt), nil
}

func (a *pluginDBAdapter) UpdateNoteType(id uint, opts map[string]any) (map[string]any, error) {
	editor := &query_models.NoteTypeEditor{
		ID:            id,
		Name:          getStringOpt(opts, "name"),
		Description:   getStringOpt(opts, "description"),
		CustomHeader:  getStringOpt(opts, "custom_header"),
		CustomSidebar: getStringOpt(opts, "custom_sidebar"),
		CustomSummary: getStringOpt(opts, "custom_summary"),
		CustomAvatar:  getStringOpt(opts, "custom_avatar"),
	}
	nt, err := a.ctx.CreateOrUpdateNoteType(editor)
	if err != nil {
		return nil, err
	}
	return noteTypeToMap(nt), nil
}

func (a *pluginDBAdapter) DeleteNoteType(id uint) error {
	return a.ctx.DeleteNoteType(id)
}

func noteTypeToMap(nt *models.NoteType) map[string]any {
	return map[string]any{
		"id":          float64(nt.ID),
		"name":        nt.Name,
		"description": nt.Description,
	}
}
```

**Step 5: Build and verify**

Run: `go build --tags 'json1 fts5'`

**Step 6: Commit**

```bash
git add application_context/plugin_db_adapter.go
git commit -m "feat(plugins): add Tag, Category, ResourceCategory, NoteType CRUD adapter methods"
```

---

### Task 5: Implement GroupRelation, RelationType CRUD, and Resource delete in adapter

**Files:**
- Modify: `application_context/plugin_db_adapter.go`

**Step 1: Add GroupRelation CRUD**

```go
func (a *pluginDBAdapter) CreateGroupRelation(opts map[string]any) (map[string]any, error) {
	fromGroupId := getUintOpt(opts, "from_group_id")
	toGroupId := getUintOpt(opts, "to_group_id")
	relationTypeId := getUintOpt(opts, "relation_type_id")
	rel, err := a.ctx.AddRelation(fromGroupId, toGroupId, relationTypeId)
	if err != nil {
		return nil, err
	}
	result := groupRelationToMap(rel)
	// Set name/description if provided (AddRelation doesn't accept them)
	name := getStringOpt(opts, "name")
	desc := getStringOpt(opts, "description")
	if name != "" || desc != "" {
		editQuery := query_models.GroupRelationshipQuery{
			Id:          rel.ID,
			Name:        name,
			Description: desc,
		}
		rel, err = a.ctx.EditRelation(editQuery)
		if err != nil {
			return result, nil // return the created relation even if edit fails
		}
		result = groupRelationToMap(rel)
	}
	return result, nil
}

func (a *pluginDBAdapter) UpdateGroupRelation(opts map[string]any) (map[string]any, error) {
	query := query_models.GroupRelationshipQuery{
		Id:          getUintOpt(opts, "id"),
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	rel, err := a.ctx.EditRelation(query)
	if err != nil {
		return nil, err
	}
	return groupRelationToMap(rel), nil
}

func (a *pluginDBAdapter) DeleteGroupRelation(id uint) error {
	return a.ctx.DeleteRelationship(id)
}

func groupRelationToMap(r *models.GroupRelation) map[string]any {
	result := map[string]any{
		"id":          float64(r.ID),
		"name":        r.Name,
		"description": r.Description,
	}
	if r.FromGroupId != nil {
		result["from_group_id"] = float64(*r.FromGroupId)
	}
	if r.ToGroupId != nil {
		result["to_group_id"] = float64(*r.ToGroupId)
	}
	if r.RelationTypeId != nil {
		result["relation_type_id"] = float64(*r.RelationTypeId)
	}
	return result
}
```

**Step 2: Add RelationType CRUD**

```go
func (a *pluginDBAdapter) CreateRelationType(opts map[string]any) (map[string]any, error) {
	query := &query_models.RelationshipTypeEditorQuery{
		Name:         getStringOpt(opts, "name"),
		Description:  getStringOpt(opts, "description"),
		ReverseName:  getStringOpt(opts, "reverse_name"),
		FromCategory: getUintOpt(opts, "from_category"),
		ToCategory:   getUintOpt(opts, "to_category"),
	}
	rt, err := a.ctx.AddRelationType(query)
	if err != nil {
		return nil, err
	}
	return relationTypeToMap(rt), nil
}

func (a *pluginDBAdapter) UpdateRelationType(opts map[string]any) (map[string]any, error) {
	query := &query_models.RelationshipTypeEditorQuery{
		Id:          getUintOpt(opts, "id"),
		Name:        getStringOpt(opts, "name"),
		Description: getStringOpt(opts, "description"),
	}
	rt, err := a.ctx.EditRelationType(query)
	if err != nil {
		return nil, err
	}
	return relationTypeToMap(rt), nil
}

func (a *pluginDBAdapter) DeleteRelationType(id uint) error {
	return a.ctx.DeleteRelationshipType(id)
}

func relationTypeToMap(rt *models.GroupRelationType) map[string]any {
	result := map[string]any{
		"id":          float64(rt.ID),
		"name":        rt.Name,
		"description": rt.Description,
	}
	if rt.FromCategoryId != nil {
		result["from_category"] = float64(*rt.FromCategoryId)
	}
	if rt.ToCategoryId != nil {
		result["to_category"] = float64(*rt.ToCategoryId)
	}
	return result
}
```

**Step 3: Add Resource delete**

```go
func (a *pluginDBAdapter) DeleteResource(id uint) error {
	return a.ctx.DeleteResource(id)
}
```

**Step 4: Build and verify**

Run: `go build --tags 'json1 fts5'`

**Step 5: Commit**

```bash
git add application_context/plugin_db_adapter.go
git commit -m "feat(plugins): add GroupRelation, RelationType CRUD and Resource delete"
```

---

### Task 6: Implement relationship management methods in adapter

**Files:**
- Modify: `application_context/plugin_db_adapter.go`

**Step 1: Add tag relationship methods**

For groups and resources, use existing bulk methods. For notes, use GORM association directly since no bulk method exists.

```go
func (a *pluginDBAdapter) AddTagsToEntity(entityType string, id uint, tagIds []uint) error {
	if len(tagIds) == 0 {
		return nil
	}
	switch entityType {
	case "group":
		return a.ctx.BulkAddTagsToGroups(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "resource":
		return a.ctx.BulkAddTagsToResources(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "note":
		return a.ctx.AddTagsToNote(id, tagIds)
	default:
		return fmt.Errorf("unsupported entity type %q for add_tags", entityType)
	}
}

func (a *pluginDBAdapter) RemoveTagsFromEntity(entityType string, id uint, tagIds []uint) error {
	if len(tagIds) == 0 {
		return nil
	}
	switch entityType {
	case "group":
		return a.ctx.BulkRemoveTagsFromGroups(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "resource":
		return a.ctx.BulkRemoveTagsFromResources(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  tagIds,
		})
	case "note":
		return a.ctx.RemoveTagsFromNote(id, tagIds)
	default:
		return fmt.Errorf("unsupported entity type %q for remove_tags", entityType)
	}
}
```

**Step 2: Add group relationship methods**

```go
func (a *pluginDBAdapter) AddGroupsToEntity(entityType string, id uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	switch entityType {
	case "resource":
		return a.ctx.BulkAddGroupsToResources(&query_models.BulkEditQuery{
			BulkQuery: query_models.BulkQuery{ID: []uint{id}},
			EditedId:  groupIds,
		})
	case "note":
		return a.ctx.AddGroupsToNote(id, groupIds)
	default:
		return fmt.Errorf("unsupported entity type %q for add_groups", entityType)
	}
}

func (a *pluginDBAdapter) RemoveGroupsFromEntity(entityType string, id uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	switch entityType {
	case "resource":
		return a.ctx.RemoveGroupsFromResource(id, groupIds)
	case "note":
		return a.ctx.RemoveGroupsFromNote(id, groupIds)
	default:
		return fmt.Errorf("unsupported entity type %q for remove_groups", entityType)
	}
}
```

**Step 3: Add note-resource relationship methods**

```go
func (a *pluginDBAdapter) AddResourcesToNote(noteId uint, resourceIds []uint) error {
	if len(resourceIds) == 0 {
		return nil
	}
	return a.ctx.AddResourcesToNote(noteId, resourceIds)
}

func (a *pluginDBAdapter) RemoveResourcesFromNote(noteId uint, resourceIds []uint) error {
	if len(resourceIds) == 0 {
		return nil
	}
	return a.ctx.RemoveResourcesFromNote(noteId, resourceIds)
}
```

**Step 4: Build — this will fail because the relationship helper methods don't exist yet on MahresourcesContext**

Run: `go build --tags 'json1 fts5'`
Expected: Compilation errors for missing methods (AddTagsToNote, RemoveTagsFromNote, AddGroupsToNote, RemoveGroupsFromNote, RemoveGroupsFromResource, AddResourcesToNote, RemoveResourcesFromNote)

**Step 5: Commit the adapter code anyway (or proceed to Task 7 to add the missing methods first)**

```bash
git add application_context/plugin_db_adapter.go
git commit -m "feat(plugins): add relationship management adapter methods"
```

---

### Task 7: Add missing relationship helper methods to application_context

**Files:**
- Modify: `application_context/note_context.go` (add note relationship helpers)
- Modify: `application_context/resource_bulk_context.go` (add resource-group removal)

These methods don't exist yet. They need to be simple GORM association operations.

**Step 1: Add note relationship helpers in note_context.go**

Add at the end of `application_context/note_context.go`:

```go
// AddTagsToNote adds tags to a note by ID.
func (ctx *MahresourcesContext) AddTagsToNote(noteId uint, tagIds []uint) error {
	if len(tagIds) == 0 {
		return nil
	}
	note := models.Note{ID: noteId}
	tags := BuildAssociationSlice(tagIds, TagFromID)
	return ctx.db.Model(&note).Association("Tags").Append(&tags)
}

// RemoveTagsFromNote removes tags from a note by ID.
func (ctx *MahresourcesContext) RemoveTagsFromNote(noteId uint, tagIds []uint) error {
	if len(tagIds) == 0 {
		return nil
	}
	note := models.Note{ID: noteId}
	tags := BuildAssociationSlice(tagIds, TagFromID)
	return ctx.db.Model(&note).Association("Tags").Delete(&tags)
}

// AddGroupsToNote adds groups to a note by ID.
func (ctx *MahresourcesContext) AddGroupsToNote(noteId uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	note := models.Note{ID: noteId}
	groups := BuildAssociationSlice(groupIds, GroupFromID)
	return ctx.db.Model(&note).Association("Groups").Append(&groups)
}

// RemoveGroupsFromNote removes groups from a note by ID.
func (ctx *MahresourcesContext) RemoveGroupsFromNote(noteId uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	note := models.Note{ID: noteId}
	groups := BuildAssociationSlice(groupIds, GroupFromID)
	return ctx.db.Model(&note).Association("Groups").Delete(&groups)
}

// AddResourcesToNote adds resources to a note by ID.
func (ctx *MahresourcesContext) AddResourcesToNote(noteId uint, resourceIds []uint) error {
	if len(resourceIds) == 0 {
		return nil
	}
	note := models.Note{ID: noteId}
	resources := BuildAssociationSlice(resourceIds, ResourceFromID)
	return ctx.db.Model(&note).Association("Resources").Append(&resources)
}

// RemoveResourcesFromNote removes resources from a note by ID.
func (ctx *MahresourcesContext) RemoveResourcesFromNote(noteId uint, resourceIds []uint) error {
	if len(resourceIds) == 0 {
		return nil
	}
	note := models.Note{ID: noteId}
	resources := BuildAssociationSlice(resourceIds, ResourceFromID)
	return ctx.db.Model(&note).Association("Resources").Delete(&resources)
}
```

**Step 2: Add resource-group removal in resource_bulk_context.go**

Add at the end of `application_context/resource_bulk_context.go`:

```go
// RemoveGroupsFromResource removes group associations from a single resource.
func (ctx *MahresourcesContext) RemoveGroupsFromResource(resourceId uint, groupIds []uint) error {
	if len(groupIds) == 0 {
		return nil
	}
	return ctx.db.Exec(
		"DELETE FROM groups_related_resources WHERE resource_id = ? AND group_id IN ?",
		resourceId, groupIds,
	).Error
}
```

**Step 3: Build and verify everything compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles successfully

**Step 4: Commit**

```bash
git add application_context/note_context.go application_context/resource_bulk_context.go
git commit -m "feat: add relationship helper methods for notes and resources"
```

---

### Task 8: Add interface compliance check and wire SetEntityWriter in main

**Files:**
- Modify: `application_context/plugin_db_adapter.go` (interface compliance)
- Modify: Where `SetEntityQuerier` is called — find and wire `SetEntityWriter`

**Step 1: Add interface compliance check**

Add at the top of `plugin_db_adapter.go` (after imports):

```go
var _ plugin_system.EntityWriter = (*pluginDBAdapter)(nil)
```

**Step 2: Find where SetEntityQuerier is called and add SetEntityWriter alongside it**

Search for `SetEntityQuerier` in the codebase and add `SetEntityWriter(NewPluginDBWriteAdapter(ctx))` next to it.

Run: `grep -rn "SetEntityQuerier" .`

This is likely in `main.go` or the context initialization. Add:

```go
pluginManager.SetEntityWriter(application_context.NewPluginDBWriteAdapter(ctx))
```

right after the existing:

```go
pluginManager.SetEntityQuerier(application_context.NewPluginDBAdapter(ctx))
```

**Step 3: Build and run tests**

Run: `go build --tags 'json1 fts5' && go test --tags 'json1 fts5' ./...`
Expected: Compiles and all existing tests pass

**Step 4: Commit**

```bash
git add application_context/plugin_db_adapter.go main.go  # or wherever SetEntityQuerier is called
git commit -m "feat(plugins): wire EntityWriter adapter into application startup"
```

---

### Task 9: Register CRUD Lua functions in registerDbModule

**Files:**
- Modify: `plugin_system/db_api.go`

**Step 1: Add create/update/delete Lua functions for Groups**

Add inside `registerDbModule()`, after the existing `create_resource_from_data` registration (around line 263), but before `mahMod.RawSetString("db", dbMod)`:

```go
// --- Entity CRUD functions ---

// Helper: get writer or return error to Lua
writeOrErr := func(L *lua.LState) EntityWriter {
	w := pm.getDbWriter()
	if w == nil {
		L.Push(lua.LNil)
		L.Push(lua.LString("database writer not available"))
	}
	return w
}

// mah.db.create_group(opts) -> table or (nil, error)
dbMod.RawSetString("create_group", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil {
		return 2
	}
	opts := luaTableToGoMap(L.CheckTable(1))
	result, err := w.CreateGroup(opts)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(goToLuaTable(L, result))
	return 1
}))

// mah.db.update_group(id, opts) -> table or (nil, error)
dbMod.RawSetString("update_group", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil {
		return 2
	}
	id := uint(L.CheckNumber(1))
	opts := luaTableToGoMap(L.CheckTable(2))
	result, err := w.UpdateGroup(id, opts)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(goToLuaTable(L, result))
	return 1
}))

// mah.db.delete_group(id) -> true or (nil, error)
dbMod.RawSetString("delete_group", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil {
		return 2
	}
	id := uint(L.CheckNumber(1))
	if err := w.DeleteGroup(id); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))
```

**Step 2: Add Lua functions for Notes, Tags, Categories, ResourceCategories, NoteTypes**

Follow the exact same pattern as Groups:
- `create_note(opts)`, `update_note(id, opts)`, `delete_note(id)`
- `create_tag(opts)`, `update_tag(id, opts)`, `delete_tag(id)`
- `create_category(opts)`, `update_category(id, opts)`, `delete_category(id)`
- `create_resource_category(opts)`, `update_resource_category(id, opts)`, `delete_resource_category(id)`
- `create_note_type(opts)`, `update_note_type(id, opts)`, `delete_note_type(id)`

For each, the create/update pattern is:
```go
dbMod.RawSetString("create_X", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	opts := luaTableToGoMap(L.CheckTable(1))
	result, err := w.CreateX(opts)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(goToLuaTable(L, result))
	return 1
}))
```

**Step 3: Add Lua functions for GroupRelation, RelationType**

GroupRelation update takes opts (not id + opts) since the id is inside opts:
```go
// mah.db.create_group_relation(opts) -> table or (nil, error)
// mah.db.update_group_relation(opts) -> table or (nil, error)  (id is in opts)
// mah.db.delete_group_relation(id) -> true or (nil, error)
// mah.db.create_relation_type(opts) -> table or (nil, error)
// mah.db.update_relation_type(opts) -> table or (nil, error)   (id is in opts)
// mah.db.delete_relation_type(id) -> true or (nil, error)
```

**Step 4: Add delete_resource**

```go
// mah.db.delete_resource(id) -> true or (nil, error)
dbMod.RawSetString("delete_resource", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	id := uint(L.CheckNumber(1))
	if err := w.DeleteResource(id); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))
```

**Step 5: Build and verify**

Run: `go build --tags 'json1 fts5'`

**Step 6: Commit**

```bash
git add plugin_system/db_api.go
git commit -m "feat(plugins): register all entity CRUD Lua functions in mah.db"
```

---

### Task 10: Register relationship management Lua functions

**Files:**
- Modify: `plugin_system/db_api.go`

**Step 1: Add add_tags, remove_tags, add_groups, remove_groups Lua functions**

```go
// mah.db.add_tags(entity_type, id, tag_ids) -> true or (nil, error)
dbMod.RawSetString("add_tags", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	entityType := L.CheckString(1)
	id := uint(L.CheckNumber(2))
	tagIdsTbl := L.CheckTable(3)
	tagIds := luaTableToUintSlice(tagIdsTbl)
	if err := w.AddTagsToEntity(entityType, id, tagIds); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))

// mah.db.remove_tags(entity_type, id, tag_ids) -> true or (nil, error)
dbMod.RawSetString("remove_tags", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	entityType := L.CheckString(1)
	id := uint(L.CheckNumber(2))
	tagIdsTbl := L.CheckTable(3)
	tagIds := luaTableToUintSlice(tagIdsTbl)
	if err := w.RemoveTagsFromEntity(entityType, id, tagIds); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))

// mah.db.add_groups(entity_type, id, group_ids) -> true or (nil, error)
dbMod.RawSetString("add_groups", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	entityType := L.CheckString(1)
	id := uint(L.CheckNumber(2))
	groupIdsTbl := L.CheckTable(3)
	groupIds := luaTableToUintSlice(groupIdsTbl)
	if err := w.AddGroupsToEntity(entityType, id, groupIds); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))

// mah.db.remove_groups(entity_type, id, group_ids) -> true or (nil, error)
dbMod.RawSetString("remove_groups", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	entityType := L.CheckString(1)
	id := uint(L.CheckNumber(2))
	groupIdsTbl := L.CheckTable(3)
	groupIds := luaTableToUintSlice(groupIdsTbl)
	if err := w.RemoveGroupsFromEntity(entityType, id, groupIds); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))

// mah.db.add_resources_to_note(note_id, resource_ids) -> true or (nil, error)
dbMod.RawSetString("add_resources_to_note", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	noteId := uint(L.CheckNumber(1))
	resourceIdsTbl := L.CheckTable(2)
	resourceIds := luaTableToUintSlice(resourceIdsTbl)
	if err := w.AddResourcesToNote(noteId, resourceIds); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))

// mah.db.remove_resources_from_note(note_id, resource_ids) -> true or (nil, error)
dbMod.RawSetString("remove_resources_from_note", L.NewFunction(func(L *lua.LState) int {
	w := writeOrErr(L)
	if w == nil { return 2 }
	noteId := uint(L.CheckNumber(1))
	resourceIdsTbl := L.CheckTable(2)
	resourceIds := luaTableToUintSlice(resourceIdsTbl)
	if err := w.RemoveResourcesFromNote(noteId, resourceIds); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	return 1
}))
```

**Step 2: Add the luaTableToUintSlice helper**

Add this helper in `plugin_system/db_api.go` (or `hooks.go` alongside other helpers):

```go
// luaTableToUintSlice converts a Lua table (array of numbers) to []uint.
func luaTableToUintSlice(tbl *lua.LTable) []uint {
	var result []uint
	tbl.ForEach(func(_, value lua.LValue) {
		if n, ok := value.(lua.LNumber); ok && float64(n) > 0 {
			result = append(result, uint(n))
		}
	})
	return result
}
```

**Step 3: Build and verify**

Run: `go build --tags 'json1 fts5'`

**Step 4: Commit**

```bash
git add plugin_system/db_api.go
git commit -m "feat(plugins): register relationship management Lua functions (add/remove tags/groups/resources)"
```

---

### Task 11: Write unit tests for adapter methods

**Files:**
- Create: `application_context/plugin_db_adapter_test.go`

**Step 1: Write tests**

Use the same `createTestContext(t)` pattern from existing tests:

```go
package application_context

import (
	"testing"
)

func TestPluginDBAdapter_GroupCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create
	result, err := adapter.CreateGroup(map[string]any{
		"name":        "Test Group",
		"description": "A test group",
	})
	if err != nil {
		t.Fatalf("CreateGroup failed: %v", err)
	}
	if result["name"] != "Test Group" {
		t.Errorf("expected name 'Test Group', got %v", result["name"])
	}
	groupId := uint(result["id"].(float64))

	// Update
	result, err = adapter.UpdateGroup(groupId, map[string]any{
		"name":        "Updated Group",
		"description": "Updated desc",
	})
	if err != nil {
		t.Fatalf("UpdateGroup failed: %v", err)
	}
	if result["name"] != "Updated Group" {
		t.Errorf("expected name 'Updated Group', got %v", result["name"])
	}

	// Delete
	if err := adapter.DeleteGroup(groupId); err != nil {
		t.Fatalf("DeleteGroup failed: %v", err)
	}
}

func TestPluginDBAdapter_NoteCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateNote(map[string]any{
		"name":        "Test Note",
		"description": "A test note",
	})
	if err != nil {
		t.Fatalf("CreateNote failed: %v", err)
	}
	noteId := uint(result["id"].(float64))

	result, err = adapter.UpdateNote(noteId, map[string]any{
		"name":        "Updated Note",
		"description": "Updated desc",
	})
	if err != nil {
		t.Fatalf("UpdateNote failed: %v", err)
	}
	if result["name"] != "Updated Note" {
		t.Errorf("expected name 'Updated Note', got %v", result["name"])
	}

	if err := adapter.DeleteNote(noteId); err != nil {
		t.Fatalf("DeleteNote failed: %v", err)
	}
}

func TestPluginDBAdapter_TagCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateTag(map[string]any{
		"name": "test-tag",
	})
	if err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	tagId := uint(result["id"].(float64))

	result, err = adapter.UpdateTag(tagId, map[string]any{
		"name": "renamed-tag",
	})
	if err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}
	if result["name"] != "renamed-tag" {
		t.Errorf("expected name 'renamed-tag', got %v", result["name"])
	}

	if err := adapter.DeleteTag(tagId); err != nil {
		t.Fatalf("DeleteTag failed: %v", err)
	}
}

func TestPluginDBAdapter_CategoryCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateCategory(map[string]any{
		"name": "Test Category",
	})
	if err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	catId := uint(result["id"].(float64))

	result, err = adapter.UpdateCategory(catId, map[string]any{
		"name": "Updated Category",
	})
	if err != nil {
		t.Fatalf("UpdateCategory failed: %v", err)
	}

	if err := adapter.DeleteCategory(catId); err != nil {
		t.Fatalf("DeleteCategory failed: %v", err)
	}
}

func TestPluginDBAdapter_ResourceCategoryCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateResourceCategory(map[string]any{
		"name": "Test RC",
	})
	if err != nil {
		t.Fatalf("CreateResourceCategory failed: %v", err)
	}
	rcId := uint(result["id"].(float64))

	result, err = adapter.UpdateResourceCategory(rcId, map[string]any{
		"name": "Updated RC",
	})
	if err != nil {
		t.Fatalf("UpdateResourceCategory failed: %v", err)
	}

	if err := adapter.DeleteResourceCategory(rcId); err != nil {
		t.Fatalf("DeleteResourceCategory failed: %v", err)
	}
}

func TestPluginDBAdapter_NoteTypeCRUD(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	result, err := adapter.CreateNoteType(map[string]any{
		"name": "Test NoteType",
	})
	if err != nil {
		t.Fatalf("CreateNoteType failed: %v", err)
	}
	ntId := uint(result["id"].(float64))

	result, err = adapter.UpdateNoteType(ntId, map[string]any{
		"name": "Updated NoteType",
	})
	if err != nil {
		t.Fatalf("UpdateNoteType failed: %v", err)
	}

	if err := adapter.DeleteNoteType(ntId); err != nil {
		t.Fatalf("DeleteNoteType failed: %v", err)
	}
}

func TestPluginDBAdapter_RelationshipManagement(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}

	// Create a group and tags to work with
	groupResult, _ := adapter.CreateGroup(map[string]any{"name": "RelTest Group"})
	groupId := uint(groupResult["id"].(float64))

	tagResult, _ := adapter.CreateTag(map[string]any{"name": "rel-tag"})
	tagId := uint(tagResult["id"].(float64))

	// Add tags
	if err := adapter.AddTagsToEntity("group", groupId, []uint{tagId}); err != nil {
		t.Fatalf("AddTagsToEntity failed: %v", err)
	}

	// Remove tags
	if err := adapter.RemoveTagsFromEntity("group", groupId, []uint{tagId}); err != nil {
		t.Fatalf("RemoveTagsFromEntity failed: %v", err)
	}

	// Create a note and test note relationships
	noteResult, _ := adapter.CreateNote(map[string]any{"name": "RelTest Note"})
	noteId := uint(noteResult["id"].(float64))

	if err := adapter.AddTagsToEntity("note", noteId, []uint{tagId}); err != nil {
		t.Fatalf("AddTagsToEntity (note) failed: %v", err)
	}

	if err := adapter.AddGroupsToEntity("note", noteId, []uint{groupId}); err != nil {
		t.Fatalf("AddGroupsToEntity (note) failed: %v", err)
	}

	// Cleanup
	adapter.DeleteNote(noteId)
	adapter.DeleteTag(tagId)
	adapter.DeleteGroup(groupId)
}
```

**Step 2: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestPluginDBAdapter -v`
Expected: All tests pass

**Step 3: Commit**

```bash
git add application_context/plugin_db_adapter_test.go
git commit -m "test(plugins): add unit tests for EntityWriter adapter methods"
```

---

### Task 12: Run full test suite and fix any issues

**Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All tests pass

**Step 2: Build the full application**

Run: `npm run build`
Expected: Clean build

**Step 3: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: All E2E tests pass

**Step 4: Fix any failures found**

**Step 5: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix: address test failures from plugin entity CRUD implementation"
```

---

### Task 13: Update example plugin to demonstrate new APIs

**Files:**
- Modify: `plugins/example-plugin/plugin.lua`

**Step 1: Add examples of the new CRUD functions**

Add a new section to the example plugin demonstrating the API. Keep it commented out like the existing HTTP examples, but with clear documentation:

```lua
-- Entity CRUD examples (uncomment to try):
--
-- Create a tag:
-- local tag = mah.db.create_tag({name = "auto-generated", description = "Created by plugin"})
-- mah.log("info", "Created tag: " .. tag.name .. " (id: " .. tag.id .. ")")
--
-- Create a category:
-- local cat = mah.db.create_category({name = "Plugin Category", description = "Auto-created"})
--
-- Create a group:
-- local group = mah.db.create_group({name = "Plugin Group", category_id = cat.id, tags = {tag.id}})
--
-- Update a group:
-- mah.db.update_group(group.id, {name = "Renamed Group"})
--
-- Add tags:
-- mah.db.add_tags("group", group.id, {tag.id})
--
-- Remove tags:
-- mah.db.remove_tags("group", group.id, {tag.id})
--
-- Delete:
-- mah.db.delete_group(group.id)
-- mah.db.delete_category(cat.id)
-- mah.db.delete_tag(tag.id)
```

**Step 2: Commit**

```bash
git add plugins/example-plugin/plugin.lua
git commit -m "docs(plugins): add entity CRUD examples to example plugin"
```
