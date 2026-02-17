package template_context_providers

import (
	"mahresources/application_context/mock_context"
	"net/http/httptest"
	"testing"
)

func TestGroupContextProviderImpl(t *testing.T) {
	reader := mock_context.NewMockGroupContext()
	provider := groupContextProviderImpl(reader)

	req := httptest.NewRequest("GET", "http://example.com/group?id=1", nil)
	ctx := provider(req)

	if ctx["pageTitle"] == nil {
		t.Error("expected pageTitle in context, got nil")
	}

	if ctx["group"] == nil {
		t.Error("expected group in context, got nil")
	}

	if ctx["breadcrumb"] == nil {
		t.Error("expected breadcrumb in context, got nil")
	}
}

func TestGroupContextProviderImpl_NoID(t *testing.T) {
	reader := mock_context.NewMockGroupContext()
	provider := groupContextProviderImpl(reader)

	req := httptest.NewRequest("GET", "http://example.com/group", nil)
	ctx := provider(req)

	// When no ID is provided, the decoder sets ID to 0 and GetGroup(0)
	// succeeds with the mock, so we still get a valid context.
	if ctx["group"] == nil {
		t.Error("expected group in context, got nil")
	}

	if ctx["pageTitle"] == nil {
		t.Error("expected pageTitle in context, got nil")
	}
}
