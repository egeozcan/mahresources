package download_queue

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mahresources/contracts"
	"mahresources/hls"
	"mahresources/models"
	"mahresources/models/query_models"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	MaxConcurrentDownloads = 3
	MaxQueueSize           = 100
	// ShutdownDrainTimeout bounds how long Shutdown waits for in-flight download
	// workers to stamp and record their (cancelled) terminal state.
	ShutdownDrainTimeout = 5 * time.Second

	JobRetentionDuration       = 1 * time.Hour
	PausedJobRetentionDuration = 24 * time.Hour
	MaxResourceNameLength      = 1000
)

// ManagerConfig controls runtime parameters of the DownloadManager. Zero
// values fall back to the package constants MaxConcurrentDownloads and
// JobRetentionDuration. Export retention is now part of DownloadSettings so
// it can be updated at runtime without a restart.
type ManagerConfig struct {
	Concurrency  int           // max concurrent jobs across all sources
	JobRetention time.Duration // how long completed/failed jobs linger in memory

	// ClientPolicy decorates the HTTP client each download uses, so the queue
	// fetches only where the deployment permits. A download URL is user-supplied
	// and fetched server-side, which is a server-side request forgery primitive
	// until something says where the server may go.
	//
	// Injected rather than imported. The policy and its address classifier live
	// beside the plugin egress work that wrote them, and this package sits below
	// the layer that knows about either — the same shape as HistoryRecorder,
	// declared here and implemented above. nil leaves the client undecorated,
	// which is what the tests and any embedder that has no policy get.
	//
	// The connect timeout is passed per call rather than captured, because the
	// decoration replaces the client's dialler and that dialler carries the
	// timeout. Reading a startup value there while TLSHandshakeTimeout and
	// ResponseHeaderTimeout beside it still tracked the live runtime setting
	// left `remote_connect_timeout` half-applied — worse than not applying,
	// because the symptom does not point at the cause.
	ClientPolicy func(client *http.Client, connectTimeout time.Duration) *http.Client

	// FfmpegPath and HLSOptions are what an HLS download needs, injected for
	// the same reason ClientPolicy is: this package sits below the layer that
	// reads configuration. An empty FfmpegPath makes an HLS URL fail with a
	// message naming ffmpeg rather than store the playlist text as the video,
	// which is what a deployment without ffmpeg should hear.
	FfmpegPath string
	HLSOptions hls.Options

	// RefusalMessage renders a policy refusal for the job's error field, which
	// its submitter reads. It exists so the address a hostname resolved to does
	// not travel to whoever submitted the URL; ok is false for any error that is
	// not a refusal. nil passes errors through unchanged.
	//
	// It takes the URL, and is the place the unsanitized error is logged: this
	// package has no logger, and the operator half of a refusal has to go
	// somewhere or "we tell the operator instead" is a claim with nothing behind
	// it — an operator debugging a refused internal download would have no way
	// to learn which address was refused.
	RefusalMessage func(url string, err error) (msg string, ok bool)
}

// ResourceCreator is the interface needed to create resources
// This avoids a circular dependency with application_context
type ResourceCreator interface {
	AddResource(file contracts.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error)
}

// actorResourceCreator is the optional capability (implemented by
// *application_context.MahresourcesContext, not by test doubles) that binds a
// download's submitter as the create actor, so CreatedByUserId is stamped on the
// resource and its initial version. Reached via a type assertion in the worker,
// so plain ResourceCreator mocks are unaffected.
type actorResourceCreator interface {
	WithActorUserID(userID uint) ResourceCreator
}

// TimeoutConfig holds timeout settings for remote downloads
type TimeoutConfig struct {
	ConnectTimeout time.Duration
	IdleTimeout    time.Duration
	OverallTimeout time.Duration
}

// DownloadSettings is the runtime configuration surface for the download
// manager. Reads are called per download start so runtime changes take effect
// without a restart. See application_context.RuntimeSettings.
type DownloadSettings interface {
	ConnectTimeout() time.Duration
	IdleTimeout() time.Duration
	OverallTimeout() time.Duration
	ExportRetention() time.Duration
}

// NewStaticDownloadSettings returns a DownloadSettings whose values never
// change. Used by tests and by the legacy NewDownloadManager constructor.
func NewStaticDownloadSettings(tc TimeoutConfig, exportRetention time.Duration) DownloadSettings {
	return staticDownloadSettings{tc: tc, er: exportRetention}
}

type staticDownloadSettings struct {
	tc TimeoutConfig
	er time.Duration
}

func (s staticDownloadSettings) ConnectTimeout() time.Duration  { return s.tc.ConnectTimeout }
func (s staticDownloadSettings) IdleTimeout() time.Duration     { return s.tc.IdleTimeout }
func (s staticDownloadSettings) OverallTimeout() time.Duration  { return s.tc.OverallTimeout }
func (s staticDownloadSettings) ExportRetention() time.Duration { return s.er }

