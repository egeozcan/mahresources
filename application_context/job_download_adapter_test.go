package application_context

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/plugin_system"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/jmoiron/sqlx"
	"gorm.io/driver/sqlite"
)

// This file drives the download Kind adapter end to end: a submission that becomes
// a durable Job before anything is dispatched, a transfer that publishes its whole
// lifecycle into that Job through the execution token that owns it, and the control
// surface the Job advertises.
//
// It is driven through the same seam the HTTP layer uses — the context's own
// submission method, a real queue, a real running runtime — because the properties
// worth pinning are about the chain, and a fake at any link would let the test pass
// while a download ran unmirrored.

// newDownloadJobContext builds a context that can run one download from submission
// to a created Resource: the resource tables the library writes, the durable job
// core, a replay keyring so a Job's input can be sealed, a control plane with this
// context's Kinds registered, and a running runtime — which is what adopts a
// submitted transfer and lets it publish.
func newDownloadJobContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	return newJobHarnessContext(t, true)
}

// newJobHarnessContext builds the same context, with the dispatch loop optional.
//
// The loop is what a test needs when it is about work being *run*; a test about what
// happens to work nobody is running — reconciliation, above all — needs the Job to stay
// exactly as it was left, and a running loop would adopt it a tick later and finish it
// first. One harness with one switch, rather than two fixtures that drift.
func newJobHarnessContext(t *testing.T, withRuntime bool) *MahresourcesContext {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "download-jobs.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(
		&models.Resource{}, &models.ResourceVersion{}, &models.ResourceCategory{},
		&models.Series{}, &models.Tag{}, &models.Group{}, &models.Note{}, &models.NoteType{},
		&models.Category{}, &models.Preview{}, &models.ImageHash{}, &models.GroupRelation{},
		&models.GroupRelationType{}, &models.NoteBlock{}, &models.User{}, &models.LogEntry{},
		&models.PluginKV{}, &models.PluginState{}, &models.RuntimeSetting{}, &models.DownloadHistoryEntry{}, &models.ScheduledDownload{},
		&models.Query{}, &models.SavedMRQLQuery{}, &models.SavedSearch{}, &models.UserSetting{}, &models.Session{}, &models.ApiToken{}, &models.TemplatePartial{}, &models.ResourceSimilarity{},
		&models.PluginSchedule{}, &models.PluginCommandRun{}, &models.PluginCommandImport{}, &models.ResourceReduction{},
		&models.Job{}, &models.JobEvent{}, &models.JobEventSequence{}, &models.JobLink{},
		&models.JobOutput{}, &models.JobReplayEnvelope{}, &models.JobClaim{},
		&models.JobCapacityLease{}, &models.JobPreference{}, &models.JobPinGuard{},
		&models.JobCommandRequest{}, &models.JobLegacyHandle{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	// One plugin the deferred tests can schedule through: a deferred download
	// always names the plugin that asked for it, and its egress policy is what the
	// transfer runs under, so the dispatch path cannot be exercised without one.
	//
	// A second plugin declares the things the plugin-action Kind runs — an async
	// action, a failing action, an action that starts a child job, and a schedule —
	// because none of that can be exercised through a plugin that declares none.
	// Neither is enabled by the harness: a test says which one it needs, and a
	// disabled plugin must not be reachable by accident.
	pluginDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(pluginDir, downloadTestPlugin), 0o755); err != nil {
		t.Fatalf("mkdir plugin: %v", err)
	}
	pluginSource := `
plugin = { name = "` + downloadTestPlugin + `", version = "1.0", api_version = 1,
           capabilities = { "db:write" },
           network = { "127.0.0.1" } }
function init() end
`
	if err := os.WriteFile(filepath.Join(pluginDir, downloadTestPlugin, "plugin.lua"), []byte(pluginSource), 0o644); err != nil {
		t.Fatalf("write plugin: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(pluginDir, pluginActionTestPlugin), 0o755); err != nil {
		t.Fatalf("mkdir action plugin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, pluginActionTestPlugin, "plugin.lua"), []byte(pluginActionTestSource), 0o644); err != nil {
		t.Fatalf("write action plugin: %v", err)
	}

	cfg := &MahresourcesConfig{
		DbType:     constants.DbTypeSqlite,
		PluginPath: pluginDir,
		// An httptest server binds to loopback, which the host fetch policy denies
		// by default — that deny is the point of -allow-private-fetch. A test that
		// fetches from its own server declares it, exactly as a deployment fetching
		// from a LAN service would.
		AllowPrivateFetch: []string{"127.0.0.1", "::1"},
	}
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), cfg)

	ring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("build replay keyring: %v", err)
	}
	ctx.SetJobReplayKeyring(ring)

	service := jobs.NewService()
	ctx.SetJobService(service)

	if !withRuntime {
		return ctx
	}
	// 50ms rather than 20ms, for the reason the api_tests harness gives: one claim query
	// per registered Kind per tick against a shared-cache in-memory database, where a
	// reader and a writer of one table can collide in a way the production DSN does not.
	runtime := NewJobRuntime(ctx, service, JobRuntimeConfig{
		Claimant: "download-adapter-test",
		Interval: 50 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)
	return ctx
}

