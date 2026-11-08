package plugin_system

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

const notAllowedInModelPreview = "not allowed in a model preview"

type previewWriter struct {
	mockQuerier
	stubWriter
	deletedNotes []uint
}

func (p *previewWriter) DeleteNote(id uint) error {
	p.deletedNotes = append(p.deletedNotes, id)
	return nil
}

// previewCases are shortcode bodies a model-written template could reach when
// the plugin opt-in is on. Each one writes, registers or starts work.
var previewRefusedCases = map[string]string{
	"db_write":      `mah.db.delete_note(1)`,
	"media_trim":    `mah.media.trim(1, 0, 1)`,
	"job_progress":  `mah.job_progress("j", 10, "x")`,
	"job_complete":  `mah.job_complete("j", {})`,
	"job_fail":      `mah.job_fail("j", "x")`,
	"inject":        `mah.inject("page_bottom", function() return "" end)`,
	"page":          `mah.page("p", function() return "" end)`,
	"shortcode":     `mah.shortcode({name = "x", label = "X", render = function() return "" end})`,
	"block_type":    `mah.block_type({type = "x", label = "X", render_view = function() return "" end, render_edit = function() return "" end})`,
	"display_type":  `mah.display_type({type = "x", label = "X", render = function() return "" end})`,
	"on":            `mah.on("after_note_create", function() end)`,
	"menu":          `mah.menu("m", "/m")`,
	"kv_set":        `mah.kv.set("k", "v")`,
	"http_get":      `mah.http.get("http://example.invalid/")`,
	"sleep":         `mah.sleep(0)`,
	"abort":         `mah.abort("x")`,
	"db_trans":      `mah.db.transaction(function() end)`,
	"action":        `mah.action({id = "a", label = "A", entity = "note", handler = function() end})`,
	"schedule":      `mah.schedule({id = "s", every = "1h", handler = function() end})`,
	"start_job":     `mah.start_job({id = "s"})`,
	"fs_read":       `mah.fs.read("a", "b")`,
	"commands_run":  `mah.commands.run("x", {})`,
	"create_in_db":  `mah.db.create_tag({name = "t"})`,
	"mrql_query_ro": `mah.db.mrql_query("type=note", {})`,
}

func previewPlugin(t *testing.T, w *previewWriter) *PluginManager {
	t.Helper()
	dir := t.TempDir()
	caps := `"` + strings.Join(AllCapabilities, `","`) + `"`
	var b strings.Builder
	fmt.Fprintf(&b, "plugin = { name = \"pv\", version = \"1.0\", api_version = 1, capabilities = { %s } }\nfunction init()\n", caps)
	names := make([]string, 0, len(previewRefusedCases))
	for n := range previewRefusedCases {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&b, "mah.shortcode({name = %q, label = %q, render = function(ctx) %s; return \"ran\" end})\n", "c_"+strings.ReplaceAll(n, "_", "-"), n, previewRefusedCases[n])
	}
	b.WriteString(`mah.shortcode({name = "reader", label = "r", render = function(ctx) local g = mah.db.get_group(1); return g.name end})
mah.shortcode({name = "logger", label = "l", render = function(ctx) mah.log("info", "SENTINEL-LOG"); return "ok" end})
mah.shortcode({name = "documenter", label = "d", render = function(ctx) mah.doc({name = "late-doc", label = "Late"}); return "ok" end})
`)
	b.WriteString("end\n")
	writePlugin(t, dir, "pv", b.String())
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	pm.SetEntityQuerier(w)
	pm.SetEntityWriter(w)
	pm.SetMRQLExecutor(&mockMRQLExecutor{})
	if err := pm.EnablePlugin("pv"); err != nil {
		t.Fatal(err)
	}
	return pm
}

func renderPV(pm *PluginManager, ctx context.Context, name string) (string, error) {
	return pm.RenderShortcode(ctx, "pv", "plugin:pv:"+name, "group", 1, json.RawMessage(`{}`), map[string]string{}, nil, "", false)
}

func TestModelPreview_RefusesEveryNonReader(t *testing.T) {
	w := &previewWriter{}
	pm := previewPlugin(t, w)
	marked := WithModelPreview(context.Background())
	injectionsBefore := len(pm.injections["page_bottom"])
	for n := range previewRefusedCases {
		_, err := renderPV(pm, marked, "c_"+strings.ReplaceAll(n, "_", "-"))
		if err == nil || !strings.Contains(err.Error(), notAllowedInModelPreview) {
			t.Errorf("%s: err = %v, want %q", n, err, notAllowedInModelPreview)
		}
	}
	if len(w.deletedNotes) != 0 {
		t.Errorf("a model preview deleted notes %v", w.deletedNotes)
	}
	if got := len(pm.injections["page_bottom"]); got != injectionsBefore {
		t.Errorf("injections changed %d -> %d", injectionsBefore, got)
	}
}