// DownloadManager manages background download jobs.
// Concurrency discipline: settings is written under mu.Lock (SetSettings,
// constructor) and read under mu.RLock (currentSettings). All other mu-guarded
// fields follow the same pattern.
type DownloadManager struct {
	mu          sync.RWMutex
	jobs        map[string]*DownloadJob
	jobOrder    []string // Maintains insertion order
	resourceCtx ResourceCreator
	ffmpegPath  string
	hlsOptions  hls.Options

	// policyResolver answers "which egress policy does this plugin fetch under".
	//
	// It is set after construction rather than through ManagerConfig, because
	// this manager is built while the context is still assembling itself and the
	// plugin manager does not exist yet — a closure captured here would capture
	// nil. Same shape as SetKVStore, and the same doctrine: unset must fail
	// closed. A plugin download with no resolver is refused, never fetched under
	// the host's wider policy.
	policyResolver atomic.Value
	settings       DownloadSettings
	semaphore      chan struct{}
	// workers counts the download goroutines in flight, so Shutdown can wait for
	// their terminal history writes rather than exiting under them.
	workers       sync.WaitGroup
	subscribers   map[chan JobEvent]struct{}
	subscribersMu sync.RWMutex
	// notifyMu orders snapshot-and-publish against itself, so the sequence
	// subscribers see is the sequence the snapshots were taken in. See notifyJob.
	// Held across a Snapshot and a set of non-blocking sends, and never while any
	// other lock is being acquired in the other direction: no job method reaches the
	// manager, and no caller holds a job's mutex across a notification.
	notifyMu      sync.Mutex
	cleanupTicker *time.Ticker
	done          chan struct{}
	concurrency   int
	jobRetention  time.Duration
	// clientPolicy and refusalMessage carry the deployment's egress policy into
	// each transfer. Both are set once at construction and never reassigned, so
	// they are read without the mutex. See ManagerConfig.
	clientPolicy   func(*http.Client, time.Duration) *http.Client
	refusalMessage func(string, error) (string, bool)
	exportSweepFn  func() // called by cleanupOldJobs to sweep expired export tars from disk
	// history is the durable store for terminal downloads (see history.go). Optional:
	// nil means the manager keeps its pre-history behaviour, which is what the CLI's
	// and the tests' bare managers rely on.
	history        HistoryRecorder
	historyLog     HistoryLogger
	historySweepFn func() // called by cleanupOldJobs to delete expired history rows
	// jobEventState carries the optional terminal-job observer (see
	// job_events.go). Nil sink means every emit is a nil check, which is what a
	// deployment with no plugin listening gets.
	jobEventState
}

// NewDownloadManagerWithConfig constructs a DownloadManager with the given
// runtime config. Zero-value Concurrency and JobRetention fall back to the
// package constants so existing call sites that don't care stay simple.
// The settings provider is called per download start so runtime changes take
// effect without a restart.
func NewDownloadManagerWithConfig(resourceCtx ResourceCreator, settings DownloadSettings, cfg ManagerConfig) *DownloadManager {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = MaxConcurrentDownloads
	}
	if cfg.JobRetention <= 0 {
		cfg.JobRetention = JobRetentionDuration
	}
	dm := &DownloadManager{
		jobs:           make(map[string]*DownloadJob),
		jobOrder:       make([]string, 0),
		resourceCtx:    resourceCtx,
		settings:       settings,
		semaphore:      make(chan struct{}, cfg.Concurrency),
		subscribers:    make(map[chan JobEvent]struct{}),
		done:           make(chan struct{}),
		concurrency:    cfg.Concurrency,
		jobRetention:   cfg.JobRetention,
		clientPolicy:   cfg.ClientPolicy,
		ffmpegPath:     cfg.FfmpegPath,
		hlsOptions:     cfg.HLSOptions,
		refusalMessage: cfg.RefusalMessage,
	}

	// Start cleanup goroutine
	dm.cleanupTicker = time.NewTicker(5 * time.Minute)
	go dm.cleanupLoop()

	return dm
}

// NewDownloadManager is the legacy constructor. Kept as a thin wrapper so
// existing callers that don't care about concurrency/retention tuning still
// work. Delegates to NewDownloadManagerWithConfig with a static settings provider.
func NewDownloadManager(resourceCtx ResourceCreator, tc TimeoutConfig) *DownloadManager {
	return NewDownloadManagerWithConfig(resourceCtx, NewStaticDownloadSettings(tc, 0), ManagerConfig{})
}

// currentSettings returns the active DownloadSettings provider under a
// read-lock. Callers should cache the result for the duration of a single
// download start to avoid repeated lock acquisitions on the hot path.
func (m *DownloadManager) currentSettings() DownloadSettings {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.settings
}

// SetSettings replaces the DownloadSettings provider. Used by main.go to wire
// the live runtime-settings service after NewMahresourcesContext has already
// initialized the manager with a static provider.
func (m *DownloadManager) SetSettings(settings DownloadSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = settings
}

// ExportRetention returns how long completed group-export tars should
// linger on disk before the sweep deletes them.
func (m *DownloadManager) ExportRetention() time.Duration {
	return m.currentSettings().ExportRetention()
}

// SetExportSweepFn registers a function that cleanupOldJobs will call
// periodically to sweep expired export tars from disk. Called by
// application_context during initialization.
func (m *DownloadManager) SetExportSweepFn(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exportSweepFn = fn
}

// generateShortID creates a short random ID for display.
//
// Eight bytes, not four. A job id used to live only in memory, where a handful of
// concurrent jobs made 32 bits plenty — but it is now the unique key of the
// durable history row, and history keeps a week of them. Collisions there are not
// cosmetic: the upsert would merge two unrelated downloads into one row, and
// because the row keeps the first submitter as its owner, the second user would
// lose their download while the first inherited its URL, its error and its
// retryable payload. Sixty-four bits puts that beyond reach for any table this
// application will hold.
func generateShortID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails.
		return fmt.Sprintf("%016x", uint64(time.Now().UnixNano()))
	}
	return hex.EncodeToString(b)
}

func trimResourceName(name string) string {
	if len(name) <= MaxResourceNameLength {
		return name
	}

	ext := path.Ext(name)
	if ext != "" && len(ext) < MaxResourceNameLength {
		stemLimit := MaxResourceNameLength - len(ext)
		return trimStringBytes(name[:len(name)-len(ext)], stemLimit) + ext
	}

	return trimStringBytes(name, MaxResourceNameLength)
}

func trimStringBytes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	for i := range value {
		if i >= limit {
			return value[:i]
		}
	}
	return value[:limit]
}

// makeRoomForNewJob evicts old jobs to make space for a new one.
// Priority: completed (oldest first), then failed/cancelled (oldest first).
// Never evicts active (pending/downloading/processing) or paused jobs.
// Must be called with dm.mu held.
func (dm *DownloadManager) makeRoomForNewJob() bool {
	if len(dm.jobs) < MaxQueueSize {
		return true // Already have room
	}

	// First pass: find oldest completed job
	for _, id := range dm.jobOrder {
		job := dm.jobs[id]
		if job.GetStatus() == JobStatusCompleted {
			dm.evictJob(id, job)
			return true
		}
	}

	// Second pass: find oldest failed/cancelled job
	for _, id := range dm.jobOrder {
		job := dm.jobs[id]
		status := job.GetStatus()
		if status == JobStatusFailed || status == JobStatusCancelled {
			dm.evictJob(id, job)
			return true
		}
	}

	// No evictable jobs found (all are active or paused)
	return false
}

