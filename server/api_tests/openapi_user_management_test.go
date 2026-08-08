package api_tests

import (
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"mahresources/server"
	"mahresources/server/openapi"
)

func TestOpenAPIUserManagementSchemasAreOperationSpecific(t *testing.T) {
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()

	wantSchemas := map[string][]string{
		"CreateUserRequest":      {"username", "password", "role", "displayName", "scopeGroupId", "disabled"},
		"UpdateUserRequest":      {"id", "username", "password", "role", "displayName", "scopeGroupId", "disabled"},
		"ChangePasswordRequest":  {"currentPassword", "newPassword"},
		"CreateTokenRequest":     {"name", "expiresIn"},
		"OneTimeTokenResponse":   {"token", "id", "name", "prefix"},
		"UserManagementResponse": {"id", "username", "displayName", "role", "scopeGroupId", "disabled"},
	}
	for name, properties := range wantSchemas {
		schemaRef := spec.Components.Schemas[name]
		if schemaRef == nil || schemaRef.Value == nil {
			t.Errorf("missing component schema %s", name)
			continue
		}
		for _, property := range properties {
			if schemaRef.Value.Properties[property] == nil {
				t.Errorf("%s missing property %s", name, property)
			}
		}
	}

	update := spec.Components.Schemas["UpdateUserRequest"]
	if update != nil && update.Value != nil {
		for _, field := range []string{"username", "displayName", "password", "role", "scopeGroupId", "disabled"} {
			property := update.Value.Properties[field]
			if property == nil || property.Value == nil || !property.Value.Nullable {
				t.Errorf("partial update field %s must be optional and nullable", field)
			}
		}
	}
}

func TestOpenAPIUserManagementDocumentsErrorsAndClearSemantics(t *testing.T) {
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()

	checks := []struct {
		path, method string
		statuses     []string
	}{
		{"/v1/users", http.MethodPost, []string{"400", "401", "403", "409"}},
		{"/v1/user", http.MethodPost, []string{"400", "401", "403", "404", "409"}},
		{"/v1/account/password", http.MethodPost, []string{"400", "401", "403"}},
		{"/v1/account/tokens", http.MethodPost, []string{"400", "401", "403", "409"}},
		{"/v1/account/tokens/delete", http.MethodPost, []string{"400", "401", "403", "404"}},
	}
	for _, check := range checks {
		item := spec.Paths.Find(check.path)
		if item == nil {
			t.Errorf("missing path %s", check.path)
			continue
		}
		op := operation(item, check.method)
		if op == nil {
			t.Errorf("missing %s %s", check.method, check.path)
			continue
		}
		for _, status := range check.statuses {
			if op.Responses.Value(status) == nil {
				t.Errorf("%s %s missing %s response", check.method, check.path, status)
			}
		}
	}

	update := operation(spec.Paths.Find("/v1/user"), http.MethodPost)
	if update == nil || update.Description == "" {
		t.Fatal("partial user update must document explicit-clear semantics")
	}
}

func operation(item *openapi3.PathItem, method string) *openapi3.Operation {
	if item == nil {
		return nil
	}
	switch method {
	case http.MethodPost:
		return item.Post
	case http.MethodGet:
		return item.Get
	case http.MethodDelete:
		return item.Delete
	default:
		return nil
	}
}
