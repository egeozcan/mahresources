package template_context_providers

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/flosch/pongo2/v4"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
)

// DownloadsPageContext is what /downloads needs: the durable history plus the
// live queue, so a download that is still running appears beside the finished
// ones instead of being invisible until it ends.
type DownloadsPageContext interface {
	contracts.DownloadHistoryReader
	DownloadManager() *download_queue.DownloadManager
}

var downloadStatuses = []SelectOption{
	{Link: "", Title: "All statuses", Active: true},
	{Link: models.DownloadHistoryStatusFailed, Title: "Failed"},
	{Link: models.DownloadHistoryStatusCancelled, Title: "Cancelled"},
	{Link: models.DownloadHistoryStatusCompleted, Title: "Completed"},
}

// downloadRetriedOptions is a single-choice filter, so it goes through
// makeFilterOptions rather than the repeatable list above.
var downloadRetriedOptions = []SelectOption{
	{Link: "", Title: "Retried or not", Active: true},
	{Link: query_models.DownloadRetriedYes, Title: "Retried"},
	{Link: query_models.DownloadRetriedNo, Title: "Never retried"},
}

// downloadRow is one line of the table: a stored row, a live job, or a stored
// row that a live job has since moved past.
type downloadRow struct {
	ID              uint
	JobID           string
	Name            string
	URL             string
	Status          string
	Error           string
	ResourceID      *uint
	TotalSize       int64
	Progress        int64
	Attempts        int
	CreatedAt       time.Time
	CompletedAt     *time.Time
	LastRetryJobID  string
	LastRetryAt     *time.Time
	Live            bool // the queue is still working on this one
	Retryable       bool
	Deletable       bool
	CreatedByUserId *uint
}

// DownloadListContextProvider provides the context for /downloads.
func DownloadListContextProvider(context DownloadsPageContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := StaticTemplateCtx(request)

		var query query_models.DownloadHistoryQuery
		if err := decoder.Decode(&query, request.URL.Query()); err != nil {
			return addErrContext(err, baseContext)
		}
		principal := auth.PrincipalFromContext(request.Context())
		// Set from the principal, after decoding, exactly as the API handler does:
		// these two fields are the visibility decision, not request input.
		query.OwnerUserID, query.OwnerRestricted = downloadHistoryScope(principal)

		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage

		entries, err := context.GetDownloadHistory(int(offset), constants.MaxResultsPerPage, &query)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		count, err := context.GetDownloadHistoryCount(&query)
		if err != nil {
			return addErrContext(err, baseContext)
		}
		if redirect := outOfRangePageRedirect(request, count, constants.MaxResultsPerPage, page); redirect != nil {
			return redirect
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), count, constants.MaxResultsPerPage, int(page))
		if err != nil {
			return addErrContext(err, baseContext)
		}

		rows := buildDownloadRows(entries, liveDownloadJobs(context, principal), &query, page == 1)

		return pongo2.Context{
			"pageTitle": "Downloads",
			// No hideSidebar: the filter form *is* the sidebar on this page, and
			// .content--no-sidebar sets `display: none` on it. Setting it left the
			// page with no way to filter at all except by typing a query string.
			"downloads":        rows,
			"downloadsCount":   count,
			"downloadStatuses": makeMultiFilterOptions(downloadStatuses, query.Status),
			"downloadRetried":  makeFilterOptions(downloadRetriedOptions, query.Retried),
			"pagination":       pagination,
			"queryValues":      request.URL.Query(),
		}.Update(baseContext)
	}
}

// downloadHistoryScope mirrors the API's historyScope and the queue's
// jobVisibleToPrincipal: admins and the auth-off super-user see everything,
// everyone else sees only what they submitted, and an ownerless row is nobody's.
//
// Duplicated rather than shared because those two live in api_handlers, which
// this package must not import. The rule is small; what matters is that it is
// applied to *both* sources below, since the manager hands out every user's jobs.
func downloadHistoryScope(p *auth.Principal) (ownerID *uint, restricted bool) {
	if p == nil || p.IsAdmin() {
		return nil, false
	}
	id := p.UserID
	return &id, true
}

