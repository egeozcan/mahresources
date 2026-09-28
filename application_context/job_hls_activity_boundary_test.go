package application_context

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/hls"
	"mahresources/jobs"
	"mahresources/models/query_models"
)

type hlsActivityObservation struct {
	at      time.Time
	queue   *download_queue.DownloadJob
	durable jobs.Snapshot
}

type hlsActivityObserver struct {
	*jobDownloadSink
	updates chan hlsActivityObservation
}

func (s *hlsActivityObserver) DownloadProgress(ref download_queue.CanonicalRef, queueJob *download_queue.DownloadJob) error {
	if err := s.jobDownloadSink.DownloadProgress(ref, queueJob); err != nil {
		return err
	}
	snapshot, err := s.ctx.GetJob(ref.JobID)
	if err != nil {
		return err
	}
	select {
	case s.updates <- hlsActivityObservation{at: time.Now(), queue: queueJob, durable: snapshot}:
	default:
	}
	return nil
}

// A separate encrypted audio rendition fetches its key after video segment
// counts are mirrored. Hold every audio body, and let the video response wait
// after its final payload bytes so its completed count is independently
// observed. Only real segment-body reads may keep the canonical count rate
// fresh through the queue and durable Jobs service.
func TestCanonicalHLSAudioKeyDoesNotRenewSegmentRateFreshness(t *testing.T) {
	ffmpeg := hlsTestFfmpeg(t)
	dir := buildHLSStream(t, ffmpeg)

	keyPath := filepath.Join(dir, "audio.key")
	if err := os.WriteFile(keyPath, []byte("0123456789abcdef"), 0o600); err != nil {
		t.Fatal(err)
	}
	keyInfo := filepath.Join(dir, "audio-key-info")
	if err := os.WriteFile(keyInfo, []byte("audio.key\n"+keyPath+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=2",
		"-c:a", "aac", "-vn", "-f", "hls", "-hls_time", "1", "-hls_list_size", "0",
		"-hls_key_info_file", keyInfo,
		"-hls_segment_filename", filepath.Join(dir, "a%d.ts"),
		filepath.Join(dir, "audio.m3u8"),
	).CombinedOutput(); err != nil {
		t.Fatalf("building encrypted audio rendition: %v\n%s", err, output)
	}

	master := "#EXTM3U\n" +
		`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="aac",NAME="English",DEFAULT=YES,URI="audio.m3u8"` + "\n" +
		`#EXT-X-STREAM-INF:BANDWIDTH=200000,RESOLUTION=160x120,AUDIO="aac"` + "\nvideo.m3u8\n"
	releaseAudio := make(chan struct{})
	var releaseOnce sync.Once
	freeAudio := func() { releaseOnce.Do(func() { close(releaseAudio) }) }
	t.Cleanup(freeAudio)
	audioKeyStarted := make(chan struct{})
	var audioKeyStartedOnce sync.Once
	audioStarted := make(chan struct{})
	var audioStartedOnce sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		switch name {
		case "master.m3u8":
			_, _ = fmt.Fprint(w, master)
			return
		case "video.m3u8":
			http.ServeFile(w, r, filepath.Join(dir, "index.m3u8"))
			return
		case "audio.key":
			audioKeyStartedOnce.Do(func() { close(audioKeyStarted) })
			time.Sleep(1300 * time.Millisecond)
			http.ServeFile(w, r, keyPath)
			return
		}
		if strings.HasPrefix(name, "a") && strings.HasSuffix(name, ".ts") {
			audioStartedOnce.Do(func() { close(audioStarted) })
			select {
			case <-releaseAudio:
			case <-r.Context().Done():
				return
			}
		}
		if strings.HasPrefix(name, "s") && strings.HasSuffix(name, ".ts") {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			half := len(data) / 2
			_, _ = w.Write(data[:half])
			w.(http.Flusher).Flush()
			time.Sleep(650 * time.Millisecond)
			_, _ = w.Write(data[half:])
			w.(http.Flusher).Flush()
			time.Sleep(650 * time.Millisecond)
			return
		}
		http.ServeFile(w, r, filepath.Join(dir, name))
	}))
	t.Cleanup(server.Close)

	ctx := newJobHarnessContext(t, false)
	ctx.Config.FfmpegPath = ffmpeg
	ctx.downloadManager.Shutdown()
	ctx.downloadManager = download_queue.NewDownloadManagerWithConfig(ctx,
		download_queue.NewStaticDownloadSettings(download_queue.TimeoutConfig{OverallTimeout: 40 * time.Second}, 0),
		download_queue.ManagerConfig{Concurrency: 1, FfmpegPath: func() string { return ffmpeg }, HLSOptions: hls.Options{Concurrency: 1}},
	)
	observer := &hlsActivityObserver{jobDownloadSink: &jobDownloadSink{ctx: ctx}, updates: make(chan hlsActivityObservation, 128)}
	ctx.downloadManager.SetCanonicalSink(observer)
	runtime := NewJobRuntime(ctx, ctx.JobService(), JobRuntimeConfig{Claimant: "canonical-hls-activity-boundary", Interval: 50 * time.Millisecond})
	runtime.Start()
	t.Cleanup(runtime.Stop)

	submissions := ctx.SubmitRemoteDownloads(&query_models.ResourceFromRemoteCreator{URL: server.URL + "/master.m3u8"}, nil, "", "api")
	if len(submissions) != 1 || submissions[0].Err != nil {
		t.Fatalf("submitting HLS download: %+v", submissions)
	}
	jobID := submissions[0].CanonicalJobID
	select {
	case <-audioKeyStarted:
	case <-time.After(10 * time.Second):
		freeAudio()
		t.Fatal("the encrypted audio key was never requested")
	}

	var observations []hlsActivityObservation
	collect := func() {
		for {
			select {
			case observation := <-observer.updates:
				observations = append(observations, observation)
			default:
				return
			}
		}
	}
	collect()
	var videoComplete *hlsActivityObservation
	for i := range observations {
		observation := &observations[i]
		if observation.queue.Phase == hls.PhaseSegments &&
			observation.queue.PhaseCount > 0 &&
			observation.queue.PhaseCount < observation.queue.PhaseTotal &&
			observation.durable.ProgressSeries.Anchor != nil {
			videoComplete = observation
		}
	}
	if videoComplete == nil {
		freeAudio()
		t.Fatalf("no independently mirrored video count with a rate anchor before the audio key; observations=%+v", observations)
	}
	if rate := videoComplete.durable.LiveRate(videoComplete.at); rate == nil || *rate <= 0 {
		freeAudio()
		t.Fatalf("completed video count had no live rate before the audio key: %v", rate)
	}
	if eta, estimated := videoComplete.durable.ExpectedFinish(videoComplete.at); eta == nil || !estimated {
		freeAudio()
		t.Fatalf("completed video count had no estimated ETA before the audio key: %v estimated=%v", eta, estimated)
	}
	select {
	case <-audioStarted:
	case <-time.After(5 * time.Second):
		freeAudio()
		t.Fatal("the encrypted audio segment body was never held")
	}

	var keyOnly *hlsActivityObservation
	deadline := time.After(3 * time.Second)
	for keyOnly == nil {
		select {
		case observation := <-observer.updates:
			observations = append(observations, observation)
			if observation.queue.Phase == hls.PhaseSegments &&
				observation.queue.PhaseCount == videoComplete.queue.PhaseCount &&
				observation.queue.Progress > videoComplete.queue.Progress {
				copy := observation
				keyOnly = &copy
			}
		case <-deadline:
			collect()
			freeAudio()
			t.Fatalf("the audio-key byte increase was not mirrored while its segment body was held; video=%+v observations=%+v", videoComplete, observations)
		}
	}
	if keyOnly.queue.ProgressActivity {
		freeAudio()
		t.Fatalf("audio key traffic renewed segment activity without an audio body read: %+v", keyOnly.queue)
	}
	if keyOnly.durable.ProgressSeries.Anchor == nil || keyOnly.durable.ProgressSeries.Anchor.At != videoComplete.durable.ProgressSeries.Anchor.At {
		freeAudio()
		t.Fatalf("audio key changed the completed-count anchor: before=%+v after=%+v", videoComplete.durable.ProgressSeries.Anchor, keyOnly.durable.ProgressSeries.Anchor)
	}
	oldAnchor := time.UnixMilli(videoComplete.durable.ProgressSeries.Anchor.At)
	if wait := time.Until(oldAnchor.Add(10*time.Second + 150*time.Millisecond)); wait > 0 {
		timer := time.NewTimer(wait)
		defer timer.Stop()
		<-timer.C
	}
	current, err := ctx.GetJob(jobID)
	if err != nil {
		freeAudio()
		t.Fatal(err)
	}
	now := time.Now()
	if rate := current.LiveRate(now); rate != nil {
		freeAudio()
		t.Fatalf("key-only traffic kept canonical rate %g fresh %s after the count anchor", *rate, now.Sub(oldAnchor))
	}
	if eta, estimated := current.ExpectedFinish(now); eta != nil || estimated {
		freeAudio()
		t.Fatalf("key-only traffic kept canonical ETA %v estimated=%v fresh after the count anchor", eta, estimated)
	}

	freeAudio()
	if done := waitForSnapshot(t, ctx, jobID, "the released encrypted audio download", func(s jobs.Snapshot) bool { return s.State.Terminal() }); done.State != jobs.StateSucceeded {
		t.Fatalf("released HLS download ended as %s: %+v", done.State, done.Failure)
	}
}
