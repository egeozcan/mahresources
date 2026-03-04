package plugin_system

import (
	lua "github.com/yuin/gopher-lua"
)

// EntityQuerier provides database access to entities for plugins.
// All methods return map[string]any to avoid importing models into the plugin_system package.
// Numeric values must be float64 for Lua compatibility.
type EntityQuerier interface {
	// Single entity by ID — returns nil map if not found
	GetNoteData(id uint) (map[string]any, error)
	GetResourceData(id uint) (map[string]any, error)
	GetGroupData(id uint) (map[string]any, error)
	GetTagData(id uint) (map[string]any, error)
	GetCategoryData(id uint) (map[string]any, error)
	// List queries with simple filters
	QueryNotes(filter map[string]any) ([]map[string]any, error)
	QueryResources(filter map[string]any) ([]map[string]any, error)
	QueryGroups(filter map[string]any) ([]map[string]any, error)
	// Resource file data — returns base64 content and MIME type
	GetResourceFileData(id uint) (string, string, error)
	// Resource creation
	CreateResourceFromURL(url string, options map[string]any) (map[string]any, error)
	CreateResourceFromData(base64Data string, options map[string]any) (map[string]any, error)
}

// EntityWriter provides write access to entities for plugins.
// Note: Update methods replace ALL fields; omitted fields revert to zero values.
// This includes associations — updating a note without specifying tags will clear its tags.
type EntityWriter interface {
	CreateGroup(opts map[string]any) (map[string]any, error)
	UpdateGroup(id uint, opts map[string]any) (map[string]any, error)
	DeleteGroup(id uint) error
	CreateNote(opts map[string]any) (map[string]any, error)
	UpdateNote(id uint, opts map[string]any) (map[string]any, error)
	DeleteNote(id uint) error
	CreateTag(opts map[string]any) (map[string]any, error)
	UpdateTag(id uint, opts map[string]any) (map[string]any, error)
	DeleteTag(id uint) error
	CreateCategory(opts map[string]any) (map[string]any, error)
	UpdateCategory(id uint, opts map[string]any) (map[string]any, error)
	DeleteCategory(id uint) error
	CreateResourceCategory(opts map[string]any) (map[string]any, error)
	UpdateResourceCategory(id uint, opts map[string]any) (map[string]any, error)
	DeleteResourceCategory(id uint) error
	CreateNoteType(opts map[string]any) (map[string]any, error)
	UpdateNoteType(id uint, opts map[string]any) (map[string]any, error)
	DeleteNoteType(id uint) error
	CreateGroupRelation(opts map[string]any) (map[string]any, error)
	UpdateGroupRelation(opts map[string]any) (map[string]any, error)
	DeleteGroupRelation(id uint) error
	CreateRelationType(opts map[string]any) (map[string]any, error)
	UpdateRelationType(opts map[string]any) (map[string]any, error)
	DeleteRelationType(id uint) error
	AddTagsToEntity(entityType string, id uint, tagIds []uint) error
	RemoveTagsFromEntity(entityType string, id uint, tagIds []uint) error
	AddGroupsToEntity(entityType string, id uint, groupIds []uint) error
	RemoveGroupsFromEntity(entityType string, id uint, groupIds []uint) error
	AddResourcesToNote(noteId uint, resourceIds []uint) error
	RemoveResourcesFromNote(noteId uint, resourceIds []uint) error
	DeleteResource(id uint) error
}

// SetEntityQuerier sets the database provider for plugin DB access.
// This is called after context creation to break the circular dependency
// between plugin_system and application_context.
func (pm *PluginManager) SetEntityQuerier(eq EntityQuerier) {
	pm.dbProvider.Store(eq)
}

// getDbProvider returns the current EntityQuerier, or nil if not yet set.
func (pm *PluginManager) getDbProvider() EntityQuerier {
	v := pm.dbProvider.Load()
	if v == nil {
		return nil
	}
	return v.(EntityQuerier)
}

// SetEntityWriter sets the database writer for plugin entity CRUD.
// This is called after context creation to break the circular dependency
// between plugin_system and application_context.
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

