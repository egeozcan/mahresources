package application_context

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mahresources/jobs"
	"mahresources/models"

	"gorm.io/gorm"
)

// SelectCommandJobs implements the exact command selector for both download
// Kinds. The plugin named by the sanitized summary is part of current command
// authority for scoped principals, so it is checked as a correlated database
// predicate instead of one lookup per Job.
func (a *downloadJobAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	states, supported := downloadCommandStates(request.Key)
	if !supported {
		return nil, false, nil
	}
	allowed, scoped := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed {
		return emptyCommandSelection(request), true, nil
	}
	query := request.Jobs
	if states != nil {
		query = query.Where("jobs.state IN ?", states)
	}
	if scoped {
		plugin := jobSummaryTextExpr(request.Deps.DB, "plugin")
		query = query.Where("(COALESCE("+plugin+", '') = '' OR "+pluginScopedAccessPredicate(plugin)+")", true, true)
	}
	return query.Select("jobs.id"), true, nil
}

func downloadCommandStates(key string) ([]jobs.State, bool) {
	switch key {
	case jobs.CommandCancel:
		return nil, true
	case jobs.CommandResume:
		return []jobs.State{jobs.StateBlocked, jobs.StatePaused}, true
	case jobs.CommandRetry:
		return unsuccessfulJobStates(), true
	default:
		return nil, false
	}
}

func (a *groupExportAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	states, supported := exportCommandStates(request.Key)
	if !supported {
		return nil, false, nil
	}
	allowed, _ := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed {
		return emptyCommandSelection(request), true, nil
	}
	query := request.Jobs
	if states != nil {
		query = query.Where("jobs.state IN ?", states)
	}
	return query.Select("jobs.id"), true, nil
}

func exportCommandStates(key string) ([]jobs.State, bool) {
	switch key {
	case jobs.CommandCancel:
		return nil, true
	case jobs.CommandRetry:
		return unsuccessfulJobStates(), true
	case jobs.CommandRepeat:
		return []jobs.State{jobs.StateSucceeded}, true
	default:
		return nil, false
	}
}

func (a *importParseAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	states, supported := importParseCommandStates(request.Key)
	if !supported {
		return nil, false, nil
	}
	allowed, _ := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed {
		return emptyCommandSelection(request), true, nil
	}
	query := request.Jobs
	if states != nil {
		query = query.Where("jobs.state IN ?", states)
	}
	if request.Key == jobs.CommandRetry {
		query = query.Where(importFactPredicate(request.Deps.DB, "handle", "archive_available"), true)
	}
	return query.Select("jobs.id"), true, nil
}

func importParseCommandStates(key string) ([]jobs.State, bool) {
	switch key {
	case jobs.CommandCancel:
		return nil, true
	case jobs.CommandRetry:
		return unsuccessfulJobStates(), true
	default:
		return nil, false
	}
}

func (a *importApplyAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	states, supported := importApplyCommandStates(request.Key)
	if !supported {
		return nil, false, nil
	}
	allowed, _ := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed {
		return emptyCommandSelection(request), true, nil
	}
	query := request.Jobs
	if states != nil {
		query = query.Where("jobs.state IN ?", states)
	}
	if request.Key == jobs.CommandRetry {
		query = query.Where(importFactPredicate(request.Deps.DB, "parseHandle", "archive_available"), true)
		query = query.Where(importFactPredicate(request.Deps.DB, "parseHandle", "plan_available"), true)
	}
	return query.Select("jobs.id"), true, nil
}

func importApplyCommandStates(key string) ([]jobs.State, bool) {
	switch key {
	case jobs.CommandCancel:
		return commandNonterminalJobStates(), true
	case jobs.CommandRetry:
		return unsuccessfulJobStates(), true
	default:
		return nil, false
	}
}

func (a *reductionComputeAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	states, supported := reductionCommandStates(request.Key)
	if !supported {
		return nil, false, nil
	}
	allowed, _ := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed {
		return emptyCommandSelection(request), true, nil
	}
	query := request.Jobs
	if states != nil {
		query = query.Where("jobs.state IN ?", states)
	}
	if request.Key == jobs.CommandRetry {
		summaryID := jobSummaryTextExpr(request.Deps.DB, "reductionId")
		idCast := "(CASE WHEN " + summaryID + " <> '' AND " + summaryID + " NOT GLOB '*[^0-9]*' THEN CAST(" + summaryID + " AS INTEGER) END)"
		if request.Deps.DB.Dialector.Name() == "postgres" {
			idCast = "(CASE WHEN " + summaryID + " ~ '^[0-9]+$' THEN CAST(" + summaryID + " AS bigint) END)"
		}
		now := time.Now()
		query = query.Where(`EXISTS (
			SELECT 1 FROM resource_reductions r
			WHERE r.id = `+idCast+`
			  AND (r.status = ? OR (r.status = ? AND r.compute_deadline IS NOT NULL AND r.compute_deadline < ?))
		)`, models.ReductionStatusFailed, models.ReductionStatusComputing, now)
	}
	return query.Select("jobs.id"), true, nil
}