// downloadTestPlugin is the plugin the deferred tests schedule through.
const downloadTestPlugin = "planner-plugin"

// enableDownloadTestPlugin enables that plugin, which is what makes its network
// policy resolvable: a deferred Job's transfer runs under the policy of the plugin
// that asked for it, and an unresolvable policy is a refusal.
func enableDownloadTestPlugin(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	pm := ctx.PluginManager()
	if pm == nil {
		t.Fatal("the download test context has no plugin manager")
	}
	if err := pm.EnablePlugin(downloadTestPlugin); err != nil {
		t.Fatalf("enable %s: %v", downloadTestPlugin, err)
	}
}

// plainContentServer serves one file, so a download has something to fetch.
func plainContentServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

// waitForSnapshot polls one Job until cond holds.
func waitForSnapshot(t *testing.T, ctx *MahresourcesContext, jobID string, what string, cond func(jobs.Snapshot) bool) jobs.Snapshot {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	var last jobs.Snapshot
	for time.Now().Before(deadline) {
		snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
		if err == nil {
			last = snap
			if cond(snap) {
				return snap
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; the job is %s/%s", what, last.State, last.Phase)
	return last
}

// TestASubmissionAcceptsADurableJobBeforeDispatchAndRunsItToSuccess is the whole
// dual-publication contract for a fresh download: the Job exists durably with a
// sanitized summary and a sealed input before the transfer is dispatched, the
// transfer publishes into it, and the outcome points at the Resource it created.
func TestASubmissionAcceptsADurableJobBeforeDispatchAndRunsItToSuccess(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server := plainContentServer(t, "canonical download body")

	creator := &query_models.ResourceFromRemoteCreator{
		URL:     server.URL + "/clip.mp4?token=super-secret#frag",
		Headers: map[string]string{"Referer": "https://internal.example/secret-page"},
	}
	submissions := ctx.SubmitRemoteDownloads(creator, nil, "", "api")
	if len(submissions) != 1 {
		t.Fatalf("%d submissions, want one", len(submissions))
	}
	if submissions[0].Err != nil {
		t.Fatalf("submit: %v", submissions[0].Err)
	}
	if submissions[0].Job == nil {
		t.Fatalf("no queue entry was created")
	}
	canonicalID := submissions[0].CanonicalJobID
	if canonicalID == "" {
		t.Fatalf("the submission created no durable job")
	}
	if submissions[0].Job.CanonicalJobID != canonicalID {
		t.Fatalf("the queue entry names %q, want %q", submissions[0].Job.CanonicalJobID, canonicalID)
	}

	// The Job exists, is sanitized, and is already reachable by the legacy id.
	accepted, err := ctx.GetJob(canonicalID)
	if err != nil {
		t.Fatalf("read the accepted job: %v", err)
	}
	if accepted.Kind != JobKindRemoteDownload || accepted.KindVersion != jobDownloadKindVersion {
		t.Fatalf("the job is %s v%d, want %s v%d", accepted.Kind, accepted.KindVersion,
			JobKindRemoteDownload, jobDownloadKindVersion)
	}
	summary := string(accepted.Summary)
	for _, secret := range []string{"super-secret", "token=", "frag", "internal.example", "Referer"} {
		if strings.Contains(summary, secret) {
			t.Fatalf("the searchable summary leaks %q: %s", secret, summary)
		}
	}
	if !strings.Contains(summary, "127.0.0.1") {
		t.Fatalf("the summary does not name the host it fetches: %s", summary)
	}
	if _, err := ctx.ResolveJobHandle(DownloadHandleNamespace, submissions[0].Job.ID); err != nil {
		t.Fatalf("the legacy id does not resolve to its job: %v", err)
	}

	// And the transfer runs to a success that points at what it created.
	succeeded := waitForSnapshot(t, ctx, canonicalID, "the download to succeed",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if succeeded.State != jobs.StateSucceeded {
		t.Fatalf("the download ended %s (%+v)", succeeded.State, succeeded.Failure)
	}
	outputs, err := ctx.GetJobOutputs(canonicalID)
	if err != nil {
		t.Fatalf("outputs: %v", err)
	}
	var resourceID uint
	for _, output := range outputs {
		if output.Key != jobDownloadResourceOutput {
			continue
		}
		var reference struct {
			ResourceID uint `json:"resourceId"`
		}
		if err := json.Unmarshal(output.Reference, &reference); err != nil {
			t.Fatalf("output reference: %v", err)
		}
		resourceID = reference.ResourceID
	}
	if resourceID == 0 {
		t.Fatalf("a succeeded download published no resource output: %+v", outputs)
	}
	var stored models.Resource
	if err := ctx.db.Where("id = ?", resourceID).First(&stored).Error; err != nil {
		t.Fatalf("the output names resource %d, which does not exist: %v", resourceID, err)
	}
	if !strings.HasPrefix(stored.Name, "clip.mp4") {
		t.Fatalf("created resource is named %q, want the submitted file's name", stored.Name)
	}

	// Progress is a snapshot rather than an event, so it is asserted where it lives —
	// and the timeline is asserted for what it must not contain.
	timeline, err := ctx.GetJobTimeline(canonicalID, 0, 0)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	if len(timeline) == 0 {
		t.Fatalf("the job has no timeline at all")
	}
	for _, event := range timeline {
		if strings.Contains(string(event.Detail), "super-secret") {
			t.Fatalf("a job event leaked the URL's query: %s", event.Detail)
		}
	}
}

// TestAMultiURLSubmissionAcceptsEachURLIndependently proves the per-URL shape: one
// refused line does not take the accepted ones with it, and the refusal is reported
// against its own URL.
func TestAMultiURLSubmissionAcceptsEachURLIndependently(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server := plainContentServer(t, "multi body")

	creator := &query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/one.txt\n\n" + server.URL + "/two.txt",
	}
	submissions := ctx.SubmitRemoteDownloads(creator, nil, "", "api")
	if len(submissions) != 2 {
		t.Fatalf("%d submissions, want one per non-empty URL", len(submissions))
	}
	for i, submission := range submissions {
		if submission.Err != nil {
			t.Fatalf("URL %d was refused: %v", i, submission.Err)
		}
		if submission.CanonicalJobID == "" {
			t.Fatalf("URL %d created no durable job", i)
		}
	}

	labels := map[string]bool{}
	for _, submission := range submissions {
		labels[submission.URL] = true
	}
	if !labels[server.URL+"/one.txt"] || !labels[server.URL+"/two.txt"] {
		t.Fatalf("the submissions name %v, want both URLs", labels)
	}

	// A submission the queue refuses is reported for its own URL, and the Job that
	// was accepted for it does not linger claiming work nothing will dispatch.
	refused := ctx.submitRemoteDownload(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/three.txt",
		Headers: map[string]string{
			"Content-Length": "12",
		},
	}, nil, "", "api")
	if refused.Err == nil {
		t.Fatalf("a refused header was accepted")
	}
	if refused.CanonicalJobID == "" {
		t.Fatalf("the refusal left no durable record to inspect")
	}
	failed := waitForSnapshot(t, ctx, refused.CanonicalJobID, "the refused submission to be recorded",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if failed.State != jobs.StateFailed || failed.Failure == nil || failed.Failure.Code != "submission-refused" {
		t.Fatalf("the refused submission was left as %s (%+v)", failed.State, failed.Failure)
	}
}

// TestHeldWorkIsBlockedAndResumedThroughTheCanonicalSurface pins the honest
// representation of this queue's pause: it cannot checkpoint, so a held transfer is
// not `paused` — it is blocked for a person — and the Job advertises resume rather
// than pause.
func TestHeldWorkIsBlockedAndResumedThroughTheCanonicalSurface(t *testing.T) {
	ctx := newDownloadJobContext(t)

	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte("slow body"))
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) })

	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/slow.bin",
	}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID

	waitForSnapshot(t, ctx, jobID, "the transfer to start",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateRunning })

	// No pause is advertised, because this Kind cannot confirm a checkpoint.
	commands, err := ctx.AdvertisedJobCommands(context.Background(), jobID)
	if err != nil {
		t.Fatalf("advertised commands: %v", err)
	}
	if hasCommand(commands, jobs.CommandPause) {
		t.Fatalf("the download Kind advertises pause, which it cannot honor: %+v", commands)
	}

	// The legacy pause holds the transfer, and the Job says so.
	if err := ctx.DownloadManager().Pause(submissions[0].Job.ID); err != nil {
		t.Fatalf("pause: %v", err)
	}
	held := waitForSnapshot(t, ctx, jobID, "the hold to be mirrored",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateBlocked })
	if held.State != jobs.StateBlocked {
		t.Fatalf("a held download is %s, want blocked", held.State)
	}

	commands, err = ctx.AdvertisedJobCommands(context.Background(), jobID)
	if err != nil {
		t.Fatalf("advertised commands after the hold: %v", err)
	}
	if !hasCommand(commands, jobs.CommandResume) {
		t.Fatalf("held work offers no resume: %+v", commands)
	}

	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: jobID, Key: jobs.CommandResume, IdempotencyKey: "resume-1", ExpectedVersion: held.Version,
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if result.Status != jobs.CommandStatusSucceeded {
		t.Fatalf("resume answered %s/%s: %s", result.Status, result.Code, result.Message)
	}
	waitForSnapshot(t, ctx, jobID, "the resumed transfer to run again",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateRunning })
}

