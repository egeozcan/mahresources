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
	MaxConcurrentDownloads = 3
	MaxQueueSize           = 100
	JobRetentionDuration   = 1 * time.Hour
)

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

// DownloadManager manages background download jobs
type DownloadManager struct {
	mu            sync.RWMutex
	jobs          map[string]*DownloadJob
	jobOrder      []string // Maintains insertion order
	resourceCtx   ResourceCreator
	timeoutConfig TimeoutConfig
	semaphore     chan struct{}
	subscribers   map[chan JobEvent]struct{}
	subscribersMu sync.RWMutex
	cleanupTicker *time.Ticker
	done          chan struct{}
}

// NewDownloadManager creates a new download manager
func NewDownloadManager(resourceCtx ResourceCreator, timeoutConfig TimeoutConfig) *DownloadManager {
	dm := &DownloadManager{
		jobs:          make(map[string]*DownloadJob),
		jobOrder:      make([]string, 0),
		resourceCtx:   resourceCtx,
		timeoutConfig: timeoutConfig,
		semaphore:     make(chan struct{}, MaxConcurrentDownloads),
		subscribers:   make(map[chan JobEvent]struct{}),
		done:          make(chan struct{}),
	}

	// Start cleanup goroutine
	dm.cleanupTicker = time.NewTicker(5 * time.Minute)
	go dm.cleanupLoop()

	return dm
}

// generateShortID creates a short random ID for display
func generateShortID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// Submit adds a new download job to the queue
func (dm *DownloadManager) Submit(creator *query_models.ResourceFromRemoteCreator) (*DownloadJob, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if len(dm.jobs) >= MaxQueueSize {
		return nil, fmt.Errorf("download queue is full (max %d jobs)", MaxQueueSize)
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
	case <-job.ctx.Done():
		job.SetStatus(JobStatusCancelled)
		job.SetError("Cancelled before starting")
		dm.notifySubscribers(JobEvent{Type: "updated", Job: job})
		return
	}

	now := time.Now()
	job.SetStartedAt(now)
	job.SetStatus(JobStatusDownloading)
	dm.notifySubscribers(JobEvent{Type: "updated", Job: job})

	// Perform the download with progress tracking
	resource, err := dm.downloadWithProgress(job)

	now = time.Now()
	job.SetCompletedAt(now)

	if err != nil {
		if job.ctx.Err() != nil {
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

// createHTTPClient creates an HTTP client with context support
func (dm *DownloadManager) createHTTPClient() *http.Client {
	return &http.Client{
		Timeout: dm.timeoutConfig.OverallTimeout,
		Transport: &http.Transport{
			DialContext:           (&net.Dialer{Timeout: dm.timeoutConfig.ConnectTimeout}).DialContext,
			TLSHandshakeTimeout:   dm.timeoutConfig.ConnectTimeout / 2,
			ResponseHeaderTimeout: dm.timeoutConfig.ConnectTimeout,
			IdleConnTimeout:       90 * time.Second,
		},
	}
}

// downloadWithProgress performs the HTTP download with progress tracking
func (dm *DownloadManager) downloadWithProgress(job *DownloadJob) (*models.Resource, error) {
	httpClient := dm.createHTTPClient()

	req, err := http.NewRequestWithContext(job.ctx, "GET", job.URL, nil)
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
	timeoutBody := NewTimeoutReaderWithContext(resp.Body, dm.timeoutConfig.IdleTimeout, job.ctx)
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
	return dm.resourceCtx.AddResource(progressBody, name, &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{
			Name:             name,
			Description:      job.creator.Description,
			OwnerId:          job.creator.OwnerId,
			Groups:           job.creator.Groups,
			Tags:             job.creator.Tags,
			Notes:            job.creator.Notes,
			Meta:             job.creator.Meta,
			ContentCategory:  job.creator.ContentCategory,
			Category:         job.creator.Category,
			OriginalName:     job.URL,
			OriginalLocation: job.URL,
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

	job.cancel() // This triggers context cancellation
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

// cleanupOldJobs removes jobs that completed more than JobRetentionDuration ago
func (dm *DownloadManager) cleanupOldJobs() {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	cutoff := time.Now().Add(-JobRetentionDuration)
	newOrder := make([]string, 0, len(dm.jobOrder))

	for _, id := range dm.jobOrder {
		job := dm.jobs[id]
		if job.CompletedAt != nil && job.CompletedAt.Before(cutoff) {
			delete(dm.jobs, id)
			dm.notifySubscribers(JobEvent{Type: "removed", Job: job})
		} else {
			newOrder = append(newOrder, id)
		}
	}

	dm.jobOrder = newOrder
}

// Shutdown gracefully shuts down the download manager
func (dm *DownloadManager) Shutdown() {
	close(dm.done)
	dm.cleanupTicker.Stop()

	// Cancel all active jobs
	dm.mu.Lock()
	for _, job := range dm.jobs {
		if job.IsActive() {
			job.cancel()
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
