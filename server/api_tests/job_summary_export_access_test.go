package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/models"
)

func loginSummaryExportSession(t *testing.T, tc *TestContext, username, password string) (*http.Cookie, string) {
	t.Helper()
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, username, password)))
	if login.Code != http.StatusOK {
		t.Fatalf("login as %s: status=%d body=%s", username, login.Code, login.Body.String())
	}
	cookie := sessionCookie(t, login)
	return cookie, csrfFor(t, tc, cookie)
}

func TestSummaryExportOutputIsHiddenAfterAdminOwnerDemotion(t *testing.T) {
	tc := setupAuthEnv(t)
	installJobControlPlane(t, tc)

	admin, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "summary-export-admin", Password: "password1", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create export administrator: %v", err)
	}
	other, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "summary-export-other", Password: "password1", Role: models.RoleUser,
	})
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}

	adminCookie, adminCSRF := loginSummaryExportSession(t, tc, admin.Username, "password1")
	otherCookie, otherCSRF := loginSummaryExportSession(t, tc, other.Username, "password1")
	rootCookie, rootCSRF := loginSummaryExportSession(t, tc, "admin", "adminpw1")

	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "download fixture", http.StatusInternalServerError)
	}))
	t.Cleanup(remote.Close)
	downloaderHeaders := map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": otherCSRF,
	}
	downloaderBody := fmt.Sprintf(`{"URL":%q}`, remote.URL+"/summary-export.bin")
	download := doReq(tc, http.MethodPost, "/v1/download/submit", downloaderHeaders,
		[]*http.Cookie{otherCookie}, strings.NewReader(downloaderBody))
	if download.Code != http.StatusAccepted {
		t.Fatalf("other user download submit: status=%d body=%s", download.Code, download.Body.String())
	}

	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	exportBody, err := json.Marshal(map[string]any{"from": from, "to": to, "format": "json"})
	if err != nil {
		t.Fatalf("encode summary export request: %v", err)
	}
	export := doReq(tc, http.MethodPost, "/v1/jobs/summary/export?kinds="+application_context.JobKindRemoteDownload,
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": adminCSRF},
		[]*http.Cookie{adminCookie}, strings.NewReader(string(exportBody)))
	if export.Code != http.StatusAccepted {
		t.Fatalf("summary export submit: status=%d body=%s", export.Code, export.Body.String())
	}
	var accepted struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.Unmarshal(export.Body.Bytes(), &accepted); err != nil || accepted.Job.ID == "" {
		t.Fatalf("decode summary export acceptance %s: %v", export.Body.String(), err)
	}

	waitForCanonicalState(t, tc, accepted.Job.ID, "summary export completion", func(snapshot jobs.Snapshot) bool {
		return snapshot.State == jobs.StateSucceeded
	})

	openPath := fmt.Sprintf("/v1/jobs/%s/outputs?key=summary", accepted.Job.ID)
	before := doReq(tc, http.MethodGet, openPath, map[string]string{"Accept": "application/json"}, []*http.Cookie{adminCookie}, nil)
	if before.Code != http.StatusOK {
		t.Fatalf("current administrator opening summary output: status=%d body=%s", before.Code, before.Body.String())
	}
	var exported struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(before.Body.Bytes(), &exported); err != nil || exported.Total != 1 {
		t.Fatalf("admin summary output = total %d, decode error %v; body=%s", exported.Total, err, before.Body.String())
	}
	adminDetail := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/jobs/%s", accepted.Job.ID),
		map[string]string{"Accept": "application/json"}, []*http.Cookie{adminCookie}, nil)
	var adminDetailBody struct {
		Outputs []json.RawMessage `json:"outputs"`
	}
	if err := json.Unmarshal(adminDetail.Body.Bytes(), &adminDetailBody); err != nil || adminDetail.Code != http.StatusOK || len(adminDetailBody.Outputs) != 1 {
		t.Fatalf("administrator detail status=%d outputs=%d decode error=%v body=%s; want one output", adminDetail.Code, len(adminDetailBody.Outputs), err, adminDetail.Body.String())
	}

	demoteBody, _ := json.Marshal(map[string]any{"id": admin.ID, "role": models.RoleEditor})
	demoted := doReq(tc, http.MethodPost, "/v1/user",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": rootCSRF},
		[]*http.Cookie{rootCookie}, strings.NewReader(string(demoteBody)))
	if demoted.Code != http.StatusOK {
		t.Fatalf("root demoting export owner: status=%d body=%s", demoted.Code, demoted.Body.String())
	}

	editorCookie, _ := loginSummaryExportSession(t, tc, admin.Username, "password1")
	detail := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/jobs/%s", accepted.Job.ID),
		map[string]string{"Accept": "application/json"}, []*http.Cookie{editorCookie}, nil)
	var detailBody struct {
		Outputs []json.RawMessage `json:"outputs"`
	}
	if err := json.Unmarshal(detail.Body.Bytes(), &detailBody); err != nil {
		t.Fatalf("decode demoted owner detail (%d): %v: %s", detail.Code, err, detail.Body.String())
	}
	if detail.Code != http.StatusOK || len(detailBody.Outputs) != 0 {
		t.Errorf("demoted owner Job detail status=%d outputs=%d; want 200 and no summary output", detail.Code, len(detailBody.Outputs))
	}
	interactive := doReq(tc, http.MethodGet, "/v1/jobs/summary?kinds="+application_context.JobKindRemoteDownload+"&window=90d",
		map[string]string{"Accept": "application/json"}, []*http.Cookie{editorCookie}, nil)
	var interactiveSummary struct {
		Total int64 `json:"total"`
	}
	if err := json.Unmarshal(interactive.Body.Bytes(), &interactiveSummary); err != nil || interactive.Code != http.StatusOK || interactiveSummary.Total != 0 {
		t.Errorf("demoted owner's interactive summary status=%d total=%d decode error=%v body=%s; want 200 and total 0", interactive.Code, interactiveSummary.Total, err, interactive.Body.String())
	}
	opened := doReq(tc, http.MethodGet, openPath,
		map[string]string{"Accept": "application/json"}, []*http.Cookie{editorCookie}, nil)
	if opened.Code != http.StatusNotFound || opened.Body.String() == before.Body.String() {
		t.Errorf("demoted owner opening summary output: status=%d body=%q; want 404 without aggregate", opened.Code, opened.Body.String())
	}
}
