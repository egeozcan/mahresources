package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A partial's description equal to a heading in its own source: get_partial
// without a preview must not stop the next request (round 4, #1).
func TestPrivacyModeGetPartialSourceIsReadable(t *testing.T) {
	f := newPrivacyFixture(t)
	if err := f.tc.DB.Create(&models.TemplatePartial{Name: "READABLE_hp", Description: "Welcome Aboard Friends", Content: "<h1>Welcome Aboard Friends</h1>"}).Error; err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("gp", "get_partial", map[string]any{"name": "READABLE_hp"}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("get_partial with a description equal to its source text stopped the run: %d %.300s", res.code, res.body)
	}
	if !strings.Contains(f.toolResults()["gp"], "<h1>Welcome Aboard Friends</h1>") {
		t.Fatalf("the partial source is sent: %s", f.toolResults()["gp"])
	}
}

// The root group is resolved from the database beyond the preloaded owners, and
// [link to="root"] prints its ID. A numeric protected Name equal to that ID must
// not stop the request (round 4, #2).
func TestPrivacyModeRootIDInALinkIsAPreservedScalar(t *testing.T) {
	f := newPrivacyFixture(t)
	create := func(v any) {
		t.Helper()
		if err := f.tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	top := &models.Group{ID: 7384951, Name: "chain top"}
	create(top)
	// Deeper than the owners an entity read preloads, so the root is resolved
	// from the database and never visited by Entity.
	owner := top
	for i := 0; i < 6; i++ {
		mid := &models.Group{Name: fmt.Sprintf("chain link %d", i), OwnerId: &owner.ID}
		create(mid)
		owner = mid
	}
	leaf := &models.Group{Name: "chain leaf", OwnerId: &owner.ID, CategoryId: &f.cat.ID}
	create(leaf)
	tag := &models.Tag{Name: fmt.Sprint(top.ID)}
	create(tag)
	if err := f.tc.DB.Model(leaf).Association("Tags").Append(tag); err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(
				call("lk", "preview", map[string]any{"field": "CustomHeader", "content": `<a href="[link to="root"]">root</a>`, "entityType": "group", "entityId": leaf.ID}),
				call("tg", "get_entity", map[string]any{"type": "group", "id": leaf.ID}),
			)
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("a link to the root group's ID stopped the run: %d %.300s", res.code, res.body)
	}
	if !strings.Contains(f.toolResults()["lk"], "7384951") {
		t.Fatalf("the link prints the root ID: %s", f.toolResults()["lk"])
	}
}
