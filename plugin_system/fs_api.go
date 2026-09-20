package plugin_system

import (
	"fmt"
	"sync"
	"time"

	"mahresources/plugin_commands"

	lua "github.com/yuin/gopher-lua"
)

// ExchangeMediator is the application-owned filesystem/import seam. It owns
// authorization, descriptor-safe file access, leases, and durable imports.
type ExchangeMediator interface {
	CommandRuns(plugin_commands.Access) ([]plugin_commands.RunView, error)
	ListCommandFiles(plugin_commands.Access, string) (plugin_commands.Listing, error)
	ReadCommandFile(plugin_commands.Access, string, string, int64) ([]byte, error)
	SubmitCommandImport(plugin_commands.ImportSubmission) (plugin_commands.ImportSubmitResult, error)
	DiscardCommandFile(plugin_commands.Access, string, string) error
	DiscardCommandRun(plugin_commands.Access, string) error
}

// SetExchangeMediator publishes the recovered exchange/import host.
func (pm *PluginManager) SetExchangeMediator(mediator ExchangeMediator) {
	if mediator != nil {
		pm.exchangeMediator.Store(mediator)
	}
}

func (pm *PluginManager) exchangeHost() ExchangeMediator {
	value := pm.exchangeMediator.Load()
	if value == nil {
		return nil
	}
	return value.(ExchangeMediator)
}

func (pm *PluginManager) registerFSAPI(L *lua.LState, mahMod *lua.LTable, canCreateResource bool) {
	module := L.NewTable()
	functions := map[string]lua.LGFunction{
		"runs": func(L *lua.LState) int {
			checkExactArgs(L, "mah.fs.runs", 0)
			host, access, err := pm.exchangeCall(L)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			defer access.release()
			runs, err := host.CommandRuns(access.Access)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			result := L.NewTable()
			for i, run := range runs {
				result.RawSetInt(i+1, runViewToLua(L, run))
			}
			L.Push(result)
			return 1
		},
		"list": func(L *lua.LState) int {
			checkExactArgs(L, "mah.fs.list", 1)
			host, access, err := pm.exchangeCall(L)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			defer access.release()
			listing, err := host.ListCommandFiles(access.Access, L.CheckString(1))
			if err != nil {
				return pushLuaHostError(L, err)
			}
			L.Push(listingToLua(L, listing))
			return 1
		},
		"read": func(L *lua.LState) int {
			checkExactArgs(L, "mah.fs.read", 3)
			host, access, err := pm.exchangeCall(L)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			defer access.release()
			value := L.CheckNumber(3)
			if value <= 0 || value != lua.LNumber(int64(value)) || int64(value) > plugin_commands.MaxReadBytes {
				L.ArgError(3, fmt.Sprintf("max_bytes must be a positive whole number no greater than %d", plugin_commands.MaxReadBytes))
			}
			maxBytes := int64(value)
			body, err := host.ReadCommandFile(access.Access, L.CheckString(1), L.CheckString(2), maxBytes)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			L.Push(lua.LString(string(body)))
			return 1
		},
		"discard": func(L *lua.LState) int {
			checkExactArgs(L, "mah.fs.discard", 2)
			host, access, err := pm.exchangeCall(L)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			defer access.release()
			if err := host.DiscardCommandFile(access.Access, L.CheckString(1), L.CheckString(2)); err != nil {
				return pushLuaHostError(L, err)
			}
			L.Push(lua.LTrue)
			return 1
		},
		"discard_run": func(L *lua.LState) int {
			checkExactArgs(L, "mah.fs.discard_run", 1)
			host, access, err := pm.exchangeCall(L)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			defer access.release()
			if err := host.DiscardCommandRun(access.Access, L.CheckString(1)); err != nil {
				return pushLuaHostError(L, err)
			}
			L.Push(lua.LTrue)
			return 1
		},
	}
	if canCreateResource {
		functions["create_resource"] = pm.createResourceFromExchange
	}
	L.SetFuncs(module, functions)
	mahMod.RawSetString("fs", module)
}

type liveExchangeAccess struct {
	plugin_commands.Access
	Generation uint64
	root       *lua.LState
	release    func()
}

func (pm *PluginManager) exchangeCall(L *lua.LState) (ExchangeMediator, liveExchangeAccess, error) {
	admission, err := pm.beginCommandCall(L)
	if err != nil {
		return nil, liveExchangeAccess{}, err
	}
	host := pm.exchangeHost()
	if host == nil {
		admission.release()
		return nil, liveExchangeAccess{}, fmt.Errorf("%s", commandRuntimeUnavailableMessage)
	}
	return host, liveExchangeAccess{Access: plugin_commands.Access{
		PluginName:  admission.pluginName,
		ActorUserID: actorPointer(pm.actorFor(L)),
	}, Generation: admission.generation, root: admission.root, release: admission.release}, nil
}

