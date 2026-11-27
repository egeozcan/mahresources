package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/html"

	"mahresources/mrql"
)

// fakePivotExecutor answers every query with rows (mode "aggregated" unless
// mode is set) and records the calls.
type fakePivotExecutor struct {
	rows  []map[string]any
	mode  string
	err   error
	calls []usingCall
}

func (f *fakePivotExecutor) exec(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
	f.calls = append(f.calls, usingCall{query, opts})
	if f.err != nil {
		return nil, f.err
	}
	mode := f.mode
	if mode == "" {
		mode = "aggregated"
	}
	return &QueryResult{Mode: mode, Rows: f.rows, DrillDownLimit: 1000}, nil
}

func pivotCtx() MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "group", EntityID: 7, ScopeGroupID: 7, ParentGroupID: 3, RootGroupID: 1}
}

func renderPivot(t *testing.T, attrs map[string]string, f *fakePivotExecutor) string {
	t.Helper()
	return RenderPivotShortcode(context.Background(), Shortcode{Name: "pivot", Attrs: attrs}, pivotCtx(), f.exec)
}

// statusAttrs is a count pivot of notes by meta.status and meta.assignee.
func statusAttrs(extra ...string) map[string]string {
	attrs := map[string]string{"query": "type = note", "rows": "meta.status", "cols": "meta.assignee"}
	for i := 0; i+1 < len(extra); i += 2 {
		attrs[extra[i]] = extra[i+1]
	}
	return attrs
}

func row(r, c any, count any) map[string]any {
	return map[string]any{"meta.status": r, "meta.assignee": c, "count": count}
}

// statusRows is a 3 × 2 result in the order the query returns it (count DESC).
func statusRows() []map[string]any {
	return []map[string]any{
		row("done", "ann", int64(4)),
		row("open", "ann", int64(3)),
		row("open", "bob", int64(2)),
		row("blocked", "bob", int64(1)),
	}
}

// pivotCell is one rendered <th> or <td>: the text a sighted reader sees
// (sr-only text left out), the text a screen reader reads (aria-hidden left
// out), and its link, if any.
type pivotCell struct {
	tag, visible, spoken, href, label, class string
}

type pivotGrid struct {
	captions int
	caption  string
	region   string // aria-label of the scroll region
	head     []pivotCell
	body     [][]pivotCell
	foot     []pivotCell
	hasFoot  bool
}

func attrOf(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func nodeText(n *html.Node, skip func(*html.Node) bool) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skip(n) {
			return
		}
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.TrimSpace(b.String())
}

func parsePivot(t *testing.T, out string) pivotGrid {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(out))
	require.NoError(t, err)
	var g pivotGrid
	srOnly := func(n *html.Node) bool { return strings.Contains(" "+attrOf(n, "class")+" ", " sr-only ") }
	hidden := func(n *html.Node) bool { return attrOf(n, "aria-hidden") == "true" }
	cell := func(n *html.Node) pivotCell {
		c := pivotCell{tag: n.Data, visible: nodeText(n, srOnly), spoken: nodeText(n, hidden), class: attrOf(n, "class")}
		var find func(*html.Node)
		find = func(n *html.Node) {
			if n.Type == html.ElementNode && n.Data == "a" {
				c.href, c.label = attrOf(n, "href"), attrOf(n, "aria-label")
			}
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				find(ch)
			}
		}
		find(n)
		return c
	}
	cells := func(tr *html.Node) []pivotCell {
		var out []pivotCell
		for c := tr.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "th" || c.Data == "td") {
				out = append(out, cell(c))
			}
		}
		return out
	}
	var walk func(*html.Node, string)
	walk = func(n *html.Node, section string) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "caption":
				g.captions++
				g.caption = nodeText(n, func(*html.Node) bool { return false })
			case "div":
				if attrOf(n, "role") == "region" {
					g.region = attrOf(n, "aria-label")
				}
			case "thead", "tbody":
				section = n.Data
			case "tfoot":
				section = n.Data
				g.hasFoot = true
			case "tr":
				switch section {
				case "thead":
					g.head = cells(n)
				case "tbody":
					g.body = append(g.body, cells(n))
				case "tfoot":
					g.foot = cells(n)
				}
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, section)
		}
	}
	walk(doc, "")
	return g
}

func visible(cells []pivotCell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.visible
	}
	return out
}

func (g pivotGrid) bodyVisible() [][]string {
	out := make([][]string, len(g.body))
	for i, r := range g.body {
		out[i] = visible(r)
	}
	return out
}

