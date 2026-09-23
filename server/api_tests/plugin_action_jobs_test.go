package api_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
)

// This file pins the per-entity acceptance contract of the async action route.
//
// A bulk submission is not one unit of work: every entity was validated on its
// own above, and each becomes its own Job. So one refusal must not discard the
// others, and the answer has to say exactly which ids were accepted and why the
// rest were not — the previous behaviour reported only the first failure, which
// made a partial batch indistinguishable from a total one.
//
// The runner here is a double at the *application* seam: it stands in for a
// context that owns a Job control plane, so the loop and the response shape are
// what is under test rather than SQLite. The real durable path — acceptance,
// claims, dispatch and outcomes — is driven end to end in application_context.

// acceptingActionRunner is a PluginActionRunner that also owns the durable
// acceptance seam, refusing the entities it was told to refuse.
type acceptingActionRunner struct {
	*testPluginRunner
	refuse map[uint]bool
}

// RunPluginActionAsync implements the handler's optional durable seam.
func (r *acceptingActionRunner) RunPluginActionAsync(owner *uint, pluginName, actionID string, entityID uint, params map[string]any, expectFilters string) (string, string, error) {
	if r.refuse[entityID] {
		return "", "", fmt.Errorf("entity %d could not be accepted", entityID)
	}
	return fmt.Sprintf("handle-%d", entityID), fmt.Sprintf("job-%d", entityID), nil
}

// asyncActionRunnerForTest builds a runner whose plugin declares one async action
// with no filters, so nothing but the acceptance loop decides the outcome.
func asyncActionRunnerForTest(t *testing.T, tc *TestContext) *acceptingActionRunner {
	t.Helper()
	pluginDir := t.TempDir()
	pm := enableTestPluginWithActions(t, pluginDir)
	return &acceptingActionRunner{
		testPluginRunner: &testPluginRunner{
			pm:         pm,
			reader:     tc.AppCtx.ActionEntityRefReader(),
			dataReader: tc.AppCtx.ActionEntityDataReader(),
		},
		refuse: map[uint]bool{},
	}
}

func TestActionRun_BulkReportsPerEntityAcceptance(t *testing.T) {
	tc := SetupTestEnv(t)
	runner := asyncActionRunnerForTest(t, tc)
	runner.refuse[3] = true

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := `{"plugin":"actions-plugin","action":"work","entity_ids":[1,2,3,4],"params":{}}`
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202 for a partially accepted batch, got %d body=%s", rr.Code, rr.Body.String())
	}

	var response struct {
		JobIDs []string `json:"job_ids"`
		Jobs   []struct {
			EntityID       uint   `json:"entity_id"`
			JobID          string `json:"job_id"`
			CanonicalJobID string `json:"canonical_job_id"`
		} `json:"jobs"`
		Failures []struct {
			EntityID uint   `json:"entity_id"`
			Error    string `json:"error"`
		} `json:"failures"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("the response is not JSON: %v (%s)", err, rr.Body.String())
	}

	if len(response.Jobs) != 3 {
		t.Fatalf("expected the three accepted entities to be reported, got %+v", response.Jobs)
	}
	if len(response.Failures) != 1 || response.Failures[0].EntityID != 3 {
		t.Fatalf("expected one failure naming entity 3, got %+v", response.Failures)
	}
	if len(response.JobIDs) != 3 {
		t.Fatalf("expected three job ids, got %+v", response.JobIDs)
	}
	// The fourth entity was accepted *after* the third failed, which is the
	// property that makes this per-entity rather than first-failure-wins.
	fourth := false
	for _, entry := range response.Jobs {
		if entry.EntityID == 4 {
			fourth = true
		}
		if entry.JobID == "" || entry.CanonicalJobID == "" {
			t.Fatalf("an accepted entity was reported without both ids: %+v", entry)
		}
	}
	if !fourth {
		t.Fatalf("the batch stopped at the refused entity: %+v", response.Jobs)
	}
}

func TestActionRun_AllRefusalsAreAServerError(t *testing.T) {
	tc := SetupTestEnv(t)
	runner := asyncActionRunnerForTest(t, tc)
	runner.refuse[1] = true
	runner.refuse[2] = true

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/jobs/action/run", api_handlers.GetActionRunHandler(runner))

	body := `{"plugin":"actions-plugin","action":"work","entity_ids":[1,2],"params":{}}`
	req, _ := http.NewRequest("POST", "/v1/jobs/action/run", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when nothing was accepted, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// enableTestPluginWithActions writes and enables a plugin declaring one async
// action and nothing else.
func enableTestPluginWithActions(t *testing.T, dir string) *plugin_system.PluginManager {
	t.Helper()
	pluginDir := filepath.Join(dir, "actions-plugin")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir the plugin: %v", err)
	}
	source := `
plugin = { name = "actions-plugin", version = "1.0", api_version = 1,
           capabilities = { "actions" } }

function work(ctx)
    mah.job_complete(ctx.job_id, { message = "done" })
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
end
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(source), 0o644); err != nil {
		t.Fatalf("write the plugin: %v", err)
	}
	pm, err := plugin_system.NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("actions-plugin"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	return pm
}

