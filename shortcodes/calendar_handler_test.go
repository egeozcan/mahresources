package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/mrql"
)

// pinCalendarClock fixes calendarNow and time.Local for one test.
func pinCalendarClock(t *testing.T, now time.Time, loc *time.Location) {
	t.Helper()
	prevNow, prevLocal := calendarNow, time.Local
	calendarNow = func() time.Time { return now }
	time.Local = loc
	t.Cleanup(func() { calendarNow, time.Local = prevNow, prevLocal })
}

// fakeCalendarExecutor answers the list query with list and the count query
// with agg, and records every call.
type fakeCalendarExecutor struct {
	list  *QueryResult
	agg   *QueryResult
	err   error
	calls []usingCall
}

func (f *fakeCalendarExecutor) exec(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
	f.calls = append(f.calls, usingCall{query, opts})
	if f.err != nil {
		return nil, f.err
	}
	if strings.Contains(query, "GROUP BY") {
		return f.agg, nil
	}
	return f.list, nil
}

func calNote(id uint, name string, created time.Time, meta string) QueryResultItem {
	return QueryResultItem{
		EntityType: "note",
		EntityID:   id,
		Entity:     &models.Note{ID: id, Name: name, CreatedAt: created, UpdatedAt: created},
		Meta:       []byte(meta),
	}
}

func calendarCtx() MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "group", EntityID: 7, ScopeGroupID: 7, ParentGroupID: 3, RootGroupID: 1}
}

func renderCalendar(t *testing.T, reqCtx context.Context, attrs map[string]string, f *fakeCalendarExecutor) string {
	t.Helper()
	if reqCtx == nil {
		reqCtx = context.Background()
	}
	return RenderCalendarShortcode(reqCtx, Shortcode{Name: "calendar", Attrs: attrs}, calendarCtx(), f.exec)
}

var (
	ariaHiddenSpan = regexp.MustCompile(`<span aria-hidden="true">[^<]*</span>|<span class="[^"]*" aria-hidden="true">[^<]*</span>`)
	anyTag         = regexp.MustCompile(`<[^>]*>`)
)

// accessibleText is the text a screen reader reads from a fragment of flat
// markup: aria-hidden spans dropped, sr-only spans kept.
func accessibleText(fragment string) string {
	return strings.TrimSpace(anyTag.ReplaceAllString(ariaHiddenSpan.ReplaceAllString(fragment, ""), ""))
}

// cellFor returns the <td> of day d in rendered calendar output.
func cellFor(t *testing.T, out, month string, d int) string {
	t.Helper()
	cells := regexp.MustCompile(`<td class="calendar-shortcode-cell[^"]*">.*?</td>`).FindAllString(out, -1)
	for _, c := range cells {
		if strings.Contains(c, fmt.Sprintf(`>%s %d, `, month, d)) {
			return c
		}
	}
	t.Fatalf("no cell for %s %d", month, d)
	return ""
}

