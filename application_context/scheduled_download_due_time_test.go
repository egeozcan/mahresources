package application_context

import (
	"fmt"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

// A deferred download's due time reaches the database in two forms. `start_at`
// arrives as unix seconds and is converted to UTC; `delay` is added to the
// server's local clock. Rows written by earlier releases are stored either way,
// and the scheduler compares them with its own local clock, so the comparison has
// to be between instants, not between the texts SQLite stores. The zones below
// are the server's local zone: one east of UTC, one west of it, and the most
// easterly offset in use.
var deferredDueTimeZones = []*time.Location{
	time.FixedZone("UTC+2", 2*60*60),
	time.FixedZone("UTC-5", -5*60*60),
	time.FixedZone("UTC+14", 14*60*60),
}

// deferredDueTimeRow is one row of the scenario: a due time relative to the
// server's clock, and whether it was written in UTC (the start_at form) or in the
// server's zone (the delay form).
type deferredDueTimeRow struct {
	name   string
	offset time.Duration
	utc    bool
}

var deferredDueTimeRows = []deferredDueTimeRow{
	{name: "start_at, 40 minutes ago", offset: -40 * time.Minute, utc: true},
	{name: "delay, 30 minutes ago", offset: -30 * time.Minute},
	{name: "delay, 80 minutes ahead", offset: 80 * time.Minute},
	{name: "start_at, 90 minutes ahead", offset: 90 * time.Minute, utc: true},
}

// seedDeferredDueTimeRows writes the scenario's rows as a writer in the server's
// zone would, straight to the table: what any release stored, not what this one
// would choose to store.
func seedDeferredDueTimeRows(t *testing.T, ctx *MahresourcesContext, now time.Time, owner uint) map[string]uint {
	t.Helper()
	ids := make(map[string]uint, len(deferredDueTimeRows))
	for i, spec := range deferredDueTimeRows {
		due := now.Add(spec.offset)
		if spec.utc {
			due = due.UTC()
		}
		url := fmt.Sprintf("https://example.invalid/due-%d.bin", i)
		row := models.ScheduledDownload{
			PluginName:      "feeds",
			URL:             url,
			Payload:         mustScheduledPayload(t, &query_models.ResourceFromRemoteCreator{URL: url}),
			DueAt:           due,
			Status:          models.ScheduledDownloadStatusPending,
			CreatedByUserId: &owner,
		}
		if err := ctx.db.Create(&row).Error; err != nil {
			t.Fatalf("seed %s: %v", spec.name, err)
		}
		ids[spec.name] = row.ID
	}
	return ids
}

func fireDeferredDueTimeRows(t *testing.T, ctx *MahresourcesContext, now time.Time) int {
	t.Helper()
	fired, err := ctx.FireDueScheduledDownloads(ScheduledDownloadFireConfig{
		Now:             now,
		PluginAvailable: func(string) bool { return true },
		Submit: func(creator *query_models.ResourceFromRemoteCreator, _ *uint, _ string) (string, error) {
			return "queue-" + creator.URL, nil
		},
	})
	if err != nil {
		t.Fatalf("fire due rows: %v", err)
	}
	return fired
}

// assertDeferredDueTimeRowsFireOnTime runs the sweep at the server's now and
// again two hours later, and checks each row went out exactly when it was due.
func assertDeferredDueTimeRowsFireOnTime(t *testing.T, ctx *MahresourcesContext, zone *time.Location) {
	t.Helper()
	owner := createDownloadOwner(t, ctx)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC).In(zone)
	ids := seedDeferredDueTimeRows(t, ctx, now, owner.ID)

	if fired := fireDeferredDueTimeRows(t, ctx, now); fired != 2 {
		t.Errorf("the sweep at the server's now fired %d rows, want the 2 that were due", fired)
	}
	for _, spec := range deferredDueTimeRows {
		row := scheduledDownloadRow(t, ctx, ids[spec.name])
		want := models.ScheduledDownloadStatusPending
		if spec.offset <= 0 {
			want = models.ScheduledDownloadStatusSubmitted
		}
		if row.Status != want {
			t.Errorf("%s is %s after the sweep at the server's now, want %s", spec.name, row.Status, want)
		}
	}

	if fired := fireDeferredDueTimeRows(t, ctx, now.Add(2*time.Hour)); fired != 2 {
		t.Errorf("the sweep two hours later fired %d rows, want the 2 that had come due", fired)
	}
	for _, spec := range deferredDueTimeRows {
		if row := scheduledDownloadRow(t, ctx, ids[spec.name]); row.Status != models.ScheduledDownloadStatusSubmitted {
			t.Errorf("%s is %s two hours later, want submitted", spec.name, row.Status)
		}
	}
}