func setupRetryableActionProjectionEnv(t *testing.T) (*TestContext, *models.User, *models.User, string) {
	t.Helper()
	pluginRoot := t.TempDir()
	pluginDir := filepath.Join(pluginRoot, "retry-projection")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatalf("mkdir the plugin: %v", err)
	}
	source := `
plugin = { name = "retry-projection", version = "1.0", api_version = 1,
           capabilities = { "actions", "jobs" } }

function fail_work(ctx)
    mah.job_fail(ctx.job_id, "the action refused")
end

function init()
    mah.action({ id = "retryable", label = "Retryable", entity = "resource", async = true,
                 retry = true, handler = fail_work })
end
`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.lua"), []byte(source), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}
	tc := setupTestEnvWithConfig(t, func(config *application_context.MahresourcesConfig) {
		config.AuthEnabled = true
		config.SessionTTL = time.Hour
		config.PluginPath = pluginRoot
	})
	pm := tc.AppCtx.PluginManager()
	if pm == nil {
		t.Fatal("the app context has no plugin manager")
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("retry-projection"); err != nil {
		t.Fatalf("enable plugin: %v", err)
	}
	owner, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "action-owner", Password: "correct-horse-battery", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create action owner: %v", err)
	}
	admin, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "action-admin", Password: "correct-horse-battery", Role: models.RoleAdmin,
	})
	if err != nil {
		t.Fatalf("create action admin: %v", err)
	}
	resource := &models.Resource{Name: "action-target", ResourceCategoryId: 1}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatalf("create action target: %v", err)
	}
	ownerToken, _, err := tc.AppCtx.CreateApiToken(owner.ID, "action owner", nil)
	if err != nil {
		t.Fatalf("create owner token: %v", err)
	}
	return tc, owner, admin, ownerToken
}

func runRetryableActionToFailure(t *testing.T, tc *TestContext, owner *models.User, key string) (string, jobs.Snapshot) {
	t.Helper()
	ownerCtx := tc.AppCtx.WithPrincipal(auth.FromUser(owner))
	handle, canonicalID, err := ownerCtx.RunPluginActionAsync(&owner.ID, "retry-projection", "retryable", 1, nil, "")
	if err != nil {
		t.Fatalf("run retryable action: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		job, err := ownerCtx.GetJob(canonicalID)
		if err == nil && job.State.Terminal() {
			if job.State != jobs.StateFailed {
				t.Fatalf("retryable action ended %s, want failed", job.State)
			}
			return handle, job
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("retryable action %s did not fail", key)
	return "", jobs.Snapshot{}
}

func retryCanonicalAction(t *testing.T, ctx *application_context.MahresourcesContext, job jobs.Snapshot, key string) jobs.CommandResult {
	t.Helper()
	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: job.ID, Key: jobs.CommandRetry, IdempotencyKey: key, ExpectedVersion: job.Version,
	})
	if err != nil {
		t.Fatalf("retry canonical action %s: %v", job.ID, err)
	}
	if result.Status != jobs.CommandStatusSucceeded || result.SuccessorID == "" {
		t.Fatalf("canonical retry answered %s/%s with successor %q", result.Status, result.Code, result.SuccessorID)
	}
	return result
}

func TestLegacyActionJobGetResolvesCurrentRetryLeaf(t *testing.T) {
	tc, owner, admin, ownerToken := setupRetryableActionProjectionEnv(t)
	ownerCtx := tc.AppCtx.WithPrincipal(auth.FromUser(owner))
	adminCtx := tc.AppCtx.WithPrincipal(auth.FromUser(admin))
	pm := tc.AppCtx.PluginManager()

	// A retry accepted while every plugin execution slot is occupied stays queued;
	// the old manager entry remains in memory and keeps the same legacy handle.
	handle, original := runRetryableActionToFailure(t, tc, owner, "visible")
	release := pm.FillJobBudgetForTest()
	visible := retryCanonicalAction(t, ownerCtx, original, "visible-successor")
	queued, err := tc.AppCtx.GetJob(visible.SuccessorID)
	release()
	if err != nil || queued.State != jobs.StateQueued {
		t.Fatalf("same-owner successor state = %s, err=%v; want queued before dispatch", queued.State, err)
	}

	visibleResponse := doReq(tc, http.MethodGet, "/v1/jobs/action/job?id="+handle,
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + ownerToken}, nil, nil)
	if visibleResponse.Code != http.StatusOK {
		t.Fatalf("owner GET current action handle: status %d body %s", visibleResponse.Code, visibleResponse.Body.String())
	}
	var visibleJob plugin_system.ActionJob
	if err := json.Unmarshal(visibleResponse.Body.Bytes(), &visibleJob); err != nil {
		t.Fatalf("decode visible action job: %v (%s)", err, visibleResponse.Body.String())
	}
	if visibleJob.ID != handle || visibleJob.CanonicalJobID != visible.SuccessorID || visibleJob.Status != "pending" {
		t.Fatalf("legacy handle projected %+v; want handle %q, queued successor %q, status pending", visibleJob, handle, visible.SuccessorID)
	}

	// An administrator's canonical Retry changes the current target's owner. The
	// old process-local row belongs to the requester, but that ancestor no longer
	// answers the handle and must not become a visibility fallback.
	hiddenHandle, hiddenOriginal := runRetryableActionToFailure(t, tc, owner, "hidden")
	release = pm.FillJobBudgetForTest()
	hidden := retryCanonicalAction(t, adminCtx, hiddenOriginal, "hidden-successor")
	hiddenQueued, err := tc.AppCtx.GetJob(hidden.SuccessorID)
	release()
	if err != nil || hiddenQueued.State != jobs.StateQueued {
		t.Fatalf("admin successor state = %s, err=%v; want queued before dispatch", hiddenQueued.State, err)
	}

	hiddenResponse := doReq(tc, http.MethodGet, "/v1/jobs/action/job?id="+hiddenHandle,
		map[string]string{"Accept": "application/json", "Authorization": "Bearer " + ownerToken}, nil, nil)
	if hiddenResponse.Code != http.StatusNotFound {
		t.Fatalf("owner GET hidden current successor: status %d body %s, want 404", hiddenResponse.Code, hiddenResponse.Body.String())
	}
}

