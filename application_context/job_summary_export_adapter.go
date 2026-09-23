package application_context

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/auth"
	"mahresources/jobs"
)

const (
	// JobKindSummaryExport is a filtered long-range Job analytics export.
	JobKindSummaryExport       = "job-summary-export"
	jobSummaryExportVersion    = 1
	jobSummaryExportOutput     = "summary"
	jobSummaryExportDirectory  = "_exports/job-summaries"
	jobSummaryExportAdminScope = "administrator"
	jobSummaryExportOwnerScope = "owner"
)

// jobSummaryExportInput is the fixed, replayable question the export answers.
// The range is explicit so a retry does not silently move the analysis window.
type jobSummaryExportInput struct {
	Filter jobs.Filter               `json:"filter"`
	From   time.Time                 `json:"from"`
	To     time.Time                 `json:"to"`
	Format string                    `json:"format"`
	Scope  jobSummaryExportDataScope `json:"scope"`
}

// jobSummaryExportDataScope is the effective visibility of the data queried
// when the export was accepted. It is sealed with the replay input so output
// access and delayed execution can revalidate the same scope after a role
// change or process restart.
type jobSummaryExportDataScope struct {
	Class       string `json:"class"`
	OwnerUserID uint   `json:"ownerUserId,omitempty"`
}

func (scope jobSummaryExportDataScope) valid() bool {
	switch scope.Class {
	case jobSummaryExportAdminScope:
		return scope.OwnerUserID == 0
	case jobSummaryExportOwnerScope:
		return scope.OwnerUserID != 0
	default:
		return false
	}
}

func (scope jobSummaryExportDataScope) access() jobs.Access {
	return jobs.Access{UserID: scope.OwnerUserID, Administrator: scope.Class == jobSummaryExportAdminScope}
}

func (scope jobSummaryExportDataScope) contains(principal *auth.Principal) bool {
	if principal == nil || !principal.CanWrite() || !scope.valid() {
		return false
	}
	if principal.IsAdmin() {
		return true
	}
	return scope.Class == jobSummaryExportOwnerScope && principal.UserID == scope.OwnerUserID
}

func jobSummaryExportScopeFor(principal *auth.Principal, filter jobs.Filter) (jobSummaryExportDataScope, error) {
	if principal == nil || !principal.CanWrite() {
		return jobSummaryExportDataScope{}, ErrRoleCapability
	}
	if principal.IsAdmin() {
		if filter.OwnerID != nil && *filter.OwnerID != 0 {
			return jobSummaryExportDataScope{Class: jobSummaryExportOwnerScope, OwnerUserID: *filter.OwnerID}, nil
		}
		return jobSummaryExportDataScope{Class: jobSummaryExportAdminScope}, nil
	}
	if principal.UserID == 0 {
		return jobSummaryExportDataScope{}, errors.New("a Job summary export needs a principal with a durable owner")
	}
	return jobSummaryExportDataScope{Class: jobSummaryExportOwnerScope, OwnerUserID: principal.UserID}, nil
}

type jobSummaryExportDescription struct {
	From   time.Time `json:"from"`
	To     time.Time `json:"to"`
	Format string    `json:"format"`
}

func jobSummaryExportCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: func(raw json.RawMessage) (json.RawMessage, error) {
			input, err := jobSummaryExportInputOf(raw)
			if err != nil {
				return nil, err
			}
			return json.Marshal(jobSummaryExportDescription{From: input.From, To: input.To, Format: input.Format})
		},
		Encode: func(raw json.RawMessage) (json.RawMessage, error) {
			if _, err := jobSummaryExportInputOf(raw); err != nil {
				return nil, err
			}
			return raw, nil
		},
		Decode: func(raw json.RawMessage, version uint) (json.RawMessage, error) {
			if version != jobSummaryExportVersion {
				return nil, fmt.Errorf("%w: Job summary export v%d input", jobs.ErrReplayCodecUnregistered, version)
			}
			if _, err := jobSummaryExportInputOf(raw); err != nil {
				return nil, err
			}
			return raw, nil
		},
		Migrate: func(raw json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no summary export input migration from v%d to v%d", fromVersion, toVersion)
			}
			return raw, nil
		},
	}
}

