package application_context

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/groupio"
	"mahresources/jobs"
	"mahresources/models"
)

// TestASecondRuntimeCannotSettleAnImportFromAPlanBeforeItsExecutorIsQuiescent
// holds the parse at both sides of publication: while bytes are being written and
// after the complete plan has been renamed into place but before ParseImport returns.
func TestASecondRuntimeCannotSettleAnImportFromAPlanBeforeItsExecutorIsQuiescent(t *testing.T) {
	first := newJobHarnessContext(t, false)
	first.queueClaimLease = time.Hour // keep the test's explicit expiry ahead of the heartbeat
	key := sharedReplayKey(t)
	holdJobReplayKey(t, first, key)
	other, _ := newSecondProcessJobContext(t, first, key)

	const handle = "import-plan-race-1"
	staging := writeImportArchiveForTest(t, first, handle)
	gate := newGatedImportPlanFs(first.GetDefaultFs(), importPlanPathFor(handle))
	t.Cleanup(gate.unblock)
	first.fs = gate
	other.fs = gate // both application contexts model processes on the shared filesystem
	first.groupio = groupio.NewService(gate, first.altFileSystems)
	other.groupio = first.groupio

	submission := first.SubmitImportParse(handle, staging, "api")
	if submission.Err != nil {
		t.Fatalf("submit the parse: %v", submission.Err)
	}
	select {
	case <-gate.partialWrite:
	case <-time.After(10 * time.Second):
		t.Fatal("the parse did not enter its staged plan write")
	}

	if err := first.db.Model(&models.JobClaim{}).Where("job_id = ?", submission.CanonicalJobID).
		Update("lease_expires_at", time.Now().UTC().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("expire the parse claim: %v", err)
	}
	if decision := reconcileOnce(t, other, submission.CanonicalJobID); decision != jobs.ReconcileExternalWorkUnproven {
		t.Fatalf("the second runtime decided %q while the plan write was held, want %q",
			decision, jobs.ReconcileExternalWorkUnproven)
	}
	assertImportParseStillQuarantined(t, other, submission.CanonicalJobID)

	gate.releasePartial()
	select {
	case <-gate.renamed:
	case <-time.After(10 * time.Second):
		t.Fatal("the parse did not atomically publish its complete plan")
	}
	planBytes, err := afero.ReadFile(other.GetDefaultFs(), importPlanPathFor(handle))
	if err != nil {
		t.Fatalf("read atomically published plan: %v", err)
	}
	var plan map[string]any
	if err := json.Unmarshal(planBytes, &plan); err != nil {
		t.Fatalf("the published plan is incomplete JSON: %v", err)
	}

	// Make the already-quarantined claim due again while the executor is deliberately
	// stopped just after rename. A valid final file alone is not quiescence evidence.
	if err := other.db.Model(&models.JobClaim{}).Where("job_id = ?", submission.CanonicalJobID).
		Update("next_reconcile_at", time.Now().UTC().Add(-time.Second)).Error; err != nil {
		t.Fatalf("make the quarantine due: %v", err)
	}
	if _, err := other.JobService().ReconcileQuarantined(context.Background(), other.jobDeps(), "second-import-runtime", 32); err != nil {
		t.Fatalf("reconcile the post-rename quarantine: %v", err)
	}
	assertImportParseStillQuarantined(t, other, submission.CanonicalJobID)

	gate.releaseRename()
	finished := waitForSnapshot(t, first, submission.CanonicalJobID, "the owning parse to finish", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("the owning parse ended %s (%+v)", finished.State, finished.Failure)
	}
	if claim := storedClaim(t, first, submission.CanonicalJobID); claim.State != models.JobClaimStateReleased {
		t.Fatalf("the claim is %s after the parse completed, want released", claim.State)
	}
}

func assertImportParseStillQuarantined(t *testing.T, ctx *MahresourcesContext, jobID string) {
	t.Helper()
	snap := jobSnapshot(t, ctx.JobService(), ctx, jobID)
	if snap.State != jobs.StateBlocked {
		t.Fatalf("the parse is %s before its executor is quiescent, want blocked with its claim held", snap.State)
	}
	if claim := storedClaim(t, ctx, jobID); claim.State != models.JobClaimStateQuarantined {
		t.Fatalf("the parse claim is %s before its executor is quiescent, want quarantined", claim.State)
	}
	if outputs, err := ctx.GetJobOutputs(jobID); err != nil {
		t.Fatalf("read outputs: %v", err)
	} else if _, exists := findJobOutput(outputs, jobImportPlanOutput); exists {
		t.Fatal("the plan output was published before the parse executor was quiescent")
	}
}

type gatedImportPlanFs struct {
	afero.Fs
	path              string
	partialWrite      chan struct{}
	partialResume     chan struct{}
	renamed           chan struct{}
	renameResume      chan struct{}
	partialOnce       sync.Once
	renameOnce        sync.Once
	partialResumeOnce sync.Once
	renameResumeOnce  sync.Once
	released          sync.Once
}

func newGatedImportPlanFs(inner afero.Fs, path string) *gatedImportPlanFs {
	return &gatedImportPlanFs{
		Fs: inner, path: path,
		partialWrite: make(chan struct{}), partialResume: make(chan struct{}),
		renamed: make(chan struct{}), renameResume: make(chan struct{}),
	}
}

func (g *gatedImportPlanFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	f, err := g.Fs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	if name == g.path || strings.HasPrefix(name, g.path+".tmp-") {
		return &gatedImportPlanFile{File: f, gate: g}, nil
	}
	return f, nil
}

func (g *gatedImportPlanFs) Rename(oldName, newName string) error {
	if err := g.Fs.Rename(oldName, newName); err != nil {
		return err
	}
	if newName == g.path {
		g.renameOnce.Do(func() { close(g.renamed) })
		<-g.renameResume
	}
	return nil
}

func (g *gatedImportPlanFs) releasePartial() {
	g.partialResumeOnce.Do(func() { close(g.partialResume) })
}
func (g *gatedImportPlanFs) releaseRename() { g.renameResumeOnce.Do(func() { close(g.renameResume) }) }
func (g *gatedImportPlanFs) unblock() {
	g.released.Do(func() {
		g.releasePartial()
		g.releaseRename()
	})
}

type gatedImportPlanFile struct {
	afero.File
	gate *gatedImportPlanFs
}

func (f *gatedImportPlanFile) Write(p []byte) (int, error) {
	var written int
	var writeErr error
	f.gate.partialOnce.Do(func() {
		half := len(p) / 2
		if half == 0 {
			half = len(p)
		}
		var n int
		n, writeErr = f.File.Write(p[:half])
		written += n
		close(f.gate.partialWrite)
		<-f.gate.partialResume
		if writeErr == nil && half < len(p) {
			n, writeErr = f.File.Write(p[half:])
			written += n
		}
	})
	if writeErr != nil {
		return written, writeErr
	}
	return written, nil
}
