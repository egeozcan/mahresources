package template_context_providers

import (
	"fmt"
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
	listErr     error
}

func (f *fakeJobListReader) ListJobs(filter jobs.Filter, cursor jobs.Cursor, _ int) (jobs.Page, error) {
	f.listed = append(f.listed, filter)
	f.listedAfter = append(f.listedAfter, cursor)
	return f.page, f.listErr
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

// TestJobListWritesItsUndismissedDefaultIntoTheAddress pins how the page's
// default meets the API's: a navigation without a dismissed parameter is sent
// to the same URL with dismissed=false, keeping every other parameter, so each
// URL the page shows a list under reads the same on /v1/jobs and the CLI, which
// apply no dismissal filter when the parameter is absent.
func TestJobListWritesItsUndismissedDefaultIntoTheAddress(t *testing.T) {
	for target, want := range map[string]string{
		"/jobs":                                  "/jobs?dismissed=false",
		"/jobs?dismissed=":                       "/jobs?dismissed=false",
		"/jobs?state=failed&search=a+b&view=all": "/jobs?dismissed=false&search=a+b&state=failed&view=all",
		"/jobs?cursor=list-v1.x&kind=export&kind=import": "/jobs?cursor=list-v1.x&dismissed=false&kind=export&kind=import",
	} {
		reader := &fakeJobListReader{}
		ctx := renderJobList(t, reader, target)
		if got := ctx["_redirect"]; got != want {
			t.Errorf("%s redirected to %v, want %s", target, got, want)
		}
		if len(reader.listed) != 0 {
			t.Errorf("%s listed Jobs before redirecting", target)
		}
	}

	// A fetched variant is answered in place with the page's default, and a
	// named value is never redirected.
	for _, target := range []string{"/jobs.body?state=failed", "/jobs.json", "/jobs?dismissed=false", "/jobs?dismissed=any", "/jobs?dismissed=true"} {
		ctx := renderJobList(t, &fakeJobListReader{}, target)
		if got, ok := ctx["_redirect"]; ok {
			t.Errorf("%s redirected to %v", target, got)
		}
	}
	// A request the renderer answers as JSON is answered in place, whatever
	// else its Accept header names: the renderer serves it JSON, and a redirect
	// would answer it 302 instead.
	for _, accept := range []string{"application/json", "application/json, text/html", "text/html, application/json;q=0.9"} {
		jsonRequest := httptest.NewRequest(http.MethodGet, "/jobs?state=failed", nil)
		jsonRequest.Header.Set("Accept", accept)
		if got, ok := jobListContextProvider(&fakeJobListReader{})(jsonRequest)["_redirect"]; ok {
			t.Errorf("a request accepting %q was redirected to %v", accept, got)
		}
	}
}

func TestJobListDefaultsToTheViewersUndismissedJobs(t *testing.T) {
	reader := &fakeJobListReader{}
	ctx := renderJobList(t, reader, "/jobs.body")
	if got := reader.listed[0].Dismissed; got == nil || *got {
		t.Fatalf("the default list asked Dismissed=%v, want false", got)
	}
	if form := ctx["jobFilter"].(JobFilterForm); form.Dismissed != "false" {
		t.Fatalf("the filter form shows Dismissed=%q, want the default in effect", form.Dismissed)
	}

	reader = &fakeJobListReader{}
	renderJobList(t, reader, "/jobs?dismissed=false")
	if got := reader.listed[0].Dismissed; got == nil || *got {
		t.Fatalf("dismissed=false asked Dismissed=%v, want false", got)
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
	ctx := renderJobList(t, reader, "/jobs?kind=remote-download&state=queued&dismissed=false")
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
	if attention.Path != "/jobs" || attention.Query().Get("kind") != "remote-download" || attention.Query().Get("dismissed") != "false" ||
		strings.Join(attention.Query()["state"], ",") != "blocked,failed,interrupted" {
		t.Errorf("needs attention link = %s", byKey["attention"].Link)
	}

	active := renderJobList(t, reader, "/jobs?state=queued&state=running&state=paused&state=scheduled&dismissed=false")
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

	current, _ := url.Parse("/jobs.body?kind=remote-download&dismissed=false&cursor=" + prevToken)
	prevLink, nextLink := jobListPageLinks(current, jobs.Page{Next: &next, Prev: &prev})
	for name, link := range map[string]string{"previous": prevLink, "next": nextLink} {
		parsed, _ := url.Parse(link)
		if parsed.Path != "/jobs" || parsed.Query().Get("kind") != "remote-download" || parsed.Query().Get("dismissed") != "false" {
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
	if _, present := renderJobList(t, reader, "/jobs?dismissed=false")["pagination"]; !present {
		t.Fatal("a page with a next page rendered no pagination")
	}
	reader = &fakeJobListReader{}
	if _, present := renderJobList(t, reader, "/jobs?dismissed=false")["pagination"]; present {
		t.Fatal("a single page rendered pagination")
	}

	reader = &fakeJobListReader{pageBefore: jobs.Page{Next: &next}}
	renderJobList(t, reader, "/jobs?dismissed=false&before="+prevToken)
	if len(reader.before) != 1 || reader.before[0] != prev || len(reader.listedAfter) != 0 {
		t.Fatalf("before= did not read backwards: before=%+v after=%+v", reader.before, reader.listedAfter)
	}
}

func TestJobListRefusesAnUnreadableFilter(t *testing.T) {
	ctx := renderJobList(t, &fakeJobListReader{}, "/jobs?ownerId=nobody&dismissed=false")
	if ctx["_statusCode"] != http.StatusBadRequest {
		t.Fatalf("status = %v, want 400", ctx["_statusCode"])
	}
	if _, ok := ctx["jobFilter"]; !ok {
		t.Fatal("the refused page must still render the filter form so the reader can fix it")
	}
}

// A filter the service refuses reads on the page as the API reads it: the
// problem in the reader's terms, without the service's internal wrapping.
func TestJobListRefusalNamesOnlyTheFilterProblem(t *testing.T) {
	refused := fmt.Errorf("jobs: list: %w", fmt.Errorf("%w: unknown state %q", jobs.ErrInvalidFilter, "bogus"))
	ctx := renderJobList(t, &fakeJobListReader{listErr: refused}, "/jobs?state=bogus&dismissed=false")
	if ctx["_statusCode"] != http.StatusBadRequest {
		t.Fatalf("status = %v, want 400", ctx["_statusCode"])
	}
	if got := ctx["errorMessage"]; got != `invalid filter: unknown state "bogus"` {
		t.Fatalf("errorMessage = %q", got)
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
	rows := renderJobList(t, reader, "/jobs?dismissed=false")["jobs"].([]JobRow)
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

	// The amount is not the bar's label: it leads the stats line, formatted.
	paused := jobRowProgress(jobs.Snapshot{State: jobs.StatePaused, Progress: jobs.Progress{Completed: i64(20), Total: i64(50), Unit: "MB"}})
	if paused == nil || !paused.Known || paused.Percent != 40 || paused.Text != "" || paused.Indeterminate ||
		paused.AccessibleText != "20 of 50 MB" || paused.Stats != "20 of 50 MB" {
		t.Fatalf("paused = %+v", paused)
	}
	sized := jobRowProgress(jobs.Snapshot{State: jobs.StateRunning, Progress: jobs.Progress{
		Completed: i64(8051532), Total: i64(20971520), Unit: "bytes", Message: "downloading"}})
	if sized.Text != "downloading" || sized.AccessibleText != "downloading; 7.6 MB of 20.0 MB" || sized.Stats != "7.6 MB of 20.0 MB" {
		t.Fatalf("sized download = %+v", sized)
	}

	unknown := jobRowProgress(jobs.Snapshot{State: jobs.StateRunning, Progress: jobs.Progress{Completed: i64(240350), Unit: "bytes"}})
	if unknown == nil || unknown.Known || !unknown.Indeterminate || unknown.AccessibleText != "Working; 235 KB processed; total unknown" || unknown.Stats != "235 KB" {
		t.Fatalf("unknown total = %+v", unknown)
	}

	// A rebuild's phase is not what a finished card says, nor its count twice.
	rebuilt := jobRowProgress(jobs.Snapshot{State: jobs.StateSucceeded, Progress: jobs.Progress{
		Completed: i64(11), Total: i64(11), Unit: "items", Message: "recomputing"}})
	if rebuilt.Text != "Completed" || rebuilt.Percent != 100 || rebuilt.Stats != "11 items" {
		t.Fatalf("finished rebuild = %+v", rebuilt)
	}

	// Stopped work whose total was never known keeps its amount, with its unit,
	// and draws no fill: neither Known nor Indeterminate.
	cancelled := jobRowProgress(jobs.Snapshot{State: jobs.StateCancelled, Progress: jobs.Progress{Completed: i64(240350), Unit: "bytes"}})
	if cancelled == nil || cancelled.Known || cancelled.Indeterminate || cancelled.Stats != "235 KB" {
		t.Fatalf("cancelled download of unknown size = %+v", cancelled)
	}

	finished := jobRowProgress(jobs.Snapshot{State: jobs.StateSucceeded})
	if finished == nil || !finished.Known || finished.Percent != 100 || finished.AccessibleText != "Completed" || finished.Indeterminate {
		t.Fatalf("finished unknown size = %+v", finished)
	}

	// A partial success is not "Completed": the bar says what the badge says.
	partial := jobRowProgress(jobs.Snapshot{State: jobs.StateSucceeded, Phase: jobs.PhasePartial})
	if partial == nil || partial.Text != "Partially completed" || partial.AccessibleText != "Partially completed" {
		t.Fatalf("partial success with no progress = %+v", partial)
	}
	// A finished total and the run's last message still read as partial, with
	// the message kept: it is usually what says how much is left.
	partialDone := jobRowProgress(jobs.Snapshot{State: jobs.StateSucceeded, Phase: jobs.PhasePartial,
		Progress: jobs.Progress{Completed: i64(3), Total: i64(3), Message: "Did 3 of 9 shares."}})
	if partialDone.Text != "Partially completed: Did 3 of 9 shares." || partialDone.AccessibleText != partialDone.Text || partialDone.Percent != 100 {
		t.Fatalf("partial success with a finished total = %+v", partialDone)
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

// TestOnlyRunningWorkReadsAsWorking pins what a card says about work nobody is
// doing: a scheduled or queued Job with nothing to report draws no bar at all, a
// paused one keeps what it reports without pulsing, and a failed Job's last
// report is never called "Working". Running work with nothing to report is the
// one bar that is indeterminate.
func TestOnlyRunningWorkReadsAsWorking(t *testing.T) {
	i64 := func(v int64) *int64 { return &v }
	for _, state := range []jobs.State{jobs.StateScheduled, jobs.StateQueued, jobs.StatePaused, jobs.StateBlocked, jobs.StateFailed, jobs.StateInterrupted, jobs.StateCancelled} {
		if bar := jobRowProgress(jobs.Snapshot{State: state}); bar != nil {
			t.Fatalf("a %s Job with nothing to report drew %+v", state, bar)
		}
		if bar := jobRowProgress(jobs.Snapshot{State: state, Phase: "queued", Progress: jobs.Progress{Phase: "queued"}}); bar != nil {
			t.Fatalf("a %s Job whose only report is its phase drew %+v", state, bar)
		}
		if bar := jobRowProgress(jobs.Snapshot{State: state, Progress: jobs.Progress{Total: i64(100)}}); bar != nil {
			t.Fatalf("a %s Job whose only report is a total drew %+v", state, bar)
		}
	}
	running := jobRowProgress(jobs.Snapshot{State: jobs.StateRunning})
	if running == nil || running.Text != "Working" || !running.Indeterminate {
		t.Fatalf("running work with nothing to report = %+v", running)
	}
	waiting := jobRowProgress(jobs.Snapshot{State: jobs.StateQueued, Progress: jobs.Progress{Message: "Waiting for another download of this URL to finish"}})
	if waiting == nil || waiting.Indeterminate || waiting.Text != "Waiting for another download of this URL to finish" {
		t.Fatalf("queued work that says why it waits = %+v", waiting)
	}
	paused := jobRowProgress(jobs.Snapshot{State: jobs.StatePaused, Progress: jobs.Progress{Completed: i64(7), Unit: "bytes", Message: "Paused"}})
	if paused == nil || paused.Indeterminate || paused.Text != "Paused" {
		t.Fatalf("paused work with an unknown total = %+v", paused)
	}
	failed := jobRowProgress(jobs.Snapshot{State: jobs.StateFailed, Progress: jobs.Progress{Completed: i64(7), Unit: "bytes"}})
	if failed == nil || failed.Indeterminate || failed.Text == "Working" {
		t.Fatalf("failed work's last report = %+v", failed)
	}
}

// TestAStoppedJobThatReportedOnlyMetricsShowsThem: a metric is a report, on the
// card as on the Job Center, and the card's text names the first one rather than
// calling stopped work "Working".
func TestAStoppedJobThatReportedOnlyMetricsShowsThem(t *testing.T) {
	total := float64(40)
	metrics := []jobs.Metric{
		{Key: "downloaded", Label: "Downloaded", Value: 12 * 1024 * 1024, Unit: "bytes"},
		{Key: "segments", Label: "Segments", Value: 4, Total: &total},
	}
	failed := jobRowProgress(jobs.Snapshot{State: jobs.StateFailed, Progress: jobs.Progress{Metrics: metrics}})
	if failed == nil || failed.Indeterminate || failed.Text != "Downloaded: 12.0 MB" {
		t.Fatalf("a failed Job that reported only metrics = %+v", failed)
	}
	if got := jobMetricSummary(metrics[1]); got != "Segments: 4 of 40" {
		t.Fatalf("a metric with a total reads %q", got)
	}
	running := jobRowProgress(jobs.Snapshot{State: jobs.StateRunning, Progress: jobs.Progress{Metrics: metrics}})
	if running == nil || running.Text != "Working" || !running.Indeterminate {
		t.Fatalf("running work that reported only metrics = %+v", running)
	}
}

// TestJobRowSaysWhenScheduledWorkStarts: the time is what tells one scheduled
// Job from another, and the card shows it, in the same zone as its other times.
func TestJobRowSaysWhenScheduledWorkStarts(t *testing.T) {
	due := time.Date(2026, 9, 28, 12, 17, 0, 0, time.UTC)
	row := jobRow(&fakeJobListReader{}, jobs.Snapshot{ID: "s", Kind: "deferred-download", State: jobs.StateScheduled, ScheduledFor: &due})
	if row.ScheduledFor.ISO == "" || row.ScheduledFor.ISO != due.In(time.Local).Format(time.RFC3339) {
		t.Fatalf("the scheduled row names its start as %+v", row.ScheduledFor)
	}
	if row.Progress != nil {
		t.Fatalf("scheduled work drew a progress bar: %+v", row.Progress)
	}
	started := jobRow(&fakeJobListReader{}, jobs.Snapshot{ID: "r", Kind: "deferred-download", State: jobs.StateRunning, ScheduledFor: &due})
	if started.ScheduledFor.ISO != "" {
		t.Fatalf("a download that already started still says when it will start: %+v", started.ScheduledFor)
	}
}

// TestJobRowBadgeFollowsTheStateTable: a card's badge colour is its state's tone,
// so a paused Job is not drawn as a failure and a blocked one reads as needing
// attention rather than as failed.
func TestJobRowBadgeFollowsTheStateTable(t *testing.T) {
	cases := map[jobs.State]string{
		jobs.StateRunning:     "job-tone--working",
		jobs.StateQueued:      "job-tone--waiting",
		jobs.StatePaused:      "job-tone--paused",
		jobs.StateBlocked:     "job-tone--warning",
		jobs.StateSucceeded:   "job-tone--done",
		jobs.StateFailed:      "job-tone--failed",
		jobs.StateInterrupted: "job-tone--failed",
		jobs.StateCancelled:   "job-tone--neutral",
	}
	for state, want := range cases {
		if got := jobRow(&fakeJobListReader{}, jobs.Snapshot{ID: "x", State: state}).BadgeClass; got != want {
			t.Fatalf("a %s card's badge is %q, want %q", state, got, want)
		}
	}
	if got := jobRow(&fakeJobListReader{}, jobs.Snapshot{ID: "p", State: jobs.StateSucceeded, Phase: jobs.PhasePartial}).BadgeClass; got != "job-tone--warning" {
		t.Fatalf("a partial success's badge is %q", got)
	}
	// A pause asked for and not yet confirmed: the card says it is pausing, not
	// that it is paused, and its work still reads as work in progress.
	pausing := jobRow(&fakeJobListReader{}, jobs.Snapshot{ID: "q", State: jobs.StateRunning, ControlIntent: jobs.ControlIntentPause})
	if pausing.StateLabel != "Pausing" || pausing.Progress == nil || !pausing.Progress.Indeterminate {
		t.Fatalf("a running Job with a pause requested is %q with progress %+v", pausing.StateLabel, pausing.Progress)
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
	labels := map[string]string{}
	for _, option := range options {
		labels[option.Value] = option.Label
	}
	// The select shows words, not keys: what a Job card's button says.
	for key, label := range map[string]string{
		"retry": "Retry", "continue": "Continue", "inspect": "Inspect command history",
		"retry-import": "Retry import", "pin-lineage": "Pin with related jobs", "forget": "Forget retry data",
	} {
		if labels[key] != label {
			t.Errorf("the command filter offers %q as %q, want %q", key, labels[key], label)
		}
	}
	if kept := jobCommandOptions("future-key"); kept[len(kept)-1] != (JobSelectOption{Value: "future-key", Label: "future-key"}) {
		t.Errorf("a key the URL names was dropped from the select: %v", kept)
	}
}

func TestJobInboundRelationshipOptionsKeepAValueTheURLNames(t *testing.T) {
	options := jobInboundRelationshipOptions("")
	want := []JobSelectOption{
		{Value: "retry-of", Label: "retried or continued"},
		{Value: "repeat-of", Label: "repeated"},
		{Value: "parent-child", Label: "a child stage"},
	}
	if !slices.Equal(options, want) {
		t.Fatalf("inbound options = %v, want %v", options, want)
	}
	if kept := jobInboundRelationshipOptions("future-link"); len(kept) != 4 || kept[3].Value != "future-link" {
		t.Fatalf("a relationship the URL names was dropped from the select: %v", kept)
	}
	form := jobFilterForm(url.Values{"inboundRelationship": {"repeat-of"}, "noInboundRelationship": {"retry-of"}})
	if form.InboundRelationship != "repeat-of" || form.NoInboundRelationship != "retry-of" {
		t.Fatalf("form = %+v", form)
	}
}

func TestJobKindOptionsKeepAKindTheURLNames(t *testing.T) {
	ctx := renderJobList(t, &fakeJobListReader{}, "/jobs?kind=retired-kind&kind=group-export&dismissed=false")
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

// TestJobStateOptionsOfferPartiallyCompleted pins the State fieldset: every
// lifecycle state under its own spelling, and the filter-only partial token
// right after succeeded, which it narrows.
func TestJobStateOptionsOfferPartiallyCompleted(t *testing.T) {
	options := jobStateOptions()
	var values []string
	for _, option := range options {
		values = append(values, option.Value)
	}
	want := []string{"scheduled", "queued", "running", "paused", "blocked", "succeeded", jobs.FilterStatePartial, "failed", "cancelled", "interrupted"}
	if !slices.Equal(values, want) {
		t.Fatalf("state option values = %v, want %v", values, want)
	}
	if label := options[6].Label; label != "partially completed" {
		t.Fatalf("partial option label = %q, want %q", label, "partially completed")
	}
	if label := options[5].Label; label != "succeeded" {
		t.Fatalf("succeeded option label = %q, want its own spelling", label)
	}
}

// TestJobRowNamesAPartialSuccess is the card for a succeeded Job its Kind left
// unfinished: the badge says so, and the raw phase is not repeated beside it.
// The same phase on a running Job is only a phase.
func TestJobRowNamesAPartialSuccess(t *testing.T) {
	reader := &fakeJobListReader{}
	partial := jobRow(reader, jobs.Snapshot{ID: "p", Kind: "plugin-action", State: jobs.StateSucceeded, Phase: jobs.PhasePartial})
	if partial.StateLabel != "Partially completed" || partial.Phase != "" {
		t.Fatalf("partial row = label %q, phase %q; want Partially completed and no phase", partial.StateLabel, partial.Phase)
	}
	// The live list announces a state change from this payload, so it carries
	// the phase its label is read from.
	if !strings.Contains(partial.Entity, `"phase":"partial"`) {
		t.Fatalf("partial row payload %s lacks its phase", partial.Entity)
	}
	complete := jobRow(reader, jobs.Snapshot{ID: "c", Kind: "plugin-action", State: jobs.StateSucceeded})
	if complete.StateLabel != "Succeeded" {
		t.Fatalf("complete row label = %q, want Succeeded", complete.StateLabel)
	}
	running := jobRow(reader, jobs.Snapshot{ID: "r", Kind: "plugin-action", State: jobs.StateRunning, Phase: jobs.PhasePartial})
	if running.StateLabel != "Running" || running.Phase != jobs.PhasePartial {
		t.Fatalf("running row = label %q, phase %q; want Running and its phase", running.StateLabel, running.Phase)
	}
}

// The Owner select is one field, named owner, whose choices are "Mine", an
// account, or a deleted account, so the form can submit only one of them, and
// "Mine" travels as the API's own owner=me. A link written with the API's own
// parameters shows its choice selected.
func TestTheOwnerSelectAsksForOneOwnerChoice(t *testing.T) {
	cases := []struct {
		target string
		want   func(jobs.Filter) bool
		shown  string
	}{
		{"/jobs?owner=me&dismissed=false", func(f jobs.Filter) bool { return f.OwnedByViewer && f.OwnerID == nil && !f.OwnerDeleted }, "me"},
		{"/jobs?owner=deleted&dismissed=false", func(f jobs.Filter) bool { return f.OwnerDeleted && f.OwnerID == nil && !f.OwnedByViewer }, "deleted"},
		{"/jobs?owner=7&dismissed=false", func(f jobs.Filter) bool {
			return f.OwnerID != nil && *f.OwnerID == 7 && !f.OwnerDeleted && !f.OwnedByViewer
		}, "7"},
		{"/jobs?ownerDeleted=true&dismissed=false", func(f jobs.Filter) bool { return f.OwnerDeleted }, "deleted"},
		{"/jobs?ownerId=7&dismissed=false", func(f jobs.Filter) bool { return f.OwnerID != nil && *f.OwnerID == 7 }, "7"},
	}
	// Each address names the dismissed filter, as the page's own links do: a bare
	// /jobs redirects to that default before anything is listed.
	for _, c := range cases {
		reader := &fakeJobListReader{}
		ctx := renderJobList(t, reader, c.target)
		if len(reader.listed) != 1 || !c.want(reader.listed[0]) {
			t.Fatalf("%s asked %+v", c.target, reader.listed)
		}
		if form := ctx["jobFilter"].(JobFilterForm); form.Owner != c.shown {
			t.Fatalf("%s shows the Owner select as %q, want %q", c.target, form.Owner, c.shown)
		}
	}
}
