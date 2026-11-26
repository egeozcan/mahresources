package shortcodes

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"mahresources/mrql"
)

const (
	// calendarListMax is the most entities a month lists by title. The list
	// query asks for one more (calendarListProbe) to learn whether the month
	// holds more than that without counting it.
	calendarListMax   = 100
	calendarListProbe = calendarListMax + 1
	// calendarItemsPerDay is how many titles a day cell shows before "+k more".
	calendarItemsPerDay = 3
	// calendarMaxRows caps the per-day count query of a dense month. The rows
	// come back ordered by date, so the cap always drops the latest values.
	calendarMaxRows = 1000
)

// calendarNow is the clock "current" month and the today highlight read. A
// test seam; the server's local clock is the rule (§4.4 of the plan).
var calendarNow = time.Now

var (
	calendarMetaPath     = regexp.MustCompile(`^meta\.[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
	calendarMonthPattern = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})$`)
)

const calendarMonthProblem = `[calendar] month must be "current" or YYYY-MM`

// calendarQueries is the MRQL one [calendar] runs for one month. List and
// Count share the filter and the month window; Day is the link target of a
// single day and carries the same filter and guard, so a day link lists
// exactly what its cell counted.
type calendarQueries struct {
	List  string                 // flat list of the month's entities, by date then ID
	Count string                 // per-day counts, for a month too dense to list
	Day   func(time.Time) string // one day's entities
	Field string                 // "created", "updated" or "meta.<path>"
	// KeyColumn is the row key Count's rows carry the date under.
	KeyColumn string
	meta      bool
}

// parseCalendarMonth reads month=: blank or "current" is the month of
// calendarNow on the server's local clock, otherwise YYYY-MM with a year from
// 1000 to 9999. It returns the first day of that month at local midnight.
func parseCalendarMonth(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "current" {
		now := calendarNow().In(time.Local)
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local), true
	}
	m := calendarMonthPattern.FindStringSubmatch(raw)
	if m == nil {
		return time.Time{}, false
	}
	year, _ := strconv.Atoi(m[1])
	month, _ := strconv.Atoi(m[2])
	if year < 1000 || month < 1 || month > 12 {
		return time.Time{}, false
	}
	return time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.Local), true
}

// calendarFilterProblem checks the shape of an already parsed filter. The
// renderer and the linter share it, so they say the same thing.
func calendarFilterProblem(q *mrql.Query) string {
	if q.GroupBy != nil || len(q.OrderBy) > 0 || q.Limit >= 0 || q.Offset >= 0 || q.Scope != nil {
		return "[calendar] mrql must be a filter only (no GROUP BY, ORDER BY, LIMIT, OFFSET or SCOPE)"
	}
	if len(mrql.ListParams(q)) > 0 {
		return "[calendar] mrql cannot use $parameters"
	}
	if mrql.ExtractEntityType(q) == mrql.EntityUnspecified {
		return `[calendar] mrql must name one entity type (type = "group", "resource" or "note")`
	}
	return ""
}

// calendarDateField validates date=: blank means created.
func calendarDateField(date string) (string, bool) {
	date = strings.TrimSpace(date)
	switch {
	case date == "":
		return "created", true
	case date == "created" || date == "updated":
		return date, true
	case calendarMetaPath.MatchString(date):
		return date, true
	}
	return "", false
}

const calendarDateProblem = "[calendar] date must be created, updated or meta.<path>"

