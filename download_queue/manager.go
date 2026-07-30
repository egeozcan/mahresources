package download_queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"net"
	"net/http"
	"path"
	"strings"
	"sync"
	"time"
)

const (
	MaxConcurrentDownloads     = 3
	MaxQueueSize               = 100
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
	mu            sync.RWMutex
	jobs          map[string]*DownloadJob
	jobOrder      []string // Maintains insertion order
	resourceCtx   ResourceCreator
	settings      DownloadSettings
	semaphore     chan struct{}
	subscribers   map[chan JobEvent]struct{}
	subscribersMu sync.RWMutex
	cleanupTicker *time.Ticker
	done          chan struct{}
	concurrency   int
	jobRetention  time.Duration
	exportSweepFn func() // called by cleanupOldJobs to sweep expired export tars from disk
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
		jobs:         make(map[string]*DownloadJob),
		jobOrder:     make([]string, 0),
		resourceCtx:  resourceCtx,
		settings:     settings,
		semaphore:    make(chan struct{}, cfg.Concurrency),
		subscribers:  make(map[chan JobEvent]struct{}),
		done:         make(chan struct{}),
		concurrency:  cfg.Concurrency,
		jobRetention: cfg.JobRetention,
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

// generateShortID creates a short random ID for display
func generateShortID() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		// Fallback to timestamp-based ID if crypto/rand fails
		return fmt.Sprintf("%08x", time.Now().UnixNano()&0xFFFFFFFF)
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

	dm.notifySubscribers(JobEvent{Type: "removed", Job: job})
}