// evictJob removes a job from the queue. Must be called with dm.mu held.
func (dm *DownloadManager) evictJob(id string, job *DownloadJob) {
	delete(dm.jobs, id)

	// Remove from jobOrder
	newOrder := make([]string, 0, len(dm.jobOrder)-1)
	for _, oid := range dm.jobOrder {
		if oid != id {
			newOrder = append(newOrder, oid)
		}
	}
	dm.jobOrder = newOrder

	dm.notifyJob("removed", job)
}

// Submit adds a new download job to the queue
// Submit enqueues a background download. ownerUserID is the submitting user (nil
// for the auth-off super-user / unowned jobs); it is set on the job at
// construction — before the processing goroutine starts — so the worker can
// attribute the created resource (CreatedByUserId) to the submitter without a
// race, and so queue-visibility RBAC is applied before processing.
func (dm *DownloadManager) Submit(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint) (*DownloadJob, error) {
	return dm.SubmitForPlugin(creator, ownerUserID, "")
}

// SubmitForPlugin enqueues a download on a plugin's behalf.
//
// pluginName is not decoration: it selects the egress policy every fetch this
// job makes runs under. A plugin's declared network list is narrower than the
// host policy, which allows any public host, so a plugin download that fell
// back to the host policy would reach places the operator consented to nothing
// about. An empty name is a person's download and takes the host policy, which
// is what Submit passes.
func (dm *DownloadManager) SubmitForPlugin(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName string) (*DownloadJob, error) {
	dm.mu.Lock()

	if !dm.makeRoomForNewJob() {
		dm.mu.Unlock()
		return nil, fmt.Errorf("download queue is full (max %d jobs) - all jobs are active or paused", MaxQueueSize)
	}

	ctx, cancel := context.WithCancel(context.Background())

	job := &DownloadJob{
		ID:              generateShortID(),
		URL:             strings.TrimSpace(creator.URL),
		Status:          JobStatusPending,
		Progress:        0,
		TotalSize:       -1,
		ProgressPercent: -1,
		CreatedAt:       time.Now(),
		Source:          "download",
		creator:         creator,
		ctx:             ctx,
		cancel:          cancel,
		ownerUserID:     ownerUserID,
		pluginName:      pluginName,
	}

	dm.jobs[job.ID] = job
	dm.jobOrder = append(dm.jobOrder, job.ID)

	// "added" goes out before the worker exists, and while the registry lock is still
	// held, so nothing can describe this job to a subscriber that has not been told it
	// exists. The go statement used to come first: the worker could claim its start
	// and broadcast "updated" before this line ran, and the panel applies "updated" by
	// looking the row up — so it dropped the real status on the floor and then drew
	// the row from the late "added". Live pointers hid it, because the late event
	// marshalled the job's *current* state; snapshots do not, which is why the
	// ordering and the snapshot had to be fixed together.
	dm.notifyJob("added", job)

	dm.mu.Unlock()

	// The go statement happens-before the goroutine's execution, so ownerUserID (set
	// above) is visible to the worker.
	dm.startDownloadWorker(job)

	return job, nil
}

// SubmitMultiple submits multiple URLs (newline-separated) as individual jobs,
// each owned by ownerUserID (see Submit).
func (dm *DownloadManager) SubmitMultiple(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint) ([]*DownloadJob, error) {
	urls := strings.Split(creator.URL, "\n")
	var jobs []*DownloadJob

	for _, url := range urls {
		url = strings.TrimSpace(url)
		if url == "" {
			continue
		}

		// Create a copy of the creator for each URL
		singleCreator := *creator
		singleCreator.URL = url

		job, err := dm.Submit(&singleCreator, ownerUserID)
		if err != nil {
			// If queue is full, return what we have so far
			if len(jobs) > 0 {
				return jobs, err
			}
			return nil, err
		}
		jobs = append(jobs, job)
	}

	if len(jobs) == 0 {
		return nil, fmt.Errorf("no valid URLs provided")
	}

	return jobs, nil
}

// processJob handles the download in a background goroutine
// startDownloadWorker runs a download and registers it with the drain.
//
// The counter is incremented here, before the goroutine exists, so Shutdown
// cannot miss a worker that has been decided on but not yet scheduled. Wrapping
// rather than counting inside processJob keeps the tests that drive processJob
// directly working — and keeps the pairing visible in one place.
func (dm *DownloadManager) startDownloadWorker(job *DownloadJob) {
	dm.workers.Add(1)
	go func() {
		defer dm.workers.Done()
		dm.processJob(job)
	}()
}