// composeCalendarQueries builds the queries for the month starting at first.
//
// The filter is wrapped in parentheses, which the parser drops (parsePrimary
// returns the inner node), so ExtractEntityType still sees its top-level
// type = and the AND chain cannot rebind an OR inside the filter. A meta date
// also carries the guard F ~ "????-??-??*": a number, a boolean or a string
// that does not start like YYYY-MM-DD matches nothing on either dialect, so
// the counts, the titles and the day links agree about which values count.
func composeCalendarQueries(filter, date string, first time.Time) (calendarQueries, error) {
	q, err := mrql.Parse(filter)
	if err != nil {
		return calendarQueries{}, fmt.Errorf("[calendar] %w", err)
	}
	if problem := calendarFilterProblem(q); problem != "" {
		return calendarQueries{}, errors.New(problem)
	}
	field, ok := calendarDateField(date)
	if !ok {
		return calendarQueries{}, errors.New(calendarDateProblem)
	}

	isMeta := strings.HasPrefix(field, "meta.")
	guard := ""
	if isMeta {
		guard = " AND " + field + ` ~ "????-??-??*"`
	}
	window := func(from, to time.Time) string {
		// A bound in year 10000 would not compare as a date: "10000-01-01"
		// sorts before "9999-…". A Meta string is capped just past the last
		// day instead, which also keeps out date-shaped text such as
		// "9999-13-01"; a timestamp cannot pass 9999-12-31, so it needs none.
		upper := ` AND ` + field + ` < "` + to.Format("2006-01-02") + `"`
		if to.Year() > 9999 {
			upper = ""
			if isMeta {
				upper = ` AND ` + field + ` < "9999-12-32"`
			}
		}
		return "(" + filter + ") AND " + field + ` >= "` + from.Format("2006-01-02") + `"` + upper + guard
	}
	next := first.AddDate(0, 1, 0)
	month := window(first, next)

	out := calendarQueries{
		List:  month + " ORDER BY " + field + " ASC",
		Field: field,
		Day: func(d time.Time) string {
			return window(d, d.AddDate(0, 0, 1))
		},
		meta: isMeta,
	}
	if isMeta {
		out.KeyColumn = field
		out.Count = month + " GROUP BY " + field + " COUNT() ORDER BY " + field + " ASC"
	} else {
		out.KeyColumn = field + ".day"
		out.Count = month + " GROUP BY " + field + ".day COUNT() ORDER BY " + field + ".day ASC"
	}
	return out, nil
}

// calendarDay is one day of the shown month.
type calendarDay struct {
	count int
	items []QueryResultItem // up to calendarItemsPerDay, in list order
}

// calendarItemDay places a listed entity on a day: the local date of its
// timestamp for created/updated, and the date as written for a meta value.
// ok is false for a value that names no date.
func calendarItemDay(item QueryResultItem, q calendarQueries) (string, bool) {
	if q.meta {
		if len(item.Meta) == 0 {
			return "", false
		}
		var decoded any
		if err := json.Unmarshal(item.Meta, &decoded); err != nil {
			return "", false
		}
		v, found := navigateJSONValue(decoded, strings.TrimPrefix(q.Field, "meta."))
		s, isString := v.(string)
		if !found || !isString {
			return "", false
		}
		return calendarDateKey(s)
	}
	prop := "CreatedAt"
	if q.Field == "updated" {
		prop = "UpdatedAt"
	}
	v, ok := traversePropertyPath(item.Entity, prop, false)
	if !ok {
		return "", false
	}
	t, isTime := v.Interface().(time.Time)
	if !isTime || t.IsZero() {
		return "", false
	}
	return t.In(time.Local).Format("2006-01-02"), true
}

// calendarDateKey is the YYYY-MM-DD a value starts with, when it starts with
// a real date.
func calendarDateKey(s string) (string, bool) {
	if len(s) < 10 {
		return "", false
	}
	if _, err := time.Parse("2006-01-02", s[:10]); err != nil {
		return "", false
	}
	return s[:10], true
}

// calendarRowKey and calendarRowCount read an aggregated row's columns in the
// shapes the two dialects leave them in.
func calendarRowKey(v any) (string, bool) {
	switch x := v.(type) {
	case string:
		return x, true
	case []byte:
		return string(x), true
	}
	return "", false
}

func calendarRowCount(v any) (int, bool) {
	switch x := v.(type) {
	case int64:
		return int(x), true
	case int:
		return x, true
	case int32:
		return int(x), true
	case float64:
		return int(x), true
	case []byte:
		n, err := strconv.Atoi(string(x))
		return n, err == nil
	case string:
		n, err := strconv.Atoi(x)
		return n, err == nil
	}
	return 0, false
}

