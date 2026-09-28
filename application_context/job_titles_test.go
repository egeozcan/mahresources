package application_context

import (
	"strings"
	"testing"
	"unicode/utf8"

	"mahresources/jobs"
	"mahresources/models"
)

// An export is titled by the group it exports, and by how many more when it
// exports several, so two exports in one list can be told apart.
func TestAnExportIsTitledByTheGroupsItExports(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	first := createExportGroupForTest(t, ctx, "Video clips")
	second := createExportGroupForTest(t, ctx, "Photos 2026")
	third := createExportGroupForTest(t, ctx, "Archive")

	one := ctx.SubmitGroupExport(exportRequestForTest(first), "api")
	if one.Err != nil {
		t.Fatalf("submit the export: %v", one.Err)
	}
	if got := jobTitleForTest(t, ctx, one.CanonicalJobID); got != "Export of Video clips" {
		t.Fatalf("one root is titled %q, want %q", got, "Export of Video clips")
	}

	several := exportRequestForTest(second)
	several.RootGroupIDs = append(several.RootGroupIDs, first, third)
	many := ctx.SubmitGroupExport(several, "api")
	if many.Err != nil {
		t.Fatalf("submit the export: %v", many.Err)
	}
	if got := jobTitleForTest(t, ctx, many.CanonicalJobID); got != "Export of Photos 2026 and 2 more groups" {
		t.Fatalf("three roots are titled %q, want %q", got, "Export of Photos 2026 and 2 more groups")
	}

	pair := exportRequestForTest(third)
	pair.RootGroupIDs = append(pair.RootGroupIDs, first)
	two := ctx.SubmitGroupExport(pair, "api")
	if two.Err != nil {
		t.Fatalf("submit the export: %v", two.Err)
	}
	if got := jobTitleForTest(t, ctx, two.CanonicalJobID); got != "Export of Archive and 1 more group" {
		t.Fatalf("two roots are titled %q, want %q", got, "Export of Archive and 1 more group")
	}
}

// A long name is cut to a bounded length with an ellipsis, so a title does not
// grow with the input.
func TestALongNameIsCutInAJobTitle(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	long := strings.Repeat("Ö", 150)
	group := createExportGroupForTest(t, ctx, long)
	submission := ctx.SubmitGroupExport(exportRequestForTest(group), "api")
	if submission.Err != nil {
		t.Fatalf("submit the export: %v", submission.Err)
	}
	got := jobTitleForTest(t, ctx, submission.CanonicalJobID)
	want := "Export of " + strings.Repeat("Ö", jobTitleNameRunes-1) + "…"
	if got != want {
		t.Fatalf("a long name is titled %q (%d runes), want it cut to %d runes with an ellipsis",
			got, utf8.RuneCountInString(got), jobTitleNameRunes)
	}
	if len(got) > jobs.MaxTitleBytes || !utf8.ValidString(got) {
		t.Fatalf("the title is %d bytes, valid UTF-8 %t; want at most %d", len(got), utf8.ValidString(got), jobs.MaxTitleBytes)
	}

	if got := importJobTitle("Import of ", strings.Repeat("x", 300)+".tar", "Group import"); utf8.RuneCountInString(got) != len("Import of ")+jobTitleNameRunes {
		t.Fatalf("a long upload name is titled %q, want the name cut to %d characters", got, jobTitleNameRunes)
	}
}

// A clustering run is titled by the Resource Reduction it computes.
func TestAClusteringRunIsTitledByItsReduction(t *testing.T) {
	ctx := newWorkflowJobContext(t)
	reduction := createReductionRowForTest(t, ctx, `{"clusters":[]}`, models.ReductionStatusFailed)
	if err := ctx.db.Model(reduction).Update("name", "L6 reduction photos").Error; err != nil {
		t.Fatalf("name the Reduction: %v", err)
	}
	if _, err := ctx.RequestReductionCompute(reduction.ID, reduction.Version, nil, false, nil); err != nil {
		t.Fatalf("request the compute: %v", err)
	}
	snap := jobOfKindForTest(t, ctx, JobKindReductionCompute)
	if snap.Title != "Clusters for L6 reduction photos" {
		t.Fatalf("the clustering run is titled %q, want %q", snap.Title, "Clusters for L6 reduction photos")
	}
	// The run finishes before the test does, so nothing it writes outlives the
	// test's database and directory.
	waitForSnapshot(t, ctx, snap.ID, "the clustering run to finish", func(s jobs.Snapshot) bool { return s.State.Terminal() })
}

// jobTitleForTest answers a Job's title once the Job has finished, so nothing it
// writes outlives the test's database and directory.
func jobTitleForTest(t *testing.T, ctx *MahresourcesContext, jobID string) string {
	t.Helper()
	snap := waitForSnapshot(t, ctx, jobID, "the Job to finish", func(s jobs.Snapshot) bool { return s.State.Terminal() })
	return snap.Title
}
