package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// get_template, get_revision (full and by field) and diff_revisions on a
// resource category must not send AutoDetectRules unmasked, and must not
// register the mixed response as readable (round 3, #1).
func TestPrivacyModeResourceCategoryAutoDetectRulesAreMasked(t *testing.T) {
	f := newPrivacyFixture(t)
	rc := &models.ResourceCategory{
		Name:            "Scans READABLE_RC_NAME",
		CustomHeader:    "<p>READABLE_RC_SOURCE</p>",
		AutoDetectRules: `{"contentTypes":["image/png; name=PROT_ADR_CURRENT.png"]}`,
	}
	if err := f.tc.DB.Create(rc).Error; err != nil {
		t.Fatal(err)
	}
	for i, rules := range []string{`{"contentTypes":["image/png; name=PROT_ADR_ONE.png"]}`, `{"contentTypes":["image/png; name=PROT_ADR_TWO.png"]}`} {
		snapshot := fmt.Sprintf(`{"CustomHeader":"<p>READABLE_RC_REV_%d</p>","AutoDetectRules":%q}`, i+1, rules)
		row := &models.TemplateRevision{CarrierType: models.TemplateCarrierResourceCategory, CarrierID: rc.ID, Number: uint(i + 1), Source: "form",
			Snapshot: snapshot, Applied: `[]`, ChangedSlots: `["AutoDetectRules"]`}
		if err := f.tc.DB.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(
				call("a1", "get_template", map[string]any{"carrierType": "resourceCategory", "id": rc.ID}),
				call("a2", "get_template", map[string]any{"carrierType": "resourceCategory", "id": rc.ID, "fields": []string{"AutoDetectRules", "CustomHeader"}}),
				call("a3", "get_revision", map[string]any{"carrierType": "resourceCategory", "id": rc.ID, "number": 1}),
				call("a4", "get_revision", map[string]any{"carrierType": "resourceCategory", "id": rc.ID, "number": 2, "field": "AutoDetectRules"}),
				call("a5", "diff_revisions", map[string]any{"carrierType": "resourceCategory", "id": rc.ID, "from": 1, "to": 2}),
			)
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/resourceCategory/generateTemplate", map[string]any{"categoryId": rc.ID, "entityId": f.resource.ID})
	if res.code != http.StatusOK {
		t.Fatalf("run: %d %.300s", res.code, res.body)
	}
	bodies := f.allBodies()
	for _, marker := range []string{"PROT_ADR_CURRENT", "PROT_ADR_ONE", "PROT_ADR_TWO"} {
		if strings.Contains(bodies, marker) {
			t.Errorf("AutoDetectRules marker %q reached the provider", marker)
		}
	}
	results := f.toolResults()
	for _, id := range []string{"a1", "a2", "a3", "a4", "a5"} {
		if text := results[id]; text == "" || strings.HasPrefix(text, "error:") {
			t.Errorf("call %s did not succeed: %q", id, text)
		}
	}
	if !strings.Contains(results["a1"], "READABLE_RC_SOURCE") || !strings.Contains(results["a3"], "READABLE_RC_REV_1") {
		t.Errorf("the template slots stay readable: %s | %s", results["a1"], results["a3"])
	}
	if !strings.Contains(results["a1"], "⟦") || !strings.Contains(results["a4"], "⟦") || !strings.Contains(results["a5"], "⟦") {
		t.Errorf("AutoDetectRules shows Placeholders: %s", results["a1"])
	}
}