// hasCommand reports whether one advertised set contains a key.
func hasCommand(commands []jobs.Command, key string) bool {
	for _, command := range commands {
		if command.Key == key {
			return true
		}
	}
	return false
}

// TestADownloadClassifiesItsFailureWithoutCarryingTheURL keeps the two halves of a
// failure apart: the queue's own error text stays on the legacy surfaces, and the
// Job records a bounded taxonomy — the queue's errors can name the URL, and a Job's
// failure message is searchable text.
func TestADownloadClassifiesItsFailureWithoutCarryingTheURL(t *testing.T) {
	ctx := newDownloadJobContext(t)
	// A server that fails every request: the transfer fails for a reason only the
	// queue can explain, and that explanation must not become the Job's text.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	secret := server.URL + "/secret-path.bin?signature=do-not-store"
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: secret}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	failed := waitForSnapshot(t, ctx, submissions[0].CanonicalJobID, "the download to fail",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if failed.State != jobs.StateFailed {
		t.Fatalf("the failed transfer ended %s", failed.State)
	}
	if failed.Failure == nil || failed.Failure.Code == "" || failed.Failure.Class == "" {
		t.Fatalf("the failure is not classified: %+v", failed.Failure)
	}
	if strings.Contains(failed.Failure.Message, "signature") || strings.Contains(failed.Failure.Message, "secret-path") {
		t.Fatalf("the failure message carries the URL: %q", failed.Failure.Message)
	}
	timeline, err := ctx.GetJobTimeline(submissions[0].CanonicalJobID, 0, 0)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	for _, event := range timeline {
		if strings.Contains(string(event.Detail), "signature") {
			t.Fatalf("an event carries the URL's query: %s", event.Detail)
		}
	}
}

