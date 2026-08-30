package plugin_system

import (
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// recordingSubmitter stands in for the host queue.
type recordingSubmitter struct {
	mu   sync.Mutex
	opts map[string]any
	name string
}

func (r *recordingSubmitter) SubmitDownload(pluginName string, actorUserID uint, url string, opts map[string]any) (map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.name, r.opts = pluginName, opts
	return map[string]any{"id": "job-1", "url": url, "status": "pending"}, nil
}

func (r *recordingSubmitter) read() (string, map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.name, r.opts
}

// TestDownloadSubmitForwardsAHeadersTable crosses the Lua boundary rather than
// calling the host helper directly. A `headers` table an author writes is a
// nested table, and the host reads it as map[string]any -- so the conversion is
// a real link in the chain, and a test that starts below it would stay green
// while `headers = {...}` reached the queue as nothing at all.
func TestDownloadSubmitForwardsAHeadersTable(t *testing.T) {
	pm := mustEnable(t, t.TempDir(), "dl", `
plugin = { name = "dl", version = "1.0", api_version = 1, capabilities = { "db:write" } }
function init() end
`)
	sink := &recordingSubmitter{}
	pm.SetDownloadSubmitter(sink)

	L := stateForPlugin(t, pm, "dl")
	if err := L.DoString(`
__job, __err = mah.download.submit("https://example.invalid/v.mp4", {
  owner_id = 5,
  headers = { Referer = "https://example.invalid/watch" },
})
`); err != nil {
		t.Fatalf("mah.download.submit: %v", err)
	}
	if errVal := L.GetGlobal("__err"); errVal != lua.LNil {
		t.Fatalf("submit reported %v", errVal)
	}

	name, opts := sink.read()
	if name != "dl" {
		t.Errorf("the submission named plugin %q", name)
	}
	headers, ok := opts["headers"].(map[string]any)
	if !ok {
		t.Fatalf("the host received headers as %T (%v)", opts["headers"], opts["headers"])
	}
	if headers["Referer"] != "https://example.invalid/watch" {
		t.Fatalf("the host received headers %v", headers)
	}
}