// stringLiterals lists the string values compared in a parsed filter.
func stringLiterals(n mrql.Node) []string {
	switch x := n.(type) {
	case *mrql.BinaryExpr:
		return append(stringLiterals(x.Left), stringLiterals(x.Right)...)
	case *mrql.ComparisonExpr:
		if s, ok := x.Value.(*mrql.StringLiteral); ok {
			return []string{s.Value}
		}
	}
	return nil
}

// linkQuery is the MRQL a cell link opens.
func linkQuery(t *testing.T, href string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(href, "/mrql?q="), href)
	u, err := url.Parse(href)
	require.NoError(t, err)
	return u.Query().Get("q")
}

func TestPivotComposesOneAggregatedQuery(t *testing.T) {
	f := &fakePivotExecutor{rows: statusRows()}
	renderPivot(t, map[string]string{"query": "type = note AND meta.status IS NOT NULL", "rows": "meta.status", "cols": "owner"}, f)
	require.Len(t, f.calls, 1)
	assert.Equal(t, "(type = note AND meta.status IS NOT NULL) GROUP BY meta.status, owner COUNT() ORDER BY count DESC, meta.status ASC, owner ASC", f.calls[0].query)
	assert.Equal(t, pivotFetchLimit+1, f.calls[0].opts.Limit)

	f = &fakePivotExecutor{}
	renderPivot(t, map[string]string{"query": "type = resource", "rows": "contentType", "cols": "created.year", "value": "sum", "field": "fileSize"}, f)
	require.Len(t, f.calls, 1)
	assert.Equal(t, "(type = resource) GROUP BY contentType, created.year COUNT() SUM(fileSize) ORDER BY count DESC, contentType ASC, created.year ASC", f.calls[0].query)

	for scope, want := range map[string]uint{"": 7, "entity": 7, "parent": 3, "root": 1, "global": 0, "42": 42} {
		f := &fakePivotExecutor{}
		renderPivot(t, statusAttrs("scope", scope), f)
		require.Len(t, f.calls, 1)
		assert.Equal(t, want, f.calls[0].opts.ScopeGroupID, "scope=%q", scope)
	}
}

func TestPivotRejectsInvalidInputWithoutQuerying(t *testing.T) {
	const filterMsg = "[pivot] query must be a filter expression; use the scope attribute and leave GROUP BY/ORDER BY/LIMIT/OFFSET/SCOPE out"
	const fieldMsg = "[pivot] field must be a meta.* path, or fileSize, width or height on resources"
	unsupported := func(attr, name string) string {
		return fmt.Sprintf("[pivot] %s: %q is not supported; use a scalar field, owner, groups, tags, a meta.* path, or a date bucket like created.year", attr, name)
	}
	ids := func(attr, name string) string {
		return fmt.Sprintf("[pivot] %s: %q holds IDs, which would head the table as bare numbers; pivot by a meta.* field instead, or filter by %s in query", attr, name, name)
	}
	cases := []struct {
		attrs map[string]string
		want  string
	}{
		{map[string]string{"query": "type = note", "rows": "meta.status"}, "[pivot] requires query, rows and cols"},
		{map[string]string{"query": "  ", "rows": "meta.status", "cols": "owner"}, "[pivot] requires query, rows and cols"},
		{statusAttrs("query", "type = note GROUP BY name"), filterMsg},
		{statusAttrs("query", "type = note ORDER BY name ASC"), filterMsg},
		{statusAttrs("query", "type = note LIMIT 5"), filterMsg},
		{statusAttrs("query", "type = note OFFSET 2"), filterMsg},
		{statusAttrs("query", "type = note SCOPE 3"), filterMsg},
		{statusAttrs("query", "type = note AND name = $n"), "[pivot] query cannot use $parameters"},
		{statusAttrs("query", `name = "x"`), "[pivot] query must name one entity type (type = resource, note or group)"},
		{statusAttrs("rows", "meta.x) OR (1"), `[pivot] rows: "meta.x) OR (1" is not a field name`},
		{statusAttrs("rows", "owner.name"), unsupported("rows", "owner.name")},
		{statusAttrs("cols", "tags.name"), unsupported("cols", "tags.name")},
		{statusAttrs("cols", "tags.count"), unsupported("cols", "tags.count")},
		{statusAttrs("query", "type = resource", "rows", "similarImages"), unsupported("rows", "similarImages")},
		{statusAttrs("rows", "description"), `[pivot] rows: "description" is unbounded text; filter by it in query instead`},
		{statusAttrs("query", "type = group", "cols", "url"), `[pivot] cols: "url" is unbounded text; filter by it in query instead`},
		{statusAttrs("query", "type = resource", "rows", "originalLocation"), `[pivot] rows: "originalLocation" is unbounded text; filter by it in query instead`},
		{statusAttrs("query", "type = resource", "cols", "originalName"), `[pivot] cols: "originalName" is unbounded text; filter by it in query instead`},
		{statusAttrs("rows", "created"), `[pivot] rows: "created" is a timestamp; use a date bucket like created.month or created.year`},
		{statusAttrs("cols", "startDate"), `[pivot] cols: "startDate" is a timestamp; use a date bucket like startDate.month or startDate.year`},
		{statusAttrs("cols", "noteType"), ids("cols", "noteType")},
		{statusAttrs("query", "type = resource", "rows", "category"), ids("rows", "category")},
		{statusAttrs("rows", "id"), ids("rows", "id")},
		{statusAttrs("query", "type = resource", "cols", "shared"), `[pivot] cols: "shared" is not a resource field`},
		{statusAttrs("rows", "group", "cols", "groups"), "[pivot] rows and cols must be different fields"},
		{statusAttrs("cols", "meta.status"), "[pivot] rows and cols must be different fields"},
		{statusAttrs("value", "avg"), "[pivot] value must be count or sum"},
		{statusAttrs("value", "sum"), `[pivot] value="sum" requires field`},
		{statusAttrs("value", "sum", "field", "name"), fieldMsg},
		{statusAttrs("value", "sum", "field", "id"), fieldMsg},
		{statusAttrs("value", "sum", "field", "noteType"), fieldMsg},
		{statusAttrs("value", "sum", "field", "fileSize"), fieldMsg},
		{statusAttrs("value", "sum", "field", "meta.a) + (1"), fieldMsg},
	}
	for _, tc := range cases {
		f := &fakePivotExecutor{rows: statusRows()}
		out := renderPivot(t, tc.attrs, f)
		assert.Equal(t, mrqlErrorHTML(errors.New(tc.want), false), out, "%v", tc.attrs)
		assert.Empty(t, f.calls, "%v ran a query", tc.attrs)
	}

	// A query that does not parse is reported with the [pivot] prefix.
	f := &fakePivotExecutor{}
	out := renderPivot(t, statusAttrs("query", "type = note AND ("), f)
	assert.Contains(t, out, "mrql-error")
	assert.Contains(t, out, "[pivot] ")
	assert.Empty(t, f.calls)
}

