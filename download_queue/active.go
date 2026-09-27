package download_queue

// ActiveDownloadForURL reports an entry that is fetching this URL, for the paths
// that have no durable Job to name: the legacy retry routes and deferred rows
// written before the Job Service.
//
// It is the queue's one busy-URL predicate (activeEntryForURLLocked), the same
// one a submission, a canonical dispatch and a resume are arbitrated by, so no
// two paths can disagree about whether a URL is busy. A held (paused) entry is
// not fetching and holds no URL; a spelling that sends the same request is the
// same URL (TransferKey).
func ActiveDownloadForURL(dm *DownloadManager, url string) (string, bool) {
	if dm == nil || url == "" {
		return "", false
	}
	id := dm.OtherActiveTransfer(url, "")
	return id, id != ""
}
