package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A preview that takes its content from the editor snapshot (no content
// argument) returns source that is JSON-escaped on the wire. A protected Name
// equal to that escaped spelling must not stop the run (review finding 3).
func TestPrivacyModeSnapshotPreviewWithEscapedTextIsNotALeak(t *testing.T) {
	f := newPrivacyFixture(t)
	source := `<p>Public heading "quoted"</p>`
	if err := f.tc.DB.Model(&models.Category{}).Where("id = ?", f.cat.ID).Update("custom_header", source).Error; err != nil {
		t.Fatal(err)
	}
	tag := &models.Tag{Name: `Public heading \"quoted\"`}
	if err := f.tc.DB.Create(tag).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.tc.DB.Model(f.child).Association("Tags").Append(tag); err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("v1", "preview", map[string]any{"field": "CustomHeader"}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomHeader": "<b>x</b>", "CustomHeaderCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", map[string]any{"slot": "CustomHeader", "target": "slot", "mode": "html", "content": source})
	if res.code != http.StatusOK || strings.Contains(string(res.body), "privacy_guard") {
		t.Fatalf("escaped readable text tripped the guard: %d %.300s", res.code, res.body)
	}
	if !strings.Contains(f.toolResults()["v1"], `quoted`) {
		t.Fatalf("the snapshot preview was sent: %v", f.toolResults()["v1"])
	}
}