func jobSummaryExportInputOf(raw json.RawMessage) (*jobSummaryExportInput, error) {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, errors.New("a Job summary export input is not valid JSON")
	}
	var input jobSummaryExportInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, fmt.Errorf("a Job summary export input is not readable: %w", err)
	}
	if !input.Scope.valid() {
		return nil, errors.New("a Job summary export needs a valid creation-time data scope")
	}
	if input.From.IsZero() || input.To.IsZero() || !input.From.Before(input.To) {
		return nil, errors.New("a Job summary export needs a start before its end")
	}
	if input.To.Sub(input.From) <= jobs.MaxSummaryWindow {
		return nil, fmt.Errorf("a Job summary export range must exceed %s", jobs.MaxSummaryWindow)
	}
	if input.Format != "csv" && input.Format != "json" {
		return nil, errors.New("a Job summary export format must be csv or json")
	}
	input.From = input.From.UTC()
	input.To = input.To.UTC()
	return &input, nil
}

type jobSummaryExportAdapter struct{ ctx *MahresourcesContext }

func (a *jobSummaryExportAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind: JobKindSummaryExport, KindVersion: jobSummaryExportVersion,
		Restorable: true, Visibility: jobs.VisibilityOwner,
	}
}

func (a *jobSummaryExportAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a == nil || a.ctx == nil {
		return errors.New("the Job summary export context is not available")
	}
	if execution.KindVersion != jobSummaryExportVersion {
		return fmt.Errorf("%w: Job summary export v%d input", jobs.ErrReplayCodecUnregistered, execution.KindVersion)
	}
	input, err := jobSummaryExportInputOf(execution.Input)
	if err != nil {
		return err
	}
	a = a.forExecution(execution)
	if execution.Access.UserID != 0 {
		if err := a.ctx.requireWriteRole("export a Job summary"); err != nil {
			return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, "role-refused")
		}
	}
	if !input.Scope.contains(a.ctx.Principal()) {
		return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, "scope-refused")
	}

	summary, err := a.ctx.getJobSummaryRange(input.Scope.access(), input.Filter, input.From, input.To)
	if err != nil {
		return fmt.Errorf("summarize Jobs for export: %w", err)
	}
	content, err := encodeJobSummaryExport(summary, input.Format)
	if err != nil {
		return err
	}
	path := jobSummaryExportPath(execution.JobID, input.Format)
	if err := writeJobSummaryExport(a.ctx.GetDefaultFs(), path, content); err != nil {
		return fmt.Errorf("write Job summary export: %w", err)
	}
	if err := a.ctx.publishQueueArtifact(execution, jobSummaryExportOutput, "Job summary export", path,
		a.ctx.exportArtifactExpiry(), input.Scope); err != nil {
		return fmt.Errorf("publish Job summary export: %w", err)
	}
	return a.ctx.finishQueueJob(execution, jobs.StateSucceeded, nil, []string{jobSummaryExportOutput})
}

func (a *jobSummaryExportAdapter) forExecution(execution jobs.Execution) *jobSummaryExportAdapter {
	if a.ctx == nil || execution.Access.UserID == 0 {
		return a
	}
	return &jobSummaryExportAdapter{ctx: a.ctx.WithPrincipal(a.ctx.principalForPluginActor(execution.Access.UserID))}
}