type actionContextWithoutPluginManager struct {
	*application_context.MahresourcesContext
}

func (actionContextWithoutPluginManager) PluginManager() *plugin_system.PluginManager { return nil }

func TestLegacyActionJobGetUsesDurableHandleWithoutPluginManager(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	handle, _ := runRetryableActionToFailure(t, tc, owner, "no plugin manager")
	ctx := actionContextWithoutPluginManager{MahresourcesContext: tc.AppCtx.WithPrincipal(auth.FromUser(owner))}
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/action/job?id="+handle, nil)
	request = request.WithContext(auth.WithPrincipal(request.Context(), auth.FromUser(owner)))
	response := httptest.NewRecorder()
	api_handlers.GetActionJobHandler(ctx)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("durable action handle without manager: status %d body %s, want 200", response.Code, response.Body.String())
	}
	var job plugin_system.ActionJob
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatalf("decode durable action job: %v (%s)", err, response.Body.String())
	}
	if job.ID != handle || job.Status != "failed" {
		t.Fatalf("durable action response %+v; want handle %q with failed status", job, handle)
	}
}

type pluginActionEventsWriter struct {
	header         http.Header
	mu             sync.Mutex
	body           bytes.Buffer
	initWritten    chan struct{}
	initialFlushed chan struct{}
	actionWritten  chan struct{}
	releaseInitial chan struct{}
	gateInitial    bool
	initOnce       sync.Once
	flushOnce      sync.Once
	actionOnce     sync.Once
}

func newPluginActionEventsWriter(gateInitial bool) *pluginActionEventsWriter {
	return &pluginActionEventsWriter{
		header:         make(http.Header),
		initWritten:    make(chan struct{}),
		initialFlushed: make(chan struct{}),
		actionWritten:  make(chan struct{}),
		releaseInitial: make(chan struct{}),
		gateInitial:    gateInitial,
	}
}

func (w *pluginActionEventsWriter) Header() http.Header { return w.header }
func (*pluginActionEventsWriter) WriteHeader(int)       {}
func (w *pluginActionEventsWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	_, _ = w.body.Write(data)
	w.mu.Unlock()
	if bytes.Contains(data, []byte("event: init")) {
		w.initOnce.Do(func() { close(w.initWritten) })
	}
	if bytes.Contains(data, []byte("event: action_")) {
		w.actionOnce.Do(func() { close(w.actionWritten) })
	}
	return len(data), nil
}
func (w *pluginActionEventsWriter) Flush() {
	w.flushOnce.Do(func() {
		close(w.initialFlushed)
		if w.gateInitial {
			<-w.releaseInitial
		}
	})
}
func (w *pluginActionEventsWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func startPluginActionEventsRequest(t *testing.T, tc *TestContext, token string, gateInitial bool) (*pluginActionEventsWriter, context.CancelFunc, <-chan struct{}) {
	t.Helper()
	streamCtx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs/events", nil).WithContext(streamCtx)
	request.Header.Set("Authorization", "Bearer "+token)
	writer := newPluginActionEventsWriter(gateInitial)
	done := make(chan struct{})
	go func() {
		tc.Router.ServeHTTP(writer, request)
		close(done)
	}()
	select {
	case <-writer.initWritten:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("legacy action SSE did not write its init event")
	}
	select {
	case <-writer.initialFlushed:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("legacy action SSE did not flush its init event")
	}
	return writer, cancel, done
}

func stopPluginActionEventsRequest(t *testing.T, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("legacy action SSE did not stop after cancellation")
	}
}

