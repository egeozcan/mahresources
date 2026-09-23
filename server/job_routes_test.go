package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/gorilla/mux"
	"mahresources/server/openapi"
)

func TestCanonicalJobRoutesAreOptInUntilCutover(t *testing.T) {
	router := mux.NewRouter()
	registerCanonicalJobRoutes(router, nil, false)
	if router.Match(&http.Request{Method: http.MethodGet, URL: mustURL(t, "/v1/jobs")}, &mux.RouteMatch{}) {
		t.Fatal("canonical Job list route registered while cutover is disabled")
	}

	enabled := mux.NewRouter()
	registerCanonicalJobRoutes(enabled, nil, true)
	paths := map[string]bool{}
	if err := enabled.Walk(func(route *mux.Route, _ *mux.Router, _ []*mux.Route) error {
		path, err := route.GetPathTemplate()
		if err == nil {
			paths[path] = true
		}
		return nil
	}); err != nil {
		t.Fatalf("walk enabled canonical routes: %v", err)
	}
	for _, path := range []string{
		"/v1/jobs", "/v1/jobs/summary", "/v1/jobs/summary/export", "/v1/jobs/{id}", "/v1/jobs/{id}/events",
		"/v1/jobs/{id}/outputs", "/v1/jobs/{id}/commands/{command}", "/v1/jobs/commands/{command}",
	} {
		if !paths[path] {
			t.Errorf("enabled canonical route missing %s", path)
		}
	}
	matched := &mux.RouteMatch{}
	if !enabled.Match(&http.Request{Method: http.MethodGet, URL: mustURL(t, "/v1/jobs/summary")}, matched) {
		t.Fatal("summary route did not match its canonical path")
	}
	if path, _ := matched.Route.GetPathTemplate(); path != "/v1/jobs/summary" {
		t.Fatalf("summary route matched template %q, want static summary route", path)
	}
}

func TestJobMigrationReadinessIsAdminOnly(t *testing.T) {
	if got := requiredCapability(http.MethodGet, "/v1/admin/jobs/migration-readiness"); got != capSystem {
		t.Fatalf("migration readiness capability = %v, want admin-only system capability", got)
	}
}

func TestPublicOpenAPISpecFollowsCanonicalJobRouteGate(t *testing.T) {
	registry := openapi.NewRegistry()
	RegisterAPIRoutesWithOpenAPI(registry)
	paths := registry.GenerateSpec().Paths.Map()
	for _, path := range []string{
		"/v1/jobs", "/v1/jobs/summary", "/v1/jobs/summary/export", "/v1/jobs/{id}", "/v1/jobs/{id}/events",
		"/v1/jobs/{id}/outputs", "/v1/jobs/{id}/commands/{command}", "/v1/jobs/commands/{command}",
	} {
		if advertised := paths[path] != nil; advertised != canonicalJobAPICutoverComplete {
			t.Errorf("public OpenAPI route %s advertised=%t, want gate=%t", path, advertised, canonicalJobAPICutoverComplete)
		}
	}
	if paths["/v1/jobs/events"] == nil || paths["/v1/jobs/get"] == nil {
		t.Fatal("legacy Jobs event/get routes disappeared from the public spec")
	}
}

func TestCanonicalSummaryExportOpenAPIRouteIsDefinedButGated(t *testing.T) {
	registry := openapi.NewRegistry()
	registerCanonicalJobRoutesOpenAPI(registry)
	path := registry.GenerateSpec().Paths.Map()["/v1/jobs/summary/export"]
	if path == nil || path.Post == nil {
		t.Fatal("canonical summary export has no gated OpenAPI definition")
	}
	if path.Post.OperationID != "exportCanonicalJobSummary" || path.Post.Responses.Value("202") == nil {
		t.Fatalf("summary export operation = %+v, want stable operation id and 202 response", path.Post)
	}

	public := openapi.NewRegistry()
	RegisterAPIRoutesWithOpenAPI(public)
	if route := public.GenerateSpec().Paths.Map()["/v1/jobs/summary/export"]; (route != nil) != canonicalJobAPICutoverComplete {
		t.Fatalf("summary export publication does not match cutover gate: published=%t, gate=%t", route != nil, canonicalJobAPICutoverComplete)
	}
}

