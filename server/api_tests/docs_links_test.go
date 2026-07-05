package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
)

func TestDocsLinks_RenderOverrideAndDisable(t *testing.T) {
	tc := SetupTestEnv(t)

	rr := tc.MakeRequest(http.MethodGet, "/mrql", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /mrql: want 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), application_context.DefaultDocsSiteBaseURL+"/features/mrql-reference") {
		t.Fatalf("default docs link missing from /mrql")
	}

	if err := tc.AppCtx.Settings().Set(application_context.KeyDocsSiteBaseURL, "https://docs.example.com/base/", "", ""); err != nil {
		t.Fatalf("set docs base: %v", err)
	}
	rr = tc.MakeRequest(http.MethodGet, "/mrql", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /mrql after override: want 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "https://docs.example.com/base/features/mrql-reference") {
		t.Fatalf("custom docs link missing from /mrql")
	}

	if err := tc.AppCtx.Settings().Set(application_context.KeyDocsLinksDisabled, "1", "", ""); err != nil {
		t.Fatalf("set docs disabled: %v", err)
	}
	rr = tc.MakeRequest(http.MethodGet, "/mrql", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /mrql after disable: want 200, got %d; body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "https://docs.example.com/base/features/mrql-reference") ||
		strings.Contains(rr.Body.String(), "Full reference") {
		t.Fatalf("docs link rendered even though docs_links_disabled=1")
	}
}