func (a *jobSummaryExportAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a == nil || a.ctx == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	input, err := jobSummaryExportInputOf(request.Input)
	if err != nil {
		return jobs.ReconcileFail, nil
	}
	path := jobSummaryExportPath(request.Snapshot.ID, input.Format)
	outputs, err := a.ctx.jobOutputsFor(request.Snapshot.ID)
	if err != nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if output, exists := findJobOutput(outputs, jobSummaryExportOutput); exists {
		if output.Availability != jobs.OutputAvailable {
			return jobs.ReconcileFail, nil
		}
		if _, err := a.ctx.GetDefaultFs().Stat(path); err != nil {
			return jobs.ReconcileFail, nil
		}
		return jobs.ReconcileSucceed, nil
	}
	if _, err := a.ctx.GetDefaultFs().Stat(path); err == nil {
		if err := a.ctx.publishQueueArtifact(request.Execution, jobSummaryExportOutput, "Job summary export", path,
			a.ctx.exportArtifactExpiry(), input.Scope); err != nil {
			return jobs.ReconcileExternalWorkUnproven, nil
		}
		return jobs.ReconcileSucceed, nil
	} else if !errors.Is(err, fs.ErrNotExist) {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
}

func (a *jobSummaryExportAdapter) CleanupArtifacts(_ context.Context, request jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	result := jobs.ArtifactCleanupResult{}
	for _, artifact := range request.Artifacts {
		removed, err := a.ctx.removeArtifact(artifact.Reference)
		if err != nil {
			return jobs.ArtifactCleanupResult{}, err
		}
		if removed {
			result.Removed = append(result.Removed, artifact.Key)
		}
	}
	return result, nil
}

func (*jobSummaryExportAdapter) Commands(context.Context, jobs.CommandContext) ([]jobs.Command, error) {
	return nil, nil
}

func (*jobSummaryExportAdapter) ExecuteCommand(context.Context, jobs.CommandExecution) (jobs.CommandOutcome, error) {
	return jobs.CommandOutcome{}, jobs.ErrCommandNotAdvertised
}

func (*jobSummaryExportAdapter) SelectCommandJobs(context.Context, jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	return nil, false, nil
}

func jobSummaryExportPath(jobID, format string) string {
	return fmt.Sprintf("%s/%s.%s", jobSummaryExportDirectory, jobID, format)
}

func writeJobSummaryExport(fileSystem afero.Fs, path string, content []byte) error {
	if err := fileSystem.MkdirAll(jobSummaryExportDirectory, 0755); err != nil {
		return err
	}
	partial := path + ".part"
	if err := afero.WriteFile(fileSystem, partial, content, 0644); err != nil {
		return err
	}
	if err := fileSystem.Rename(partial, path); err != nil {
		_ = fileSystem.Remove(partial)
		return err
	}
	return nil
}

// AuthorizeJobOutput binds every advertised and opened export to the effective
// visibility copied into its durable artifact reference. Older outputs without
// this proof are denied, and artifact access does not depend on replay retention.
func (a *jobSummaryExportAdapter) AuthorizeJobOutput(_ context.Context, request JobOutputOpenRequest) error {
	if a == nil || a.ctx == nil || request.Principal == nil ||
		request.Snapshot.Kind != JobKindSummaryExport || request.Snapshot.KindVersion != jobSummaryExportVersion ||
		request.Output.Key != jobSummaryExportOutput || request.Output.Type != jobs.OutputTypeArtifact {
		return ErrJobOutputForbidden
	}
	var reference queueArtifactReference
	if len(request.Output.Reference) == 0 || json.Unmarshal(request.Output.Reference, &reference) != nil ||
		reference.SummaryExportScope == nil || !reference.SummaryExportScope.valid() ||
		!reference.SummaryExportScope.contains(request.Principal) {
		return ErrJobOutputForbidden
	}
	if reference.SummaryExportScope.Class == jobSummaryExportOwnerScope &&
		(request.Snapshot.OwnerUserID == nil || *request.Snapshot.OwnerUserID != reference.SummaryExportScope.OwnerUserID) {
		return ErrJobOutputForbidden
	}
	var description jobSummaryExportDescription
	if len(request.Snapshot.Summary) == 0 || json.Unmarshal(request.Snapshot.Summary, &description) != nil ||
		(description.Format != "csv" && description.Format != "json") ||
		reference.Path != jobSummaryExportPath(request.Snapshot.ID, description.Format) {
		return ErrJobOutputForbidden
	}
	return nil
}

func encodeJobSummaryExport(summary jobs.Summary, format string) ([]byte, error) {
	if format == "json" {
		return json.MarshalIndent(summary, "", "  ")
	}
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write([]string{"metric", "dimension", "value"}); err != nil {
		return nil, err
	}
	write := func(metric, dimension, value string) error {
		return writer.Write([]string{metric, dimension, value})
	}
	for _, state := range sortedCountKeys(summary.ByState) {
		if err := write("jobs_by_state", state, strconv.FormatInt(summary.ByState[state], 10)); err != nil {
			return nil, err
		}
	}
	for _, kind := range sortedCountKeys(summary.ByKind) {
		if err := write("jobs_by_kind", kind, strconv.FormatInt(summary.ByKind[kind], 10)); err != nil {
			return nil, err
		}
	}
	for _, row := range []struct{ metric, value string }{
		{"total", strconv.FormatInt(summary.Total, 10)},
		{"succeeded", strconv.FormatInt(summary.Succeeded, 10)},
		{"failed", strconv.FormatInt(summary.Failed, 10)},
		{"terminal", strconv.FormatInt(summary.Terminal, 10)},
		{"success_rate", strconv.FormatFloat(summary.SuccessRate, 'f', -1, 64)},
		{"queue_median", summary.Queue.Median.String()},
		{"queue_p95", summary.Queue.P95.String()},
		{"run_median", summary.Run.Median.String()},
		{"run_p95", summary.Run.P95.String()},
	} {
		if err := write(row.metric, "", row.value); err != nil {
			return nil, err
		}
	}
	failures := append([]jobs.FailureClassCount(nil), summary.Failures...)
	sort.Slice(failures, func(i, j int) bool { return failures[i].Class < failures[j].Class })
	for _, failure := range failures {
		if err := write("failures_by_class", failure.Class, strconv.FormatInt(failure.Count, 10)); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return []byte(builder.String()), nil
}

func sortedCountKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// SubmitJobSummaryExport accepts one durable, owner-visible long-range analysis.
func (ctx *MahresourcesContext) SubmitJobSummaryExport(filter jobs.Filter, from, to time.Time, format, origin string) (jobs.Snapshot, error) {
	if ctx == nil {
		return jobs.Snapshot{}, errors.New("Job summary export context is unavailable")
	}
	if err := ctx.requireWriteRole("export a Job summary"); err != nil {
		return jobs.Snapshot{}, err
	}
	if err := jobs.ValidateFilter(filter); err != nil {
		return jobs.Snapshot{}, err
	}
	if len(origin) > jobs.MaxOriginBytes {
		return jobs.Snapshot{}, fmt.Errorf("origin may not exceed %d bytes", jobs.MaxOriginBytes)
	}
	scope, err := jobSummaryExportScopeFor(ctx.Principal(), filter)
	if err != nil {
		return jobs.Snapshot{}, err
	}
	input := jobSummaryExportInput{Filter: filter, From: from.UTC(), To: to.UTC(), Format: format, Scope: scope}
	encoded, err := json.Marshal(input)
	if err != nil {
		return jobs.Snapshot{}, err
	}
	if _, err := jobSummaryExportInputOf(encoded); err != nil {
		return jobs.Snapshot{}, err
	}
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Snapshot{}, err
	}
	owner := ctx.queueSubmitterOwner()
	return service.Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindSummaryExport, KindVersion: jobSummaryExportVersion,
		State: jobs.StateQueued, OwnerUserID: owner, ActorUserID: owner, Origin: origin,
		Title: "Job summary export", Replay: jobs.ReplayInput{Input: encoded},
	})
}

// GetJobSummaryRange is the facade used by the summary export executor.
func (ctx *MahresourcesContext) GetJobSummaryRange(filter jobs.Filter, from, to time.Time) (jobs.Summary, error) {
	return ctx.getJobSummaryRange(ctx.jobAccess(), filter, from, to)
}

func (ctx *MahresourcesContext) getJobSummaryRange(access jobs.Access, filter jobs.Filter, from, to time.Time) (jobs.Summary, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Summary{}, err
	}
	return service.SummaryRange(ctx.jobDeps(), access, filter, from, to)
}
