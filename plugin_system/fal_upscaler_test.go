package plugin_system

import (
	"image/color"
	"regexp"
	"strings"
	"testing"
)

func TestFalRecraftUpscalersPreparePNG(t *testing.T) {
	for _, model := range []string{"crisp", "creative"} {
		for _, format := range []string{"png", "jpeg"} {
			t.Run(model+"/"+format, func(t *testing.T) {
				uri := encodeTestImage(t, 23, 17, color.RGBA{R: 100, A: 128})
				if format == "jpeg" {
					uri = encodeTestImageJPEG(t, 23, 17, color.RGBA{R: 100, A: 255})
				}
				var submittedURL string
				run := falPreparation(t, uri, &submittedURL)
				payload, reads, _ := run("upscale", `{model="recraft_`+model+`", clarity_prompt="must not leak", bria_desired_increase="4"}`)
				if want := "https://queue.fal.run/fal-ai/recraft/upscale/" + model; submittedURL != want {
					t.Fatalf("submitted to %q, want %q", submittedURL, want)
				}
				prepared := payload["image_url"].(string)
				if !strings.HasPrefix(prepared, "data:image/png;base64,") {
					t.Fatal("Recraft did not receive PNG")
				}
				if format == "png" && prepared != uri {
					t.Error("existing PNG should remain byte-for-byte unchanged")
				}
				original, converted := decodeDataURI(t, uri), decodeDataURI(t, prepared)
				if original.Bounds() != converted.Bounds() {
					t.Fatal("PNG preparation changed dimensions")
				}
				for y := 0; y < 17; y++ {
					for x := 0; x < 23; x++ {
						if color.NRGBAModel.Convert(original.At(x, y)) != color.NRGBAModel.Convert(converted.At(x, y)) {
							t.Fatalf("PNG preparation changed decoded pixel at %d,%d", x, y)
						}
					}
				}
				if reads != 1 || len(payload) != 1 {
					t.Errorf("unexpected resource reads or unsupported parameters: reads=%d payload=%v", reads, payload)
				}
			})
		}
	}
}

func TestFalBriaIncreaseResolutionOptions(t *testing.T) {
	uri := encodeTestImage(t, 23, 17, color.RGBA{R: 100, A: 128})
	var submittedURL string
	run := falPreparation(t, uri, &submittedURL)
	payload, _, _ := run("upscale", `{model="bria", bria_desired_increase="4", bria_output_type="png", bria_precision_preserve_alpha=false, bria_preserve_color="true", clarity_prompt="must not leak"}`)
	if submittedURL != "https://queue.fal.run/bria/increase-resolution" {
		t.Fatalf("wrong endpoint: %s", submittedURL)
	}
	if payload["desired_increase"] != float64(4) || payload["output_type"] != "png" || payload["preserve_alpha"] != false || payload["preserve_color"] != true || payload["image_url"] != uri || len(payload) != 5 {
		t.Errorf("Bria options not mapped to the API schema: %v", payload)
	}
}

func TestFalUpscalerGuidesIncludeDatedPrices(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("fal-ai"); err != nil {
		t.Fatal(err)
	}
	checked := regexp.MustCompile(`checked \d{4}-\d{2}-\d{2}`)
	for _, action := range pm.GetActions("resource", nil) {
		if action.PluginName != "fal-ai" || action.ID != "upscale" {
			continue
		}
		guides := map[string]string{}
		var models []string
		for _, param := range action.Params {
			if param.Name == "model" {
				models = param.Options
			}
			if param.Type == "info" && strings.HasPrefix(param.Name, "model_info_") {
				for _, model := range showWhenValues(&param, "model") {
					guides[model] = param.Description
				}
			}
		}
		for _, model := range models {
			description := guides[model]
			if !strings.Contains(description, "Last known fal.ai cost (USD;") || !checked.MatchString(description) || !strings.Contains(description, "Published rate") {
				t.Errorf("%s needs a dated price, currency, and estimate caveat: %s", model, description)
			}
		}
		for model, fragments := range map[string][]string{
			"seedvr":            {"$0.001/MP", "$0.0025/MP", "seamless"},
			"topaz_generative":  {"started 8 output MP", "started 4 output MP", "Wonder 3/3.5"},
			"topaz":             {"started 24 output MP"},
			"topaz_transparent": {"started 24 output MP"},
			"topaz_creative":    {"started 2 output MP"},
			"esrgan":            {"compute second", "runtime"},
			"creative":          {"compute second", "runtime"},
			"aura_sr":           {"compute second", "runtime"},
		} {
			for _, fragment := range fragments {
				if !strings.Contains(guides[model], fragment) {
					t.Errorf("%s price is missing %q", model, fragment)
				}
			}
		}
		return
	}
	t.Fatal("upscale action was not registered")
}