func downloadJobVisible(p *auth.Principal, owner *uint) bool {
	if p == nil || p.IsAdmin() {
		return true
	}
	return owner != nil && *owner == p.UserID
}

// liveDownloadJobs returns the in-flight downloads the caller may see, keyed by
// job id. Terminal jobs are left out: the history row is the better record of
// those, and it is the one carrying the row id the page's actions address.
func liveDownloadJobs(context DownloadsPageContext, p *auth.Principal) map[string]*download_queue.DownloadJob {
	live := map[string]*download_queue.DownloadJob{}
	dm := context.DownloadManager()
	if dm == nil {
		return live
	}
	for _, job := range dm.GetJobs() {
		if job.Source != download_queue.JobSourceDownload {
			continue
		}
		if !downloadJobVisible(p, job.GetOwnerUserID()) {
			continue
		}
		status := job.GetStatus()
		if status == download_queue.JobStatusCompleted || status == download_queue.JobStatusFailed || status == download_queue.JobStatusCancelled {
			continue
		}
		live[job.ID] = job
	}
	return live
}

// buildDownloadRows merges the two sources. The live job wins on a job id both
// know about — the row records how the download ended last time, and the queue
// knows it is running again.
//
// includeLiveOnly is false past the first page: an in-flight download has no
// history row, so it has no place in a paginated slice of the table, and
// repeating it under every page would misreport the count.
func buildDownloadRows(
	entries []models.DownloadHistoryEntry,
	live map[string]*download_queue.DownloadJob,
	query *query_models.DownloadHistoryQuery,
	includeLiveOnly bool,
) []downloadRow {
	rows := make([]downloadRow, 0, len(entries)+len(live))
	seen := make(map[string]struct{}, len(entries))

	for _, e := range entries {
		row := downloadRow{
			ID: e.ID, JobID: e.JobID, Name: e.Name, URL: e.URL,
			Status: e.Status, Error: e.Error, ResourceID: e.ResourceID,
			TotalSize: e.TotalSize, Progress: e.Progress, Attempts: e.Attempts,
			CreatedAt: e.CreatedAt, CompletedAt: e.CompletedAt,
			LastRetryJobID: displayRetryJobID(e.LastRetryJobID), LastRetryAt: e.LastRetryAt,
			CreatedByUserId: e.CreatedByUserId,
		}
		seen[e.JobID] = struct{}{}
		if job, running := live[e.JobID]; running {
			row.Live = true
			row.Status = string(job.GetStatus())
			row.Error = ""
		} else if retry, retrying := live[e.LastRetryJobID]; retrying && e.LastRetryJobID != "" {
			// The attempt this row spawned is a *different* job, so the row's own id
			// says nothing about it: without this the old failure kept its Retry and
			// Delete buttons while its retry ran, and both answered 409.
			row.Live = true
			row.Status = string(retry.GetStatus())
			// Cleared for the same reason the own-job branch clears it: the row now
			// describes an attempt that is running, and the previous attempt's error
			// under a "downloading" pill reads as a download that is failing.
			row.Error = ""
			seen[e.LastRetryJobID] = struct{}{}
		}
		if row.Live && downloadFilterExcludesLive(query) {
			// The stored row matched `Status=failed`; the merge has just relabelled
			// it with the live status, and a page filtered to failures must not then
			// show a row that says "downloading". A status filter excludes in-flight
			// downloads, whether they arrived here as a live job or as a stored row a
			// live job has moved past.
			continue
		}
		row.Retryable = !row.Live && models.DownloadHistoryRetryable(row.Status)
		row.Deletable = !row.Live
		rows = append(rows, row)
	}

	if includeLiveOnly && !downloadFilterExcludesLive(query) {
		for id, job := range live {
			if _, known := seen[id]; known {
				continue
			}
			snap := job.Snapshot()
			// The submitted name, when the download carries one: a live row whose
			// name is its URL cannot be found by the name the user gave it, and the
			// filter box searches both columns for stored rows.
			name := snap.URL
			if creator := job.CreatorCopy(); creator != nil && creator.Name != "" {
				name = creator.Name
			}
			candidate := downloadRow{
				JobID: snap.ID, Name: name, URL: snap.URL,
				Status: string(snap.Status), TotalSize: snap.TotalSize, Progress: snap.Progress,
				CreatedAt: snap.CreatedAt, Live: true,
			}
			if !liveRowMatchesFilter(query, candidate) {
				continue
			}
			rows = append(rows, candidate)
		}
	}

	// Newest first, as the jobs panel sorts, with the job id breaking ties so the
	// order is stable — downloads submitted in one call share a timestamp.
	sort.SliceStable(rows, func(i, j int) bool {
		if !rows[i].CreatedAt.Equal(rows[j].CreatedAt) {
			return rows[i].CreatedAt.After(rows[j].CreatedAt)
		}
		return rows[i].JobID > rows[j].JobID
	})
	return rows
}