func (dm *DownloadManager) processJob(job *DownloadJob) {

	// The attempt this goroutine owns, and the context its result must be judged
	// against. Both are read once: Resume installs a fresh context, so a worker that
	// asked the job for "the" context after unwinding could classify its own failure
	// against a *later* attempt's live context — and report `failed` for a download
	// its own pause had cancelled.
	runID, ctx := job.attempt()

	// Acquire semaphore slot (limits concurrent downloads)
	select {
	case dm.semaphore <- struct{}{}:
		defer func() { <-dm.semaphore }()
	case <-ctx.Done():
		// finish is a no-op on a paused job: Pause cancels the context too, and a
		// paused job waits for Resume rather than being retired here.
		if snap, stamped := job.finishSnapshot(runID, JobStatusCancelled, "Cancelled before starting", 0, time.Now()); stamped {
			dm.notifyJob("updated", job)
			// Recorded like any other terminal state. A download cancelled while it
			// was still queued is exactly the kind the user wants to find later and
			// press Retry on — leaving it out made the queue's own eviction the only
			// record of it, which is what history exists to outlive.
			dm.recordTerminal(job, snap)
			dm.emitJobEvent(job, snap)
		}
		return
	}

	// The forward transition is claimed too, or a Pause landing in the moment
	// between the semaphore acquisition and this write would be overwritten by it
	// (see DownloadJob.claimStart). A refusal means a control owns the job now and
	// has already notified subscribers.
	if !job.claimStart(runID, JobStatusDownloading, time.Now()) {
		return
	}
	dm.notifyJob("updated", job)

	// Perform the download with progress tracking
	resource, err := dm.downloadWithProgress(ctx, runID, job)

	status, errMsg, resourceID := JobStatusCompleted, "", uint(0)
	switch {
	case err != nil && ctx.Err() != nil:
		status, errMsg = JobStatusCancelled, "Download cancelled"
	case err != nil:
		status, errMsg = JobStatusFailed, err.Error()
	default:
		// Deliberately not overridden by an accepted cancel: the resource exists and
		// the version row is written, so reporting `cancelled` here would orphan a
		// file the user can see. Cancellation of a download is best-effort, and one
		// that lands after AddResource has returned has landed too late.
		resourceID = resource.ID
	}

	// One atomic terminal write. Reading the status to see whether the job was paused
	// and then writing the terminal one is the same check-then-act the controls had:
	// a Pause landing between the two was silently overwritten, and a Pause landing
	// just before the read stranded the job (see DownloadJob.finish).
	snap, stamped := job.finishSnapshot(runID, status, errMsg, resourceID, time.Now())
	if !stamped {
		return
	}

	dm.notifyJob("updated", job)

	// Persisted after the state is stamped and outside every lock, but *from* the
	// capture the stamp took under it — a Retry landing between the two would
	// otherwise be what the row described. The other two terminal writes (the
	// cancelled-while-queued branch above, and Cancel's paused branch) record the
	// same way.
	dm.recordTerminal(job, snap)
	dm.emitJobEvent(job, snap)
}

// createHTTPClient creates an HTTP client with context support.
// settings is snapshotted by the caller (downloadWithProgress) so timeout
// values reflect the live runtime configuration at the moment the download
// starts.
// createHTTPClient builds the client one transfer uses, under the policy its
// origin calls for.
//
// pluginName selects that policy and the selection can fail, which is why this
// returns an error rather than a client: a plugin whose policy cannot be
// resolved — it was disabled, renamed, or the resolver was never wired — must
// have its download refused, not fetched under the host's wider policy. Failing
// open here is the confused deputy this whole seam exists to prevent.
func (dm *DownloadManager) createHTTPClientFor(s DownloadSettings, pluginName string) (*http.Client, error) {
	if pluginName == "" {
		return dm.createHTTPClient(s), nil
	}
	resolve := dm.currentPolicyResolver()
	if resolve == nil {
		return nil, fmt.Errorf("refusing to fetch: this deployment cannot resolve plugin %q's network policy", pluginName)
	}
	policy, ok := resolve(pluginName)
	if !ok || policy == nil {
		return nil, fmt.Errorf("refusing to fetch: plugin %q's network policy is not available (is the plugin still enabled?)", pluginName)
	}
	client := dm.baseHTTPClient(s)
	return policy(client, s.ConnectTimeout()), nil
}

// PolicyResolver answers which client decoration a named plugin's fetches run
// under. ok is false when the plugin is unknown or no longer enabled, which is
// a refusal rather than a reason to fall back.
type PolicyResolver func(pluginName string) (policy func(*http.Client, time.Duration) *http.Client, ok bool)

// SetPolicyResolver wires the plugin egress lookup, after the plugin manager
// exists. See DownloadManager.policyResolver.
func (dm *DownloadManager) SetPolicyResolver(r PolicyResolver) {
	if r != nil {
		dm.policyResolver.Store(r)
	}
}

func (dm *DownloadManager) currentPolicyResolver() PolicyResolver {
	v := dm.policyResolver.Load()
	if v == nil {
		return nil
	}
	return v.(PolicyResolver)
}

// baseHTTPClient is the undecorated client one transfer uses. Built per
// download rather than pooled: a policy replaces the dialler, so a client that
// outlived one transfer could serve a connection opened under a policy that no
// longer applies.
func (dm *DownloadManager) baseHTTPClient(s DownloadSettings) *http.Client {
	return &http.Client{
		Timeout: s.OverallTimeout(),
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: s.ConnectTimeout()}).DialContext,
			TLSHandshakeTimeout:   s.ConnectTimeout() / 2,
			ResponseHeaderTimeout: s.ConnectTimeout(),
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

func (dm *DownloadManager) createHTTPClient(s DownloadSettings) *http.Client {
	client := dm.baseHTTPClient(s)
	// Per download, and after the transport is built: the policy replaces the
	// dialler, so decorating a client that outlived one transfer would let a
	// pooled connection opened under one policy serve another.
	if dm.clientPolicy != nil {
		client = dm.clientPolicy(client, s.ConnectTimeout())
	}
	return client
}

// assembleHLS downloads a streaming playlist and returns the single file it
// assembles to.
//
// The client is the one this transfer already built, decorated with the
// deployment's egress policy. Passing it on is the whole point: a playlist
// names further URLs, and every one of them is fetched under the same policy
// the submitted URL was. ffmpeg is handed local files only and cannot open a
// socket.
//
// Progress is reported through the job's phase counters rather than its byte
// counters, because the size of an HLS stream is not known until every segment
// has been fetched -- TotalSize stays -1, which is what the queue already
// means by "unknown".
func (dm *DownloadManager) assembleHLS(ctx context.Context, runID uint64, job *DownloadJob, client *http.Client, head []byte, body io.Reader) (*hls.Result, error) {
	var lastNotify time.Time
	result, err := hls.Fetch(ctx, hls.Deps{Client: client, FfmpegPath: dm.ffmpegPath}, job.URL, head, body, dm.hlsOptions,
		func(phase string, done, total int64) {
			job.SetPhase(phase)
			job.SetPhaseProgress(done, total)
			// Throttled like the byte progress, and for the same reason: a
			// four-hundred-segment stream would otherwise be four hundred SSE
			// events. A phase change is always sent, since those are rare and
			// are the part a watcher is actually waiting for.
			if done == 0 || time.Since(lastNotify) >= progressNotifyInterval {
				lastNotify = time.Now()
				dm.notifyJob("updated", job)
			}
		})
	if err != nil {
		return nil, dm.describeFetchError(job.URL, err)
	}

	// The transfer is over by the time Fetch returns, so the job moves to
	// `processing` here rather than at an EOF the progress reader would have
	// seen. Same order as attemptReporter.onComplete: status first, then one
	// notification, never two for one transition.
	job.updateProgressForRun(runID, result.Size, result.Size)
	if job.setStatusForRun(runID, JobStatusProcessing) {
		dm.notifyJob("updated", job)
	}
	return result, nil
}

