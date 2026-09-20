package plugin_system

import (
	"context"
	"testing"
	"time"

	"mahresources/plugin_commands"

	lua "github.com/yuin/gopher-lua"
)

func TestRunViewToLuaOmitsBootSessionIdentity(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	table := runViewToLua(L, plugin_commands.RunView{RunRecord: plugin_commands.RunRecord{ID: "run-a"}})
	for _, key := range []string{"boot_session_id", "bootSessionID", "BootSessionID"} {
		if got := table.RawGetString(key); got != lua.LNil {
			t.Fatalf("runViewToLua exposed %q as %v", key, got)
		}
	}
}

func TestFSOperationsUseLiveActorAndReturnCompleteTables(t *testing.T) {
	actor := uint(55)
	started := time.Unix(100, 0).UTC()
	finished := time.Unix(200, 0).UTC()
	resource := uint(91)
	host := &commandLuaHost{
		runs: []plugin_commands.RunView{{RunRecord: plugin_commands.RunRecord{
			ID: "run-a", PluginName: "commander", CommandName: "download", Status: plugin_commands.RunStatusSucceeded,
			StartedAt: &started, FinishedAt: &finished, OutputUnverified: false,
		}, Imports: []plugin_commands.ImportMapEntry{{RunID: "run-a", FileName: "out.bin", ImportID: "import-a", Status: plugin_commands.ImportStatusFailed, Error: "decoder failed", SourceDeletePending: true}}}},
		listing:  plugin_commands.Listing{Entries: []plugin_commands.Entry{{Name: "out.bin", Size: 4, Modified: finished}}, Truncated: true},
		readBody: []byte("data"),
		imported: plugin_commands.ImportSubmitResult{ImportID: "import-b", CompletionRegistered: true},
	}
	pm, L := enableCommandPlugin(t, `"commands", "db:write"`, host)
	L.SetContext(withInvocation(context.Background(), NewInvocation(actor)))
	if err := L.DoString(`
__runs, __runs_err = mah.fs.runs()
__listing, __list_err = mah.fs.list("run-a")
__body, __read_err = mah.fs.read("run-a", "out.bin", 1024)
__import_id, __import_err = mah.fs.create_resource("run-a", "out.bin", {
  name="made", description="desc", tags={1,2}, groups={3}, meta={source="command"}
}, function(result) __import_result = result end)
__discard_ok, __discard_err = mah.fs.discard("run-a", "out.bin")
__discard_run_ok, __discard_run_err = mah.fs.discard_run("run-a")
`); err != nil {
		t.Fatal(err)
	}
	L.RemoveContext()

	if L.GetGlobal("__body").String() != "data" || L.GetGlobal("__import_id").String() != "import-b" {
		t.Fatalf("body/import = %v/%v", L.GetGlobal("__body"), L.GetGlobal("__import_id"))
	}
	runs := L.GetGlobal("__runs").(*lua.LTable)
	run := runs.RawGetInt(1).(*lua.LTable)
	if run.RawGetString("id").String() != "run-a" || run.RawGetString("command").String() != "download" || run.RawGetString("started_at") == lua.LNil {
		t.Fatalf("run table incomplete: %v", run)
	}
	imports := run.RawGetString("imports").(*lua.LTable)
	mapped := imports.RawGetString("out.bin").(*lua.LTable)
	if mapped.RawGetString("resource_id") != lua.LNil || mapped.RawGetString("status").String() != plugin_commands.ImportStatusFailed || mapped.RawGetString("error").String() != "decoder failed" || mapped.RawGetString("source_delete_pending") != lua.LTrue {
		t.Fatalf("import map incomplete: %v", mapped)
	}
	listing := L.GetGlobal("__listing").(*lua.LTable)
	if listing.RawGetString("truncated") != lua.LTrue || listing.RawGetString("entries").(*lua.LTable).RawGetInt(1).(*lua.LTable).RawGetString("name").String() != "out.bin" {
		t.Fatalf("listing incomplete: %v", listing)
	}

	host.mu.Lock()
	if len(host.imports) != 1 {
		host.mu.Unlock()
		t.Fatalf("imports = %d", len(host.imports))
	}
	request := host.imports[0]
	host.mu.Unlock()
	if request.Access.ActorUserID == nil || *request.Access.ActorUserID != actor || request.ActorUserID == nil || *request.ActorUserID != actor {
		t.Fatalf("import actor/access = %+v / %+v", request.ActorUserID, request.Access)
	}
	if request.Fields.Name != "made" || len(request.Fields.TagIDs) != 2 || len(request.Fields.GroupIDs) != 1 || request.Fields.Meta["source"] != "command" {
		t.Fatalf("fields = %+v", request.Fields)
	}

	request.Completion(plugin_commands.ImportResult{OK: true, ImportID: "import-b", ResourceID: &resource, SourceDeletePending: true})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu := pm.LockVM(L)
		if mu == nil {
			t.Fatal("plugin disappeared")
		}
		if got := L.GetGlobal("__import_result"); got != lua.LNil {
			tbl := got.(*lua.LTable)
			if tbl.RawGetString("run_id").String() != "run-a" || tbl.RawGetString("name").String() != "out.bin" || tbl.RawGetString("resource_id").String() != "91" || tbl.RawGetString("source_delete_pending") != lua.LTrue {
				mu.Unlock()
				t.Fatalf("import callback = %v", tbl)
			}
			mu.Unlock()
			return
		}
		mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("import callback did not run")
}

