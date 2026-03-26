package template_context_providers

import (
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestNormalizeQueryValues_LowercaseKey(t *testing.T) {
	values := url.Values{"name": {"QA"}}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "QA" {
		t.Errorf("expected Name=QA, got %q", got)
	}
	// Original lowercase key should also be preserved
	if got := result.Get("name"); got != "QA" {
		t.Errorf("expected name=QA (original preserved), got %q", got)
	}
}

func TestNormalizeQueryValues_PreservesUppercaseKey(t *testing.T) {
	values := url.Values{"Name": {"QA"}}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "QA" {
		t.Errorf("expected Name=QA, got %q", got)
	}
}

func TestNormalizeQueryValues_DoesNotOverrideExistingUppercase(t *testing.T) {
	values := url.Values{
		"name": {"lower"},
		"Name": {"upper"},
	}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "upper" {
		t.Errorf("expected Name=upper (explicit uppercase wins), got %q", got)
	}
}

func TestNormalizeQueryValues_MultipleKeys(t *testing.T) {
	values := url.Values{
		"name":        {"QA"},
		"description": {"test"},
	}
	result := normalizeQueryValues(values)

	if got := result.Get("Name"); got != "QA" {
		t.Errorf("expected Name=QA, got %q", got)
	}
	if got := result.Get("Description"); got != "test" {
		t.Errorf("expected Description=test, got %q", got)
	}
}

func TestStaticTemplateCtx_QueryValuesNormalized(t *testing.T) {
	req := httptest.NewRequest("GET", "http://example.com/tags?name=QA", nil)
	ctx := staticTemplateCtx(req)

	queryValues, ok := ctx["queryValues"].(url.Values)
	if !ok {
		t.Fatal("expected queryValues to be url.Values")
	}

	if got := queryValues.Get("Name"); got != "QA" {
		t.Errorf("expected queryValues[Name]=QA, got %q", got)
	}
}
