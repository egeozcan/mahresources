package plugin_system

import (
	lua "github.com/yuin/gopher-lua"
)

// EntityQuerier provides read-only access to entities for plugins.
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

	mahMod.RawSetString("db", dbMod)
}
