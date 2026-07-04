package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"
)

type lintIssue struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type lintResponse struct {
	Issues []lintIssue `json:"issues"`
}

func lintContent(t *testing.T, tc *TestContext, content string) lintResponse {
	t.Helper()
	rr := tc.MakeRequest(http.MethodPost, "/v1/shortcodes/lint", map[string]any{"content": content})
	if rr.Code != http.StatusOK {
		t.Fatalf("lint: expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp lintResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("lint: decode failed: %v", err)
	}
	return resp
}

func TestShortcodeLintEndpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	// Clean template — no issues.
	if resp := lintContent(t, tc, `[meta path="x"]`); len(resp.Issues) != 0 {
		t.Errorf("expected no issues for clean template, got %+v", resp.Issues)
	}

	// Broken conditional — missing operator and unclosed block.
	resp := lintContent(t, tc, `[conditional path="x"]hello`)
	var sawError bool
	for _, iss := range resp.Issues {
		if iss.Severity == "error" {
			sawError = true
		}
	}
	if !sawError {
		t.Errorf("expected at least one error for broken conditional, got %+v", resp.Issues)
	}

	// MRQL syntax error surfaces through the endpoint.
	resp = lintContent(t, tc, `[mrql query="this is not valid mrql !!!"]`)
	var sawMRQL bool
	for _, iss := range resp.Issues {
		if iss.Severity == "error" {
			sawMRQL = true
		}
	}
	if !sawMRQL {
		t.Errorf("expected an MRQL syntax error, got %+v", resp.Issues)
	}
}

type shortcodeDocAttr struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Default  string   `json:"default"`
	Enum     []string `json:"enum"`
	Wildcard bool     `json:"wildcard"`
}

type shortcodeDoc struct {
	Name        string             `json:"name"`
	Syntax      string             `json:"syntax"`
	Description string             `json:"description"`
	IsBlock     string             `json:"isBlock"`
	Source      string             `json:"source"`
	Attrs       []shortcodeDocAttr `json:"attrs"`
	Examples    []struct {
		Title string `json:"title"`
		Code  string `json:"code"`
	} `json:"examples"`
}

// TestShortcodeDocsEndpoint verifies the docs registry endpoint returns all four
// built-ins with the expected shape. Plugins are not loaded in the test harness,
// so only built-ins appear (plugin enumeration is covered in plugin_system tests).
func TestShortcodeDocsEndpoint(t *testing.T) {
	tc := SetupTestEnv(t)

	rr := tc.MakeRequest(http.MethodGet, "/v1/shortcodes/docs", nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	var docs []shortcodeDoc
	if err := json.NewDecoder(rr.Body).Decode(&docs); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	byName := map[string]shortcodeDoc{}
	for _, d := range docs {
		byName[d.Name] = d
	}

	for _, want := range []string{"meta", "property", "mrql", "conditional", "link", "each", "item", "partial"} {
		d, ok := byName[want]
		if !ok {
			t.Errorf("missing built-in %q in docs", want)
			continue
		}
		if d.Source != "builtin" {
			t.Errorf("%q: expected source=builtin, got %q", want, d.Source)
		}
		if d.Description == "" {
			t.Errorf("%q: expected non-empty description", want)
		}
	}

	// Block capability snapshot for the built-ins.
	if got := byName["meta"].IsBlock; got != "no" {
		t.Errorf("meta isBlock: expected 'no', got %q", got)
	}
	if got := byName["conditional"].IsBlock; got != "required" {
		t.Errorf("conditional isBlock: expected 'required', got %q", got)
	}
	if got := byName["mrql"].IsBlock; got != "optional" {
		t.Errorf("mrql isBlock: expected 'optional', got %q", got)
	}
	if got := byName["each"].IsBlock; got != "required" {
		t.Errorf("each isBlock: expected 'required', got %q", got)
	}
	if got := byName["item"].IsBlock; got != "no" {
		t.Errorf("item isBlock: expected 'no', got %q", got)
	}

	// meta requires path.
	meta := byName["meta"]
	var pathAttr *shortcodeDocAttr
	for i := range meta.Attrs {
		if meta.Attrs[i].Name == "path" {
			pathAttr = &meta.Attrs[i]
		}
	}
	if pathAttr == nil {
		t.Fatalf("meta: missing path attr")
	}
	if !pathAttr.Required {
		t.Errorf("meta.path: expected required=true")
	}

	// mrql carries the param- wildcard and a scope enum.
	mrql := byName["mrql"]
	var sawWildcard, sawScopeEnum bool
	for _, a := range mrql.Attrs {
		if a.Name == "param-" && a.Wildcard {
			sawWildcard = true
		}
		if a.Name == "scope" && len(a.Enum) > 0 {
			sawScopeEnum = true
		}
	}
	if !sawWildcard {
		t.Errorf("mrql: expected a wildcard param- attr")
	}
	if !sawScopeEnum {
		t.Errorf("mrql: expected scope attr with enum values")
	}
}