func TestLegacyActionEventsInitProjectsQueuedRetryLeaf(t *testing.T) {
	tc, owner, _, ownerToken := setupRetryableActionProjectionEnv(t)
	ownerCtx := tc.AppCtx.WithPrincipal(auth.FromUser(owner))
	handle, original := runRetryableActionToFailure(t, tc, owner, "SSE init")
	release := tc.AppCtx.PluginManager().FillJobBudgetForTest()
	retry := retryCanonicalAction(t, ownerCtx, original, "sse-init-successor")
	queued, err := tc.AppCtx.GetJob(retry.SuccessorID)
	release()
	if err != nil || queued.State != jobs.StateQueued {
		t.Fatalf("SSE successor state = %s, err=%v; want queued", queued.State, err)
	}

	writer, cancel, done := startPluginActionEventsRequest(t, tc, ownerToken, false)
	stopPluginActionEventsRequest(t, cancel, done)
	body := writer.String()
	frameEnd := strings.Index(body, "\n\n")
	if frameEnd < 0 {
		t.Fatalf("SSE response has no complete init frame: %q", body)
	}
	frame := body[:frameEnd]
	dataAt := strings.Index(frame, "data: ")
	if dataAt < 0 {
		t.Fatalf("SSE init frame has no data field: %q", frame)
	}
	var init struct {
		ActionJobs []plugin_system.ActionJob `json:"actionJobs"`
	}
	if err := json.Unmarshal([]byte(frame[dataAt+len("data: "):]), &init); err != nil {
		t.Fatalf("decode SSE init: %v (%s)", err, frame)
	}
	if len(init.ActionJobs) != 1 {
		t.Fatalf("SSE init has %d action jobs, want the one durable queued successor: %+v", len(init.ActionJobs), init.ActionJobs)
	}
	job := init.ActionJobs[0]
	if job.ID != handle || job.CanonicalJobID != retry.SuccessorID || job.Status != "pending" {
		t.Fatalf("SSE init projected %+v; want handle %q, queued successor %q, pending", job, handle, retry.SuccessorID)
	}
}

func TestLegacyActionEventsNotifyConnectedClientWhenRetryMovesHandle(t *testing.T) {
	tc, owner, _, ownerToken := setupRetryableActionProjectionEnv(t)
	ownerCtx := tc.AppCtx.WithPrincipal(auth.FromUser(owner))
	writer, cancel, done := startPluginActionEventsRequest(t, tc, ownerToken, false)
	defer stopPluginActionEventsRequest(t, cancel, done)

	handle, original := runRetryableActionToFailure(t, tc, owner, "live retry move")
	// Drain any process-local events from the original execution before the
	// retry. The stream remains open with the old handle in its initial snapshot.
	time.Sleep(50 * time.Millisecond)
	initialBody := writer.String()
	if !strings.Contains(initialBody, `"canonicalJobId":"`+original.ID+`"`) {
		t.Fatalf("initial live stream did not include failed action %q: %s", original.ID, initialBody)
	}

	release := tc.AppCtx.PluginManager().FillJobBudgetForTest()
	budgetHeld := true
	defer func() {
		if budgetHeld {
			release()
		}
	}()
	retry := retryCanonicalAction(t, ownerCtx, original, "sse-live-retry-successor")
	queued, err := tc.AppCtx.GetJob(retry.SuccessorID)
	if err != nil || queued.State != jobs.StateQueued {
		t.Fatalf("retry successor state = %s, err=%v; want queued before dispatch", queued.State, err)
	}

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(writer.String(), `"canonicalJobId":"`+retry.SuccessorID+`"`) {
		time.Sleep(10 * time.Millisecond)
	}
	body := writer.String()
	foundMovedFrame := false
	for _, frame := range strings.Split(body, "\n\n") {
		if strings.Contains(frame, `"id":"`+handle+`"`) && strings.Contains(frame, `"canonicalJobId":"`+retry.SuccessorID+`"`) {
			if !strings.HasPrefix(frame, "event: action_updated\n") {
				t.Fatalf("Retry changed a stable legacy handle into a non-update row: %s", frame)
			}
			foundMovedFrame = true
		}
	}
	release()
	budgetHeld = false
	if !foundMovedFrame {
		t.Fatalf("connected legacy SSE did not emit an update for capacity-queued Retry successor %q: %s", retry.SuccessorID, body)
	}
}