// TestTheLegacyQueueStillRunsWithoutAControlPlane is the compatibility half: a
// context whose deployment installed no control plane submits exactly as it did
// before, and nothing is mirrored.
func TestTheLegacyQueueStillRunsWithoutAControlPlane(t *testing.T) {
	ctx := newDownloadJobContext(t)
	ctx.SetJobService(nil)

	server := plainContentServer(t, "legacy body")
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{
		URL: server.URL + "/plain.txt",
	}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	if submissions[0].CanonicalJobID != "" {
		t.Fatalf("a deployment with no control plane reported canonical job %q", submissions[0].CanonicalJobID)
	}
	entry := submissions[0].Job
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if snap := entry.Snapshot(); snap.Status == download_queue.JobStatusCompleted {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("the legacy transfer never completed: %s", entry.GetStatus())
}

// TestADeferredDownloadIsAcceptedAsAScheduledJob pins the deferred half of the
// contract: one-off deferred work names one concrete execution, so it is a
// `scheduled` Job immediately rather than a definition that materializes later —
// and the row the plugin management surfaces list is its handle.
func TestADeferredDownloadIsAcceptedAsAScheduledJob(t *testing.T) {
	ctx := newDownloadJobContext(t)

	actor, err := ctx.CreateUser(&UserInput{Username: "planner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}

	due := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Second)
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/later.bin"}, due)
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}

	job, err := ctx.ResolveJobHandle(ScheduledDownloadHandleNamespace, strconv.FormatUint(uint64(row.ID), 10))
	if err != nil {
		t.Fatalf("resolve the row's handle: %v", err)
	}
	if job.Kind != JobKindDeferredDownload {
		t.Fatalf("the deferred job is %s, want %s", job.Kind, JobKindDeferredDownload)
	}
	if job.State != jobs.StateScheduled {
		t.Fatalf("the deferred job is %s, want scheduled", job.State)
	}
	if job.ScheduledFor == nil || !job.ScheduledFor.Equal(due) {
		t.Fatalf("the deferred job is scheduled for %v, want %v", job.ScheduledFor, due)
	}
	if job.OwnerUserID == nil || *job.OwnerUserID != actor.ID {
		t.Fatalf("the deferred job's owner is %v, want %d", job.OwnerUserID, actor.ID)
	}
	if job.ActorUserID == nil || *job.ActorUserID != actor.ID {
		t.Fatalf("the deferred job's actor is %v, want %d", job.ActorUserID, actor.ID)
	}

	// Nothing has been dispatched, and the queue knows nothing about it: a
	// deferred download is durable work that has not started, not a queued
	// transfer waiting for a worker.
	if entries := ctx.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("%d queue entries exist for work that is not due yet", len(entries))
	}
}

