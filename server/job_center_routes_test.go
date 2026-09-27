package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/mux"
	"mahresources/server/template_handlers/loaders"
	"mahresources/server/template_handlers/template_context_providers"
	"mahresources/server/template_handlers/template_entities"
)

func TestJobCenterRoutesAreOptIn(t *testing.T) {
	t.Chdir("..")

	disabled := mux.NewRouter()
	registerJobCenterRoutes(disabled, nil, false)
	for _, path := range []string{"/jobs", "/job", "/jobs.json", "/job.body"} {
		response := httptest.NewRecorder()
		disabled.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("disabled %s route returned %d, want 404", path, response.Code)
		}
	}

	enabled := mux.NewRouter()
	registerJobCenterRoutes(enabled, nil, true)
	for _, path := range []string{"/jobs", "/job", "/jobs.json", "/job.body"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		match := &mux.RouteMatch{}
		if !enabled.Match(request, match) {
			t.Errorf("enabled route %s did not match", path)
		}
	}
}

func TestJobCenterTemplatesRender(t *testing.T) {
	set := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("../templates", nil))
	for _, testCase := range []struct {
		path     string
		template string
		provider func(*http.Request) pongo2.Context
	}{
		{
			path:     "/jobs",
			template: "listJobs.tpl",
			provider: template_context_providers.JobCenterListContextProvider(nil),
		},
		{
			path:     "/job?id=job-fixture",
			template: "displayJob.tpl",
			provider: template_context_providers.JobDetailContextProvider(nil),
		},
	} {
		request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		context := testCase.provider(request)
		page, err := set.FromFile(testCase.template)
		if err != nil {
			t.Errorf("parse %s: %v", testCase.template, err)
			continue
		}
		rendered, err := page.Execute(context)
		if err != nil {
			t.Errorf("render %s: %v", testCase.template, err)
			continue
		}
		if template_context_providers.JobCenterCutoverEnabled {
			if !strings.Contains(rendered, `data-testid="job-panel-root"`) {
				t.Errorf("%s did not render the enabled Job Center panel", testCase.template)
			}
			if strings.Contains(rendered, `data-testid="cockpit-trigger"`) {
				t.Errorf("%s rendered the legacy download panel after cutover", testCase.template)
			}
		} else {
			if strings.Contains(rendered, `data-testid="job-panel-root"`) {
				t.Errorf("%s rendered the Job Center panel before cutover", testCase.template)
			}
			if !strings.Contains(rendered, `data-testid="cockpit-trigger"`) {
				t.Errorf("%s did not preserve the legacy download panel before cutover", testCase.template)
			}
		}
	}
}

// TestAnEmptiedJobPageDoesNotClaimThereAreNoJobs renders the case a keyset page
// meets when its rows are dismissed or leave the filter: the page is empty but
// Previous still leads to Jobs. The shared empty state reads a page position as
// no filter at all and would say "No jobs yet".
func TestAnEmptiedJobPageDoesNotClaimThereAreNoJobs(t *testing.T) {
	set := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("../templates", nil))
	page, err := set.FromFile("listJobs.tpl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context := template_context_providers.JobCenterListContextProvider(nil)(httptest.NewRequest(http.MethodGet, "/jobs?before=list-v1.x", nil))
	context["pagination"] = template_entities.KeysetPagination("/jobs?before=list-v1.x", "")
	rendered, err := page.Execute(context)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(rendered, "No jobs yet") {
		t.Fatal("an emptied later page said there are no jobs, though Previous leads to some")
	}
	if !strings.Contains(rendered, "No jobs on this page") {
		t.Fatal("an emptied later page did not point back to the earlier ones")
	}
}