func TestCalendarComposeQueries(t *testing.T) {
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	q, err := composeCalendarQueries(`type = "note"`, "", march)
	require.NoError(t, err)
	assert.Equal(t, `(type = "note") AND created >= "2026-03-01" AND created < "2026-04-01" ORDER BY created ASC`, q.List)
	assert.Equal(t, `(type = "note") AND created >= "2026-03-01" AND created < "2026-04-01" GROUP BY created.day COUNT() ORDER BY created.day ASC`, q.Count)
	assert.Equal(t, `(type = "note") AND created >= "2026-03-04" AND created < "2026-03-05"`, q.Day(time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "created.day", q.KeyColumn)

	q, err = composeCalendarQueries(`type = "group"`, "updated", march)
	require.NoError(t, err)
	assert.Equal(t, `(type = "group") AND updated >= "2026-03-01" AND updated < "2026-04-01" ORDER BY updated ASC`, q.List)
	assert.Equal(t, `(type = "group") AND updated >= "2026-03-01" AND updated < "2026-04-01" GROUP BY updated.day COUNT() ORDER BY updated.day ASC`, q.Count)

	q, err = composeCalendarQueries(`type = "note"`, "meta.due", march)
	require.NoError(t, err)
	guard := ` AND meta.due ~ "????-??-??*"`
	assert.Equal(t, `(type = "note") AND meta.due >= "2026-03-01" AND meta.due < "2026-04-01"`+guard+` ORDER BY meta.due ASC`, q.List)
	assert.Equal(t, `(type = "note") AND meta.due >= "2026-03-01" AND meta.due < "2026-04-01"`+guard+` GROUP BY meta.due COUNT() ORDER BY meta.due ASC`, q.Count)
	assert.Equal(t, `(type = "note") AND meta.due >= "2026-03-31" AND meta.due < "2026-04-01"`+guard, q.Day(time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, "meta.due", q.KeyColumn)

	q, err = composeCalendarQueries(`type = "note"`, "", time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Contains(t, q.List, `created >= "2026-12-01" AND created < "2027-01-01"`)
	assert.Equal(t, `(type = "note") AND created >= "2026-12-31" AND created < "2027-01-01"`, q.Day(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)))

	// December 9999 cannot be bounded by "10000-01-01", which sorts before
	// every 9999 date: a Meta string is capped past the last day instead.
	q, err = composeCalendarQueries(`type = "note"`, "meta.due", time.Date(9999, 12, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, `(type = "note") AND meta.due >= "9999-12-01" AND meta.due < "9999-12-32"`+guard+` ORDER BY meta.due ASC`, q.List)
	assert.Equal(t, `(type = "note") AND meta.due >= "9999-12-31" AND meta.due < "9999-12-32"`+guard, q.Day(time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)))
	assert.Equal(t, `(type = "note") AND meta.due >= "9999-12-30" AND meta.due < "9999-12-31"`+guard, q.Day(time.Date(9999, 12, 30, 0, 0, 0, 0, time.UTC)))
	q, err = composeCalendarQueries(`type = "note"`, "", time.Date(9999, 12, 1, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	assert.Equal(t, `(type = "note") AND created >= "9999-12-01" ORDER BY created ASC`, q.List)
}

// The parentheses are transparent to ExtractEntityType, and an OR inside the
// filter stays inside them.
func TestCalendarComposedQueriesKeepTheEntityType(t *testing.T) {
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for _, filter := range []string{`type = "note"`, `type = "resource" AND name ~ "x"`, `type = group AND (name = "a" OR name = "b")`} {
		want := mrql.ExtractEntityType(mustParseMRQL(t, filter))
		for _, date := range []string{"created", "meta.due"} {
			q, err := composeCalendarQueries(filter, date, march)
			require.NoError(t, err)
			for _, text := range []string{q.List, q.Count, q.Day(march)} {
				parsed := mustParseMRQL(t, text)
				assert.Equal(t, want, mrql.ExtractEntityType(parsed), text)
			}
		}
	}
}

func mustParseMRQL(t *testing.T, s string) *mrql.Query {
	t.Helper()
	q, err := mrql.Parse(s)
	require.NoError(t, err, s)
	return q
}

func TestCalendarComposeRejections(t *testing.T) {
	march := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	clause := "[calendar] mrql must be a filter only (no GROUP BY, ORDER BY, LIMIT, OFFSET or SCOPE)"
	cases := map[string]string{
		`type = "note" AND name = $x`:         "[calendar] mrql cannot use $parameters",
		`type = "note" GROUP BY name COUNT()`: clause,
		`type = "note" ORDER BY name ASC`:     clause,
		`type = "note" LIMIT 5`:               clause,
		`type = "note" LIMIT 5 OFFSET 2`:      clause,
		`type = "note" SCOPE 3`:               clause,
		`type = "note" OR type = "group"`:     `[calendar] mrql must name one entity type (type = "group", "resource" or "note")`,
		`name = "x"`:                          `[calendar] mrql must name one entity type (type = "group", "resource" or "note")`,
		`NOT type = "note"`:                   `[calendar] mrql must name one entity type (type = "group", "resource" or "note")`,
	}
	for filter, want := range cases {
		_, err := composeCalendarQueries(filter, "created", march)
		require.Error(t, err, filter)
		assert.Equal(t, want, err.Error(), filter)
	}
	_, err := composeCalendarQueries(`type = "note" AND (`, "created", march)
	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "[calendar] "), err.Error())

	for _, date := range []string{"name", "meta", "meta.", "meta.1x", "meta.a..b", "meta.a-b", "created.day", "CREATED"} {
		_, err := composeCalendarQueries(`type = "note"`, date, march)
		require.Error(t, err, date)
		assert.Equal(t, calendarDateProblem, err.Error(), date)
	}
	for _, date := range []string{"meta.due", "meta.event.start_date", "meta._x"} {
		_, err := composeCalendarQueries(`type = "note"`, date, march)
		assert.NoError(t, err, date)
	}
}

func TestCalendarParseMonth(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), time.UTC)
	for in, want := range map[string]string{"": "2026-03", "current": "2026-03", " 2026-11 ": "2026-11", "1000-01": "1000-01", "9999-12": "9999-12"} {
		got, ok := parseCalendarMonth(in)
		require.True(t, ok, in)
		assert.Equal(t, want, got.Format("2006-01"), in)
	}
	for _, in := range []string{"2026-13", "2026-00", "0999-12", "2026-3", "26-03", "march", "2026-03-01", "+2026-03"} {
		_, ok := parseCalendarMonth(in)
		assert.False(t, ok, in)
	}
}