// TestDueDeferredWorkRunsAsTheSameJob is the "no second execution" property: the
// Job accepted for the future is the Job that runs, and its due time is what makes
// it claimable.
func TestDueDeferredWorkRunsAsTheSameJob(t *testing.T) {
	ctx := newDownloadJobContext(t)
	server := plainContentServer(t, "deferred body")

	enableDownloadTestPlugin(t, ctx)

	actor, err := ctx.CreateUser(&UserInput{Username: "planner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: server.URL + "/later.txt"},
		time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	accepted, err := ctx.ResolveJobHandle(ScheduledDownloadHandleNamespace, strconv.FormatUint(uint64(row.ID), 10))
	if err != nil {
		t.Fatalf("resolve the row's handle: %v", err)
	}

	// The row's own fire path materializes it, and the Job it materializes is the
	// one that was accepted.
	jobID, materialized, err := ctx.materializeDeferredDownloadJob(row.ID)
	if err != nil {
		t.Fatalf("materialize the deferred job: %v", err)
	}
	if !materialized || jobID != accepted.ID {
		t.Fatalf("the due time materialized %q (%v), want the accepted job %s", jobID, materialized, accepted.ID)
	}

	// The transfer is dispatched as the accepted Job and runs to an outcome, which is
	// what "the same execution" means here: the Job that was accepted for the future
	// is the Job that ran. (The plugin that asked for it confines its fetches to its
	// own declared hosts, and every plugin's policy denies loopback outright, so the
	// transfer itself fails — that is the egress layer working, not this test's
	// subject.)
	ran := waitForSnapshot(t, ctx, accepted.ID, "the deferred download to be dispatched",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if ran.ID != accepted.ID {
		t.Fatalf("the deferred download ran as %s, want the accepted job", ran.ID)
	}
	if _, ok := ctx.DownloadManager().GetJobByCanonicalJobID(accepted.ID); !ok {
		t.Fatalf("the accepted job was never dispatched to the queue as one execution")
	}
	// Exactly one Job of the deferred Kind exists: a materialization that created a
	// second execution would be visible right here.
	page, err := ctx.ListJobs(jobs.Filter{Kinds: []string{JobKindDeferredDownload}}, jobs.Cursor{}, 0)
	if err != nil {
		t.Fatalf("list deferred jobs: %v", err)
	}
	if len(page.Jobs) != 1 {
		t.Fatalf("%d deferred jobs exist, want one execution", len(page.Jobs))
	}
}

// TestDeferredWorkWithADeletedActorIsBlockedAndNeverRunsAsTheHost is acceptance
// property: a scheduled Job whose actor is gone blocks rather than falling back to
// root or the host, because the principal a deferred command would act as is the
// one that asked for it and that identity cannot be substituted.
func TestDeferredWorkWithADeletedActorIsBlockedAndNeverRunsAsTheHost(t *testing.T) {
	ctx := newDownloadJobContext(t)

	actor, err := ctx.CreateUser(&UserInput{Username: "departing", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create the acting user: %v", err)
	}
	row, err := ctx.CreateScheduledDownload(downloadTestPlugin, actor.ID,
		&query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/orphan.bin"},
		time.Now().Add(-time.Second))
	if err != nil {
		t.Fatalf("create the deferred download: %v", err)
	}
	accepted, err := ctx.ResolveJobHandle(ScheduledDownloadHandleNamespace, strconv.FormatUint(uint64(row.ID), 10))
	if err != nil {
		t.Fatalf("resolve the row's handle: %v", err)
	}
	if err := ctx.DeleteUser(actor.ID); err != nil {
		t.Fatalf("delete the acting user: %v", err)
	}

	blocked := waitForSnapshot(t, ctx, accepted.ID, "the orphaned deferred work to block",
		func(snap jobs.Snapshot) bool { return snap.State == jobs.StateBlocked })
	if blocked.State != jobs.StateBlocked {
		t.Fatalf("the orphaned deferred job is %s, want blocked", blocked.State)
	}
	if entries := ctx.DownloadManager().GetJobs(); len(entries) != 0 {
		t.Fatalf("work whose actor is gone was dispatched anyway: %d queue entries", len(entries))
	}
}

// TestAPluginsImmediateDownloadIsADurableJob pins the one production Lua surface
// that used to bypass durable acceptance: mah.download.submit.
//
// It reaches the queue through SubmitDownload, and going straight to
// SubmitForPlugin made its work memory-only — no canonical Job, nothing in the Job
// Center, and nothing left of it but a legacy history row once the process moved
// on. It goes through the same funnel every other remote download does instead,
// and answers the plugin the shape it always has.
func TestAPluginsImmediateDownloadIsADurableJob(t *testing.T) {
	ctx := newDownloadJobContext(t)
	enableDownloadTestPlugin(t, ctx)

	// A reserved, never-resolving host: a plugin's transfer is policed by its own
	// egress list, which denies the loopback address a test server would use, and
	// what is under test here is the durable acceptance rather than the transfer.
	response, err := ctx.SubmitDownload(downloadTestPlugin, 0, "https://example.invalid/plugin.bin", nil)
	if err != nil {
		t.Fatalf("submit the plugin's download: %v", err)
	}
	legacyID, _ := response["id"].(string)
	if legacyID == "" {
		t.Fatalf("the plugin was answered %+v", response)
	}

	// The id the plugin holds is the legacy handle of a canonical Job, and the
	// queue entry it names is that Job's execution.
	resolved, err := ctx.ResolveJobHandle(DownloadHandleNamespace, legacyID)
	if err != nil {
		t.Fatalf("the answered id resolves to no durable job: %v", err)
	}
	if resolved.Kind != JobKindRemoteDownload {
		t.Fatalf("the plugin's download is a %s job, want %s", resolved.Kind, JobKindRemoteDownload)
	}
	entry, ok := ctx.DownloadManager().GetJob(legacyID)
	if !ok {
		t.Fatalf("no queue entry carries the answered id")
	}
	if entry.CanonicalJobID != resolved.ID {
		t.Fatalf("the queue entry names job %q, want %q", entry.CanonicalJobID, resolved.ID)
	}
	summary := string(resolved.Summary)
	if !strings.Contains(summary, downloadTestPlugin) {
		t.Fatalf("the durable record does not name the plugin that asked: %s", summary)
	}

	// The transfer itself fails — the host does not resolve — and that failure is
	// the Job's, recorded on the durable record rather than only in the queue's
	// memory.
	finished := waitForSnapshot(t, ctx, resolved.ID, "the download to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if finished.State != jobs.StateFailed {
		t.Fatalf("the download ended %s (%+v), want the transfer's failure", finished.State, finished.Failure)
	}
}

// TestAPluginsImmediateDownloadReportsAnAcceptanceFailure is the other half: when
// the Job cannot be accepted at all, the plugin is told and the queue is left
// alone. A submission that started a transfer nothing durable had agreed to is the
// failure this path exists to make impossible.
func TestAPluginsImmediateDownloadReportsAnAcceptanceFailure(t *testing.T) {
	ctx := newDownloadJobContext(t)
	enableDownloadTestPlugin(t, ctx)

	// No replay key: a download Job whose input must be sealed cannot be accepted,
	// and the refusal has to reach the caller rather than the queue.
	ctx.SetJobReplayKeyring(nil)
	before := len(ctx.DownloadManager().GetJobs())

	if _, err := ctx.SubmitDownload(downloadTestPlugin, 0, "https://example.invalid/unsealed.bin", nil); err == nil {
		t.Fatalf("a submission with no replay key was accepted")
	}
	if after := len(ctx.DownloadManager().GetJobs()); after != before {
		t.Fatalf("the refused submission left %d queue entries behind", after-before)
	}
}

// TestAQueueBackedReconcileNeedsProofTheExecutorIsGone is §3's rule for a Kind whose
// executor is one process's memory.
//
// The queue is not a database: an entry this process cannot see may be running
// perfectly well in another one, so "no entry here" is not "nobody is running it",
// and queuing the work again on that absence starts a second transfer of a URL a
// live process is already fetching. What decides is the claim's own runtime
// identity — the process that took the Job when it was dispatched — and until that
// process is proved gone the Job stays nonterminal and blocked with its claim and
// its capacity held.
func TestAQueueBackedReconcileNeedsProofTheExecutorIsGone(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	input, err := json.Marshal(downloadJobInput{
		Creator: &query_models.ResourceFromRemoteCreator{URL: "https://example.invalid/x.bin"},
	})
	if err != nil {
		t.Fatalf("encode the input: %v", err)
	}

	claimDownloadForTest := func(t *testing.T, jobID, claimant string) {
		t.Helper()
		_, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
			JobID: jobID, Claimant: claimant, Lease: 20 * time.Millisecond,
		})
		if err != nil || !claimed {
			t.Fatalf("claim %s as %q: claimed=%v err=%v", jobID, claimant, claimed, err)
		}
	}

	acceptDownloadForTest := func(t *testing.T) jobs.Snapshot {
		t.Helper()
		return acceptJobFor(t, ctx, jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
			State: jobs.StateQueued, Origin: "api",
			Replay: jobs.ReplayInput{Input: input},
		})
	}

	// A process that is still running — this one — holds the transfer. Re-queuing it
	// would start a second one, so the Job stays blocked with its claim held.
	live := acceptDownloadForTest(t)
	claimDownloadForTest(t, live.ID, plugin_system.CurrentRuntimeIdentity().String())
	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, ctx, live.ID); decision != jobs.ReconcileExternalWorkUnproven {
		t.Fatalf("a transfer whose runtime is alive was decided %q, want it left unresolved", decision)
	}
	held, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, live.ID)
	if err != nil {
		t.Fatalf("read the held job: %v", err)
	}
	if held.State != jobs.StateBlocked {
		t.Fatalf("the job is %s, want blocked while its runtime may still be running it", held.State)
	}
	if claim := storedClaim(t, ctx, live.ID); claim.State == models.JobClaimStateReleased {
		t.Fatalf("the claim was released, so a replacement could be dispatched over live work")
	}

	// A process that cannot exist any more: the work may be queued again, and the
	// next runtime starts it.
	goneUntil := plugin_system.CurrentRuntimeIdentity().Host + "/boot-that-ended/4242"
	gone := acceptDownloadForTest(t)
	claimDownloadForTest(t, gone.ID, goneUntil)
	time.Sleep(40 * time.Millisecond)
	if decision := reconcileOnce(t, ctx, gone.ID); decision != jobs.ReconcileQueue {
		t.Fatalf("a transfer whose runtime is proved gone was decided %q, want it queued again", decision)
	}
}

