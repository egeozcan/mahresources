package plugin_system

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"sync"

	"mahresources/plugin_commands"

	lua "github.com/yuin/gopher-lua"
)

// CommandSubmitter is the application-owned command admission seam. The Lua
// runtime validates declarations and literal parameters before crossing it;
// execution and durable state stay below application_context.
type CommandSubmitter interface {
	SubmitPluginCommand(plugin_commands.CommandRequest) (string, error)
}

type commandAdmissionKey struct {
	plugin     string
	generation uint64
}

type commandAdmission struct {
	pluginName string
	generation uint64
	root       *lua.LState
	release    func()
}

// ClosePluginCommandAdmission linearizes plugin disable with command/import
// submission. It waits for host submissions already in progress and refuses any
// later call from the generation being disabled. A re-enabled plugin receives a
// new generation, so it does not inherit this closed gate.
func (pm *PluginManager) ClosePluginCommandAdmission(pluginName string) (uint64, bool) {
	pm.mu.RLock()
	var generation uint64
	for _, info := range pm.plugins {
		if info.Name == pluginName {
			generation = info.Generation
			break
		}
	}
	pm.mu.RUnlock()
	if generation == 0 {
		return 0, false
	}

	pm.commandAdmissionMu.Lock()
	pm.closedCommandAdmission[commandAdmissionKey{plugin: pluginName, generation: generation}] = struct{}{}
	pm.commandAdmissionMu.Unlock()
	return generation, true
}

// ReopenPluginCommandAdmission is used only when disable was refused while the
// same generation remains active. It never opens a replacement generation.
func (pm *PluginManager) ReopenPluginCommandAdmission(pluginName string, generation uint64) {
	if generation == 0 {
		return
	}
	pm.commandAdmissionMu.Lock()
	delete(pm.closedCommandAdmission, commandAdmissionKey{plugin: pluginName, generation: generation})
	pm.commandAdmissionMu.Unlock()
}

// SetCommandSubmitter publishes the process-lifetime command host. Until it is
// set, mah.commands fails closed instead of retaining work in Lua memory.
func (pm *PluginManager) SetCommandSubmitter(submitter CommandSubmitter) {
	if submitter != nil {
		pm.commandSubmitter.Store(submitter)
	}
}

func (pm *PluginManager) commandHost() CommandSubmitter {
	value := pm.commandSubmitter.Load()
	if value == nil {
		return nil
	}
	return value.(CommandSubmitter)
}

func (pm *PluginManager) registerCommandsAPI(L *lua.LState, mahMod *lua.LTable, declarations []plugin_commands.Declaration) {
	byName := make(map[string]plugin_commands.Declaration, len(declarations))
	for _, declaration := range declarations {
		copyOfDeclaration := declaration
		copyOfDeclaration.Argv = append([]string(nil), declaration.Argv...)
		copyOfDeclaration.SensitiveParams = append([]string(nil), declaration.SensitiveParams...)
		byName[declaration.Name] = copyOfDeclaration
	}

	module := L.NewTable()
	L.SetFuncs(module, map[string]lua.LGFunction{
		"run": func(L *lua.LState) int {
			if L.GetTop() < 2 || L.GetTop() > 3 {
				L.RaiseError("mah.commands.run expects name, params, and optional callback")
			}
			admission, err := pm.beginCommandCall(L)
			if err != nil {
				return pushLuaHostError(L, err)
			}
			defer admission.release()
			name := L.CheckString(1)
			declaration, ok := byName[name]
			if !ok {
				return pushLuaHostError(L, fmt.Errorf("command %q is not declared by this plugin", name))
			}
			params := checkCommandParams(L, 2)
			// Validate the complete declaration substitution before the durable
			// host sees the request. The real exchange directory is host-filled;
			// a nonempty sentinel exercises the same declaration branch.
			if _, err := plugin_commands.BuildInvocation(declaration, params, "/host/exchange"); err != nil {
				return pushLuaHostError(L, err)
			}

			var callback *lua.LFunction
			if L.GetTop() == 3 && L.Get(3) != lua.LNil {
				callback = L.CheckFunction(3)
			}
			actor := actorPointer(pm.actorFor(L))
			completion, release := pm.commandCompletion(admission.pluginName, admission.generation, admission.root, callback, actor)
			host := pm.commandHost()
			if host == nil {
				release()
				return pushLuaHostError(L, fmt.Errorf("plugin commands are not available until startup recovery completes"))
			}
			runID, err := host.SubmitPluginCommand(plugin_commands.CommandRequest{
				PluginName:       admission.pluginName,
				PluginGeneration: admission.generation,
				ActorUserID:      actor,
				Declaration:      declaration,
				Params:           params,
				Completion:       completion,
			})
			if err != nil {
				release()
				return pushLuaHostError(L, err)
			}
			L.Push(lua.LString(runID))
			return 1
		},
	})
	mahMod.RawSetString("commands", module)
}