func TestCalendarOptionsAndScope(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), time.UTC)
	for scope, want := range map[string]uint{"global": 0, "parent": 3, "root": 1, "42": 42, "entity": 7, "": 7} {
		f := &fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}
		attrs := map[string]string{"mrql": `type = "note"`}
		if scope != "" {
			attrs["scope"] = scope
		}
		renderCalendar(t, nil, attrs, f)
		require.Len(t, f.calls, 1, scope)
		assert.Equal(t, QueryOptions{Limit: 101, ScopeGroupID: want}, f.calls[0].opts, scope)
	}
}

func TestCalendarItemsModeIsOneQuery(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), time.UTC)
	var items []QueryResultItem
	for i := 0; i < 100; i++ {
		items = append(items, calNote(uint(i+1), fmt.Sprintf("n%d", i), time.Date(2026, 3, 1+i%31, 9, 0, 0, 0, time.UTC), ""))
	}
	f := &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}}
	out := renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`}, f)
	require.Len(t, f.calls, 1)
	assert.Contains(t, out, "(100 items)")
	assert.NotContains(t, out, "calendar-shortcode-note")
	// Day 1 holds items 0, 31, 62, 93: three titles and "+1 more".
	assert.Equal(t, "March 1, 4 items", accessibleText(regexp.MustCompile(`<a class="calendar-shortcode-daylink"[^>]*>.*?</a>`).FindString(cellFor(t, out, "March", 1))))
	assert.Contains(t, cellFor(t, out, "March", 1), ">+1 more<span class=\"sr-only\"> item on March 1</span></a>")
}

func TestCalendarCountsMode(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC), time.UTC)
	items := make([]QueryResultItem, 101)
	for i := range items {
		items[i] = calNote(uint(i+1), "n", time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC), "")
	}
	agg := &QueryResult{Mode: "aggregated", Rows: []map[string]any{
		{"created.day": "2026-03-02", "count": int64(60)},
		{"created.day": []byte("2026-03-03"), "count": float64(30)},
		{"created.day": "2026-03-04", "count": []byte("7")},
		{"created.day": "2026-03-05", "count": "4"},
		{"created.day": "2026-04-01", "count": int64(99)}, // outside the month
		{"created.day": "not a date", "count": int64(5)},
		{"created.day": nil, "count": int64(5)},
	}}
	f := &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}, agg: agg}
	out := renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`}, f)
	require.Len(t, f.calls, 2)
	assert.Equal(t, QueryOptions{Limit: 1000, ScopeGroupID: 7}, f.calls[1].opts)
	assert.Contains(t, f.calls[1].query, "GROUP BY created.day COUNT()")
	assert.Contains(t, out, "Showing counts only: more than 100 items this month.")
	assert.NotContains(t, out, "Counts may be incomplete")
	assert.NotContains(t, out, "calendar-shortcode-items", "counts mode lists no titles")
	assert.Contains(t, out, "(101 items)")
	for d, want := range map[int]string{2: "March 2, 60 items", 3: "March 3, 30 items", 4: "March 4, 7 items", 5: "March 5, 4 items", 6: "March 6, no items"} {
		assert.Equal(t, want, accessibleText(cellFor(t, out, "March", d)), d)
	}

	// Raw meta keys fold by their date; the cap note shows at 1000 rows.
	rows := make([]map[string]any, 1000)
	for i := range rows {
		rows[i] = map[string]any{"meta.due": "2026-03-04T0" + fmt.Sprint(i%10) + ":00:00Z", "count": int64(1)}
	}
	rows[0] = map[string]any{"meta.due": "2026-03-09", "count": int64(1)}
	f = &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}, agg: &QueryResult{Mode: "aggregated", Rows: rows}}
	out = renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "date": "meta.due"}, f)
	assert.Contains(t, out, "Counts may be incomplete: more than 1000 distinct date values this month.")
	assert.Equal(t, "March 4, 999 items", accessibleText(cellFor(t, out, "March", 4)))
	assert.Equal(t, "March 9, 1 item", accessibleText(cellFor(t, out, "March", 9)))
}