// TestAResumeQueuesWorkRatherThanStartingAnUnbudgetedTransfer is the admission half
// of releasing a hold.
//
// A held download has already given its claim and its capacity back, so a resume that
// started a worker from inside the command would run unowned and unbudgeted — and a
// runtime would be entitled to claim the queued Job in the same instant, giving one
// transfer two executors. The command therefore only records the release: it queues the
// Job, and the executor starts inside a fresh claim with that claim's token and the
// deployment's budget.
func TestAResumeQueuesWorkRatherThanStartingAnUnbudgetedTransfer(t *testing.T) {
	ctx := newDownloadJobContextWithBudget(t, 1)

	server, requests, unblock := heldTransferServer(t)
	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/held.bin"}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil || submissions[0].Job == nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	handle := submissions[0].Row.ID

	if snap := waitForSnapshot(t, ctx, jobID, "the transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	}); snap.State != jobs.StateRunning {
		t.Fatalf("the transfer is %s, want running", snap.State)
	}

	// A person holds it. The queue's own pause is the executor's side of that, and the
	// durable Job is blocked once the mirror has recorded it.
	if err := ctx.DownloadManager().Pause(handle); err != nil {
		t.Fatalf("pause the transfer: %v", err)
	}
	held := waitForSnapshot(t, ctx, jobID, "the hold to be recorded", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateBlocked
	})
	if storedCapacity(t, ctx, jobs.CapacityGroupGlobal) != 0 {
		t.Fatalf("a held transfer still occupies the deployment budget")
	}

	// The deployment's one slot is taken by something else, so there is no room for the
	// transfer to start again in.
	holder := holdTheDeploymentBudgetIn(t, ctx)
	unblock() // the held transfer's own request may return; it is paused, so nothing reads it

	before := requests.Load()
	result, err := ctx.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: jobID, Key: jobs.CommandResume, IdempotencyKey: "resume-unbudgeted",
		ExpectedVersion: held.Version,
	})
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	if result.Status != jobs.CommandStatusSucceeded {
		t.Fatalf("the resume answered %s: %s", result.Status, result.Message)
	}

	// Queued, and running nowhere: no worker was started by the command.
	if snap := jobSnapshot(t, ctx.JobService(), ctx, jobID); snap.State != jobs.StateQueued {
		t.Fatalf("the resumed job is %s, want queued for a claim", snap.State)
	}
	if entry, found := ctx.DownloadManager().GetJob(handle); !found || entry.GetStatus() != download_queue.JobStatusPaused {
		t.Fatalf("the resume started an executor directly: the queue entry is %v", entry)
	}
	if got := requests.Load(); got != before {
		t.Fatalf("the resumed transfer fetched %d more times before any claim admitted it", got-before)
	}
	if held := storedCapacity(t, ctx, jobs.CapacityGroupGlobal); held != 1 {
		t.Fatalf("the deployment budget holds %d slots, want only the other execution's one", held)
	}

	// The slot frees, a runtime claims the queued Job with its capacity, and the paused
	// entry is resumed inside that claim.
	if _, err := ctx.JobService().ReleaseClaim(ctx.jobDeps(), jobs.ReleaseRequest{
		ExecutionRef: jobs.ExecutionRef{JobID: holder.JobID, ExecutionToken: holder.ExecutionToken},
		To:           jobs.StateQueued,
		Reason:       "test released the budget",
	}); err != nil {
		t.Fatalf("release the budget holder: %v", err)
	}

	finished := waitForSnapshot(t, ctx, jobID, "the resumed transfer to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the resumed transfer ended %s (%+v)", finished.State, finished.Failure)
	}
	if got := requests.Load(); got != before+1 {
		t.Fatalf("the resumed transfer fetched %d times, want once: %d", got-before, got)
	}
	if storedCapacity(t, ctx, jobs.CapacityGroupGlobal) != 0 {
		t.Fatalf("the deployment budget still holds slots after the resumed transfer ended")
	}
}