// registerDbModule registers the mah.db sub-table in the Lua VM.
// Functions check pm.dbProvider at call time (not at registration) so they
// work even though the provider is set after plugin loading.
func (pm *PluginManager) registerDbModule(L *lua.LState, mahMod *lua.LTable) {
	dbMod := L.NewTable()

	// mah.db.get_note(id) -> table or nil
	dbMod.RawSetString("get_note", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		id := uint(L.CheckNumber(1))
		data, err := db.GetNoteData(id)
		if err != nil || data == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaTable(L, data))
		return 1
	}))

	// mah.db.get_resource(id) -> table or nil
	dbMod.RawSetString("get_resource", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		id := uint(L.CheckNumber(1))
		data, err := db.GetResourceData(id)
		if err != nil || data == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaTable(L, data))
		return 1
	}))

	// mah.db.get_group(id) -> table or nil
	dbMod.RawSetString("get_group", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		id := uint(L.CheckNumber(1))
		data, err := db.GetGroupData(id)
		if err != nil || data == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaTable(L, data))
		return 1
	}))

	// mah.db.get_tag(id) -> table or nil
	dbMod.RawSetString("get_tag", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		id := uint(L.CheckNumber(1))
		data, err := db.GetTagData(id)
		if err != nil || data == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaTable(L, data))
		return 1
	}))

	// mah.db.get_category(id) -> table or nil
	dbMod.RawSetString("get_category", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		id := uint(L.CheckNumber(1))
		data, err := db.GetCategoryData(id)
		if err != nil || data == nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(goToLuaTable(L, data))
		return 1
	}))

	// mah.db.query_notes({name = "meeting%", limit = 10}) -> array of tables
	dbMod.RawSetString("query_notes", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		filterTable := L.OptTable(1, L.NewTable())
		filter := luaTableToGoMap(filterTable)
		results, err := db.QueryNotes(filter)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		for i, item := range results {
			tbl.RawSetInt(i+1, goToLuaTable(L, item))
		}
		L.Push(tbl)
		return 1
	}))

	// mah.db.query_resources({name = "photo%", content_type = "image/%", limit = 10}) -> array of tables
	dbMod.RawSetString("query_resources", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		filterTable := L.OptTable(1, L.NewTable())
		filter := luaTableToGoMap(filterTable)
		results, err := db.QueryResources(filter)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		for i, item := range results {
			tbl.RawSetInt(i+1, goToLuaTable(L, item))
		}
		L.Push(tbl)
		return 1
	}))

	// mah.db.query_groups({name = "team%", limit = 10}) -> array of tables
	dbMod.RawSetString("query_groups", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		filterTable := L.OptTable(1, L.NewTable())
		filter := luaTableToGoMap(filterTable)
		results, err := db.QueryGroups(filter)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		tbl := L.NewTable()
		for i, item := range results {
			tbl.RawSetInt(i+1, goToLuaTable(L, item))
		}
		L.Push(tbl)
		return 1
	}))

	// mah.db.get_resource_data(id) -> base64_string, mime_type or nil
	dbMod.RawSetString("get_resource_data", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			return 1
		}
		id := uint(L.CheckNumber(1))
		base64Data, mimeType, err := db.GetResourceFileData(id)
		if err != nil {
			L.Push(lua.LNil)
			return 1
		}
		L.Push(lua.LString(base64Data))
		L.Push(lua.LString(mimeType))
		return 2
	}))

	// mah.db.create_resource_from_url(url, options) -> table or (nil, error)
	dbMod.RawSetString("create_resource_from_url", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database not available"))
			return 2
		}
		url := L.CheckString(1)
		opts := make(map[string]any)
		if optTbl := L.OptTable(2, nil); optTbl != nil {
			opts = luaTableToGoMap(optTbl)
		}
		result, err := db.CreateResourceFromURL(url, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.create_resource_from_data(base64, options) -> table or (nil, error)
	dbMod.RawSetString("create_resource_from_data", L.NewFunction(func(L *lua.LState) int {
		db := pm.getDbProvider()
		if db == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database not available"))
			return 2
		}
		base64Data := L.CheckString(1)
		opts := make(map[string]any)
		if optTbl := L.OptTable(2, nil); optTbl != nil {
			opts = luaTableToGoMap(optTbl)
		}
		result, err := db.CreateResourceFromData(base64Data, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// --- Entity CRUD ---

	// mah.db.create_group(opts) -> table or (nil, error)
	dbMod.RawSetString("create_group", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
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
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
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
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
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

	// mah.db.create_note(opts) -> table or (nil, error)
	dbMod.RawSetString("create_note", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateNote(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_note(id, opts) -> table or (nil, error)
	dbMod.RawSetString("update_note", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		opts := luaTableToGoMap(L.CheckTable(2))
		result, err := w.UpdateNote(id, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_note(id) -> true or (nil, error)
	dbMod.RawSetString("delete_note", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteNote(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.create_tag(opts) -> table or (nil, error)
	dbMod.RawSetString("create_tag", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateTag(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_tag(id, opts) -> table or (nil, error)
	dbMod.RawSetString("update_tag", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		opts := luaTableToGoMap(L.CheckTable(2))
		result, err := w.UpdateTag(id, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_tag(id) -> true or (nil, error)
	dbMod.RawSetString("delete_tag", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteTag(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.create_category(opts) -> table or (nil, error)
	dbMod.RawSetString("create_category", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateCategory(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_category(id, opts) -> table or (nil, error)
	dbMod.RawSetString("update_category", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		opts := luaTableToGoMap(L.CheckTable(2))
		result, err := w.UpdateCategory(id, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_category(id) -> true or (nil, error)
	dbMod.RawSetString("delete_category", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteCategory(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.create_resource_category(opts) -> table or (nil, error)
	dbMod.RawSetString("create_resource_category", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateResourceCategory(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_resource_category(id, opts) -> table or (nil, error)
	dbMod.RawSetString("update_resource_category", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		opts := luaTableToGoMap(L.CheckTable(2))
		result, err := w.UpdateResourceCategory(id, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_resource_category(id) -> true or (nil, error)
	dbMod.RawSetString("delete_resource_category", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteResourceCategory(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.create_note_type(opts) -> table or (nil, error)
	dbMod.RawSetString("create_note_type", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateNoteType(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_note_type(id, opts) -> table or (nil, error)
	dbMod.RawSetString("update_note_type", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		opts := luaTableToGoMap(L.CheckTable(2))
		result, err := w.UpdateNoteType(id, opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_note_type(id) -> true or (nil, error)
	dbMod.RawSetString("delete_note_type", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteNoteType(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.create_group_relation(opts) -> table or (nil, error)
	dbMod.RawSetString("create_group_relation", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateGroupRelation(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_group_relation(opts) -> table or (nil, error)
	dbMod.RawSetString("update_group_relation", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.UpdateGroupRelation(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_group_relation(id) -> true or (nil, error)
	dbMod.RawSetString("delete_group_relation", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteGroupRelation(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.create_relation_type(opts) -> table or (nil, error)
	dbMod.RawSetString("create_relation_type", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.CreateRelationType(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.update_relation_type(opts) -> table or (nil, error)
	dbMod.RawSetString("update_relation_type", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		opts := luaTableToGoMap(L.CheckTable(1))
		result, err := w.UpdateRelationType(opts)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, result))
		return 1
	}))

	// mah.db.delete_relation_type(id) -> true or (nil, error)
	dbMod.RawSetString("delete_relation_type", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteRelationType(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.delete_resource(id) -> true or (nil, error)
	dbMod.RawSetString("delete_resource", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		id := uint(L.CheckNumber(1))
		if err := w.DeleteResource(id); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// --- Relationship management ---

	// mah.db.add_tags(entity_type, id, tag_ids) -> true or (nil, error)
	dbMod.RawSetString("add_tags", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := uint(L.CheckNumber(2))
		ids := luaTableToUintSlice(L.CheckTable(3))
		if err := w.AddTagsToEntity(entityType, id, ids); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.remove_tags(entity_type, id, tag_ids) -> true or (nil, error)
	dbMod.RawSetString("remove_tags", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := uint(L.CheckNumber(2))
		ids := luaTableToUintSlice(L.CheckTable(3))
		if err := w.RemoveTagsFromEntity(entityType, id, ids); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.add_groups(entity_type, id, group_ids) -> true or (nil, error)
	dbMod.RawSetString("add_groups", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := uint(L.CheckNumber(2))
		ids := luaTableToUintSlice(L.CheckTable(3))
		if err := w.AddGroupsToEntity(entityType, id, ids); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.remove_groups(entity_type, id, group_ids) -> true or (nil, error)
	dbMod.RawSetString("remove_groups", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		entityType := L.CheckString(1)
		id := uint(L.CheckNumber(2))
		ids := luaTableToUintSlice(L.CheckTable(3))
		if err := w.RemoveGroupsFromEntity(entityType, id, ids); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.add_resources_to_note(note_id, resource_ids) -> true or (nil, error)
	dbMod.RawSetString("add_resources_to_note", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		noteId := uint(L.CheckNumber(1))
		ids := luaTableToUintSlice(L.CheckTable(2))
		if err := w.AddResourcesToNote(noteId, ids); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	// mah.db.remove_resources_from_note(note_id, resource_ids) -> true or (nil, error)
	dbMod.RawSetString("remove_resources_from_note", L.NewFunction(func(L *lua.LState) int {
		w := pm.getDbWriter()
		if w == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("database writer not available"))
			return 2
		}
		noteId := uint(L.CheckNumber(1))
		ids := luaTableToUintSlice(L.CheckTable(2))
		if err := w.RemoveResourcesFromNote(noteId, ids); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LTrue)
		return 1
	}))

	mahMod.RawSetString("db", dbMod)
}

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
