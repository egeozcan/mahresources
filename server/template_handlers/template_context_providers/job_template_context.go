package template_context_providers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/jobs"
	"mahresources/server/http_utils"
	"mahresources/server/jobview"
	"mahresources/server/template_handlers/template_entities"
)

// JobCenterCutoverEnabled is the single release gate for the canonical Job UI.
const JobCenterCutoverEnabled = true

// jobListPageSize is how many Jobs one /jobs page shows.
const jobListPageSize = jobs.DefaultPageSize

// The administrator's Owner select is one field, owner, so it submits one choice:
// "Mine" (jobOwnerMineOption), an account's id, or a deleted account
// (jobOwnerDeletedOption). "Mine" is the API's own owner=me, so a link to
// /jobs?owner=me opens with it chosen and every later submit keeps it. The page
// reads an id as the API's ownerId and "deleted" as ownerDeleted=true; being
// choices of one select, none can be submitted beside another, which the list
// refuses.
const (
	jobOwnerMineOption    = "me"
	jobOwnerDeletedOption = "deleted"
)

// Quick-filter groupings. Their names are the glossary's (CONTEXT.md): Active
// and Finished Jobs, and the Jobs that Need Attention. They come from the state
// table every Job surface reads (jobview.PresentState), and Finished is every
// terminal state, so a failed Job is both Finished and Needs attention here.
var (
	jobStatesNeedingAttention = jobview.StatesInGroup("attention")
	jobStatesActive           = jobview.StatesInGroup("active")
	jobStatesFinished         = jobview.TerminalStates()
)

// JobListReader is what the /jobs page reads, as the viewer.
type JobListReader interface {
	ListJobs(filter jobs.Filter, cursor jobs.Cursor, limit int) (jobs.Page, error)
	ListJobsBefore(filter jobs.Filter, before jobs.Cursor, limit int) (jobs.Page, error)
	CountJobsByState(filter jobs.Filter) (map[string]int64, error)
	GetOpenableJobOutputs(jobID string) ([]jobs.Output, error)
	VisibleJobKinds() []string
}

var _ JobListReader = (*application_context.MahresourcesContext)(nil)

// JobAccountReader names the accounts a page of Jobs records as owners, and lists
// the accounts an administrator can filter by. It is a separate seam so a reader
// that cannot name anyone still renders the list, with no owner shown.
type JobAccountReader interface {
	JobAccountLabels(ids []uint) (map[uint]string, error)
	JobAccountOptions() ([]application_context.JobAccountOption, error)
}

var _ JobAccountReader = (*application_context.MahresourcesContext)(nil)

// JobRow is one Job as the list card draws it.
type JobRow struct {
	ID         string
	Title      string
	Kind       string
	State      string
	StateLabel string
	// BadgeClass is the badge colour the state's tone takes (jobRowBadgeClasses).
	BadgeClass string
	Phase      string
	Pinned     bool
	// SummaryText is a summary the Kind wrote as one sentence; SummaryFields is
	// one it wrote as fields (jobSummaryPresentation).
	SummaryText    string
	SummaryFields  []JobSummaryField
	FailureMessage string
	Accepted       JobRowTime
	// Started and Finished are pre-formatted because a nil *time.Time is truthy
	// in a pongo2 `if`: the empty string is what the template tests.
	Started  JobRowTime
	Finished JobRowTime
	// ScheduledFor is when scheduled work starts, set only while it is waiting
	// for that time.
	ScheduledFor JobRowTime
	Version      uint64
	Progress     *JobRowProgress
	// ProgressUpdatedAt is when the progress drawn was reported, for the page
	// to tell whether a live progress frame it holds is newer than a refresh.
	ProgressUpdatedAt string
	Result            jobview.ResultLink
	// Owner names whose Job this is, for an administrator reading somebody
	// else's: empty for the viewer's own Jobs and for work that never had an
	// owner.
	Owner     string
	DetailURL string
	// Entity is the selection payload the bulk bar reads.
	Entity string
}

// JobRowTime is an optional instant, formatted for a <time> element in the
// server's zone — every time on a card in the same one, so a page without
// JavaScript never mixes zones. The browser re-renders them in the reader's.
type JobRowTime struct {
	ISO     string
	Display string
	Minute  string
}