func TestCalendarPlacement(t *testing.T) {
	berlin := time.FixedZone("UTC+1", 3600)
	pinCalendarClock(t, time.Date(2026, 3, 15, 12, 0, 0, 0, berlin), berlin)

	// 23:30 UTC on March 4 is 00:30 on March 5 local; 23:30 local on March 4
	// is 22:30 UTC on March 4.
	items := []QueryResultItem{
		calNote(1, "late utc", time.Date(2026, 3, 4, 23, 30, 0, 0, time.UTC), ""),
		calNote(2, "late local", time.Date(2026, 3, 4, 23, 30, 0, 0, berlin), ""),
		calNote(3, "february", time.Date(2026, 2, 28, 22, 0, 0, 0, berlin), ""),
	}
	out := renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})
	assert.Contains(t, cellFor(t, out, "March", 5), "late utc")
	assert.Contains(t, cellFor(t, out, "March", 4), "late local")
	assert.NotContains(t, out, "february", "an out-of-window value is skipped")
	assert.Contains(t, out, "(2 items)")

	items = []QueryResultItem{
		calNote(1, "date", time.Now(), `{"due":"2026-03-04"}`),
		calNote(2, "datetime", time.Now(), `{"due":"2026-03-04T23:30:00-05:00"}`),
		calNote(3, "number", time.Now(), `{"due":20260304}`),
		calNote(4, "bool", time.Now(), `{"due":true}`),
		calNote(5, "text", time.Now(), `{"due":"soon"}`),
		calNote(6, "bad date", time.Now(), `{"due":"2026-02-30"}`),
		calNote(7, "missing", time.Now(), `{}`),
		calNote(8, "nested", time.Now(), `{"ev":{"at":"2026-03-09"}}`),
	}
	out = renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "date": "meta.due"}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})
	cell := cellFor(t, out, "March", 4)
	assert.Contains(t, cell, ">date<")
	assert.Contains(t, cell, ">datetime<")
	for _, absent := range []string{"number", "bool", "text", "bad date", "missing", "nested"} {
		assert.NotContains(t, out, ">"+absent+"<", absent)
	}
	assert.Contains(t, out, "(2 items)")

	out = renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "date": "meta.ev.at"}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})
	assert.Contains(t, cellFor(t, out, "March", 9), ">nested<")
}

func calendarRows(out string) []string {
	return regexp.MustCompile(`<tr>.*?</tr>`).FindAllString(strings.SplitN(out, "<tbody>", 2)[1], -1)
}

func TestCalendarGridShape(t *testing.T) {
	pinCalendarClock(t, time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC), time.UTC)
	cases := []struct {
		month            string
		lead, rows, days int
	}{
		{"2026-03", 6, 6, 31}, // March 2026 starts on a Sunday
		{"2027-02", 0, 4, 28}, // February 2027 starts on a Monday
		{"2028-02", 1, 5, 29}, // leap year
		{"2026-06", 0, 5, 30},
	}
	for _, c := range cases {
		out := renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "month": c.month}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}})
		rows := calendarRows(out)
		assert.Len(t, rows, c.rows, c.month)
		cells := strings.Count(out, "<td")
		assert.Zero(t, cells%7, c.month)
		assert.Equal(t, c.days, strings.Count(out, "calendar-shortcode-cell"), c.month)
		assert.Equal(t, c.lead, strings.Count(rows[0], "calendar-shortcode-pad"), c.month)
	}
}