func TestJobMigrationReadinessOpenAPIIsAdminRoute(t *testing.T) {
	registry := openapi.NewRegistry()
	RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()
	operation := spec.Paths.Map()["/v1/admin/jobs/migration-readiness"]
	if operation == nil || operation.Get == nil {
		t.Fatal("Job migration readiness is missing from the admin OpenAPI routes")
	}
	if operation.Get.OperationID != "getJobMigrationReadiness" || operation.Get.Responses.Value("200") == nil || operation.Get.Responses.Value("403") == nil {
		t.Fatalf("migration readiness operation = %+v, want stable operation id and 200/403 responses", operation.Get)
	}
	response := operation.Get.Responses.Value("200").Value.Content["application/json"]
	if response == nil || response.Schema == nil {
		t.Fatal("migration readiness response has no JSON schema")
	}
	schema := response.Schema
	if schema.Value == nil && schema.Ref == "#/components/schemas/JobMigrationReadiness" {
		schema = spec.Components.Schemas["JobMigrationReadiness"]
	}
	if schema == nil || schema.Value == nil {
		t.Fatal("migration readiness response schema reference is unresolved")
	}
	for _, field := range []string{"ready", "writerEpoch", "phase", "sourceCounts", "blockers"} {
		if schema.Value.Properties[field] == nil {
			t.Errorf("migration readiness response is missing %q", field)
		}
	}
	counts := schema.Value.Properties["sourceCounts"]
	if counts == nil || counts.Value == nil || counts.Value.AdditionalProperties.Schema == nil ||
		counts.Value.AdditionalProperties.Schema.Value == nil ||
		counts.Value.AdditionalProperties.Schema.Value.AdditionalProperties.Schema == nil {
		t.Error("sourceCounts must map source Kind to status-count maps")
	}
	blockers := schema.Value.Properties["blockers"]
	if blockers == nil || blockers.Value == nil || blockers.Value.AdditionalProperties.Schema == nil || blockers.Value.Items != nil {
		t.Error("blockers must be a map of safe blocker code to count")
	}
}

func TestLegacyJobOpenAPIAdvertisesRetirementAndControlKeyHeaders(t *testing.T) {
	registry := openapi.NewRegistry()
	RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()
	paths := spec.Paths.Map()
	if (paths["/v1/jobs"] != nil) != canonicalJobAPICutoverComplete {
		t.Fatalf("canonical Job list publication does not match cutover gate: published=%t, gate=%t", paths["/v1/jobs"] != nil, canonicalJobAPICutoverComplete)
	}

	for _, route := range []struct {
		path   string
		method string
	}{
		{"/v1/download/queue", http.MethodGet},
		{"/v1/download/retry", http.MethodPost},
		{"/v1/downloads", http.MethodGet},
		{"/v1/jobs/get", http.MethodGet},
		{"/v1/jobs/events", http.MethodGet},
	} {
		item := paths[route.path]
		if item == nil {
			t.Fatalf("legacy OpenAPI route %s is missing", route.path)
		}
		var operation *openapi3.Operation
		switch route.method {
		case http.MethodGet:
			operation = item.Get
		case http.MethodPost:
			operation = item.Post
		}
		if operation == nil || !operation.Deprecated {
			t.Errorf("%s %s must be marked deprecated", route.method, route.path)
			continue
		}
		for _, status := range []string{"200", "400"} {
			response := operation.Responses.Value(status)
			if response == nil || response.Value == nil {
				t.Errorf("%s %s response %s is missing", route.method, route.path, status)
				continue
			}
			for _, name := range []string{"Deprecation", "Sunset", "Link"} {
				if response.Value.Headers[name] == nil {
					t.Errorf("%s %s response %s has no %s header", route.method, route.path, status, name)
				}
			}
		}
	}

	for _, path := range []string{"/v1/download/cancel", "/v1/download/resume", "/v1/download/retry", "/v1/jobs/cancel", "/v1/jobs/resume", "/v1/jobs/retry", "/v1/downloads/retry"} {
		item := paths[path]
		if item == nil || item.Post == nil {
			t.Errorf("legacy command route %s is missing", path)
			continue
		}
		found := false
		for _, parameter := range item.Post.Parameters {
			if parameter.Value != nil && parameter.Value.In == "header" && parameter.Value.Name == "Idempotency-Key" {
				found = true
			}
		}
		if !found {
			t.Errorf("legacy command route %s does not document Idempotency-Key", path)
		}
	}
	for _, path := range []string{"/v1/download/submit", "/v1/jobs/download/submit", "/v1/jobs/action/run"} {
		item := paths[path]
		if item == nil || item.Post == nil || item.Post.Responses.Value("202") == nil {
			t.Errorf("%s OpenAPI must document its HTTP 202 accepted response", path)
		}
	}
	if body := paths["/v1/downloads/retry"].Post.RequestBody.Value.Content["application/json"].Schema.Value; body.Properties["idempotencyKey"] == nil {
		t.Fatal("download history Retry body does not document idempotencyKey")
	}
}