func TestModelPreview_ReadersRenderAndHumanPreviewsAreUnchanged(t *testing.T) {
	w := &previewWriter{}
	pm := previewPlugin(t, w)
	got, err := renderPV(pm, WithModelPreview(context.Background()), "reader")
	if err != nil || got != "Test Group" {
		t.Fatalf("model preview get_group = %q, %v", got, err)
	}
	// The same write from a human preview behaves as today.
	if _, err := renderPV(pm, context.Background(), "c_db-write"); err != nil {
		t.Fatalf("human preview delete_note: %v", err)
	}
	if len(w.deletedNotes) != 1 {
		t.Errorf("human preview deletes = %v, want one", w.deletedNotes)
	}
}

type countingLogger struct{ entries []string }

func (c *countingLogger) PluginLog(plugin, level, message string, _ map[string]any) {
	c.entries = append(c.entries, message)
}

func TestModelPreview_LogWritesNoRow(t *testing.T) {
	pm := previewPlugin(t, &previewWriter{})
	logger := &countingLogger{}
	pm.SetPluginLogger(logger)
	if _, err := renderPV(pm, WithModelPreview(context.Background()), "logger"); err != nil {
		t.Fatal(err)
	}
	if len(logger.entries) != 0 {
		t.Fatalf("model preview logged %v", logger.entries)
	}
	if _, err := renderPV(pm, context.Background(), "logger"); err != nil {
		t.Fatal(err)
	}
	if len(logger.entries) != 1 {
		t.Fatalf("human preview logged %v, want one entry", logger.entries)
	}
}

func TestModelPreview_RegistrationBypassingTheTableIsRefused(t *testing.T) {
	pm := previewPlugin(t, &previewWriter{})
	if _, err := renderPV(pm, WithModelPreview(context.Background()), "documenter"); err != nil {
		t.Fatal(err)
	}
	if n := len(pm.docs["pv"]); n != 0 {
		t.Fatalf("a model preview registered %d docs", n)
	}
	if _, err := renderPV(pm, context.Background(), "documenter"); err != nil {
		t.Fatal(err)
	}
	if n := len(pm.docs["pv"]); n != 1 {
		t.Fatalf("a human preview registered %d docs, want 1", n)
	}
}

func flattenMah(tbl *lua.LTable, prefix string, out map[string]*lua.LFunction) {
	tbl.ForEach(func(k, v lua.LValue) {
		switch x := v.(type) {
		case *lua.LTable:
			flattenMah(x, prefix+k.String()+".", out)
		case *lua.LFunction:
			out[prefix+k.String()] = x
		}
	})
}

// Drift guard: a plugin granted every capability. Any function in the live mah
// table must be on the allow-list or refuse under the mark, so a function added
// later cannot silently become reachable from a model preview.
func TestModelPreview_EveryLiveFunctionIsAllowListedOrRefused(t *testing.T) {
	dir := t.TempDir()
	caps := `"` + strings.Join(AllCapabilities, `","`) + `"`
	pm := mustEnable(t, dir, "all", fmt.Sprintf(`plugin = { name = "all", version = "1.0", api_version = 1, capabilities = { %s } }`, caps))
	L := stateForPlugin(t, pm, "all")
	live := map[string]*lua.LFunction{}
	flattenMah(L.GetGlobal("mah").(*lua.LTable), "", live)
	if len(live) < 100 {
		t.Fatalf("walked only %d functions; the mah table did not load fully", len(live))
	}
	for name := range modelPreviewAllowed {
		if _, ok := live[name]; !ok {
			t.Errorf("allow-list names %q, which no longer exists", name)
		}
	}
	L.SetContext(WithModelPreview(context.Background()))
	defer L.RemoveContext()
	for name, fn := range live {
		if modelPreviewAllowed[name] {
			continue
		}
		err := L.CallByParam(lua.P{Fn: fn, NRet: 0, Protect: true})
		if err == nil || !strings.Contains(err.Error(), notAllowedInModelPreview) {
			t.Errorf("mah.%s is neither allow-listed nor refused under the mark (err = %v)", name, err)
		}
	}
}