func TestCalendarAccessibleMarkup(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), time.UTC)
	at := func(d int) time.Time { return time.Date(2026, 3, d, 9, 0, 0, 0, time.UTC) }
	items := []QueryResultItem{
		calNote(1, "a", at(4), ""), calNote(2, "b", at(4), ""), calNote(3, "c", at(4), ""),
		calNote(4, "d", at(4), ""), calNote(5, "<script>alert(1)</script>", at(4), ""),
		calNote(6, "solo", at(9), ""),
	}
	var calls []string
	reqCtx := WithRefreshSigner(context.Background(), fakeSigner(&calls))
	out := renderCalendar(t, reqCtx, map[string]string{"mrql": `type = "note"`, "scope": "global"}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})

	dayLink := regexp.MustCompile(`<a class="calendar-shortcode-daylink"[^>]*>.*?</a>`)
	assert.Equal(t, "March 4, 5 items", accessibleText(dayLink.FindString(cellFor(t, out, "March", 4))))
	assert.Equal(t, "March 9, 1 item", accessibleText(dayLink.FindString(cellFor(t, out, "March", 9))))

	empty := cellFor(t, out, "March", 5)
	assert.NotContains(t, empty, "<a")
	assert.Equal(t, "March 5, no items", accessibleText(empty))

	four := cellFor(t, out, "March", 4)
	assert.Equal(t, 3, strings.Count(four, "<li>"))
	assert.Contains(t, accessibleText(four), "+2 more items on March 4")
	assert.NotContains(t, out, "<script>")

	assert.Equal(t, 1, strings.Count(out, `aria-current="date"`))
	assert.Contains(t, cellFor(t, out, "March", 9), `calendar-shortcode-today`)
	assert.Contains(t, cellFor(t, out, "March", 9), `aria-current="date"`)
	assert.Equal(t, 7, strings.Count(out, `<th scope="col">`))
	assert.Contains(t, out, `<span aria-hidden="true">Mon</span><span class="sr-only">Monday</span>`)

	host := regexp.MustCompile(`<calendar-shortcode class="calendar-shortcode" role="group" aria-labelledby="(cal-[A-Z0-9]{12}-cap)"`).FindStringSubmatch(out)
	require.NotNil(t, host, out[:200])
	assert.Contains(t, out, `<caption id="`+host[1]+`">March 2026 <span class="calendar-shortcode-total">(6 items)</span></caption>`)

	buttons := regexp.MustCompile(`<button [^>]*data-calendar-nav="(prev|next)">(.*?)</button>`).FindAllStringSubmatch(out, -1)
	require.Len(t, buttons, 2)
	assert.Equal(t, "Previous month, February 2026", accessibleText(buttons[0][2]))
	assert.Equal(t, "Next month, April 2026", accessibleText(buttons[1][2]))

	again := renderCalendar(t, reqCtx, map[string]string{"mrql": `type = "note"`}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}})
	other := regexp.MustCompile(`aria-labelledby="(cal-[A-Z0-9]{12}-cap)"`).FindStringSubmatch(again)
	assert.NotEqual(t, host[1], other[1], "two calendars never share an ID")

	// Not the current month: no today.
	out = renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "month": "2026-04"}, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}})
	assert.NotContains(t, out, `aria-current`)
	assert.Contains(t, out, "(no items)")
}

