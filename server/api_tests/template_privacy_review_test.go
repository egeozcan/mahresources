package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A protected Name equal to a server refusal text must not stop the run when
// that refusal is later sent (review finding 4).
func TestPrivacyModeRefusalTextEqualToAProtectedNameIsNotALeak(t *testing.T) {
	f := newPrivacyFixture(t)
	refusal := "no such entity, or it is not readable by this account"
	if err := f.tc.DB.Create(&models.Tag{Name: refusal}).Error; err != nil {
		t.Fatal(err)
	}
	var tag models.Tag
	f.tc.DB.Where("name = ?", refusal).First(&tag)
	if err := f.tc.DB.Model(f.child).Association("Tags").Append(&tag); err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		switch n {
		case 1:
			return chatToolCalls(
				call("e1", "get_entity", map[string]any{"type": "group", "id": f.child.ID}),
				call("e2", "get_entity", map[string]any{"type": "group", "id": 999999}),
				call("e3", "preview", map[string]any{"field": "CustomHeader", "content": `[mrql saved="x"]`, "entityType": "group", "entityId": f.child.ID}),
			)
		default:
			return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
		}
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("a refusal text equal to a protected value stopped the run: %d %s", res.code, res.body)
	}
	if !strings.Contains(f.toolResults()["e2"], refusal) {
		t.Fatalf("the refusal was sent: %v", f.toolResults()["e2"])
	}
}

// A Seed that hits a masker limit answers 400 and publishes no Disclosure (finding 9).
func TestPrivacyModeSeedOverflowAnswers400OnTheDryRun(t *testing.T) {
	f := newPrivacyFixture(t)
	f.script = harnessScript(f)
	f.install(t, "harness")
	if err := f.tc.DB.Model(&models.Group{}).Where("id = ?", f.child.ID).Update("description", strings.Repeat("x", 300<<10)).Error; err != nil {
		t.Fatal(err)
	}
	rec := f.tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", f.requestBody(map[string]any{"dryRun": true}))
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "too much entity data for Privacy mode") {
		t.Fatalf("dry run: %d %.200s", rec.Code, rec.Body.String())
	}
}
