package download_queue

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/interfaces"
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
	AddResource(file interfaces.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error)
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
func (dm *DownloadManager) Submit(creator *query_models.ResourceFromRemoteCreator) (*DownloadJob, error) {
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
	}

	dm.jobs[job.ID] = job
	dm.jobOrder = append(dm.jobOrder, job.ID)

	// Start download in background
	go dm.processJob(job)

	dm.notifySubscribers(JobEvent{Type: "added", Job: job})

	return job, nil
}

// SubmitMultiple submits multiple URLs (newline-separated) as individual jobs
func (dm *DownloadManager) SubmitMultiple(creator *query_models.ResourceFromRemoteCreator) ([]*DownloadJob, error) {
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

		job, err := dm.Submit(&singleCreator)
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
		// Check if job was paused (don't override paused status)
		if job.GetStatus() != JobStatusPaused {
			job.SetStatus(JobStatusCancelled)
			job.SetError("Cancelled before starting")
			dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
		}
		return
	}

	now := time.Now()
	job.SetStartedAt(now)
	job.SetStatus(JobStatusDownloading)
	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	// Perform the download with progress tracking
	resource, err := dm.downloadWithProgress(job)

	// Check if job was paused during download (don't set completion status)
	if job.GetStatus() == JobStatusPaused {
		return
	}

	now = time.Now()
	job.SetCompletedAt(now)

	if err != nil {
		if job.GetContext().Err() != nil {
			job.SetStatus(JobStatusCancelled)
			job.SetError("Download cancelled")
		} else {
			job.SetStatus(JobStatusFailed)
			job.SetError(err.Error())
		}
	} else {
		job.SetStatus(JobStatusCompleted)
		job.SetResourceID(resource.ID)
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

	// Determine filename
	name := job.creator.FileName
	if name == "" {
		name = path.Base(job.URL)
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

	return dm.resourceCtx.AddResource(progressBody, name, &query_models.ResourceCreator{
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

// Cancel cancels a download job by ID
func (dm *DownloadManager) Cancel(jobID string) error {
	dm.mu.RLock()
	job, exists := dm.jobs[jobID]
	dm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	if !job.IsActive() {
		return fmt.Errorf("job %s already finished", jobID)
	}

	job.Cancel() // This triggers context cancellation
	return nil
}

// Pause pauses a download job by ID
func (dm *DownloadManager) Pause(jobID string) error {
	dm.mu.RLock()
	job, exists := dm.jobs[jobID]
	dm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job %s not found", jobID)
	}

	if !job.CanPause() {
		return fmt.Errorf("job %s cannot be paused (status: %s)", jobID, job.GetStatus())
	}

	// Mark as paused BEFORE cancelling context to avoid race condition
	// where goroutine sees cancellation before status change
	job.SetStatus(JobStatusPaused)
	job.SetError("") // Clear any previous error

	// Now cancel the download context
	job.Cancel()

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	return nil
}

// Resume resumes a paused download job by ID
func (dm *DownloadManager) Resume(jobID string) error {
	dm.mu.Lock()
	job, exists := dm.jobs[jobID]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("job %s not found", jobID)
	}

	if !job.CanResume() {
		dm.mu.Unlock()
		return fmt.Errorf("job %s cannot be resumed (status: %s)", jobID, job.GetStatus())
	}

	// Create a new context for the resumed download
	ctx, cancel := context.WithCancel(context.Background())
	job.SetContext(ctx, cancel)

	// Reset progress and mark as pending (all under lock)
	job.SetStatus(JobStatusPending)
	job.UpdateProgress(0, -1)
	job.SetStartedAt(time.Time{})

	dm.mu.Unlock()

	// Start download in background — dispatch to the correct processor.
	if job.runFn != nil {
		go dm.processGenericJob(job)
	} else {
		go dm.processJob(job)
	}

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	return nil
}

// Retry retries a failed or cancelled download job by ID
func (dm *DownloadManager) Retry(jobID string) error {
	dm.mu.Lock()
	job, exists := dm.jobs[jobID]
	if !exists {
		dm.mu.Unlock()
		return fmt.Errorf("job %s not found", jobID)
	}

	if !job.CanRetry() {
		dm.mu.Unlock()
		return fmt.Errorf("job %s cannot be retried (status: %s)", jobID, job.GetStatus())
	}

	// Create a new context for the retried download
	ctx, cancel := context.WithCancel(context.Background())
	job.SetContext(ctx, cancel)

	// Reset progress and error, mark as pending (all under lock)
	job.SetStatus(JobStatusPending)
	job.SetError("")
	job.UpdateProgress(0, -1)
	job.SetStartedAt(time.Time{})
	job.SetCompletedAt(time.Time{})
	job.SetResourceID(0)

	dm.mu.Unlock()

	// Start download in background — dispatch to the correct processor.
	if job.runFn != nil {
		go dm.processGenericJob(job)
	} else {
		go dm.processJob(job)
	}

	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	return nil
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
