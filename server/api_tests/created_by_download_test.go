package api_tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/models"
)

// TestCreatedBy_BackgroundDownloadStampsSubmitter proves the P2 fix: a background
// remote download attributes the created resource (and its initial version) to
// the submitting user under auth-on, instead of stamping NULL because the worker
// runs on the singleton context.
func TestCreatedBy_BackgroundDownloadStampsSubmitter(t *testing.T) {
	tc := setupAuthEnv(t)
	userID, userBearer := userWithBearer(t, tc, "dl_user", models.RoleUser)

	// Local content server the download worker fetches from.
	content := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="8" height="8"><rect width="8" height="8"/></svg>`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write(content)
	}))
	defer server.Close()

	url := server.URL + "/cbdl-remote.svg"
	body := `{"URL":"` + url + `"}`
	hdr := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": userBearer}
	rr := doReq(tc, http.MethodPost, "/v1/download/submit", hdr, nil, strings.NewReader(body))
	if rr.Code >= 300 {
		t.Fatalf("download submit: status %d body=%s", rr.Code, rr.Body.String())
	}

	// The download runs on a background goroutine; poll for the created resource.
	var res models.Resource
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := tc.DB.Where("name = ?", "cbdl-remote.svg").First(&res).Error; err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the background download to create the resource")
		}
		time.Sleep(50 * time.Millisecond)
	}

	if res.CreatedByUserId == nil || *res.CreatedByUserId != userID {
		t.Fatalf("downloaded resource created_by=%v, want submitter %d (background attribution lost)", res.CreatedByUserId, userID)
	}

	// The initial version, if AddResource created one, must carry the same actor.
	var versions []models.ResourceVersion
	if err := tc.DB.Where("resource_id = ?", res.ID).Find(&versions).Error; err != nil {
		t.Fatalf("load versions: %v", err)
	}
	for _, v := range versions {
		if v.CreatedByUserId == nil || *v.CreatedByUserId != userID {
			t.Fatalf("initial version created_by=%v, want submitter %d", v.CreatedByUserId, userID)
		}
	}
}