// TestAScheduledJobCardSaysWhenItStarts renders the card a scheduled Job gets: its
// start time, in a <time> the page shows in the reader's zone, no progress bar,
// and a badge whose colour is its state's tone.
func TestAScheduledJobCardSaysWhenItStarts(t *testing.T) {
	set := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("../templates", nil))
	page, err := set.FromFile("listJobs.tpl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context := template_context_providers.JobCenterListContextProvider(nil)(httptest.NewRequest(http.MethodGet, "/jobs?dismissed=false", nil))
	context["jobs"] = []template_context_providers.JobRow{{
		ID: "scheduled-card", Title: "Download later", Kind: "deferred-download", State: "scheduled",
		StateLabel: "Scheduled", BadgeClass: "card-badge--live", DetailURL: "/job?id=scheduled-card", Entity: "{}",
		ScheduledFor: template_context_providers.JobRowTime{ISO: "2026-09-28T14:17:00+02:00", Display: "2026-09-28 14:17:00", Minute: "2026-09-28 14:17"},
	}}
	rendered, err := page.Execute(context)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(rendered, `Starts <time data-local-time datetime="2026-09-28T14:17:00+02:00">2026-09-28 14:17</time>`) {
		t.Fatal("the scheduled card does not say when it starts")
	}
	if !strings.Contains(rendered, `class="card-badge card-badge--live" data-testid="job-state">Scheduled</span>`) {
		t.Fatal("the scheduled card's badge does not follow its tone")
	}
	start := strings.Index(rendered, `data-job-id="scheduled-card"`)
	end := strings.Index(rendered[start:], "</article>")
	if start < 0 || end < 0 {
		t.Fatal("the scheduled card was not rendered")
	}
	if card := rendered[start : start+end]; strings.Contains(card, `role="progressbar"`) || strings.Contains(card, "In progress") {
		t.Fatal("the scheduled card draws a progress bar for work nobody is doing")
	}
}

// TestAnIndeterminateJobCardHonoursReducedMotion renders a running card whose
// total is unknown. Its bar may pulse only for a reader who has not asked for
// reduced motion, as the drawer's and the detail page's bars do.
func TestAnIndeterminateJobCardHonoursReducedMotion(t *testing.T) {
	set := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("../templates", nil))
	page, err := set.FromFile("listJobs.tpl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context := template_context_providers.JobCenterListContextProvider(nil)(httptest.NewRequest(http.MethodGet, "/jobs?dismissed=false", nil))
	context["jobs"] = []template_context_providers.JobRow{{
		ID: "running-card", Title: "Download", Kind: "remote-download", State: "running",
		StateLabel: "Running", BadgeClass: "card-badge--live", DetailURL: "/job?id=running-card", Entity: "{}",
		Progress: &template_context_providers.JobRowProgress{Text: "Working", Indeterminate: true, AccessibleText: "Working; total unknown"},
	}}
	rendered, err := page.Execute(context)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	start := strings.Index(rendered, `data-job-id="running-card"`)
	end := strings.Index(rendered[start:], "</article>")
	if start < 0 || end < 0 {
		t.Fatal("the running card was not rendered")
	}
	card := rendered[start : start+end]
	if !strings.Contains(card, "motion-safe:animate-pulse") {
		t.Fatal("the indeterminate bar does not pulse at all")
	}
	if strings.Contains(strings.ReplaceAll(card, "motion-safe:animate-pulse", ""), "animate-pulse") {
		t.Fatal("the indeterminate bar pulses for a reader who asked for reduced motion")
	}
}

// TestAJobCardListsItsSummaryAsFields renders a card whose Kind summarized it as
// fields: they are listed under their labels in the card's details, and the
// card shows no JSON.
func TestAJobCardListsItsSummaryAsFields(t *testing.T) {
	set := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("../templates", nil))
	page, err := set.FromFile("listJobs.tpl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context := template_context_providers.JobCenterListContextProvider(nil)(httptest.NewRequest(http.MethodGet, "/jobs?dismissed=false", nil))
	context["jobs"] = []template_context_providers.JobRow{{
		ID: "summary-card", Title: "sunrise.png", Kind: "remote-download", State: "failed",
		StateLabel: "Failed", BadgeClass: "card-badge--danger", DetailURL: "/job?id=summary-card", Entity: "{}",
		SummaryFields: []template_context_providers.JobSummaryField{{Label: "Host", Value: "files.example.test"}, {Label: "Targets", Value: "owner:1"}},
	}}
	rendered, err := page.Execute(context)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	start := strings.Index(rendered, `data-job-id="summary-card"`)
	end := strings.Index(rendered[start:], "</article>")
	if start < 0 || end < 0 {
		t.Fatal("the card was not rendered")
	}
	card := rendered[start : start+end]
	if !strings.Contains(card, `<div data-job-summary-field><dt class="inline">Host</dt> <dd class="inline break-words">files.example.test</dd></div>`) {
		t.Fatalf("the card does not list its summary's fields:\n%s", card)
	}
	if strings.Contains(card, `{&quot;`) || strings.Contains(card, `{"`) {
		t.Fatal("the card shows its summary as JSON")
	}
}