type actionHandleQueryCounter struct {
	logger.Interface
	mu         sync.Mutex
	reads      int
	statements []string
}

func (counter *actionHandleQueryCounter) Trace(ctx context.Context, begin time.Time, query func() (string, int64), err error) {
	sql, rows := query()
	if strings.Contains(strings.ToLower(sql), "job_legacy_handles") {
		counter.mu.Lock()
		counter.reads++
		counter.statements = append(counter.statements, sql)
		counter.mu.Unlock()
	}
	counter.Interface.Trace(ctx, begin, func() (string, int64) { return sql, rows }, err)
}

func (counter *actionHandleQueryCounter) capturedStatements() []string {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return append([]string(nil), counter.statements...)
}

func (counter *actionHandleQueryCounter) count() int {
	counter.mu.Lock()
	defer counter.mu.Unlock()
	return counter.reads
}

func TestProjectActionJobsExcludesExpiredHandleHistoryInOneQuery(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	_, terminal := runRetryableActionToFailure(t, tc, owner, "expired query fixture")
	oldAcceptance := time.Now().UTC().Add(-2 * time.Hour)
	if err := tc.DB.Model(&models.Job{}).Where("id = ?", terminal.ID).
		Update("accepted_at", oldAcceptance).Error; err != nil {
		t.Fatalf("age accepted fixture: %v", err)
	}
	var stored models.Job
	if err := tc.DB.First(&stored, "id = ?", terminal.ID).Error; err != nil {
		t.Fatalf("read terminal fixture: %v", err)
	}
	if stored.FinishedAt == nil {
		t.Fatalf("terminal fixture has no finish timestamp: %+v", stored)
	}

	counter := &actionHandleQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	priorLogger := tc.DB.Config.Logger
	tc.DB.Config.Logger = counter
	t.Cleanup(func() { tc.DB.Config.Logger = priorLogger })
	projected, err := tc.AppCtx.WithPrincipal(auth.FromUser(owner)).ProjectActionJobs()
	if err != nil {
		t.Fatalf("project recently finished action with old acceptance time: %v", err)
	}
	if len(projected) != 1 || projected[0].CanonicalJobID != terminal.ID {
		t.Fatalf("old accepted but recently finished action projected as %+v; want one recent terminal row", projected)
	}
	if got := counter.count(); got != 1 {
		t.Fatalf("recent terminal projection issued %d handle-table queries, want one", got)
	}

	oldFinish := time.Now().UTC().Add(-2 * time.Hour)
	if err := tc.DB.Model(&models.Job{}).Where("id = ?", terminal.ID).
		Update("finished_at", oldFinish).Error; err != nil {
		t.Fatalf("age terminal finish time: %v", err)
	}
	history := make([]models.JobLegacyHandle, 500)
	for i := range history {
		history[i] = models.JobLegacyHandle{
			Namespace: application_context.PluginActionHandleNamespace,
			Handle:    fmt.Sprintf("old-action-%04d", i),
			JobID:     terminal.ID,
			CreatedAt: time.Now().Add(-2 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		}
	}
	if err := tc.DB.Create(&history).Error; err != nil {
		t.Fatalf("create expired legacy handle history: %v", err)
	}

	counter.mu.Lock()
	counter.reads = 0
	counter.mu.Unlock()
	projected, err = tc.AppCtx.WithPrincipal(auth.FromUser(owner)).ProjectActionJobs()
	if err != nil {
		t.Fatalf("project action jobs: %v", err)
	}
	if len(projected) != 0 {
		t.Fatalf("expired terminal action history projected %d rows, want none", len(projected))
	}
	if got := counter.count(); got != 1 {
		t.Fatalf("action handle projection issued %d handle-table queries for 501 historical handles, want one filtered query", got)
	}
}

func TestClearPluginActionHandleLocksMappingBeforeReadingIt(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	handle, terminal := runRetryableActionToFailure(t, tc, owner, "clear lock ordering")
	counter := &actionHandleQueryCounter{Interface: logger.Default.LogMode(logger.Silent)}
	priorLogger := tc.DB.Config.Logger
	tc.DB.Config.Logger = counter
	t.Cleanup(func() { tc.DB.Config.Logger = priorLogger })

	marked, err := tc.AppCtx.WithPrincipal(auth.FromUser(owner)).ClearPluginActionHandle(handle, terminal.ID)
	if err != nil || !marked {
		t.Fatalf("clear terminal action handle: marked=%v err=%v", marked, err)
	}
	statements := counter.capturedStatements()
	if len(statements) < 2 || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(statements[0])), "UPDATE ") {
		t.Fatalf("clear transaction must take its SQLite writer lock before reading the handle; statements=%q", statements)
	}
}