// pivotDateBucketSuffixes is a copy of mrql's; this keeps the two equal.
func TestPivotDateBucketSuffixesMatchMRQL(t *testing.T) {
	for _, s := range []string{"day", "week", "month", "year", "quarter", "hour", "minute", "decade", "dow", "date"} {
		q, err := mrql.Parse("type = resource GROUP BY created." + s + " COUNT()")
		require.NoError(t, err)
		accepted := mrql.Validate(q) == nil
		assert.Equal(t, accepted, pivotDateBucketSuffixes[s], "suffix %q", s)
	}
}

func TestPivotTableShape(t *testing.T) {
	out := renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: statusRows()})
	g := parsePivot(t, out)
	assert.Equal(t, 1, g.captions)
	assert.Equal(t, "Count by status and assignee", g.caption)
	assert.Equal(t, "Count by status and assignee", g.region)
	assert.Contains(t, out, `role="region" aria-label="Count by status and assignee" tabindex="0"`)
	assert.Equal(t, []string{"status", "ann", "bob", "Total"}, visible(g.head))
	assert.Equal(t, [][]string{
		{"open", "3", "2", "5"},
		{"done", "4", "0", "4"},
		{"blocked", "0", "1", "1"},
	}, g.bodyVisible())
	assert.Equal(t, []string{"Total", "7", "3", "10"}, visible(g.foot))
	// The corner and column headers are scope=col, every row header scope=row.
	assert.Equal(t, 4, strings.Count(out, `<th scope="col"`))
	assert.Equal(t, 4, strings.Count(out, `<th scope="row"`))
	// A combination the query did not return is a muted 0.
	assert.Contains(t, g.body[1][2].class, "text-stone-500")
	assert.Empty(t, g.body[1][2].href)

	sum := renderPivot(t, statusAttrs("value", "sum", "field", "meta.hours"), &fakePivotExecutor{})
	assert.NotContains(t, sum, "Sum of") // zero rows: no table
	sg := parsePivot(t, renderPivot(t, statusAttrs("value", "sum", "field", "meta.hours"), &fakePivotExecutor{rows: []map[string]any{
		{"meta.status": "open", "meta.assignee": "ann", "count": int64(2), "sum_meta.hours": 1.5},
	}}))
	assert.Equal(t, "Sum of hours by status and assignee", sg.caption)
	assert.Equal(t, [][]string{{"open", "1.5", "1.5"}}, sg.bodyVisible())
}

