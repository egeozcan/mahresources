package template_handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flosch/pongo2/v4"
)

func TestRenderTemplateRedirectOnlyLocal(t *testing.T) {
	t.Chdir("../..")
	serve := func(v string) *httptest.ResponseRecorder {
		h := RenderTemplate("partials/form/autocompleter.tpl", func(*http.Request) pongo2.Context {
			return pongo2.Context{"_redirect": v, "id": "x"}
		})
		rr := httptest.NewRecorder()
		h(rr, httptest.NewRequest("GET", "/partials/autocompleter", nil))
		return rr
	}
	for _, v := range []string{"https://evil.example", "//evil.example", "http:/evil.example", "/\\evil.example"} {
		if rr := serve(v); rr.Code == http.StatusFound || rr.Header().Get("Location") != "" {
			t.Errorf("%q: code=%d Location=%q", v, rr.Code, rr.Header().Get("Location"))
		}
	}
	if rr := serve("/notes?page=2"); rr.Code != http.StatusFound || rr.Header().Get("Location") != "/notes?page=2" {
		t.Errorf("local: %d %q", rr.Code, rr.Header().Get("Location"))
	}
}