func (pm *PluginManager) createResourceFromExchange(L *lua.LState) int {
	if L.GetTop() < 3 || L.GetTop() > 4 {
		L.RaiseError("mah.fs.create_resource expects run_id, name, fields, and optional callback")
	}
	host, access, err := pm.exchangeCall(L)
	if err != nil {
		return pushLuaHostError(L, err)
	}
	defer access.release()
	fields := checkImportFields(L, 3)
	var callback *lua.LFunction
	if L.GetTop() == 4 && L.Get(4) != lua.LNil {
		callback = L.CheckFunction(4)
	}
	completion, release := pm.importCompletion(access, access.root, callback, L.CheckString(1), L.CheckString(2))
	result, err := host.SubmitCommandImport(plugin_commands.ImportSubmission{
		Access:           access.Access,
		RunID:            L.CheckString(1),
		Name:             L.CheckString(2),
		Fields:           fields,
		PluginGeneration: access.Generation,
		ActorUserID:      cloneUintPointer(access.ActorUserID),
		Completion:       completion,
	})
	if err != nil {
		release()
		return pushLuaHostError(L, err)
	}
	if !result.CompletionRegistered {
		// A terminal replay and a repeated pending/running submission both have
		// another owner (or no future work). This invocation registered no
		// callback, so it must release its VM-lifecycle reference immediately.
		release()
	}
	if result.ResourceID != nil {
		L.Push(lua.LNumber(*result.ResourceID))
		return 1
	}
	L.Push(lua.LString(result.ImportID))
	return 1
}

func checkImportFields(L *lua.LState, index int) plugin_commands.ResourceFields {
	table := L.CheckTable(index)
	checkEntityIDOpts(L, index, table)
	allowed := map[string]bool{"name": true, "description": true, "tags": true, "groups": true, "meta": true}
	table.ForEach(func(key, _ lua.LValue) {
		name, ok := key.(lua.LString)
		if !ok || !allowed[string(name)] {
			L.ArgError(index, fmt.Sprintf("unknown resource field %q", key.String()))
		}
	})

	fields := plugin_commands.ResourceFields{}
	if value := table.RawGetString("name"); value != lua.LNil {
		literal, ok := value.(lua.LString)
		if !ok {
			L.ArgError(index, "name must be a string")
		}
		fields.Name = string(literal)
	}
	if value := table.RawGetString("description"); value != lua.LNil {
		literal, ok := value.(lua.LString)
		if !ok {
			L.ArgError(index, "description must be a string")
		}
		fields.Description = string(literal)
	}
	if value := table.RawGetString("tags"); value != lua.LNil {
		ids, ok := value.(*lua.LTable)
		if !ok {
			L.ArgError(index, "tags must be an array of ids")
		}
		checkEntityIDList(L, index, ids)
		fields.TagIDs = luaTableToUintSlice(ids)
	}
	if value := table.RawGetString("groups"); value != lua.LNil {
		ids, ok := value.(*lua.LTable)
		if !ok {
			L.ArgError(index, "groups must be an array of ids")
		}
		checkEntityIDList(L, index, ids)
		fields.GroupIDs = luaTableToUintSlice(ids)
	}
	if value := table.RawGetString("meta"); value != lua.LNil {
		meta, ok := value.(*lua.LTable)
		if !ok {
			L.ArgError(index, "meta must be a table")
		}
		fields.Meta = luaTableToGoMap(meta)
	}
	return fields
}

func (pm *PluginManager) importCompletion(access liveExchangeAccess, L *lua.LState, callback *lua.LFunction, runID, name string) (func(plugin_commands.ImportResult), func()) {
	L = mainState(L)
	if callback == nil {
		return nil, func() {}
	}
	waitGroup := pm.actionWaitGroup(access.PluginName)
	waitGroup.Add(1)
	var settle sync.Once
	release := func() { settle.Do(waitGroup.Done) }
	completion := func(result plugin_commands.ImportResult) {
		settle.Do(func() {
			go runProtectedDurableCallback("import", waitGroup.Done, func() {
				pm.runDurableCallback(access.PluginName, access.Generation, L, callback, access.ActorUserID, map[string]any{
					"ok":                    result.OK,
					"error":                 optionalLuaError(result.Error),
					"import_id":             result.ImportID,
					"resource_id":           result.ResourceID,
					"source_delete_pending": result.SourceDeletePending,
					"run_id":                runID,
					"name":                  name,
				})
			})
		})
	}
	return completion, release
}

func runViewToLua(L *lua.LState, run plugin_commands.RunView) *lua.LTable {
	table := goToLuaTable(L, map[string]any{
		"id":                run.ID,
		"command":           run.CommandName,
		"status":            run.Status,
		"started_at":        formatOptionalTime(run.StartedAt),
		"finished_at":       formatOptionalTime(run.FinishedAt),
		"exit_code":         run.ExitCode,
		"error":             optionalLuaError(run.Error),
		"output_unverified": run.OutputUnverified,
	})
	imports := L.NewTable()
	for _, item := range run.Imports {
		imports.RawSetString(item.FileName, goToLuaTable(L, map[string]any{
			"import_id":             item.ImportID,
			"resource_id":           item.ResourceID,
			"status":                item.Status,
			"error":                 optionalLuaError(item.Error),
			"source_delete_pending": item.SourceDeletePending,
		}))
	}
	table.RawSetString("imports", imports)
	return table
}

func listingToLua(L *lua.LState, listing plugin_commands.Listing) *lua.LTable {
	entries := L.NewTable()
	for i, entry := range listing.Entries {
		entries.RawSetInt(i+1, goToLuaTable(L, map[string]any{
			"name":     entry.Name,
			"size":     entry.Size,
			"modified": entry.Modified.UTC().Format(time.RFC3339Nano),
		}))
	}
	result := L.NewTable()
	result.RawSetString("entries", entries)
	result.RawSetString("truncated", lua.LBool(listing.Truncated))
	return result
}

func formatOptionalTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func cloneUintPointer(value *uint) *uint {
	if value == nil {
		return nil
	}
	copyOfValue := *value
	return &copyOfValue
}

func checkExactArgs(L *lua.LState, name string, count int) {
	if L.GetTop() != count {
		L.RaiseError("%s expects exactly %d arguments", name, count)
	}
}
