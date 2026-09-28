package application_context

import (
	"context"
	"encoding/json"
	"testing"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

// A parsed import links to the page that restores its review, and keeps the link
// while that page has the plan or the apply's report to show; once the import's
// files are removed the link goes.
func TestAParsedImportLinksToItsReviewWhileItsFilesRemain(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	handle := "imp-review-link-1"
	staging := writeImportArchiveForTest(t, ctx, handle)

	submission := ctx.SubmitImportParse(handle, staging, "trip.tar", "api")
	if submission.Err != nil {
		t.Fatalf("submit the parse: %v", submission.Err)
	}
	parse := waitForSnapshot(t, ctx, submission.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if parse.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", parse.State, parse.Failure)
	}

	admin := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleAdmin})
	offered := func() (jobs.Output, bool) {
		t.Helper()
		outputs, err := admin.GetOpenableJobOutputs(parse.ID)
		if err != nil {
			t.Fatalf("list the outputs: %v", err)
		}
		return findJobOutput(outputs, jobImportReviewOutput)
	}
	review, found := offered()
	if !found {
		t.Fatal("the parsed import offers no link to its review")
	}
	if review.Type != jobs.OutputTypeEntity || review.Label != "Import review" {
		t.Fatalf("the review link is %+v, want an entity output labelled Import review", review)
	}
	content, err := admin.OpenJobOutput(context.Background(), parse.ID, jobImportReviewOutput)
	if err != nil {
		t.Fatalf("open the review link: %v", err)
	}
	if want := "/admin/import?job=" + handle; content.Location != want {
		t.Fatalf("the review link opens %q, want %q", content.Location, want)
	}

	apply := admin.SubmitImportApply(handle, &ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	}, "api")
	if apply.Err != nil {
		t.Fatalf("apply the import: %v", apply.Err)
	}
	if applied := waitForSnapshot(t, ctx, apply.CanonicalJobID, "the apply to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	}); applied.State != jobs.StateSucceeded {
		t.Fatalf("the apply ended %s (%+v)", applied.State, applied.Failure)
	}
	if _, found := offered(); !found {
		t.Fatal("the review link went away while the page still has the apply's report to show")
	}

	if err := ctx.DeleteImportFiles(handle); err != nil {
		t.Fatalf("remove the import's files: %v", err)
	}
	if _, found := offered(); found {
		t.Fatal("the review link is still offered after the import's files were removed")
	}
	if _, err := admin.OpenJobOutput(context.Background(), parse.ID, jobImportReviewOutput); err == nil {
		t.Fatal("the review link still opens after the import's files were removed")
	}
}

// Only a parse's output may name an import review: the same reference from any
// other Kind opens nothing.
func TestOnlyAParseOffersAnImportReview(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	reference := json.RawMessage(`{"importReview":"imp-anything"}`)
	offered, isReview, err := ctx.importReviewOutputOffered(JobKindRemoteDownload, reference)
	if err != nil || !isReview || offered {
		t.Fatalf("a download's review reference: offered=%t isReview=%t err=%v, want refused", offered, isReview, err)
	}
	if _, isReview, _ := ctx.importReviewOutputOffered(JobKindGroupImportParse, json.RawMessage(`{"groupId":3}`)); isReview {
		t.Fatal("an ordinary entity reference was read as an import review")
	}
	for _, bad := range []string{`{"importReview":"../x"}`, `{"importReview":"a?b"}`, `{"importReview":""}`} {
		if _, ok := importReviewTarget(json.RawMessage(bad)); ok {
			t.Errorf("%s was accepted as an import handle", bad)
		}
	}
}