// RenderCalendarShortcode renders [calendar]: one month as a grid, Monday
// first, with each day's count, up to three linked titles and a link to the
// day's full list, and Prev/Next buttons that swap the month in place.
//
// Every count and title comes from the executor, so the principal's subtree
// confinement, the page query budget and its cache apply as they do to
// [mrql]. A month of up to 100 entities costs one query and its titles and
// counts come from the same rows; a denser month adds a per-day count query
// and shows counts only.
func RenderCalendarShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, executor QueryExecutor) string {
	// A <button> takes phrasing content only; a table is not.
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("calendar", "[calendar] cannot be used inside a [reload] button face")
	}
	filter := sc.Attrs["mrql"]
	if strings.TrimSpace(filter) == "" {
		return shortcodeErrorMarker("calendar", "[calendar] requires mrql=")
	}
	first, ok := parseCalendarMonth(sc.Attrs["month"])
	if !ok {
		return shortcodeErrorMarker("calendar", calendarMonthProblem)
	}
	queries, err := composeCalendarQueries(filter, sc.Attrs["date"], first)
	if err != nil {
		return shortcodeErrorMarker("calendar", err.Error())
	}
	scope := resolveScopeKeyword(sc.Attrs["scope"], ctx)

	m := meterFrom(reqCtx)
	list, err := executor(reqCtx, queries.List, QueryOptions{Limit: calendarListProbe, ScopeGroupID: scope})
	if err != nil {
		if errors.Is(err, ErrQueryUnavailable) {
			return unavailableComment(err)
		}
		return mrqlErrorHTML(err, false)
	}
	if !m.alive() {
		return ""
	}

	daysIn := first.AddDate(0, 1, -1).Day()
	days := make([]calendarDay, daysIn+1) // 1-based
	monthPrefix := first.Format("2006-01-")
	dayOf := func(key string) int {
		if !strings.HasPrefix(key, monthPrefix) {
			return 0
		}
		d, err := strconv.Atoi(key[len(monthPrefix):])
		if err != nil || d < 1 || d > daysIn {
			return 0
		}
		return d
	}

	countsMode, capped := false, false
	var items []QueryResultItem
	if list != nil {
		items = list.Items
	}
	if len(items) > calendarListMax {
		countsMode = true
		agg, err := executor(reqCtx, queries.Count, QueryOptions{Limit: calendarMaxRows, ScopeGroupID: scope})
		if err != nil {
			if errors.Is(err, ErrQueryUnavailable) {
				return unavailableComment(err)
			}
			return mrqlErrorHTML(err, false)
		}
		if !m.alive() {
			return ""
		}
		if agg != nil {
			capped = len(agg.Rows) >= calendarMaxRows
			for _, row := range agg.Rows {
				raw, ok := calendarRowKey(row[queries.KeyColumn])
				if !ok {
					continue
				}
				key, ok := calendarDateKey(raw)
				if !ok {
					continue
				}
				n, ok := calendarRowCount(row["count"])
				if d := dayOf(key); ok && d > 0 && n > 0 {
					days[d].count += n
				}
			}
		}
	} else {
		for _, item := range items {
			key, ok := calendarItemDay(item, queries)
			if !ok {
				continue
			}
			d := dayOf(key)
			if d == 0 {
				continue
			}
			days[d].count++
			if len(days[d].items) < calendarItemsPerDay {
				days[d].items = append(days[d].items, item)
			}
		}
	}

	// Day links reproduce the scope the grid was counted in. An entity with no
	// owner resolves to the sentinel, which matches nothing: no links then.
	links := scope != mrql.UnresolvedScopeSentinel
	dayHref := func(d int) string {
		q := queries.Day(time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, time.Local))
		if scope != 0 {
			q = mrql.InsertScopeClause(q, scope)
		}
		return "/mrql?q=" + url.QueryEscape(q)
	}

	prevToken, nextToken := calendarNavTokens(reqCtx, sc, ctx, first)
	return buildCalendarHTML(calendarView{
		first:      first,
		days:       days,
		countsMode: countsMode,
		capped:     capped,
		links:      links,
		dayHref:    dayHref,
		prevToken:  prevToken,
		nextToken:  nextToken,
	})
}

// calendarNavTokens seals the shortcode, re-serialized with the adjacent
// month, for the Prev and Next buttons. Without a refresh signer, or for a
// carrier context the deferred endpoint cannot load, there is no navigation.
func calendarNavTokens(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, first time.Time) (prev, next string) {
	signer := refreshSignerFrom(reqCtx)
	if signer == nil || !isDeferrableEntity(ctx) {
		return "", ""
	}
	seal := func(month time.Time) string {
		body, ok := serializeCalendarShortcode(sc.Attrs, month.Format("2006-01"))
		if !ok {
			return ""
		}
		return signer(ctx.EntityType, ctx.EntityID, body, ctx.UntrustedSource)
	}
	if !(first.Year() == 1000 && first.Month() == time.January) {
		prev = seal(first.AddDate(0, -1, 0))
	}
	if !(first.Year() == 9999 && first.Month() == time.December) {
		next = seal(first.AddDate(0, 1, 0))
	}
	return prev, next
}

