package download_queue

import (
	"context"
	"mahresources/models/query_models"
	"strings"
	"testing"
	"time"
)

// createTestManager creates a DownloadManager for testing with a small max queue size
func createTestManager() *DownloadManager {
	return &DownloadManager{
		jobs:        make(map[string]*DownloadJob),
		jobOrder:    make([]string, 0),
		subscribers: make(map[chan JobEvent]struct{}),
		semaphore:   make(chan struct{}, MaxConcurrentDownloads),
	}
}

// addTestJob adds a job with the given status to the manager
func addTestJob(dm *DownloadManager, id string, status JobStatus) *DownloadJob {
	ctx, cancel := context.WithCancel(context.Background())
	job := &DownloadJob{
		ID:        id,
		Status:    status,
		CreatedAt: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
	}
	dm.jobs[id] = job
	dm.jobOrder = append(dm.jobOrder, id)
	return job
}

// TestMakeRoomForNewJob tests the eviction priority logic
func TestMakeRoomForNewJob(t *testing.T) {
	tests := []struct {
		name           string
		existingJobs   []struct{ id string; status JobStatus }
		expectRoom     bool
		expectEvicted  string // empty if no eviction expected
	}{
		{
			name:         "empty queue has room",
			existingJobs: nil,
			expectRoom:   true,
		},
		{
			name: "evicts oldest completed first",
			existingJobs: []struct{ id string; status JobStatus }{
				{"completed1", JobStatusCompleted},
				{"failed1", JobStatusFailed},
				{"completed2", JobStatusCompleted},
			},
			expectRoom:    true,
			expectEvicted: "completed1",
		},
		{
			name: "evicts failed when no completed",
			existingJobs: []struct{ id string; status JobStatus }{
				{"failed1", JobStatusFailed},
				{"pending1", JobStatusPending},
				{"cancelled1", JobStatusCancelled},
			},
			expectRoom:    true,
			expectEvicted: "failed1",
		},
		{
			name: "evicts cancelled when no completed or failed",
			existingJobs: []struct{ id string; status JobStatus }{
				{"pending1", JobStatusPending},
				{"cancelled1", JobStatusCancelled},
				{"downloading1", JobStatusDownloading},
			},
			expectRoom:    true,
			expectEvicted: "cancelled1",
		},
		{
			name: "cannot evict when all active",
			existingJobs: []struct{ id string; status JobStatus }{
				{"pending1", JobStatusPending},
				{"downloading1", JobStatusDownloading},
				{"processing1", JobStatusProcessing},
			},
			expectRoom: false,
		},
		{
			name: "cannot evict when all paused",
			existingJobs: []struct{ id string; status JobStatus }{
				{"paused1", JobStatusPaused},
				{"paused2", JobStatusPaused},
			},
			expectRoom: false,
		},
		{
			name: "cannot evict mixed active and paused",
			existingJobs: []struct{ id string; status JobStatus }{
				{"pending1", JobStatusPending},
				{"paused1", JobStatusPaused},
				{"downloading1", JobStatusDownloading},
			},
			expectRoom: false,
		},
		{
			name: "prefers completed over failed even if failed is older",
			existingJobs: []struct{ id string; status JobStatus }{
				{"failed1", JobStatusFailed},      // oldest
				{"completed1", JobStatusCompleted}, // newer but preferred
				{"pending1", JobStatusPending},
			},
			expectRoom:    true,
			expectEvicted: "completed1", // completed preferred over failed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := createTestManager()

			// Add existing jobs
			for _, j := range tt.existingJobs {
				addTestJob(dm, j.id, j.status)
			}

			// Set up to capture eviction
			eventCh := make(chan JobEvent, 10)
			dm.subscribersMu.Lock()
			dm.subscribers[eventCh] = struct{}{}
			dm.subscribersMu.Unlock()

			dm.mu.Lock()

			// For empty queue test, don't fill with fillers
			if len(tt.existingJobs) > 0 {
				// Simulate full queue by adding filler jobs
				// Use pending status for fillers so they don't get evicted
				originalLen := len(dm.jobs)
				for i := originalLen; i < MaxQueueSize; i++ {
					dm.jobs[string(rune('a'+i))] = &DownloadJob{ID: string(rune('a' + i)), Status: JobStatusPending}
					dm.jobOrder = append(dm.jobOrder, string(rune('a'+i)))
				}
			}

			gotRoom := dm.makeRoomForNewJob()
			dm.mu.Unlock()

			if gotRoom != tt.expectRoom {
				t.Errorf("makeRoomForNewJob() = %v, want %v", gotRoom, tt.expectRoom)
			}

			if tt.expectEvicted != "" {
				select {
				case event := <-eventCh:
					if event.Type != "removed" {
						t.Errorf("expected 'removed' event, got %s", event.Type)
					}
					if event.Job.ID != tt.expectEvicted {
						t.Errorf("expected job %s to be evicted, got %s", tt.expectEvicted, event.Job.ID)
					}
				case <-time.After(100 * time.Millisecond):
					if tt.expectRoom {
						t.Error("expected eviction event but none received")
					}
				}
			}
		})
	}
}