func TestPivotEscapesLabelsAndHonoursBlankLabels(t *testing.T) {
	out := renderPivot(t, statusAttrs("caption", "<b>Mine</b>", "rows-label", "   ", "cols-label", "Who & why"),
		&fakePivotExecutor{rows: []map[string]any{row("<script>x</script>", "ann", int64(1))}})
	assert.NotContains(t, out, "<script>")
	assert.NotContains(t, out, "<b>")
	assert.Contains(t, out, "&lt;script&gt;x&lt;/script&gt;")
	g := parsePivot(t, out)
	assert.Equal(t, "<b>Mine</b>", g.caption)
	assert.Equal(t, "status", g.head[0].visible, "a blank rows-label falls back to the field")
	assert.Equal(t, "1: status <script>x</script>, Who & why ann", g.body[0][1].label)
}

func TestPivotCapsRankAndOrder(t *testing.T) {
	var rows []map[string]any
	for i := 0; i < 25; i++ {
		rows = append(rows, row(fmt.Sprintf("s%02d", i), "ann", int64(100-i)))
	}
	out := renderPivot(t, statusAttrs("max-rows", "5"), &fakePivotExecutor{rows: rows})
	g := parsePivot(t, out)
	require.Len(t, g.body, 5)
	assert.Equal(t, []string{"s00", "s01", "s02", "s03", "s04"}, []string{g.body[0][0].visible, g.body[1][0].visible, g.body[2][0].visible, g.body[3][0].visible, g.body[4][0].visible})
	assert.Contains(t, out, "20 more status values are not shown; the totals include them.")
	total := 0
	for i := 0; i < 25; i++ {
		total += 100 - i
	}
	assert.Equal(t, fmt.Sprint(total), g.foot[1].visible, "column totals include hidden rows")

	// Ties break by label; a single hidden value is singular.
	tied := []map[string]any{row("b", "ann", int64(2)), row("c", "ann", int64(2)), row("a", "ann", int64(2))}
	out = renderPivot(t, statusAttrs("max-rows", "2"), &fakePivotExecutor{rows: tied})
	g = parsePivot(t, out)
	assert.Equal(t, "a", g.body[0][0].visible)
	assert.Equal(t, "b", g.body[1][0].visible)
	assert.Contains(t, out, "1 more status value is not shown; the totals include them.")

	// max-rows: clamped to 50, and an invalid value means the default 20.
	var many []map[string]any
	for i := 0; i < 60; i++ {
		many = append(many, row(fmt.Sprintf("v%02d", i), "ann", int64(1)))
	}
	assert.Len(t, parsePivot(t, renderPivot(t, statusAttrs("max-rows", "100"), &fakePivotExecutor{rows: many})).body, 50)
	assert.Len(t, parsePivot(t, renderPivot(t, statusAttrs("max-rows", "0"), &fakePivotExecutor{rows: many})).body, 20)
	assert.Len(t, parsePivot(t, renderPivot(t, statusAttrs("max-rows", "x"), &fakePivotExecutor{rows: many})).body, 20)
	var wide []map[string]any
	for i := 0; i < 30; i++ {
		wide = append(wide, row("open", fmt.Sprintf("p%02d", i), int64(1)))
	}
	assert.Len(t, parsePivot(t, renderPivot(t, statusAttrs("max-cols", "99"), &fakePivotExecutor{rows: wide})).head, 1+20+1)
	assert.Len(t, parsePivot(t, renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: wide})).head, 1+10+1)

	// A date bucket reads chronologically; the none-bucket goes last.
	years := []map[string]any{
		{"created.year": "2021", "owner": "Inbox", "count": int64(5)},
		{"created.year": nil, "owner": "Inbox", "count": int64(9)},
		{"created.year": "2020", "owner": "Inbox", "count": int64(3)},
		{"created.year": "2019", "owner": "Inbox", "count": int64(1)},
	}
	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = note", "rows": "created.year", "cols": "owner"}, &fakePivotExecutor{rows: years}))
	assert.Equal(t, [][]string{{"2019", "1", "1"}, {"2020", "3", "3"}, {"2021", "5", "5"}, {"(none)", "9", "9"}}, g.bodyVisible())
	owners := []map[string]any{
		{"meta.status": "open", "owner": nil, "count": int64(9)},
		{"meta.status": "open", "owner": "Inbox", "count": int64(2)},
		{"meta.status": "open", "owner": "", "count": int64(1)},
	}
	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = note", "rows": "meta.status", "cols": "owner"}, &fakePivotExecutor{rows: owners}))
	assert.Equal(t, []string{"status", "Inbox", "(empty)", "(none)", "Total"}, visible(g.head))
}

