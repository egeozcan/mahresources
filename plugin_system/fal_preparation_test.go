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

func TestFalLatestEditModelsMapTheirLiveSchemas(t *testing.T) {
	uri := encodeTestImage(t, 20, 20, color.RGBA{R: 255, A: 255})
	for _, tc := range []struct {
		name, params, endpoint string
		want, absent           []string
	}{
		{
			name:     "gpt image 2.5",
			params:   `{model="gptimage25_flare", prompt="test", extra_images={1}, gptimage25_image_size="auto", gptimage25_quality="max", gptimage25_output_format="webp", gptimage25_output_compression=80, gptimage25_background="auto", gptimage25_mask_url="https://example.com/mask.png"}`,
			endpoint: "https://queue.fal.run/openai/gpt-image-2.5/flare/edit",
			want:     []string{"image_urls", "mask_url", "image_size", "quality", "output_format", "output_compression"},
		},
		{
			name:     "qwen image 3",
			params:   `{model="qwen3", prompt="test", extra_images={1}, qwen3_negative_prompt="avoid", qwen3_image_size="square", qwen3_enable_prompt_expansion=false, qwen3_enable_safety_checker=true, qwen3_output_format="webp", qwen3_seed=42}`,
			endpoint: "https://queue.fal.run/alibaba/qwen-image-3/edit",
			want:     []string{"image_urls", "negative_prompt", "image_size", "enable_prompt_expansion", "enable_safety_checker", "output_format", "seed"},
		},
		{
			name:     "mai image 2.5",
			params:   `{model="mai25", prompt="test", extra_images={1}, mai25_aspect_ratio="16:9", mai25_output_format="jpeg"}`,
			endpoint: "https://queue.fal.run/microsoft/mai-image-2.5/edit",
			want:     []string{"image_url", "aspect_ratio", "output_format"},
			absent:   []string{"image_urls"},
		},
		{
			name:     "ideogram v4",
			params:   `{model="ideogram_v4", prompt="test", extra_images={1}, ideogram_v4_expansion_model="None", ideogram_v4_image_size="auto", ideogram_v4_rendering_speed="QUALITY", ideogram_v4_acceleration="high", ideogram_v4_strength=0.6, ideogram_v4_seed=42, ideogram_v4_enable_safety_checker=true, ideogram_v4_output_format="png"}`,
			endpoint: "https://queue.fal.run/ideogram/v4/image-to-image",
			want:     []string{"image_url", "expansion_model", "image_size", "rendering_speed", "acceleration", "strength", "seed", "enable_safety_checker", "output_format"},
			absent:   []string{"image_urls"},
		},
		{
			name:     "seedream 5 lite",
			params:   `{model="seedream5lite", prompt="test", extra_images={1}, seedream5lite_image_size="auto_3K", seedream5lite_enable_safety_checker=true}`,
			endpoint: "https://queue.fal.run/bytedance/seedream/v5/lite/edit",
			want:     []string{"image_urls", "image_size", "enable_safety_checker"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var submittedURL string
			run := falPreparation(t, uri, &submittedURL)
			payload, _, _ := run("edit", tc.params)
			if submittedURL != tc.endpoint {
				t.Fatalf("endpoint: got %q, want %q", submittedURL, tc.endpoint)
			}
			for _, key := range tc.want {
				if _, ok := payload[key]; !ok {
					t.Errorf("payload is missing live-schema field %q: %v", key, payload)
				}
			}
			for _, key := range tc.absent {
				if _, ok := payload[key]; ok {
					t.Errorf("payload contains field %q that this model does not accept: %v", key, payload)
				}
			}
		})
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
