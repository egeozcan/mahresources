package main

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/jobs"
)

func TestJobCenterCutoverRequiresRetirementAndCompleteKinds(t *testing.T) {
	var ctx application_context.MahresourcesContext
	service := jobs.NewService()
	ctx.SetJobService(service)
	ready := application_context.JobMigrationReadiness{Ready: true, WriterEpoch: 2, Phase: "complete"}
	if err := validateJobCenterCutover(service, ready); err != nil {
		t.Fatalf("complete Kind inventory and retirement were refused: %v", err)
	}
	ready.Ready = false
	if err := validateJobCenterCutover(service, ready); err == nil || !strings.Contains(err.Error(), "migration") {
		t.Fatalf("incomplete migration was admitted: %v", err)
	}
	ready.Ready = true
	ready.WriterEpoch = 1
	if err := validateJobCenterCutover(service, ready); err == nil {
		t.Fatal("epoch 1 report was admitted despite ready flag")
	}
	if err := validateJobCenterCutover(jobs.NewService(), application_context.JobMigrationReadiness{Ready: true}); err == nil {
		t.Fatal("missing Kind inventory was admitted")
	}
}

// The review list is said at startup, naming the Jobs, and nothing is said when it
// is empty.
func TestStartupNamesTheJobsToReviewAndOnlyThem(t *testing.T) {
	var buffer bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&buffer)
	t.Cleanup(func() { log.SetOutput(previous) })

	warnJobPrincipalReviewCandidates(application_context.JobReviewCandidates{})
	if buffer.Len() != 0 {
		t.Fatalf("an empty review list logged %q", buffer.String())
	}
	warnJobPrincipalReviewCandidates(application_context.JobReviewCandidates{
		Count: 3,
		Jobs: []application_context.JobReviewCandidate{
			{ID: "job-a", Kind: "remote-download", State: "queued"},
			{ID: "job-b", Kind: "plugin-action", State: "blocked"},
		},
	})
	said := buffer.String()
	for _, want := range []string{"WARNING: 3 unfinished Job(s)", "job-a (remote-download, queued)", "job-b (plugin-action, blocked)", "and 1 more", "reviewCandidates"} {
		if !strings.Contains(said, want) {
			t.Fatalf("the startup warning %q does not say %q", said, want)
		}
	}
}
