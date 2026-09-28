package application_context

import (
	"testing"

	"mahresources/jobs"
	"mahresources/models/query_models"
)

// An HLS download counts segments from the moment it knows how many there are
// until it finishes. The playlist's own Content-Length is the size of a few
// lines of text, not of the video, so it must never become the Job's total;
// and the assembly after the last segment must neither drop the bar back to
// zero nor switch the Job to another unit, which would erase the history its
// speed and graph are drawn from.
func TestAnHLSDownloadCountsItsSegmentsToTheEnd(t *testing.T) {
	ffmpeg := hlsTestFfmpeg(t)
	ctx := newDownloadJobContext(t)
	ctx.Config.FfmpegPath = ffmpeg
	srv := serveDir(t, buildHLSStream(t, ffmpeg))

	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: srv.URL + "/index.m3u8"}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submit: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	done := waitForSnapshot(t, ctx, jobID, "the HLS download to finish",
		func(snap jobs.Snapshot) bool { return snap.State.Terminal() })
	if done.State != jobs.StateSucceeded {
		t.Fatalf("the download ended %s (%+v)", done.State, done.Failure)
	}

	progress := done.Progress
	if progress.Unit != "items" || progress.Completed == nil || progress.Total == nil ||
		*progress.Total < 1 || *progress.Completed != *progress.Total {
		t.Fatalf("final progress = %+v (completed %v, total %v); want every segment of the total", progress, progress.Completed, progress.Total)
	}
	resources, err := ctx.GetResources(0, 10, &query_models.ResourceSearchQuery{})
	if err != nil || len(resources) != 1 {
		t.Fatalf("resources = %d (%v); want the one assembled video", len(resources), err)
	}
	size := metricByKey(progress.Metrics, "size")
	if size == nil || size.Unit != "bytes" || size.Value != float64(resources[0].FileSize) {
		t.Fatalf("final metrics = %+v; want the video's size, %d bytes", progress.Metrics, resources[0].FileSize)
	}

	series := done.ProgressSeries
	if series.Unit != "items" {
		t.Fatalf("series unit = %q; want the segment count's history kept to the end", series.Unit)
	}
	counted := 0
	for _, point := range series.Points {
		if point.Completed != nil {
			counted++
		}
	}
	if counted == 0 {
		t.Fatalf("series = %+v; no point holds a segment count", series.Points)
	}
}