func TestFSCreateResourceRepeatedNonterminalImportReleasesCallbackOwnership(t *testing.T) {
	for _, status := range []string{plugin_commands.ImportStatusPending, plugin_commands.ImportStatusRunning} {
		t.Run(status, func(t *testing.T) {
			host := &commandLuaHost{imported: plugin_commands.ImportSubmitResult{ImportID: "existing-import"}}
			pm, L := enableCommandPlugin(t, `"commands", "db:write"`, host)
			if err := L.DoString(`
__existing, __err = mah.fs.create_resource("run-a", "out.bin", {}, function() __unexpected = true end)
`); err != nil {
				t.Fatal(err)
			}
			if got := L.GetGlobal("__existing").String(); got != "existing-import" {
				t.Fatalf("existing import id = %q", got)
			}
			settled := make(chan struct{})
			go func() {
				pm.actionWaitGroup("commander").Wait()
				close(settled)
			}()
			select {
			case <-settled:
			case <-time.After(time.Second):
				t.Fatal("repeated import retained callback lifecycle ownership")
			}
			host.mu.Lock()
			request := host.imports[0]
			host.mu.Unlock()
			request.Completion(plugin_commands.ImportResult{OK: true, ImportID: "existing-import"})
			time.Sleep(20 * time.Millisecond)
			if L.GetGlobal("__unexpected") != lua.LNil {
				t.Fatal("repeated import callback was promised and fired")
			}
			disabled := make(chan error, 1)
			go func() { disabled <- pm.DisablePlugin("commander") }()
			select {
			case err := <-disabled:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(time.Second):
				t.Fatal("plugin disable hung on repeated import callback ownership")
			}
		})
	}
}

func TestFSCreateResourceSynchronousSuccessDoesNotInvokeCallback(t *testing.T) {
	resource := uint(88)
	host := &commandLuaHost{imported: plugin_commands.ImportSubmitResult{ImportID: "old", ResourceID: &resource}}
	_, L := enableCommandPlugin(t, `"commands", "db:write"`, host)
	if err := L.DoString(`
__resource, __err = mah.fs.create_resource("run-a", "out.bin", {}, function() __unexpected = true end)
`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__resource").String() != "88" || L.GetGlobal("__unexpected") != lua.LNil {
		t.Fatalf("resource/callback = %v/%v", L.GetGlobal("__resource"), L.GetGlobal("__unexpected"))
	}
}

func TestFSReadRequiresExplicitMaxBytes(t *testing.T) {
	host := &commandLuaHost{readBody: []byte("data")}
	_, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`mah.fs.read("run-a", "out.bin")`); err == nil {
		t.Fatal("mah.fs.read accepted the undocumented two-argument form")
	}
}

func TestFSActorlessRunIsAvailableInAuthOffInvocation(t *testing.T) {
	host := &commandLuaHost{
		enforceAccess: true,
		runs: []plugin_commands.RunView{{RunRecord: plugin_commands.RunRecord{
			ID: "actorless", PluginName: "commander", CommandName: "download",
			Status: plugin_commands.RunStatusSucceeded, ActorlessAtSubmission: true,
		}}},
		listing:  plugin_commands.Listing{Entries: []plugin_commands.Entry{{Name: "out.bin", Size: 4}}},
		readBody: []byte("data"),
		imported: plugin_commands.ImportSubmitResult{ImportID: "import-auth-off"},
	}
	_, L := enableCommandPlugin(t, `"commands", "db:write"`, host)
	if err := L.DoString(`
__actorless_runs, __runs_err = mah.fs.runs()
__actorless_list, __list_err = mah.fs.list("actorless")
__actorless_body, __read_err = mah.fs.read("actorless", "out.bin", 16)
__actorless_import, __import_err = mah.fs.create_resource("actorless", "out.bin", {})
__actorless_discard, __discard_err = mah.fs.discard("actorless", "out.bin")
`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__runs_err") != lua.LNil || L.GetGlobal("__list_err") != lua.LNil ||
		L.GetGlobal("__read_err") != lua.LNil || L.GetGlobal("__import_err") != lua.LNil ||
		L.GetGlobal("__discard_err") != lua.LNil {
		t.Fatal("auth-off actorless exchange operation was refused")
	}
	if L.GetGlobal("__actorless_runs").(*lua.LTable).Len() != 1 || L.GetGlobal("__actorless_body").String() != "data" || L.GetGlobal("__actorless_import").String() != "import-auth-off" {
		t.Fatal("auth-off actorless exchange result was incomplete")
	}
}

func TestFSCallsFromCoroutineUseRootVM(t *testing.T) {
	host := &commandLuaHost{runs: []plugin_commands.RunView{{RunRecord: plugin_commands.RunRecord{ID: "run-a", PluginName: "commander", ActorlessAtSubmission: true}}}}
	_, L := enableCommandPlugin(t, `"commands"`, host)
	if err := L.DoString(`
local co = coroutine.create(function()
  __coroutine_runs, __coroutine_runs_err = mah.fs.runs()
end)
local ok, err = coroutine.resume(co)
assert(ok, err)
`); err != nil {
		t.Fatal(err)
	}
	if L.GetGlobal("__coroutine_runs_err") != lua.LNil || L.GetGlobal("__coroutine_runs").(*lua.LTable).Len() != 1 {
		t.Fatal("coroutine filesystem call did not use the root VM")
	}
}

func TestFSCreateResourceRejectsUnknownAndMalformedFields(t *testing.T) {
	host := &commandLuaHost{}
	_, L := enableCommandPlugin(t, `"commands", "db:write"`, host)
	for _, script := range []string{
		`mah.fs.create_resource("r", "f", {owner_id=1})`,
		`mah.fs.create_resource("r", "f", {tags={"1"}})`,
		`mah.fs.create_resource("r", "f", {name=7})`,
	} {
		if err := L.DoString(script); err == nil {
			t.Fatalf("invalid fields accepted: %s", script)
		}
	}
	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.imports) != 0 {
		t.Fatal("invalid create_resource reached host")
	}
}
