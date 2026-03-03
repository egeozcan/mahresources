package plugin_system

import (
	"testing"
	"time"
)

func TestRunActionAsync_CreatesJob(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "async-plugin", `
plugin = { name = "async-plugin", version = "1.0", description = "async test" }

function do_work(ctx)
    mah.job_progress(ctx.job_id, 50, "halfway")
    mah.job_complete(ctx.job_id, { message = "all done", count = 42 })
end

function init()
    mah.action({
        id = "async-work",
        label = "Async Work",
        entity = "resource",
        async = true,
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("async-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	jobID, err := pm.RunActionAsync("async-plugin", "async-work", 42, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync: %v", err)
	}

	if jobID == "" {
		t.Fatal("expected non-empty job ID")
	}

	// Poll until job completes (with timeout).
	deadline := time.After(10 * time.Second)
	for {
		job := pm.GetActionJob(jobID)
		if job == nil {
			t.Fatal("job not found")
		}

		if job.Status == "completed" {
			if job.Progress != 100 {
				t.Errorf("expected progress 100, got %d", job.Progress)
			}
			if job.Message != "all done" {
				t.Errorf("expected message 'all done', got %q", job.Message)
			}
			if job.Result == nil {
				t.Fatal("expected result to be non-nil")
			}
			if job.Result["count"] != float64(42) {
				t.Errorf("expected result.count=42, got %v", job.Result["count"])
			}
			if job.PluginName != "async-plugin" {
				t.Errorf("expected pluginName 'async-plugin', got %q", job.PluginName)
			}
			if job.ActionID != "async-work" {
				t.Errorf("expected actionId 'async-work', got %q", job.ActionID)
			}
			if job.EntityID != 42 {
				t.Errorf("expected entityId 42, got %d", job.EntityID)
			}
			if job.Source != "plugin" {
				t.Errorf("expected source 'plugin', got %q", job.Source)
			}
			break
		}

		if job.Status == "failed" {
			t.Fatalf("job failed unexpectedly: %s", job.Message)
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job completion, status=%s", job.Status)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestRunActionAsync_JobProgress(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "progress-plugin", `
plugin = { name = "progress-plugin", version = "1.0", description = "progress test" }

function do_work(ctx)
    mah.job_progress(ctx.job_id, 25, "step 1")
    mah.job_progress(ctx.job_id, 50, "step 2")
    mah.job_progress(ctx.job_id, 75, "step 3")
    mah.job_complete(ctx.job_id, { message = "finished" })
end

function init()
    mah.action({
        id = "progress-work",
        label = "Progress Work",
        entity = "resource",
        async = true,
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("progress-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Subscribe to events before starting the action.
	ch := pm.SubscribeActionJobs()
	defer pm.UnsubscribeActionJobs(ch)

	_, err = pm.RunActionAsync("progress-plugin", "progress-work", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync: %v", err)
	}

	// Collect events until we see the completion event.
	var events []ActionJobEvent
	deadline := time.After(10 * time.Second)

	for {
		select {
		case event := <-ch:
			events = append(events, event)
			snap := event.Job.Snapshot()
			if snap.Status == "completed" || snap.Status == "failed" {
				goto done
			}
		case <-deadline:
			t.Fatalf("timed out waiting for events, got %d events so far", len(events))
		}
	}

done:
	// Verify we got at least: added + completed (may also include running + progress updates)
	if len(events) < 2 {
		t.Errorf("expected at least 2 events (added + completed), got %d", len(events))
	}

	// First event should be "added"
	if events[0].Type != "added" {
		t.Errorf("expected first event type 'added', got %q", events[0].Type)
	}

	// Last event should be "updated" with status completed
	lastEvent := events[len(events)-1]
	if lastEvent.Type != "updated" {
		t.Errorf("expected last event type 'updated', got %q", lastEvent.Type)
	}
	lastSnap := lastEvent.Job.Snapshot()
	if lastSnap.Status != "completed" {
		t.Errorf("expected last event job status 'completed', got %q", lastSnap.Status)
	}

	// Verify there is at least one "updated" event (could be progress, running, or completed)
	hasUpdated := false
	for _, e := range events {
		if e.Type == "updated" {
			hasUpdated = true
			break
		}
	}
	if !hasUpdated {
		t.Error("expected at least one 'updated' event")
	}
}

func TestRunActionAsync_HandlerError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "error-plugin", `
plugin = { name = "error-plugin", version = "1.0", description = "error test" }

function do_work(ctx)
    error("boom")
end

function init()
    mah.action({
        id = "error-work",
        label = "Error Work",
        entity = "resource",
        async = true,
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("error-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	jobID, err := pm.RunActionAsync("error-plugin", "error-work", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync: %v", err)
	}

	// Poll until job fails (with timeout).
	deadline := time.After(10 * time.Second)
	for {
		job := pm.GetActionJob(jobID)
		if job == nil {
			t.Fatal("job not found")
		}

		if job.Status == "failed" {
			if job.Message == "" {
				t.Error("expected non-empty error message")
			}
			break
		}

		if job.Status == "completed" {
			t.Fatal("expected job to fail, but it completed")
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job failure, status=%s", job.Status)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestRunActionAsync_JobFail(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "fail-plugin", `
plugin = { name = "fail-plugin", version = "1.0", description = "explicit fail test" }

function do_work(ctx)
    mah.job_progress(ctx.job_id, 30, "working...")
    mah.job_fail(ctx.job_id, "something went wrong")
end

function init()
    mah.action({
        id = "fail-work",
        label = "Fail Work",
        entity = "resource",
        async = true,
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("fail-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	jobID, err := pm.RunActionAsync("fail-plugin", "fail-work", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync: %v", err)
	}

	// Poll until job fails.
	deadline := time.After(10 * time.Second)
	for {
		job := pm.GetActionJob(jobID)
		if job == nil {
			t.Fatal("job not found")
		}

		if job.Status == "failed" {
			if job.Message != "something went wrong" {
				t.Errorf("expected message 'something went wrong', got %q", job.Message)
			}
			break
		}

		if job.Status == "completed" {
			t.Fatal("expected job to fail, but it completed")
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job failure, status=%s", job.Status)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}

func TestRunActionAsync_GetAllJobs(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "multi-plugin", `
plugin = { name = "multi-plugin", version = "1.0", description = "multi test" }

function do_work(ctx)
    mah.job_complete(ctx.job_id, { message = "done" })
end

function init()
    mah.action({
        id = "work-a",
        label = "Work A",
        entity = "resource",
        async = true,
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("multi-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	jobID1, err := pm.RunActionAsync("multi-plugin", "work-a", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync 1: %v", err)
	}

	jobID2, err := pm.RunActionAsync("multi-plugin", "work-a", 2, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync 2: %v", err)
	}

	if jobID1 == jobID2 {
		t.Error("expected different job IDs")
	}

	// Wait for both to complete.
	deadline := time.After(10 * time.Second)
	for {
		jobs := pm.GetAllActionJobs()
		allDone := true
		for i := range jobs {
			if jobs[i].Status != "completed" && jobs[i].Status != "failed" {
				allDone = false
				break
			}
		}
		if allDone && len(jobs) >= 2 {
			break
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for all jobs to complete")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	jobs := pm.GetAllActionJobs()
	if len(jobs) < 2 {
		t.Errorf("expected at least 2 jobs, got %d", len(jobs))
	}
}

func TestRunActionAsync_ValidationError(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "validated-async", `
plugin = { name = "validated-async", version = "1.0", description = "validation test" }

function do_work(ctx)
    mah.job_complete(ctx.job_id, { message = "done" })
end

function init()
    mah.action({
        id = "validated",
        label = "Validated",
        entity = "resource",
        async = true,
        params = {
            { name = "name", type = "text", label = "Name", required = true },
        },
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("validated-async"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	// Missing required param should fail validation before creating a job.
	_, err = pm.RunActionAsync("validated-async", "validated", 1, map[string]any{})
	if err == nil {
		t.Fatal("expected validation error")
	}

	// No jobs should have been created.
	jobs := pm.GetAllActionJobs()
	if len(jobs) != 0 {
		t.Errorf("expected 0 jobs after validation failure, got %d", len(jobs))
	}
}

func TestRunActionAsync_ReturnTable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "return-plugin", `
plugin = { name = "return-plugin", version = "1.0", description = "return table test" }

function do_work(ctx)
    return { message = "auto-completed", data = "yes" }
end

function init()
    mah.action({
        id = "return-work",
        label = "Return Work",
        entity = "resource",
        async = true,
        handler = do_work,
    })
end
`)

	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer pm.Close()

	if err := pm.EnablePlugin("return-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	jobID, err := pm.RunActionAsync("return-plugin", "return-work", 1, map[string]any{})
	if err != nil {
		t.Fatalf("RunActionAsync: %v", err)
	}

	// Poll until completed.
	deadline := time.After(10 * time.Second)
	for {
		job := pm.GetActionJob(jobID)
		if job == nil {
			t.Fatal("job not found")
		}

		if job.Status == "completed" {
			if job.Message != "auto-completed" {
				t.Errorf("expected message 'auto-completed', got %q", job.Message)
			}
			if job.Result == nil {
				t.Fatal("expected result to be non-nil")
			}
			break
		}

		if job.Status == "failed" {
			t.Fatalf("job failed unexpectedly: %s", job.Message)
		}

		select {
		case <-deadline:
			t.Fatalf("timed out waiting for job completion, status=%s", job.Status)
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}
}
