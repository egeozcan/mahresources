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
	return falPreparationWithCDN(t, uri, nil, submittedURL...)
}

// falCDN answers the plugin's CDN calls in a preparation run. Without one the
// token request fails, which is the fallback the plugin must survive: the
// images go inline exactly as before.
type falCDN func(url string, body string, headers map[string]string) map[string]any

func falPreparationWithCDN(t testing.TB, uri string, cdn falCDN, submittedURL ...*string) func(string, string) (map[string]any, int, time.Duration) {
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
	pm.registerUtilModule(L, mah)
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
		url := L.CheckString(1)
		if !strings.HasPrefix(url, "https://queue.fal.run/") {
			response := map[string]any{"error": "no CDN in this test"}
			if cdn != nil {
				headers := map[string]string{}
				if opts, ok := L.Get(3).(*lua.LTable); ok {
					if h, ok := opts.RawGetString("headers").(*lua.LTable); ok {
						h.ForEach(func(k, v lua.LValue) { headers[k.String()] = v.String() })
					}
				}
				response = cdn(url, L.CheckString(2), headers)
			}
			L.Push(goToLuaValue(L, response))
			return 1
		}
		if len(submittedURL) > 0 {
			*submittedURL[0] = url
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
		{
			name:     "flux 3",
			params:   `{model="flux3", prompt="test", extra_images={1}, flux3_aspect_ratio="auto", flux3_resolution="2k", flux3_enable_prompt_expansion=false, flux3_safety_tolerance="2", flux3_output_format="png"}`,
			endpoint: "https://queue.fal.run/blackforestlabs/flux-3/edit-image",
			want:     []string{"image_urls", "aspect_ratio", "resolution", "enable_prompt_expansion", "safety_tolerance", "output_format"},
			absent:   []string{"seed", "image_size", "guidance_scale"},
		},
		{
			name:     "flux 3 klein, automatic size",
			params:   `{model="flux3klein", prompt="test", extra_images={1}, flux3klein_image_size="auto", flux3klein_enable_prompt_expansion=true, flux3klein_seed=42, flux3klein_enable_safety_checker=true, flux3klein_output_format="webp"}`,
			endpoint: "https://queue.fal.run/blackforestlabs/flux-3/klein/edit-image",
			want:     []string{"image_urls", "enable_prompt_expansion", "seed", "enable_safety_checker", "output_format", "num_images"},
			absent:   []string{"image_size", "safety_tolerance"},
		},
		{
			name:     "nano banana 2.1",
			params:   `{model="nanobanana21", prompt="test", extra_images={1}, nanobanana21_aspect_ratio="auto", nanobanana21_resolution="2K", nanobanana21_output_format="png", nanobanana21_safety_tolerance="4", nanobanana21_seed=42, nanobanana21_thinking_level="medium", nanobanana21_enable_web_search=false, nanobanana21_limit_generations=true}`,
			endpoint: "https://queue.fal.run/google/nano-banana-2.1/edit",
			want:     []string{"image_urls", "aspect_ratio", "resolution", "output_format", "safety_tolerance", "seed", "thinking_level", "enable_web_search", "limit_generations"},
		},
		{
			name:     "krea 3 jam drops the berry-only negative prompt",
			params:   `{model="krea3", prompt="test", extra_images={1}, krea3_model="jam", krea3_image_size="auto", krea3_reasoning_level="medium", krea3_negative_prompt="blur", krea3_seed=42, krea3_enable_safety_checker=true, krea3_output_format="jpeg"}`,
			endpoint: "https://queue.fal.run/krea/v3/edit",
			want:     []string{"image_urls", "model", "image_size", "reasoning_level", "seed", "enable_safety_checker", "output_format"},
			absent:   []string{"negative_prompt"},
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

func TestFalPreparationUploadsImagesToTheCDN(t *testing.T) {
	uri := encodeTestImageJPEG(t, 20, 20, color.RGBA{R: 255, A: 255})
	raw, err := base64.StdEncoding.DecodeString(strings.SplitN(uri, ",", 2)[1])
	if err != nil {
		t.Fatal(err)
	}
	var uploads []string
	cdn := func(url, body string, headers map[string]string) map[string]any {
		switch url {
		case "https://rest.fal.ai/storage/auth/token?storage_type=fal-cdn-v3":
			if headers["Authorization"] != "Key unused-test-key" {
				t.Errorf("token request authorization: %q", headers["Authorization"])
			}
			return map[string]any{"status_code": 200, "body": `{"token":"cdn-token","token_type":"Bearer","base_url":"https://v3b.fal.media"}`}
		case "https://v3b.fal.media/files/upload":
			if headers["Authorization"] != "Bearer cdn-token" || headers["Content-Type"] != "image/jpeg" {
				t.Errorf("upload headers: %v", headers)
			}
			if body != string(raw) {
				t.Error("upload body is not the image's bytes")
			}
			uploads = append(uploads, url)
			return map[string]any{"status_code": 200, "body": `{"access_url":"https://v3b.fal.media/files/b/x/input.jpg"}`}
		}
		t.Errorf("unexpected CDN call to %s", url)
		return map[string]any{"error": "unexpected"}
	}

	t.Run("single image", func(t *testing.T) {
		uploads = nil
		payload, _, _ := falPreparationWithCDN(t, uri, cdn)("colorize", `{}`)
		if payload["image_url"] != "https://v3b.fal.media/files/b/x/input.jpg" {
			t.Errorf("image_url = %.60v, want the CDN URL", payload["image_url"])
		}
		if len(uploads) != 1 {
			t.Errorf("uploads = %d, want 1", len(uploads))
		}
	})
	t.Run("repeated selection uploads once", func(t *testing.T) {
		uploads = nil
		payload, _, _ := falPreparationWithCDN(t, uri, cdn)("edit", `{model="flux2", prompt="test", extra_images={1,1}}`)
		urls := payload["image_urls"].([]any)
		if len(urls) != 2 || urls[0] != "https://v3b.fal.media/files/b/x/input.jpg" || urls[1] != urls[0] {
			t.Errorf("image_urls = %.120v", urls)
		}
		if len(uploads) != 1 {
			t.Errorf("uploads = %d, want 1", len(uploads))
		}
	})
	t.Run("failed upload sends the image inline", func(t *testing.T) {
		failing := func(url, body string, headers map[string]string) map[string]any {
			if strings.HasSuffix(url, "/files/upload") {
				return map[string]any{"status_code": 500, "body": "nope"}
			}
			return cdn(url, body, headers)
		}
		payload, _, _ := falPreparationWithCDN(t, uri, failing)("colorize", `{}`)
		if payload["image_url"] != uri {
			t.Error("a failed upload must leave the data URI in the request")
		}
	})
	t.Run("a token naming another host keeps the default upload host", func(t *testing.T) {
		var uploadedTo string
		odd := func(url, body string, headers map[string]string) map[string]any {
			if strings.Contains(url, "/storage/auth/token") {
				return map[string]any{"status_code": 200, "body": `{"token":"t","base_url":"https://evil.example"}`}
			}
			uploadedTo = url
			return map[string]any{"status_code": 200, "body": `{"access_url":"https://v3.fal.media/files/y.jpg"}`}
		}
		falPreparationWithCDN(t, uri, odd)("colorize", `{}`)
		if uploadedTo != "https://v3.fal.media/files/upload" {
			t.Errorf("uploaded to %q", uploadedTo)
		}
	})
}