func TestClearPluginActionHandleRetriesSQLiteTableLock(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	handle, terminal := runRetryableActionToFailure(t, tc, owner, "clear sqlite lock retry")

	var injected atomic.Bool
	const callbackName = "test:plugin_action_clear_table_lock_once"
	err := tc.DB.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Table == "job_legacy_handles" && injected.CompareAndSwap(false, true) {
			tx.AddError(errors.New("database table is locked: job_legacy_handles"))
		}
	})
	if err != nil {
		t.Fatalf("register lock fault: %v", err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Update().Remove(callbackName) })

	marked, err := tc.AppCtx.WithPrincipal(auth.FromUser(owner)).ClearPluginActionHandle(handle, terminal.ID)
	if !injected.Load() {
		t.Fatal("clear did not reach the injected SQLite table-lock fault")
	}
	if err != nil || !marked {
		t.Fatalf("clear after transient SQLite table lock: marked=%v err=%v", marked, err)
	}
}

func TestLegacyActionEventsReprojectLiveHandleAndHideSuccessor(t *testing.T) {
	tc, owner, admin, ownerToken := setupRetryableActionProjectionEnv(t)
	adminCtx := tc.AppCtx.WithPrincipal(auth.FromUser(admin))
	writer, cancel, done := startPluginActionEventsRequest(t, tc, ownerToken, true)

	hiddenHandle, hiddenOriginal := runRetryableActionToFailure(t, tc, owner, "live hidden")
	release := tc.AppCtx.PluginManager().FillJobBudgetForTest()
	hiddenRetry := retryCanonicalAction(t, adminCtx, hiddenOriginal, "sse-live-hidden-successor")
	hiddenSuccessor, err := tc.AppCtx.GetJob(hiddenRetry.SuccessorID)
	release()
	if err != nil || hiddenSuccessor.State != jobs.StateQueued {
		t.Fatalf("live hidden successor state = %s, err=%v; want queued", hiddenSuccessor.State, err)
	}
	// This second, visible action is an ordered sentinel. Its event follows every
	// queued event for the hidden ancestor, so receiving it proves the SSE reader
	// consumed and refused the stale events instead of leaking the ancestor.
	visibleHandle, visibleJob := runRetryableActionToFailure(t, tc, owner, "live visible sentinel")

	close(writer.releaseInitial)
	select {
	case <-writer.actionWritten:
	case <-time.After(5 * time.Second):
		stopPluginActionEventsRequest(t, cancel, done)
		t.Fatal("legacy action SSE did not deliver the visible sentinel event")
	}
	stopPluginActionEventsRequest(t, cancel, done)
	body := writer.String()
	if strings.Contains(body, hiddenHandle) {
		t.Fatalf("legacy SSE exposed the old handle after its current successor became hidden: %s", body)
	}
	if !strings.Contains(body, `"id":"`+visibleHandle+`"`) ||
		!strings.Contains(body, `"canonicalJobId":"`+visibleJob.ID+`"`) {
		t.Fatalf("legacy SSE did not reproject the visible live event to its durable Job: %s", body)
	}
}