func reductionCommandStates(key string) ([]jobs.State, bool) {
	switch key {
	case jobs.CommandCancel:
		return nil, true
	case jobs.CommandRetry:
		return unsuccessfulJobStates(), true
	default:
		return nil, false
	}
}

func (a *similarityRecomputeAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	states, supported := maintenanceCommandStates(request.Key)
	if !supported {
		return nil, false, nil
	}
	allowed, _ := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed {
		return emptyCommandSelection(request), true, nil
	}
	query := request.Jobs
	if states != nil {
		query = query.Where("jobs.state IN ?", states)
	}
	return query.Select("jobs.id"), true, nil
}

func maintenanceCommandStates(key string) ([]jobs.State, bool) {
	switch key {
	case jobs.CommandCancel:
		return nil, true
	case jobs.CommandRetry:
		return unsuccessfulJobStates(), true
	default:
		return nil, false
	}
}

func (a *pluginActionAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	switch request.Key {
	case jobs.CommandRetry, jobs.CommandContinue:
	default:
		return nil, false, nil
	}
	allowed, scoped := a.ctx.commandActorAllowed(request.Deps, request.Access)
	if !allowed || a.ctx == nil || a.ctx.PluginManager() == nil {
		return emptyCommandSelection(request), true, nil
	}
	pm := a.ctx.PluginManager()
	var actionPairs [][2]string
	for _, entity := range []string{"resource", "note", "group"} {
		for _, action := range pm.GetActions(entity, nil) {
			if action.Retryable {
				actionPairs = append(actionPairs, [2]string{action.PluginName, action.ID})
			}
		}
	}
	// Only Retry covers scheduled occurrences: a schedule has a next tick rather
	// than a continuation, so a succeeded occurrence never advertises one.
	var schedulePairs [][2]string
	if request.Key == jobs.CommandRetry {
		for _, schedule := range pm.AllDeclaredSchedules() {
			if schedule.Retryable {
				schedulePairs = append(schedulePairs, [2]string{schedule.PluginName, schedule.ScheduleID})
			}
		}
	}
	if len(actionPairs)+len(schedulePairs) == 0 {
		return emptyCommandSelection(request), true, nil
	}

	plugin := jobSummaryTextExpr(request.Deps.DB, "plugin")
	action := jobSummaryTextExpr(request.Deps.DB, "action")
	schedule := jobSummaryTextExpr(request.Deps.DB, "scheduleId")
	subtype := jobSummaryTextExpr(request.Deps.DB, "subtype")
	query := request.Jobs
	if request.Key == jobs.CommandContinue {
		// A continuation starts from a Job that succeeded while its Kind declared
		// it unfinished — the partial phase is that statement — and only a
		// registered action can carry one.
		query = query.Where("jobs.state = ?", jobs.StateSucceeded).
			Where("jobs.phase = ?", pluginActionPhasePartial)
	} else {
		query = query.Where("jobs.state IN ?", unsuccessfulJobStates())
	}
	var clauses []string
	var args []any
	appendPairs := func(subtypeValue, field string, pairs [][2]string) {
		if len(pairs) == 0 {
			return
		}
		for _, pair := range pairs {
			clauses = append(clauses, "("+subtype+" = ? AND "+plugin+" = ? AND "+field+" = ?)")
			args = append(args, subtypeValue, pair[0], pair[1])
		}
	}
	appendPairs(pluginActionSubtypeRegistered, action, actionPairs)
	appendPairs(pluginActionSubtypeScheduled, schedule, schedulePairs)
	query = query.Where("("+strings.Join(clauses, " OR ")+")", args...)
	if scoped {
		query = query.Where(pluginScopedAccessPredicate(plugin), true, true)
	}
	return query.Select("jobs.id"), true, nil
}