func TestPivotMergesValuesMRQLCompareAsEqual(t *testing.T) {
	g := parsePivot(t, renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: []map[string]any{
		row("Open", "ann", int64(3)),
		row("open", "ann", int64(2)),
		row(int64(3), "ann", int64(1)),
		row("3", "ann", int64(1)),
		row([]byte("x"), "ann", int64(1)),
		row("x", "ann", int64(1)),
	}}))
	assert.Equal(t, [][]string{{"Open", "5", "5"}, {"3", "2", "2"}, {"x", "2", "2"}}, g.bodyVisible())

	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = note", "rows": "owner", "cols": "name"}, &fakePivotExecutor{rows: []map[string]any{
		{"owner": "Inbox", "name": "Foo", "count": int64(2)},
		{"owner": "Inbox", "name": "foo", "count": int64(1)},
		{"owner": "Inbox", "name": "Bar", "count": int64(1)},
	}}))
	assert.Equal(t, []string{"owner", "Foo", "Bar", "Total"}, visible(g.head))
	assert.Equal(t, [][]string{{"Inbox", "3", "1", "4"}}, g.bodyVisible())
}

func TestPivotMultiValuedDimensionsDropOvercountingTotals(t *testing.T) {
	tagRows := []map[string]any{
		{"tags": "red", "meta.assignee": "ann", "count": int64(2)},
		{"tags": "blue", "meta.assignee": "ann", "count": int64(1)},
	}
	out := renderPivot(t, map[string]string{"query": "type = note", "rows": "tags", "cols": "meta.assignee"}, &fakePivotExecutor{rows: tagRows})
	g := parsePivot(t, out)
	assert.False(t, g.hasFoot, "rows=tags: no column totals and no grand total")
	assert.Equal(t, []string{"tags", "ann", "Total"}, visible(g.head))

	groupRows := []map[string]any{
		{"meta.status": "open", "groups": "A", "count": int64(2)},
		{"meta.status": "open", "groups": "B", "count": int64(1)},
	}
	out = renderPivot(t, map[string]string{"query": "type = note", "rows": "meta.status", "cols": "groups"}, &fakePivotExecutor{rows: groupRows})
	g = parsePivot(t, out)
	assert.Equal(t, []string{"status", "A", "B"}, visible(g.head), "cols=groups: no Total column")
	assert.Equal(t, []string{"Total", "2", "1"}, visible(g.foot), "and no grand total")
	assert.NotContains(t, out, "total of all")
}

func TestPivotTruncation(t *testing.T) {
	big := func(n int) []map[string]any {
		rows := make([]map[string]any, 0, n)
		for i := 0; i < n; i++ {
			rows = append(rows, row(fmt.Sprintf("s%04d", i), fmt.Sprintf("a%d", i%2), int64(1)))
		}
		return rows
	}
	out := renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: big(pivotFetchLimit + 1)})
	assert.Contains(t, out, "Only the 5000 combinations with the most items were counted")
	assert.NotContains(t, out, "<a ")
	g := parsePivot(t, out)
	assert.Equal(t, "2500+", g.foot[1].visible)
	assert.Equal(t, "2500 or more", g.foot[1].spoken)
	assert.Equal(t, "5000+", g.foot[3].visible, "the grand total counts the 5000 kept rows")
	assert.Equal(t, "—", g.body[0][2].visible, "s0000 has no a1 combination")
	assert.Equal(t, "not counted", g.body[0][2].spoken)

	out = renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: big(pivotFetchLimit)})
	assert.NotContains(t, out, "Only the 5000")
	assert.NotContains(t, out, "not counted")
	assert.NotContains(t, out, "or more")
	assert.Contains(t, out, "<a ")
}

func TestPivotCountLinksFilterToTheCell(t *testing.T) {
	out := renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: statusRows()})
	g := parsePivot(t, out)
	openAnn := g.body[0][1]
	assert.Equal(t, `(type = note) AND meta.status = "open" AND meta.assignee = "ann" SCOPE 7`, linkQuery(t, openAnn.href))
	assert.Equal(t, "3: status open, assignee ann", openAnn.label)
	assert.Equal(t, `(type = note) AND meta.status = "open" SCOPE 7`, linkQuery(t, g.body[0][3].href))
	assert.Equal(t, "5: total for status open", g.body[0][3].label)
	assert.Equal(t, `(type = note) AND meta.assignee = "ann" SCOPE 7`, linkQuery(t, g.foot[1].href))
	assert.Equal(t, "7: total for assignee ann", g.foot[1].label)
	assert.Equal(t, `(type = note) SCOPE 7`, linkQuery(t, g.foot[3].href))
	assert.Equal(t, "10: total of all", g.foot[3].label)
	for _, href := range []string{openAnn.href, g.body[0][3].href, g.foot[1].href, g.foot[3].href} {
		_, err := mrql.Parse(linkQuery(t, href))
		assert.NoError(t, err, href)
	}

	// global scope carries no SCOPE clause.
	g = parsePivot(t, renderPivot(t, statusAttrs("scope", "global"), &fakePivotExecutor{rows: statusRows()}))
	assert.Equal(t, `(type = note) AND meta.status = "open" AND meta.assignee = "ann"`, linkQuery(t, g.body[0][1].href))
}

