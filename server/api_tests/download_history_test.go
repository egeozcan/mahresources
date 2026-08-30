package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
)

// recordDownload writes one history row through the same entry point the
// download manager uses, so these tests exercise the production write path.
func recordDownload(t *testing.T, tc *TestContext, jobID, status string, owner *uint, url string, completedAt time.Time) models.DownloadHistoryEntry {
	t.Helper()
	payload := fmt.Sprintf(`{"URL":%q}`, url)
	if err := tc.AppCtx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID:           jobID,
		URL:             url,
		Name:            jobID,
		Status:          status,
		CreatedAt:       completedAt.Add(-time.Minute),
		CompletedAt:     &completedAt,
		CreatedByUserId: owner,
		Payload:         []byte(payload),
	}); err != nil {
		t.Fatalf("record %s: %v", jobID, err)
	}
	var entry models.DownloadHistoryEntry
	if err := tc.DB.Where("job_id = ?", jobID).First(&entry).Error; err != nil {
		t.Fatalf("load %s: %v", jobID, err)
	}
	return entry
}

func decodeJSON(t *testing.T, res *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatalf("decode response: %v (body: %s)", err, res.Body.String())
	}
	return out
}

func postJSON(tc *TestContext, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	all := map[string]string{"Content-Type": "application/json"}
	for k, v := range headers {
		all[k] = v
	}
	return doReq(tc, http.MethodPost, path, all, nil, strings.NewReader(body))
}

