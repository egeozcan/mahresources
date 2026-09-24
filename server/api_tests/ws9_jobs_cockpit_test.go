package api_tests

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
	"gorm.io/gorm"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
)

// WS9 — jobs and downloads compatibility contracts.
//
// Finding 2 is the last high-severity row in the campaign: a paused download could
// never be cancelled. The HTTP status mapping is the backend half —
// `download_queue_handlers.go` mapped *any* manager error from Cancel to 404, so a
// state conflict was reported as a missing job. Finding 40's response also remains
// part of the legacy API contract. The global download cockpit has been replaced by
// the Job Center; its template and component contracts live with jobPanel.tpl and
// src/components/jobPanel.test.ts.

// trickleServer serves a slow, known-length body so a download job stays in
// `downloading` long enough to be paused. The finding is specifically about a
// paused *download*; a generic (runFn) job can never be paused, because
// CanPause refuses a streaming runner.
func trickleServer(t *testing.T) *httptest.Server {
	t.Helper()
	const total = 1 << 20
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.Itoa(total))
		w.WriteHeader(http.StatusOK)
		chunk := make([]byte, 4096)
		for sent := 0; sent < total; sent += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// pausedDownloadJob submits a real download against a trickling server, waits for
// it to start, and pauses it through the API — the exact sequence in finding 2's
// reproduction steps.
func pausedDownloadJob(t *testing.T, tc *TestContext) string {
	t.Helper()
	srv := trickleServer(t)
	group := tc.CreateDummyGroup("ws9 downloads")

	res := tc.MakeRequest(http.MethodPost, "/v1/download/submit",
		map[string]any{"URL": srv.URL + "/slow.dat", "OwnerId": group.ID})
	if res.Code != http.StatusAccepted {
		t.Fatalf("submit answered %d: %s", res.Code, res.Body.String())
	}
	var submitted struct {
		Jobs []struct{ ID string } `json:"jobs"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &submitted); err != nil || len(submitted.Jobs) != 1 {
		t.Fatalf("unexpected submit response %s (%v)", res.Body.String(), err)
	}
	jobID := submitted.Jobs[0].ID

	waitForJobStatus(t, tc, jobID, download_queue.JobStatusDownloading)

	if res := tc.MakeRequest(http.MethodPost, "/v1/jobs/pause?id="+jobID, nil); res.Code != http.StatusOK {
		t.Fatalf("pause answered %d: %s", res.Code, res.Body.String())
	}
	waitForJobStatus(t, tc, jobID, download_queue.JobStatusPaused)
	return jobID
}

// finishedTestJob queues a generic job that returns immediately and waits for it
// to reach `completed`, so the state-conflict assertions do not race the worker.
func finishedTestJob(t *testing.T, tc *TestContext) string {
	t.Helper()
	job, err := tc.AppCtx.DownloadManager().SubmitJob("test", "done",
		func(ctx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
			return nil
		})
	if err != nil {
		t.Fatalf("submit job: %v", err)
	}
	waitForJobStatus(t, tc, job.ID, download_queue.JobStatusCompleted)
	return job.ID
}

func waitForJobStatus(t *testing.T, tc *TestContext, jobID string, want download_queue.JobStatus) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last download_queue.JobStatus
	for time.Now().Before(deadline) {
		job, ok := tc.AppCtx.DownloadManager().GetJob(jobID)
		if ok {
			last = job.GetStatus()
			if last == want {
				return
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("job %s never reached %q (last seen %q)", jobID, want, last)
}

// ---------------------------------------------------------------- finding 2

func TestCancelPausedJob_IsAccepted(t *testing.T) {
	tc := SetupTestEnv(t)
	jobID := pausedDownloadJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id="+jobID, nil)
	if res.Code != http.StatusOK {
		t.Fatalf("finding 2: cancelling a paused job answered %d %s, want 200", res.Code, res.Body.String())
	}

	job, ok := tc.AppCtx.DownloadManager().GetJob(jobID)
	if !ok {
		t.Fatalf("job disappeared")
	}
	if got := job.GetStatus(); got != download_queue.JobStatusCancelled {
		t.Errorf("finding 2: the paused job is %q after cancel, want cancelled", got)
	}
}

func TestCancelBlockedJobRetriesSharedCacheTableLock(t *testing.T) {
	tc := setupTestEnvOn(t, openSharedCacheTestDatabase(t), nil)
	deps := jobs.Deps{DB: tc.DB}
	service := tc.AppCtx.JobService()
	accepted, err := service.Accept(deps, jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", Title: "held download",
		Replay: jobs.ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("accept download Job: %v", err)
	}
	blocked, err := service.Transition(deps, jobs.Transition{
		JobID: accepted.ID, ExpectedVersion: accepted.Version, To: jobs.StateBlocked, Phase: "paused",
	})
	if err != nil {
		t.Fatalf("block download Job: %v", err)
	}
	if blocked.State != jobs.StateBlocked {
		t.Fatalf("seeded download Job state = %q, want blocked", blocked.State)
	}
	var stored models.Job
	if err := tc.DB.Where("id = ?", blocked.ID).First(&stored).Error; err != nil {
		t.Fatalf("read seeded download Job: %v", err)
	}
	if stored.ExecutionToken != "" {
		t.Fatalf("blocked download Job still owns execution token %q", stored.ExecutionToken)
	}
	var claims int64
	if err := tc.DB.Model(&models.JobClaim{}).Where("job_id = ?", blocked.ID).Count(&claims).Error; err != nil {
		t.Fatalf("count seeded download claims: %v", err)
	}
	if claims != 0 {
		t.Fatalf("blocked download Job has %d claim rows, want none", claims)
	}

	// Shared-cache in-memory SQLite raises SQLITE_LOCKED table errors that
	// bypass busy_timeout. Hold a read transaction on the Job table until
	// the public cancel route reaches its first locked UPDATE, then release it so
	// the command's outer transaction can retry from a fresh snapshot.
	reader := tc.DB.Begin()
	if reader.Error != nil {
		t.Fatalf("begin jobs read transaction: %v", reader.Error)
	}
	var locked struct{ ID string }
	if err := reader.Table("jobs").Select("id").Where("id = ?", blocked.ID).Take(&locked).Error; err != nil {
		_ = reader.Rollback().Error
		t.Fatalf("hold read lock on job: %v", err)
	}
	probeErr := tc.DB.Exec("UPDATE jobs SET version = version WHERE id = ?", blocked.ID).Error
	if probeErr == nil || !strings.Contains(strings.ToLower(probeErr.Error()), "database table is locked") {
		_ = reader.Rollback().Error
		t.Fatalf("shared-cache lock probe = %v, want SQLITE_LOCKED on jobs", probeErr)
	}

	lockedAttempt := make(chan struct{}, 1)
	continueCommand := make(chan struct{})
	const callbackName = "test:observe-cancel-blocked-job-sqlite-lock"
	if err := tc.DB.Callback().Update().After("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table != "jobs" || tx.Error == nil {
			return
		}
		var sqliteErr sqlite3.Error
		if !errors.As(tx.Error, &sqliteErr) || sqliteErr.Code != sqlite3.ErrLocked {
			return
		}
		for _, value := range tx.Statement.Vars {
			jobID, ok := value.(string)
			if ok && jobID == blocked.ID {
				lockedAttempt <- struct{}{}
				<-continueCommand
				return
			}
		}
	}); err != nil {
		_ = reader.Rollback().Error
		t.Fatalf("observe blocked Job lock attempt: %v", err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Update().Remove(callbackName) })

	requestDone := make(chan *httptest.ResponseRecorder, 1)
	awaitRequest := func() (*httptest.ResponseRecorder, bool) {
		select {
		case res := <-requestDone:
			return res, true
		case <-time.After(5 * time.Second):
			return nil, false
		}
	}
	go func() {
		requestDone <- tc.MakeRequest(http.MethodPost, "/v1/jobs/"+blocked.ID+"/commands/cancel", map[string]any{
			"expectedVersion": blocked.Version,
			"idempotencyKey":  "sqlite-shared-lock-regression",
		})
	}()
	select {
	case <-lockedAttempt:
		// The callback pauses the route after its first real SQLITE_LOCKED jobs
		// UPDATE. The reader stays open until this point even on a slow scheduler.
	case <-time.After(5 * time.Second):
		close(continueCommand)
		_ = reader.Rollback().Error
		res, ok := awaitRequest()
		if !ok {
			t.Fatal("cancel route did not finish after releasing the jobs read lock")
		}
		t.Fatalf("cancel route answered %d without an observed SQLITE_LOCKED jobs UPDATE: %s", res.Code, res.Body.String())
	}
	if err := reader.Commit().Error; err != nil {
		close(continueCommand)
		_ = reader.Rollback().Error
		if _, ok := awaitRequest(); !ok {
			t.Fatalf("cancel route did not finish after failed read-lock release: %v", err)
		}
		t.Fatalf("release jobs read lock: %v", err)
	}
	close(continueCommand)
	res, ok := awaitRequest()
	if !ok {
		t.Fatal("cancel route did not finish after releasing the jobs read lock")
	}
	if res.Code != http.StatusOK {
		t.Fatalf("cancelling a blocked download under shared-cache contention answered %d %s, want 200", res.Code, res.Body.String())
	}

	current, err := service.Get(deps, jobs.Access{Administrator: true}, blocked.ID)
	if err != nil {
		t.Fatalf("read cancelled Job: %v", err)
	}
	if current.State != jobs.StateCancelled {
		t.Errorf("the blocked Job is %q after cancel under contention, want cancelled", current.State)
	}
}

func TestCancelFinishedJob_Is409NotA404(t *testing.T) {
	tc := SetupTestEnv(t)
	jobID := finishedTestJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id="+jobID, nil)
	if res.Code != http.StatusConflict {
		t.Errorf("finding 2: cancelling a completed job answered %d, want 409 Conflict — a 404 says the job does not exist, which is a different problem for the caller to debug. body: %s",
			res.Code, res.Body.String())
	}
	if !strings.Contains(res.Body.String(), "cannot be cancelled") {
		t.Errorf("the 409 body should say what it refused, got %s", res.Body.String())
	}
}

// The control for the test above: a genuinely unknown id is still a 404, so the
// change did not simply replace one blanket status with another.
func TestCancelUnknownJob_IsStill404(t *testing.T) {
	tc := SetupTestEnv(t)
	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/cancel?id=deadbeef", nil)
	if res.Code != http.StatusNotFound {
		t.Errorf("cancelling an unknown job answered %d, want 404. body: %s", res.Code, res.Body.String())
	}
}

func TestJobStateConflicts_AreAll409(t *testing.T) {
	// pause/resume/retry ran through statusCodeForError with a 400 fallback, and
	// its "cannot be" validation pattern claimed them — so a state conflict was a
	// Bad Request. They share the typed error with cancel now.
	for _, action := range []string{"pause", "resume", "retry"} {
		t.Run(action, func(t *testing.T) {
			tc := SetupTestEnv(t)
			// A completed job can be none of the three: pause and resume are for
			// jobs still in flight, and retry only accepts failed/cancelled.
			jobID := finishedTestJob(t, tc)

			res := tc.MakeRequest(http.MethodPost, "/v1/jobs/"+action+"?id="+jobID, nil)
			if res.Code != http.StatusConflict {
				t.Errorf("%s in the wrong state answered %d, want 409. body: %s", action, res.Code, res.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------- finding 40

func TestClearCompletedJobs_RemovesFinishedAndKeepsActive(t *testing.T) {
	tc := SetupTestEnv(t)
	dm := tc.AppCtx.DownloadManager()

	finishedID := finishedTestJob(t, tc)
	pausedID := pausedDownloadJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/clearCompleted", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("finding 40: POST /v1/jobs/clearCompleted answered %d %s", res.Code, res.Body.String())
	}
	var payload struct {
		Cleared int `json:"cleared"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("clearCompleted response is not JSON: %v (%s)", err, res.Body.String())
	}
	if payload.Cleared < 1 {
		t.Errorf("finding 40: clearCompleted reported %d cleared, want at least the one completed job", payload.Cleared)
	}

	if _, ok := dm.GetJob(finishedID); ok {
		t.Errorf("finding 40: the completed job survived clearCompleted")
	}
	// Positive control in the same test: an unfinished job must not be swept away
	// with the rest. Clearing a paused job would discard a half-downloaded file.
	if _, ok := dm.GetJob(pausedID); !ok {
		t.Errorf("finding 40: clearCompleted removed a paused job")
	}
}