// describeFetchError renders a transfer failure for the job's error field.
//
// The submitter reads that field. A dial refusal's own text names the address
// the URL resolved to — Go wraps it in *net.OpError, whose prefix carries the
// address whatever we do to our own message — so a refusal is replaced rather
// than annotated. Everything else passes through unchanged.
func (dm *DownloadManager) describeFetchError(url string, err error) error {
	if dm.refusalMessage == nil || err == nil {
		return err
	}
	if msg, ok := dm.refusalMessage(url, err); ok {
		return errors.New(msg)
	}
	return err
}

// attemptReporter is the pair of callbacks one download attempt reports through.
//
// It is a named type and not two closures because that is what makes the guard
// below testable: what has to hold is that a write from an attempt which no longer
// owns the job is dropped, and driving that through a live HTTP transfer means
// landing a control inside the few instructions between EOF and the status write.
// As a type, the same production code the transfer uses can be handed a job a
// control has already taken and asked what it does.
type attemptReporter struct {
	dm    *DownloadManager
	job   *DownloadJob
	runID uint64
	total int64

	// Only ever read and written by the single goroutine performing the reads, which
	// is the one AddResource runs on.
	lastNotify time.Time
}

// progressNotifyInterval throttles per-chunk updates so a fast transfer does not
// flood every SSE client.
const progressNotifyInterval = 500 * time.Millisecond

// onProgress is called on each chunk read.
func (r *attemptReporter) onProgress(downloaded int64) {
	if !r.job.updateProgressForRun(r.runID, downloaded, r.total) {
		return
	}
	if time.Since(r.lastNotify) >= progressNotifyInterval {
		r.lastNotify = time.Now()
		r.dm.notifyJob("updated", r.job)
	}
}

// onComplete is called when the transfer reaches EOF: the bytes are all in, and
// what remains is writing the resource.
//
// The status change comes first and the notification second, which is the order the
// rest of the manager uses and the order this did not: it flushed the final progress
// under the *old* status, then wrote `processing`, then notified again. Two events
// for one transition, the first of them describing a state the job had already left.
func (r *attemptReporter) onComplete() {
	if !r.job.setStatusForRun(r.runID, JobStatusProcessing) {
		return
	}
	r.dm.notifyJob("updated", r.job)
}

// downloadWithProgress performs the HTTP download with progress tracking.
//
// runID is the attempt this download belongs to. Everything it reports about the
// job is stamped with it, so a control that takes the job away mid-transfer — or a
// Resume that starts a second attempt beside this one — is not overwritten by
// callbacks still unwinding.
func (dm *DownloadManager) downloadWithProgress(ctx context.Context, runID uint64, job *DownloadJob) (*models.Resource, error) {
	// Snapshot settings once so all timeout values are consistent for this
	// download and the read-lock is held only briefly.
	s := dm.currentSettings()
	httpClient, err := dm.createHTTPClientFor(s, job.PluginName())
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", job.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, dm.describeFetchError(job.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Get content length if available
	contentLength := resp.ContentLength
	if job.updateProgressForRun(runID, 0, contentLength) {
		dm.notifyJob("updated", job)
	}

	// Wrap with timeout reader for idle detection and cancellation
	timeoutBody := NewTimeoutReaderWithContext(resp.Body, s.IdleTimeout(), ctx)
	defer timeoutBody.Close()

	// An HLS playlist is a list of the media, not the media, so storing what
	// arrived would file a few kilobytes of text as the video. Recognised from
	// the bytes rather than the URL: the endpoints this matters for carry
	// neither an .m3u8 extension nor a playlist content type.
	//
	// The sniff reads off the body, so the non-playlist path has to put those
	// bytes back before the progress reader sees them -- otherwise the first
	// sixty-four bytes of every download are silently dropped.
	head := make([]byte, hls.SniffLen())
	sniffed, _ := io.ReadFull(timeoutBody, head)
	head = head[:sniffed]

	var progressBody contracts.File
	assembledHLS := false
	if hls.IsPlaylist(head, resp.Header.Get("Content-Type"), job.URL) {
		assembled, hlsErr := dm.assembleHLS(ctx, runID, job, httpClient, head, timeoutBody)
		if hlsErr != nil {
			return nil, hlsErr
		}
		defer assembled.Cleanup()
		defer assembled.Body.Close()
		progressBody = io.NopCloser(assembled.Body)
		assembledHLS = true
	} else {
		// Wrap with progress reader
		reporter := &attemptReporter{dm: dm, job: job, runID: runID, total: contentLength}
		progressBody = NewProgressReader(io.MultiReader(bytes.NewReader(head), timeoutBody), reporter.onProgress, reporter.onComplete)
	}

	// The stored file name and the resource's display name are different things,
	// and conflating them lost the Name the user typed: the worker built one
	// value from FileName -> path.Base(URL) and used it for both, never
	// consulting creator.Name. AddRemoteResource (the foreground path) has
	// always preferred creator.Name, so this mirrors it.
	fileName := job.creator.FileName
	if fileName == "" {
		fileName = path.Base(job.URL)
	}
	fileName = trimResourceName(fileName)

	name := trimResourceName(job.creator.Name)
	if name == "" {
		name = fileName
	}

	// The names follow the bytes, which are MP4 whatever the URL said: an
	// assembled video still called "index.m3u8" misdescribes itself everywhere
	// it is listed, served or downloaded again. Applied to the local values
	// rather than to job.creator, which is the payload the history row stores
	// and a retry replays.
	if assembledHLS {
		fileName = hls.OutputName(fileName)
		name = hls.OutputName(name)
	}

	// Use existing AddResource logic
	originalName := job.creator.OriginalName
	if originalName == "" {
		originalName = job.URL
	}
	originalLocation := job.creator.OriginalLocation
	if originalLocation == "" {
		originalLocation = job.URL
	}

	// Attribute the created resource (and its initial version) to the submitting
	// user. Background jobs run on the unscoped singleton context, which would
	// otherwise stamp CreatedByUserId NULL under auth-on. The submit handlers
	// already validate scope targets at enqueue time, so binding only the actor
	// (not a scope filter) preserves the worker's intentional unscoped creation.
	// Under no-auth ownerUserID is nil and the stamp callback's default actor
	// (root) applies instead.
	creator := dm.resourceCtx
	if oid := job.GetOwnerUserID(); oid != nil && *oid != 0 {
		if binder, ok := dm.resourceCtx.(actorResourceCreator); ok {
			creator = binder.WithActorUserID(*oid)
		}
	}

	return creator.AddResource(progressBody, fileName, &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:               name,
			Description:        job.creator.Description,
			OwnerId:            job.creator.OwnerId,
			Groups:             job.creator.Groups,
			Tags:               job.creator.Tags,
			Notes:              job.creator.Notes,
			Meta:               job.creator.Meta,
			ContentCategory:    job.creator.ContentCategory,
			Category:           job.creator.Category,
			ResourceCategoryId: job.creator.ResourceCategoryId,
			OriginalName:       originalName,
			OriginalLocation:   originalLocation,
			Width:              job.creator.Width,
			Height:             job.creator.Height,
			SeriesSlug:         job.creator.SeriesSlug,
			SeriesId:           job.creator.SeriesId,
		},
		// BH-023: same omission as the foreground path. It matters more here,
		// because the submitted creator is persisted as the history payload and
		// replayed on retry -- dropping it only here would have made a retried
		// download land somewhere other than the original.
		PathName: job.creator.PathName,
	})
}

