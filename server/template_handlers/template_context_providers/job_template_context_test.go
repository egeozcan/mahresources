package template_context_providers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"mahresources/jobs"
	"mahresources/server/jobview"
	"mahresources/server/template_handlers/template_entities"
)

func TestJobCenterTemplateContext(t *testing.T) {
	request := httptest.NewRequest("GET", "http://example.test/jobs?state=failed", nil)
	context := JobCenterListContextProvider(nil)(request)

	if got := context["pageTitle"]; got != "Job Center" {
		t.Fatalf("pageTitle = %v, want Job Center", got)
	}
	// The list is a standard list now: its filters live in the sidebar.
	if hidden, _ := context["hideSidebar"].(bool); hidden {
		t.Fatal("the Job list hid the sidebar that holds its filters")
	}
	if got := context["jobCenterCutoverEnabled"]; got != JobCenterCutoverEnabled {
		t.Fatalf("jobCenterCutoverEnabled = %v, want %v", got, JobCenterCutoverEnabled)
	}

	menu, ok := context["menu"].([]template_entities.Entry)
	if !ok {
		t.Fatalf("menu has type %T, want []template_entities.Entry", context["menu"])
	}
	containsJobs := false
	for _, entry := range menu {
		if entry.Name == "Jobs" && entry.Url == "/jobs" {
			containsJobs = true
		}
	}
	if containsJobs != JobCenterCutoverEnabled {
		t.Fatalf("Jobs navigation visible = %v, cutover enabled = %v", containsJobs, JobCenterCutoverEnabled)
	}
}

func TestJobDetailTemplateContext(t *testing.T) {
	request := httptest.NewRequest("GET", "http://example.test/job?id=job-123", nil)
	context := JobDetailContextProvider(nil)(request)

	if got := context["pageTitle"]; got != "Job detail" {
		t.Fatalf("pageTitle = %v, want Job detail", got)
	}
	if got := context["hideSidebar"]; got != true {
		t.Fatalf("hideSidebar = %v, want true", got)
	}
}

type fakeJobListReader struct {
	page        jobs.Page
	pageBefore  jobs.Page
	listed      []jobs.Filter
	listedAfter []jobs.Cursor
	before      []jobs.Cursor
	counted     []jobs.Filter
	counts      func(jobs.Filter) map[string]int64
	outputs     map[string][]jobs.Output
}

func (f *fakeJobListReader) ListJobs(filter jobs.Filter, cursor jobs.Cursor, _ int) (jobs.Page, error) {
	f.listed = append(f.listed, filter)
	f.listedAfter = append(f.listedAfter, cursor)
	return f.page, nil
}

func (f *fakeJobListReader) ListJobsBefore(filter jobs.Filter, before jobs.Cursor, _ int) (jobs.Page, error) {
	f.listed = append(f.listed, filter)
	f.before = append(f.before, before)
	return f.pageBefore, nil
}

func (f *fakeJobListReader) CountJobsByState(filter jobs.Filter) (map[string]int64, error) {
	f.counted = append(f.counted, filter)
	if f.counts == nil {
		return map[string]int64{}, nil
	}
	return f.counts(filter), nil
}

func (f *fakeJobListReader) GetOpenableJobOutputs(jobID string) ([]jobs.Output, error) {
	return f.outputs[jobID], nil
}

func (f *fakeJobListReader) VisibleJobKinds() []string {
	return []string{"group-export", "remote-download"}
}

func renderJobList(t *testing.T, reader *fakeJobListReader, target string) map[string]any {
	t.Helper()
	return jobListContextProvider(reader)(httptest.NewRequest(http.MethodGet, target, nil))
}

func TestJobListDefaultsToTheViewersUndismissedJobs(t *testing.T) {
	reader := &fakeJobListReader{}
	renderJobList(t, reader, "/jobs")
	if got := reader.listed[0].Dismissed; got == nil || *got {
		t.Fatalf("the default list asked Dismissed=%v, want false", got)
	}

	reader = &fakeJobListReader{}
	renderJobList(t, reader, "/jobs?dismissed=any")
	if got := reader.listed[0].Dismissed; got != nil {
		t.Fatalf("dismissed=any asked Dismissed=%v, want no predicate", *got)
	}

	reader = &fakeJobListReader{}
	renderJobList(t, reader, "/jobs?dismissed=true")
	if got := reader.listed[0].Dismissed; got == nil || !*got {
		t.Fatalf("dismissed=true asked Dismissed=%v", got)
	}
}

