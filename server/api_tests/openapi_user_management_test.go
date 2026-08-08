package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"mahresources/auth"
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
		"UserManagementResponse": {"ID", "username", "displayName", "role", "scopeGroupId", "disabled"},
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
		if len(update.Value.Required) != 0 {
			t.Errorf("partial update fields must remain optional, required=%v", update.Value.Required)
		}
		for _, field := range []string{"username", "displayName", "password", "role", "disabled"} {
			property := update.Value.Properties[field]
			if property == nil || property.Value == nil || property.Value.Nullable {
				t.Errorf("partial update field %s must be optional but nonnullable", field)
			}
		}
		scope := update.Value.Properties["scopeGroupId"]
		if scope == nil || scope.Value == nil || !scope.Value.Nullable {
			t.Error("scopeGroupId must be the only nullable partial update field")
		}
	}

	assertRequiredProperties(t, spec, "CreateUserRequest", "username", "password", "role")
	assertRequiredProperties(t, spec, "ChangePasswordRequest", "currentPassword", "newPassword")
	assertRequiredProperties(t, spec, "OneTimeTokenResponse", "token", "id", "name", "prefix")

	response := spec.Components.Schemas["UserManagementResponse"]
	if response != nil && response.Value != nil && response.Value.Properties["id"] != nil {
		t.Error("user response schema must document the live uppercase ID key, not lowercase id")
	}
}

func assertRequiredProperties(t *testing.T, spec *openapi3.T, schemaName string, want ...string) {
	t.Helper()
	schema := spec.Components.Schemas[schemaName]
	if schema == nil || schema.Value == nil {
		t.Errorf("missing component schema %s", schemaName)
		return
	}
	required := make(map[string]bool, len(schema.Value.Required))
	for _, name := range schema.Value.Required {
		required[name] = true
	}
	for _, name := range want {
		if !required[name] {
			t.Errorf("%s must require %s; required=%v", schemaName, name, schema.Value.Required)
		}
	}
}

func TestOpenAPIUserManagementPasswordPolicyUsesAuthConstants(t *testing.T) {
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()
	policy := fmt.Sprintf("at least %d Unicode code points and at most %d UTF-8 bytes", auth.MinPasswordLength, auth.MaxPasswordBytes)

	for _, route := range []struct {
		path, method string
	}{
		{"/v1/users", http.MethodPost},
		{"/v1/account/password", http.MethodPost},
	} {
		op := operation(spec.Paths.Find(route.path), route.method)
		if op == nil {
			t.Errorf("missing %s %s", route.method, route.path)
			continue
		}
		if !strings.Contains(op.Description, policy) {
			t.Errorf("%s %s must document password policy %q; description=%q", route.method, route.path, policy, op.Description)
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