// serializeCalendarShortcode writes [calendar …] back out with month set, the
// author's other attributes kept, in sorted key order. A value holding both
// quote characters cannot be written back; ok is false then.
func serializeCalendarShortcode(attrs map[string]string, month string) (string, bool) {
	merged := make(map[string]string, len(attrs)+1)
	for k, v := range attrs {
		merged[k] = v
	}
	merged["month"] = month
	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString("[calendar")
	for _, k := range keys {
		v := merged[k]
		quote := `"`
		if strings.Contains(v, `"`) {
			if strings.Contains(v, "'") {
				return "", false
			}
			quote = "'"
		}
		// The deferred render unescapes the attributes again, so "&" is
		// written as "&amp;" for the value to survive the round trip.
		b.WriteString(" " + k + "=" + quote + strings.ReplaceAll(v, "&", "&amp;") + quote)
	}
	b.WriteString("]")
	return b.String(), true
}

type calendarView struct {
	first                time.Time
	days                 []calendarDay
	countsMode, capped   bool
	links                bool
	dayHref              func(int) string
	prevToken, nextToken string
}

var calendarWeekdays = [7]string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}

// calendarItems is "1 item", "N items" or "no items".
func calendarItems(n int) string {
	switch n {
	case 0:
		return "no items"
	case 1:
		return "1 item"
	}
	return strconv.Itoa(n) + " items"
}

func monthLabel(t time.Time) string {
	return t.Month().String() + " " + strconv.Itoa(t.Year())
}