// Cancel cancels a download job by ID.
//
// UI bug hunt 2026-07-29, finding 2: this gated on IsActive(), which excludes
// `paused`, so a paused download could never be abandoned — the answer was
// "job %s already finished" and the handler turned that into a 404. A paused job
// is cancellable now, and the two refusals are typed so the handler can tell a
// missing job from a state conflict.
func (dm *DownloadManager) Cancel(jobID string) error {
	job, err := dm.lookup(jobID)
	if err != nil {
		return err
	}

	// One atomic step: whether the job may be cancelled, whether it was paused, and
	// the terminal write the paused case needs are all decided under the job's own
	// lock. Deciding from a second status read is what let a concurrent Pause land in
	// between and overwrite an accepted cancellation — see DownloadJob.claimCancel.
	prev, snap, ok := job.claimCancel(time.Now())
	if !ok {
		return &StateConflictError{JobID: jobID, Action: "cancelled", Status: prev}
	}

	// The cancellation itself happened inside claimCancel, under the job's lock: doing
	// it here left a gap in which a Resume could swap the context out, so the cancel
	// landed on the new attempt and the old one kept running.
	//
	// A paused job has no goroutine left to notice any of this — Pause cancelled its
	// context and processJob returned — so the notification has to happen here, or
	// the panel would keep showing "Paused" with a Cancel button that appeared to do
	// nothing. An active job's worker notifies when it unwinds.
	if prev == JobStatusPaused {
		dm.notifyJob("updated", job)
		// claimCancel stamped the terminal state itself, so this is the write the
		// worker would otherwise have recorded — there is no worker left to do it.
		dm.recordTerminal(job, snap)
		dm.emitJobEvent(job, snap)
	}
	return nil
}

// Pause pauses a download job by ID
func (dm *DownloadManager) Pause(jobID string) error {
	job, err := dm.lookup(jobID)
	if err != nil {
		return err
	}

	// The status is written before the context is cancelled, and both the check and
	// that write are one step, so the goroutine cannot see the cancellation before
	// the status — nor can a Cancel slip between them.
	status, ok := job.claimPause()
	if !ok {
		return &StateConflictError{JobID: jobID, Action: "paused", Status: status}
	}

	dm.notifyJob("updated", job)

	return nil
}

// Resume resumes a paused download job by ID
func (dm *DownloadManager) Resume(jobID string) error {
	// The registry's read lock is held across the claim and the start, which is what
	// makes "a job that is running is in the queue" true: ClearFinished and
	// cleanupOldJobs both take the write lock, and either of them landing between the
	// lookup and the claim would leave a worker running on a job no longer in the map
	// — unlistable, uncancellable and never retired.
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	job, exists := dm.jobs[jobID]
	if !exists {
		return &NotFoundError{JobID: jobID}
	}

	// The context is built before the claim and discarded if the claim loses, so the
	// status change and the context installation stay one atomic step.
	ctx, cancel := context.WithCancel(context.Background())
	status, ok := job.claimResume(ctx, cancel)
	if !ok {
		cancel() // don't leak the context we just built
		return &StateConflictError{JobID: jobID, Action: "resumed", Status: status}
	}

	dm.startWorker(job)

	dm.notifyJob("updated", job)

	return nil
}

// Retry retries a failed or cancelled download job by ID
func (dm *DownloadManager) Retry(jobID string) error {
	// Read-locked across the claim and the start, for the reason Resume gives: a
	// ClearFinished between the two would start a worker on a job that is no longer in
	// the queue. Retry is the exposed one, because the states it accepts (failed,
	// cancelled) are exactly the states ClearFinished removes.
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	job, exists := dm.jobs[jobID]
	if !exists {
		return &NotFoundError{JobID: jobID}
	}

	ctx, cancel := context.WithCancel(context.Background())
	status, ok := job.claimRetry(ctx, cancel)
	if !ok {
		cancel()
		return &StateConflictError{JobID: jobID, Action: "retried", Status: status}
	}

	dm.startWorker(job)

	dm.notifyJob("updated", job)

	return nil
}