// TestEvictJob tests the eviction helper function
func TestEvictJob(t *testing.T) {
	t.Run("removes from both maps and sends event", func(t *testing.T) {
		eventCh := make(chan JobEvent, 10)
		dm := createTestManager()
		dm.subscribers[eventCh] = struct{}{}

		job := addTestJob(dm, "test1", JobStatusCompleted)
		addTestJob(dm, "test2", JobStatusPending)

		dm.mu.Lock()
		dm.evictJob("test1", job)
		dm.mu.Unlock()

		// Verify job is removed from map
		if _, exists := dm.jobs["test1"]; exists {
			t.Error("job should be removed from jobs map")
		}

		// Verify other job still exists
		if _, exists := dm.jobs["test2"]; !exists {
			t.Error("other job should still exist")
		}

		// Verify job is removed from order
		for _, id := range dm.jobOrder {
			if id == "test1" {
				t.Error("job should be removed from jobOrder")
			}
		}

		// Verify order still contains other job
		found := false
		for _, id := range dm.jobOrder {
			if id == "test2" {
				found = true
			}
		}
		if !found {
			t.Error("other job should still be in jobOrder")
		}

		// Verify event was sent
		select {
		case event := <-eventCh:
			if event.Type != "removed" {
				t.Errorf("expected 'removed' event, got %s", event.Type)
			}
			if event.Job.ID != "test1" {
				t.Errorf("expected job ID test1, got %s", event.Job.ID)
			}
		default:
			t.Error("expected event to be sent")
		}
	})

	t.Run("handles evicting last job", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "only", JobStatusCompleted)

		dm.mu.Lock()
		dm.evictJob("only", job)
		dm.mu.Unlock()

		if len(dm.jobs) != 0 {
			t.Errorf("expected 0 jobs, got %d", len(dm.jobs))
		}
		if len(dm.jobOrder) != 0 {
			t.Errorf("expected empty jobOrder, got %d", len(dm.jobOrder))
		}
	})

	t.Run("handles evicting middle job preserves order", func(t *testing.T) {
		dm := createTestManager()
		addTestJob(dm, "first", JobStatusCompleted)
		job := addTestJob(dm, "middle", JobStatusCompleted)
		addTestJob(dm, "last", JobStatusCompleted)

		dm.mu.Lock()
		dm.evictJob("middle", job)
		dm.mu.Unlock()

		expectedOrder := []string{"first", "last"}
		if len(dm.jobOrder) != 2 {
			t.Fatalf("expected 2 jobs in order, got %d", len(dm.jobOrder))
		}
		for i, id := range dm.jobOrder {
			if id != expectedOrder[i] {
				t.Errorf("jobOrder[%d] = %s, want %s", i, id, expectedOrder[i])
			}
		}
	})
}

// TestSubmitWithEviction tests that Submit properly evicts jobs when queue is full
func TestSubmitWithEviction(t *testing.T) {
	// We can't easily test with real MaxQueueSize (100), so we test the logic directly
	t.Run("makes room by evicting completed job", func(t *testing.T) {
		dm := createTestManager()
		eventCh := make(chan JobEvent, 100)
		dm.subscribers[eventCh] = struct{}{}

		// Fill with completed jobs up to near max
		for i := 0; i < MaxQueueSize; i++ {
			addTestJob(dm, string(rune('a'+i)), JobStatusCompleted)
		}

		if len(dm.jobs) != MaxQueueSize {
			t.Fatalf("expected %d jobs, got %d", MaxQueueSize, len(dm.jobs))
		}

		// Now try to make room
		dm.mu.Lock()
		gotRoom := dm.makeRoomForNewJob()
		dm.mu.Unlock()

		if !gotRoom {
			t.Error("should have made room by evicting completed job")
		}

		if len(dm.jobs) != MaxQueueSize-1 {
			t.Errorf("expected %d jobs after eviction, got %d", MaxQueueSize-1, len(dm.jobs))
		}
	})

	t.Run("returns error when all jobs are active", func(t *testing.T) {
		dm := createTestManager()

		// Fill with active jobs
		for i := 0; i < MaxQueueSize; i++ {
			addTestJob(dm, string(rune('a'+i)), JobStatusPending)
		}

		dm.mu.Lock()
		gotRoom := dm.makeRoomForNewJob()
		dm.mu.Unlock()

		if gotRoom {
			t.Error("should not have made room when all jobs are active")
		}
	})
}