func TestLegacyActionEventsClearAncestorPreservesRetrySuccessor(t *testing.T) {
	tc, owner, _, ownerToken := setupRetryableActionProjectionEnv(t)
	ownerCtx := tc.AppCtx.WithPrincipal(auth.FromUser(owner))
	writer, cancel, done := startPluginActionEventsRequest(t, tc, ownerToken, true)

	handle, original := runRetryableActionToFailure(t, tc, owner, "clear stale ancestor")
	close(writer.releaseInitial)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(writer.String(), `"canonicalJobId":"`+original.ID+`"`) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(writer.String(), `"canonicalJobId":"`+original.ID+`"`) {
		stopPluginActionEventsRequest(t, cancel, done)
		t.Fatalf("SSE did not drain the original action before Retry: %s", writer.String())
	}
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(writer.String(), `"status":"failed"`) {
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(writer.String(), `"status":"failed"`) {
		stopPluginActionEventsRequest(t, cancel, done)
		t.Fatalf("SSE did not observe the failed original action before Retry: %s", writer.String())
	}

	release := tc.AppCtx.PluginManager().FillJobBudgetForTest()
	budgetHeld := true
	defer func() {
		if budgetHeld {
			release()
		}
	}()
	retry := retryCanonicalAction(t, ownerCtx, original, "sse-clear-successor")
	queued, err := tc.AppCtx.GetJob(retry.SuccessorID)
	if err != nil || queued.State != jobs.StateQueued {
		t.Fatalf("retry successor state = %s, err=%v; want queued before clear", queued.State, err)
	}

	clearResponse := doReq(tc, http.MethodPost, "/v1/jobs/clearCompleted",
		map[string]string{"Authorization": "Bearer " + ownerToken}, nil, nil)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear completed response: status %d body %s", clearResponse.Code, clearResponse.Body.String())
	}
	var cleared struct {
		Cleared int      `json:"cleared"`
		IDs     []string `json:"ids"`
	}
	if err := json.Unmarshal(clearResponse.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decode clear completed response: %v (%s)", err, clearResponse.Body.String())
	}
	for _, id := range cleared.IDs {
		if id == handle {
			t.Fatalf("clear completed told legacy client to dismiss active retry handle %q: %+v", handle, cleared)
		}
	}

	// The held capacity keeps the successor queued, so the removed ancestor event
	// must reproject to an update under the same legacy handle.
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(writer.String(), `"canonicalJobId":"`+retry.SuccessorID+`"`) {
		time.Sleep(10 * time.Millisecond)
	}
	release()
	budgetHeld = false
	// A later visible action is an ordered sentinel: receiving it proves the
	// stream remains usable after the moved-handle event.
	sentinelHandle, _ := runRetryableActionToFailure(t, tc, owner, "clear sentinel")
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(writer.String(), `"id":"`+sentinelHandle+`"`) {
		time.Sleep(10 * time.Millisecond)
	}
	stopPluginActionEventsRequest(t, cancel, done)
	body := writer.String()
	updatedMovedHandle := false
	for _, frame := range strings.Split(body, "\n\n") {
		if strings.Contains(frame, `"id":"`+handle+`"`) && strings.Contains(frame, `"canonicalJobId":"`+retry.SuccessorID+`"`) && strings.HasPrefix(frame, "event: action_updated\n") {
			updatedMovedHandle = true
		}
	}
	if !updatedMovedHandle {
		t.Fatalf("SSE did not reproject the removed ancestor as an update: %s", body)
	}
	if strings.Contains(body, "event: action_removed\n") {
		t.Fatalf("SSE removed a still-live retry successor after clearing its ancestor: %s", body)
	}
}

func TestLegacyActionClearStaysClearedAcrossReconnectAndRetry(t *testing.T) {
	tc, owner, _, ownerToken := setupRetryableActionProjectionEnv(t)
	ownerCtx := tc.AppCtx.WithPrincipal(auth.FromUser(owner))
	handle, failed := runRetryableActionToFailure(t, tc, owner, "durable clear marker")

	clearResponse := doReq(tc, http.MethodPost, "/v1/jobs/clearCompleted",
		map[string]string{"Authorization": "Bearer " + ownerToken}, nil, nil)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear completed response: status %d body %s", clearResponse.Code, clearResponse.Body.String())
	}
	var cleared struct {
		Cleared int      `json:"cleared"`
		IDs     []string `json:"ids"`
	}
	if err := json.Unmarshal(clearResponse.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decode clear completed response: %v (%s)", err, clearResponse.Body.String())
	}
	if cleared.Cleared != 1 || len(cleared.IDs) != 1 || cleared.IDs[0] != handle {
		t.Fatalf("clear completed response %+v; want only cleared handle %q", cleared, handle)
	}

	// A clear is a list-panel dismissal: the durable Job and compatibility
	// handle still answer direct reads, while a reconnect does not add the row
	// back to the legacy panel's initial snapshot.
	jobResponse := doReq(tc, http.MethodGet, "/v1/jobs/action/job?id="+handle,
		map[string]string{"Authorization": "Bearer " + ownerToken}, nil, nil)
	if jobResponse.Code != http.StatusOK {
		t.Fatalf("GET cleared but retained action handle: status %d body %s", jobResponse.Code, jobResponse.Body.String())
	}
	var retained plugin_system.ActionJob
	if err := json.Unmarshal(jobResponse.Body.Bytes(), &retained); err != nil {
		t.Fatalf("decode retained action Job: %v (%s)", err, jobResponse.Body.String())
	}
	if retained.CanonicalJobID != failed.ID {
		t.Fatalf("cleared action handle resolved to %q, want retained canonical Job %q", retained.CanonicalJobID, failed.ID)
	}
	writer, cancel, done := startPluginActionEventsRequest(t, tc, ownerToken, false)
	stopPluginActionEventsRequest(t, cancel, done)
	body := writer.String()
	frameEnd := strings.Index(body, "\n\n")
	if frameEnd < 0 {
		t.Fatalf("reconnected SSE has no complete init frame: %q", body)
	}
	frame := body[:frameEnd]
	dataAt := strings.Index(frame, "data: ")
	if dataAt < 0 {
		t.Fatalf("reconnected SSE init has no data field: %q", frame)
	}
	var init struct {
		ActionJobs []plugin_system.ActionJob `json:"actionJobs"`
	}
	if err := json.Unmarshal([]byte(frame[dataAt+len("data: "):]), &init); err != nil {
		t.Fatalf("decode reconnected SSE init: %v (%s)", err, frame)
	}
	if len(init.ActionJobs) != 0 {
		t.Fatalf("reconnected SSE re-added cleared action row: %+v", init.ActionJobs)
	}

	// Retry moves the compatibility handle to a new canonical Job. The clear
	// marker remains bound to the ancestor, so this still-live successor returns
	// to the panel and stays pollable through the same handle.
	release := tc.AppCtx.PluginManager().FillJobBudgetForTest()
	retry := retryCanonicalAction(t, ownerCtx, failed, "retry-after-clear")
	queued, err := tc.AppCtx.GetJob(retry.SuccessorID)
	release()
	if err != nil || queued.State != jobs.StateQueued {
		t.Fatalf("retried action state = %s, err=%v; want queued", queued.State, err)
	}
	projected, err := ownerCtx.ProjectActionJobs()
	if err != nil {
		t.Fatalf("project action after Retry of cleared Job: %v", err)
	}
	if len(projected) != 1 || projected[0].ID != handle || projected[0].CanonicalJobID != retry.SuccessorID || projected[0].Status != "pending" {
		t.Fatalf("retry successor did not become visible after the ancestor was cleared: %+v", projected)
	}
}