// TestJobQuickFilterCountsMatchTheRowsTheirLinksOpen pins the sidebar's rule: a
// count applies every filter except the dimension its link sets, and the link
// sets exactly that dimension, keeps the other filters, and drops the page
// position. Clicking an active quick filter clears it.
func TestJobQuickFilterCountsMatchTheRowsTheirLinksOpen(t *testing.T) {
	reader := &fakeJobListReader{counts: func(filter jobs.Filter) map[string]int64 {
		if filter.Pinned != nil && *filter.Pinned {
			return map[string]int64{"succeeded": 2}
		}
		return map[string]int64{"failed": 3, "blocked": 1, "queued": 4, "succeeded": 5}
	}}
	ctx := renderJobList(t, reader, "/jobs?kind=remote-download&state=queued")
	if len(reader.counted) != 2 || reader.counted[0].States != nil || len(reader.counted[0].Kinds) != 1 {
		t.Fatalf("state counts were asked with %+v, want the kind filter and no states", reader.counted)
	}

	filters := ctx["jobQuickFilters"].([]JobQuickFilter)
	byKey := map[string]JobQuickFilter{}
	for _, filter := range filters {
		byKey[filter.Key] = filter
	}
	if got := byKey["attention"].Count; got != 4 {
		t.Errorf("needs attention = %d, want failed 3 + blocked 1", got)
	}
	if got := byKey["active"].Count; got != 4 {
		t.Errorf("active = %d, want queued 4", got)
	}
	if got := byKey["finished"].Count; got != 8 {
		t.Errorf("finished = %d, want succeeded 5 + failed 3", got)
	}
	if got := byKey["pinned"].Count; got != 2 {
		t.Errorf("pinned = %d, want 2", got)
	}

	attention, _ := url.Parse(byKey["attention"].Link)
	if attention.Path != "/jobs" || attention.Query().Get("kind") != "remote-download" ||
		strings.Join(attention.Query()["state"], ",") != "blocked,failed,interrupted" {
		t.Errorf("needs attention link = %s", byKey["attention"].Link)
	}

	active := renderJobList(t, reader, "/jobs?state=queued&state=running&state=paused&state=scheduled")
	for _, filter := range active["jobQuickFilters"].([]JobQuickFilter) {
		if filter.Key != "active" {
			continue
		}
		if !filter.Active {
			t.Fatal("the Active quick filter did not read as chosen for exactly its states")
		}
		if link, _ := url.Parse(filter.Link); len(link.Query()["state"]) != 0 {
			t.Errorf("clicking the chosen quick filter should clear it, got %s", filter.Link)
		}
	}
}

func TestJobListPaginatesByKeyset(t *testing.T) {
	next := jobs.Cursor{AcceptedAt: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), ID: "n"}
	prev := jobs.Cursor{AcceptedAt: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC), ID: "p"}
	nextToken, _ := jobview.EncodeCursor(next)
	prevToken, _ := jobview.EncodeCursor(prev)

	current, _ := url.Parse("/jobs.body?kind=remote-download&cursor=" + prevToken)
	prevLink, nextLink := jobListPageLinks(current, jobs.Page{Next: &next, Prev: &prev})
	for name, link := range map[string]string{"previous": prevLink, "next": nextLink} {
		parsed, _ := url.Parse(link)
		if parsed.Path != "/jobs" || parsed.Query().Get("kind") != "remote-download" {
			t.Errorf("%s link %s must name /jobs, never the .body a live refresh fetched, and keep the filter", name, link)
		}
	}
	if parsed, _ := url.Parse(prevLink); parsed.Query().Get("before") != prevToken || parsed.Query().Has("cursor") {
		t.Errorf("previous link = %s", prevLink)
	}
	if parsed, _ := url.Parse(nextLink); parsed.Query().Get("cursor") != nextToken || parsed.Query().Has("before") {
		t.Errorf("next link = %s", nextLink)
	}

	reader := &fakeJobListReader{page: jobs.Page{Next: &next}}
	if _, present := renderJobList(t, reader, "/jobs")["pagination"]; !present {
		t.Fatal("a page with a next page rendered no pagination")
	}
	reader = &fakeJobListReader{}
	if _, present := renderJobList(t, reader, "/jobs")["pagination"]; present {
		t.Fatal("a single page rendered pagination")
	}

	reader = &fakeJobListReader{pageBefore: jobs.Page{Next: &next}}
	renderJobList(t, reader, "/jobs?before="+prevToken)
	if len(reader.before) != 1 || reader.before[0] != prev || len(reader.listedAfter) != 0 {
		t.Fatalf("before= did not read backwards: before=%+v after=%+v", reader.before, reader.listedAfter)
	}
}