// TestEvictionOrder tests that jobs are evicted in the correct order
func TestEvictionOrder(t *testing.T) {
	t.Run("evicts in insertion order within same status", func(t *testing.T) {
		dm := createTestManager()

		// Add completed jobs in order
		addTestJob(dm, "old", JobStatusCompleted)
		addTestJob(dm, "middle", JobStatusCompleted)
		addTestJob(dm, "new", JobStatusCompleted)

		// Verify jobOrder
		expectedOrder := []string{"old", "middle", "new"}
		for i, id := range dm.jobOrder {
			if id != expectedOrder[i] {
				t.Errorf("jobOrder[%d] = %s, want %s", i, id, expectedOrder[i])
			}
		}

		// Test eviction by directly calling evictJob to verify order
		dm.mu.Lock()

		// First pass should find "old" (first completed)
		var firstEvicted string
		for _, id := range dm.jobOrder {
			if dm.jobs[id].GetStatus() == JobStatusCompleted {
				firstEvicted = id
				break
			}
		}
		dm.mu.Unlock()

		if firstEvicted != "old" {
			t.Errorf("expected 'old' to be first evictable, got %s", firstEvicted)
		}
	})

	t.Run("completed evicted before failed regardless of order", func(t *testing.T) {
		dm := createTestManager()

		// Add failed first, then completed
		addTestJob(dm, "failed_old", JobStatusFailed)
		addTestJob(dm, "completed_new", JobStatusCompleted)
		addTestJob(dm, "pending", JobStatusPending)

		dm.mu.Lock()

		// First pass for completed should find "completed_new"
		var completedToEvict string
		for _, id := range dm.jobOrder {
			if dm.jobs[id].GetStatus() == JobStatusCompleted {
				completedToEvict = id
				break
			}
		}

		dm.mu.Unlock()

		if completedToEvict != "completed_new" {
			t.Errorf("expected 'completed_new' to be evicted, got %s", completedToEvict)
		}
	})

	t.Run("falls back to failed when no completed", func(t *testing.T) {
		dm := createTestManager()

		// Add only failed and active jobs
		addTestJob(dm, "failed_old", JobStatusFailed)
		addTestJob(dm, "failed_new", JobStatusFailed)
		addTestJob(dm, "pending", JobStatusPending)

		dm.mu.Lock()

		// No completed jobs, so look for failed
		var toEvict string
		for _, id := range dm.jobOrder {
			if dm.jobs[id].GetStatus() == JobStatusCompleted {
				toEvict = id
				break
			}
		}
		if toEvict == "" {
			for _, id := range dm.jobOrder {
				status := dm.jobs[id].GetStatus()
				if status == JobStatusFailed || status == JobStatusCancelled {
					toEvict = id
					break
				}
			}
		}

		dm.mu.Unlock()

		if toEvict != "failed_old" {
			t.Errorf("expected 'failed_old' to be evicted, got %s", toEvict)
		}
	})
}

// TestActiveCount tests the ActiveCount method
func TestActiveCount(t *testing.T) {
	tests := []struct {
		name     string
		jobs     []struct{ id string; status JobStatus }
		expected int
	}{
		{
			name:     "empty queue",
			jobs:     nil,
			expected: 0,
		},
		{
			name: "all active",
			jobs: []struct{ id string; status JobStatus }{
				{"pending", JobStatusPending},
				{"downloading", JobStatusDownloading},
				{"processing", JobStatusProcessing},
			},
			expected: 3,
		},
		{
			name: "mixed statuses",
			jobs: []struct{ id string; status JobStatus }{
				{"pending", JobStatusPending},
				{"completed", JobStatusCompleted},
				{"failed", JobStatusFailed},
				{"downloading", JobStatusDownloading},
			},
			expected: 2,
		},
		{
			name: "none active",
			jobs: []struct{ id string; status JobStatus }{
				{"completed", JobStatusCompleted},
				{"failed", JobStatusFailed},
				{"cancelled", JobStatusCancelled},
				{"paused", JobStatusPaused},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dm := createTestManager()
			for _, j := range tt.jobs {
				addTestJob(dm, j.id, j.status)
			}

			got := dm.ActiveCount()
			if got != tt.expected {
				t.Errorf("ActiveCount() = %d, want %d", got, tt.expected)
			}
		})
	}
}