func jobRowTime(at *time.Time) JobRowTime {
	if at == nil || at.IsZero() {
		return JobRowTime{}
	}
	local := at.In(time.Local)
	return JobRowTime{ISO: local.Format(time.RFC3339), Display: local.Format("2006-01-02 15:04:05"), Minute: local.Format("2006-01-02 15:04")}
}

// JobRowProgress is a card's progress bar. Percent is set only when the total is
// known; an unknown total on Active work is Indeterminate, and on stopped work it
// is simply unknown.
type JobRowProgress struct {
	Text           string
	Percent        int
	Known          bool
	Indeterminate  bool
	AccessibleText string
	// Stats is the speed line under the bar: the live rate and time left while
	// the Job runs, its average rate once it has finished.
	Stats string
}

// JobQuickFilter is one sidebar link: a named slice of the list with the number
// of rows it opens.
type JobQuickFilter struct {
	Key    string
	Label  string
	Count  int64
	Link   string
	Active bool
}

// JobFilterForm is what the sidebar form shows as currently chosen.
type JobFilterForm struct {
	Search  string
	Command string
	Kinds   []string
	States  []string
	Origins []string
	// Owner is the administrator's Owner select: "me", an account id, or
	// "deleted" (jobOwnerChoice). OwnerID is the plain owner id field others see.
	Owner          string
	OwnerID        string
	ActorID        string
	OwnerDeleted   string
	AcceptedAfter  string
	AcceptedBefore string
	// The instants the bounds were read as, for the browser: it shows them in the
	// reader's own zone and sends an untouched one back exactly as it came.
	AcceptedAfterInstant  string
	AcceptedBeforeInstant string
	Relationship          string
	InboundRelationship   string
	NoInboundRelationship string
	Pinned                string
	Dismissed             string
}

// JobCenterListContextProvider renders /jobs: one keyset page of the Jobs the
// viewer may see, the quick filters and their counts, and the filter form.
func JobCenterListContextProvider(ctx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	var reader JobListReader
	if ctx != nil {
		reader = ctx
	}
	return jobListContextProvider(reader)
}

func jobListContextProvider(reader JobListReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		query := request.URL.Query()
		base := pongo2.Context{
			"pageTitle":               "Job Center",
			"jobCenterCutoverEnabled": JobCenterCutoverEnabled,
			"jobFilter":               jobFilterForm(query),
			"jobStateOptions":         jobStateOptions(),
			"jobCommandOptions":       jobCommandOptions(query.Get("command")),
			"jobInboundOptions":       jobInboundRelationshipOptions(query.Get("inboundRelationship")),
			"jobNoInboundOptions":     jobInboundRelationshipOptions(query.Get("noInboundRelationship")),
			"jobs":                    []JobRow{},
		}.Update(StaticTemplateCtx(request))
		if reader == nil {
			return base
		}
		if target := undismissedDefaultRedirect(request); target != "" {
			return pongo2.Context{"_redirect": target}
		}
		base["jobKindOptions"] = jobKindOptions(reader.VisibleJobKinds(), jobFilterForm(query).Kinds)
		base["jobOriginOptions"] = jobOriginOptions(jobFilterForm(query).Origins)

		filter, err := jobListFilter(query)
		if err != nil {
			return addMessageErrContext(err.Error(), http.StatusBadRequest, base)
		}
		after, err := jobview.DecodeCursor(query.Get("cursor"))
		if err != nil {
			return addMessageErrContext(err.Error(), http.StatusBadRequest, base)
		}
		before, err := jobview.DecodeCursor(query.Get("before"))
		if err != nil {
			return addMessageErrContext(err.Error(), http.StatusBadRequest, base)
		}

		var page jobs.Page
		if before.ID != "" {
			page, err = reader.ListJobsBefore(filter, before, jobListPageSize)
		} else {
			page, err = reader.ListJobs(filter, after, jobListPageSize)
		}
		if err != nil {
			return addJobListError(err, base)
		}
		quickFilters, err := jobQuickFilters(reader, request.URL, filter)
		if err != nil {
			return addJobListError(err, base)
		}

		rows := make([]JobRow, 0, len(page.Jobs))
		for _, snapshot := range page.Jobs {
			rows = append(rows, jobRow(reader, snapshot))
		}
		if accounts, ok := reader.(JobAccountReader); ok {
			if viewer := auth.PrincipalFromContext(request.Context()); viewer.IsAdmin() {
				if err := nameJobRowOwners(accounts, viewer.UserID, page.Jobs, rows); err != nil {
					return addJobListError(err, base)
				}
				options, err := accounts.JobAccountOptions()
				if err != nil {
					return addJobListError(err, base)
				}
				base["jobOwnerOptions"] = jobAccountSelectOptions(options, jobOwnerChoice(query))
				base["jobActorOptions"] = jobAccountSelectOptions(options, query.Get("actorId"))
			}
		}
		base["jobs"] = rows
		base["jobQuickFilters"] = quickFilters

		if pagination := jobListPagination(request.URL, page); pagination != nil {
			base["pagination"] = pagination
		}
		return base
	}
}