func TestLegacyActionClearFindsDurableTerminalFromAnotherProcess(t *testing.T) {
	tc, owner, _, _ := setupRetryableActionProjectionEnv(t)
	handle, terminal := runRetryableActionToFailure(t, tc, owner, "cross-process clear")
	// Model another server process sharing the same database: it has the durable
	// Job projection but no in-memory entry for the action owned by this process.
	ctx := actionContextWithoutPluginManager{MahresourcesContext: tc.AppCtx.WithPrincipal(auth.FromUser(owner))}
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs/clearCompleted", nil)
	request = request.WithContext(auth.WithPrincipal(request.Context(), auth.FromUser(owner)))
	response := httptest.NewRecorder()
	api_handlers.GetJobsClearCompletedHandler(ctx)(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("clear durable action without local manager: status %d body %s", response.Code, response.Body.String())
	}
	var cleared struct {
		Cleared int      `json:"cleared"`
		IDs     []string `json:"ids"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &cleared); err != nil {
		t.Fatalf("decode cross-process clear response: %v (%s)", err, response.Body.String())
	}
	if cleared.Cleared != 1 || len(cleared.IDs) != 1 || cleared.IDs[0] != handle {
		t.Fatalf("cross-process clear returned %+v; want durable handle %q for Job %q", cleared, handle, terminal.ID)
	}
	projected, err := tc.AppCtx.WithPrincipal(auth.FromUser(owner)).ProjectActionJobs()
	if err != nil {
		t.Fatalf("project after cross-process clear: %v", err)
	}
	if len(projected) != 0 {
		t.Fatalf("cross-process clear left %d terminal action rows visible: %+v", len(projected), projected)
	}
}

func TestLegacyActionSSEClearDoesNotDuplicateRemovalOnDurablePoll(t *testing.T) {
	tc, owner, _, ownerToken := setupRetryableActionProjectionEnv(t)
	_, _ = runRetryableActionToFailure(t, tc, owner, "clear removal dedupe")
	writer, cancel, done := startPluginActionEventsRequest(t, tc, ownerToken, false)
	defer stopPluginActionEventsRequest(t, cancel, done)

	clearResponse := doReq(tc, http.MethodPost, "/v1/jobs/clearCompleted",
		map[string]string{"Authorization": "Bearer " + ownerToken}, nil, nil)
	if clearResponse.Code != http.StatusOK {
		t.Fatalf("clear completed response: status %d body %s", clearResponse.Code, clearResponse.Body.String())
	}
	deadline := time.Now().Add(2500 * time.Millisecond)
	for time.Now().Before(deadline) && strings.Count(writer.String(), "event: action_removed\n") == 0 {
		time.Sleep(10 * time.Millisecond)
	}
	if strings.Count(writer.String(), "event: action_removed\n") > 0 {
		// Keep the connection past one durable poll tick to catch a duplicate
		// removal when the local manager event races the snapshot diff.
		time.Sleep(2200 * time.Millisecond)
	}
	if got := strings.Count(writer.String(), "event: action_removed\n"); got != 1 {
		t.Fatalf("clear should produce one removal even after a durable poll tick, got %d: %s", got, writer.String())
	}
}