func (pm *PluginManager) beginCommandCall(L *lua.LState) (commandAdmission, error) {
	goos := pm.commandRuntimeGOOS
	if goos == "" {
		goos = runtime.GOOS
	}
	if goos == "windows" {
		return commandAdmission{}, plugin_commands.ErrExchangeUnsupported
	}
	if pm.inTransaction(L) {
		return commandAdmission{}, fmt.Errorf("mah.commands and mah.fs are unavailable inside mah.db.transaction")
	}

	root := mainState(L)
	pm.commandAdmissionMu.RLock()
	pm.mu.RLock()
	generation, hasGeneration := pm.generations[root]
	_, hasVMLock := pm.vmLocks[root]
	pluginName := ""
	for i, state := range pm.states {
		if state == root && i < len(pm.plugins) && pm.plugins[i].Generation == generation {
			pluginName = pm.plugins[i].Name
			break
		}
	}
	pm.mu.RUnlock()
	if pluginName == "" || generation == 0 || !hasGeneration || !hasVMLock {
		pm.commandAdmissionMu.RUnlock()
		return commandAdmission{}, fmt.Errorf("plugin command generation is no longer active")
	}
	if _, closed := pm.closedCommandAdmission[commandAdmissionKey{plugin: pluginName, generation: generation}]; closed {
		pm.commandAdmissionMu.RUnlock()
		return commandAdmission{}, fmt.Errorf("plugin command generation is no longer active")
	}
	return commandAdmission{
		pluginName: pluginName,
		generation: generation,
		root:       root,
		release:    pm.commandAdmissionMu.RUnlock,
	}, nil
}

func checkCommandParams(L *lua.LState, index int) map[string]string {
	table := L.CheckTable(index)
	params := make(map[string]string)
	count := 0
	total := 0
	table.ForEach(func(key, value lua.LValue) {
		count++
		if count > plugin_commands.MaxParameters {
			L.RaiseError("command params exceed maximum of %d", plugin_commands.MaxParameters)
		}
		name, ok := key.(lua.LString)
		if !ok {
			L.RaiseError("command param keys must be strings")
		}
		literal, ok := value.(lua.LString)
		if !ok {
			L.RaiseError("command param %q must be a string", string(name))
		}
		if len(literal) > plugin_commands.MaxParameterBytes {
			L.RaiseError("command param %q exceeds %d bytes", string(name), plugin_commands.MaxParameterBytes)
		}
		total += len(literal)
		if total > plugin_commands.MaxAggregateParameterBytes {
			L.RaiseError("command params exceed aggregate limit of %d bytes", plugin_commands.MaxAggregateParameterBytes)
		}
		params[string(name)] = string(literal)
	})
	return params
}

func pushLuaHostError(L *lua.LState, err error) int {
	L.Push(lua.LNil)
	L.Push(lua.LString(err.Error()))
	return 2
}

func optionalLuaError(message string) any {
	if message == "" {
		return nil
	}
	return message
}

func actorPointer(actor uint) *uint {
	if actor == 0 {
		return nil
	}
	copyOfActor := actor
	return &copyOfActor
}

// commandCompletion makes a future callback part of the plugin's existing
// in-flight teardown barrier. release must be called when admission fails.
func (pm *PluginManager) commandCompletion(pluginName string, generation uint64, L *lua.LState, callback *lua.LFunction, actor *uint) (func(plugin_commands.Result), func()) {
	L = mainState(L)
	if callback == nil {
		return nil, func() {}
	}
	waitGroup := pm.actionWaitGroup(pluginName)
	waitGroup.Add(1)
	var settle sync.Once
	release := func() { settle.Do(waitGroup.Done) }
	completion := func(result plugin_commands.Result) {
		settle.Do(func() {
			go runProtectedDurableCallback("command", waitGroup.Done, func() {
				pm.runDurableCallback(pluginName, generation, L, callback, actor, map[string]any{
					"ok":        result.OK,
					"exit_code": result.ExitCode,
					"error":     optionalLuaError(result.Error),
					"run_id":    result.RunID,
				})
			})
		})
	}
	return completion, release
}

func runProtectedDurableCallback(kind string, done func(), callback func()) {
	defer done()
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[plugin] warning: %s callback panic: %v", kind, recovered)
		}
	}()
	callback()
}

func (pm *PluginManager) runDurableCallback(pluginName string, generation uint64, L *lua.LState, callback *lua.LFunction, actor *uint, result map[string]any) {
	L = mainState(L)
	if !pm.GenerationActive(pluginName, generation) {
		return
	}
	mu := pm.LockVM(L)
	if mu == nil {
		return
	}
	defer mu.Unlock()
	if !pm.GenerationActive(pluginName, generation) {
		return
	}

	actorID := uint(0)
	if actor != nil {
		actorID = *actor
	}
	timeout := pm.durableCallbackTimeout
	if timeout <= 0 {
		timeout = asyncActionTimeout
	}
	ctx, cancel := context.WithTimeout(withInvocation(context.Background(), NewInvocation(actorID)), timeout)
	defer cancel()
	L.SetContext(ctx)
	defer L.RemoveContext()
	if err := L.CallByParam(lua.P{Fn: callback, NRet: 0, Protect: true}, goToLuaTable(L, result)); err != nil {
		log.Printf("[plugin] warning: durable command callback error: %v", err)
	}
}