// assertDeferredDueTimeRowsListInDueOrder checks the management listing orders
// rows by when they are due, not by how their due time was written.
func assertDeferredDueTimeRowsListInDueOrder(t *testing.T, ctx *MahresourcesContext, zone *time.Location) {
	t.Helper()
	owner := createDownloadOwner(t, ctx)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC).In(zone)
	ids := seedDeferredDueTimeRows(t, ctx, now, owner.ID)
	rows, err := ctx.PluginScheduledDownloadsFor("feeds")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != len(deferredDueTimeRows) {
		t.Fatalf("listed %d rows, want %d", len(rows), len(deferredDueTimeRows))
	}
	for i, spec := range deferredDueTimeRows {
		if rows[i].ID != ids[spec.name] {
			got := make([]string, len(rows))
			for j, row := range rows {
				got[j] = row.DueAt.UTC().Format(time.RFC3339)
			}
			t.Fatalf("listing order by due time = %v, want %s at position %d", got, spec.name, i)
		}
	}
}

// assertDeferredClaimAgeIsAnInstant checks the stale-claim rule, which compares
// claimed_at the same way: a claim ten seconds old is live and must not be taken,
// and one two minutes old is abandoned and must be.
func assertDeferredClaimAgeIsAnInstant(t *testing.T, ctx *MahresourcesContext, zone *time.Location) {
	t.Helper()
	owner := createDownloadOwner(t, ctx)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC).In(zone)
	live := seedScheduledDownload(t, ctx, now.Add(-time.Hour), &owner.ID)
	stale := seedScheduledDownload(t, ctx, now.Add(-time.Hour), &owner.ID)
	for id, claimedAt := range map[uint]time.Time{
		live.ID:  now.Add(-10 * time.Second).UTC(),
		stale.ID: now.Add(-2 * time.Minute).UTC(),
	} {
		if err := ctx.db.Model(&models.ScheduledDownload{}).Where("id = ?", id).
			Updates(map[string]any{"claim_token": "another-process", "claimed_at": claimedAt}).Error; err != nil {
			t.Fatalf("seed claim: %v", err)
		}
	}
	if claimed, err := ctx.ClaimScheduledDownload(live.ID, "this-process", now); err != nil || claimed {
		t.Errorf("claim over a 10-second-old claim = %v, %v; want refused", claimed, err)
	}
	if claimed, err := ctx.ClaimScheduledDownload(stale.ID, "this-process", now); err != nil || !claimed {
		t.Errorf("claim over a 2-minute-old claim = %v, %v; want taken", claimed, err)
	}
}

func TestADeferredDownloadFiresAtItsDueTimeInEveryZone(t *testing.T) {
	for _, zone := range deferredDueTimeZones {
		t.Run(zone.String(), func(t *testing.T) {
			assertDeferredDueTimeRowsFireOnTime(t, newScheduledDownloadTestContext(t), zone)
		})
	}
}

func TestDeferredDownloadsListInDueOrderInEveryZone(t *testing.T) {
	for _, zone := range deferredDueTimeZones {
		t.Run(zone.String(), func(t *testing.T) {
			assertDeferredDueTimeRowsListInDueOrder(t, newScheduledDownloadTestContext(t), zone)
		})
	}
}

func TestADeferredDownloadClaimAgesByTheClockInEveryZone(t *testing.T) {
	for _, zone := range deferredDueTimeZones {
		t.Run(zone.String(), func(t *testing.T) {
			assertDeferredClaimAgeIsAnInstant(t, newScheduledDownloadTestContext(t), zone)
		})
	}
}

// A row due a fraction of a millisecond from now is not due yet. SQLite's
// julianday resolves milliseconds, rounding, so a bare comparison would promote
// the Job before the time its own claim waits for.
func TestADeferredDownloadIsNotDueAFractionOfAMillisecondEarly(t *testing.T) {
	ctx := newScheduledDownloadTestContext(t)
	owner := createDownloadOwner(t, ctx)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	row := seedScheduledDownload(t, ctx, now.Add(400*time.Microsecond), &owner.ID)
	if fired := fireDeferredDueTimeRows(t, ctx, now); fired != 0 {
		t.Fatalf("the sweep fired %d rows 0.4ms before their time, want none", fired)
	}
	if claimed, err := ctx.ClaimScheduledDownload(row.ID, "early", now); err != nil || claimed {
		t.Fatalf("claim 0.4ms before the due time = %v, %v; want refused", claimed, err)
	}
	if fired := fireDeferredDueTimeRows(t, ctx, now.Add(2*time.Millisecond)); fired != 1 {
		t.Fatalf("the sweep fired %d rows once they were due, want 1", fired)
	}
}