func TestPivotLinkLiterals(t *testing.T) {
	// PostgreSQL returns a Meta number as text, so the key links as text.
	g := parsePivot(t, renderPivot(t, statusAttrs("scope", "global"), &fakePivotExecutor{rows: []map[string]any{row("1", "ann", int64(1))}}))
	assert.Equal(t, `(type = note) AND meta.status = "1" AND meta.assignee = "ann"`, linkQuery(t, g.body[0][1].href))

	// A float-fed meta bucket (SQLite) is not linked; its partner still is.
	g = parsePivot(t, renderPivot(t, statusAttrs("scope", "global"), &fakePivotExecutor{rows: []map[string]any{row(3.5, "ann", int64(1))}}))
	assert.Equal(t, "3.5", g.body[0][0].visible)
	assert.Empty(t, g.body[0][1].href)
	assert.Empty(t, g.body[0][2].href, "row total")
	assert.Equal(t, `(type = note) AND meta.assignee = "ann"`, linkQuery(t, g.foot[1].href))

	// Quotes and backslashes are escaped so the literal reads back exactly.
	label := `say "hi" \ it's`
	g = parsePivot(t, renderPivot(t, statusAttrs("scope", "global"), &fakePivotExecutor{rows: []map[string]any{row(label, "ann", int64(1))}}))
	q := linkQuery(t, g.body[0][1].href)
	assert.Equal(t, `(type = note) AND meta.status = "say \"hi\" \\ it's" AND meta.assignee = "ann"`, q)
	parsed, err := mrql.Parse(q)
	require.NoError(t, err)
	assert.Contains(t, stringLiterals(parsed.Where), label)

	// fileSize links with a bare number; created.year, the none-bucket and an
	// opaque value are not linked.
	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = resource", "rows": "fileSize", "cols": "created.year", "scope": "global"}, &fakePivotExecutor{rows: []map[string]any{
		{"fileSize": int64(1024), "created.year": "2024", "count": int64(2)},
		{"fileSize": nil, "created.year": "2024", "count": int64(1)},
		{"fileSize": struct{}{}, "created.year": "2024", "count": int64(1)},
	}}))
	assert.Equal(t, `(type = resource) AND fileSize = 1024`, linkQuery(t, g.body[0][2].href))
	assert.Empty(t, g.body[0][1].href, "a date bucket cell")
	assert.Empty(t, g.foot[1].href, "a date bucket total")
	for _, r := range g.body[1:] {
		assert.Empty(t, r[2].href, "%s row total", r[0].visible)
	}
	assert.NotEmpty(t, g.foot[2].href, "the grand total is linked")

	// shared groups as 1/0 and links as true/false.
	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = note", "rows": "shared", "cols": "meta.assignee", "scope": "global"}, &fakePivotExecutor{rows: []map[string]any{
		{"shared": int64(1), "meta.assignee": "ann", "count": int64(2)},
		{"shared": int64(0), "meta.assignee": "ann", "count": int64(1)},
	}}))
	assert.Equal(t, [][]string{{"yes", "2", "2"}, {"no", "1", "1"}}, g.bodyVisible())
	assert.Equal(t, `(type = note) AND shared = true`, linkQuery(t, g.body[0][2].href))
	assert.Equal(t, `(type = note) AND shared = false`, linkQuery(t, g.body[1][2].href))

	// On a multi-valued relation, two rows in one cell may be two same-named
	// entities on one item: that bucket loses its links.
	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = note", "rows": "groups", "cols": "meta.assignee", "scope": "global"}, &fakePivotExecutor{rows: []map[string]any{
		{"groups": "Red", "meta.assignee": "ann", "count": int64(1)},
		{"groups": "red", "meta.assignee": "ann", "count": int64(1)},
		{"groups": "Blue", "meta.assignee": "ann", "count": int64(1)},
	}}))
	assert.Equal(t, [][]string{{"Red", "2", "2"}, {"Blue", "1", "1"}}, g.bodyVisible())
	assert.Empty(t, g.body[0][1].href)
	assert.Empty(t, g.body[0][2].href)
	assert.Equal(t, `(type = note) AND groups = "Blue" AND meta.assignee = "ann"`, linkQuery(t, g.body[1][1].href))
	// A single-valued owner keeps its link: owner = "Inbox" matches both.
	g = parsePivot(t, renderPivot(t, map[string]string{"query": "type = note", "rows": "owner", "cols": "meta.assignee", "scope": "global"}, &fakePivotExecutor{rows: []map[string]any{
		{"owner": "Inbox", "meta.assignee": "ann", "count": int64(1)},
		{"owner": "inbox", "meta.assignee": "ann", "count": int64(1)},
	}}))
	assert.Equal(t, `(type = note) AND owner = "Inbox" AND meta.assignee = "ann"`, linkQuery(t, g.body[0][1].href))
}