func TestCanonicalJobOpenAPIRoutesDescribeResponsesAndIdempotency(t *testing.T) {
	registry := openapi.NewRegistry()
	registerCanonicalJobRoutesOpenAPI(registry)
	spec := registry.GenerateSpec()
	loader := openapi3.NewLoader()
	if err := loader.ResolveRefsIn(spec, nil); err != nil {
		t.Fatalf("resolve canonical Job OpenAPI references: %v", err)
	}
	if err := spec.Validate(context.Background()); err != nil {
		t.Fatalf("canonical Job OpenAPI spec is invalid: %v", err)
	}
	paths := spec.Paths.Map()
	for _, path := range []string{
		"/v1/jobs", "/v1/jobs/summary", "/v1/jobs/{id}", "/v1/jobs/{id}/events",
		"/v1/jobs/events", "/v1/jobs/{id}/outputs", "/v1/jobs/{id}/commands/{command}",
		"/v1/jobs/commands/{command}",
	} {
		if paths[path] == nil {
			t.Errorf("canonical Job OpenAPI path missing %s", path)
		}
	}
	for _, path := range []string{"/v1/jobs", "/v1/jobs/summary"} {
		found := false
		for _, parameter := range paths[path].Get.Parameters {
			if parameter.Value != nil && parameter.Value.Name == "command" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s OpenAPI must advertise command filtering now that the Service applies it before pagination", path)
		}
	}
	command := paths["/v1/jobs/{id}/commands/{command}"].Post
	if command == nil || command.RequestBody == nil || command.Responses.Value("409") == nil {
		t.Fatal("command OpenAPI operation must include a request and conflict response")
	}
	bodySchema := command.RequestBody.Value.Content["application/json"].Schema.Value
	if bodySchema.Properties["expectedVersion"] == nil || bodySchema.Properties["idempotencyKey"] == nil {
		t.Fatal("command schema must document expectedVersion and idempotencyKey")
	}
	requiredVersion := false
	for _, field := range bodySchema.Required {
		if field == "expectedVersion" {
			requiredVersion = true
		}
	}
	if !requiredVersion {
		t.Fatal("command schema must require expectedVersion")
	}
	conflictSchema := command.Responses.Value("409").Value.Content["application/json"].Schema.Value
	if conflictSchema == nil || conflictSchema.Properties["job"] == nil || conflictSchema.Properties["result"] == nil {
		t.Fatal("command conflict response schema must include the fresh Job and result")
	}
	stream := paths["/v1/jobs/events"].Get
	if stream == nil || stream.Parameters == nil {
		t.Fatal("canonical event stream must document its version parameter")
	}
	foundVersion := false
	for _, parameter := range stream.Parameters {
		if parameter.Value != nil && parameter.Value.Name == "version" {
			foundVersion = !parameter.Value.Required && strings.Contains(parameter.Value.Description, "omit")
		}
	}
	if !foundVersion {
		t.Fatal("event stream OpenAPI must document that version=2 selects canonical events and omission preserves legacy events")
	}
}

func mustURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL %q: %v", raw, err)
	}
	return parsed
}
