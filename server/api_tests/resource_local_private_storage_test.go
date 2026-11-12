package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"mahresources/jobs"
	"mahresources/models"
)

// POST /v1/resource/local registers whatever file under the storage root the
// caller names. It is an administrator's tool, and even an administrator must
// not be able to turn application-private storage (the Job replay key, export
// archives) into a resource whose bytes the resource-ID readers then serve and
// whose delete then removes the file.
func TestResourceFromLocalRefusesPrivateStorageAndNonAdmins(t *testing.T) {
	tc := setupAuthEnv(t)

	scope := &models.Group{Name: "local-scope"}
	outside := &models.Group{Name: "local-outside"}
	for _, g := range []*models.Group{scope, outside} {
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}

	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	if err != nil {
		t.Fatalf("default fs: %v", err)
	}
	secret := []byte("SECRET-REPLAY-KEY-BYTES")
	files := map[string][]byte{
		jobs.JobReplayKeyFileName: secret,
		"_exports/x.tar":          []byte("ANOTHER-USERS-EXPORT"),
		"victim.bin":              []byte("OTHER-TENANT-BYTES"),
		"public.txt":              []byte("an ordinary staged file"),
	}
	for name, data := range files {
		if err := afero.WriteFile(fs, name, data, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	victim := &models.Resource{Name: "victim", OwnerId: &outside.ID, Location: "victim.bin", Hash: "victimhash"}
	if err := tc.DB.Create(victim).Error; err != nil {
		t.Fatalf("create victim resource: %v", err)
	}

	bearers := map[string]string{
		"scoped user": scopedUserBearer(t, tc, scope.ID),
		"editor":      roleBearer(t, tc, models.RoleEditor),
		"admin":       roleBearer(t, tc, models.RoleAdmin),
	}
	postLocal := func(bearer, localPath string) (int, string) {
		t.Helper()
		body, _ := json.Marshal(map[string]any{"LocalPath": localPath, "OwnerId": scope.ID})
		rr := doReq(tc, http.MethodPost, "/v1/resource/local", map[string]string{
			"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer,
		}, nil, strings.NewReader(string(body)))
		return rr.Code, rr.Body.String()
	}

	for _, who := range []string{"scoped user", "editor"} {
		for _, p := range []string{jobs.JobReplayKeyFileName, "_exports/x.tar", "victim.bin", "public.txt"} {
			if code, body := postLocal(bearers[who], p); code != http.StatusForbidden {
				t.Errorf("%s POST /v1/resource/local %q = %d, want 403 (body: %s)", who, p, code, body)
			}
		}
	}

	reserved := []string{
		jobs.JobReplayKeyFileName,
		"/" + jobs.JobReplayKeyFileName,
		"_exports/x.tar",
		"public/../_exports/x.tar",
		`\_exports\x.tar`,
		"../" + jobs.JobReplayKeyFileName,
	}
	for _, p := range reserved {
		code, body := postLocal(bearers["admin"], p)
		if code != http.StatusForbidden {
			t.Errorf("admin POST /v1/resource/local %q = %d, want 403 (body: %s)", p, code, body)
		}
		if strings.Contains(body, "Hash") {
			t.Errorf("admin POST /v1/resource/local %q disclosed resource metadata: %s", p, body)
		}
	}
	if code, body := postLocal(bearers["admin"], "public.txt"); code != http.StatusOK {
		t.Errorf("admin POST /v1/resource/local for an ordinary file = %d, want 200 (body: %s)", code, body)
	}

	var planted int64
	tc.DB.Model(&models.Resource{}).Where("location LIKE ? OR location LIKE ?", "%_job_replay_key%", "%_exports%").Count(&planted)
	if planted != 0 {
		t.Fatalf("%d resources were registered over private storage", planted)
	}

	// A row reaching a private root by some other way must still never be
	// served by resource ID, nor have its file removed by a delete.
	plantedResource := &models.Resource{Name: "planted", OwnerId: &scope.ID, Location: jobs.JobReplayKeyFileName, Hash: "plantedhash", ContentType: "application/octet-stream"}
	if err := tc.DB.Create(plantedResource).Error; err != nil {
		t.Fatalf("plant resource: %v", err)
	}
	plantedVersion := &models.ResourceVersion{ResourceID: plantedResource.ID, VersionNumber: 1, Hash: "plantedhash", Location: jobs.JobReplayKeyFileName, ContentType: "application/octet-stream"}
	if err := tc.DB.Create(plantedVersion).Error; err != nil {
		t.Fatalf("plant version: %v", err)
	}
	for who, bearer := range bearers {
		rr := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/resource/version/file?versionId=%d", plantedVersion.ID),
			map[string]string{"Authorization": bearer}, nil, nil)
		if rr.Code == http.StatusOK || strings.Contains(rr.Body.String(), string(secret)) {
			t.Errorf("%s GET version file of a planted private location = %d, body %q", who, rr.Code, rr.Body.String())
		}
	}

	rr := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/resource/delete?Id=%d", plantedResource.ID),
		map[string]string{"Accept": "application/json", "Authorization": bearers["admin"]}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin delete of planted resource = %d (body: %s)", rr.Code, rr.Body.String())
	}

	for name, want := range files {
		got, err := afero.ReadFile(fs, name)
		if err != nil {
			t.Errorf("%s is gone after the attempts: %v", name, err)
			continue
		}
		if string(got) != string(want) {
			t.Errorf("%s changed: %q", name, got)
		}
	}
}
