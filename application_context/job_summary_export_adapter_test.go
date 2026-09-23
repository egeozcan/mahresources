package application_context

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/models"
)

func TestSummaryExportIsAnOwnerVisibleJobWithExpiringTypedOutput(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	owner, err := ctx.CreateUser(&UserInput{Username: "summary-owner", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	other, err := ctx.CreateUser(&UserInput{Username: "summary-other", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create other user: %v", err)
	}
	ownerCtx := ctx.WithPrincipal(auth.FromUser(owner))

	acceptDownload := func(principalCtx *MahresourcesContext) jobs.Snapshot {
		principal := principalCtx.Principal()
		id := principal.UserID
		accepted, err := principalCtx.JobService().Accept(principalCtx.jobDeps(), jobs.Acceptance{
			Kind: JobKindRemoteDownload, KindVersion: jobDownloadKindVersion,
			State: jobs.StateQueued, OwnerUserID: &id, ActorUserID: &id,
			Origin: "api", Title: "historical download", Replay: jobs.ReplayInput{NonReplayable: true},
		})
		if err != nil {
			t.Fatalf("accept download for %d: %v", id, err)
		}
		return accepted
	}
	acceptDownload(ownerCtx)
	acceptDownload(ctx.WithPrincipal(auth.FromUser(other)))

	from := time.Now().UTC().Add(-181 * 24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	accepted, err := ownerCtx.SubmitJobSummaryExport(
		jobs.Filter{Kinds: []string{JobKindRemoteDownload}}, from, to, "json", "api",
	)
	if err != nil {
		t.Fatalf("submit summary export: %v", err)
	}
	if accepted.Kind != JobKindSummaryExport || accepted.KindVersion != jobSummaryExportVersion || accepted.OwnerUserID == nil || *accepted.OwnerUserID != owner.ID {
		t.Fatalf("accepted Job = %+v, want an owner-visible summary export for %d", accepted, owner.ID)
	}
	if _, registered := ctx.JobService().AdapterFor(JobKindSummaryExport, jobSummaryExportVersion); !registered {
		t.Fatal("SetJobService did not register the summary export adapter")
	}

	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindSummaryExport, KindVersion: jobSummaryExportVersion, Claimant: "summary-test-runtime",
	})
	if err != nil || !claimed || execution.JobID != accepted.ID {
		t.Fatalf("claim summary export = (%+v, %t, %v)", execution, claimed, err)
	}
	adapter, ok := ctx.JobService().AdapterFor(JobKindSummaryExport, jobSummaryExportVersion)
	if !ok {
		t.Fatal("summary export adapter is not registered")
	}
	if err := adapter.Dispatch(context.Background(), execution); err != nil {
		t.Fatalf("dispatch summary export: %v", err)
	}

	finished, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{UserID: owner.ID}, accepted.ID)
	if err != nil {
		t.Fatalf("read summary export: %v", err)
	}
	if finished.State != jobs.StateSucceeded {
		t.Fatalf("summary export state = %s, want succeeded", finished.State)
	}
	outputs, err := ownerCtx.GetOpenableJobOutputs(accepted.ID)
	if err != nil {
		t.Fatalf("read output metadata: %v", err)
	}
	if len(outputs) != 1 || outputs[0].Key != jobSummaryExportOutput || outputs[0].Type != jobs.OutputTypeArtifact || outputs[0].Availability != jobs.OutputAvailable || outputs[0].ExpiresAt == nil {
		t.Fatalf("summary output = %+v, want one available expiring artifact", outputs)
	}
	content, err := ownerCtx.OpenJobOutput(context.Background(), accepted.ID, jobSummaryExportOutput)
	if err != nil {
		t.Fatalf("open summary output: %v", err)
	}
	defer content.Body.Close()
	body, err := io.ReadAll(content.Body)
	if err != nil {
		t.Fatalf("read summary artifact: %v", err)
	}
	var summary jobs.Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode exported JSON: %v (%s)", err, body)
	}
	if summary.Total != 1 || summary.ByKind[JobKindRemoteDownload] != 1 {
		t.Fatalf("export included hidden or unrelated Jobs: %+v", summary)
	}
	if content.ContentType != "application/json" {
		t.Fatalf("content type = %q, want application/json", content.ContentType)
	}
}

func TestSummaryExportCSVIsStableAndComplete(t *testing.T) {
	data, err := encodeJobSummaryExport(jobs.Summary{
		Total: 3, ByState: map[string]int64{"queued": 2, "failed": 1},
		ByKind:    map[string]int64{"group-export": 1, "remote-download": 2},
		Succeeded: 0, Failed: 1, Terminal: 1, SuccessRate: 0,
		Failures: []jobs.FailureClassCount{{Class: "internal", Count: 1}},
	}, "csv")
	if err != nil {
		t.Fatalf("encode CSV: %v", err)
	}
	want := "metric,dimension,value\njobs_by_state,failed,1\njobs_by_state,queued,2\njobs_by_kind,group-export,1\njobs_by_kind,remote-download,2\ntotal,,3\nsucceeded,,0\nfailed,,1\nterminal,,1\nsuccess_rate,,0\nqueue_median,,0s\nqueue_p95,,0s\nrun_median,,0s\nrun_p95,,0s\nfailures_by_class,internal,1\n"
	if string(data) != want {
		t.Fatalf("CSV =\n%s\nwant\n%s", data, want)
	}
}

func TestSummaryExportInputRequiresLongExplicitRangeAndKnownFormat(t *testing.T) {
	from := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		to     time.Time
		format string
	}{
		{name: "interactive length", to: from.Add(jobs.MaxSummaryWindow), format: "json"},
		{name: "unknown format", to: from.Add(91 * 24 * time.Hour), format: "xml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(jobSummaryExportInput{From: from, To: tc.to, Format: tc.format})
			if _, err := jobSummaryExportInputOf(input); err == nil {
				t.Fatal("invalid export input was accepted")
			}
		})
	}
}
