package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"

	"mahresources/server/template_handlers/loaders"

	"mahresources/server/template_handlers/template_context_providers"
)

// renderJobCardForTest renders /jobs with one card and answers that card's markup.
func renderJobCardForTest(t *testing.T, row template_context_providers.JobRow) string {
	t.Helper()
	set := pongo2.NewSet("", loaders.MustNewLocalFileSystemLoader("../templates", nil))
	page, err := set.FromFile("listJobs.tpl")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	context := template_context_providers.JobCenterListContextProvider(nil)(httptest.NewRequest(http.MethodGet, "/jobs?dismissed=false", nil))
	context["jobs"] = []template_context_providers.JobRow{row}
	rendered, err := page.Execute(context)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	start := strings.Index(rendered, `data-job-id="`+row.ID+`"`)
	end := strings.Index(rendered[start:], "</article>")
	if start < 0 || end < 0 {
		t.Fatalf("the card for %s was not rendered", row.ID)
	}
	return rendered[start : start+end]
}

// A card names its Kind in words, and a stopped transfer whose total was never
// known draws an empty track: a fill of no width, not a block that fills the
// track and reads as done. The live-progress hooks stay on the one fill.
func TestAJobCardNamesItsKindAndDrawsNoFillForAnUnknownTotalItStopped(t *testing.T) {
	card := renderJobCardForTest(t, template_context_providers.JobRow{
		ID: "stopped-card", Title: "stopped.bin", Kind: "remote-download", KindLabel: "Download", State: "cancelled",
		StateLabel: "Cancelled", BadgeClass: "job-tone--neutral", DetailURL: "/job?id=stopped-card", Entity: "{}",
		Progress: &template_context_providers.JobRowProgress{AccessibleText: "235 KB processed; total unknown", Stats: "235 KB"},
	})
	if !strings.Contains(card, `data-testid="job-kind">Download</span>`) {
		t.Fatalf("the card does not name its Kind in words: %s", card)
	}
	if !strings.Contains(card, `data-job-progress-fill style="width:0"`) || strings.Contains(card, "animate-pulse") {
		t.Fatalf("a stopped transfer of unknown size draws a fill: %s", card)
	}
	if !strings.Contains(card, `data-job-stats>235 KB</p>`) {
		t.Fatalf("the stats line is missing its amount: %s", card)
	}

	running := renderJobCardForTest(t, template_context_providers.JobRow{
		ID: "running-card", Title: "running.bin", Kind: "remote-download", KindLabel: "Download", State: "running",
		StateLabel: "Running", BadgeClass: "job-tone--working", DetailURL: "/job?id=running-card", Entity: "{}",
		Progress: &template_context_providers.JobRowProgress{Text: "downloading", Indeterminate: true, AccessibleText: "downloading; total unknown"},
	})
	if !strings.Contains(running, "w-full motion-safe:animate-pulse") || strings.Contains(running, `style="width:0"`) {
		t.Fatalf("running work of unknown size does not pulse: %s", running)
	}
	if !strings.Contains(running, `data-job-stats hidden>`) {
		t.Fatalf("an empty stats line is not kept, hidden, for a live frame: %s", running)
	}
}
