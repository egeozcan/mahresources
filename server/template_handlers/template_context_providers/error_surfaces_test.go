package template_context_providers

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRecoveryLinksRejectBackslashTargets(t *testing.T) {
	u, _ := url.Parse("/v1/tags/merge?redirect=/%5Cevil.example")
	links := RecoveryLinksForRequest(u, "http://localhost/\\evil.example/x")
	var haveTags, haveDash bool
	for _, l := range links {
		if strings.Contains(l.Url, "\\") {
			t.Errorf("link with backslash: %q", l.Url)
		}
		haveTags = haveTags || l.Url == "/tags"
		haveDash = haveDash || l.Url == "/dashboard"
	}
	if !haveTags || !haveDash {
		t.Errorf("expected tags and dashboard links: %+v", links)
	}
	u2, _ := url.Parse("/v1/tags/merge?redirect=/tags?x=1")
	if l := RecoveryLinksForRequest(u2, ""); l[0].Url != "/tags?x=1" {
		t.Errorf("first link = %q", l[0].Url)
	}
}

func TestLoginContextNextValidated(t *testing.T) {
	p := LoginContextProvider(nil)
	ctx := p(httptest.NewRequest("GET", "/login?next=/%5Cevil.example", nil))
	if _, ok := ctx["next"]; ok {
		t.Errorf("unsafe next leaked: %v", ctx["next"])
	}
	ctx = p(httptest.NewRequest("GET", "/login?next=/notes?id=3", nil))
	if ctx["next"] != "/notes?id=3" {
		t.Errorf("next = %v", ctx["next"])
	}
}

func TestPartialContextProviderAllowList(t *testing.T) {
	p := PartialContextProvider(nil)
	ctx := p(httptest.NewRequest("GET", "/partials/autocompleter?profile=tag&_redirect=https://x&parameters=a&onChange=b&partial=x", nil))
	for _, k := range []string{"_redirect", "parameters", "onChange"} {
		if _, ok := ctx[k]; ok {
			t.Errorf("key %s copied", k)
		}
	}
	if ctx["profile"] != "tag" {
		t.Errorf("profile = %v", ctx["profile"])
	}
	if _, ok := ctx["partial"].(string); ok {
		t.Errorf("partial overwritten")
	}
}
