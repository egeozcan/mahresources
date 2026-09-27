package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"mahresources/jobs"
)

func jobAPIError(t *testing.T, tc *TestContext, method, url string, body any, wantStatus int) map[string]any {
	t.Helper()
	res := tc.MakeRequest(method, url, body)
	if res.Code != wantStatus {
		t.Fatalf("%s %s answered %d, want %d: %s", method, url, res.Code, wantStatus, res.Body.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("decode %s %s: %v (%s)", method, url, err, res.Body.String())
	}
	return decoded
}

// TestJobListRefusalsSayWhatTheCallerGotWrong pins the wording of the list's 400s:
// the refused value in the caller's terms, without the service's internal
// wrapping, and a limit refused exactly where the message says the range ends.
func TestJobListRefusalsSayWhatTheCallerGotWrong(t *testing.T) {
	tc := SetupTestEnv(t)

	for url, want := range map[string]string{
		"/v1/jobs?state=bogus":         `invalid filter: unknown state "bogus"`,
		"/v1/jobs/summary?state=bogus": `invalid filter: unknown state "bogus"`,
		"/v1/jobs?limit=0":             "limit must be between 1 and 200",
		"/v1/jobs?limit=-1":            "limit must be between 1 and 200",
		"/v1/jobs?limit=201":           "limit must be between 1 and 200",
	} {
		if got := jobAPIError(t, tc, http.MethodGet, url, nil, http.StatusBadRequest)["error"]; got != want {
			t.Errorf("%s answered error %q, want %q", url, got, want)
		}
	}
}

// TestJobSummaryWindowRefusalsNameTheCeilingInDays covers windows written the
// way the parameter documents them: a window over the ceiling is refused for
// being too long, not for being malformed, and the ceiling is named as the
// 90 days the interface talks in.
func TestJobSummaryWindowRefusalsNameTheCeilingInDays(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, window := range []string{"91d", "13w", "2160h1s", "999999999d"} {
		url := "/v1/jobs/summary?window=" + window
		if got := jobAPIError(t, tc, http.MethodGet, url, nil, http.StatusBadRequest)["error"]; got != "window may not exceed 90 days" {
			t.Errorf("%s answered error %q", url, got)
		}
	}
	for _, window := range []string{"0d", "-1h", "soon"} {
		url := "/v1/jobs/summary?window=" + window
		if got := jobAPIError(t, tc, http.MethodGet, url, nil, http.StatusBadRequest)["error"]; got != "window must be a positive duration such as 24h or 7d" {
			t.Errorf("%s answered error %q", url, got)
		}
	}
	res := tc.MakeRequest(http.MethodGet, "/v1/jobs/summary?window=90d", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("the 90-day ceiling itself answered %d: %s", res.Code, res.Body.String())
	}

	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	body := map[string]any{"from": from, "to": from.Add(30 * 24 * time.Hour), "format": "csv"}
	if got := jobAPIError(t, tc, http.MethodPost, "/v1/jobs/summary/export", body, http.StatusBadRequest)["error"]; got != "summary export range must exceed 90 days" {
		t.Errorf("a short export range answered error %q", got)
	}
}

// TestJobCommandConflictsSayWhichConflict covers the single-command 409s that
// carried no result: a client could not tell a reused idempotency key from a
// request still in flight or a lineage that already moved on. Each now names its
// conflict and carries the same code a bulk answer gives it.
func TestJobCommandConflictsSayWhichConflict(t *testing.T) {
	tc := SetupTestEnv(t)
	accepted, err := tc.AppCtx.JobService().Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", Title: "a download to cancel",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	url := "/v1/jobs/" + accepted.ID + "/commands/cancel"
	first := map[string]any{"expectedVersion": accepted.Version, "idempotencyKey": "reused-key", "origin": "api"}
	if res := tc.MakeRequest(http.MethodPost, url, first); res.Code != http.StatusOK {
		t.Fatalf("first cancel answered %d: %s", res.Code, res.Body.String())
	}

	different := map[string]any{"expectedVersion": accepted.Version, "idempotencyKey": "reused-key", "origin": "cli"}
	answer := jobAPIError(t, tc, http.MethodPost, url, different, http.StatusConflict)
	if answer["error"] != "that idempotency key was used for a different request" {
		t.Errorf("a reused key answered error %q", answer["error"])
	}
	result, _ := answer["result"].(map[string]any)
	if result["code"] != jobs.CommandCodeKeyReused || result["jobId"] != accepted.ID {
		t.Errorf("a reused key answered result %v, want code %q for %s", answer["result"], jobs.CommandCodeKeyReused, accepted.ID)
	}
}

// TestJobPreferenceFiltersAcceptAny covers the Job Center's "Any" choice sent to
// the API: a /jobs query string with dismissed=any, pasted into the API or the
// CLI, must read the same rather than be refused.
func TestJobPreferenceFiltersAcceptAny(t *testing.T) {
	tc := SetupTestEnv(t)
	for _, url := range []string{
		"/v1/jobs?dismissed=any", "/v1/jobs?pinned=any",
		"/v1/jobs/summary?dismissed=any", "/v1/jobs/summary?pinned=any",
	} {
		if res := tc.MakeRequest(http.MethodGet, url, nil); res.Code != http.StatusOK {
			t.Errorf("%s answered %d: %s", url, res.Code, res.Body.String())
		}
	}
	if got := jobAPIError(t, tc, http.MethodGet, "/v1/jobs?dismissed=maybe", nil, http.StatusBadRequest)["error"]; got != "dismissed must be true, false or any" {
		t.Errorf("dismissed=maybe answered error %q", got)
	}
}

// TestJobCenterJSONIsAnsweredInPlace covers the Job Center's own address asked
// for as JSON. The page writes its dismissal default into its address with a
// redirect, and a request the renderer answers as JSON must get the JSON, with
// the default applied, rather than a 302.
func TestJobCenterJSONIsAnsweredInPlace(t *testing.T) {
	tc := SetupTestEnv(t)
	for _, accept := range []string{"application/json", "application/json, text/html"} {
		res := tc.requestWithAccept(http.MethodGet, "/jobs", accept, "")
		if res.Code != http.StatusOK {
			t.Fatalf("/jobs with Accept %q answered %d to %q", accept, res.Code, res.Header().Get("Location"))
		}
	}
	if res := tc.requestWithAccept(http.MethodGet, "/jobs", browserAccept, ""); res.Code != http.StatusFound || res.Header().Get("Location") != "/jobs?dismissed=false" {
		t.Fatalf("a browser navigation to /jobs answered %d to %q, want the default written into the address", res.Code, res.Header().Get("Location"))
	}
}