func buildCalendarHTML(v calendarView) string {
	id := "cal-" + rand.Text()[:12]
	capID := id + "-cap"
	month := v.first.Month().String()
	daysIn := len(v.days) - 1

	total := 0
	for _, d := range v.days {
		total += d.count
	}

	today := 0
	if now := calendarNow().In(time.Local); now.Year() == v.first.Year() && now.Month() == v.first.Month() {
		today = now.Day()
	}

	var b strings.Builder
	b.WriteString(`<calendar-shortcode class="calendar-shortcode" role="group" aria-labelledby="` + capID + `"`)
	if v.prevToken != "" {
		b.WriteString(` data-prev-token="` + html.EscapeString(v.prevToken) + `"`)
	}
	if v.nextToken != "" {
		b.WriteString(` data-next-token="` + html.EscapeString(v.nextToken) + `"`)
	}
	b.WriteString(`>`)

	if v.prevToken != "" || v.nextToken != "" {
		b.WriteString(`<div class="calendar-shortcode-nav">`)
		if v.prevToken != "" {
			b.WriteString(`<button type="button" class="calendar-shortcode-navbtn" data-calendar-nav="prev"><span aria-hidden="true">‹</span> <span class="sr-only">Previous month, </span>` + monthLabel(v.first.AddDate(0, -1, 0)) + `</button>`)
		}
		if v.nextToken != "" {
			b.WriteString(`<button type="button" class="calendar-shortcode-navbtn" data-calendar-nav="next"><span class="sr-only">Next month, </span>` + monthLabel(v.first.AddDate(0, 1, 0)) + ` <span aria-hidden="true">›</span></button>`)
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<table class="calendar-shortcode-grid"><caption id="` + capID + `">` + monthLabel(v.first) +
		` <span class="calendar-shortcode-total">(` + calendarItems(total) + `)</span></caption><thead><tr>`)
	for _, wd := range calendarWeekdays {
		b.WriteString(`<th scope="col"><span aria-hidden="true">` + wd[:3] + `</span><span class="sr-only">` + wd + `</span></th>`)
	}
	b.WriteString(`</tr></thead><tbody><tr>`)

	lead := (int(v.first.Weekday()) + 6) % 7 // Monday first
	col := 0
	for ; col < lead; col++ {
		b.WriteString(`<td class="calendar-shortcode-pad"></td>`)
	}
	for d := 1; d <= daysIn; d++ {
		if col == 7 {
			b.WriteString(`</tr><tr>`)
			col = 0
		}
		writeCalendarDay(&b, v, month, d, d == today)
		col++
	}
	for ; col < 7; col++ {
		b.WriteString(`<td class="calendar-shortcode-pad"></td>`)
	}
	b.WriteString(`</tr></tbody></table>`)

	if v.countsMode {
		b.WriteString(`<p class="calendar-shortcode-note">Showing counts only: more than 100 items this month.</p>`)
		if v.capped {
			b.WriteString(`<p class="calendar-shortcode-note">Counts may be incomplete: more than 1000 distinct date values this month.</p>`)
		}
	}
	b.WriteString(`</calendar-shortcode>`)
	return b.String()
}

func writeCalendarDay(b *strings.Builder, v calendarView, month string, d int, isToday bool) {
	day := v.days[d]
	cls := "calendar-shortcode-cell"
	current := ""
	if isToday {
		cls += " calendar-shortcode-today"
		current = ` aria-current="date"`
	}
	ds := strconv.Itoa(d)
	b.WriteString(`<td class="` + cls + `">`)

	if day.count == 0 || !v.links {
		b.WriteString(`<span class="calendar-shortcode-day" aria-hidden="true">` + ds + `</span>`)
		if day.count > 0 {
			b.WriteString(`<span class="calendar-shortcode-count" aria-hidden="true">` + strconv.Itoa(day.count) + `</span>`)
		}
		b.WriteString(`<span class="sr-only"` + current + `>` + month + " " + ds + ", " + calendarItems(day.count) + `</span>`)
	} else {
		// The name is one sr-only sentence and the visible digits are hidden
		// from assistive technology. Interleaving visible and sr-only pieces
		// reads "March 4 , 5 items" in Chromium, which puts a space beside an
		// absolutely positioned (sr-only) span. Both visible numbers are in the
		// name, in the order they are shown.
		b.WriteString(`<a class="calendar-shortcode-daylink" href="` + html.EscapeString(v.dayHref(d)) + `"` + current +
			`><span class="calendar-shortcode-day" aria-hidden="true">` + ds + `</span><span class="calendar-shortcode-count" aria-hidden="true">` +
			strconv.Itoa(day.count) + `</span><span class="sr-only">` + month + " " + ds + ", " + calendarItems(day.count) + `</span></a>`)
	}

	if len(day.items) > 0 && !v.countsMode {
		b.WriteString(`<ul class="calendar-shortcode-items">`)
		for _, item := range day.items {
			name := html.EscapeString(extractEntityName(item))
			if v.links {
				fmt.Fprintf(b, `<li><a href="/%s?id=%d">%s</a></li>`, html.EscapeString(item.EntityType), item.EntityID, name)
			} else {
				b.WriteString(`<li>` + name + `</li>`)
			}
		}
		b.WriteString(`</ul>`)
		if more := day.count - len(day.items); more > 0 && v.links {
			unit := " items on "
			if more == 1 {
				unit = " item on "
			}
			b.WriteString(`<a class="calendar-shortcode-more" href="` + html.EscapeString(v.dayHref(d)) + `">+` + strconv.Itoa(more) +
				` more<span class="sr-only">` + unit + month + " " + ds + `</span></a>`)
		}
	}
	b.WriteString(`</td>`)
}

// lintCalendar reports the renderer's own refusals, word for word. A filter
// that does not parse is left to the MRQL pass (calendarLintQueries), which
// reports the parse error at mrql=.
func lintCalendar(tk token, reloadSpans [][2]int, add func(start, end int, severity, msg string)) {
	if insideSpans(tk.start, reloadSpans) {
		add(tk.start, tk.end, SeverityError, "[calendar] cannot be used inside a [reload] button face")
	}
	if _, ok := parseCalendarMonth(tk.attrs["month"]); !ok {
		add(tk.start, tk.end, SeverityError, calendarMonthProblem)
	}
	if _, ok := calendarDateField(tk.attrs["date"]); !ok {
		add(tk.start, tk.end, SeverityError, calendarDateProblem)
	}
	filter := tk.attrs["mrql"]
	if strings.TrimSpace(filter) == "" {
		return // the required-attribute check reports it
	}
	if q, err := mrql.Parse(filter); err == nil {
		if problem := calendarFilterProblem(q); problem != "" {
			add(tk.start, tk.end, SeverityError, problem)
		}
	}
}

type calendarLintQuery struct {
	query string
	opts  MRQLCheckOptions
}

// calendarLintQueries is what the MRQL pass validates for a [calendar]: the
// list and count queries the renderer runs, with its limits, for the authored
// month (calendarNow's when unset). A filter that does not parse is validated
// on its own, so its parse error is reported once. A calendar the renderer
// refuses for another reason runs no query, and lintCalendar has already said
// why.
func calendarLintQueries(attrs map[string]string) []calendarLintQuery {
	filter := attrs["mrql"]
	if strings.TrimSpace(filter) == "" {
		return nil
	}
	if _, err := mrql.Parse(filter); err != nil {
		return []calendarLintQuery{{filter, MRQLCheckOptions{Limit: calendarListProbe}}}
	}
	first, ok := parseCalendarMonth(attrs["month"])
	if !ok {
		return nil
	}
	q, err := composeCalendarQueries(filter, attrs["date"], first)
	if err != nil {
		return nil
	}
	return []calendarLintQuery{
		{q.List, MRQLCheckOptions{Limit: calendarListProbe}},
		{q.Count, MRQLCheckOptions{Limit: calendarMaxRows}},
	}
}