// nameJobRowOwners names the owner of every row an administrator reads that is
// not their own, with one read for the whole page. A deleted account says so; a
// Job that never had an owner shows none.
func nameJobRowOwners(accounts JobAccountReader, viewerID uint, snapshots []jobs.Snapshot, rows []JobRow) error {
	ids := make([]uint, 0, len(snapshots))
	for _, snapshot := range snapshots {
		if snapshot.OwnerUserID != nil && *snapshot.OwnerUserID != viewerID {
			ids = append(ids, *snapshot.OwnerUserID)
		}
	}
	labels, err := accounts.JobAccountLabels(ids)
	if err != nil {
		return err
	}
	for i, snapshot := range snapshots {
		switch {
		case snapshot.OwnerDeleted:
			rows[i].Owner = "deleted account"
		case snapshot.OwnerUserID == nil || *snapshot.OwnerUserID == viewerID:
		case labels[*snapshot.OwnerUserID] != "":
			rows[i].Owner = labels[*snapshot.OwnerUserID]
		default:
			rows[i].Owner = fmt.Sprintf("account %d", *snapshot.OwnerUserID)
		}
	}
	return nil
}

// jobAccountSelectOptions is the Owner or Actor select an administrator filters
// by: every account by name, plus the id the URL names when it is not among them
// (a deleted account's, from a bookmark), so resubmitting the form keeps it.
func jobAccountSelectOptions(accounts []application_context.JobAccountOption, current string) []JobSelectOption {
	options := make([]JobSelectOption, 0, len(accounts)+1)
	for _, account := range accounts {
		options = append(options, JobSelectOption{Value: strconv.FormatUint(uint64(account.ID), 10), Label: account.Label})
	}
	if current == jobOwnerMineOption || current == jobOwnerDeletedOption {
		return options
	}
	return withURLOption(options, current)
}

// jobKindOptions is the Kind checkboxes: the registered Kinds the viewer can see,
// plus any Kind the URL names that is not among them — a retained Job of a
// retired Kind — so resubmitting the form does not silently drop it.
func jobKindOptions(registered, requested []string) []string {
	options := append([]string(nil), registered...)
	for _, kind := range requested {
		if !slices.Contains(options, kind) {
			options = append(options, kind)
		}
	}
	return options
}

// jobOrigins are the origins the host records on the Jobs it accepts: a request
// to the API or a page ("api"), the CLI's commands ("cli"), a plugin ("plugin"),
// a schedule or a deferred start ("schedule"), and administrative maintenance
// ("admin"). A client may name its own origin on a command, so the vocabulary is
// not closed; a successor from the page keeps its ancestor's.
var jobOrigins = []string{"api", "cli", "plugin", "schedule", "admin"}

// jobOriginOptions is the Origin checkboxes: the host's origins, plus any origin
// the URL names that is not among them — one a client chose itself — so
// resubmitting the form does not silently drop it.
func jobOriginOptions(requested []string) []string {
	options := append([]string(nil), jobOrigins...)
	for _, origin := range requested {
		if !slices.Contains(options, origin) {
			options = append(options, origin)
		}
	}
	return options
}

// jobCommandOptions is the "Available command" select: the known vocabulary in
// words, plus the key the URL already names when it is not in it, so a link
// naming a key the list does not know keeps its filter when another one is
// changed.
func jobCommandOptions(current string) []JobSelectOption {
	var options []JobSelectOption
	for _, option := range application_context.JobCommandFilterOptions() {
		options = append(options, JobSelectOption{Value: option.Key, Label: option.Label})
	}
	return withURLOption(options, current)
}

// withURLOption appends the value a URL names when no option carries it, shown
// as the value itself.
func withURLOption(options []JobSelectOption, current string) []JobSelectOption {
	current = strings.TrimSpace(current)
	if current != "" && !slices.ContainsFunc(options, func(o JobSelectOption) bool { return o.Value == current }) {
		options = append(options, JobSelectOption{Value: current, Label: current})
	}
	return options
}