// Submit adds a new download job to the queue
// Submit enqueues a background download. ownerUserID is the submitting user (nil
// for the auth-off super-user / unowned jobs); it is set on the job at
// construction — before the processing goroutine starts — so the worker can
// attribute the created resource (CreatedByUserId) to the submitter without a
// race, and so queue-visibility RBAC is applied before processing.
func (dm *DownloadManager) Submit(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint) (*DownloadJob, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.makeRoomForNewJob() {
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
	}

	dm.jobs[job.ID] = job
	dm.jobOrder = append(dm.jobOrder, job.ID)

	// Start download in background. The go statement happens-before the
	// goroutine's execution, so ownerUserID (set above) is visible to the worker.
	go dm.processJob(job)

	dm.notifySubscribers(JobEvent{Type: "added", Job: job})

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
func (dm *DownloadManager) processJob(job *DownloadJob) {
	// Acquire semaphore slot (limits concurrent downloads)
	select {
	case dm.semaphore <- struct{}{}:
		defer func() { <-dm.semaphore }()
	case <-job.GetContext().Done():
		// finish is a no-op on a paused job: Pause cancels the context too, and a
		// paused job waits for Resume rather than being retired here.
		if job.finish(JobStatusCancelled, "Cancelled before starting", 0, time.Now()) {
			dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
		}
		return
	}

	// The forward transition is claimed too, or a Pause landing in the moment
	// between the semaphore acquisition and this write would be overwritten by it
	// (see DownloadJob.claimStart). A refusal means a control owns the job now and
	// has already notified subscribers.
	if !job.claimStart(JobStatusDownloading, time.Now()) {
		return
	}
	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	// Perform the download with progress tracking
	resource, err := dm.downloadWithProgress(job)

	status, errMsg, resourceID := JobStatusCompleted, "", uint(0)
	switch {
	case err != nil && job.GetContext().Err() != nil:
		status, errMsg = JobStatusCancelled, "Download cancelled"
	case err != nil:
		status, errMsg = JobStatusFailed, err.Error()
	default:
		resourceID = resource.ID
	}

	// One atomic terminal write. Reading the status to see whether the job was paused
	// and then writing the terminal one is the same check-then-act the controls had:
	// a Pause landing between the two was silently overwritten, and a Pause landing
	// just before the read stranded the job (see DownloadJob.finish).
	if !job.finish(status, errMsg, resourceID, time.Now()) {
		return
	}

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
}

// createHTTPClient creates an HTTP client with context support.
// settings is snapshotted by the caller (downloadWithProgress) so timeout
// values reflect the live runtime configuration at the moment the download
// starts.
func (dm *DownloadManager) createHTTPClient(s DownloadSettings) *http.Client {
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

// downloadWithProgress performs the HTTP download with progress tracking
func (dm *DownloadManager) downloadWithProgress(job *DownloadJob) (*models.Resource, error) {
	// Snapshot settings once so all timeout values are consistent for this
	// download and the read-lock is held only briefly.
	s := dm.currentSettings()
	httpClient := dm.createHTTPClient(s)

	req, err := http.NewRequestWithContext(job.GetContext(), "GET", job.URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	// Get content length if available
	contentLength := resp.ContentLength
	job.UpdateProgress(0, contentLength)
	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	// Wrap with timeout reader for idle detection and cancellation
	timeoutBody := NewTimeoutReaderWithContext(resp.Body, s.IdleTimeout(), job.GetContext())
	defer timeoutBody.Close()

	// Throttle progress updates to avoid flooding SSE clients
	var lastNotify time.Time
	const notifyInterval = 500 * time.Millisecond

	// Wrap with progress reader
	progressBody := NewProgressReader(timeoutBody,
		// onProgress - called on each chunk read
		func(downloaded int64) {
			job.UpdateProgress(downloaded, contentLength)
			// Only notify if enough time has passed
			if time.Since(lastNotify) >= notifyInterval {
				lastNotify = time.Now()
				dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
			}
		},
		// onComplete - called when download finishes (EOF)
		func() {
			// Send final progress update, then switch to processing
			dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
			job.SetStatus(JobStatusProcessing)
			dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
		},
	)

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
	prev, ok := job.claimCancel(time.Now())
	if !ok {
		return &StateConflictError{JobID: jobID, Action: "cancelled", Status: prev}
	}

	// The context cancellation happens outside the job's lock, because a CancelFunc
	// can run arbitrary registered work. That is safe now: claimCancel has already
	// recorded the intent, so a Pause arriving here is refused rather than winning.
	job.Cancel()

	// A paused job has no goroutine left to notice any of this — Pause cancelled its
	// context and processJob returned — so the notification has to happen here, or
	// the panel would keep showing "Paused" with a Cancel button that appeared to do
	// nothing. An active job's worker notifies when it unwinds.
	if prev == JobStatusPaused {
		dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
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

	job.Cancel()

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	return nil
}

// Resume resumes a paused download job by ID
func (dm *DownloadManager) Resume(jobID string) error {
	job, err := dm.lookup(jobID)
	if err != nil {
		return err
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

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	return nil
}

// Retry retries a failed or cancelled download job by ID
func (dm *DownloadManager) Retry(jobID string) error {
	job, err := dm.lookup(jobID)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	status, ok := job.claimRetry(ctx, cancel)
	if !ok {
		cancel()
		return &StateConflictError{JobID: jobID, Action: "retried", Status: status}
	}

	dm.startWorker(job)

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

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
		go dm.processJob(job)
	}
}

// GetJobs returns all jobs in order
func (dm *DownloadManager) GetJobs() []*DownloadJob {
	dm.mu.RLock()
	defer dm.mu.RUnlock()

	result := make([]*DownloadJob, 0, len(dm.jobOrder))
	for _, id := range dm.jobOrder {
		if job, exists := dm.jobs[id]; exists {
			result = append(result, job)
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
			dm.notifySubscribers(JobEvent{Type: "removed", Job: job})
		} else {
			newOrder = append(newOrder, id)
		}
	}

	dm.jobOrder = newOrder
	sweepFn := dm.exportSweepFn

	dm.mu.Unlock()

	// Sweep expired export tars from disk outside the lock (involves I/O).
	if sweepFn != nil {
		sweepFn()
	}
}

// Shutdown gracefully shuts down the download manager
func (dm *DownloadManager) Shutdown() {
	close(dm.done)
	dm.cleanupTicker.Stop()

	// Cancel all active jobs
	dm.mu.Lock()
	for _, job := range dm.jobs {
		if job.IsActive() {
			job.Cancel()
		}
	}
	dm.mu.Unlock()
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
		dm.notifySubscribers(JobEvent{Type: "removed", Job: job})
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