// Plugin command inspection is visible to the same principals as the Job, and
// cancellation is narrowed by the host's nonterminal-state predicate. Import
// Retry shares its SQL fact predicate with the detail advertisement and never
// probes the exchange filesystem once per Job during list or summary reads.
func (a *pluginCommandJobAdapter) SelectCommandJobs(_ context.Context, request jobs.CommandFilterRequest) (*gorm.DB, bool, error) {
	switch request.Key {
	case jobs.CommandCancel, pluginCommandInspectKey:
		return request.Jobs.Select("jobs.id"), true, nil
	case pluginCommandImportRetryKey:
		if a.kind == JobKindPluginCommandImport {
			if a.ctx == nil {
				return emptyCommandSelection(request), true, nil
			}
			token, owned := a.ctx.pluginCommandImportRetryFenceToken()
			pluginNames := a.ctx.pluginCommandImportRetryPluginNames()
			keys := pluginRetryReplayKeyring(request.Deps)
			if !owned || len(pluginNames) == 0 || keys == nil {
				return emptyCommandSelection(request), true, nil
			}
			return pluginCommandImportRetrySelection(request.Deps.DB, request.Jobs, token, pluginNames,
				keys.KeyIDs(), pluginRetryReplayNow(request.Deps)), true, nil
		}
	}
	return nil, false, nil
}

// JobCommandFilterOption is one entry of the "Available command" filter: the
// key the filter sends and the words the select shows.
type JobCommandFilterOption struct {
	Key   string
	Label string
}

// JobCommandFilterOptions is the command vocabulary the Job list's "Available
// command" filter offers: the host's own keys and every key a Kind's selector
// above answers. The selectors are switches, so this list is kept beside them;
// a key a selector gains belongs here too, or the filter cannot offer it.
//
// A label is the generic name of the command. A Kind may advertise the same key
// under its own words (an export's Repeat is "Export again"), but the filter
// spans Kinds, so it names what every one of them shares.
func JobCommandFilterOptions() []JobCommandFilterOption {
	return []JobCommandFilterOption{
		{jobs.CommandCancel, "Cancel"},
		{jobs.CommandPause, "Pause"},
		{jobs.CommandResume, "Resume"},
		{jobs.CommandRetry, "Retry"},
		{jobs.CommandContinue, "Continue"},
		{jobs.CommandRepeat, "Repeat"},
		{pluginCommandInspectKey, "Inspect command history"},
		{pluginCommandImportRetryKey, "Retry import"},
		{jobs.CommandDismiss, "Dismiss"},
		{jobs.CommandPin, "Pin"},
		{jobs.CommandUnpin, "Unpin"},
		{jobs.CommandPinLineage, "Pin visible lineage"},
		{jobs.CommandForget, "Forget replay input"},
	}
}

func unsuccessfulJobStates() []jobs.State {
	return []jobs.State{jobs.StateFailed, jobs.StateCancelled, jobs.StateInterrupted}
}

func commandNonterminalJobStates() []jobs.State {
	return []jobs.State{jobs.StateScheduled, jobs.StateQueued, jobs.StateRunning, jobs.StatePaused, jobs.StateBlocked}
}

func emptyCommandSelection(request jobs.CommandFilterRequest) *gorm.DB {
	return request.Jobs.Where("1 = 0").Select("jobs.id")
}

// commandActorAllowed is the same current actor role check as
// commandActorRefusal, split out so one query can answer it for a whole selector.
func (ctx *MahresourcesContext) commandActorAllowed(deps jobs.Deps, access jobs.Access) (allowed, scoped bool) {
	if ctx == nil || access.Administrator || access.UserID == 0 {
		return true, false
	}
	principal := commandActorOn(deps.DB, access.UserID)
	if principal == nil {
		return true, false
	}
	if !principal.CanWrite() {
		return false, false
	}
	return true, !principal.IsAdmin() && (principal.IsScoped() || principal.RequiresScope())
}

func jobSummaryTextExpr(db *gorm.DB, field string) string {
	// The field names passed here are literals owned by this file.
	if db != nil && db.Dialector.Name() == "postgres" {
		return "(jobs.summary ->> '" + field + "')"
	}
	return "(CASE WHEN json_valid(jobs.summary) THEN json_extract(jobs.summary, '$." + field + "') END)"
}

func pluginScopedAccessPredicate(pluginExpr string) string {
	return `EXISTS (
		SELECT 1 FROM plugin_states ps
		WHERE ps.plugin_name = ` + pluginExpr + `
		  AND ps.enabled = ?
		  AND ps.allow_scoped_principals = ?
	)`
}

func importFactPredicate(db *gorm.DB, summaryField, availabilityColumn string) string {
	if availabilityColumn != "archive_available" && availabilityColumn != "plan_available" {
		panic(fmt.Sprintf("unknown import availability column %q", availabilityColumn))
	}
	parseHandle := jobSummaryTextExpr(db, summaryField)
	return `EXISTS (
		SELECT 1 FROM job_import_command_facts f
		WHERE f.parse_handle = ` + parseHandle + `
		  AND f.` + availabilityColumn + ` = ?
	)`
}