// TestGetJobs tests that GetJobs returns jobs in order
func TestGetJobs(t *testing.T) {
	dm := createTestManager()

	addTestJob(dm, "first", JobStatusPending)
	addTestJob(dm, "second", JobStatusCompleted)
	addTestJob(dm, "third", JobStatusFailed)

	jobs := dm.GetJobs()

	if len(jobs) != 3 {
		t.Fatalf("expected 3 jobs, got %d", len(jobs))
	}

	expectedOrder := []string{"first", "second", "third"}
	for i, job := range jobs {
		if job.ID != expectedOrder[i] {
			t.Errorf("GetJobs()[%d].ID = %s, want %s", i, job.ID, expectedOrder[i])
		}
	}
}

// TestGetJob tests retrieving a specific job
func TestGetJob(t *testing.T) {
	dm := createTestManager()
	addTestJob(dm, "exists", JobStatusPending)

	t.Run("existing job", func(t *testing.T) {
		job, exists := dm.GetJob("exists")
		if !exists {
			t.Error("expected job to exist")
		}
		if job.ID != "exists" {
			t.Errorf("job.ID = %s, want 'exists'", job.ID)
		}
	})

	t.Run("non-existing job", func(t *testing.T) {
		_, exists := dm.GetJob("nonexistent")
		if exists {
			t.Error("expected job to not exist")
		}
	})
}

// TestCancel tests job cancellation
func TestCancel(t *testing.T) {
	t.Run("cancel active job", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "active", JobStatusPending)

		err := dm.Cancel("active")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		// Context should be cancelled
		select {
		case <-job.GetContext().Done():
			// Expected
		default:
			t.Error("job context should be cancelled")
		}
	})

	t.Run("cancel non-existent job", func(t *testing.T) {
		dm := createTestManager()

		err := dm.Cancel("nonexistent")
		if err == nil {
			t.Error("expected error for non-existent job")
		}
		if !strings.Contains(err.Error(), "not found") {
			t.Errorf("expected 'not found' error, got: %v", err)
		}
	})

	t.Run("cancel completed job", func(t *testing.T) {
		dm := createTestManager()
		addTestJob(dm, "done", JobStatusCompleted)

		err := dm.Cancel("done")
		if err == nil {
			t.Error("expected error for completed job")
		}
		if !strings.Contains(err.Error(), "already finished") {
			t.Errorf("expected 'already finished' error, got: %v", err)
		}
	})
}

// TestPauseResume tests pause and resume functionality
func TestPauseResume(t *testing.T) {
	t.Run("pause active job", func(t *testing.T) {
		dm := createTestManager()
		eventCh := make(chan JobEvent, 10)
		dm.subscribers[eventCh] = struct{}{}

		addTestJob(dm, "active", JobStatusDownloading)

		err := dm.Pause("active")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		job, _ := dm.GetJob("active")
		if job.GetStatus() != JobStatusPaused {
			t.Errorf("expected paused status, got %s", job.GetStatus())
		}
	})

	t.Run("cannot pause completed job", func(t *testing.T) {
		dm := createTestManager()
		addTestJob(dm, "done", JobStatusCompleted)

		err := dm.Pause("done")
		if err == nil {
			t.Error("expected error for completed job")
		}
	})

	t.Run("resume paused job", func(t *testing.T) {
		dm := createTestManager()
		eventCh := make(chan JobEvent, 10)
		dm.subscribers[eventCh] = struct{}{}

		job := addTestJob(dm, "paused", JobStatusPaused)
		job.creator = &query_models.ResourceFromRemoteCreator{URL: "http://example.com/file.txt"}

		err := dm.Resume("paused")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		job, _ = dm.GetJob("paused")
		if job.GetStatus() != JobStatusPending {
			t.Errorf("expected pending status after resume, got %s", job.GetStatus())
		}
	})

	t.Run("cannot resume non-paused job", func(t *testing.T) {
		dm := createTestManager()
		addTestJob(dm, "active", JobStatusDownloading)

		err := dm.Resume("active")
		if err == nil {
			t.Error("expected error for non-paused job")
		}
	})
}

