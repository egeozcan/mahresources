package application_context

import (
	"context"
	"fmt"
	"testing"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

// An apply that created groups links to the first of them, the top of what it
// imported, as every other Kind's Job links to what it made.
func TestAnAppliedImportLinksToTheGroupItCreated(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	handle := "imp-group-link-1"
	staging := writeImportArchiveForTest(t, ctx, handle)

	parse := ctx.SubmitImportParse(handle, staging, "", "api")
	if parse.Err != nil {
		t.Fatalf("submit the parse: %v", parse.Err)
	}
	if parsed := waitForSnapshot(t, ctx, parse.CanonicalJobID, "the parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	}); parsed.State != jobs.StateSucceeded {
		t.Fatalf("the parse ended %s (%+v)", parsed.State, parsed.Failure)
	}
	apply := ctx.SubmitImportApply(handle, &ImportDecisions{
		MappingActions:  map[string]MappingAction{},
		DanglingActions: map[string]DanglingAction{},
	}, "api")
	if apply.Err != nil {
		t.Fatalf("apply the import: %v", apply.Err)
	}
	applied := waitForSnapshot(t, ctx, apply.CanonicalJobID, "the apply to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if applied.State != jobs.StateSucceeded {
		t.Fatalf("the apply ended %s (%+v)", applied.State, applied.Failure)
	}
	var created models.Group
	if err := ctx.db.Where("name = ?", "Imported").First(&created).Error; err != nil {
		t.Fatalf("read the imported group: %v", err)
	}

	admin := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleAdmin})
	content, err := admin.OpenJobOutput(context.Background(), applied.ID, jobImportGroupOutput)
	if err != nil {
		t.Fatalf("open the imported group's link: %v", err)
	}
	if want := fmt.Sprintf("/group?id=%d", created.ID); content.Location != want {
		t.Fatalf("the apply links to %q, want %q", content.Location, want)
	}
}