// JobSelectOption is one option of a sidebar select.
type JobSelectOption struct {
	Value string
	Label string
}

// jobInboundRelationshipOptions is the "Has been" and "Has not been" selects:
// the far end of each lineage link, read from the Job it points at — retried or
// continued, repeated, or made a child stage of another Job. A URL naming a
// value outside these keeps it as its own option, so resubmitting the form does
// not drop it.
func jobInboundRelationshipOptions(current string) []JobSelectOption {
	return withURLOption([]JobSelectOption{
		{Value: string(jobs.LinkRetryOf), Label: "retried or continued"},
		{Value: string(jobs.LinkRepeatOf), Label: "repeated"},
		{Value: string(jobs.LinkParentChild), Label: "a child stage"},
	}, current)
}

// JobStateOption is one checkbox of the State fieldset.
type JobStateOption struct {
	Value string
	Label string
}

// jobStateOptions is the State fieldset: every lifecycle state under its own
// spelling, and the filter-only partial token right after the succeeded state
// it narrows.
func jobStateOptions() []JobStateOption {
	options := make([]JobStateOption, 0, len(jobs.AllStates)+1)
	for _, state := range jobs.AllStates {
		options = append(options, JobStateOption{Value: string(state), Label: string(state)})
		if state == jobs.StateSucceeded {
			options = append(options, JobStateOption{Value: jobs.FilterStatePartial, Label: "partially completed"})
		}
	}
	return options
}

// undismissedDefaultRedirect answers the URL a page navigation to /jobs without a
// dismissed parameter is sent to: the same URL, every other parameter kept, with
// the page's default written into it as dismissed=false. It answers "" when the
// URL already names a value, and for the .json and .body variants, which a client
// fetches rather than navigates to.
//
// The page lists what the viewer has not dismissed unless asked otherwise, while
// the API reads an absent dismissed as no filter. Writing the default into the
// address means every URL the page shows a list under, and every link, form,
// pagination and live-refresh request built from it, names its dismissal filter,
// so the same query string gives the same Jobs on /jobs, /v1/jobs and the CLI.
func undismissedDefaultRedirect(request *http.Request) string {
	if strings.TrimSpace(request.URL.Query().Get("dismissed")) != "" {
		return ""
	}
	if strings.HasSuffix(request.URL.Path, ".body") || http_utils.TemplateRequestWantsJSON(request) {
		return ""
	}
	target := *request.URL
	values := target.Query()
	values.Set("dismissed", "false")
	target.RawQuery = values.Encode()
	// RequestURI, not String: the absolute form would let a proxied Host header
	// steer the redirect off-site.
	return target.RequestURI()
}

// jobListFilter reads the page's filter. The sidebar form submits every field,
// so an empty value means "not asked" and is dropped before parsing — the API
// refuses `command=` and an empty origin, which a person never typed. The page
// lists what the viewer has not dismissed unless they ask otherwise;
// `dismissed=any` is that asking, and reads as it does on the API.
func jobListFilter(query url.Values) (jobs.Filter, error) {
	query = withoutEmptyValues(query)
	switch owner := query.Get("owner"); {
	case owner == jobOwnerDeletedOption:
		query.Del("owner")
		query.Set("ownerDeleted", "true")
	case owner != "" && owner != jobOwnerMineOption && query.Get("ownerId") == "":
		query.Del("owner")
		query.Set("ownerId", owner)
	}
	filter, err := jobview.ParseFilter(query)
	if err != nil {
		return jobs.Filter{}, err
	}
	if query.Get("dismissed") == "" {
		undismissed := false
		filter.Dismissed = &undismissed
	}
	return filter, nil
}

func withoutEmptyValues(values url.Values) url.Values {
	out := make(url.Values, len(values))
	for key, list := range values {
		for _, value := range list {
			if strings.TrimSpace(value) != "" {
				out[key] = append(out[key], value)
			}
		}
	}
	return out
}

func addJobListError(err error, ctx pongo2.Context) pongo2.Context {
	if errors.Is(err, jobs.ErrInvalidFilter) || errors.Is(err, jobs.ErrInvalidCursor) ||
		errors.Is(err, jobs.ErrInvalidPage) || errors.Is(err, jobs.ErrInvalidCommand) {
		return addMessageErrContext(jobview.RequestErrorMessage(err), http.StatusBadRequest, ctx)
	}
	return addErrContext(err, ctx)
}