func TestCalendarDayLinks(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), time.UTC)
	items := []QueryResultItem{calNote(1, "a", time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC), `{"due":"2026-03-04"}`)}
	href := func(out string) string {
		m := regexp.MustCompile(`class="calendar-shortcode-daylink" href="/mrql\?q=([^"]*)"`).FindStringSubmatch(out)
		require.NotNil(t, m)
		q, err := url.QueryUnescape(strings.ReplaceAll(m[1], "&amp;", "&"))
		require.NoError(t, err)
		return q
	}
	attrs := map[string]string{"mrql": `type = "note"`, "date": "meta.due"}
	out := renderCalendar(t, nil, attrs, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})
	assert.Equal(t, `(type = "note") AND meta.due >= "2026-03-04" AND meta.due < "2026-03-05" AND meta.due ~ "????-??-??*" SCOPE 7`, href(out))
	assert.Contains(t, out, `<li><a href="/note?id=1">a</a></li>`)

	attrs["scope"] = "global"
	out = renderCalendar(t, nil, attrs, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})
	assert.Equal(t, `(type = "note") AND meta.due >= "2026-03-04" AND meta.due < "2026-03-05" AND meta.due ~ "????-??-??*"`, href(out))

	// An ownerless entity's scope matches nothing: no links at all.
	attrs["scope"] = fmt.Sprint(mrql.UnresolvedScopeSentinel)
	out = renderCalendar(t, nil, attrs, &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}})
	assert.NotContains(t, out, "<a ")
}

func TestCalendarNavigationTokens(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), time.UTC)
	type sealed struct {
		typ       string
		id        uint
		body      string
		untrusted bool
	}
	var got []sealed
	signer := func(entityType string, entityID uint, body string, untrusted bool) string {
		got = append(got, sealed{entityType, entityID, body, untrusted})
		return fmt.Sprintf("TOK%d", len(got))
	}
	reqCtx := WithRefreshSigner(context.Background(), signer)
	attrs := map[string]string{"mrql": `type = "note" AND name ~ "ab"`, "date": "meta.due", "scope": "global", "month": "2026-03"}
	ctx := calendarCtx()
	ctx.UntrustedSource = true
	out := RenderCalendarShortcode(reqCtx, Shortcode{Name: "calendar", Attrs: attrs}, ctx, (&fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}).exec)
	assert.Contains(t, out, `data-prev-token="TOK1" data-next-token="TOK2"`)
	require.Len(t, got, 2)
	for i, month := range []string{"2026-02", "2026-04"} {
		assert.Equal(t, "group", got[i].typ)
		assert.Equal(t, uint(7), got[i].id)
		assert.True(t, got[i].untrusted)
		parsed := ParseWithBlocks(got[i].body)
		require.Len(t, parsed, 1, got[i].body)
		assert.Equal(t, "calendar", parsed[0].Name)
		want := map[string]string{}
		for k, v := range attrs {
			want[k] = v
		}
		want["month"] = month
		assert.Equal(t, want, parsed[0].Attrs)
	}

	// No signer, or a carrier context: no navigation.
	out = RenderCalendarShortcode(context.Background(), Shortcode{Name: "calendar", Attrs: attrs}, ctx, (&fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}).exec)
	assert.NotContains(t, out, "data-calendar-nav")
	assert.NotContains(t, out, "data-prev-token")
	carrier := MetaShortcodeContext{EntityType: "category", EntityID: 3}
	out = RenderCalendarShortcode(reqCtx, Shortcode{Name: "calendar", Attrs: attrs}, carrier, (&fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}).exec)
	assert.NotContains(t, out, "data-calendar-nav")

	// Year bounds.
	attrs["month"] = "1000-01"
	out = RenderCalendarShortcode(reqCtx, Shortcode{Name: "calendar", Attrs: attrs}, ctx, (&fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}).exec)
	assert.NotContains(t, out, `data-calendar-nav="prev"`)
	assert.Contains(t, out, `data-calendar-nav="next"`)
	attrs["month"] = "9999-12"
	out = RenderCalendarShortcode(reqCtx, Shortcode{Name: "calendar", Attrs: attrs}, ctx, (&fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}).exec)
	assert.Contains(t, out, `data-calendar-nav="prev"`)
	assert.NotContains(t, out, `data-calendar-nav="next"`)
}

func TestCalendarSerializeQuoting(t *testing.T) {
	body, ok := serializeCalendarShortcode(map[string]string{"mrql": `type = "note"`, "date": "created"}, "2026-05")
	require.True(t, ok)
	assert.Equal(t, `[calendar date="created" month="2026-05" mrql='type = "note"']`, body)
	_, ok = serializeCalendarShortcode(map[string]string{"mrql": `a"b'c`}, "2026-05")
	assert.False(t, ok)

	// The deferred render unescapes the attributes again: a value holding an
	// entity reference must come back exactly as it was parsed.
	attrs := map[string]string{"mrql": `name = "&lt;b&gt; & co"`}
	body, ok = serializeCalendarShortcode(attrs, "2026-05")
	require.True(t, ok)
	parsed := Parse(body)
	require.Len(t, parsed, 1)
	assert.Equal(t, attrs["mrql"], parsed[0].Attrs["mrql"])
}

