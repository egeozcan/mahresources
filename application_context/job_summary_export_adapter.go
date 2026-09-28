package application_context

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"slices"
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
	// PrincipalUserID preserves the requester's preference identity for an
	// administrator query. It does not narrow the administrator's visibility.
	PrincipalUserID uint `json:"principalUserId,omitempty"`
}

func (scope jobSummaryExportDataScope) valid() bool {
	switch scope.Class {
	case jobSummaryExportAdminScope:
		return scope.OwnerUserID == 0
	case jobSummaryExportOwnerScope:
		return scope.OwnerUserID != 0 && scope.PrincipalUserID == 0
	default:
		return false
	}
}

func (scope jobSummaryExportDataScope) access() jobs.Access {
	if scope.Class == jobSummaryExportAdminScope {
		return jobs.Access{UserID: scope.PrincipalUserID, Administrator: true}
	}
	return jobs.Access{UserID: scope.OwnerUserID}
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
		return jobSummaryExportDataScope{Class: jobSummaryExportAdminScope, PrincipalUserID: principal.UserID}, nil
	}
	if principal.UserID == 0 {
		return jobSummaryExportDataScope{}, errors.New("a Job summary export needs a principal with a durable owner")
	}
	return jobSummaryExportDataScope{Class: jobSummaryExportOwnerScope, OwnerUserID: principal.UserID}, nil
}

// jobSummaryExportDescription is the export's readable summary: its range, its
// format and the filter it applies, so a list of exports reads as a list of
// different questions rather than one title repeated.
type jobSummaryExportDescription struct {
	From   time.Time           `json:"from"`
	To     time.Time           `json:"to"`
	Format string              `json:"format"`
	Filter map[string][]string `json:"filter,omitempty"`
}

// jobSummaryFilterTerms is a sealed filter in the list API's own parameter names
// and spellings, one term per value, in a fixed order: what an export's Job and
// its file say it summarizes. The dimensions an export refuses to seal
// (unsealableSummaryFilterDimension) never reach it.
func jobSummaryFilterTerms(filter jobs.Filter) [][2]string {
	var terms [][2]string
	add := func(name string, values ...string) {
		for _, value := range values {
			terms = append(terms, [2]string{name, value})
		}
	}
	id := func(value *uint) string { return strconv.FormatUint(uint64(*value), 10) }
	instant := func(value *time.Time) string { return value.UTC().Format(time.RFC3339Nano) }
	add("state", filter.States...)
	add("kind", filter.Kinds...)
	add("origin", filter.Origins...)
	if filter.OwnerID != nil {
		add("ownerId", id(filter.OwnerID))
	}
	if filter.ActorID != nil {
		add("actorId", id(filter.ActorID))
	}
	if filter.AcceptedAfter != nil {
		add("acceptedAfter", instant(filter.AcceptedAfter))
	}
	if filter.AcceptedBefore != nil {
		add("acceptedBefore", instant(filter.AcceptedBefore))
	}
	if filter.Relationship != "" {
		add("relationship", filter.Relationship)
	}
	if filter.Search != "" {
		add("search", filter.Search)
	}
	if filter.Pinned != nil {
		add("pinned", strconv.FormatBool(*filter.Pinned))
	}
	if filter.Dismissed != nil {
		add("dismissed", strconv.FormatBool(*filter.Dismissed))
	}
	if filter.Command != "" {
		add("command", filter.Command)
	}
	return terms
}

// jobSummaryFilterValues is jobSummaryFilterTerms keyed by parameter, as a query
// string holds it. It is never nil, so a JSON export with no filter says so.
func jobSummaryFilterValues(filter jobs.Filter) map[string][]string {
	values := map[string][]string{}
	for _, term := range jobSummaryFilterTerms(filter) {
		values[term[0]] = append(values[term[0]], term[1])
	}
	return values
}

// jobSummaryExportTitle names the range an export covers, by day.
func jobSummaryExportTitle(from, to time.Time) string {
	return fmt.Sprintf("Job summary, %s to %s", from.UTC().Format("2006-01-02"), to.UTC().Format("2006-01-02"))
}

func jobSummaryExportCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: func(raw json.RawMessage) (json.RawMessage, error) {
			input, err := jobSummaryExportInputOf(raw)
			if err != nil {
				return nil, err
			}
			description := jobSummaryExportDescription{From: input.From, To: input.To, Format: input.Format}
			if values := jobSummaryFilterValues(input.Filter); len(values) > 0 {
				description.Filter = values
			}
			return json.Marshal(description)
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

// unsealableSummaryFilterDimension names a filter dimension an export may not
// seal, or "" when there is none. An export's filter is decoded by whichever
// worker claims it, with plain JSON, so a worker from a release that predates a
// dimension drops it and exports a wider summary than was asked for, with no
// error; a v1 worker that meets the partial token refuses the whole filter. The
// dimensions below postdate v1 and are refused here instead of sealed. Offering
// them to exports needs a Kind version that older workers do not claim, and a
// way back for the ones such a worker blocks as adapter-missing.
func unsealableSummaryFilterDimension(filter jobs.Filter) string {
	switch {
	case slices.Contains(filter.States, jobs.FilterStatePartial):
		return "the partially completed state"
	case filter.InboundRelationship != "":
		return "inboundRelationship"
	case filter.NoInboundRelationship != "":
		return "noInboundRelationship"
	case filter.OwnerDeleted:
		return "ownerDeleted"
	case filter.OwnedByViewer:
		return "owner=me"
	}
	return ""
}

// SummaryExportUnsealableDimension names the filter dimension a summary export
// refuses to seal, or "" when it can seal the filter: the rule acceptance
// applies, for a page that offers the export.
func SummaryExportUnsealableDimension(filter jobs.Filter) string {
	return unsealableSummaryFilterDimension(filter)
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
	if dimension := unsealableSummaryFilterDimension(input.Filter); dimension != "" {
		return nil, fmt.Errorf("%w: a Job summary export cannot filter by %s", jobs.ErrInvalidFilter, dimension)
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
	bound, err := a.forExecution(execution)
	if err != nil {
		return a.ctx.answerDispatchCheck(execution, err)
	}
	a = bound
	a.ctx.dispatchChecksAnswered(execution.JobID)
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
	content, err := encodeJobSummaryExport(summary, input.Filter, input.Format)
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

// forExecution binds the account the export acts as. A read that failed binds
// nothing and is returned (dispatchBinding).
func (a *jobSummaryExportAdapter) forExecution(execution jobs.Execution) (*jobSummaryExportAdapter, error) {
	if a.ctx == nil || execution.Access.UserID == 0 {
		return a, nil
	}
	bound, err := a.ctx.dispatchBinding(execution.Access.UserID)
	if err != nil {
		return nil, err
	}
	return &jobSummaryExportAdapter{ctx: bound}, nil
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

// encodeJobSummaryExport writes one export's file. Both formats say what the
// numbers describe before the numbers: the range, and the filter in the list's
// own parameter names, so a file downloaded and set aside still says which
// question it answers.
func encodeJobSummaryExport(summary jobs.Summary, filter jobs.Filter, format string) ([]byte, error) {
	if format == "json" {
		return json.MarshalIndent(struct {
			jobs.Summary
			Filter map[string][]string `json:"filter"`
		}{summary, jobSummaryFilterValues(filter)}, "", "  ")
	}
	var builder strings.Builder
	writer := csv.NewWriter(&builder)
	if err := writer.Write([]string{"metric", "dimension", "value"}); err != nil {
		return nil, err
	}
	write := func(metric, dimension, value string) error {
		return writer.Write([]string{metric, dimension, value})
	}
	if err := write("range", "from", summary.From.UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	if err := write("range", "to", summary.To.UTC().Format(time.RFC3339Nano)); err != nil {
		return nil, err
	}
	for _, term := range jobSummaryFilterTerms(filter) {
		if err := write("filter", term[0], term[1]); err != nil {
			return nil, err
		}
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
		Title: jobSummaryExportTitle(input.From, input.To), Replay: jobs.ReplayInput{Input: encoded},
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