// lookup resolves a job id against the registry. The manager's lock covers the map
// only; the job's own state is claimed separately by the caller.
func (dm *DownloadManager) lookup(jobID string) (*DownloadJob, error) {
	dm.mu.RLock()
	job, exists := dm.jobs[jobID]
	dm.mu.RUnlock()

	if !exists {
		return nil, &NotFoundError{JobID: jobID}
	}
	return job, nil
}

// startWorker dispatches a job to the processor its kind needs. runFn is set at
// construction and never reassigned, so it needs no lock.
func (dm *DownloadManager) startWorker(job *DownloadJob) {
	if job.runFn != nil {
		go dm.processGenericJob(job)
	} else {
		dm.startDownloadWorker(job)
	}
}

// GetJobs returns a point-in-time copy of every job, in order.
//
// Copies and not the live jobs: both callers are HTTP handlers that JSON-encode the
// result on their own goroutine, holding no job lock, while workers write progress
// and status. GetJob still hands back the live job, because its caller needs the
// object rather than a listing of it.
func (dm *DownloadManager) GetJobs() []*DownloadJob {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*DownloadJob, 0, len(dm.jobOrder))
	for _, id := range dm.jobOrder {
		if job, exists := dm.jobs[id]; exists {
			result = append(result, job.Snapshot())
		}
	}
	return result
}

// GetJob returns a specific job by ID
func (dm *DownloadManager) GetJob(jobID string) (*DownloadJob, bool) {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	job, exists := dm.jobs[jobID]
	return job, exists
}

// Subscribe creates a channel that receives job events
func (dm *DownloadManager) Subscribe() (<-chan JobEvent, func()) {
	ch := make(chan JobEvent, 100)

	dm.subscribersMu.Lock()
	dm.subscribers[ch] = struct{}{}
	dm.subscribersMu.Unlock()

	unsubscribe := func() {
		dm.subscribersMu.Lock()
		delete(dm.subscribers, ch)
		dm.subscribersMu.Unlock()
		close(ch)
	}

	return ch, unsubscribe
}

// notifyJob broadcasts a change to one job as a point-in-time copy.
//
// The copy is the point. JobEvent.Job used to be the live job on every download
// path — generic_job.go already snapshotted, which is how the inconsistency was
// visible — and the SSE handler marshals it on its own goroutine with no lock. That
// is a data race in the plain sense (the -race detector flags it), and its readable
// consequence is a payload assembled from two different instants: a Progress from
// after an update beside the TotalSize and ProgressPercent from before it. Warnings
// is worse, because marshalling a slice header while another goroutine appends to it
// can read a length that does not match the array.
//
// Taking the copy and publishing it are one step, under notifyMu, so subscribers
// receive the snapshots in the order they were taken. A copy is a statement about one
// instant, and two goroutines that snapshot in one order and publish in the other
// deliver a stale statement last — which the live pointer could not do, because it
// always marshalled the present. Concretely: a progress callback snapshots
// `downloading`, a Pause claims `paused` and publishes, then the callback publishes
// its older copy. The panel is left showing a running download with no Resume, and
// nothing repairs it, because the worker's terminal write is correctly suppressed on
// a paused job. So the snapshot fix needs an ordering fix beside it — the same
// coupling as the "added" ordering in Submit.
func (dm *DownloadManager) notifyJob(eventType string, job *DownloadJob) {
	dm.notifyMu.Lock()
	defer dm.notifyMu.Unlock()
	dm.notifySubscribers(JobEvent{Type: eventType, Job: job.Snapshot()})
}

// notifySubscribers sends an event to all subscribers
func (dm *DownloadManager) notifySubscribers(event JobEvent) {
	dm.subscribersMu.RLock()
	defer dm.subscribersMu.RUnlock()

	for ch := range dm.subscribers {
		select {
		case ch <- event:
		default:
			// Channel full, skip (subscriber is slow)
		}
	}
}

// cleanupLoop periodically removes old completed jobs
func (dm *DownloadManager) cleanupLoop() {
	for {
		select {
		case <-dm.cleanupTicker.C:
			dm.cleanupOldJobs()
		case <-dm.done:
			return
		}
	}
}

// cleanupOldJobs removes jobs that completed more than jobRetention ago
// and paused jobs older than PausedJobRetentionDuration. It also calls
// exportSweepFn (if set) to purge expired export tars from disk.
func (dm *DownloadManager) cleanupOldJobs() {
	// Read the export retention outside the main lock to avoid a lock-order
	// concern (currentSettings takes mu.RLock, cleanupOldJobs takes mu.Lock).
	exportRetention := dm.currentSettings().ExportRetention()

	dm.mu.Lock()

	baseRetention := dm.jobRetention
	if baseRetention <= 0 {
		baseRetention = JobRetentionDuration
	}
	pausedCutoff := time.Now().Add(-PausedJobRetentionDuration)
	newOrder := make([]string, 0, len(dm.jobOrder))

	for _, id := range dm.jobOrder {
		job := dm.jobs[id]
		shouldRemove := false

		// Remove completed/failed/cancelled jobs after the appropriate retention
		// period. Completed export jobs use exportRetention (which matches how
		// long the tar file stays on disk); all other terminal jobs use the
		// shorter jobRetention. Failed/cancelled export jobs have no downloadable
		// tar, so they also fall back to jobRetention.
		if completedAt := job.GetCompletedAt(); completedAt != nil {
			retention := baseRetention
			if job.Source == JobSourceGroupExport && job.Status == JobStatusCompleted && exportRetention > 0 {
				retention = exportRetention
			}
			if completedAt.Before(time.Now().Add(-retention)) {
				shouldRemove = true
			}
		}

		// Remove paused jobs after longer retention period (based on creation time)
		if job.GetStatus() == JobStatusPaused && job.CreatedAt.Before(pausedCutoff) {
			shouldRemove = true
		}

		if shouldRemove {
			delete(dm.jobs, id)
			dm.notifyJob("removed", job)
		} else {
			newOrder = append(newOrder, id)
		}
	}

	dm.jobOrder = newOrder
	sweepFn := dm.exportSweepFn
	historySweep := dm.historySweepFn

	dm.mu.Unlock()

	// Sweep expired export tars from disk outside the lock (involves I/O).
	if sweepFn != nil {
		sweepFn()
	}

	// Likewise for expired download-history rows, which is a database write and
	// reads its retention windows from the live settings on every call.
	if historySweep != nil {
		historySweep()
	}
}