// newDownloadJobContextWithBudget is the download harness with a deployment budget and
// a running loop.
//
// The budget has to be the deployment's own *before* the runtime is built — a runtime
// reads it once, at construction — and the loop has to exist for the second half of the
// admission contract: work the command only queued is claimed by whatever process has a
// free slot.
func newDownloadJobContextWithBudget(t *testing.T, budget int) *MahresourcesContext {
	t.Helper()
	ctx := newJobHarnessContext(t, false)
	ctx.Config.MaxJobConcurrency = budget
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{
		Claimant: "download-budget-test",
		Interval: 25 * time.Millisecond,
	})
	runtime.Start()
	t.Cleanup(runtime.Stop)
	return ctx
}

// TestACancellationRecordedByAnotherProcessStopsTheTransferItOwns is the delivery half
// of §4's cancellation, across the gap a process boundary opens.
//
// The intent is durable, so it survives an executor that stops answering — but
// recording it is only half the contract. A cancellation routed to a runtime that does
// not hold the transfer reaches no executor at all: the process running the work keeps
// running it, and its eventual success is refused by an intent it never saw, which
// leaves the Job nonterminal and the side effects already made. The execution reads the
// intent for itself, and that is what this test drives: one context submits and runs,
// another cancels, and nothing carries the request between them but the row.
func TestACancellationRecordedByAnotherProcessStopsTheTransferItOwns(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.Config.MaxJobConcurrency = 2
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, _ := newSecondProcessJobContext(t, first, key)

	server, requests, unblock := heldTransferServer(t)
	submissions := first.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/held.bin"}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil || submissions[0].Job == nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	handle := submissions[0].Row.ID

	if snap := waitForSnapshot(t, first, jobID, "the transfer to start", func(s jobs.Snapshot) bool {
		return s.State == jobs.StateRunning
	}); snap.State != jobs.StateRunning {
		t.Fatalf("the transfer is %s, want running", snap.State)
	}
	// The transfer is genuinely in flight before anything is cancelled: the Job is
	// running from its claim, and the request is what the server is holding.
	waitFor(t, "the transfer's request to reach the server", func() bool { return requests.Load() >= 1 })

	// The other process cancels. It holds no queue entry for the transfer, so the
	// command records the intent and answers; nothing it can do stops the transfer.
	result, err := other.ExecuteJobCommand(context.Background(), jobs.CommandRequest{
		JobID: jobID, Key: jobs.CommandCancel, IdempotencyKey: "cross-process-cancel",
		ExpectedVersion: jobSnapshot(t, first.JobService(), first, jobID).Version,
	})
	if err != nil {
		t.Fatalf("the other process refused to cancel: %v", err)
	}
	if result.Status != jobs.CommandStatusSucceeded {
		t.Fatalf("the cancellation answered %s: %s", result.Status, result.Message)
	}
	if snap := jobSnapshot(t, first.JobService(), first, jobID); snap.ControlIntent != jobs.ControlIntentCancel {
		t.Fatalf("the cancellation was not recorded durably: intent %q", snap.ControlIntent)
	}

	// The transfer is stopped while the server is still holding the response, so it
	// cannot have reached a terminal status on its own: only the owning execution
	// reading the intent can explain it.
	cancelled := waitForSnapshot(t, first, jobID, "the owning execution to stop its transfer", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if cancelled.State != jobs.StateCancelled {
		t.Fatalf("a cancelled transfer ended %s (%+v), want cancelled", cancelled.State, cancelled.Failure)
	}
	if entry, found := first.DownloadManager().GetJob(handle); !found || entry.GetStatus() != download_queue.JobStatusCancelled {
		t.Fatalf("the transfer's queue entry is %v after the cancellation, want cancelled", entry)
	}

	unblock()
	time.Sleep(200 * time.Millisecond)
	if got := requests.Load(); got != 1 {
		t.Fatalf("the transfer was fetched %d times, want the one attempt it stopped", got)
	}
}