func TestJobListRefusesAnUnreadableFilter(t *testing.T) {
	ctx := renderJobList(t, &fakeJobListReader{}, "/jobs?ownerId=nobody")
	if ctx["_statusCode"] != http.StatusBadRequest {
		t.Fatalf("status = %v, want 400", ctx["_statusCode"])
	}
	if _, ok := ctx["jobFilter"]; !ok {
		t.Fatal("the refused page must still render the filter form so the reader can fix it")
	}
}

func TestJobRowShowsAResultLinkForASucceededJob(t *testing.T) {
	reader := &fakeJobListReader{
		page: jobs.Page{Jobs: []jobs.Snapshot{
			{ID: "ok", Kind: "remote-download", Title: "cat.jpg", State: jobs.StateSucceeded},
			{ID: "bad", Kind: "remote-download", State: jobs.StateFailed, Failure: &jobs.Failure{Message: "404"}},
		}},
		outputs: map[string][]jobs.Output{
			"ok": {{Key: "entity", Type: jobs.OutputTypeEntity, Label: "Created resource", Availability: jobs.OutputAvailable}},
		},
	}
	rows := renderJobList(t, reader, "/jobs")["jobs"].([]JobRow)
	if rows[0].Result.URL != "/v1/jobs/ok/outputs?key=entity" || rows[0].Progress == nil || rows[0].Progress.Percent != 100 {
		t.Fatalf("succeeded row = %+v", rows[0])
	}
	if rows[1].Title != "remote-download" || rows[1].FailureMessage != "404" || rows[1].Progress != nil {
		t.Fatalf("failed row = %+v", rows[1])
	}
}

// TestJobRowProgressNamesEveryBar is findings 41 and 113 on the server-rendered
// card: paused work keeps its known percentage, an unknown total on running work
// is a named indeterminate bar, and a finished download whose size was never
// known reads as complete rather than pulsing forever.
func TestJobRowProgressNamesEveryBar(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }

	paused := jobRowProgress(jobs.Snapshot{State: jobs.StatePaused, Progress: jobs.Progress{Completed: i64(20), Total: i64(50), Unit: "MB"}})
	if paused == nil || !paused.Known || paused.Percent != 40 || paused.Text != "20 / 50 MB" || paused.Indeterminate {
		t.Fatalf("paused = %+v", paused)
	}

	unknown := jobRowProgress(jobs.Snapshot{State: jobs.StateRunning, Progress: jobs.Progress{Completed: i64(7), Unit: "bytes"}})
	if unknown == nil || unknown.Known || !unknown.Indeterminate || unknown.AccessibleText != "7 bytes processed; total unknown" {
		t.Fatalf("unknown total = %+v", unknown)
	}

	finished := jobRowProgress(jobs.Snapshot{State: jobs.StateSucceeded})
	if finished == nil || !finished.Known || finished.Percent != 100 || finished.AccessibleText != "Completed" || finished.Indeterminate {
		t.Fatalf("finished unknown size = %+v", finished)
	}

	stale := jobRowProgress(jobs.Snapshot{State: jobs.StateSucceeded, Progress: jobs.Progress{Completed: i64(70), Total: i64(100), Message: "Fetching"}})
	if stale.Text != "Completed" || stale.Percent != 100 {
		t.Fatalf("a succeeded job's stale percentage = %+v", stale)
	}

	stopped := jobRowProgress(jobs.Snapshot{State: jobs.StateFailed, Progress: jobs.Progress{Completed: i64(3)}})
	if stopped == nil || stopped.Indeterminate {
		t.Fatalf("stopped work must not animate: %+v", stopped)
	}
	if bar := jobRowProgress(jobs.Snapshot{State: jobs.StateFailed}); bar != nil {
		t.Fatalf("a failed job with no progress drew a bar: %+v", bar)
	}
}