func jobFilterForm(query url.Values) JobFilterForm {
	form := JobFilterForm{
		Search:                query.Get("search"),
		Command:               query.Get("command"),
		Kinds:                 nonEmptyTokens(query, "kind", "kinds"),
		States:                nonEmptyTokens(query, "state", "states"),
		Origins:               nonEmptyTokens(query, "origin", "origins"),
		OwnerID:               query.Get("ownerId"),
		ActorID:               query.Get("actorId"),
		OwnerDeleted:          query.Get("ownerDeleted"),
		AcceptedAfter:         datetimeInputValue(query.Get("acceptedAfter"), false),
		AcceptedBefore:        datetimeInputValue(query.Get("acceptedBefore"), true),
		Relationship:          query.Get("relationship"),
		InboundRelationship:   query.Get("inboundRelationship"),
		NoInboundRelationship: query.Get("noInboundRelationship"),
		Pinned:                query.Get("pinned"),
		Dismissed:             query.Get("dismissed"),
	}
	form.Owner = jobOwnerChoice(query)
	if owner := query.Get("owner"); form.OwnerID == "" && owner != jobOwnerMineOption && owner != jobOwnerDeletedOption {
		form.OwnerID = owner
	}
	// The select shows the filter in effect, and with no value that is the
	// page's default.
	if strings.TrimSpace(form.Dismissed) == "" {
		form.Dismissed = "false"
	}
	form.AcceptedAfterInstant = boundInstant(query.Get("acceptedAfter"), false)
	form.AcceptedBeforeInstant = boundInstant(query.Get("acceptedBefore"), true)
	return form
}

// jobOwnerChoice is the Owner select's value for a query: its own owner
// parameter, or the choice the API's ownerId or ownerDeleted=true names.
func jobOwnerChoice(query url.Values) string {
	switch {
	case strings.TrimSpace(query.Get("owner")) != "":
		return query.Get("owner")
	case strings.TrimSpace(query.Get("ownerId")) != "":
		return query.Get("ownerId")
	case query.Get("ownerDeleted") == "true":
		return jobOwnerDeletedOption
	default:
		return ""
	}
}

func nonEmptyTokens(query url.Values, names ...string) []string {
	var out []string
	for _, name := range names {
		for _, value := range query[name] {
			for token := range strings.SplitSeq(value, ",") {
				if token = strings.TrimSpace(token); token != "" {
					out = append(out, token)
				}
			}
		}
	}
	return out
}

// boundInstant is the exact instant a bound was read as, or "" when it has none.
func boundInstant(raw string, end bool) string {
	at, err := jobview.Bound(raw, end)
	if err != nil || at == nil {
		return ""
	}
	return at.UTC().Format(time.RFC3339Nano)
}

// datetimeInputValue shows a bound in the sidebar's datetime input, in server
// time, without widening it. An RFC 3339 instant — a bookmark, or the legacy
// /downloads translator — keeps its time of day, to the second when it has one.
// A bare date names a whole day, so it shows as that day's first minute when it
// starts a range and its last minute when it ends one; resubmitting either reads
// back as the same bound.
func datetimeInputValue(raw string, end bool) string {
	if raw == "" {
		return ""
	}
	if day, err := time.ParseInLocation("2006-01-02", raw, time.Local); err == nil {
		if end {
			return day.Format("2006-01-02") + "T23:59"
		}
		return day.Format("2006-01-02") + "T00:00"
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		local := parsed.In(time.Local)
		if local.Second() != 0 || local.Nanosecond() != 0 {
			return local.Format("2006-01-02T15:04:05")
		}
		return local.Format("2006-01-02T15:04")
	}
	return raw
}

