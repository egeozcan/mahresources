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
	KindLabel  string // the Kind in words, from server/jobview/job_vocabulary.json
	State      string
	StateLabel string
	// BadgeClass is the colour the state's tone takes (jobToneClass).
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
	// ProgressSnapshotJSON carries the public progress fields needed to keep a
	// running card's server-rendered bar and figures current between stream frames.
	ProgressSnapshotJSON string
	Result               jobview.ResultLink
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

type jobRowProgressSnapshot struct {
	Phase        string     `json:"phase,omitempty"`
	Completed    *int64     `json:"completed,omitempty"`
	Total        *int64     `json:"total,omitempty"`
	Unit         string     `json:"unit,omitempty"`
	Message      string     `json:"message,omitempty"`
	Rate         *float64   `json:"rate,omitempty"`
	ETA          *time.Time `json:"eta,omitempty"`
	ETAEstimated bool       `json:"etaEstimated,omitempty"`
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
		base["jobSummaryQuery"] = jobAPIQuery(query)
		base["jobOriginOptions"] = jobOriginOptions(jobFilterForm(query).Origins)
		// An administrator's Owner and Actor selects are drawn before the filter
		// is read, so a page refusing its filter still offers them with the
		// address's choices, and correcting the filter keeps them. With
		// authentication off every request is the one implicit administrator, so
		// there is nobody to filter by: the Owner and Actor filters are not
		// offered at all, and cards name no owner.
		accounts, _ := reader.(JobAccountReader)
		viewer := auth.PrincipalFromContext(request.Context())
		accountFilters := viewer == nil || !viewer.SuperUser
		base["jobAccountFilters"] = accountFilters
		if accounts != nil && accountFilters && viewer.IsAdmin() {
			options, err := accounts.JobAccountOptions()
			if err != nil {
				return addJobListError(err, base)
			}
			base["jobOwnerOptions"] = jobAccountSelectOptions(options, jobOwnerChoice(query))
			base["jobActorOptions"] = jobAccountSelectOptions(options, query.Get("actorId"))
		}

		filter, err := jobListFilter(query)
		if err != nil {
			return refuseJobListFilter(err, base)
		}
		after, err := jobview.DecodeCursor(query.Get("cursor"))
		if err != nil {
			return refuseJobListFilter(err, base)
		}
		before, err := jobview.DecodeCursor(query.Get("before"))
		if err != nil {
			return refuseJobListFilter(err, base)
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
		// An export is a write: an account that cannot write is not offered one.
		base["jobSummaryExportOffered"] = viewer == nil || viewer.CanWrite()
		base["jobSummaryExportQuery"], base["jobSummaryExportRefusal"] = jobSummaryExportFilter(query, filter, viewer)
		if accounts != nil && accountFilters && viewer.IsAdmin() {
			if err := nameJobRowOwners(accounts, viewer.UserID, page.Jobs, rows); err != nil {
				return addJobListError(err, base)
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
// retired Kind — so resubmitting the form does not silently drop it. Each is
// labelled in words and keeps its identifier as its value, so an address or a
// saved filter naming a Kind still selects it.
func jobKindOptions(registered, requested []string) []JobSelectOption {
	kinds := append([]string(nil), registered...)
	for _, kind := range requested {
		if !slices.Contains(kinds, kind) {
			kinds = append(kinds, kind)
		}
	}
	options := make([]JobSelectOption, 0, len(kinds))
	for _, kind := range kinds {
		options = append(options, JobSelectOption{Value: kind, Label: jobview.KindLabel(kind)})
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
	query = withAPIOwner(withoutEmptyValues(query))
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

// jobAPIQuery is the list's filter as the Job API reads it, for the summary
// panel and the summary export: the page position dropped, and the Owner
// select's choice spelled as the API reads it.
func jobAPIQuery(query url.Values) string {
	values := withAPIOwner(withoutEmptyValues(query))
	for _, position := range []string{"cursor", "before", "view"} {
		values.Del(position)
	}
	return values.Encode()
}

// jobSummaryExportFilter is the list's filter as a summary export seals it, or,
// when the export cannot seal it, why not in the words of the form. An export's
// filter runs later, possibly on an older worker, so it refuses the dimensions
// added after its Kind version; owner=me is one of them, and the viewer's own
// Jobs are the same question asked by the viewer's id, which it seals.
func jobSummaryExportFilter(query url.Values, filter jobs.Filter, viewer *auth.Principal) (string, string) {
	values := withAPIOwner(withoutEmptyValues(query))
	for _, position := range []string{"cursor", "before", "view"} {
		values.Del(position)
	}
	if values.Get("owner") == jobOwnerMineOption && viewer != nil && viewer.UserID != 0 {
		values.Del("owner")
		values.Set("ownerId", strconv.FormatUint(uint64(viewer.UserID), 10))
		filter.OwnedByViewer = false
	}
	if refused := application_context.SummaryExportUnsealableDimension(filter); refused != "" {
		if words, ok := jobSummaryExportDimensionWords[refused]; ok {
			refused = words
		}
		return "", "A summary export cannot filter by " + refused + ". Change the filter to export a summary."
	}
	return values.Encode(), ""
}

// jobSummaryExportDimensionWords names the dimensions an export refuses as the
// filter form does.
var jobSummaryExportDimensionWords = map[string]string{
	"inboundRelationship":   "Has been",
	"noInboundRelationship": "Has not been",
	"ownerDeleted":          "a deleted account as owner",
	"owner=me":              "your own jobs",
}

// withAPIOwner spells the Owner select's choice as the API reads it: an account
// id as ownerId and a deleted account as ownerDeleted=true, leaving "me", the
// API's own owner=me, as it is.
func withAPIOwner(values url.Values) url.Values {
	switch owner := values.Get("owner"); {
	case owner == jobOwnerDeletedOption:
		values.Del("owner")
		values.Set("ownerDeleted", "true")
	case owner != "" && owner != jobOwnerMineOption && values.Get("ownerId") == "":
		values.Del("owner")
		values.Set("ownerId", owner)
	}
	return values
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
		return refuseJobListFilter(err, ctx)
	}
	return addErrContext(err, ctx)
}

// refuseJobListFilter answers a filter the page cannot use, from a hand-edited
// address or a stale bookmark, with the Job Center itself: status 400, the
// filter form, and the problem in the list's place (listJobs.tpl), so the reader
// can correct or clear it. An error page would drop the form and offer only a
// way out of the Job Center.
func refuseJobListFilter(err error, ctx pongo2.Context) pongo2.Context {
	return ctx.Update(pongo2.Context{
		"jobListError":     jobListErrorText(err),
		"_statusCode":      http.StatusBadRequest,
		"_statusKeepsPage": true,
	})
}

// jobListErrorText is the problem as the page says it after "This filter cannot
// be used:": the API's message without the category the page already names.
func jobListErrorText(err error) string {
	message := jobview.RequestErrorMessage(err)
	for _, sentinel := range []error{jobs.ErrInvalidFilter, jobs.ErrInvalidCursor, jobs.ErrInvalidPage, jobs.ErrInvalidCommand} {
		message = strings.TrimPrefix(message, strings.TrimPrefix(sentinel.Error(), "jobs: ")+": ")
	}
	return message
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
		title = jobview.KindLabel(snapshot.Kind)
	}
	if title == "" {
		title = snapshot.ID
	}
	presentation := jobview.PresentJob(snapshot)
	now := time.Now()
	row := JobRow{
		ID: snapshot.ID, Title: title, Kind: snapshot.Kind, KindLabel: jobview.KindLabel(snapshot.Kind), State: string(snapshot.State),
		StateLabel: presentation.Label, BadgeClass: jobToneClass(presentation.Tone),
		Phase: jobview.PhaseText(snapshot), Pinned: snapshot.Pinned,
		Accepted: jobRowTime(&snapshot.AcceptedAt),
		Started:  jobRowTime(snapshot.StartedAt), Finished: jobRowTime(snapshot.FinishedAt), Version: snapshot.Version,
		Progress:             jobRowProgressAt(snapshot, now),
		ProgressSnapshotJSON: jobRowProgressSnapshotJSON(snapshot, now),
		DetailURL:            "/job?id=" + url.QueryEscape(snapshot.ID),
	}
	row.SummaryText, row.SummaryFields = jobSummaryPresentation(snapshot.Summary)
	if snapshot.ProgressUpdatedAt != nil {
		row.ProgressUpdatedAt = snapshot.ProgressUpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	if jobIsPartial(snapshot) {
		// The badge already says it; the phase beside it would say it twice.
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
	// The owner and the failure are for the page's own announcement of a change
	// (jobList.js announceChanges): whether the drawer says it, and why a Job
	// failed when the page does.
	payload := map[string]any{
		"id": snapshot.ID, "title": title, "kind": snapshot.Kind, "state": snapshot.State,
		"phase": snapshot.Phase, "version": snapshot.Version, "pinned": snapshot.Pinned, "dismissed": snapshot.Dismissed,
		"ownerUserId": snapshot.OwnerUserID,
	}
	if snapshot.Failure != nil {
		payload["failure"] = map[string]string{"code": snapshot.Failure.Code, "message": snapshot.Failure.Message}
	}
	entity, _ := json.Marshal(payload)
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

// jobToneClass is the class a state's tone takes on every Job surface
// (public/index.css): the card badge here, the drawer's pill and the Job page's.
func jobToneClass(tone string) string {
	return "job-tone--" + tone
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
		whole := json.NewDecoder(bytes.NewReader(trimmed))
		whole.UseNumber()
		var value any
		if err := whole.Decode(&value); err != nil {
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
// would otherwise keep an unknown total forever, and a rebuild would keep the
// phase it was in. Work nobody is doing — waiting, paused or stopped — with
// nothing to report draws no bar at all, and only work being done (the state
// table's Working) may pulse or read "Working". A phase alone is not something
// to report: the card shows it beside the kind.
//
// The bar's label says what the Job is doing; the amount it counted leads the
// stats line under it, formatted as every Job surface formats it.
func jobRowProgress(snapshot jobs.Snapshot) *JobRowProgress {
	return jobRowProgressAt(snapshot, time.Now())
}

func jobRowProgressAt(snapshot jobs.Snapshot, now time.Time) *JobRowProgress {
	out := jobRowProgressBar(snapshot)
	if out != nil {
		out.Stats = jobRowStats(snapshot, now)
	}
	return out
}

func jobRowProgressSnapshotJSON(snapshot jobs.Snapshot, now time.Time) string {
	if snapshot.State != jobs.StateRunning {
		return ""
	}
	rate := snapshot.LiveRate(now)
	eta, estimated := snapshot.ExpectedFinish(now)
	encoded, err := json.Marshal(jobRowProgressSnapshot{
		Phase: snapshot.Progress.Phase, Completed: snapshot.Progress.Completed, Total: snapshot.Progress.Total,
		Unit: snapshot.Progress.Unit, Message: snapshot.Progress.Message,
		Rate: rate, ETA: eta, ETAEstimated: estimated,
	})
	if err != nil {
		return ""
	}
	return string(encoded)
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
	if succeeded {
		return &JobRowProgress{Text: "Completed", Percent: 100, Known: true, AccessibleText: "Completed"}
	}

	out := &JobRowProgress{}
	switch {
	case progress.Message != "":
		out.Text = progress.Message
	case progress.Phase != "":
		out.Text = progress.Phase
	case working:
		out.Text = "Working"
	case progress.Completed == nil && len(progress.Metrics) > 0:
		// Work nobody is doing that reported only metrics names the first of
		// them rather than claiming to be working.
		out.Text = jobMetricSummary(progress.Metrics[0])
	}
	amount := formatJobAmount(progress, snapshot.State.Terminal())
	if progress.Completed != nil && progress.Total != nil && *progress.Total > 0 {
		percent := math.Round(float64(*progress.Completed) / float64(*progress.Total) * 100)
		out.Percent = int(math.Max(0, math.Min(100, percent)))
		out.Known = true
		out.AccessibleText = joinNonEmpty("; ", out.Text, amount)
		return out
	}
	out.Indeterminate = working
	if amount != "" {
		amount += " processed"
	}
	out.AccessibleText = joinNonEmpty("; ", out.Text, amount, "total unknown")
	return out
}

func joinNonEmpty(separator string, parts ...string) string {
	kept := parts[:0:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, separator)
}

// JobDetailReader is what the Job page reads before it renders, as the viewer:
// the Job, for the page's title and heading, and the Job a legacy id names.
type JobDetailReader interface {
	GetJob(jobID string) (jobs.Snapshot, error)
	JobIDForLegacyHandle(handle string) (string, error)
}

var _ JobDetailReader = (*application_context.MahresourcesContext)(nil)

// JobDetailContextProvider renders /job. The page reads the Job, its timeline
// and outputs itself and follows it live; the server reads it once more so the
// document title and the page's one heading name the Job, and so a Job that
// does not exist, or that the viewer may not see, is a 404 page with a way back.
func JobDetailContextProvider(ctx *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	var reader JobDetailReader
	if ctx != nil {
		reader = ctx
	}
	return jobDetailContextProvider(reader)
}

func jobDetailContextProvider(reader JobDetailReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		base := pongo2.Context{
			"pageTitle":               "Job",
			"hideSidebar":             true,
			"jobCenterCutoverEnabled": JobCenterCutoverEnabled,
		}.Update(StaticTemplateCtx(request))
		if reader == nil {
			return base
		}
		id := strings.TrimSpace(request.URL.Query().Get("id"))
		if id == "" {
			return addMessageErrContext("A job ID is required.", http.StatusBadRequest, base)
		}
		snapshot, err := reader.GetJob(id)
		if errors.Is(err, jobs.ErrNotFound) {
			// An old link or a script may still name a Job by the id the legacy
			// routes gave it; that id names the Job its lineage ends in now.
			canonical, handleErr := reader.JobIDForLegacyHandle(id)
			if handleErr == nil && canonical != "" && canonical != id {
				return pongo2.Context{"_redirect": "/job?id=" + url.QueryEscape(canonical)}
			}
			if handleErr == nil {
				return addMessageErrContext("That job doesn't exist, or it has been deleted.", http.StatusNotFound, base)
			}
			err = handleErr
		}
		if err != nil {
			// A read that failed says nothing about the Job: the page reads it
			// itself, and offers Try again if that read fails too.
			return base
		}
		heading := jobHeading(snapshot)
		base["pageTitle"] = jobDocumentTitle(heading, jobStateLabel(snapshot), jobs.ShortID(snapshot.ID))
		base["headingTitle"] = heading
		return base
	}
}

// jobHeading names a Job as every surface does: its title, else its Kind, else
// its id.
func jobHeading(snapshot jobs.Snapshot) string {
	for _, name := range []string{snapshot.Title, snapshot.Kind, snapshot.ID} {
		if trimmed := strings.TrimSpace(name); trimmed != "" {
			return trimmed
		}
	}
	return "Job"
}

// jobDocumentTitle is the Job page's title before the site name: the Job, its
// state and the end of its id, so two Job tabs, the history, and the page a
// Retry opens each say which Job they are, attempts of one download included.
// src/components/jobCenter.js jobDocumentTitle keeps it current as the state
// changes.
func jobDocumentTitle(heading, stateLabel, shortID string) string {
	title := fmt.Sprintf("%s (%s) - Job", heading, stateLabel)
	if shortID != "" {
		title += " " + shortID
	}
	return title
}
