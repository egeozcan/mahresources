package plugin_system

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/plugin_commands"
)

// commandIntegrationHost records the Lua-to-host contract and withholds the
// asynchronous import completion. This package-level fixture proves callback
// table shape and that waiting for a later completion does not retain the VM;
// server/api_tests covers the production dispatcher and a real blocked import.
type commandIntegrationHost struct {
	*commandLuaHost
	importQueued chan struct{}
	discarded    chan struct{}
	importOnce   sync.Once
	discardOnce  sync.Once
}

func (h *commandIntegrationHost) SubmitCommandImport(sub plugin_commands.ImportSubmission) (plugin_commands.ImportSubmitResult, error) {
	h.mu.Lock()
	h.imports = append(h.imports, sub)
	h.mu.Unlock()
	h.importOnce.Do(func() { close(h.importQueued) })
	return plugin_commands.ImportSubmitResult{ImportID: "integration-import"}, nil
}

func (h *commandIntegrationHost) DiscardCommandFile(access plugin_commands.Access, runID, name string) error {
	if err := h.commandLuaHost.DiscardCommandFile(access, runID, name); err != nil {
		return err
	}
	h.discardOnce.Do(func() { close(h.discarded) })
	return nil
}

func TestPluginCommandLuaContractQueuesWorkAndReleasesVMLockBeforeCompletion(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "command-integration", `
plugin = {
  name = "command-integration", version = "1", api_version = 1,
  capabilities = {"commands", "db:write", "pages"},
  commands = {{name = "produce", argv = {"fixture-command", "{{exchange_dir}}"}, timeout = 60}}
}

local completion_result = nil
local listing_result = nil
local import_result = nil

function init()
  mah.page("start", function()
    local run_id, err = mah.commands.run("produce", {}, function(result)
      completion_result = {
        ok = result.ok,
        run_id = result.run_id,
        exit_code_type = type(result.exit_code),
        error_type = type(result.error),
      }
      local listing, list_err = mah.fs.list(result.run_id)
      assert(listing, list_err)
      listing_result = listing
      for _, entry in ipairs(listing.entries) do
        if entry.name == "import.bin" then
          local import_id, import_err = mah.fs.create_resource(result.run_id, entry.name, {
            name = "command integration resource",
            description = "created from the exchange folder",
            meta = {source = "command-integration"},
          }, function(imported)
            import_result = imported
          end)
          assert(import_id, import_err)
        elseif entry.name == "discard.txt" then
          local discarded, discard_err = mah.fs.discard(result.run_id, entry.name)
          assert(discarded, discard_err)
        end
      end
    end)
    assert(run_id, err)
    return run_id
  end)

  mah.page("status", function()
    return mah.json.encode({
      completion = completion_result,
      listing = listing_result,
      imported = import_result,
    })
  end)
end
`)

	base := &commandLuaHost{
		listing: plugin_commands.Listing{Entries: []plugin_commands.Entry{
			{Name: "import.bin", Size: 7, Modified: time.Unix(100, 0)},
			{Name: "discard.txt", Size: 8, Modified: time.Unix(101, 0)},
		}},
	}
	host := &commandIntegrationHost{
		commandLuaHost: base,
		importQueued:   make(chan struct{}),
		discarded:      make(chan struct{}),
	}
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	pm.SetCommandSubmitter(host)
	pm.SetExchangeMediator(host)
	store := newSharedConsentStore()
	discovered := pm.GetDiscoveredPlugin("command-integration")
	if discovered == nil {
		t.Fatal("integration plugin was not discovered")
	}
	grants, err := GrantsForEnable(discovered.Manifest, true)
	if err != nil {
		t.Fatal(err)
	}
	store.records["command-integration"] = grants
	pm.SetConsentStore(store)
	if err := pm.EnablePlugin("command-integration"); err != nil {
		t.Fatal(err)
	}

	started, err := pm.HandlePage(context.Background(), "command-integration", "start", PageContext{Method: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	if started != "run-123" {
		t.Fatalf("start page = %q", started)
	}
	host.mu.Lock()
	if len(host.requests) != 1 {
		host.mu.Unlock()
		t.Fatalf("submitted commands = %d", len(host.requests))
	}
	request := host.requests[0]
	host.mu.Unlock()

	request.Completion(plugin_commands.Result{OK: true, RunID: "run-123"})
	select {
	case <-host.importQueued:
	case <-time.After(3 * time.Second):
		t.Fatal("completion callback did not queue the import")
	}
	select {
	case <-host.discarded:
	case <-time.After(3 * time.Second):
		t.Fatal("completion callback did not discard the unwanted file")
	}

	// The asynchronous completion is deliberately withheld. The real host's
	// byte-transfer boundary is exercised with a blocked AddResource destination
	// in server/api_tests/plugin_command_integration_test.go.
	pageDone := make(chan struct {
		body string
		err  error
	}, 1)
	go func() {
		body, err := pm.HandlePage(context.Background(), "command-integration", "status", PageContext{Method: "GET"})
		pageDone <- struct {
			body string
			err  error
		}{body: body, err: err}
	}()
	select {
	case got := <-pageDone:
		if got.err != nil {
			t.Fatal(got.err)
		}
		for _, want := range []string{`"ok":true`, `"run_id":"run-123"`, `"exit_code_type":"nil"`, `"error_type":"nil"`, `"name":"import.bin"`, `"truncated":false`} {
			if !strings.Contains(got.body, want) {
				t.Fatalf("status page %q does not contain %q", got.body, want)
			}
		}
	case <-time.After(time.Second):
		t.Fatal("plugin page waited for the pending asynchronous import completion")
	}

	host.mu.Lock()
	if len(host.imports) != 1 {
		host.mu.Unlock()
		t.Fatalf("queued imports = %d", len(host.imports))
	}
	importRequest := host.imports[0]
	discards := append([]string(nil), host.discards...)
	host.mu.Unlock()
	if len(discards) != 1 || discards[0] != "run-123/discard.txt" {
		t.Fatalf("discard operations = %v", discards)
	}
	resourceID := uint(91)
	importRequest.Completion(plugin_commands.ImportResult{OK: true, ImportID: "integration-import", ResourceID: &resourceID})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		body, pageErr := pm.HandlePage(context.Background(), "command-integration", "status", PageContext{Method: "GET"})
		if pageErr != nil {
			t.Fatal(pageErr)
		}
		if strings.Contains(body, `"import_id":"integration-import"`) &&
			strings.Contains(body, `"run_id":"run-123"`) &&
			strings.Contains(body, `"name":"import.bin"`) &&
			strings.Contains(body, `"resource_id":91`) &&
			strings.Contains(body, `"ok":true`) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("documented import callback table did not reach Lua")
}
