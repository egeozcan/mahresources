package api_tests

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"mahresources/models"
)

// A Note with an explicitly stored zero StartDate (non-nil) is a present instant:
// the preview shows the stand-in, default and formatted, and the stored
// 0001-01-01 never reaches the provider (round 8, #1).
func TestPrivacyModeStoredZeroDateIsTheStandIn(t *testing.T) {
	f := newPrivacyFixture(t)
	var zero time.Time
	note := &models.Note{Name: "Dated note", OwnerId: &f.child.ID, StartDate: &zero}
	if err := f.tc.DB.Create(note).Error; err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("zd", "preview", map[string]any{
				"field": "CustomHeader", "entityType": "note", "entityId": note.ID,
				"content": `[property path="StartDate"]|[property path="StartDate" format="date"]|[property path="EndDate" default="none"]`,
			}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("%d %.300s", res.code, res.body)
	}
	out := f.toolResults()["zd"]
	if !strings.Contains(out, "2001-02-03T04:05:06Z") || strings.Count(out, "2001-02-03") < 2 {
		t.Fatalf("the present zero date renders as the stand-in, default and formatted: %s", out)
	}
	if !strings.Contains(out, "none") {
		t.Fatalf("a nil EndDate stays absent: %s", out)
	}
	if strings.Contains(f.allBodies(), "0001-01-01") {
		t.Fatal("the stored zero instant reached the provider")
	}
}
