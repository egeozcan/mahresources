package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"mahresources/application_context"
)

func TestEntityPickerSlotsHTTPPartialUpdates(t *testing.T) {
	for _, carrier := range []struct{ path, table string }{
		{"/v1/category", "categories"}, {"/v1/resourceCategory", "resource_categories"}, {"/v1/note/noteType", "note_types"},
	} {
		for _, encoding := range []string{"json", "form"} {
			t.Run(carrier.table+"/"+encoding, func(t *testing.T) {
				tc := SetupTestEnv(t)
				post := func(body map[string]any) *httptest.ResponseRecorder {
					if encoding == "json" {
						return tc.MakeRequest(http.MethodPost, carrier.path, body)
					}
					form := url.Values{}
					for k, v := range body {
						form.Set(k, fmt.Sprint(v))
					}
					return tc.MakeFormRequest(http.MethodPost, carrier.path, form)
				}
				var id uint
				for i, step := range []struct {
					fields    map[string]any
					html, css string
				}{
					{map[string]any{"Name": "Picker", "CustomEntityPickerResult": "<b>Name</b>", "CustomEntityPickerResultCSS": ".picker{color:red}"}, "<b>Name</b>", ".picker{color:red}"},
					{map[string]any{"Description": "Preserve omitted slots"}, "<b>Name</b>", ".picker{color:red}"},
					{map[string]any{"CustomEntityPickerResult": "", "CustomEntityPickerResultCSS": ""}, "", ""},
				} {
					if id != 0 {
						step.fields["ID"] = id
					}
					resp := post(step.fields)
					if resp.Code != http.StatusOK {
						t.Fatalf("step %d: %d %s", i, resp.Code, resp.Body.String())
					}
					var response struct{ ID uint }
					if err := json.Unmarshal(resp.Body.Bytes(), &response); err != nil {
						t.Fatal(err)
					}
					id = response.ID
					var saved struct {
						ID                                                    uint
						CustomEntityPickerResult, CustomEntityPickerResultCSS string
					}
					if err := tc.DB.Table(carrier.table).First(&saved, id).Error; err != nil {
						t.Fatal(err)
					}
					if saved.CustomEntityPickerResult != step.html || saved.CustomEntityPickerResultCSS != step.css {
						t.Fatalf("step %d saved (%q, %q), want (%q, %q)", i, saved.CustomEntityPickerResult, saved.CustomEntityPickerResultCSS, step.html, step.css)
					}
				}
			})
		}
	}
}

func TestEntityPickerSlotsCanBeGenerated(t *testing.T) {
	for _, carrier := range []string{"category", "resourceCategory", "noteType"} {
		t.Run(carrier, func(t *testing.T) {
			tc := SetupTestEnv(t)
			tc.AppCtx.SetTemplateGenerator(&fakeAPITemplateGenerator{result: &application_context.TemplateGenerationResult{
				Target: application_context.TemplateTargetSlot, Valid: true,
				Slots: map[string]string{"CustomEntityPickerResult": "<b>Picker</b>", "CustomEntityPickerResultCSS": ".picker{color:red}"},
			}})
			resp := tc.MakeRequest(http.MethodPost, "/v1/"+carrier+"/generateTemplate", map[string]any{
				"target": "slot", "slot": "CustomEntityPickerResult", "prompt": "A picker result", "mode": "html",
			})
			if resp.Code != http.StatusOK {
				t.Fatalf("%d %s", resp.Code, resp.Body.String())
			}
			var result application_context.TemplateGenerationResult
			if err := json.Unmarshal(resp.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Slots["CustomEntityPickerResult"] != "<b>Picker</b>" || result.Slots["CustomEntityPickerResultCSS"] != ".picker{color:red}" {
				t.Fatalf("generated pair missing: %#v", result)
			}
		})
	}
}
