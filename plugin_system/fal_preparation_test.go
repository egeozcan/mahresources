package plugin_system

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"math/rand/v2"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func falBenchmarkImage(t testing.TB) string {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2048, 2048))
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = byte(rng.Uint32())
		img.Pix[i+1] = byte(rng.Uint32())
		img.Pix[i+2] = byte(rng.Uint32())
		img.Pix[i+3] = 255
	}
	var buf bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// Run the bundled action up to its first HTTP request, using the real image
// and JSON bindings. Storage is in memory and HTTP is stopped before any I/O.
func falPreparation(t testing.TB, uri string, submittedURL ...*string) func(string, string) (map[string]any, int, time.Duration) {
	t.Helper()
	L := lua.NewState()
	t.Cleanup(L.Close)
	if err := L.DoString(`
		actions = {}
		mah = {
			log = function() end, doc = function() end, menu = function() end,
			page = function() end, job_progress = function() end, job_fail = function() end,
			get_setting = function() return "unused-test-key" end,
			action = function(a) actions[a.id] = a.handler end,
			db = {}, http = {}
		}
	`); err != nil {
		t.Fatal(err)
	}
	mah := L.GetGlobal("mah").(*lua.LTable)
	pm := &PluginManager{}
	pm.registerImageModule(L, mah)
	pm.registerJsonModule(L, mah)
	reads := 0
	db := mah.RawGetString("db").(*lua.LTable)
	db.RawSetString("get_resource_data", L.NewFunction(func(L *lua.LState) int {
		reads++
		prefix, data, _ := strings.Cut(uri, ",")
		L.Push(lua.LString(data))
		L.Push(lua.LString(strings.TrimSuffix(strings.TrimPrefix(prefix, "data:"), ";base64")))
		return 2
	}))
	var payload map[string]any
	mah.RawGetString("http").(*lua.LTable).RawSetString("post_sync", L.NewFunction(func(L *lua.LState) int {
		if len(submittedURL) > 0 {
			*submittedURL[0] = L.CheckString(1)
		}
		if err := json.Unmarshal([]byte(L.CheckString(2)), &payload); err != nil {
			t.Fatal(err)
		}
		L.Push(goToLuaValue(L, map[string]any{"error": "preparation test stops before network"}))
		return 1
	}))
	if err := L.DoFile(filepath.Join("..", "plugins", "fal-ai", "plugin.lua")); err != nil {
		t.Fatal(err)
	}
	if err := L.DoString("init()"); err != nil {
		t.Fatal(err)
	}
	return func(action, params string) (map[string]any, int, time.Duration) {
		reads, payload = 0, nil
		start := time.Now()
		if err := L.DoString(`actions["` + action + `"]({entity_id=1, job_id="test", params=` + params + `})`); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(start)
		if payload == nil {
			t.Fatal("action failed before reaching HTTP submission")
		}
		return payload, reads, elapsed
	}
}

func TestFalPreparationReadsSelectedImageOnce(t *testing.T) {
	uri := encodeTestImage(t, 20, 20, color.RGBA{R: 255, A: 255})
	for _, tc := range []struct {
		name, params  string
		reads, images int
	}{
		{"default selection", `{model="flux2", prompt="test", extra_images={1}}`, 1, 1},
		{"repeated selection", `{model="flux2", prompt="test", extra_images={1,2,2}}`, 2, 3},
		{"empty selection", `{model="flux2", prompt="test", extra_images={}}`, 1, 1},
		{"absent selection", `{model="flux2", prompt="test"}`, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			run := falPreparation(t, uri)
			payload, reads, elapsed := run("edit", tc.params)
			t.Logf("preparation: %s, resource reads: %d", elapsed, reads)
			if reads != tc.reads {
				t.Errorf("resource reads: got %d, want %d", reads, tc.reads)
			}
			urls := payload["image_urls"].([]any)
			if len(urls) != tc.images {
				t.Fatalf("image count: got %d, want %d", len(urls), tc.images)
			}
			for _, url := range urls {
				if url != uri {
					t.Error("selected image was not preserved in the request")
				}
			}
		})
	}
}

func TestFalPreparationPreservesMatchingImage(t *testing.T) {
	uri := encodeTestImageJPEG(t, 200, 200, color.RGBA{R: 255, A: 255})
	run := falPreparation(t, uri)
	payload, _, elapsed := run("restore", `{model="photo_restoration", aspect_ratio="1:1"}`)
	t.Logf("preparation: %s", elapsed)
	if payload["image_url"] != uri {
		t.Error("matching image was re-encoded despite needing no padding")
	}
}

func BenchmarkFalPreparation(b *testing.B) {
	// Setup is excluded: these measure the actual Lua preparation stage.
	for _, ratio := range []string{"1:1", "4:3"} {
		b.Run(ratio, func(b *testing.B) {
			// A deterministic high-detail PNG avoids an unrealistically cheap
			// solid-color compression workload.
			uri := falBenchmarkImage(b)
			run := falPreparation(b, uri)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				run("restore", `{model="photo_restoration", aspect_ratio="`+ratio+`"}`)
			}
		})
	}
}
