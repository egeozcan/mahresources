package application_context

import (
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/hls"
	"mahresources/jobs"
)

func metricByKey(metrics []jobs.Metric, key string) *jobs.Metric {
	for i := range metrics {
		if metrics[i].Key == key {
			return &metrics[i]
		}
	}
	return nil
}

func TestDownloadJobProgressChoosesAMeasureWithATotalWhenThereIsOne(t *testing.T) {
	sized := downloadJobProgress(&download_queue.DownloadJob{
		Status: download_queue.JobStatusDownloading, Progress: 400, TotalSize: 1000,
	})
	if sized.Unit != "bytes" || sized.Completed == nil || *sized.Completed != 400 || sized.Total == nil || *sized.Total != 1000 {
		t.Fatalf("sized transfer = %+v; want 400 of 1000 bytes", sized)
	}

	// A chunked response has no size but still has a speed.
	chunked := downloadJobProgress(&download_queue.DownloadJob{
		Status: download_queue.JobStatusDownloading, Progress: 400, TotalSize: -1,
	})
	if chunked.Unit != "bytes" || chunked.Completed == nil || *chunked.Completed != 400 || chunked.Total != nil {
		t.Fatalf("chunked transfer = %+v; want 400 bytes of an unknown total", chunked)
	}

	// An HLS stream's size is unknown until the end; its segments have a total.
	hls := downloadJobProgress(&download_queue.DownloadJob{
		Status: download_queue.JobStatusDownloading, Progress: 5 << 20, TotalSize: -1,
		Phase: "fetching segments", PhaseCount: 12, PhaseTotal: 40,
	})
	if hls.Unit != "items" || hls.Completed == nil || *hls.Completed != 12 || hls.Total == nil || *hls.Total != 40 {
		t.Fatalf("HLS transfer = %+v; want 12 of 40 segments as the measure", hls)
	}
	if downloaded := metricByKey(hls.Metrics, "downloaded"); downloaded == nil || downloaded.Value != 5<<20 || downloaded.Unit != "bytes" {
		t.Fatalf("HLS metrics = %+v; want the bytes kept as a metric", hls.Metrics)
	}
	if hls.Message != "fetching segments" || hls.Phase != "downloading" {
		t.Fatalf("HLS phase/message = %q/%q", hls.Phase, hls.Message)
	}

	// Once assembled the queue knows the video's size, and the segments still
	// come first: switching to bytes at the end would erase the Job's history.
	// The byte counters now hold the video, not what crossed the network, so
	// the metric says which figure it is.
	assembled := downloadJobProgress(&download_queue.DownloadJob{
		Status: download_queue.JobStatusProcessing, Progress: 9 << 20, TotalSize: 9 << 20,
		Phase: "assembling video", PhaseCount: 40, PhaseTotal: 40,
	})
	if assembled.Unit != "items" || *assembled.Completed != 40 || *assembled.Total != 40 {
		t.Fatalf("assembled HLS transfer = %+v; want 40 of 40 segments", assembled)
	}
	if size := metricByKey(assembled.Metrics, "size"); size == nil || size.Value != 9<<20 || size.Label != "Video size" || size.Unit != "bytes" {
		t.Fatalf("assembled HLS metrics = %+v; want the video's size", assembled.Metrics)
	}
	if downloaded := metricByKey(assembled.Metrics, "downloaded"); downloaded != nil {
		t.Fatalf("assembled HLS metrics = %+v; the video's size is not the bytes received", assembled.Metrics)
	}

	queued := downloadJobProgress(&download_queue.DownloadJob{Status: download_queue.JobStatusPending})
	if queued.Completed != nil || queued.Total != nil || len(queued.Metrics) != 0 {
		t.Fatalf("a queued transfer reported progress: %+v", queued)
	}
}

func TestDownloadJobProgressCarriesOnlyActiveHLSByteHeartbeats(t *testing.T) {
	activityAt := time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC)
	snap := &download_queue.DownloadJob{
		Status: download_queue.JobStatusDownloading,
		Phase:  hls.PhaseSegments, PhaseCount: 2, PhaseTotal: 20,
		Progress: 4096, ProgressActivity: true, ProgressActivityAt: activityAt,
	}
	progress := downloadJobProgress(snap)
	if !progress.Activity {
		t.Fatal("active segment-byte snapshot lost its sampler activity hint")
	}
	if progress.ActivityAt == nil || !progress.ActivityAt.Equal(activityAt) {
		t.Fatalf("active segment-byte snapshot activity time = %v, want original read time %v", progress.ActivityAt, activityAt)
	}

	for _, tc := range []struct {
		name string
		edit func(*download_queue.DownloadJob)
	}{
		{"assembly", func(s *download_queue.DownloadJob) { s.Phase = hls.PhaseMuxing }},
		{"paused", func(s *download_queue.DownloadJob) { s.Status = download_queue.JobStatusPaused }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := *snap
			tc.edit(&copy)
			progress := downloadJobProgress(&copy)
			if progress.Activity || progress.ActivityAt != nil {
				t.Fatalf("%s snapshot retained active segment heartbeat", tc.name)
			}
			if paused := pausedDownloadProgress(progress); paused.Activity {
				t.Fatalf("paused progress retained active heartbeat: %+v", paused)
			}
		})
	}
}

func TestQueueJobProgressKeepsAnExportsItemCountAsAMetric(t *testing.T) {
	progress := queueJobProgress(&download_queue.DownloadJob{
		Source: download_queue.JobSourceGroupExport, Phase: "resources",
		Progress: 2048, TotalSize: 8192, PhaseCount: 3, PhaseTotal: 10,
	})
	if progress.Unit != "bytes" || progress.Completed == nil || *progress.Completed != 2048 {
		t.Fatalf("export progress = %+v; want bytes as the measure", progress)
	}
	items := metricByKey(progress.Metrics, "items")
	if items == nil || items.Value != 3 || items.Total == nil || *items.Total != 10 {
		t.Fatalf("export metrics = %+v; want 3 of 10 items", progress.Metrics)
	}

	// A tick that only moves the item count is still a change worth writing.
	moved := queueJobProgress(&download_queue.DownloadJob{
		Source: download_queue.JobSourceGroupExport, Phase: "resources",
		Progress: 2048, TotalSize: 8192, PhaseCount: 4, PhaseTotal: 10,
	})
	if sameProgress(progress, moved) {
		t.Fatal("sameProgress ignored a metric change, so the item count would never be published")
	}
	if !sameProgress(progress, queueJobProgress(&download_queue.DownloadJob{
		Source: download_queue.JobSourceGroupExport, Phase: "resources",
		Progress: 2048, TotalSize: 8192, PhaseCount: 3, PhaseTotal: 10,
	})) {
		t.Fatal("identical snapshots compared as different")
	}
}
