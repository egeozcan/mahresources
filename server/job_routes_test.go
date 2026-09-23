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
		"/v1/jobs", "/v1/jobs/summary", "/v1/jobs/{id}", "/v1/jobs/{id}/events",
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

func TestPublicOpenAPISpecFollowsCanonicalJobRouteGate(t *testing.T) {
	registry := openapi.NewRegistry()
	RegisterAPIRoutesWithOpenAPI(registry)
	paths := registry.GenerateSpec().Paths.Map()
	for _, path := range []string{
		"/v1/jobs", "/v1/jobs/summary", "/v1/jobs/{id}", "/v1/jobs/{id}/events",
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
	for _, parameter := range paths["/v1/jobs"].Get.Parameters {
		if parameter.Value != nil && parameter.Value.Name == "command" {
			t.Fatal("list OpenAPI must not advertise command filtering until the Service can answer it without page post-filtering")
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