// TestJobFilterFormKeepsWhatTheURLAsked covers the form's round trip: every origin
// the URL named stays in the field, and an instant from a bookmark or the legacy
// /downloads translator keeps its time of day rather than widening to a date.
func TestJobFilterFormKeepsWhatTheURLAsked(t *testing.T) {
	form := jobFilterForm(url.Values{
		"origin":         {"api", "plugin,schedule"},
		"acceptedAfter":  {time.Date(2026, 9, 1, 14, 0, 0, 0, time.Local).UTC().Format(time.RFC3339)},
		"acceptedBefore": {"2026-09-02"},
	})
	if form.OriginText != "api, plugin, schedule" {
		t.Errorf("origins = %q", form.OriginText)
	}
	if form.AcceptedAfter != "2026-09-01T14:00" {
		t.Errorf("after = %q, want the instant's local minute", form.AcceptedAfter)
	}
	if form.AcceptedBefore != "2026-09-02T23:59" {
		t.Errorf("before = %q, want the last minute of the day a bare date names", form.AcceptedBefore)
	}

	filter, err := jobListFilter(url.Values{"acceptedBefore": {form.AcceptedBefore}})
	if err != nil {
		t.Fatalf("jobListFilter: %v", err)
	}
	endOfDay := time.Date(2026, 9, 3, 0, 0, 0, 0, time.Local).Add(-time.Nanosecond).UTC()
	if !filter.AcceptedBefore.Equal(endOfDay) {
		t.Errorf("resubmitting the shown value = %v, want the same end of day %v", filter.AcceptedBefore, endOfDay)
	}
}

func TestJobCommandOptionsKeepEveryFilterableKey(t *testing.T) {
	options := jobCommandOptions("")
	for _, key := range []string{"retry", "inspect", "retry-import", "forget"} {
		if !slices.Contains(options, key) {
			t.Errorf("the command filter does not offer %q: %v", key, options)
		}
	}
	if kept := jobCommandOptions("future-key"); kept[len(kept)-1] != "future-key" {
		t.Errorf("a key the URL names was dropped from the select: %v", kept)
	}
}

func TestJobKindOptionsKeepAKindTheURLNames(t *testing.T) {
	ctx := renderJobList(t, &fakeJobListReader{}, "/jobs?kind=retired-kind&kind=group-export")
	options := ctx["jobKindOptions"].([]string)
	if !slices.Contains(options, "retired-kind") || !slices.Contains(options, "group-export") || !slices.Contains(options, "remote-download") {
		t.Fatalf("kind options = %v: a Kind the URL names must stay offered, or resubmitting drops it", options)
	}
}

func TestJobRowTimesShareOneZone(t *testing.T) {
	accepted := time.Date(2026, 9, 1, 15, 0, 0, 0, time.UTC)
	started := accepted.Add(time.Minute)
	row := jobRow(&fakeJobListReader{}, jobs.Snapshot{ID: "z", AcceptedAt: accepted, StartedAt: &started, State: jobs.StateRunning})
	want := accepted.In(time.Local).Format("2006-01-02 15:04")
	if row.Accepted.Minute != want || row.Started.Display[:16] != started.In(time.Local).Format("2006-01-02 15:04") {
		t.Fatalf("accepted %q / started %q, want both in the server's zone (%q)", row.Accepted.Minute, row.Started.Display, want)
	}
}