// TestClearCompletedJobs_NamesWhatItCleared is review remediation finding 2.
//
// The panel used to decide what to dismiss from its *own* snapshot of the queue,
// taken before the request went out. The server decides at handling time, so a job
// that reached a terminal state inside that window was removed server-side, was
// absent from the client's dismissed set, and the `removed` handler — whose job is
// to retain finished jobs for display — put it straight back as a phantom row that
// survived until the next reconnect.
//
// The client cannot close that window on its own; only the server knows what it
// actually cleared. So the response names the ids.
func TestClearCompletedJobs_NamesWhatItCleared(t *testing.T) {
	tc := SetupTestEnv(t)

	finishedID := finishedTestJob(t, tc)
	pausedID := pausedDownloadJob(t, tc)

	res := tc.MakeRequest(http.MethodPost, "/v1/jobs/clearCompleted", nil)
	if res.Code != http.StatusOK {
		t.Fatalf("POST /v1/jobs/clearCompleted answered %d %s", res.Code, res.Body.String())
	}
	var payload struct {
		Cleared int      `json:"cleared"`
		IDs     []string `json:"ids"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("clearCompleted response is not JSON: %v (%s)", err, res.Body.String())
	}

	if len(payload.IDs) != payload.Cleared {
		t.Errorf("clearCompleted reported %d cleared but named %d ids (%v) — the client dismisses what this list says",
			payload.Cleared, len(payload.IDs), payload.IDs)
	}
	if !slices.Contains(payload.IDs, finishedID) {
		t.Errorf("review finding 2: the cleared job %s is not in the response's ids %v, so the panel cannot know it went",
			finishedID, payload.IDs)
	}
	// The positive control: the list is what was cleared, not everything the caller
	// can see. A paused job stays, so naming it would make the panel drop a row the
	// server still has.
	if slices.Contains(payload.IDs, pausedID) {
		t.Errorf("the paused job %s was named as cleared, but it was kept: %v", pausedID, payload.IDs)
	}
}

// between returns the substring from the first occurrence of from to the first
// following occurrence of to.
func between(body, from, to string) string {
	start := strings.Index(body, from)
	if start < 0 {
		return ""
	}
	end := strings.Index(body[start:], to)
	if end < 0 {
		return ""
	}
	return body[start : start+end]
}

// TestHeaderDialogs_StackAboveHeaderDropdowns covers the remaining dialog that
// lives in the header. The Job Center panel also appears in the header source, but
// its dialog is teleported into .overlays; strip that template before finding the
// header boundary because its dialog contains its own <header> element.
func TestHeaderDialogs_StackAboveHeaderDropdowns(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	header := between(stripTeleportedTemplates(body), "<header", "</header>")
	if header == "" {
		t.Fatalf("no rendered <header> in the page — this test measured nothing")
	}
	if strings.Contains(header, `data-testid="job-panel-overlay"`) {
		t.Fatalf("the Job Center panel overlay is still in the header after stripping teleported templates")
	}

	zClass := regexp.MustCompile(`(?:^|\s)z-(?:\[(\d+)\]|(\d+))(?:\s|"|$)`)
	dialogs := map[string]int{}
	others := map[string]int{}
	for _, tag := range openTagsWithin(header, "div") {
		m := zClass.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		raw := m[1]
		if raw == "" {
			raw = m[2]
		}
		z, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		if strings.Contains(tag, "fixed inset-0") {
			dialogs[tag] = z
		} else {
			others[tag] = z
		}
	}
	if len(dialogs) == 0 {
		t.Fatalf("expected the global search dialog overlay in the header, found no full-viewport overlays")
	}
	if len(others) == 0 {
		t.Fatalf("expected a settings or account dropdown to carry a z-index class — this test measured nothing")
	}
	for tag, z := range dialogs {
		for otherTag, otherZ := range others {
			if z <= otherZ {
				t.Errorf("a header dialog at z-index %d does not outrank header chrome at z-index %d, so the chrome paints over an aria-modal dialog.\n dialog: %s\n chrome: %s", z, otherZ, tag, otherTag)
			}
		}
	}
}

// stripTeleportedTemplates removes every `<template x-teleport …>…</template>` block,
// nesting included, from a fragment of served HTML. Teleported content is authored
// under the header but renders in another layer, so markup placement checks must skip it.
func stripTeleportedTemplates(markup string) string {
	const openTag, closeTag = "<template", "</template>"
	for {
		start := strings.Index(markup, "<template x-teleport")
		if start < 0 {
			return markup
		}
		depth, i, end := 0, start, -1
		for i < len(markup) {
			nextOpen := strings.Index(markup[i:], openTag)
			nextClose := strings.Index(markup[i:], closeTag)
			if nextClose < 0 {
				return markup[:start]
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				depth++
				i += nextOpen + len(openTag)
				continue
			}
			depth--
			i += nextClose + len(closeTag)
			if depth == 0 {
				end = i
				break
			}
		}
		if end < 0 {
			return markup[:start]
		}
		markup = markup[:start] + markup[end:]
	}
}

// TestJobCenterPanel_IsTeleportedIntoTheOverlaysLayer pins the placement contract
// for the Job Center dialog. x-if must be outside x-teleport so Alpine keeps the
// dialog under the panel component's data root; the panel layer must also stay below
// the other .overlays dialogs because a teleport appends its node last.
func TestJobCenterPanel_IsTeleportedIntoTheOverlaysLayer(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	rootIdx := strings.Index(body, `data-testid="job-panel-root"`)
	if rootIdx < 0 {
		t.Fatalf("the served page is missing the Job Center panel root")
	}
	panelSource := body[rootIdx:]
	ifIdx := strings.Index(panelSource, `<template x-if="isOpen">`)
	teleIdx := strings.Index(panelSource, `<template x-teleport=".overlays">`)
	overlayIdx := strings.Index(panelSource, `data-testid="job-panel-overlay"`)
	panelIdx := strings.Index(panelSource, `id="job-center-panel"`)
	if ifIdx < 0 || teleIdx < 0 || overlayIdx < 0 || panelIdx < 0 {
		t.Fatalf("the served page is missing Job Center panel markup (if=%d teleport=%d overlay=%d panel=%d)", ifIdx, teleIdx, overlayIdx, panelIdx)
	}
	if !(ifIdx < teleIdx && teleIdx < overlayIdx && overlayIdx < panelIdx) {
		t.Errorf("the Job Center panel must have x-if outside x-teleport before its overlay and dialog (if=%d teleport=%d overlay=%d panel=%d)", ifIdx, teleIdx, overlayIdx, panelIdx)
	}
	panel := findOpenTag(body, `id="job-center-panel"`, "section")
	if !strings.Contains(panel, `aria-modal="true"`) || !strings.Contains(panel, "x-trap.noscroll.noreturn") {
		t.Errorf("the Job Center dialog must retain its modal semantics and focus trap:\n%s", whitespaceRe.ReplaceAllString(panel, " "))
	}

	for _, tag := range openTagsWithin(body, "template") {
		if strings.Contains(tag, "x-teleport") && strings.Contains(tag, "x-if") {
			t.Errorf("x-if and x-teleport share a <template>, which can leave a second permanent Job Center overlay:\n%s", tag)
		}
	}

	overlay := findOpenTag(body, `data-testid="job-panel-overlay"`, "div")
	m := regexp.MustCompile(`(?:^|\s)z-(?:\[(\d+)\]|(\d+))(?:\s|"|$)`).FindStringSubmatch(overlay)
	if m == nil {
		t.Fatalf("the Job Center overlay carries no z-index class — this test measured nothing:\n%s", overlay)
	}
	raw := m[1]
	if raw == "" {
		raw = m[2]
	}
	z, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("unreadable z-index %q on the Job Center overlay", raw)
	}
	if z >= 50 {
		t.Errorf("the teleported Job Center overlay is at z-index %d, which is not below the lowest .overlays sibling (50)\n%s", z, overlay)
	}
}

// The Job Center panel declines to open while a true modal is up, and it recognises one by
// sweeping the document for `[aria-modal="true"]` (src/utils/modality.js). That is a
// contract with the markup, and the markup is where it can silently stop being true:
// an overlay added without the attribute would be a dialog the panel happily opens
// underneath, with focus trapped in the panel nobody can see.
//
// This counts the overlays in `.overlays` because that is the layer where a new one
// is most likely to be added. The browser half is in the Job Center browser suite, which
// CI does run (the e2e-browser job covers tests/regressions/); this is the cheaper
// source-level guard that fails in seconds rather than minutes.
func TestOverlayModals_AreAllReachableByTheJobPanelGuard(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	overlays := between(body, `<div class="overlays">`, "</body>")
	if overlays == "" {
		t.Fatalf("no .overlays layer in the page — this test measured nothing")
	}

	dialogs := 0
	for _, tag := range openTagsWithin(overlays, "div") {
		if !strings.Contains(tag, `role="dialog"`) {
			continue
		}
		dialogs++
		if !strings.Contains(tag, `aria-modal="true"`) {
			t.Errorf("an overlay dialog carries no aria-modal, so jobPanel.blockingModal() cannot see it and the jobs panel will open underneath it:\n %s", tag)
		}
	}
	// The lightbox, paste-upload, the plugin action modal and the entity picker. A
	// count of zero would satisfy every assertion above without measuring anything.
	if dialogs < 4 {
		t.Errorf("found %d dialogs in the overlays layer, expected at least 4 — either the layer changed or this test stopped finding it", dialogs)
	}
}

// The premise the guard rests on: the overlays layer really does paint above the
// whole header, so raising the panel's own z-index could never have fixed this. The
// header is a stacking context, so its descendants are ordered inside its layer no
// matter what number they carry.
func TestOverlaysLayer_OutranksTheEntireHeaderLayer(t *testing.T) {
	css, err := os.ReadFile("../../public/index.css")
	if err != nil {
		t.Fatalf("read index.css: %v", err)
	}

	zOf := func(selector string) int {
		t.Helper()
		rule := between(string(css), selector+" {", "}")
		if rule == "" {
			t.Fatalf("no %s rule in public/index.css — this test measured nothing", selector)
		}
		m := regexp.MustCompile(`z-index:\s*(\d+)`).FindStringSubmatch(rule)
		if m == nil {
			t.Fatalf("%s carries no z-index — this test measured nothing", selector)
		}
		n, convErr := strconv.Atoi(m[1])
		if convErr != nil {
			t.Fatalf("%s z-index %q is not a number", selector, m[1])
		}
		return n
	}

	overlays, header := zOf(".overlays"), zOf(".header")
	if overlays <= header {
		t.Errorf("the overlays layer is z-index %d and the header layer is %d, so a true modal no longer paints above the Job Center panel", overlays, header)
	}
}

// The plugin action modal returns focus itself, so its trap must not also try.
//
// x-trap restores to whatever had focus when it armed. For an action started from a
// card menu that is a menu item the menu has since hidden — connected, display:none,
// unfocusable — so the reader was dropped on <body>; and on the jobs-panel hand-off
// the trap moved focus through that stale control on its way out, which a screen
// reader announces. `close()` owns the return now, and every way the dialog closes
// goes through it, so the trap must be `.noreturn` or the two fight.
//
// A markup assertion because the whole contract is one modifier that is easy to drop
// in a refactor, and a source sweep says so precisely where a behavioural test would
// only say "focus went somewhere odd".
func TestPluginActionModal_LeavesTheFocusReturnToItsComponent(t *testing.T) {
	tc := SetupTestEnv(t)
	_, body := tc.getHTML(t, "/dashboard")

	overlays := between(body, `<div class="overlays">`, "</body>")
	if overlays == "" {
		t.Fatalf("no .overlays layer in the page — this test measured nothing")
	}

	var modal string
	for _, tag := range openTagsWithin(overlays, "div") {
		if strings.Contains(tag, "plugin-action-modal") && strings.Contains(tag, `role="dialog"`) {
			modal = tag
			break
		}
	}
	if modal == "" {
		t.Fatalf("no plugin action modal dialog in the overlays layer — this test measured nothing")
	}
	if !strings.Contains(modal, "x-trap") {
		t.Fatalf("the plugin action modal has no focus trap at all:\n %s", modal)
	}
	if !strings.Contains(modal, "x-trap.noreturn") {
		t.Errorf("the plugin action modal's trap restores focus as well as its own close(), and the two disagree: the trap returns to the control that opened the modal, which for a card action is a menu item the menu has since hidden.\n %s", modal)
	}
}
