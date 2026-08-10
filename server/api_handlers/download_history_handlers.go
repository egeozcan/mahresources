package api_handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
)

// DownloadHistoryContext is what the /v1/downloads endpoints need: the durable
// history, plus the live queue (a retry may reach a job that is still in memory,
// and a delete has to take the queue entry with it) and the group-visibility
// check the retry path re-applies.
type DownloadHistoryContext interface {
	contracts.DownloadHistoryReader
	contracts.DownloadHistoryWriter
	DownloadManager() *download_queue.DownloadManager
	GroupVisible(id uint) bool
	NoteVisible(id uint) bool
}

// downloadHistoryPageSize is how many rows one listing returns. The page is a
// management surface rather than a feed, so it uses the app's standard page size.
const downloadHistoryPageSize = constants.MaxResultsPerPage

// historyScope resolves the caller's visibility into the owner predicate the
// query scope applies. It is the durable-store twin of jobVisibleToPrincipal:
// admins and the auth-off super-user see everything, every other principal sees
// only the downloads it submitted, and a row with no owner is nobody's.
func historyScope(p *auth.Principal) (ownerID *uint, restricted bool) {
	if p == nil || p.IsAdmin() {
		return nil, false
	}
	id := p.UserID
	return &id, true
}

// idListRequest is the body shape shared by the two bulk endpoints. Ids arrive as
// a JSON array or as repeated `ids` form fields, so both the page's fetch calls
// and a plain curl work.
type idListRequest struct {
	IDs []uint `json:"ids" schema:"ids"`
}

