package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func TestCategoryMetadataIndexesRoundTrip(t *testing.T) {
	for _, path := range []string{"/v1/resourceCategory", "/v1/category", "/v1/note/noteType"} {
		t.Run(path, func(t *testing.T) {
			tc := SetupTestEnv(t)
			raw := `[{"key":"score","kind":"numeric"},{"key":"camera.model","kind":"text"}]`
			created := tc.MakeRequest(http.MethodPost, path, map[string]any{"Name": "Indexed category", "MetadataIndexes": raw})
			if created.Code != http.StatusOK {
				t.Fatalf("create: %d %s", created.Code, created.Body.String())
			}
			var row struct {
				ID              uint
				MetadataIndexes string
			}
			if err := json.Unmarshal(created.Body.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			if row.MetadataIndexes != raw {
				t.Fatalf("definitions not persisted: %+v", row)
			}
			updated := tc.MakeRequest(http.MethodPost, path, map[string]any{"ID": row.ID, "Description": "changed"})
			if updated.Code != http.StatusOK {
				t.Fatalf("partial update: %d %s", updated.Code, updated.Body.String())
			}
			if err := json.Unmarshal(updated.Body.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			if row.MetadataIndexes != raw {
				t.Fatal("omitted declarations were cleared")
			}
			cleared := tc.MakeRequest(http.MethodPost, path, map[string]any{"ID": row.ID, "MetadataIndexes": "[]"})
			if cleared.Code != http.StatusOK {
				t.Fatalf("clear: %d %s", cleared.Code, cleared.Body.String())
			}
			if err := json.Unmarshal(cleared.Body.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			if row.MetadataIndexes != "[]" {
				t.Fatal("explicit removal was ignored")
			}
			invalid := tc.MakeRequest(http.MethodPost, path, map[string]any{"ID": row.ID, "MetadataIndexes": `[{"key":"x');DROP TABLE resources;--","kind":"numeric"}]`})
			if invalid.Code < 400 {
				t.Fatalf("unsafe key accepted: %d", invalid.Code)
			}
			if !tc.DB.Migrator().HasTable("resources") {
				t.Fatal("unsafe DDL reached database")
			}
		})
	}
}

func TestMetadataIndexStatusAdminOnly(t *testing.T) {
	tc := setupAuthEnv(t)
	for _, role := range []models.Role{models.RoleAdmin, models.RoleEditor, models.RoleUser, models.RoleGuest} {
		headers := map[string]string{"Authorization": roleBearer(t, tc, role), "Accept": "application/json"}
		resp := doReq(tc, http.MethodGet, "/v1/admin/settings/metadata-index-status", headers, nil, nil)
		want := http.StatusForbidden
		if role == models.RoleAdmin {
			want = http.StatusOK
		}
		if resp.Code != want {
			t.Fatalf("%s: status=%d want=%d", role, resp.Code, want)
		}
	}
}

func TestCategoryMetadataIndexesCreateBuildAndDelete(t *testing.T) {
	tc := SetupTestEnv(t)
	indexes := `[{"key":"score","kind":"numeric"}]`
	var ids []uint
	for i := 0; i < 2; i++ {
		resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": fmt.Sprintf("Shared %d", i), "MetadataIndexes": indexes})
		if resp.Code != http.StatusOK {
			t.Fatal(resp.Body.String())
		}
		var row models.ResourceCategory
		if err := json.Unmarshal(resp.Body.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, row.ID)
	}
	worker := application_context.NewMetadataIndexer(tc.DB)
	if err := worker.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if worker.Status().State != "ready" {
		t.Fatal(worker.Status())
	}
	if err := tc.AppCtx.DeleteResourceCategory(ids[0]); err != nil {
		t.Fatal(err)
	}
	if err := worker.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if worker.Status().State != "ready" {
		t.Fatal(worker.Status())
	}
}
