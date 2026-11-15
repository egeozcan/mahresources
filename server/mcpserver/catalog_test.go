package mcpserver_test

import (
	"sort"
	"strings"
	"testing"

	"mahresources/server"
	"mahresources/server/mcpserver"
	"mahresources/server/openapi"
)

func buildCatalog(t *testing.T) *mcpserver.Catalog {
	t.Helper()
	registry := openapi.NewRegistry()
	server.RegisterAPIRoutesWithOpenAPI(registry)
	c, err := mcpserver.BuildCatalog(registry.GenerateSpec())
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	return c
}

// Every operation in the registry has a safety class (BuildCatalog refuses to
// build otherwise), and every name in the safety tables is a real operation,
// so a renamed route cannot silently fall out of its class.
func TestEverySafetyTableEntryNamesARealOperation(t *testing.T) {
	c := buildCatalog(t)
	var stale []string
	for _, id := range mcpserver.ClassifiedOperationIDs() {
		if _, ok := c.Lookup(id); !ok {
			stale = append(stale, id)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("safety tables name operations that no longer exist: %v", stale)
	}
}

func TestEveryOperationIsClassified(t *testing.T) {
	c := buildCatalog(t)
	if len(c.All()) < 250 {
		t.Fatalf("catalog has only %d operations; the registry was not read", len(c.All()))
	}
	for _, op := range c.All() {
		if op.Safety < mcpserver.ReadOnly || op.Safety > mcpserver.Excluded {
			t.Errorf("%s has no safety class", op.ID)
		}
	}
}

func TestCredentialIssuanceIsNotExposed(t *testing.T) {
	c := buildCatalog(t)
	for _, id := range []string{"createOwnToken", "changeOwnPassword", "createUser", "updateUser", "login", "logout"} {
		if _, ok := c.Get(id); ok {
			t.Errorf("%s is exposed over MCP", id)
		}
	}
}

func TestEveryDeleteAndMergeIsDestructive(t *testing.T) {
	c := buildCatalog(t)
	for _, op := range c.Exposed() {
		if op.Method == "GET" {
			continue
		}
		lower := op.ID
		isDelete := op.Method == "DELETE" || containsFold(lower, "delete") || containsFold(lower, "merge") ||
			containsFold(lower, "massEdit") || containsFold(lower, "purge")
		if isDelete && op.Safety != mcpserver.Destructive && op.ID != "deleteOwnSetting" {
			t.Errorf("%s (%s %s) looks destructive but is %v", op.ID, op.Method, op.Path, op.Safety)
		}
	}
}

func TestNonJSONResponsesAreLinkedOrInlined(t *testing.T) {
	c := buildCatalog(t)
	want := map[string]mcpserver.ResponseKind{
		"viewResource":           mcpserver.ResponseLink,
		"getVersionFile":         mcpserver.ResponseLink,
		"downloadGroupExport":    mcpserver.ResponseLink,
		"openCanonicalJobOutput": mcpserver.ResponseLink,
		"getResourcePreview":     mcpserver.ResponseImage,
		"getVersionThumbnail":    mcpserver.ResponseImage,
		"getRenderedTextBlocks":  mcpserver.ResponseText,
		"listResources":          mcpserver.ResponseJSON,
	}
	for id, kind := range want {
		op, ok := c.Get(id)
		if !ok {
			t.Errorf("%s missing", id)
			continue
		}
		if op.Response != kind {
			t.Errorf("%s response kind = %v, want %v", id, op.Response, kind)
		}
	}
}

func TestInputSchemaCarriesIDQueryParam(t *testing.T) {
	c := buildCatalog(t)
	op, ok := c.Get("getNote")
	if !ok {
		t.Fatal("getNote missing")
	}
	if !requires(op.InputSchema, "id") {
		t.Fatalf("getNote schema does not require id: %v", op.InputSchema)
	}
	del, _ := c.Get("deleteResource")
	if !requires(del.InputSchema, "Id") {
		t.Fatalf("deleteResource schema does not require Id: %v", del.InputSchema)
	}
}

func TestInputSchemaResolvesBodyRefs(t *testing.T) {
	c := buildCatalog(t)
	op, _ := c.Get("mergeResources")
	props := op.InputSchema["properties"].(map[string]any)
	for _, f := range []string{"Winner", "Losers"} {
		if _, ok := props[f]; !ok {
			t.Errorf("mergeResources schema lacks %s: %v", f, props)
		}
	}
	if _, ok := props["$ref"]; ok {
		t.Error("unresolved $ref in schema")
	}
}

func TestMultipartUploadTakesBase64Files(t *testing.T) {
	c := buildCatalog(t)
	op, _ := c.Get("createResource")
	if op.Body == nil || op.Body.ContentType != "multipart/form-data" {
		t.Fatalf("createResource body = %+v", op.Body)
	}
	if !op.Body.FileFields["resource"] {
		t.Fatalf("createResource file fields = %v", op.Body.FileFields)
	}
	props := op.InputSchema["properties"].(map[string]any)
	res := props["resource"].(map[string]any)
	if res["type"] != "array" {
		t.Fatalf("resource arg schema = %v", res)
	}
}

func TestPaginatedListExposesPage(t *testing.T) {
	c := buildCatalog(t)
	op, _ := c.Get("listResources")
	props := op.InputSchema["properties"].(map[string]any)
	if _, ok := props["page"]; !ok {
		t.Fatalf("listResources lacks page: %v", props)
	}
}

func requires(schema map[string]any, name string) bool {
	req, _ := schema["required"].([]any)
	for _, r := range req {
		if r == name {
			return true
		}
	}
	return false
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
