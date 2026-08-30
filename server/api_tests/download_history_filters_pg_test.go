//go:build postgres

package api_tests

import (
	"net/http"
	"reflect"
	"sort"
	"testing"
	"time"

	"mahresources/download_queue"
	"mahresources/models"
)

// The download filters, against a real Postgres.
//
// SetupTestEnv is SQLite even under the postgres build tag, so the ordinary
// download-history tests say nothing about this engine — and these filters are
// exactly where the two diverge: the error box and the reason buckets go through
// GetLikeOperator (ILIKE here, LIKE there), and an RFC 3339 date bound is
// normalised in Go for a timestamptz column here and rewritten with strftime
// there. A comma fraction is the sharp edge: Go's RFC 3339 admits it, Postgres
// rejects it as timestamptz input, and passing the caller's text straight
// through answered a documented value with a 500.
func TestDownloadHistoryFiltersOnPostgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	monday := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	wednesday := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	// Between the two, and just after the instant the +23:00 bound below names.
	// Without it that assertion proves nothing about offset handling: an
	// implementation that clamped the offset to Postgres's own +15:00 limit
	// would name 08:00Z instead of midnight and still return the same rows.
	justAfterMidnight := time.Date(2026, 3, 3, 4, 0, 0, 0, time.UTC)
	recordDownload(t, tc, "early", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/early.bin", monday)
	recordDownload(t, tc, "midnightish", models.DownloadHistoryStatusCompleted, nil, "http://example.invalid/mid.bin", justAfterMidnight)
	late := recordDownload(t, tc, "late", models.DownloadHistoryStatusFailed, nil, "http://example.invalid/late.bin", wednesday)
	if err := tc.DB.Model(&models.DownloadHistoryEntry{}).Where("id = ?", late.ID).
		Update("error", "HTTP 404: 404 Not Found").Error; err != nil {
		t.Fatalf("set the error text: %v", err)
	}
	// One submission date for both rows. recordDownload derives CreatedAt from
	// the completion time, which put each row's two timestamps on the same side
	// of every boundary below — so a filter secretly still reading created_at
	// would have satisfied every assertion in this test.
	submitted := time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC)
	if err := tc.DB.Model(&models.DownloadHistoryEntry{}).
		Where("job_id IN ?", []string{"early", "midnightish", "late"}).
		Update("created_at", submitted).Error; err != nil {
		t.Fatalf("align the submission dates: %v", err)
	}

	jobIDs := func(query string) []string {
		t.Helper()
		res := doReq(tc, http.MethodGet, "/v1/downloads?"+query, nil, nil, nil)
		if res.Code != http.StatusOK {
			t.Fatalf("%s: status %d (%s)", query, res.Code, res.Body.String())
		}
		out := []string{}
		for _, row := range decodeJSON(t, res)["downloads"].([]any) {
			out = append(out, row.(map[string]any)["jobId"].(string))
		}
		sort.Strings(out)
		return out
	}

	for query, want := range map[string][]string{
		"completedAfter=2026-03-03":                 {"late", "midnightish"},
		"completedBefore=2026-03-03":                {"early"},
		"completedAfter=2026-03-03T00%3A00%3A00Z":   {"late", "midnightish"},
		"completedBefore=2026-03-03T00%3A00%3A00Z":  {"early"},
		"completedAfter=2026-03-03T00%3A00%3A00,5Z": {"late", "midnightish"},
		// Sub-second precision is truncated on SQLite and kept here, so the
		// assertions that distinguish the two engines stay inside one second.
		"completedAfter=2026-03-03T00%3A00%3A00.500Z":     {"late", "midnightish"},
		"completedBefore=2026-03-03T02%3A00%3A00%2B02:00": {"early"},
		"reason=http":  {"late"},
		"reason=other": {},
		// ILIKE here, LIKE there: the same casing must answer the same on both.
		"error=NOT+FOUND": {"late"},
		"error=not+found": {"late"},
		// A wildcard is a literal on both engines.
		"error=%25": {},
		// Bytes Postgres refuses inside a text parameter (SQLSTATE 22021): a NUL,
		// and invalid UTF-8. Both were an HTTP 500 on either search box rather
		// than "no matches".
		"error=%00": {},
		"url=%00":   {},
		"error=%FF": {},
		"url=%FF":   {},
	} {
		if got := jobIDs(query); !reflect.DeepEqual(got, want) {
			t.Errorf("?%s = %v, want %v", query, got, want)
		}
	}

	// A download that never finished, to prove the "restricts nothing" end of an
	// out-of-range bound still keeps an unfinished row out of a finish window.
	if err := tc.AppCtx.RecordTerminalDownload(download_queue.HistoryRecord{
		JobID: "unfinished", URL: "http://example.invalid/unfinished.bin", Name: "unfinished",
		Status: models.DownloadHistoryStatusFailed, CreatedAt: monday,
	}); err != nil {
		t.Fatalf("record the unfinished row: %v", err)
	}

	// Bounds Go's RFC 3339 admits and Postgres does not: year zero is SQLSTATE
	// 22008, an offset past ±15:59 is 22009, and both reached the query as
	// written before. Each is answered by which end of the range it is, so the
	// rows matter as much as the status — asserting only "not a 500" would pass
	// against a filter that had silently become no filter at all.
	for query, want := range map[string][]string{
		"completedAfter=0000-01-01T00%3A00%3A00Z":          {"early", "late", "midnightish"},
		"completedAfter=0000-01-01":                        {"early", "late", "midnightish"},
		"completedBefore=0000-01-01T00%3A00%3A00Z":         {},
		"completedBefore=9999-12-31T23%3A00%3A00-14%3A00":  {"early", "late", "midnightish"},
		"completedAfter=9999-12-31T23%3A00%3A00-14%3A00":   {},
		"completedAfter=2026-03-03T23%3A00%3A00%2B23%3A00": {"late", "midnightish"},
	} {
		if got := jobIDs(query); !reflect.DeepEqual(got, want) {
			t.Errorf("?%s = %v, want %v — the unfinished row belongs in no finish window", query, got, want)
		}
	}
}