func TestPivotSumModeHasNoLinks(t *testing.T) {
	out := renderPivot(t, statusAttrs("value", "sum", "field", "meta.hours"), &fakePivotExecutor{rows: []map[string]any{
		{"meta.status": "open", "meta.assignee": "ann", "count": int64(2), "sum_meta.hours": 1.25},
		{"meta.status": "open", "meta.assignee": "bob", "count": int64(1), "sum_meta.hours": "2"},
		{"meta.status": "done", "meta.assignee": "bob", "count": int64(1), "sum_meta.hours": nil},
	}})
	assert.NotContains(t, out, "<a")
	g := parsePivot(t, out)
	// bob (2) outranks ann (1.25); a NULL sum counts as 0.
	assert.Equal(t, [][]string{{"open", "2", "1.25", "3.25"}, {"done", "0", "0", "0"}}, g.bodyVisible())
	assert.Equal(t, []string{"Total", "2", "1.25", "3.25"}, visible(g.foot))
}

func TestPivotSumCapsRankBySumNotItemCount(t *testing.T) {
	rows := []map[string]any{row("many", "ann", int64(2)), row("few", "bob", int64(1))}
	rows[0]["sum_meta.hours"] = 2
	rows[1]["sum_meta.hours"] = 10
	attrs := statusAttrs("value", "sum", "field", "meta.hours", "max-rows", "1", "max-cols", "1")
	g := parsePivot(t, renderPivot(t, attrs, &fakePivotExecutor{rows: rows}))
	assert.Equal(t, []string{"status", "bob", "Total"}, visible(g.head))
	assert.Equal(t, [][]string{{"few", "10", "10"}}, g.bodyVisible())
	assert.Equal(t, []string{"Total", "10", "12"}, visible(g.foot))
}

func TestPivotReadsPostgresShapedValues(t *testing.T) {
	g := parsePivot(t, renderPivot(t, statusAttrs("value", "sum", "field", "meta.hours"), &fakePivotExecutor{rows: []map[string]any{
		{"meta.status": []byte("open"), "meta.assignee": "ann", "count": "3", "sum_meta.hours": []byte("12.5")},
		{"meta.status": "open", "meta.assignee": "bob", "count": []byte("2"), "sum_meta.hours": "0.25"},
	}}))
	assert.Equal(t, [][]string{{"open", "12.5", "0.25", "12.75"}}, g.bodyVisible())
	g = parsePivot(t, renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: []map[string]any{
		row("open", "ann", "3"), row("open", "bob", []byte("2")),
	}}))
	assert.Equal(t, [][]string{{"open", "3", "2", "5"}}, g.bodyVisible())
}

func TestPivotSharesThePageBudgetAndCache(t *testing.T) {
	calls := 0
	base := func(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		calls++
		return &QueryResult{Mode: "aggregated", Rows: statusRows(), DrillDownLimit: 1000}, nil
	}
	exec := BudgetedExecutor(base, nil)
	reqCtx := WithQueryBudget(context.Background(), 1)
	tpl := `[pivot query="type = note" rows="meta.status" cols="meta.assignee"]`
	out := Process(reqCtx, tpl+tpl+`[pivot query="type = note" rows="meta.assignee" cols="meta.status"]`, pivotCtx(), nil, exec)
	assert.Equal(t, 1, calls, "an identical pivot is a cache hit")
	assert.Equal(t, 2, strings.Count(out, `<table`))
	assert.Contains(t, out, "inline query budget exceeded")
}

