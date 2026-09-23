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