func TestCalendarErrors(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), time.UTC)
	marker := func(detail string) string { return shortcodeErrorMarker("calendar", detail) }
	none := &fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}

	assert.Equal(t, marker("[calendar] requires mrql="), renderCalendar(t, nil, map[string]string{}, none))
	assert.Equal(t, marker("[calendar] requires mrql="), renderCalendar(t, nil, map[string]string{"mrql": "  "}, none))
	assert.Equal(t, marker(calendarMonthProblem), renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "month": "2026-13"}, none))
	assert.Equal(t, marker(calendarDateProblem), renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`, "date": "name"}, none))
	assert.Equal(t, marker("[calendar] mrql cannot use $parameters"), renderCalendar(t, nil, map[string]string{"mrql": `type = "note" AND name = $n`}, none))
	assert.Contains(t, renderCalendar(t, nil, map[string]string{"mrql": `type = "note" LIMIT 3`}, none), "filter only")
	assert.Contains(t, renderCalendar(t, nil, map[string]string{"mrql": `name = "x"`}, none), "one entity type")
	assert.Contains(t, renderCalendar(t, nil, map[string]string{"mrql": `type = "note" AND (`}, none), "shortcode-error")
	assert.Empty(t, none.calls, "an invalid calendar runs no query")

	face := markReloadFace(context.Background())
	assert.Equal(t, marker("[calendar] cannot be used inside a [reload] button face"), renderCalendar(t, face, map[string]string{"mrql": `type = "note"`}, none))

	budget := errors.New("inline query budget exceeded (200 per page); refine templates or raise -mrql-page-query-budget")
	assert.Equal(t, mrqlErrorHTML(budget, false), renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`}, &fakeCalendarExecutor{err: budget}))

	unavailable := NewQueryUnavailable("")
	assert.Equal(t, unavailableComment(unavailable), renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`}, &fakeCalendarExecutor{err: unavailable}))

	// The count query failing renders its error too.
	items := make([]QueryResultItem, 101)
	for i := range items {
		items[i] = calNote(uint(i+1), "n", time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC), "")
	}
	f := &fakeCalendarExecutor{list: &QueryResult{Mode: "flat", Items: items}}
	exec := func(c context.Context, q string, o QueryOptions) (*QueryResult, error) {
		if strings.Contains(q, "GROUP BY") {
			return nil, budget
		}
		return f.exec(c, q, o)
	}
	assert.Equal(t, mrqlErrorHTML(budget, false), RenderCalendarShortcode(context.Background(), Shortcode{Name: "calendar", Attrs: map[string]string{"mrql": `type = "note"`}}, calendarCtx(), exec))

	// No executor at all: the processor leaves a comment.
	assert.Equal(t, shortcodeComment("calendar unavailable in this context"), Process(context.Background(), `[calendar mrql='type = "note"']`, calendarCtx(), nil, nil))

	// Zero results: a full grid with no links.
	out := renderCalendar(t, nil, map[string]string{"mrql": `type = "note"`}, none)
	assert.Contains(t, out, "(no items)")
	assert.NotContains(t, out, "<a ")
	assert.Equal(t, 31, strings.Count(out, ", no items</span>"))
}

// The processor dispatches [calendar] through the parser.
func TestCalendarThroughProcess(t *testing.T) {
	pinCalendarClock(t, time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), time.UTC)
	f := &fakeCalendarExecutor{list: &QueryResult{Mode: "flat"}}
	out := Process(context.Background(), `<div>[calendar mrql='type = "note"' month="2026-03"]</div>`, calendarCtx(), nil, f.exec)
	assert.True(t, strings.HasPrefix(out, `<div><calendar-shortcode `), out)
	assert.True(t, strings.HasSuffix(out, `</calendar-shortcode></div>`))
	require.Len(t, f.calls, 1)
}
