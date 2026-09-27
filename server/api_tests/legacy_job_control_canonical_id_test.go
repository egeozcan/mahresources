package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"mahresources/download_queue"
	"mahresources/jobs"
)

// legacyControlAnswer is the body every legacy control answers with.
type legacyControlAnswer struct {
	Status         string `json:"status"`
	CanonicalJobID string `json:"canonicalJobId"`
}

func postLegacyControl(t *testing.T, tc *TestContext, action, id string) legacyControlAnswer {
	t.Helper()
	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/"+action+"?id="+id, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("%s by id %s answered %d: %s", action, id, res.Code, res.Body.String())
	}
	var answer legacyControlAnswer
	if err := json.Unmarshal(res.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s answer %s: %v", action, res.Body.String(), err)
	}
	return answer
}

// TestLegacyJobControlsAcceptTheCanonicalJobID drives the ids `/v1/jobs` and
// `mr jobs list` print through the controls `mr job cancel|pause|resume|retry`
// call. Every one of them answered 404 for a canonical id, so the only ids a
// client could list were ids it could not act on.
func TestLegacyJobControlsAcceptTheCanonicalJobID(t *testing.T) {
	tc := SetupTestEnv(t)
	srv := trickleServer(t)
	group := tc.CreateDummyGroup("canonical id controls")

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/download/submit",
		map[string]any{"URL": srv.URL + "/slow.dat", "OwnerId": group.ID})
	if res.Code != http.StatusAccepted {
		t.Fatalf("submit answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		Jobs []struct {
			ID             string `json:"id"`
			CanonicalJobID string `json:"canonicalJobId"`
		} `json:"jobs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil || len(submitted.Jobs) != 1 || submitted.Jobs[0].CanonicalJobID == "" {
		t.Fatalf("unexpected submit response %s (%v)", res.Body.String(), err)
	}
	handle, canonicalID := submitted.Jobs[0].ID, submitted.Jobs[0].CanonicalJobID
	waitForJobStatus(t, tc, handle, download_queue.JobStatusDownloading)

	if answer := postLegacyControl(t, tc, "pause", canonicalID); answer.Status != "paused" || answer.CanonicalJobID != canonicalID {
		t.Fatalf("pause answered %+v, want paused for %s", answer, canonicalID)
	}
	waitForJobStatus(t, tc, handle, download_queue.JobStatusPaused)

	res = tc.MakeRequest(http.MethodGet, "/v1/jobs/get?id="+canonicalID, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("get by canonical id answered %d: %s", res.Code, res.Body.String())
	}
	var row struct {
		ID             string `json:"id"`
		CanonicalJobID string `json:"canonicalJobId"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &row); err != nil || row.ID != canonicalID || row.CanonicalJobID != canonicalID {
		t.Fatalf("get by canonical id answered %s (%v), want the row under the id asked for", res.Body.String(), err)
	}

	if answer := postLegacyControl(t, tc, "resume", canonicalID); answer.Status != "resumed" || answer.CanonicalJobID != canonicalID {
		t.Fatalf("resume answered %+v, want resumed for %s", answer, canonicalID)
	}
	if answer := postLegacyControl(t, tc, "cancel", canonicalID); answer.Status != "cancelled" || answer.CanonicalJobID != canonicalID {
		t.Fatalf("cancel answered %+v, want cancelled for %s", answer, canonicalID)
	}
	waitForCanonicalState(t, tc, canonicalID, "the Job to be cancelled", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StateCancelled
	})

	retried := postLegacyControl(t, tc, "retry", canonicalID)
	if retried.Status != "retrying" || retried.CanonicalJobID == "" || retried.CanonicalJobID == canonicalID {
		t.Fatalf("retry answered %+v, want a successor of %s", retried, canonicalID)
	}
	postLegacyControl(t, tc, "cancel", retried.CanonicalJobID)
}
