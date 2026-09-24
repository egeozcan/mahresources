package application_context

import (
	"context"
	"encoding/json"
	"errors"
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
	if _, err := ctx.JobService().ForgetReplay(ctx.jobDeps(), jobs.Access{UserID: owner.ID}, accepted.ID); err != nil {
		t.Fatalf("forget completed export input: %v", err)
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

func TestSummaryExportAdminOwnerFilterRetainsAdminQueryAndOutputAccess(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	admin, err := ctx.CreateUser(&UserInput{Username: "summary-filter-admin", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create administrator: %v", err)
	}
	if _, err := ctx.CreateUser(&UserInput{Username: "summary-filter-replacement-admin", Password: "password1", Role: models.RoleAdmin}); err != nil {
		t.Fatalf("create replacement administrator: %v", err)
	}
	owner, err := ctx.CreateUser(&UserInput{Username: "summary-filter-owner", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create filtered owner: %v", err)
	}
	adminCtx := ctx.WithPrincipal(auth.FromUser(admin))
	ownerID := owner.ID
	actorID := admin.ID
	adminVisibleJob, err := ctx.JobService().Accept(adminCtx.jobDeps(), jobs.Acceptance{
		Kind: JobKindSimilarityRecompute, KindVersion: jobMaintenanceKindVersion,
		State: jobs.StateQueued, OwnerUserID: &ownerID, ActorUserID: &actorID,
		Origin: "api", Title: "admin-visible job for filtered export",
		Replay: jobs.ReplayInput{Input: maintenanceJobInputJSON()},
	})
	if err != nil {
		t.Fatalf("accept admin-visible Job: %v", err)
	}
	pinned := true
	if err := adminCtx.SetJobPreference(jobs.PreferenceRequest{JobID: adminVisibleJob.ID, Pinned: &pinned}); err != nil {
		t.Fatalf("pin admin-visible Job as export requester: %v", err)
	}
	dismissed := false
	filter := jobs.Filter{OwnerID: &ownerID, Pinned: &pinned, Dismissed: &dismissed}
	from := time.Now().UTC().Add(-181 * 24 * time.Hour)
	to := time.Now().UTC().Add(time.Hour)
	accepted, err := adminCtx.SubmitJobSummaryExport(filter, from, to, "json", "api")
	if err != nil {
		t.Fatalf("submit owner-filtered admin export: %v", err)
	}

	execution, claimed, err := ctx.JobService().Claim(context.Background(), ctx.jobDeps(), jobs.ClaimRequest{
		Kind: JobKindSummaryExport, KindVersion: jobSummaryExportVersion, Claimant: "admin-summary-filter-test",
	})
	if err != nil || !claimed || execution.JobID != accepted.ID {
		t.Fatalf("claim summary export = (%+v, %t, %v)", execution, claimed, err)
	}
	adapter, registered := ctx.JobService().AdapterFor(JobKindSummaryExport, jobSummaryExportVersion)
	if !registered {
		t.Fatal("summary export adapter is not registered")
	}
	if err := adapter.Dispatch(context.Background(), execution); err != nil {
		t.Fatalf("dispatch owner-filtered admin export: %v", err)
	}

	outputs, err := adminCtx.GetOpenableJobOutputs(accepted.ID)
	if err != nil || len(outputs) != 1 || outputs[0].Key != jobSummaryExportOutput {
		t.Fatalf("administrator openable outputs = %#v, err=%v; want summary artifact", outputs, err)
	}
	content, err := adminCtx.OpenJobOutput(context.Background(), accepted.ID, jobSummaryExportOutput)
	if err != nil {
		t.Fatalf("administrator could not open filtered summary: %v", err)
	}
	body, err := io.ReadAll(content.Body)
	_ = content.Body.Close()
	if err != nil {
		t.Fatalf("read filtered summary: %v", err)
	}
	var summary jobs.Summary
	if err := json.Unmarshal(body, &summary); err != nil {
		t.Fatalf("decode filtered summary: %v (%s)", err, body)
	}
	if summary.Total != 1 || summary.ByKind[JobKindSimilarityRecompute] != 1 {
		t.Fatalf("admin summary with owner and preference filters = %+v, want its pinned admin-visible Job", summary)
	}

	if _, err := adminCtx.UpdateUser(admin.ID, &UserUpdate{Role: UserField[models.Role]{Set: true, Value: models.RoleEditor}}); err != nil {
		t.Fatalf("demote export administrator: %v", err)
	}
	demotedCtx := ctx.WithPrincipal(&auth.Principal{UserID: admin.ID, Role: models.RoleEditor})
	if outputs, err := demotedCtx.GetOpenableJobOutputs(accepted.ID); err != nil || len(outputs) != 0 {
		t.Fatalf("demoted administrator's openable outputs = %#v, err=%v; want hidden", outputs, err)
	}
	if _, err := demotedCtx.OpenJobOutput(context.Background(), accepted.ID, jobSummaryExportOutput); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("demoted administrator opened admin-scoped export: %v", err)
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
	scope := jobSummaryExportDataScope{Class: jobSummaryExportOwnerScope, OwnerUserID: 7}
	for _, tc := range []struct {
		name   string
		to     time.Time
		format string
	}{
		{name: "interactive length", to: from.Add(jobs.MaxSummaryWindow), format: "json"},
		{name: "unknown format", to: from.Add(91 * 24 * time.Hour), format: "xml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input, _ := json.Marshal(jobSummaryExportInput{From: from, To: tc.to, Format: tc.format, Scope: scope})
			if _, err := jobSummaryExportInputOf(input); err == nil {
				t.Fatal("invalid export input was accepted")
			}
		})
	}
}

func TestSummaryExportInputRequiresCreationTimeScope(t *testing.T) {
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	input, err := json.Marshal(jobSummaryExportInput{
		From: from, To: from.Add(91 * 24 * time.Hour), Format: "json",
	})
	if err != nil {
		t.Fatalf("encode scope-less summary export input: %v", err)
	}
	if _, err := jobSummaryExportInputOf(input); err == nil {
		t.Fatal("summary export input without a creation-time scope was accepted")
	}
}

func TestSummaryExportScopeRevalidatesAgainstCurrentVisibility(t *testing.T) {
	owner := &auth.Principal{UserID: 7, Role: models.RoleEditor}
	other := &auth.Principal{UserID: 8, Role: models.RoleEditor}
	admin := &auth.Principal{UserID: 9, Role: models.RoleAdmin}
	guest := &auth.Principal{UserID: 7, Role: models.RoleGuest}

	adminScope := jobSummaryExportDataScope{Class: jobSummaryExportAdminScope}
	if !adminScope.contains(admin) {
		t.Fatal("administrator could not read an administrator-scoped export")
	}
	if adminScope.contains(owner) {
		t.Fatal("editor retained access to an administrator-scoped export")
	}
	filteredOwnerID := owner.UserID
	filteredAdminScope, err := jobSummaryExportScopeFor(admin, jobs.Filter{OwnerID: &filteredOwnerID})
	if err != nil || filteredAdminScope != (jobSummaryExportDataScope{Class: jobSummaryExportAdminScope, PrincipalUserID: admin.UserID}) {
		t.Fatalf("administrator scope filtered to one owner = %+v, err=%v; want administrator scope with the query principal", filteredAdminScope, err)
	}
	if filteredAdminScope.contains(owner) {
		t.Fatal("filtered owner gained access to an administrator export")
	}
	if got, want := filteredAdminScope.access(), (jobs.Access{UserID: admin.UserID, Administrator: true}); got != want {
		t.Fatalf("administrator summary query access = %+v, want %+v", got, want)
	}

	ownerScope := jobSummaryExportDataScope{Class: jobSummaryExportOwnerScope, OwnerUserID: owner.UserID}
	if !ownerScope.contains(owner) {
		t.Fatal("owner lost access to an export scoped to its own Jobs")
	}
	if ownerScope.contains(other) {
		t.Fatal("another editor gained access to an owner-scoped export")
	}
	if !ownerScope.contains(admin) {
		t.Fatal("administrator could not read an owner-scoped export within its current visibility")
	}
	if ownerScope.contains(guest) {
		t.Fatal("read-only owner gained access to an owner-scoped export")
	}
	if got := ownerScope.access(); got != (jobs.Access{UserID: owner.UserID}) {
		t.Fatalf("owner-scoped export query access = %+v, want owner-only access", got)
	}
}

func TestSummaryExportLegacyArtifactWithoutPersistedScopeIsHidden(t *testing.T) {
	ctx := newJobHarnessContext(t, false)
	owner, err := ctx.CreateUser(&UserInput{Username: "legacy-summary-owner", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	from := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	input, err := json.Marshal(jobSummaryExportInput{
		From: from, To: from.Add(91 * 24 * time.Hour), Format: "json",
		Scope: jobSummaryExportDataScope{Class: jobSummaryExportOwnerScope, OwnerUserID: owner.ID},
	})
	if err != nil {
		t.Fatalf("encode summary export input: %v", err)
	}
	accepted, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindSummaryExport, KindVersion: jobSummaryExportVersion,
		State: jobs.StateQueued, OwnerUserID: &owner.ID, ActorUserID: &owner.ID,
		Origin: "api", Title: "Legacy summary export", Replay: jobs.ReplayInput{Input: input},
	})
	if err != nil {
		t.Fatalf("accept summary export: %v", err)
	}
	// This is the reference shape persisted before scope metadata was added.
	publishTestOutput(t, ctx, accepted.ID, jobSummaryExportOutput, jobs.OutputTypeArtifact,
		`{"path":"_exports/job-summaries/legacy.json","size":20}`, jobs.OutputAvailable, nil)
	ownerCtx := ctx.WithPrincipal(auth.FromUser(owner))
	outputs, err := ownerCtx.GetOpenableJobOutputs(accepted.ID)
	if err != nil || len(outputs) != 0 {
		t.Fatalf("legacy summary output metadata = %#v, err=%v; want hidden", outputs, err)
	}
	if _, err := ownerCtx.OpenJobOutput(context.Background(), accepted.ID, jobSummaryExportOutput); !errors.Is(err, ErrJobOutputForbidden) {
		t.Fatalf("legacy summary output open error = %v, want forbidden", err)
	}
}