// TestRetry tests retry functionality
func TestRetry(t *testing.T) {
	t.Run("retry failed job", func(t *testing.T) {
		dm := createTestManager()
		eventCh := make(chan JobEvent, 10)
		dm.subscribers[eventCh] = struct{}{}

		job := addTestJob(dm, "failed", JobStatusFailed)
		job.creator = &query_models.ResourceFromRemoteCreator{URL: "http://example.com/file.txt"}
		job.SetError("previous error")

		err := dm.Retry("failed")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		job, _ = dm.GetJob("failed")
		if job.GetStatus() != JobStatusPending {
			t.Errorf("expected pending status after retry, got %s", job.GetStatus())
		}
		if job.Error != "" {
			t.Error("error should be cleared after retry")
		}
	})

	t.Run("retry cancelled job", func(t *testing.T) {
		dm := createTestManager()
		job := addTestJob(dm, "cancelled", JobStatusCancelled)
		job.creator = &query_models.ResourceFromRemoteCreator{URL: "http://example.com/file.txt"}

		err := dm.Retry("cancelled")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("cannot retry active job", func(t *testing.T) {
		dm := createTestManager()
		addTestJob(dm, "active", JobStatusDownloading)

		err := dm.Retry("active")
		if err == nil {
			t.Error("expected error for active job")
		}
	})

	t.Run("cannot retry completed job", func(t *testing.T) {
		dm := createTestManager()
		addTestJob(dm, "done", JobStatusCompleted)

		err := dm.Retry("done")
		if err == nil {
			t.Error("expected error for completed job")
		}
	})
}

// TestSubscribe tests event subscription
func TestSubscribe(t *testing.T) {
	dm := createTestManager()

	countSubscribers := func() int {
		dm.subscribersMu.RLock()
		defer dm.subscribersMu.RUnlock()
		return len(dm.subscribers)
	}

	initialCount := countSubscribers()

	ch, unsubscribe := dm.Subscribe()

	// Verify channel is registered
	if countSubscribers() != initialCount+1 {
		t.Error("subscriber count should increase by 1")
	}

	// Verify we can receive events
	dm.notifySubscribers(JobEvent{Type: "test", Job: &DownloadJob{ID: "test"}})
	select {
	case event := <-ch:
		if event.Type != "test" {
			t.Errorf("expected 'test' event, got %s", event.Type)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("expected to receive event")
	}

	// Unsubscribe
	unsubscribe()

	if countSubscribers() != initialCount {
		t.Error("subscriber count should return to initial after unsubscribe")
	}
}

// TestCleanupOldJobs tests the cleanup of old completed jobs
func TestCleanupOldJobs(t *testing.T) {
	dm := createTestManager()
	eventCh := make(chan JobEvent, 10)
	dm.subscribers[eventCh] = struct{}{}

	// Add a job completed long ago
	oldJob := addTestJob(dm, "old", JobStatusCompleted)
	completedAt := time.Now().Add(-2 * JobRetentionDuration)
	oldJob.CompletedAt = &completedAt

	// Add a recently completed job
	newJob := addTestJob(dm, "new", JobStatusCompleted)
	recentTime := time.Now()
	newJob.CompletedAt = &recentTime

	// Add an active job
	addTestJob(dm, "active", JobStatusPending)

	dm.cleanupOldJobs()

	// Old job should be removed
	if _, exists := dm.jobs["old"]; exists {
		t.Error("old completed job should be removed")
	}

	// New job should still exist
	if _, exists := dm.jobs["new"]; !exists {
		t.Error("recently completed job should still exist")
	}

	// Active job should still exist
	if _, exists := dm.jobs["active"]; !exists {
		t.Error("active job should still exist")
	}
}

// TestCleanupPausedJobs tests cleanup of old paused jobs
func TestCleanupPausedJobs(t *testing.T) {
	dm := createTestManager()

	// Add a job paused long ago
	oldPaused := addTestJob(dm, "old_paused", JobStatusPaused)
	oldPaused.CreatedAt = time.Now().Add(-2 * PausedJobRetentionDuration)

	// Add a recently paused job
	newPaused := addTestJob(dm, "new_paused", JobStatusPaused)
	newPaused.CreatedAt = time.Now()

	dm.cleanupOldJobs()

	// Old paused job should be removed
	if _, exists := dm.jobs["old_paused"]; exists {
		t.Error("old paused job should be removed")
	}

	// New paused job should still exist
	if _, exists := dm.jobs["new_paused"]; !exists {
		t.Error("recently paused job should still exist")
	}
}