func TestDownloadHistoryListFiltersByStatusAndTerm(t *testing.T) {
	tc := SetupTestEnv(t)
	now := time.Now()
	recordDownload(t, tc, "h-failed", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/holiday.zip", now)
	recordDownload(t, tc, "h-done", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/other.bin", now)
	recordDownload(t, tc, "h-cancelled", models.DownloadHistoryStatusCancelled, nil, "http://example.invalid/third.bin", now)

	res := doReq(tc, http.MethodGet, "/v1/downloads", nil, nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("list: status %d (%s)", res.Code, res.Body.String())
	}
	body := decodeJSON(t, res)
	if got := len(body["downloads"].([]any)); got != 3 {
		t.Fatalf("unfiltered listing = %d rows, want 3", got)
	}

	res = doReq(tc, http.MethodGet, "/v1/downloads?status=failed&status=cancelled", nil, nil, nil)
	body = decodeJSON(t, res)
	if got := len(body["downloads"].([]any)); got != 2 {
		t.Fatalf("status-filtered listing = %d rows, want 2", got)
	}

	res = doReq(tc, http.MethodGet, "/v1/downloads?url=holiday", nil, nil, nil)
	body = decodeJSON(t, res)
	rows := body["downloads"].([]any)
	if len(rows) != 1 {
		t.Fatalf("term-filtered listing = %d rows, want 1", len(rows))
	}
	if got := rows[0].(map[string]any)["jobId"]; got != "h-failed" {
		t.Fatalf("term-filtered row = %v, want h-failed", got)
	}
}

// The finish window, the reason bucket and the error box reach the endpoint the
// same way the status and term filters do. Here as well as in the context
// package because this is where the query is decoded from a real query string —
// a field the decoder cannot see is a filter the page silently ignores.
func TestDownloadHistoryListFiltersByFinishTimeAndFailure(t *testing.T) {
	tc := SetupTestEnv(t)
	monday := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	wednesday := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	recordDownload(t, tc, "early", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/early.bin", monday)
	late := recordDownload(t, tc, "late", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/late.bin", wednesday)
	if err := tc.DB.Model(&models.DownloadHistoryEntry{}).Where("id = ?", late.ID).
		Update("error", "HTTP 404: 404 Not Found").Error; err != nil {
		t.Fatalf("set the error text: %v", err)
	}
	// One submission date for both rows. recordDownload derives CreatedAt from
	// the completion time, which put each row's two timestamps on the same side
	// of every boundary below — so a filter secretly still reading created_at
	// would have satisfied every assertion in this test.
	submitted := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	if err := tc.DB.Model(&models.DownloadHistoryEntry{}).
		Where("job_id IN ?", []string{"early", "late"}).
		Update("created_at", submitted).Error; err != nil {
		t.Fatalf("align the submission dates: %v", err)
	}

	jobIDs := func(query string) []string {
		t.Helper()
		res := doReq(tc, http.MethodGet, "/v1/downloads?"+query, nil, nil, nil)
		if res.Code != http.StatusOK {
			t.Fatalf("%s: status %d (%s)", query, res.Code, res.Body.String())
		}
		out := []string{}
		for _, row := range decodeJSON(t, res)["downloads"].([]any) {
			out = append(out, row.(map[string]any)["jobId"].(string))
		}
		sort.Strings(out)
		return out
	}

	for query, want := range map[string][]string{
		"completedAfter=2026-03-03":  {"late"},
		"completedBefore=2026-03-03": {"early"},
		// RFC 3339 bounds, which each engine reaches by a different route: a
		// normalised strftime comparison on SQLite, a canonical timestamptz on
		// Postgres. A comma fraction is valid RFC 3339 to Go and is rejected by
		// both engines' own parsers, so it has to survive the round trip here.
		"completedAfter=2026-03-03T12%3A00%3A00Z":   {"late"},
		"completedBefore=2026-03-03T12%3A00%3A00Z":  {"early"},
		"completedAfter=2026-03-03T12%3A00%3A00,5Z": {"late"},
		"reason=http":            {"late"},
		"reason=timeout":         {},
		"error=404":              {"late"},
		"reason=http&error=nope": {},
	} {
		if got := jobIDs(query); !reflect.DeepEqual(got, want) {
			t.Errorf("?%s = %v, want %v", query, got, want)
		}
	}

	// A date the caller mistyped is the caller's mistake, and it used to be
	// reported as an outage: "is not a valid date" matches nothing in
	// statusCodeForError's message scan, so the error fell through to 500.
	res := doReq(tc, http.MethodGet, "/v1/downloads?completedAfter=last+tuesday", nil, nil, nil)
	if res.Code != http.StatusBadRequest {
		t.Errorf("a malformed date answered %d (%s), want 400", res.Code, res.Body.String())
	}
}

// Recording the same job twice updates the row it already has and counts the
// attempt.
//
// Here rather than only in the context package because this is the one
// dialect-sensitive statement in the feature: the ON CONFLICT ... DO UPDATE has
// to name the target table to read the stored value (`download_history_entries.
// attempts + 1`), and this package is the one that also runs under the postgres
// build tag.
func TestDownloadHistoryUpsertCountsAttempts(t *testing.T) {
	tc := SetupTestEnv(t)
	now := time.Now()

	recordDownload(t, tc, "twice", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/twice", now)
	entry := recordDownload(t, tc, "twice", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/twice", now)

	if entry.Attempts != 2 {
		t.Fatalf("attempts = %d, want 2", entry.Attempts)
	}
	if entry.Status != models.DownloadHistoryStatusCompleted {
		t.Fatalf("status = %q, want the latest outcome", entry.Status)
	}

	var rows int64
	tc.DB.Model(&models.DownloadHistoryEntry{}).Where("job_id = ?", "twice").Count(&rows)
	if rows != 1 {
		t.Fatalf("rows for one job = %d, want 1", rows)
	}
}

// A row's stored payload is never rendered: it can carry meta the user would not
// expect a listing to publish, and the page has no use for it.
func TestDownloadHistoryListOmitsPayload(t *testing.T) {
	tc := SetupTestEnv(t)
	recordDownload(t, tc, "h-1", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/a", time.Now())

	res := doReq(tc, http.MethodGet, "/v1/downloads", nil, nil, nil)
	if strings.Contains(res.Body.String(), "payload") || strings.Contains(res.Body.String(), "Payload") {
		t.Fatalf("listing exposes the stored payload: %s", res.Body.String())
	}
}

// Under -auth a non-admin sees only its own downloads, and a row with no owner
// belongs to nobody — the same predicate the live queue and the SSE stream apply.
func TestDownloadHistoryVisibilityIsPerUser(t *testing.T) {
	tc := setupAuthEnv(t)
	userBearer := roleBearer(t, tc, models.RoleUser)

	var user models.User
	if err := tc.DB.Where("username = ?", "rb_user").First(&user).Error; err != nil {
		t.Fatalf("load user: %v", err)
	}
	other := uint(9999)

	mine := recordDownload(t, tc, "mine", models.DownloadHistoryStatusFailed, &user.ID, "http://example.invalid/mine", time.Now())
	theirs := recordDownload(t, tc, "theirs", models.DownloadHistoryStatusFailed, &other, "http://example.invalid/theirs", time.Now())
	recordDownload(t, tc, "ownerless", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/none", time.Now())

	res := doReq(tc, http.MethodGet, "/v1/downloads", map[string]string{"Authorization": userBearer}, nil, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("list as user: status %d (%s)", res.Code, res.Body.String())
	}
	rows := decodeJSON(t, res)["downloads"].([]any)
	if len(rows) != 1 {
		t.Fatalf("user sees %d rows, want only its own", len(rows))
	}
	if got := rows[0].(map[string]any)["id"]; uint(got.(float64)) != mine.ID {
		t.Fatalf("user sees row %v, want %d", got, mine.ID)
	}

	// Another user's row cannot be deleted, and is reported as absent rather than
	// forbidden so ids cannot be probed.
	res = postJSON(tc, "/v1/downloads/delete", fmt.Sprintf(`{"ids":[%d]}`, theirs.ID), map[string]string{"Authorization": userBearer})
	if res.Code != http.StatusNotFound {
		t.Fatalf("delete of another user's row: status %d, want 404 (%s)", res.Code, res.Body.String())
	}
	var stillThere int64
	tc.DB.Model(&models.DownloadHistoryEntry{}).Where("id = ?", theirs.ID).Count(&stillThere)
	if stillThere != 1 {
		t.Fatal("another user's history row was deleted")
	}

	// An admin sees everything, including the ownerless row.
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	res = doReq(tc, http.MethodGet, "/v1/downloads", map[string]string{"Authorization": adminBearer}, nil, nil)
	if got := len(decodeJSON(t, res)["downloads"].([]any)); got != 3 {
		t.Fatalf("admin sees %d rows, want 3", got)
	}
}

// A retry of a row whose job the queue has long since evicted still works — that
// is the whole reason the submission is stored alongside it.
func TestDownloadHistoryRetryResubmitsEvictedJob(t *testing.T) {
	tc := SetupTestEnv(t)
	entry := recordDownload(t, tc, "gone-from-queue", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/retry-me", time.Now())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("retry: status %d (%s)", res.Code, res.Body.String())
	}
	body := decodeJSON(t, res)
	if got := body["retried"].(float64); got != 1 {
		t.Fatalf("retried = %v, want 1", got)
	}
	results := body["results"].([]any)
	newJobID, _ := results[0].(map[string]any)["jobId"].(string)
	if newJobID == "" {
		t.Fatal("retry did not report the job it queued")
	}
	if newJobID == entry.JobID {
		t.Fatalf("retry reused the evicted job id %q; a resubmission is a new job", newJobID)
	}

	if _, exists := tc.AppCtx.DownloadManager().GetJob(newJobID); !exists {
		t.Fatalf("job %q is not in the queue", newJobID)
	}

	var reloaded models.DownloadHistoryEntry
	if err := tc.DB.First(&reloaded, entry.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.LastRetryJobID != newJobID || reloaded.LastRetryAt == nil {
		t.Fatalf("row does not point at the retry (jobId=%q at=%v)", reloaded.LastRetryJobID, reloaded.LastRetryAt)
	}
}

// Retrying a completed download would create a second copy of a resource that
// already exists, so it is refused rather than quietly duplicated.
func TestDownloadHistoryRetryRefusesCompleted(t *testing.T) {
	tc := SetupTestEnv(t)
	entry := recordDownload(t, tc, "already-done", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/done", time.Now())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("retry of a completed download: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
}

// A batch reports per id. Failing the whole request would hide the rows that
// were accepted.
func TestDownloadHistoryRetryReportsPerID(t *testing.T) {
	tc := SetupTestEnv(t)
	ok := recordDownload(t, tc, "retryable", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/ok", time.Now())
	no := recordDownload(t, tc, "not-retryable", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/no", time.Now())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d,%d]}`, ok.ID, no.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("mixed batch: status %d, want 200 (%s)", res.Code, res.Body.String())
	}
	body := decodeJSON(t, res)
	if got := body["retried"].(float64); got != 1 {
		t.Fatalf("retried = %v, want 1", got)
	}
	results := body["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("results = %d, want one per requested id", len(results))
	}
	for _, r := range results {
		row := r.(map[string]any)
		id := uint(row["id"].(float64))
		if id == no.ID {
			if row["ok"].(bool) {
				t.Error("a completed download was reported as retried")
			}
			if row["reason"] == "" {
				t.Error("a refused id carries no reason")
			}
		}
	}
}

// Deleting a row takes its queue entry with it. Otherwise the SSE stream replays
// the whole queue on the next reconnect and the download reappears in the panel.
func TestDownloadHistoryDeleteRemovesQueuedJob(t *testing.T) {
	tc := SetupTestEnv(t)
	dm := tc.AppCtx.DownloadManager()

	// A finished job in the queue, and the history row that names it.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()
	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForJobStatus(t, tc, job.ID, download_queue.JobStatusFailed)

	var entry models.DownloadHistoryEntry
	if err := tc.DB.Where("job_id = ?", job.ID).First(&entry).Error; err != nil {
		t.Fatalf("the failed download was not recorded: %v", err)
	}

	res := postJSON(tc, "/v1/downloads/delete", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("delete: status %d (%s)", res.Code, res.Body.String())
	}

	if _, exists := dm.GetJob(job.ID); exists {
		t.Fatal("the queue still holds the deleted download; an SSE reconnect would bring it back")
	}
	var remaining int64
	tc.DB.Model(&models.DownloadHistoryEntry{}).Where("id = ?", entry.ID).Count(&remaining)
	if remaining != 0 {
		t.Fatal("the history row survived the delete")
	}
}

// The listing merges nothing from the queue, so an in-flight download has no row
// yet — but a paused one that somehow has a row must not be deletable, since it
// holds a half-transferred file.
func TestDownloadHistoryDeleteRefusesRunningJob(t *testing.T) {
	tc := SetupTestEnv(t)
	dm := tc.AppCtx.DownloadManager()

	// A server that never finishes, so the job stays in flight.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForJobStatus(t, tc, job.ID, download_queue.JobStatusDownloading)

	// A row naming the running job (as one would exist after an earlier failure
	// that has since been retried in place).
	entry := recordDownload(t, tc, job.ID, models.DownloadHistoryStatusFailed, nil, server.URL, time.Now())

	res := postJSON(tc, "/v1/downloads/delete", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("delete of a running download: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	var remaining int64
	tc.DB.Model(&models.DownloadHistoryEntry{}).Where("id = ?", entry.ID).Count(&remaining)
	if remaining != 1 {
		t.Fatal("a running download's row was deleted")
	}
}

// A group-limited principal may not resubmit a payload that targets a group
// outside its subtree. The stored payload records what was once asked for; it is
// not a standing permission.
func TestDownloadHistoryRetryRevalidatesScope(t *testing.T) {
	tc := setupAuthEnv(t)

	outside := &models.Group{Name: "outside-scope"}
	if err := tc.DB.Create(outside).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	scopeGroup := &models.Group{Name: "inside-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	confined, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "confined", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scopeGroup.ID,
	})
	if err != nil {
		t.Fatalf("create confined user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(confined.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	// The row is the confined user's own, so visibility is not what refuses it.
	payload := fmt.Sprintf(`{"URL":"http://example.invalid/x","OwnerId":%d}`, outside.ID)
	if err := tc.AppCtx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID: "out-of-scope", URL: "http://example.invalid/x", Status: models.DownloadHistoryStatusFailed,
		CreatedAt: time.Now(), CreatedByUserId: &confined.ID, Payload: []byte(payload),
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	var entry models.DownloadHistoryEntry
	if err := tc.DB.Where("job_id = ?", "out-of-scope").First(&entry).Error; err != nil {
		t.Fatalf("load: %v", err)
	}

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), map[string]string{"Authorization": "Bearer " + raw})
	if res.Code != http.StatusConflict {
		t.Fatalf("out-of-scope retry: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "scope") {
		t.Fatalf("refusal does not mention scope: %s", res.Body.String())
	}
}

// A row whose job is back in the queue is not retried again. The row still says
// `failed` because it records how the download *ended*, and the attempt running
// now has not ended — resubmitting from the payload here started a second,
// concurrent download of the same URL, so one Retry in the jobs panel plus one on
// this page produced two copies of the same file.
func TestDownloadHistoryRetryRefusesJobAlreadyRunning(t *testing.T) {
	tc := SetupTestEnv(t)
	dm := tc.AppCtx.DownloadManager()

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	job, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForJobStatus(t, tc, job.ID, download_queue.JobStatusDownloading)

	entry := recordDownload(t, tc, job.ID, models.DownloadHistoryStatusFailed, nil, server.URL, time.Now())
	before := len(dm.GetJobs())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("retry of a running download: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if after := len(dm.GetJobs()); after != before {
		t.Fatalf("the queue grew from %d to %d jobs: the retry forked a second download of the same URL", before, after)
	}
}

// The queue's own retry replays the same stored payload on the same unscoped
// worker as the history page's, so it clears the same bar — against the principal
// pressing the button. Owning a job is not permission to run it again after the
// account has been confined.
func TestJobRetryRevalidatesScope(t *testing.T) {
	tc := setupAuthEnv(t)

	outside := &models.Group{Name: "retry-outside-scope"}
	if err := tc.DB.Create(outside).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	scopeGroup := &models.Group{Name: "retry-inside-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	confined, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "retry-confined", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scopeGroup.ID,
	})
	if err != nil {
		t.Fatalf("create confined user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(confined.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	// Submitted straight to the manager: the job predates the confinement, which
	// is exactly the case the check exists for.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()
	job, err := tc.AppCtx.DownloadManager().Submit(&query_models.ResourceFromRemoteCreator{
		URL:               server.URL,
		ResourceQueryBase: query_models.ResourceQueryBase{OwnerId: outside.ID},
	}, &confined.ID)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForJobStatus(t, tc, job.ID, download_queue.JobStatusFailed)

	res := doReq(tc, http.MethodPost, "/v1/jobs/retry?id="+job.ID,
		map[string]string{"Authorization": "Bearer " + raw}, nil, nil)
	if res.Code != http.StatusForbidden {
		t.Fatalf("retry of an out-of-scope payload: status %d, want 403 (%s)", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "scope") {
		t.Fatalf("refusal does not mention scope: %s", res.Body.String())
	}
}

// Notes are subtree-scoped, and the download worker associates them on the
// unscoped context — so an out-of-subtree note id in the submission is a write
// outside the principal's scope. The foreground upload path validates the same
// ids through the scoped db and refuses them; this is the background path's
// equivalent.
func TestDownloadSubmitRefusesOutOfScopeNote(t *testing.T) {
	tc := setupAuthEnv(t)

	outside := &models.Group{Name: "note-outside-scope"}
	if err := tc.DB.Create(outside).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}
	scopeGroup := &models.Group{Name: "note-inside-scope"}
	if err := tc.DB.Create(scopeGroup).Error; err != nil {
		t.Fatalf("create scope group: %v", err)
	}
	foreignNote := &models.Note{Name: "not yours", OwnerId: &outside.ID}
	if err := tc.DB.Create(foreignNote).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	confined, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "note-confined", Password: "password1", Role: models.RoleUser, ScopeGroupId: &scopeGroup.ID,
	})
	if err != nil {
		t.Fatalf("create confined user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(confined.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}

	body := fmt.Sprintf(`{"URL":"http://example.invalid/x","OwnerId":%d,"Notes":[%d]}`, scopeGroup.ID, foreignNote.ID)
	res := postJSON(tc, "/v1/download/submit", body, map[string]string{"Authorization": "Bearer " + raw})
	if res.Code != http.StatusForbidden {
		t.Fatalf("submit naming an out-of-scope note: status %d, want 403 (%s)", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "scope") {
		t.Fatalf("refusal does not mention scope: %s", res.Body.String())
	}
}

// A stale outcome never overwrites a newer one. The recording goroutine runs
// after its attempt has already been published, so a slow write from a failed
// attempt can land after the retry that followed it has completed — and the row
// would then report `failed`, with the older attempt's timestamps, for a download
// whose file exists. Here rather than only in the context package because the
// guard is a dialect-sensitive `ON CONFLICT ... DO UPDATE ... WHERE`.
func TestDownloadHistoryUpsertIgnoresStaleOutcome(t *testing.T) {
	tc := SetupTestEnv(t)
	newer := time.Now()
	older := newer.Add(-time.Minute)

	entry := recordDownload(t, tc, "out-of-order", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/x", newer)
	recordDownload(t, tc, "out-of-order", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/x", older)

	var reloaded models.DownloadHistoryEntry
	if err := tc.DB.First(&reloaded, entry.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.DownloadHistoryStatusCompleted {
		t.Fatalf("status = %q, want completed: an older attempt overwrote a newer one", reloaded.Status)
	}

	// A newer outcome still lands, so the guard is a rule about order rather than
	// a row that has stopped accepting updates.
	recordDownload(t, tc, "out-of-order", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/x", newer.Add(time.Minute))
	if err := tc.DB.First(&reloaded, entry.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.DownloadHistoryStatusFailed {
		t.Fatalf("status = %q, want failed: a newer outcome was rejected", reloaded.Status)
	}
}

// A resubmission produces a *new* job, so the row's own job id says nothing about
// it — LastRetryJobID does. Without consulting it, a second press of Retry starts
// a third concurrent download of the same URL, and a delete discards the row
// while the attempt launched from it is still running.
func TestDownloadHistoryRetryAndDeleteFollowTheResubmittedJob(t *testing.T) {
	tc := SetupTestEnv(t)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	// A row whose job the queue never had: the resubmission path.
	entry := recordDownload(t, tc, "evicted-and-retried", models.DownloadHistoryStatusFailed, nil, server.URL, time.Now())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("first retry: status %d (%s)", res.Code, res.Body.String())
	}
	newJobID := decodeJSON(t, res)["results"].([]any)[0].(map[string]any)["jobId"].(string)
	waitForJobStatus(t, tc, newJobID, download_queue.JobStatusDownloading)

	before := len(tc.AppCtx.DownloadManager().GetJobs())

	res = postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("second retry while the first is running: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if after := len(tc.AppCtx.DownloadManager().GetJobs()); after != before {
		t.Fatalf("the queue grew from %d to %d: the row forked a second download of the same URL", before, after)
	}

	res = postJSON(tc, "/v1/downloads/delete", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("delete while the retry runs: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	var remaining int64
	tc.DB.Model(&models.DownloadHistoryEntry{}).Where("id = ?", entry.ID).Count(&remaining)
	if remaining != 1 {
		t.Fatal("the row was deleted while the download it started was still running")
	}
}

// A row whose resubmission already succeeded is not retried again, and the answer
// survives the queue forgetting the job. The old failure keeps its row, but the
// file it was after exists now — running the download again would fetch a second
// copy of it.
func TestDownloadHistoryRetryRefusesWhenTheResubmissionSucceeded(t *testing.T) {
	tc := SetupTestEnv(t)

	failed := recordDownload(t, tc, "old-failure", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/x", time.Now())
	recordDownload(t, tc, "the-successful-retry", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/x", time.Now())

	// The link the resubmission wrote. The job itself is long gone from the queue,
	// which is exactly the case that used to unblock a duplicate.
	if err := tc.AppCtx.MarkDownloadHistoryRetried(failed.ID, "the-successful-retry", time.Now()); err != nil {
		t.Fatalf("mark retried: %v", err)
	}
	if _, exists := tc.AppCtx.DownloadManager().GetJob("the-successful-retry"); exists {
		t.Fatal("the fixture assumes the retry job is not in the queue")
	}

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, failed.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("retry of a row whose resubmission succeeded: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "already succeeded") {
		t.Fatalf("refusal does not say why: %s", res.Body.String())
	}

	// It is not a permanent block: the row may still be deleted, because nothing is
	// running.
	res = postJSON(tc, "/v1/downloads/delete", fmt.Sprintf(`{"ids":[%d]}`, failed.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("delete of the same row: status %d, want 200 (%s)", res.Code, res.Body.String())
	}
}

// The claim marker is not a job id, and a request that reads one must not be able
// to take it over: that is the window between another request claiming the slot
// and its submit landing, and stealing it there starts the same download twice.
func TestDownloadHistoryRetryRefusesAClaimInFlight(t *testing.T) {
	tc := SetupTestEnv(t)
	entry := recordDownload(t, tc, "mid-claim", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/x", time.Now())

	// The state another request leaves behind while it is inside Submit.
	if err := tc.AppCtx.MarkDownloadHistoryRetried(entry.ID, "claiming-1-1", time.Now()); err != nil {
		t.Fatalf("mark claimed: %v", err)
	}

	before := len(tc.AppCtx.DownloadManager().GetJobs())
	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("retry during another request's claim: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if after := len(tc.AppCtx.DownloadManager().GetJobs()); after != before {
		t.Fatalf("the queue grew from %d to %d: the claim was stolen mid-submit", before, after)
	}
}

// Two rows can record the same URL — a failure, and the failure of the retry it
// spawned. Selecting both and pressing Retry is an ordinary thing to do, and it
// must not run the same transfer twice at once.
func TestDownloadHistoryBulkRetryRunsEachURLOnce(t *testing.T) {
	tc := SetupTestEnv(t)
	url := "http://example.invalid/same-file.bin"

	first := recordDownload(t, tc, "attempt-one", models.DownloadHistoryStatusFailed, nil, url, time.Now().Add(-time.Hour))
	second := recordDownload(t, tc, "attempt-two", models.DownloadHistoryStatusFailed, nil, url, time.Now())

	before := len(tc.AppCtx.DownloadManager().GetJobs())
	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d,%d]}`, first.ID, second.ID), nil)
	if res.Code != http.StatusOK {
		t.Fatalf("bulk retry: status %d (%s)", res.Code, res.Body.String())
	}
	if got := decodeJSON(t, res)["retried"].(float64); got != 1 {
		t.Fatalf("retried = %v, want 1: the same URL was queued more than once", got)
	}
	if after := len(tc.AppCtx.DownloadManager().GetJobs()); after != before+1 {
		t.Fatalf("the queue grew by %d, want 1", after-before)
	}
}

// Two requests retrying one row at the same moment must produce one download.
// Both find no live attempt, so what separates them is the claim taken before the
// submit, not the checks before it.
func TestDownloadHistoryConcurrentRetriesSubmitOnce(t *testing.T) {
	tc := SetupTestEnv(t)
	entry := recordDownload(t, tc, "contended-retry", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/x", time.Now())

	before := len(tc.AppCtx.DownloadManager().GetJobs())
	body := fmt.Sprintf(`{"ids":[%d]}`, entry.ID)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = postJSON(tc, "/v1/downloads/retry", body, nil).Code
		}()
	}
	wg.Wait()

	accepted := 0
	for _, code := range codes {
		if code == http.StatusOK {
			accepted++
		}
	}
	if accepted != 1 {
		t.Fatalf("%d of 2 concurrent retries were accepted, want 1 (codes %v)", accepted, codes)
	}
	if after := len(tc.AppCtx.DownloadManager().GetJobs()); after != before+1 {
		t.Fatalf("the queue grew by %d, want 1: the download was forked", after-before)
	}
}

// The queue always knows what it is downloading, and that is the last defence
// against running one transfer twice: a second row recording the same URL cannot
// be retried while a job is fetching it, whatever the rows say about each other.
func TestDownloadHistoryRetryRefusesAURLAlreadyDownloading(t *testing.T) {
	tc := SetupTestEnv(t)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer server.Close()
	defer close(release)

	live, err := tc.AppCtx.DownloadManager().Submit(&query_models.ResourceFromRemoteCreator{URL: server.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForJobStatus(t, tc, live.ID, download_queue.JobStatusDownloading)

	// A different row, from an earlier attempt at the same URL, with no link to the
	// running job at all.
	entry := recordDownload(t, tc, "unlinked-earlier-attempt", models.DownloadHistoryStatusFailed, nil, server.URL, time.Now())

	before := len(tc.AppCtx.DownloadManager().GetJobs())
	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("retry of a URL already downloading: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if after := len(tc.AppCtx.DownloadManager().GetJobs()); after != before {
		t.Fatalf("the queue grew from %d to %d: the same URL is being fetched twice", before, after)
	}
}

// Two rows, two jobs still in memory, one URL. Retrying each in place ran both at
// once: the queue is the authority on what is being downloaded, so it is asked
// before the in-place branch and not after it.
func TestDownloadHistoryInPlaceRetryRefusesAURLAlreadyDownloading(t *testing.T) {
	tc := SetupTestEnv(t)
	dm := tc.AppCtx.DownloadManager()

	release := make(chan struct{})
	blocking := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
	}))
	defer blocking.Close()
	defer close(release)

	// One job already fetching the URL.
	running, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: blocking.URL}, nil)
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	waitForJobStatus(t, tc, running.ID, download_queue.JobStatusDownloading)

	// A second, failed job for the same URL, still in the queue — so the retry
	// takes the in-place branch.
	failing := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer failing.Close()
	second, err := dm.Submit(&query_models.ResourceFromRemoteCreator{URL: failing.URL}, nil)
	if err != nil {
		t.Fatalf("submit second: %v", err)
	}
	waitForJobStatus(t, tc, second.ID, download_queue.JobStatusFailed)

	// Its row names the URL the other job is fetching.
	entry := recordDownload(t, tc, second.ID, models.DownloadHistoryStatusFailed, nil, blocking.URL, time.Now())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d]}`, entry.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("in-place retry of a URL already downloading: status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	if status := second.GetStatus(); status != download_queue.JobStatusFailed {
		t.Fatalf("the job was restarted anyway (status %q)", status)
	}
}

// A wholly refused batch still answers per id. Twelve rows refused for twelve
// different reasons cannot be explained by one sentence.
func TestDownloadHistoryRefusedBatchKeepsPerIDOutcomes(t *testing.T) {
	tc := SetupTestEnv(t)
	first := recordDownload(t, tc, "done-one", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/a", time.Now())
	second := recordDownload(t, tc, "done-two", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/b", time.Now())

	res := postJSON(tc, "/v1/downloads/retry", fmt.Sprintf(`{"ids":[%d,%d]}`, first.ID, second.ID), nil)
	if res.Code != http.StatusConflict {
		t.Fatalf("status %d, want 409 (%s)", res.Code, res.Body.String())
	}
	body := decodeJSON(t, res)
	results, ok := body["results"].([]any)
	if !ok || len(results) != 2 {
		t.Fatalf("the refusal dropped the per-id outcomes: %s", res.Body.String())
	}
	for _, r := range results {
		if reason, _ := r.(map[string]any)["reason"].(string); reason == "" {
			t.Fatalf("an outcome has no reason: %s", res.Body.String())
		}
	}
}