// downloadFilterExcludesLive reports whether the active filters rule out
// in-flight downloads as a class. Only the status filter does: it names terminal
// states, and a live row is by definition in none of them. A search term or a
// date range is a question about the row's own fields, so it is asked of the live
// row instead — dropping every live row whenever any filter was set meant that
// searching for a download by name hid the copy of it that was running.
func downloadFilterExcludesLive(query *query_models.DownloadHistoryQuery) bool {
	if query == nil {
		return false
	}
	for _, s := range query.Status {
		if s != "" {
			return true
		}
	}
	return false
}

// displayRetryJobID hides the transient claim marker a retry writes before it
// submits. It is not a job id, and rendering it as one would put "retried as
// claiming-12-17..." on the page for the moment the claim exists (or for good, if
// the submit that should have replaced it died). The literal is duplicated from
// api_handlers, which this package must not import — the same reason the
// visibility predicate above is.
func displayRetryJobID(marker string) string {
	if strings.HasPrefix(marker, "claiming-") {
		return ""
	}
	return marker
}

// liveRowMatchesFilter applies the term and date filters to a live row, matching
// what DownloadHistoryQuery does to the stored ones: the term is a
// case-insensitive substring of the URL or the name, and the dates bound the
// submission time. A date the SQL side would have rejected as malformed drops the
// live row rather than admitting it — the listing beside it is already an error.
func liveRowMatchesFilter(query *query_models.DownloadHistoryQuery, row downloadRow) bool {
	if query == nil {
		return true
	}
	if query.URL != "" {
		term := strings.ToLower(query.URL)
		if !strings.Contains(strings.ToLower(row.URL), term) && !strings.Contains(strings.ToLower(row.Name), term) {
			return false
		}
	}
	// A live-only row has no history row behind it, so it has been retried
	// exactly zero times: it belongs under "never retried" and must not appear
	// under "retried". A stored row a live job has moved past keeps its own
	// counters and was already filtered by SQL.
	switch query.Retried {
	case query_models.DownloadRetriedYes:
		if row.Attempts <= 1 && row.LastRetryJobID == "" {
			return false
		}
	case query_models.DownloadRetriedNo:
		if row.Attempts > 1 || row.LastRetryJobID != "" {
			return false
		}
	}
	if query.CreatedAfter != "" {
		after, ok := parseFilterDate(query.CreatedAfter)
		if !ok || row.CreatedAt.Before(after) {
			return false
		}
	}
	if query.CreatedBefore != "" {
		before, ok := parseFilterDate(query.CreatedBefore)
		if !ok || row.CreatedAt.After(before) {
			return false
		}
	}
	return true
}

// parseFilterDate accepts the two shapes the query models document, in the order
// the SQL comparison sees them: a bare day, and a full RFC 3339 instant.
func parseFilterDate(value string) (time.Time, bool) {
	for _, layout := range []string{"2006-01-02", time.RFC3339} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// makeMultiFilterOptions is makeFilterOptions for a repeatable filter: several
// values can be active at once, so "All" is active only when none are.
func makeMultiFilterOptions(options []SelectOption, current []string) []SelectOption {
	active := make(map[string]bool, len(current))
	for _, v := range current {
		if v != "" {
			active[v] = true
		}
	}
	result := make([]SelectOption, len(options))
	for i, opt := range options {
		result[i] = SelectOption{Link: opt.Link, Title: opt.Title, Active: active[opt.Link]}
	}
	if len(active) == 0 && len(result) > 0 {
		result[0].Active = true
	}
	return result
}