// Shutdown gracefully shuts down the download manager
func (dm *DownloadManager) Shutdown() {
	close(dm.done)
	dm.cleanupTicker.Stop()

	// Cancel all active jobs, and collect the paused ones.
	//
	// A paused download has no worker left — Pause ended it — so nothing would ever
	// stamp a terminal state for it, and it would leave with the process: the queue
	// is memory, and the row that should have outlived it was never written. It is
	// abandoned here exactly as Cancel abandons one, which is also what a restart
	// does to it in fact.
	var paused []*DownloadJob
	dm.mu.Lock()
	for _, job := range dm.jobs {
		if job.IsActive() {
			job.Cancel()
			continue
		}
		if job.GetStatus() == JobStatusPaused {
			paused = append(paused, job)
		}
	}
	dm.mu.Unlock()

	for _, job := range paused {
		if prev, snap, ok := job.claimCancel(time.Now()); ok && prev == JobStatusPaused {
			dm.notifyJob("updated", job)
			dm.recordTerminal(job, snap)
			dm.emitJobEvent(job, snap)
		}
	}

	// Then wait for the workers to stamp and record what the cancellation did to
	// them. Bounded, because a worker blocked in a read that ignores its context
	// must not hold a deployment open: the drain is a best effort at not losing the
	// record of a download an operator's restart interrupted.
	drained := make(chan struct{})
	go func() {
		dm.workers.Wait()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(ShutdownDrainTimeout):
	}
}

// ClearFinished removes every terminal job (completed, failed or cancelled) the
// caller may see and returns the ids that went. Pending, downloading, processing and
// **paused** jobs are kept: a paused job is not finished, and dropping one would
// discard a half-transferred download silently.
//
// It returns the ids rather than a count because the caller has to tell its own
// panel what to stop showing, and only this function knows: the browser decides
// which rows are finished before the request goes out, while this decides at
// handling time. A job that crossed into a terminal state inside that window is
// cleared here and unknown to the client — review remediation finding 2.
//
// UI bug hunt 2026-07-29, finding 40: there was no way at all to dismiss a
// finished job. The panel accumulated 20 permanent entries across sessions
// (completed downloads, completed exports, failed imports) and the only thing
// that ever removed one was the retention sweep, hours later.
//
// visible is the caller's RBAC predicate, given each job's owner id: with -auth on
// a non-admin sees only the jobs it submitted, so "Clear completed" must not reach
// anyone else's.
func (dm *DownloadManager) ClearFinished(visible func(owner *uint) bool) []string {
	var removed []*DownloadJob
	ids := make([]string, 0)

	dm.mu.Lock()
	newOrder := make([]string, 0, len(dm.jobOrder))
	for _, id := range dm.jobOrder {
		job := dm.jobs[id]
		if job == nil {
			continue
		}
		status := job.GetStatus()
		terminal := status == JobStatusCompleted || status == JobStatusFailed || status == JobStatusCancelled
		if terminal && (visible == nil || visible(job.GetOwnerUserID())) {
			delete(dm.jobs, id)
			removed = append(removed, job)
			// The map key rather than job.ID: same value, and it needs no lock.
			ids = append(ids, id)
			continue
		}
		newOrder = append(newOrder, id)
	}
	dm.jobOrder = newOrder
	dm.mu.Unlock()

	// Notified outside the lock: notifySubscribers takes no lock of its own but
	// subscribers run on other goroutines, and cleanupOldJobs is the only other
	// place that removes jobs — it notifies under the lock, which this deliberately
	// does not copy.
	for _, job := range removed {
		dm.notifyJob("removed", job)
	}

	return ids
}

// RemoveFinished removes the named terminal jobs the caller may see, and reports
// which ids went.
//
// ClearFinished is all-or-nothing, and the /downloads page needs the other shape:
// deleting one history row has to take the matching queue entry with it, or the
// SSE stream's `init` replay puts the row back on the next reconnect — the page
// would say a download was deleted and the cockpit would keep showing it.
//
// Active and paused jobs are left alone and simply not reported as removed, so a
// caller can tell the difference: a paused job holds a half-transferred download,
// and the answer to "delete this" is "cancel it first", not a silent discard.
//
// visible is the caller's RBAC predicate, as in ClearFinished.
func (dm *DownloadManager) RemoveFinished(jobIDs []string, visible func(owner *uint) bool) []string {
	if len(jobIDs) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(jobIDs))
	for _, id := range jobIDs {
		wanted[id] = struct{}{}
	}

	var removed []*DownloadJob
	ids := make([]string, 0, len(jobIDs))

	dm.mu.Lock()
	newOrder := make([]string, 0, len(dm.jobOrder))
	for _, id := range dm.jobOrder {
		job := dm.jobs[id]
		if job == nil {
			continue
		}
		if _, want := wanted[id]; want {
			status := job.GetStatus()
			terminal := status == JobStatusCompleted || status == JobStatusFailed || status == JobStatusCancelled
			if terminal && (visible == nil || visible(job.GetOwnerUserID())) {
				delete(dm.jobs, id)
				removed = append(removed, job)
				ids = append(ids, id)
				continue
			}
		}
		newOrder = append(newOrder, id)
	}
	dm.jobOrder = newOrder
	dm.mu.Unlock()

	// Outside the lock, for the reason ClearFinished gives.
	for _, job := range removed {
		// Marked before the notification, so the flag is set as early as the job is
		// out of the registry: this is the history delete, and a terminal write for
		// this job that is still on its way to the store must not put back the row
		// the user just deleted.
		job.markDiscarded()
		dm.notifyJob("removed", job)
	}

	return ids
}

// ActiveCount returns the number of active (non-completed) jobs
func (dm *DownloadManager) ActiveCount() int {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	count := 0
	for _, job := range dm.jobs {
		if job.IsActive() {
			count++
		}
	}
	return count
}