// bulkResult is one id's outcome. Bulk actions report per id rather than failing
// as a unit: a retry of twelve rows can exhaust the queue at the fifth, and
// answering "503" for the whole request would hide the seven that were accepted
// and the four that were not.
type bulkResult struct {
	ID     uint   `json:"id"`
	OK     bool   `json:"ok"`
	JobID  string `json:"jobId,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// GetDownloadHistoryListHandler handles GET /v1/downloads.
func GetDownloadHistoryListHandler(ctx DownloadHistoryContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.DownloadHistoryQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		// Overwritten unconditionally, after decoding: the two owner fields are the
		// visibility decision, and a caller must not be able to set them.
		query.OwnerUserID, query.OwnerRestricted = historyScope(auth.PrincipalFromContext(request.Context()))

		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * downloadHistoryPageSize

		entries, err := ctx.GetDownloadHistory(int(offset), downloadHistoryPageSize, &query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}
		count, err := ctx.GetDownloadHistoryCount(&query)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"downloads": entries,
			"count":     count,
			"page":      page,
			"pageSize":  downloadHistoryPageSize,
		})
	}
}

// GetDownloadHistoryRetryHandler handles POST /v1/downloads/retry.
//
// A row whose job is still in the queue is retried in place, so the panel shows
// the attempt on the row it already has. One whose job has been evicted or swept
// is resubmitted from the stored payload — which is the whole reason the payload
// is stored.
func GetDownloadHistoryRetryHandler(ctx DownloadHistoryContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		entries, principal, ok := loadRequestedEntries(ctx, writer, request)
		if !ok {
			return
		}

		owner := principalOwnerID(principal)
		results := make([]bulkResult, 0, len(entries))
		accepted := 0
		// One download per URL per batch. Selecting a failure and the failure of the
		// retry it spawned is an ordinary thing to do — they are two rows — and
		// retrying both would run the same transfer twice at once.
		batchURLs := make(map[string]uint, len(entries))

		for i := range entries {
			entry := &entries[i]
			res := bulkResult{ID: entry.ID}

			if first, repeated := batchURLs[entry.URL]; repeated && entry.URL != "" {
				res.Reason = fmt.Sprintf("the same download is already being retried as row %d in this request", first)
				results = append(results, res)
				continue
			}

			if !models.DownloadHistoryRetryable(entry.Status) {
				// Completed downloads are excluded deliberately: the file is already
				// stored, so running the download again would transfer it for nothing
				// — content-hash deduplication would then refuse the resource it made.
				res.Reason = fmt.Sprintf("a %s download cannot be retried", entry.Status)
				results = append(results, res)
				continue
			}

			creator, err := ctx.DownloadHistoryPayload(entry)
			if err != nil {
				res.Reason = err.Error()
				results = append(results, res)
				continue
			}

			// The stored payload is re-validated against the principal doing the
			// retrying, not the one that submitted it. Otherwise a payload written
			// while a user was unscoped — or by somebody else entirely — would be a
			// way around the check the submit endpoint applies.
			if err := validateDownloadScope(ctx, principal, creator); err != nil {
				res.Reason = err.Error()
				results = append(results, res)
				continue
			}

			jobID, err := retryOrResubmit(ctx, entry, creator, owner)
			if err != nil {
				res.Reason = err.Error()
				results = append(results, res)
				continue
			}

			res.OK, res.JobID = true, jobID
			accepted++
			if entry.URL != "" {
				batchURLs[entry.URL] = entry.ID
			}

			if err := ctx.MarkDownloadHistoryRetried(entry.ID, jobID, time.Now()); err != nil {
				// Reported, not failed: the download is already queued, and answering
				// with an error would invite the user to press Retry again and start a
				// second one. Set before the append — bulkResult is a value, so
				// mutating it afterwards would write to a copy the response never sees.
				res.Reason = "queued, but the retry could not be recorded"
			}

			results = append(results, res)
		}

		respondBulk(writer, request, results, accepted, "retried")
	}
}

// retryOrResubmit re-runs one stored download and returns the job id that is now
// carrying it.
func retryOrResubmit(ctx DownloadHistoryContext, entry *models.DownloadHistoryEntry, creator *query_models.ResourceFromRemoteCreator, owner *uint) (string, error) {
	dm := ctx.DownloadManager()
	if dm == nil {
		return "", fmt.Errorf("the download queue is unavailable")
	}

	// The attempt a previous resubmission produced. A row keeps its own (terminal)
	// status while the attempt launched from it runs, so without this the second
	// press of Retry submits a third download of the same URL — the same fork the
	// in-place branch below refuses, reached through the other door.
	switch marker, state := linkedRetry(ctx, entry); state {
	case retryLinkClaiming:
		return "", fmt.Errorf("this download is already being retried")
	case retryLinkRunning:
		return "", fmt.Errorf("this download is already queued as %s; wait for it to finish", marker)
	case retryLinkSucceeded:
		return "", fmt.Errorf("this download already succeeded as %s; retrying it would fetch a second copy", marker)
	}

	// Before the in-place branch, not after it: two rows can name two different
	// jobs that were fetching one URL, and retrying each in place ran both at once.
	// The queue is the authority on what is being downloaded, so it is asked first,
	// whichever way this row is about to be run again.
	if live, running := activeDownloadForURL(dm, entry.URL); running {
		return "", fmt.Errorf("this URL is already downloading as %s; wait for it to finish", live)
	}

	if job, exists := dm.GetJob(entry.JobID); exists {
		// A job that is present but not retryable is one that is already queued or
		// running — the row still says `failed` because the row records how the
		// download *ended*, and nothing has ended yet. Refused rather than
		// resubmitted: falling through to Submit here started a second, concurrent
		// download of the same URL, which is how one Retry from the jobs panel plus
		// one from this page produced two copies of the same file.
		if !job.CanRetry() {
			return "", fmt.Errorf("this download is already queued; wait for it to finish")
		}
		if err := dm.Retry(entry.JobID); err != nil {
			return "", err
		}
		return entry.JobID, nil
	}

	// The row's retry slot is claimed before the download is submitted, not
	// recorded after it: two requests that both find no live attempt would
	// otherwise both submit, and the duplicate is created by the submit. The token
	// is unique per attempt, so exactly one compare-and-set can win.
	claim := fmt.Sprintf("%s%d-%d", downloadRetryClaimPrefix, entry.ID, time.Now().UnixNano())
	claimed, err := ctx.ClaimDownloadHistoryRetry(entry.ID, entry.LastRetryJobID, entry.Status, claim, time.Now())
	if err != nil {
		return "", err
	}
	if !claimed {
		return "", fmt.Errorf("this download is already being retried")
	}

	job, err := dm.Submit(creator, owner)
	if err != nil {
		// Hand the slot back, or a queue that was momentarily full would leave the
		// row claimed by an attempt that never started.
		_, _ = ctx.ClaimDownloadHistoryRetry(entry.ID, claim, "", entry.LastRetryJobID, time.Now())
		return "", err
	}
	return job.ID, nil
}

// The retry slot's claim marker. A claim is written before the download is
// submitted and replaced by the real job id immediately after, so a marker still
// carrying the prefix means some other request is inside that window — unless it
// has been there longer than the TTL, in which case the submit died and the slot
// is abandoned rather than held.
const (
	downloadRetryClaimPrefix = "claiming-"
	downloadRetryClaimTTL    = time.Minute
)

// What the attempt a row already spawned is doing now.
type retryLinkState int

const (
	retryLinkNone      retryLinkState = iota // no attempt, or one that ended badly and may be run again
	retryLinkClaiming                        // another request is between claiming the slot and submitting
	retryLinkRunning                         // queued, downloading or paused
	retryLinkSucceeded                       // it completed: the file it downloaded exists
)

// linkedRetry reports the attempt a resubmission of this row produced, and what
// it is doing.
//
// LastRetryJobID is what ties a row to that attempt: a resubmitted download is a
// *new* job with a new id, so the row's own job id says nothing about it. Both
// the retry and the delete path need the answer — one to avoid forking the
// download, the other to avoid discarding a row whose attempt is still running.
//
// The succeeded case is read from the store rather than the queue, because the
// queue forgets a finished job within the hour while the reason not to run the
// download again — the file it produced — does not expire.
func linkedRetry(ctx DownloadHistoryContext, entry *models.DownloadHistoryEntry) (string, retryLinkState) {
	marker := entry.LastRetryJobID
	if marker == "" || marker == entry.JobID {
		return "", retryLinkNone
	}

	if strings.HasPrefix(marker, downloadRetryClaimPrefix) {
		if entry.LastRetryAt != nil && time.Since(*entry.LastRetryAt) < downloadRetryClaimTTL {
			return marker, retryLinkClaiming
		}
		return marker, retryLinkNone
	}

	if dm := ctx.DownloadManager(); dm != nil {
		if job, exists := dm.GetJob(marker); exists {
			if job.IsActive() || job.GetStatus() == download_queue.JobStatusPaused {
				return marker, retryLinkRunning
			}
			// Deliberately not `!job.CanRetry()`: that is false for a *completed*
			// job too, so treating it as "still running" both said something untrue
			// and unblocked itself the moment the queue evicted the job.
			if job.GetStatus() == download_queue.JobStatusCompleted {
				return marker, retryLinkSucceeded
			}
			return marker, retryLinkNone
		}
	}

	if status, err := ctx.DownloadHistoryStatusForJob(marker); err == nil && status == models.DownloadHistoryStatusCompleted {
		return marker, retryLinkSucceeded
	}
	return marker, retryLinkNone
}

// activeDownloadForURL reports a job that is already fetching this URL.
//
// The last line of defence against running one transfer twice, and the general
// form of the rules above: `last_retry_job_id` links a row to the attempt it
// spawned, but the link can be lost (a marker written for a submit whose bookkeeping
// update failed) or simply absent (two rows recording the same URL, each retryable
// on its own). The queue itself always knows what it is downloading.
func activeDownloadForURL(dm *download_queue.DownloadManager, url string) (string, bool) {
	if dm == nil || url == "" {
		return "", false
	}
	for _, job := range dm.GetJobs() {
		if job.Source != download_queue.JobSourceDownload || job.URL != url {
			continue
		}
		// Snapshots, so this reads the status the copy carries rather than taking
		// the live job's lock.
		if job.Status == download_queue.JobStatusPaused || job.Status == download_queue.JobStatusPending ||
			job.Status == download_queue.JobStatusDownloading || job.Status == download_queue.JobStatusProcessing {
			return job.ID, true
		}
	}
	return "", false
}

// GetDownloadHistoryDeleteHandler handles POST /v1/downloads/delete.// GetDownloadHistoryDeleteHandler handles POST /v1/downloads/delete.
//
// The queue entry goes with the row. Deleting only the row would leave the job
// in memory, and the SSE stream replays the whole queue on connect — so the
// download the page said was deleted would reappear in the jobs panel on the next
// reconnect.
func GetDownloadHistoryDeleteHandler(ctx DownloadHistoryContext) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		loadedAt := time.Now()
		entries, principal, ok := loadRequestedEntries(ctx, writer, request)
		if !ok {
			return
		}

		dm := ctx.DownloadManager()
		visible := func(owner *uint) bool { return jobVisibleToPrincipal(principal, owner) }

		// Two passes, with the queue removal between them. A row is refused while
		// anything it owns is still in flight — its own job, or the job a retry of
		// it produced — and deciding that from a status read *before* asking the
		// queue to drop the entry is check-then-act: a retry landing in the gap
		// left RemoveFinished correctly refusing while the row was deleted anyway,
		// so the download carried on and rewrote the row the user had just
		// discarded. What the queue actually released is the answer.
		results := make([]bulkResult, 0, len(entries))
		refused := make(map[uint]string, len(entries))
		jobIDs := make([]string, 0, len(entries))

		for i := range entries {
			entry := &entries[i]
			switch marker, state := linkedRetry(ctx, entry); state {
			case retryLinkClaiming:
				refused[entry.ID] = "this download is being retried right now; try again once it has started"
				continue
			case retryLinkRunning:
				refused[entry.ID] = fmt.Sprintf("the retry of this download (%s) is still running; cancel it first", marker)
				continue
			}
			if entry.JobID != "" {
				jobIDs = append(jobIDs, entry.JobID)
			}
		}

		released := map[string]struct{}{}
		if dm != nil && len(jobIDs) > 0 {
			for _, id := range dm.RemoveFinished(jobIDs, visible) {
				released[id] = struct{}{}
			}
		}

		deletable := make([]uint, 0, len(entries))
		for i := range entries {
			entry := &entries[i]
			res := bulkResult{ID: entry.ID}

			if reason, denied := refused[entry.ID]; denied {
				res.Reason = reason
				results = append(results, res)
				continue
			}

			// A paused download holds a half-transferred file, and the answer to
			// "delete this" is "cancel it first". Still present after RemoveFinished
			// means exactly that: it refused to release it.
			if dm != nil {
				if _, stillQueued := dm.GetJob(entry.JobID); stillQueued {
					if _, gone := released[entry.JobID]; !gone {
						res.Reason = "this download is still running; cancel it first"
						results = append(results, res)
						continue
					}
				}
			}

			deletable = append(deletable, entry.ID)
			res.OK = true
			results = append(results, res)
		}

		ownerID, restricted := historyScope(principal)
		// loadedAt, so a retry claimed after these rows were read keeps its row: the
		// decision to delete was made from a copy that predates the claim.
		deleted, err := ctx.DeleteDownloadHistoryEntries(deletable, ownerID, restricted, loadedAt)
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
			return
		}

		// Only when the counts disagree, which means the guard above kept a row: say
		// so per id rather than reporting a deletion that did not happen.
		if int(deleted) != len(deletable) {
			if survivors, err := ctx.GetDownloadHistoryEntries(deletable, ownerID, restricted); err == nil {
				kept := make(map[uint]struct{}, len(survivors))
				for i := range survivors {
					kept[survivors[i].ID] = struct{}{}
				}
				for i := range results {
					if _, stayed := kept[results[i].ID]; stayed && results[i].OK {
						results[i].OK = false
						results[i].Reason = "a retry of this download started while it was being deleted"
					}
				}
			}
		}

		respondBulk(writer, request, results, int(deleted), "deleted")
	}
}

// loadRequestedEntries decodes the id list and loads the rows the caller may
// see. It writes the error response itself and reports whether the handler
// should continue.
//
// Ids the caller may not see are simply absent, so a 404-shaped answer covers
// both "no such row" and "not yours" — row ids cannot be probed.
func loadRequestedEntries(ctx DownloadHistoryContext, writer http.ResponseWriter, request *http.Request) ([]models.DownloadHistoryEntry, *auth.Principal, bool) {
	var req idListRequest
	if err := tryFillStructValuesFromRequest(&req, request); err != nil {
		http_utils.HandleError(err, writer, request, http.StatusBadRequest)
		return nil, nil, false
	}
	if len(req.IDs) == 0 {
		http_utils.HandleError(fmt.Errorf("at least one id is required"), writer, request, http.StatusBadRequest)
		return nil, nil, false
	}

	principal := auth.PrincipalFromContext(request.Context())
	ownerID, restricted := historyScope(principal)
	entries, err := ctx.GetDownloadHistoryEntries(req.IDs, ownerID, restricted)
	if err != nil {
		http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusInternalServerError))
		return nil, nil, false
	}
	if len(entries) == 0 {
		http_utils.HandleError(fmt.Errorf("no matching downloads"), writer, request, http.StatusNotFound)
		return nil, nil, false
	}
	return entries, principal, true
}

// respondBulk writes the per-id outcomes.
//
// 409 when nothing was accepted, so a single-row action — which is what most of
// these requests are — gets a status code that says what happened rather than a
// 200 the caller has to read the body to interpret. A mixed batch stays 200: some
// of it worked, and the results say which.
func respondBulk(writer http.ResponseWriter, request *http.Request, results []bulkResult, accepted int, verb string) {
	body := map[string]any{verb: accepted, "results": results}

	if accepted == 0 {
		// 409 with the outcomes still attached. A refusal is per row — "this one is
		// running, that one already succeeded" — and answering a twelve-row request
		// with one sentence threw eleven of those answers away.
		reason := fmt.Sprintf("no downloads could be %s", verb)
		if len(results) == 1 && results[0].Reason != "" {
			reason = results[0].Reason
		}
		body["error"] = reason
		writer.Header().Set("Content-Type", constants.JSON)
		writer.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(writer).Encode(body)
		return
	}

	writer.Header().Set("Content-Type", constants.JSON)
	_ = json.NewEncoder(writer).Encode(body)
}
