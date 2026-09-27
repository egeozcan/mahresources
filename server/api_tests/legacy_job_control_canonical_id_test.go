package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
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

// TestALegacyPauseAskedAgainIsPaused: a pause is answered once its hold is
// recorded, and a client whose first ask timed out asks again. Asking again of a
// download already paused is the same question, so it answers "paused" rather
// than refusing a download for being in the state the client asked for.
func TestALegacyPauseAskedAgainIsPaused(t *testing.T) {
	tc := SetupTestEnv(t)
	handle := pausedDownloadJob(t, tc)

	answer := postLegacyControl(t, tc, "pause", handle)
	if answer.Status != "paused" || answer.CanonicalJobID == "" {
		t.Fatalf("asking again answered %+v, want paused", answer)
	}
	waitForCanonicalState(t, tc, answer.CanonicalJobID, "the Job to be paused", func(snap jobs.Snapshot) bool {
		return snap.State == jobs.StatePaused
	})
}

// TestLegacyPauseOfAJobThisProcessDoesNotRunIsAConflict covers a Job that is
// listed but has no transfer in this server process: queued and not yet
// dispatched, or running in another process. Pause is the queue's own control,
// so there is nothing here to hold, and saying "not found" about a Job the
// caller has just listed sends them looking for the wrong problem.
func TestLegacyPauseOfAJobThisProcessDoesNotRunIsAConflict(t *testing.T) {
	tc := SetupTestEnv(t)
	accepted, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", Title: "a download nobody here runs",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/pause?id="+accepted.ID, nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("pause answered %d, want 409: %s", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "not running in this server process") {
		t.Fatalf("pause refusal does not say why: %s", res.Body.String())
	}
	if res := tc.MakeRequest(http.MethodPost, "/v1/jobs/pause?id=01a0e1d9-0000-7000-8000-000000000000", nil); res.Code != http.StatusNotFound {
		t.Fatalf("pause of an unknown Job answered %d, want 404", res.Code)
	}
}