func TestPivotErrorsAndEmptyStates(t *testing.T) {
	boom := errors.New("no such column")
	assert.Equal(t, mrqlErrorHTML(boom, false), renderPivot(t, statusAttrs(), &fakePivotExecutor{err: boom}))
	unavailable := NewQueryUnavailable("")
	assert.Equal(t, unavailableComment(unavailable), renderPivot(t, statusAttrs(), &fakePivotExecutor{err: unavailable}))
	assert.Equal(t, `<div class="mrql-pivot"><p class="text-sm font-mono py-2 text-center">No results.</p></div>`, renderPivot(t, statusAttrs(), &fakePivotExecutor{}))
	assert.Equal(t, mrqlErrorHTML(errors.New("[pivot] unexpected result mode"), false), renderPivot(t, statusAttrs(), &fakePivotExecutor{mode: "flat"}))

	tpl := `[pivot query="type = note" rows="meta.status" cols="meta.assignee"]`
	assert.Equal(t, "<!-- mr:pivot unavailable in this context -->", Process(context.Background(), tpl, pivotCtx(), nil, nil))

	face := markReloadFace(context.Background())
	f := &fakePivotExecutor{rows: statusRows()}
	assert.Equal(t, shortcodeErrorMarker("pivot", "[pivot] cannot be used inside a [reload] button face"),
		RenderPivotShortcode(face, Shortcode{Name: "pivot", Attrs: statusAttrs()}, pivotCtx(), f.exec))
	assert.Empty(t, f.calls)

	// An exhausted output budget stops the render: no partial table.
	var many []map[string]any
	for i := 0; i < 50; i++ {
		many = append(many, row(fmt.Sprintf("s%02d", i), "ann", int64(1)))
	}
	f = &fakePivotExecutor{rows: many}
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 2000})
	out, err := ProcessChecked(bctx, tpl, pivotCtx(), nil, f.exec)
	assert.ErrorIs(t, err, ErrRenderBudgetExceeded)
	assert.Empty(t, out)
}

func TestPivotTruncatedSignedSumsArePartial(t *testing.T) {
	var rows []map[string]any
	for i := range pivotFetchLimit + 1 {
		r := row("open", fmt.Sprintf("a%04d", i), int64(1))
		r["sum_meta.hours"] = 1
		if i == pivotFetchLimit {
			r["sum_meta.hours"] = -100
		}
		rows = append(rows, r)
	}
	out := renderPivot(t, statusAttrs("value", "sum", "field", "meta.hours"), &fakePivotExecutor{rows: rows})
	g := parsePivot(t, out)
	for _, c := range []pivotCell{g.body[0][len(g.head)-1], g.foot[len(g.head)-1]} {
		assert.Equal(t, "5000 (partial)", c.visible)
		assert.Equal(t, "5000 (partial)", c.spoken)
	}
	assert.Equal(t, "1 (partial)", g.foot[1].visible)
	assert.Contains(t, out, "Totals are partial sums.")
	assert.NotContains(t, out, "or more</span>")
}

func TestPivotLinkQueryRejectsCarriageReturns(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		linkable    bool
	}{
		{"CR", "line1\rline2", false},
		{"CRLF", "line1\r\nline2", false},
		{"LF", "line1\nline2", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := parsePivot(t, renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: []map[string]any{row(tc.value, "ann", int64(1))}}))
			assert.Equal(t, "1", g.body[0][1].visible)
			assert.Equal(t, "1", g.body[0][2].visible)
			assert.Equal(t, tc.linkable, g.body[0][1].href != "", "cell link")
			assert.Equal(t, tc.linkable, g.body[0][2].href != "", "row total link")
			assert.Equal(t, `(type = note) AND meta.assignee = "ann" SCOPE 7`, linkQuery(t, g.foot[1].href))
			assert.Equal(t, `(type = note) SCOPE 7`, linkQuery(t, g.foot[2].href))
		})
	}

	t.Run("base filter", func(t *testing.T) {
		f := &fakePivotExecutor{rows: []map[string]any{row("open", "ann", int64(1))}}
		attrs := statusAttrs("query", "type = note AND name = \"line1\r\nline2\"")
		g := parsePivot(t, renderPivot(t, attrs, f))
		assert.Equal(t, [][]string{{"open", "1", "1"}}, g.bodyVisible())
		for _, c := range append(g.body[0][1:], g.foot[1:]...) {
			assert.Empty(t, c.href, "every link includes the base filter")
		}
	})
}

func TestPivotLinkQuerySizeIncludesScope(t *testing.T) {
	const filter = `(type = note) AND meta.status = "%s" AND meta.assignee = "ann" SCOPE 7`
	labelBytes := mrql.MaxQueryBytes - len(fmt.Sprintf(filter, ""))
	for _, extraBytes := range []int{0, 1} {
		label := strings.Repeat("x", labelBytes+extraBytes)
		g := parsePivot(t, renderPivot(t, statusAttrs(), &fakePivotExecutor{rows: []map[string]any{row(label, "ann", int64(1))}}))
		assert.Equal(t, "1", g.body[0][1].visible)
		if extraBytes == 0 {
			q := linkQuery(t, g.body[0][1].href)
			assert.Len(t, q, mrql.MaxQueryBytes)
			_, err := mrql.Parse(q)
			assert.NoError(t, err)
		} else {
			assert.Empty(t, g.body[0][1].href)
		}
		assert.NotEmpty(t, g.body[0][2].href, "shorter row total is still linkable")
	}
}