// jobQuickFilters counts the sidebar's slices. Each count applies every current
// filter except the one its slice replaces, so it is the number of rows the list
// shows with that slice chosen: what an inactive link opens, and what an active
// one — whose link clears it, as a tag chip's does — is showing now.
func jobQuickFilters(reader JobListReader, current *url.URL, filter jobs.Filter) ([]JobQuickFilter, error) {
	withoutStates := filter
	withoutStates.States = nil
	byState, err := reader.CountJobsByState(withoutStates)
	if err != nil {
		return nil, err
	}
	pinned := filter
	pinnedOnly := true
	pinned.Pinned = &pinnedOnly
	pinnedByState, err := reader.CountJobsByState(pinned)
	if err != nil {
		return nil, err
	}

	sum := func(counts map[string]int64, states []string) int64 {
		var total int64
		for _, state := range states {
			total += counts[state]
		}
		return total
	}
	var pinnedTotal int64
	for _, count := range pinnedByState {
		pinnedTotal += count
	}

	stateGroup := func(key, label string, states []string) JobQuickFilter {
		active := sameStringSet(filter.States, states)
		values := listQueryWithoutPaging(current)
		values.Del("state")
		values.Del("states")
		if !active {
			for _, state := range states {
				values.Add("state", state)
			}
		}
		return JobQuickFilter{Key: key, Label: label, Count: sum(byState, states), Link: jobsURL(values), Active: active}
	}

	pinnedActive := filter.Pinned != nil && *filter.Pinned
	pinnedValues := listQueryWithoutPaging(current)
	if pinnedActive {
		pinnedValues.Del("pinned")
	} else {
		pinnedValues.Set("pinned", "true")
	}

	return []JobQuickFilter{
		stateGroup("attention", "Needs attention", jobStatesNeedingAttention),
		stateGroup("active", "Active", jobStatesActive),
		stateGroup("finished", "Finished", jobStatesFinished),
		{Key: "pinned", Label: "Pinned by me", Count: pinnedTotal, Link: jobsURL(pinnedValues), Active: pinnedActive},
	}, nil
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for _, value := range left {
		if !slices.Contains(right, value) {
			return false
		}
	}
	return true
}

// listQueryWithoutPaging is the current query with its page position removed: a
// changed filter starts from the top.
func listQueryWithoutPaging(current *url.URL) url.Values {
	values := withoutEmptyValues(current.Query())
	values.Del("cursor")
	values.Del("before")
	values.Del("view")
	return values
}

// jobsURL is always /jobs, never the .body fragment a live refresh fetched.
func jobsURL(values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return "/jobs?" + encoded
	}
	return "/jobs"
}

func jobListPagination(current *url.URL, page jobs.Page) any {
	prev, next := jobListPageLinks(current, page)
	pagination := template_entities.KeysetPagination(prev, next)
	if pagination == nil {
		// Never a typed nil: pongo2 reads one as present.
		return nil
	}
	return pagination
}

// jobListPageLinks are the Previous and Next links of one page, keeping every
// filter and replacing only the position.
func jobListPageLinks(current *url.URL, page jobs.Page) (prev, next string) {
	link := func(param string, cursor *jobs.Cursor) string {
		if cursor == nil {
			return ""
		}
		token, err := jobview.EncodeCursor(*cursor)
		if err != nil {
			return ""
		}
		values := listQueryWithoutPaging(current)
		values.Set(param, token)
		return jobsURL(values)
	}
	return link("before", page.Prev), link("cursor", page.Next)
}

