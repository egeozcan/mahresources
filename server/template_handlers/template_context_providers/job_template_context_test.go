package template_context_providers

import (
	"net/http/httptest"
	"testing"

	"mahresources/server/template_handlers/template_entities"
)

func TestJobCenterTemplateContext(t *testing.T) {
	request := httptest.NewRequest("GET", "http://example.test/jobs?view=all&state=failed", nil)
	context := JobCenterListContextProvider(nil)(request)

	if got := context["pageTitle"]; got != "Job Center" {
		t.Fatalf("pageTitle = %v, want Job Center", got)
	}
	if got := context["hideSidebar"]; got != true {
		t.Fatalf("hideSidebar = %v, want true", got)
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
