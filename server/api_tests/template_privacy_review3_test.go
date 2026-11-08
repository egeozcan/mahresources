package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// The model's own Replay carries a leaf that equals a protected value (it
// reasoned about the word). Replay is model output, so it is readable and the
// resend must not stop the run (review finding 12).
func TestPrivacyModeReplayLeafEqualToAProtectedValueIsNotALeak(t *testing.T) {
	f := newPrivacyFixture(t)
	tag := &models.Tag{Name: "Quarterly plans"}
	if err := f.tc.DB.Create(tag).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.tc.DB.Model(f.child).Association("Tags").Append(tag); err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			resp := chatToolCalls(call("g1", "get_entity", map[string]any{"type": "group", "id": f.child.ID}))
			message := resp["choices"].([]any)[0].(map[string]any)["message"].(map[string]any)
			message["reasoning_content"] = "Quarterly plans"
			return resp
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("a Replay leaf equal to a protected value stopped the run: %d %.300s", res.code, res.body)
	}
	if !strings.Contains(f.allBodies(), "Quarterly plans") {
		t.Fatal("the Replay was resent verbatim, so the value is in the second request")
	}
}