func jobRow(reader JobListReader, snapshot jobs.Snapshot) JobRow {
	title := strings.TrimSpace(snapshot.Title)
	if title == "" {
		title = snapshot.Kind
	}
	if title == "" {
		title = snapshot.ID
	}
	presentation := jobview.PresentJob(snapshot)
	row := JobRow{
		ID: snapshot.ID, Title: title, Kind: snapshot.Kind, State: string(snapshot.State),
		StateLabel: presentation.Label, BadgeClass: jobRowBadgeClasses[presentation.Tone],
		Phase: snapshot.Phase, Pinned: snapshot.Pinned,
		Accepted: jobRowTime(&snapshot.AcceptedAt),
		Started:  jobRowTime(snapshot.StartedAt), Finished: jobRowTime(snapshot.FinishedAt), Version: snapshot.Version,
		Progress:  jobRowProgress(snapshot),
		DetailURL: "/job?id=" + url.QueryEscape(snapshot.ID),
	}
	row.SummaryText, row.SummaryFields = jobSummaryPresentation(snapshot.Summary)
	if snapshot.ProgressUpdatedAt != nil {
		row.ProgressUpdatedAt = snapshot.ProgressUpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if jobIsPartial(snapshot) {
		// The badge already says it; the raw phase beside it would say it twice.
		row.Phase = ""
	}
	if snapshot.State == jobs.StateScheduled {
		row.ScheduledFor = jobRowTime(snapshot.ScheduledFor)
	}
	if snapshot.Failure != nil {
		// The code when the Kind gave no message, as the Jobs drawer does, so a
		// failed card never stands without a reason.
		row.FailureMessage = strings.TrimSpace(snapshot.Failure.Message)
		if row.FailureMessage == "" {
			row.FailureMessage = strings.TrimSpace(snapshot.Failure.Code)
		}
	}
	if snapshot.State == jobs.StateSucceeded {
		// A result link needs the outputs the viewer may open. One read per
		// succeeded row on the page, bounded by the page size; a Job whose outputs
		// cannot be read simply offers no link.
		if outputs, err := reader.GetOpenableJobOutputs(snapshot.ID); err == nil {
			row.Result = jobview.ResultLinkFor(snapshot, outputs)
		}
	}
	entity, _ := json.Marshal(map[string]any{
		"id": snapshot.ID, "title": title, "kind": snapshot.Kind, "state": snapshot.State,
		"phase": snapshot.Phase, "version": snapshot.Version, "pinned": snapshot.Pinned, "dismissed": snapshot.Dismissed,
	})
	row.Entity = string(entity)
	return row
}

// jobIsPartial reports a succeeded Job its Kind recorded as stopped short of
// finished — what the State filter's "partially completed" selects.
func jobIsPartial(snapshot jobs.Snapshot) bool {
	return snapshot.State == jobs.StateSucceeded && snapshot.Phase == jobs.PhasePartial
}

func jobStateLabel(snapshot jobs.Snapshot) string {
	return jobview.PresentJob(snapshot).Label
}

// jobRowBadgeClasses is the card badge each state tone takes. A paused Job is
// work still expected to go on, not an outcome; a blocked one needs attention
// without having failed.
var jobRowBadgeClasses = map[string]string{
	"working": "card-badge--live",
	"waiting": "card-badge--live",
	"paused":  "card-badge--live",
	"warning": "card-badge--warning",
	"done":    "card-badge--success",
	"failed":  "card-badge--danger",
	"neutral": "card-badge--muted",
}

// JobSummaryField is one field of a Job's summary as a card lists it.
type JobSummaryField struct {
	Label string
	Value string
}

// jobSummaryPresentation reads a Job's summary for a card. A summary the Kind
// wrote as a string is shown as that sentence. One written as an object is
// listed field by field, in the Kind's order, each under its key in words; its
// JSON text, braces and quotes and all, is notation nobody should have to read.
// A list's items are joined, a flag reads yes or no, a nested object reads as
// its own key: value pairs, and a null field is left out.
func jobSummaryPresentation(raw json.RawMessage) (string, []JobSummaryField) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return "", nil
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		// Not an object: the whole value is one field.
		var value any
		if err := decodeJSONValue(json.NewDecoder(bytes.NewReader(trimmed)), &value); err != nil {
			return "", nil
		}
		if shown := jobSummaryValue(value); shown != "" {
			return "", []JobSummaryField{{Label: "Summary", Value: shown}}
		}
		return "", nil
	}
	var fields []JobSummaryField
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return "", nil
		}
		key, _ := token.(string)
		var value any
		if err := decoder.Decode(&value); err != nil {
			return "", nil
		}
		if shown := jobSummaryValue(value); shown != "" {
			fields = append(fields, JobSummaryField{Label: jobSummaryLabel(key), Value: shown})
		}
	}
	return "", fields
}

func decodeJSONValue(decoder *json.Decoder, value *any) error {
	decoder.UseNumber()
	return decoder.Decode(value)
}

// jobSummaryValue is one summary value as text, or "" for nothing to show. A
// nested object's keys stay as written: they are a sub-document's own names,
// such as a list filter's parameters, and sorted, since the order is lost.
func jobSummaryValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	case bool:
		if typed {
			return "yes"
		}
		return "no"
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if shown := jobSummaryValue(item); shown != "" {
				parts = append(parts, shown)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			if shown := jobSummaryValue(typed[key]); shown != "" {
				parts = append(parts, key+": "+shown)
			}
		}
		return strings.Join(parts, "; ")
	}
	return fmt.Sprint(value)
}

