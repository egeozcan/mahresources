package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"mahresources/server/openapi"
)

func revisionSpec(t *testing.T) *openapi3.T {
	t.Helper()
	registry := openapi.NewRegistry()
	RegisterAPIRoutesWithOpenAPI(registry)
	return registry.GenerateSpec()
}

func schemaProps(t *testing.T, spec *openapi3.T, name string) map[string]bool {
	t.Helper()
	ref := spec.Components.Schemas[name]
	if ref == nil || ref.Value == nil {
		t.Fatalf("schema %s missing", name)
	}
	props := map[string]bool{}
	for p := range ref.Value.Properties {
		props[p] = true
	}
	return props
}

func requireProps(t *testing.T, spec *openapi3.T, schema string, want ...string) {
	t.Helper()
	props := schemaProps(t, spec, schema)
	for _, w := range want {
		if !props[w] {
			t.Errorf("%s lacks property %q (has %v)", schema, w, props)
		}
	}
}

// The diff handler reads lowercase from and to, so the published parameter names
// must be lowercase, and from is required.
func TestTemplateRevisionOpenAPIDiffParameters(t *testing.T) {
	spec := revisionSpec(t)
	for _, c := range templateRevisionCarriers {
		op := spec.Paths.Find(c.apiPrefix + "/revisions/diff").Get
		got := map[string]bool{}
		for _, p := range op.Parameters {
			got[p.Value.Name] = p.Value.Required
		}
		for name, required := range map[string]bool{"id": true, "from": true, "to": false} {
			isRequired, ok := got[name]
			if !ok || isRequired != required {
				t.Errorf("%s diff parameter %q: present=%v required=%v, want required=%v (all: %v)", c.apiPrefix, name, ok, isRequired, required, got)
			}
		}
		for name := range got {
			if name != strings.ToLower(name) {
				t.Errorf("%s diff advertises %q, the handler reads lowercase", c.apiPrefix, name)
			}
		}
	}
}

func TestTemplateRevisionOpenAPIResponseSchemasAreComplete(t *testing.T) {
	spec := revisionSpec(t)
	requireProps(t, spec, "TemplateRevisionSummaryPartial", "id", "number", "carrierType", "carrierId", "createdAt",
		"createdByUserId", "label", "source", "applied", "changedSlots", "restoredFrom")
	requireProps(t, spec, "TemplateRevisionSummary", "id", "number", "source", "applied", "changedSlots")
	requireProps(t, spec, "TemplateRevisionDetail", "id", "number", "snapshot", "editor", "changedSlots")
	requireProps(t, spec, "TemplateRevisionDiff", "carrierType", "carrierId", "from", "to", "slots")
	requireProps(t, spec, "SlotDiffPartial", "slot", "fields")
	requireProps(t, spec, "FieldDiffPartial", "field", "hunks")
	requireProps(t, spec, "DiffHunkPartial", "op", "lines")
	requireProps(t, spec, "TemplateRevisionLabelRequest", "label")
	requireProps(t, spec, "TemplateRevisionRestoreRequest", "slot", "includeCss")
}

func TestTemplateRevisionOpenAPIDeclares403(t *testing.T) {
	spec := revisionSpec(t)
	n := 0
	for path, item := range spec.Paths.Map() {
		if !strings.Contains(path, "/revisions") {
			continue
		}
		for method, op := range map[string]*openapi3.Operation{http.MethodGet: item.Get, http.MethodPost: item.Post} {
			if op == nil {
				continue
			}
			n++
			resp := op.Responses.Value("403")
			if resp == nil || resp.Value == nil || resp.Value.Content["application/json"] == nil {
				t.Errorf("%s %s declares no 403 with an error body", method, path)
			}
		}
	}
	if n != 15 {
		t.Errorf("checked %d revision operations, want 15", n)
	}
}
