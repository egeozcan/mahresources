package plugin_system

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fileQuerier serves resource files from disk, as the real adapter streams
// them from storage. A missing id reads as an error, as one outside the
// caller's scope does.
type fileQuerier struct {
	mockQuerier
	paths map[uint]string
}

func (q *fileQuerier) OpenResourceFile(id uint) (io.ReadCloser, int64, error) {
	path, ok := q.paths[id]
	if !ok {
		return nil, 0, fmt.Errorf("record not found")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// The plugin writes every call it receives into a report, one line each.
const jsonStreamPlugin = `
plugin = { name = "jsonstream", version = "1.0", description = "streams json", api_version = 1,
           capabilities = { "db:read", "pages" } }
local function show(v)
    if type(v) ~= "table" then return tostring(v) end
    return mah.json.encode(v)
end
function init()
    mah.page("read", function(ctx)
        local lines = {}
        local stop_after = tonumber(ctx.query.stop) or math.huge
        local ok, err = mah.db.each_resource_json(tonumber(ctx.query.id), function(key, i, value)
            lines[#lines + 1] = tostring(key) .. "|" .. tostring(i) .. "|" .. show(value)
            if #lines >= stop_after then return false end
        end)
        lines[#lines + 1] = "=> " .. tostring(ok) .. " " .. tostring(err)
        return table.concat(lines, "\n")
    end)
    mah.page("raise", function(ctx)
        local ok, err = pcall(mah.db.each_resource_json, tonumber(ctx.query.id), function() error("boom") end)
        return tostring(ok) .. " " .. tostring(err)
    end)
    mah.page("count", function(ctx)
        local n = 0
        local ok, err = mah.db.each_resource_json(tonumber(ctx.query.id), function() n = n + 1 end)
        return tostring(ok) .. " " .. tostring(err) .. " " .. n
    end)
end
`

func jsonStreamFixture(t *testing.T, files map[uint]string) *PluginManager {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "jsonstream", jsonStreamPlugin)
	paths := map[uint]string{}
	for id, body := range files {
		path := filepath.Join(t.TempDir(), fmt.Sprintf("%d.json", id))
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		paths[id] = path
	}
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	pm.SetEntityQuerier(&fileQuerier{paths: paths})
	if err := pm.EnablePlugin("jsonstream"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return pm
}

func jsonStreamPage(t *testing.T, pm *PluginManager, page string, query map[string]any) string {
	t.Helper()
	out, err := pm.HandlePage(context.Background(), "jsonstream", page, PageContext{Path: page, Method: "GET", Query: query})
	if err != nil {
		t.Fatalf("HandlePage(%s): %v", page, err)
	}
	return out
}

func TestEachResourceJSON(t *testing.T) {
	pm := jsonStreamFixture(t, map[uint]string{
		1: `{"name": "x", "items": [{"a": 1}, [2, 3], "s", null], "nested": {"deep": [1, {"b": true}]}, "empty": [], "n": 4.5}`,
		2: `[{"a": 1}, {"a": 2}]`,
		3: `{"items": [1, 2, {"broken": ]}`,
		4: `"just a string"`,
		5: `{"a": 1} {"b": 2}`,
	})

	tests := []struct {
		name  string
		query map[string]any
		want  string
	}{
		{"root object: arrays element by element, other members whole", map[string]any{"id": "1"},
			`name|nil|x
items|1|{"a":1}
items|2|[2,3]
items|3|s
items|4|nil
nested|nil|{"deep":[1,{"b":true}]}
n|nil|4.5
=> true nil`},
		{"root array", map[string]any{"id": "2"}, "nil|1|{\"a\":1}\nnil|2|{\"a\":2}\n=> true nil"},
		{"the callback stops it", map[string]any{"id": "2", "stop": "1"}, "nil|1|{\"a\":1}\n=> false nil"},
		{"invalid JSON after elements already delivered", map[string]any{"id": "3"}, "items|1|1\nitems|2|2\n=> nil not valid JSON"},
		{"a scalar root", map[string]any{"id": "4"}, "=> nil not a JSON object or array"},
		{"data after the document", map[string]any{"id": "5"}, "a|nil|1\n=> nil not valid JSON: data after the end of the document"},
		{"a resource the caller cannot read", map[string]any{"id": "99"}, "=> nil record not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonStreamPage(t, pm, "read", tt.query)
			if !strings.HasPrefix(got, tt.want) {
				t.Errorf("got\n%s\nwant prefix\n%s", got, tt.want)
			}
		})
	}

	// An error inside the callback is the callback's error, not a read failure.
	if got := jsonStreamPage(t, pm, "raise", map[string]any{"id": "2"}); !strings.HasPrefix(got, "false ") || !strings.Contains(got, "boom") {
		t.Errorf("raise: %q", got)
	}
}

// The point of streaming: reading a large file holds one element at a time,
// not the file. A ~64 MB array of objects is walked with the heap staying far
// below the file's size (decoding it whole into Lua tables takes several times
// the file's size).
func TestEachResourceJSONStreamsInBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 64 MB file")
	}
	path := filepath.Join(t.TempDir(), "big.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriter(f)
	const elements = 400000
	fmt.Fprint(w, `{"locations": [`)
	for i := 0; i < elements; i++ {
		if i > 0 {
			fmt.Fprint(w, ",")
		}
		fmt.Fprintf(w, `{"latitudeE7": %d, "longitudeE7": %d, "accuracy": 12, "timestamp": "2021-01-01T00:00:%02dZ", "source": "WIFI", "deviceTag": 123456789, "padding": "%s"}`,
			500000000+i, 80000000+i, i%60, strings.Repeat("x", 40))
	}
	fmt.Fprint(w, `]}`)
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	info, _ := os.Stat(path)

	dir := t.TempDir()
	writePlugin(t, dir, "jsonstream", jsonStreamPlugin)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	pm.SetEntityQuerier(&fileQuerier{paths: map[uint]string{1: path}})
	if err := pm.EnablePlugin("jsonstream"); err != nil {
		t.Fatal(err)
	}

	runtime.GC()
	var base runtime.MemStats
	runtime.ReadMemStats(&base)
	var peak atomic.Uint64
	done := make(chan struct{})
	go func() {
		var m runtime.MemStats
		for {
			select {
			case <-done:
				return
			case <-time.After(10 * time.Millisecond):
				runtime.ReadMemStats(&m)
				if m.HeapAlloc > peak.Load() {
					peak.Store(m.HeapAlloc)
				}
			}
		}
	}()
	got := jsonStreamPage(t, pm, "count", map[string]any{"id": "1"})
	close(done)

	if got != fmt.Sprintf("true nil %d", elements) {
		t.Fatalf("count = %q", got)
	}
	growth := int64(peak.Load()) - int64(base.HeapAlloc)
	t.Logf("file %d MB, peak heap growth %d MB", info.Size()>>20, growth>>20)
	if growth > info.Size()/2 {
		t.Errorf("heap grew by %d bytes streaming a %d-byte file; it should not scale with the file", growth, info.Size())
	}
}