// jobSummaryLabel is a summary key in words: a camelCase key split at each
// capital that follows a lower-case letter, the first word capitalised, an
// all-capital or numbered word (M2M) kept as written and Id written ID.
func jobSummaryLabel(key string) string {
	var words []string
	start := 0
	runes := []rune(key)
	for i := 1; i < len(runes); i++ {
		if unicode.IsUpper(runes[i]) && unicode.IsLower(runes[i-1]) {
			words = append(words, string(runes[start:i]))
			start = i
		}
	}
	words = append(words, string(runes[start:]))
	for i, word := range words {
		switch {
		case word == "Id" || word == "id":
			words[i] = "ID"
		case strings.ToUpper(word) == word:
		default:
			words[i] = strings.ToLower(word)
		}
	}
	if len(words) > 0 && words[0] != "" {
		first := []rune(words[0])
		first[0] = unicode.ToUpper(first[0])
		words[0] = string(first)
	}
	return strings.Join(words, " ")
}

// jobRowProgress mirrors the progress rules the detail page and the panel use.
// A succeeded Job reads as complete whatever its last progress row said: nothing
// rewrites progress on the way out, so a download whose size was never known
// would otherwise keep an unknown total forever. Work nobody is doing — waiting,
// paused or stopped — with nothing to report draws no bar at all, and only work
// being done (the state table's Working) may pulse or read "Working". A phase
// alone is not something to report: the card shows it beside the kind.
func jobRowProgress(snapshot jobs.Snapshot) *JobRowProgress {
	out := jobRowProgressBar(snapshot)
	if out != nil {
		out.Stats = jobRowStats(snapshot, time.Now())
	}
	return out
}

func jobRowProgressBar(snapshot jobs.Snapshot) *JobRowProgress {
	progress := snapshot.Progress
	working := jobview.PresentJob(snapshot).Working
	// A total alone reports no work done, so it is not something to show. A
	// metric is a figure reported, and every surface shows it as one.
	hasData := progress.Completed != nil || progress.Message != "" || len(progress.Metrics) > 0
	succeeded := snapshot.State == jobs.StateSucceeded
	if !hasData && !succeeded && !working {
		return nil
	}

	knownTotal := progress.Completed != nil && progress.Total != nil && *progress.Total > 0
	if jobIsPartial(snapshot) {
		// The run is over and its share is done, but the work is not: the bar
		// says what the badge says, keeping the run's last message, which is
		// usually what says how much is left.
		text := jobStateLabel(snapshot)
		if progress.Message != "" {
			text += ": " + progress.Message
		}
		return &JobRowProgress{Text: text, Percent: 100, Known: true, AccessibleText: text}
	}
	if succeeded && !(knownTotal && *progress.Completed >= *progress.Total) {
		return &JobRowProgress{Text: "Completed", Percent: 100, Known: true, AccessibleText: "Completed"}
	}

	out := &JobRowProgress{}
	switch {
	case progress.Message != "":
		out.Text = progress.Message
	case knownTotal:
		out.Text = fmt.Sprintf("%d / %d", *progress.Completed, *progress.Total)
		if progress.Unit != "" {
			out.Text += " " + progress.Unit
		}
	case progress.Phase != "":
		out.Text = progress.Phase
	case progress.Completed != nil:
		out.Text = fmt.Sprintf("%d", *progress.Completed)
	case !working && len(progress.Metrics) > 0:
		// Work nobody is doing that reported only metrics names the first of
		// them rather than claiming to be working.
		out.Text = jobMetricSummary(progress.Metrics[0])
	default:
		out.Text = "Working"
	}
	if knownTotal {
		percent := math.Round(float64(*progress.Completed) / float64(*progress.Total) * 100)
		out.Percent = int(math.Max(0, math.Min(100, percent)))
		out.Known = true
		out.AccessibleText = out.Text
		return out
	}
	out.Indeterminate = working
	if progress.Completed != nil {
		amount := fmt.Sprintf("%d", *progress.Completed)
		if progress.Unit != "" {
			amount += " " + progress.Unit
		}
		prefix := ""
		if progress.Message != "" || progress.Phase != "" {
			prefix = out.Text + "; "
		}
		out.AccessibleText = prefix + amount + " processed; total unknown"
	} else {
		out.AccessibleText = out.Text + "; total unknown"
	}
	return out
}

func JobDetailContextProvider(_ *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		return pongo2.Context{
			"pageTitle":               "Job detail",
			"hideSidebar":             true,
			"jobCenterCutoverEnabled": JobCenterCutoverEnabled,
		}.Update(StaticTemplateCtx(request))
	}
}
