package api_tests

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
)

// get_shortcode_docs returns the built-in docs, whose property example says
// default="Unassigned". A sampled entity with that name is masked, and the docs
// are readable, so the second request is sent (round 5, #1).
func TestPrivacyModeShortcodeDocsAreReadableInAMetaSchemaRun(t *testing.T) {
	f := newPrivacyFixture(t)
	if err := f.tc.DB.Create(&models.Group{Name: "Unassigned", CategoryId: &f.cat.ID}).Error; err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("sd", "get_shortcode_docs", map[string]any{"name": "property"}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"MetaSchema": `{"type":"object","properties":{"title":{"type":"string"}}}`}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", map[string]any{"target": "metaschema", "slot": "", "mode": "json"})
	if res.code != http.StatusOK {
		t.Fatalf("the docs example equal to a sample name stopped the run: %d %.300s", res.code, res.body)
	}
	if f.calls != 2 || !strings.Contains(f.toolResults()["sd"], "Unassigned") {
		t.Fatalf("the second request must reach the transport carrying the docs (calls=%d)", f.calls)
	}
	// The sample name itself is masked wherever the Seed shows it.
	seed := string(f.bodies[0])
	if strings.Contains(seed, `\"Name\":\"Unassigned\"`) {
		t.Fatal("the sample name must be masked in the Seed")
	}
}

// Access revoked after the run: neither the legend nor the real value leaves, on
// a finished run and on a privacy_guard failure, in JSON and SSE (round 5, #5).
func TestPrivacyModeRevokedAccessDropsTheLegendAndTheValue(t *testing.T) {
	const value = "REVOKE-VALUE-5588"
	leak := &application_context.TemplatePrivacyLeakError{Path: "messages[2].content"}
	for _, failure := range []bool{false, true} {
		for _, mode := range []string{"json", "sse"} {
			name := mode + "/finished"
			if failure {
				name = mode + "/privacy_guard"
			}
			t.Run(name, func(t *testing.T) {
				tc := SetupTestEnv(t)
				cat := &models.Category{Name: "Revoke"}
				if err := tc.DB.Create(cat).Error; err != nil {
					t.Fatal(err)
				}
				if err := tc.DB.Create(&models.Group{Name: value, CategoryId: &cat.ID}).Error; err != nil {
					t.Fatal(err)
				}
				access := &flipAccess{tc: tc}
				var armed atomic.Bool // the failing Turn revokes access only once armed
				step := func(_ context.Context, n int, _ application_context.ConversationRequest) (application_context.ConversationResponse, error) {
					switch {
					case n == 1:
						return toolReply(apiToolCall("l1", application_context.ToolLint, map[string]any{"field": "CustomCSS", "content": ".x{}"})), nil
					case failure:
						if armed.Load() {
							access.revoked.Store(true) // revoked while the failing Turn is in flight
						}
						return application_context.ConversationResponse{}, leak
					}
					return toolReply(apiSubmit("s1", map[string]string{"CustomCSS": ".draft{color:red}"}, true)), nil
				}
				obs := installObserved(tc, &fakeConversation{step: step}, 5)
				body := func() map[string]any {
					b := harnessSlotBody()
					delete(b, "privacyOff") // Privacy mode on
					b["categoryId"] = cat.ID
					return b
				}
				principal := auth.SystemPrincipal()
				hash := hashOf(t, tc, principal, body())
				var header http.Header
				if mode == "sse" {
					header = streamHeader
				}

				// Positive control: before any revocation the response carries the legend entry.
				control := callGenerate(tc, access.factory(), principal, withHash(body(), hash), header)
				access.revoked.Store(false)
				if !strings.Contains(control.Body.String(), `"legend"`) || !strings.Contains(control.Body.String(), value) {
					t.Fatalf("control: an unrevoked response carries its legend and the value:\n%.600s", control.Body.String())
				}

				// Revoked: the finished run is revoked after it ends; the failing run, during it.
				if failure {
					armed.Store(true)
				} else {
					obs.afterRun = func() { access.revoked.Store(true) }
				}
				rr := callGenerate(tc, access.factory(), principal, withHash(body(), hash), header)
				got := rr.Body.String()
				for _, leaked := range []string{`"legend"`, value, `"transcript"`} {
					if strings.Contains(got, leaked) {
						t.Fatalf("a revoked response leaked %q:\n%.600s", leaked, got)
					}
				}
				if mode == "json" && rr.Code != http.StatusForbidden {
					t.Fatalf("want 403, got %d %s", rr.Code, got)
				}
			})
		}
	}
}
