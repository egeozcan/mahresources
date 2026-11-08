package api_tests

import (
	"net/http"
	"testing"

	"mahresources/models"
)

// A preview without content renders the editor's sibling slot. Its source was
// never in a Seed summary or a model argument, so the preview must register it
// (round 2, #7).
func TestPrivacyModeOmittedContentPreviewRegistersItsResolvedSource(t *testing.T) {
	f := newPrivacyFixture(t)
	tag := &models.Tag{Name: "SolarLoom"}
	if err := f.tc.DB.Create(tag).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.tc.DB.Model(f.child).Association("Tags").Append(tag); err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("v1", "preview", map[string]any{"field": "CustomSidebar"}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomHeader": "<b>x</b>", "CustomHeaderCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", map[string]any{
		"slot": "CustomHeader", "target": "slot", "mode": "html",
		"siblingSlots": `{"CustomSidebar":"<b>SolarLoom</b>"}`,
	})
	if res.code != http.StatusOK {
		t.Fatalf("a preview of the editor's own sibling source tripped the guard: %d %.300s", res.code, res.body)
	}
}
