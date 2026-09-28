package api_tests

import (
	"net/http"
	"regexp"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/jobs"
)

// The Job page answers for the Job it names: titled by that Job and its state
// with the Job's title as its one h1, a 404 with a way back to the Job Center
// for an id that names no Job the viewer may see (whatever the id holds), and
// the canonical Job's page for a legacy id.
func TestTheJobPageAnswersForTheJobItNames(t *testing.T) {
	tc := SetupTestEnv(t)
	service := installJobControlPlaneWithoutRuntime(t, tc)
	snapshot, err := service.Accept(jobs.Deps{DB: tc.DB}, jobs.Acceptance{
		Kind: application_context.JobKindRemoteDownload, KindVersion: 1, State: jobs.StateQueued,
		Origin: "api", Title: "Download from example.test",
		Replay:     jobs.ReplayInput{NonReplayable: true},
		LegacyRefs: []jobs.LegacyRef{{Namespace: application_context.DownloadHandleNamespace, Handle: "5ccbf2199da9ae9f"}},
	})
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	html := map[string]string{"Accept": "text/html"}

	page := doReq(tc, http.MethodGet, "/job?id="+snapshot.ID, html, nil, nil)
	if page.Code != http.StatusOK {
		t.Fatalf("the Job's page answered %d", page.Code)
	}
	body := page.Body.String()
	if !strings.Contains(body, "<title>Download from example.test (Queued) - Job "+jobs.ShortID(snapshot.ID)+" - ") {
		t.Fatalf("the page is not titled by its Job: %s", regexp.MustCompile(`<title>[^<]*</title>`).FindString(body))
	}
	if count := strings.Count(body, "<h1"); count != 1 {
		t.Fatalf("the page has %d h1 headings, want one", count)
	}
	heading := regexp.MustCompile(`(?s)<h1[^>]*>(.*?)</h1>`).FindStringSubmatch(body)
	if heading == nil || !strings.Contains(heading[1], "Download from example.test") {
		t.Fatalf("the page's h1 does not name the Job: %v", heading)
	}

	legacy := doReq(tc, http.MethodGet, "/job?id=5ccbf2199da9ae9f", html, nil, nil)
	if legacy.Code != http.StatusFound || legacy.Header().Get("Location") != "/job?id="+snapshot.ID {
		t.Fatalf("a legacy id answered %d to %q, want a redirect to the Job's page", legacy.Code, legacy.Header().Get("Location"))
	}

	for _, id := range []string{"01a0ffff-0000-7000-8000-000000000000", "not-a-uuid", "a%2Fb", "%3Cscript%3E"} {
		missing := doReq(tc, http.MethodGet, "/job?id="+id, html, nil, nil)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("/job?id=%s answered %d, want 404", id, missing.Code)
		}
		if !strings.Contains(missing.Body.String(), `href="/jobs?dismissed=false"`) ||
			!strings.Contains(missing.Body.String(), "Back to Job Center") {
			t.Fatalf("/job?id=%s offers no way back to the Job Center", id)
		}
	}
}

// A filter the Job Center cannot use answers 400 with the Job Center itself:
// its filter form, the problem in the reader's terms, and a way to clear it,
// rather than an error page whose only link leaves the Job Center.
func TestAnUnusableJobCenterFilterKeepsTheFilterForm(t *testing.T) {
	tc := SetupTestEnv(t)
	installJobControlPlaneWithoutRuntime(t, tc)
	html := map[string]string{"Accept": "text/html"}

	for _, query := range []string{
		"state=bogus&kind=remote-download&dismissed=false",
		"acceptedAfter=2026-09-27&acceptedBefore=2026-09-26&dismissed=false",
		"cursor=garbage&dismissed=false",
		"ownerId=abc&dismissed=false",
	} {
		page := doReq(tc, http.MethodGet, "/jobs?"+query, html, nil, nil)
		if page.Code != http.StatusBadRequest {
			t.Fatalf("/jobs?%s answered %d, want 400", query, page.Code)
		}
		body := page.Body.String()
		if !strings.Contains(body, `aria-label="Filter jobs"`) {
			t.Fatalf("/jobs?%s dropped the filter form", query)
		}
		if !strings.Contains(body, "data-job-list-error") || !strings.Contains(body, `href="/jobs?dismissed=false"`) {
			t.Fatalf("/jobs?%s does not say what is wrong or offer to clear the filters", query)
		}
		if strings.Contains(body, "<title>Error 400") || strings.Contains(body, "jobs: list:") || strings.Contains(body, "invalid filter:") {
			t.Fatalf("/jobs?%s rendered an error page or the service's wrapping", query)
		}
	}
}
