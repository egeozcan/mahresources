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
	"strings"
	"time"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/jobs"
	"mahresources/server/jobview"
	"mahresources/server/template_handlers/template_entities"
)

// JobCenterCutoverEnabled is the single release gate for the canonical Job UI.
const JobCenterCutoverEnabled = true

// jobListPageSize is how many Jobs one /jobs page shows.
const jobListPageSize = jobs.DefaultPageSize

// dismissedAny is the page-only value of the Dismissed filter that asks for
// every Job whatever the viewer dismissed. The page's default list is the
// undismissed one, so "not asked" cannot also mean "any" here as it does on the
// API.
const dismissedAny = "any"

// Quick-filter groupings. Their names are the glossary's (CONTEXT.md): Active
// and Finished Jobs, and the Jobs that Need Attention.
var (
	jobStatesNeedingAttention = []string{string(jobs.StateBlocked), string(jobs.StateFailed), string(jobs.StateInterrupted)}
	jobStatesActive           = []string{string(jobs.StateScheduled), string(jobs.StateQueued), string(jobs.StateRunning), string(jobs.StatePaused)}
	jobStatesFinished         = []string{string(jobs.StateSucceeded), string(jobs.StateFailed), string(jobs.StateCancelled), string(jobs.StateInterrupted)}
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

// JobRow is one Job as the list card draws it.
type JobRow struct {
	ID             string
	Title          string
	Kind           string
	State          string
	StateLabel     string
	Phase          string
	Pinned         bool
	SummaryText    string
	FailureMessage string
	Accepted       JobRowTime
	// Started and Finished are pre-formatted because a nil *time.Time is truthy
	// in a pongo2 `if`: the empty string is what the template tests.
	Started   JobRowTime
	Finished  JobRowTime
	Version   uint64
	Progress  *JobRowProgress
	Result    jobview.ResultLink
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
	Search         string
	Command        string
	Kinds          []string
	States         []string
	Origins        []string
	OriginText     string
	OwnerID        string
	ActorID        string
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
		base["jobKindOptions"] = jobKindOptions(reader.VisibleJobKinds(), jobFilterForm(query).Kinds)

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
		base["jobs"] = rows
		base["jobQuickFilters"] = quickFilters

		if pagination := jobListPagination(request.URL, page); pagination != nil {
			base["pagination"] = pagination
		}
		return base
	}
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

// jobListFilter reads the page's filter. The sidebar form submits every field,
// so an empty value means "not asked" and is dropped before parsing — the API
// refuses `command=` and an empty origin, which a person never typed. The page
// lists what the viewer has not dismissed unless they ask otherwise;
// `dismissed=any` is that asking.
func jobListFilter(query url.Values) (jobs.Filter, error) {
	query = withoutEmptyValues(query)
	dismissed := query.Get("dismissed")
	if dismissed == dismissedAny {
		query.Del("dismissed")
	}
	filter, err := jobview.ParseFilter(query)
	if err != nil {
		return jobs.Filter{}, err
	}
	if dismissed == "" {
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
		return addMessageErrContext(err.Error(), http.StatusBadRequest, ctx)
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
		AcceptedAfter:         datetimeInputValue(query.Get("acceptedAfter"), false),
		AcceptedBefore:        datetimeInputValue(query.Get("acceptedBefore"), true),
		Relationship:          query.Get("relationship"),
		InboundRelationship:   query.Get("inboundRelationship"),
		NoInboundRelationship: query.Get("noInboundRelationship"),
		Pinned:                query.Get("pinned"),
		Dismissed:             query.Get("dismissed"),
	}
	form.AcceptedAfterInstant = boundInstant(query.Get("acceptedAfter"), false)
	form.AcceptedBeforeInstant = boundInstant(query.Get("acceptedBefore"), true)
	// Every origin stays in the one field, comma-separated as the parser reads
	// it; showing only the first would drop the rest on the next submit.
	form.OriginText = strings.Join(form.Origins, ", ")
	if form.Dismissed == "false" {
		form.Dismissed = ""
	}
	return form
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
	row := JobRow{
		ID: snapshot.ID, Title: title, Kind: snapshot.Kind, State: string(snapshot.State),
		StateLabel: jobStateLabel(snapshot), Phase: snapshot.Phase, Pinned: snapshot.Pinned,
		SummaryText: jobSummaryText(snapshot.Summary), Accepted: jobRowTime(&snapshot.AcceptedAt),
		Started: jobRowTime(snapshot.StartedAt), Finished: jobRowTime(snapshot.FinishedAt), Version: snapshot.Version,
		Progress:  jobRowProgress(snapshot),
		DetailURL: "/job?id=" + url.QueryEscape(snapshot.ID),
	}
	if jobIsPartial(snapshot) {
		// The badge already says it; the raw phase beside it would say it twice.
		row.Phase = ""
	}
	if snapshot.Failure != nil {
		row.FailureMessage = snapshot.Failure.Message
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
		"phase": snapshot.Phase, "version": snapshot.Version, "pinned": snapshot.Pinned,
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
	if jobIsPartial(snapshot) {
		return "Partially completed"
	}
	text := strings.ReplaceAll(string(snapshot.State), "-", " ")
	if text == "" {
		return "Unknown"
	}
	return strings.ToUpper(text[:1]) + text[1:]
}

// jobSummaryText shows a Job's structured summary: a JSON string as its text,
// anything else as compact JSON.
func jobSummaryText(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		return text
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, trimmed); err != nil {
		return ""
	}
	return compact.String()
}

func jobIsActive(state jobs.State) bool {
	return slices.Contains(jobStatesActive, string(state))
}

// jobRowProgress mirrors the progress rules the detail page and the panel use.
// A succeeded Job reads as complete whatever its last progress row said: nothing
// rewrites progress on the way out, so a download whose size was never known
// would otherwise keep an unknown total forever. Stopped work with no progress
// data draws no bar at all.
func jobRowProgress(snapshot jobs.Snapshot) *JobRowProgress {
	progress := snapshot.Progress
	hasData := progress.Completed != nil || progress.Total != nil || progress.Message != "" || progress.Phase != ""
	succeeded := snapshot.State == jobs.StateSucceeded
	if !hasData && !succeeded && !jobIsActive(snapshot.State) {
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
	out.Indeterminate = jobIsActive(snapshot.State)
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
