package api_tests

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
		"CreateUserRequest":               {"username", "password", "role", "displayName", "scopeGroupId", "disabled"},
		"UpdateUserRequest":               {"id", "username", "password", "role", "displayName", "scopeGroupId", "disabled"},
		"ChangePasswordRequest":           {"currentPassword", "newPassword"},
		"CreateTokenRequest":              {"name", "expiresIn"},
		"OneTimeTokenResponse":            {"token", "id", "name", "prefix"},
		"UserManagementResponse":          {"ID", "username", "displayName", "role", "scopeGroupId", "disabled"},
		"UserManagementResponsePartial":   {"ID", "username", "displayName", "role", "scopeGroupId", "disabled"},
		"ApiTokenMetadataResponsePartial": {"ID", "CreatedAt", "UpdatedAt", "userId", "name", "prefix", "expiresAt", "lastUsedAt", "disabled"},
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
		assertRequiredProperties(t, spec, "UpdateUserRequest", "id")
		if len(update.Value.Required) != 1 {
			t.Errorf("only id is required for partial updates, required=%v", update.Value.Required)
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
	assertRequiredProperties(t, spec, "UserManagementResponse", "ID", "username", "displayName", "role", "disabled")
	assertRequiredProperties(t, spec, "UserManagementResponsePartial", "ID", "username", "displayName", "role", "disabled")
	assertRequiredProperties(t, spec, "ApiTokenMetadataResponsePartial", "ID", "CreatedAt", "UpdatedAt", "userId", "name", "prefix", "disabled")
	assertRequiredProperties(t, spec, "AccountOKResponse", "ok")

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
	for _, phrase := range []string{"empty or zero", "JSON null", "password:null", "blank password"} {
		if !strings.Contains(update.Description, phrase) {
			t.Errorf("partial update description must document %q; got %q", phrase, update.Description)
		}
	}
}

func TestOpenAPIUserManagementResponseSchemasMatchLiveShapes(t *testing.T) {
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	spec := registry.GenerateSpec()

	listUsers := operation(spec.Paths.Find("/v1/users"), http.MethodGet)
	listTokens := operation(spec.Paths.Find("/v1/account/tokens"), http.MethodGet)
	for path, expectedItem := range map[string]string{"/v1/users": "UserManagementResponsePartial", "/v1/account/tokens": "ApiTokenMetadataResponsePartial"} {
		op := map[string]*openapi3.Operation{"/v1/users": listUsers, "/v1/account/tokens": listTokens}[path]
		if op == nil || op.Responses.Value("200") == nil {
			t.Errorf("GET %s missing 200 response", path)
			continue
		}
		content := op.Responses.Value("200").Value.Content.Get("application/json")
		if content == nil || content.Schema == nil || content.Schema.Value == nil || !content.Schema.Value.Type.Is("array") || content.Schema.Value.Items == nil {
			t.Errorf("GET %s must document a typed array response", path)
			continue
		}
		if got := strings.TrimPrefix(content.Schema.Value.Items.Ref, "#/components/schemas/"); got != expectedItem {
			t.Errorf("GET %s array items=%q, want %q", path, got, expectedItem)
		}
	}

	for _, route := range []string{"/v1/account/password", "/v1/account/tokens/delete"} {
		op := operation(spec.Paths.Find(route), http.MethodPost)
		content := op.Responses.Value("200").Value.Content.Get("application/json")
		var schema *openapi3.Schema
		if content != nil && content.Schema != nil {
			schema = content.Schema.Value
			if schema == nil && strings.HasPrefix(content.Schema.Ref, "#/components/schemas/") {
				ref := spec.Components.Schemas[strings.TrimPrefix(content.Schema.Ref, "#/components/schemas/")]
				if ref != nil {
					schema = ref.Value
				}
			}
		}
		if schema == nil || schema.Properties["ok"] == nil {
			t.Errorf("POST %s must document typed {ok:true}", route)
		}
	}
	if operation(spec.Paths.Find("/v1/account/tokens"), http.MethodGet).Responses.Value("400") == nil {
		t.Error("GET /v1/account/tokens must document its live 400 no-account response")
	}
}

func TestCommittedOpenAPISpecIsFresh(t *testing.T) {
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	generated, err := registry.MarshalYAML()
	if err != nil {
		t.Fatalf("generate OpenAPI YAML: %v", err)
	}
	committedPath := filepath.Join("..", "..", "openapi.yaml")
	committed, err := os.ReadFile(committedPath)
	if err != nil {
		t.Fatalf("read %s: %v", committedPath, err)
	}
	if !bytes.Equal(committed, generated) {
		t.Fatal("openapi.yaml is stale; run go run ./cmd/openapi-gen")
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
