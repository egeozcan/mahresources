//go:build postgres

package api_tests

import (
	"net/http"
	"net/url"
	"testing"
)

func TestPG_TagCreate(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	formData := url.Values{}
	formData.Set("Name", "pg-test-tag")
	rr := tc.MakeFormRequest("POST", "/v1/tag", formData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create tag: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_GroupCreate(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	formData := url.Values{}
	formData.Set("Name", "pg-test-group")
	rr := tc.MakeFormRequest("POST", "/v1/group", formData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create group: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_NoteCreate(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	formData := url.Values{}
	formData.Set("Name", "pg-test-note")
	rr := tc.MakeFormRequest("POST", "/v1/note", formData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create note: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_NoteWithOwner(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	groupData := url.Values{}
	groupData.Set("Name", "pg-owner-group")
	tc.MakeFormRequest("POST", "/v1/group", groupData)
	noteData := url.Values{}
	noteData.Set("Name", "pg-owned-note")
	noteData.Set("OwnerId", "1")
	rr := tc.MakeFormRequest("POST", "/v1/note", noteData)
	if rr.Code != http.StatusOK {
		t.Fatalf("create note with owner: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestPG_BulkDeleteEmpty(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	rr := tc.MakeRequest("POST", "/v1/tags/delete", map[string]any{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}

func TestPG_MRQL(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	tagData := url.Values{}
	tagData.Set("Name", "mrql-pg-tag")
	tc.MakeFormRequest("POST", "/v1/tag", tagData)
	groupData := url.Values{}
	groupData.Set("Name", "mrql-pg-group")
	tc.MakeFormRequest("POST", "/v1/group", groupData)
	rr := tc.MakeRequest("POST", "/v1/mrql", map[string]interface{}{
		"query": `type = group AND name ~ "mrql-pg"`,
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("MRQL query: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
}
