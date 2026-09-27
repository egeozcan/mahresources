package download_queue

// ActiveDownloadForURL reports a job that is already fetching this URL, in any
// spelling that sends the same request (TransferKey).
//
// The queue is the authority on one-transfer-per-URL checks: history rows can
// lose the marker that links them to a retry attempt, and scheduled downloads do
// not have a job id yet. This helper is shared by the retry path and by deferred
// downloads so the status predicate cannot drift.
func ActiveDownloadForURL(dm *DownloadManager, url string) (string, bool) {
	if dm == nil || url == "" {
		return "", false
	}
	key := TransferKey(url)
	for _, job := range dm.GetJobs() {
		if job.Source != JobSourceDownload || TransferKey(job.URL) != key {
			continue
		}
		if job.Status == JobStatusPaused || job.Status == JobStatusPending ||
			job.Status == JobStatusDownloading || job.Status == JobStatusProcessing {
			return job.ID, true
		}
	}
	return "", false
}
