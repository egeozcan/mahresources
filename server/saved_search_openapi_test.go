package server

import (
	"testing"

	"mahresources/server/openapi"
)

func TestSavedSearchOpenAPI(t *testing.T) {
	r := openapi.NewRegistry()
	registerSavedSearchRoutes(r)
	spec := r.GenerateSpec()
	collection := spec.Paths.Value("/v1/account/saved-searches")
	if collection == nil {
		t.Fatal("saved search collection missing")
	}
	if collection.Post.Responses.Value("201") == nil || collection.Get.Responses.Value("200") == nil {
		t.Fatal("incorrect success codes")
	}
	item := spec.Paths.Value("/v1/account/saved-searches/{id}")
	if item == nil || item.Patch == nil || item.Delete.Responses.Value("204") == nil {
		t.Fatal("saved search mutation contract missing")
	}
	fields := spec.Components.Schemas["SavedSearchPartial"].Value.Properties
	for _, field := range []string{"id", "name", "url", "family", "layout", "createdAt", "updatedAt"} {
		if fields[field] == nil {
			t.Errorf("list schema missing %s", field)
		}
	}
	if fields["userId"] != nil || fields["UserId"] != nil {
		t.Fatal("owner leaked into list schema")
	}
}
