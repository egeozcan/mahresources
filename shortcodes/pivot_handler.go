package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mahresources/mrql"
)

const (
	// pivotFetchLimit is the most (row, column) combinations one [pivot]
	// reads. The query asks for one more to learn that the cut-off was hit.
	pivotFetchLimit     = 5000
	pivotDefaultMaxRows = 20
	pivotMaxRowsCeiling = 50
	pivotDefaultMaxCols = 10
	pivotMaxColsCeiling = 20
	pivotMaxAliasBytes  = 63 // PostgreSQL truncates longer identifiers.
)

// pivotFieldRe is checked on every field name before it is composed into the
// query, so an attribute can name a field and nothing else.
var pivotFieldRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*(\.[A-Za-z0-9_]+)*$`)

// pivotDateBucketSuffixes is a copy of mrql's unexported dateBucketSuffixes;
// TestPivotDateBucketSuffixesMatchMRQL keeps the two equal.
var pivotDateBucketSuffixes = map[string]bool{"day": true, "week": true, "month": true, "year": true}

// pivotSumScalars are the entity columns value="sum" may add up.
var pivotSumScalars = map[string]bool{"fileSize": true, "width": true, "height": true}

// pivotIDFields hold row IDs: grouped by, they would head the table with bare
// numbers no reader can decode.
var pivotIDFields = map[string]bool{"id": true, "category": true, "noteType": true}

// pivotMultiValuedColumns are the relations an entity can have several of, so
// one entity lands in several buckets and adding those buckets up overcounts.
var pivotMultiValuedColumns = map[string]bool{"tags": true, "groups": true, "notes": true, "resources": true, "children": true}

const pivotUnsupportedHint = "use a scalar field, owner, groups, tags, a meta.* path, or a date bucket like created.year"

// pivotDimKind is how a dimension's bucket values are labelled and linked.
type pivotDimKind int

const (
	pivotKindMeta   pivotDimKind = iota // meta.<path>
	pivotKindText                       // a string field, or a relation matched by name
	pivotKindNumber                     // fileSize, width, height
	pivotKindDate                       // <timestamp>.day|week|month|year
	pivotKindShared                     // a note's shared flag, grouped as 1 or 0
)

// pivotDim is one validated dimension (rows or cols).
type pivotDim struct {
	field  string // as written, e.g. meta.status
	label  string // header and caption text
	kind   pivotDimKind
	multi  bool   // a multi-valued relation
	column string // canonical column, so group and groups count as one field
}

// pivotPlan is everything the renderer needs to read the result back.
type pivotPlan struct {
	filter     string // the author's query, as written
	rows, cols pivotDim
	sum        bool
	sumLabel   string
	caption    string
	rowKey     string // result-row column holding the row value
	colKey     string
	valueKey   string // count or sum_<field>
	maxRows    int
	maxCols    int
}

// pivotQuery validates a [pivot] opener's attributes and composes the one
// aggregated query it runs. It touches no database; the renderer and the
// linter both call it, so they report the same message.
func pivotQuery(attrs map[string]string) (string, pivotPlan, error) {
	filter := strings.TrimSpace(attrs["query"])
	rows := strings.TrimSpace(attrs["rows"])
	cols := strings.TrimSpace(attrs["cols"])
	if filter == "" || rows == "" || cols == "" {
		return "", pivotPlan{}, errors.New("[pivot] requires query, rows and cols")
	}
	value := strings.TrimSpace(attrs["value"])
	if value == "" {
		value = "count"
	}
	if value != "count" && value != "sum" {
		return "", pivotPlan{}, errors.New("[pivot] value must be count or sum")
	}
	field := strings.TrimSpace(attrs["field"])
	if value == "sum" && field == "" {
		return "", pivotPlan{}, errors.New(`[pivot] value="sum" requires field`)
	}

	parsed, err := mrql.Parse(filter)
	if err != nil {
		return "", pivotPlan{}, fmt.Errorf("[pivot] %w", err)
	}
	if parsed.Scope != nil || parsed.GroupBy != nil || len(parsed.OrderBy) > 0 || parsed.Limit >= 0 || parsed.Offset >= 0 {
		return "", pivotPlan{}, errors.New("[pivot] query must be a filter expression; use the scope attribute and leave GROUP BY/ORDER BY/LIMIT/OFFSET/SCOPE out")
	}
	if len(mrql.ListParams(parsed)) > 0 {
		return "", pivotPlan{}, errors.New("[pivot] query cannot use $parameters")
	}
	entity := mrql.ExtractEntityType(parsed)
	if entity == mrql.EntityUnspecified {
		return "", pivotPlan{}, errors.New("[pivot] query must name one entity type (type = resource, note or group)")
	}

	rowDim, err := classifyPivotDim("rows", rows, entity)
	if err != nil {
		return "", pivotPlan{}, err
	}
	colDim, err := classifyPivotDim("cols", cols, entity)
	if err != nil {
		return "", pivotPlan{}, err
	}
	if rowDim.column == colDim.column {
		return "", pivotPlan{}, errors.New("[pivot] rows and cols must be different fields")
	}
	sum := value == "sum"
	if sum && !pivotSumFieldOK(field, entity) {
		return "", pivotPlan{}, errors.New("[pivot] field must be a meta.* path, or fileSize, width or height on resources")
	}

	composed := "(" + filter + ") GROUP BY " + rows + ", " + cols + " COUNT()"
	if sum {
		composed += " SUM(" + field + ")"
	}
	composed += " ORDER BY count DESC, " + rows + " ASC, " + cols + " ASC"
	cq, err := mrql.Parse(composed)
	if err != nil {
		return "", pivotPlan{}, fmt.Errorf("[pivot] %w", err)
	}
	if err := mrql.Validate(cq); err != nil {
		return "", pivotPlan{}, fmt.Errorf("[pivot] %w", err)
	}
	keys := mrql.AggregatedColumns(cq)
	for _, key := range keys {
		if len(key) > pivotMaxAliasBytes {
			return "", pivotPlan{}, fmt.Errorf("[pivot] %q exceeds the %d-byte SQL alias limit; use a shorter field path", key, pivotMaxAliasBytes)
		}
	}

	rowDim.label = pivotLabel(attrs["rows-label"], rows)
	colDim.label = pivotLabel(attrs["cols-label"], cols)
	plan := pivotPlan{
		filter:   filter,
		rows:     rowDim,
		cols:     colDim,
		sum:      sum,
		rowKey:   keys[0],
		colKey:   keys[1],
		valueKey: "count",
		maxRows:  min(parseIntAttr(attrs["max-rows"], pivotDefaultMaxRows), pivotMaxRowsCeiling),
		maxCols:  min(parseIntAttr(attrs["max-cols"], pivotDefaultMaxCols), pivotMaxColsCeiling),
	}
	if sum {
		plan.valueKey = keys[3]
		plan.sumLabel = strings.TrimPrefix(field, "meta.")
	}
	plan.caption = strings.TrimSpace(attrs["caption"])
	if plan.caption == "" {
		if sum {
			plan.caption = "Sum of " + plan.sumLabel + " by " + rowDim.label + " and " + colDim.label
		} else {
			plan.caption = "Count by " + rowDim.label + " and " + colDim.label
		}
	}
	return composed, plan, nil
}

// pivotLabel is the author's label, or the field name without its meta.
// prefix when the label is unset or blank.
func pivotLabel(label, field string) string {
	if label = strings.TrimSpace(label); label != "" {
		return label
	}
	return strings.TrimPrefix(field, "meta.")
}

// classifyPivotDim accepts a meta path, a date bucket of a timestamp field, or
// a single-part field that is neither a timestamp nor an ID. attr names the
// attribute in the message.
func classifyPivotDim(attr, name string, entity mrql.EntityType) (pivotDim, error) {
	if !pivotFieldRe.MatchString(name) {
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q is not a field name", attr, name)
	}
	d := pivotDim{field: name, column: name}
	if strings.HasPrefix(name, "meta.") {
		d.kind = pivotKindMeta
		return d, nil
	}
	if base, suffix, dotted := strings.Cut(name, "."); dotted {
		if fd, ok := mrql.LookupField(entity, base); ok && fd.Type == mrql.FieldDateTime && pivotDateBucketSuffixes[suffix] {
			d.kind = pivotKindDate
			return d, nil
		}
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q is not supported; %s", attr, name, pivotUnsupportedHint)
	}
	fd, ok := mrql.LookupField(entity, name)
	switch {
	case !ok:
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q is not a %s field", attr, name, entity)
	case fd.Type == mrql.FieldDateTime:
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q is a timestamp; use a date bucket like %s.month or %s.year", attr, name, name, name)
	case pivotIDFields[name]:
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q holds IDs, which would head the table as bare numbers; pivot by a meta.* field instead, or filter by %s in query", attr, name, name)
	case name == "description" || name == "originalName" || name == "originalLocation" || name == "url":
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q is unbounded text; filter by it in query instead", attr, name)
	case name == "similarImages":
		return pivotDim{}, fmt.Errorf("[pivot] %s: %q is not supported; %s", attr, name, pivotUnsupportedHint)
	}
	d.column = fd.Column
	switch {
	case name == "shared":
		d.kind = pivotKindShared
	case fd.Type == mrql.FieldNumber:
		d.kind = pivotKindNumber
	default:
		d.kind = pivotKindText
		d.multi = fd.Type == mrql.FieldRelation && pivotMultiValuedColumns[fd.Column]
	}
	return d, nil
}

func pivotSumFieldOK(field string, entity mrql.EntityType) bool {
	if !pivotFieldRe.MatchString(field) {
		return false
	}
	if strings.HasPrefix(field, "meta.") {
		return true
	}
	fd, ok := mrql.LookupField(entity, field)
	return pivotSumScalars[field] && ok && fd.Type == mrql.FieldNumber
}

// pivotBucket is one row or column of the table: every raw value whose text
// matches ignoring ASCII case, the same rule MRQL's = applies to strings.
type pivotBucket struct {
	label string
	// literal is the MRQL value a link compares the dimension with, or "" when
	// no link can count exactly what the bucket holds.
	literal string
	none    bool
	total   float64
	order   int // first-seen position, the bucket's index into the cells
}

type pivotAxis struct {
	dim            pivotDim
	buckets        []*pivotBucket
	byKey          map[string]*pivotBucket
	numericMetaKey bool
}

// pivotRawKind says what a raw group-by value was before it became text.
type pivotRawKind int

const (
	pivotRawText pivotRawKind = iota
	pivotRawNone              // SQL NULL
	pivotRawInteger
	pivotRawFloat
	pivotRawOpaque // a type no current dimension produces
)

func pivotKeyText(raw any) (string, pivotRawKind) {
	switch x := raw.(type) {
	case nil:
		return "", pivotRawNone
	case string:
		return x, pivotRawText
	case []byte:
		return string(x), pivotRawText
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return fmt.Sprint(x), pivotRawInteger
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64), pivotRawFloat
	default:
		return fmt.Sprint(x), pivotRawOpaque
	}
}

// mrqlStringLiteral writes s as a double-quoted MRQL string, which reads back
// as exactly s: the lexer understands \" and \\.
func mrqlStringLiteral(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

func (a *pivotAxis) bucket(raw any) *pivotBucket {
	text, kind := pivotKeyText(raw)
	// SQLite can group INTEGER and REAL together but compare them as different
	// strings in a drill-down. Even an integer key can therefore hide a REAL
	// that matches a different bucket's link. PostgreSQL returns Meta as text.
	if a.dim.kind == pivotKindMeta && (kind == pivotRawInteger || kind == pivotRawFloat) {
		a.numericMetaKey = true
	}
	// asciiLower folds A-Z only, as SQLite's LOWER does.
	key := "v" + asciiLower(text)
	if kind == pivotRawNone {
		key = "n"
	}
	b, ok := a.byKey[key]
	if !ok {
		b = &pivotBucket{order: len(a.buckets), label: text}
		switch {
		case kind == pivotRawNone:
			b.label, b.none = "(none)", true
		case a.dim.kind == pivotKindShared:
			// The group-by expression is 1 or 0, never the share token.
			switch text {
			case "1":
				b.label, b.literal = "yes", "true"
			case "0":
				b.label, b.literal = "no", "false"
			}
		case a.dim.kind == pivotKindNumber:
			if _, err := strconv.ParseFloat(text, 64); err == nil {
				b.literal = text
			}
		case a.dim.kind == pivotKindMeta, a.dim.kind == pivotKindText:
			b.literal = mrqlStringLiteral(text)
		}
		if text == "" && !b.none {
			b.label = "(empty)"
		}
		a.byKey[key] = b
		a.buckets = append(a.buckets, b)
	}
	if kind == pivotRawOpaque {
		b.literal = ""
	}
	return b
}

// shown ranks the buckets by total, then label, keeps the first max, and
// returns them in display order: chronological for a date bucket, rank order
// otherwise, with the none-bucket last either way.
func (a *pivotAxis) shown(max int) (shown []*pivotBucket, hidden int) {
	ranked := append([]*pivotBucket(nil), a.buckets...)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].total != ranked[j].total {
			return ranked[i].total > ranked[j].total
		}
		return ranked[i].label < ranked[j].label
	})
	if len(ranked) > max {
		hidden = len(ranked) - max
		ranked = ranked[:max]
	}
	if a.dim.kind == pivotKindDate {
		sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].label < ranked[j].label })
	}
	sort.SliceStable(ranked, func(i, j int) bool { return !ranked[i].none && ranked[j].none })
	return ranked, hidden
}

// pivotTable is the aggregated result, before caps.
type pivotTable struct {
	rows, cols pivotAxis
	cells      map[[2]int]float64
	grand      float64
	truncated  bool
}

// pivotValue reads a count or sum in the shapes the two dialects return it
// in; NULL (a sum over no numbers) and anything unreadable count as 0.
func pivotValue(v any) float64 {
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if v == nil {
		return 0
	}
	f, ok := toFloat(v)
	if !ok || !calcFinite(f) {
		return 0
	}
	return f
}

func buildPivotTable(plan pivotPlan, rows []map[string]any, truncated bool) pivotTable {
	t := pivotTable{
		rows:      pivotAxis{dim: plan.rows, byKey: map[string]*pivotBucket{}},
		cols:      pivotAxis{dim: plan.cols, byKey: map[string]*pivotBucket{}},
		cells:     map[[2]int]float64{},
		truncated: truncated,
	}
	for _, row := range rows {
		rb := t.rows.bucket(row[plan.rowKey])
		cb := t.cols.bucket(row[plan.colKey])
		v := pivotValue(row[plan.valueKey])
		k := [2]int{rb.order, cb.order}
		if _, merged := t.cells[k]; merged {
			// Two rows in one cell are distinct values that compare equal. On a
			// multi-valued relation they can be two same-named entities on one
			// item, which the cell counts twice and a link once.
			if plan.rows.multi {
				rb.literal = ""
			}
			if plan.cols.multi {
				cb.literal = ""
			}
		}
		t.cells[k] += v
		rb.total += v
		cb.total += v
		t.grand += v
	}
	return t
}

// pivotNumber writes a cell value as [calc] writes a number: plain decimal,
// at most 10 fraction digits, no trailing zeros.
func pivotNumber(f float64) string {
	s, _ := calcFormat(f, -1, "number")
	return s
}

// RenderPivotShortcode renders [pivot]: a cross-tab of two fields of an MRQL
// filter's results, with a count or sum in each cell and row, column and grand
// totals. In count mode linkable values drill down to the matching MRQL list.
//
// It runs one aggregated query through the executor, so the principal's
// subtree confinement, the page query budget and its cache apply as they do to
// [mrql].
func RenderPivotShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, executor QueryExecutor) string {
	// A <button> takes phrasing content only; a table is not.
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("pivot", "[pivot] cannot be used inside a [reload] button face")
	}
	composed, plan, err := pivotQuery(sc.Attrs)
	if err != nil {
		return mrqlErrorHTML(err, false)
	}
	scope := resolveScopeKeyword(sc.Attrs["scope"], ctx)

	m := meterFrom(reqCtx)
	result, err := executor(reqCtx, composed, QueryOptions{Limit: pivotFetchLimit + 1, ScopeGroupID: scope})
	if err != nil {
		if errors.Is(err, ErrQueryUnavailable) {
			return unavailableComment(err)
		}
		return mrqlErrorHTML(err, false)
	}
	if !m.alive() {
		return ""
	}
	if result != nil && result.Mode != "aggregated" {
		return mrqlErrorHTML(errors.New("[pivot] unexpected result mode"), false)
	}
	if result == nil || len(result.Rows) == 0 {
		return `<div class="mrql-pivot"><p class="text-sm font-mono py-2 text-center">No results.</p></div>`
	}
	rows := result.Rows
	truncated := len(rows) > pivotFetchLimit
	if truncated {
		rows = rows[:pivotFetchLimit]
	}
	t := buildPivotTable(plan, rows, truncated)

	href := func(filter string) string {
		if scope != 0 && scope != mrql.UnresolvedScopeSentinel {
			filter = mrql.InsertScopeClause(filter, scope)
		}
		if len(filter) > mrql.MaxQueryBytes {
			return ""
		}
		return "/mrql?q=" + url.QueryEscape(filter)
	}
	return writePivotHTML(m, plan, t, result.DrillDownLimit, withinTooltip(reqCtx), href)
}

// pivotFocus is the link and region focus ring, as [similar] and [calendar]
// draw it.
const pivotFocus = similarFocus

const (
	pivotTh       = `px-3 py-2 font-semibold whitespace-nowrap`
	pivotTd       = `px-3 py-2 text-right tabular-nums`
	pivotLinkAttr = `class="text-amber-700 hover:text-amber-900 underline rounded-sm ` + pivotFocus + `"`
)

// writePivotHTML writes the table under the render meter: every row is
// reserved right after it is formatted, so a stop holds at most one row past
// the budget.
func writePivotHTML(m *renderMeter, plan pivotPlan, t pivotTable, drillDownLimit int, tooltip bool, href func(string) string) string {
	shownRows, hiddenRows := t.rows.shown(plan.maxRows)
	shownCols, hiddenCols := t.cols.shown(plan.maxCols)
	showTotalCol := !plan.cols.multi
	showFooter := !plan.rows.multi
	links := !plan.sum && !t.truncated && !tooltip && drillDownLimit > 0
	rowLinks, colLinks := links && !t.rows.numericMetaKey, links && !t.cols.numericMetaKey
	esc := html.EscapeString

	base := "(" + plan.filter + ")"
	clause := func(d pivotDim, b *pivotBucket) string { return " AND " + d.field + " = " + b.literal }
	// value writes one number, linking only if the destination can list it.
	value := func(b *strings.Builder, n float64, filter, name string) {
		text := pivotNumber(n)
		if links && filter != "" && n <= float64(drillDownLimit) {
			if url := href(filter); url != "" {
				fmt.Fprintf(b, `<a href="%s" %s aria-label="%s">%s</a>`, esc(url), pivotLinkAttr, esc(text+": "+name), text)
				return
			}
		}
		b.WriteString(text)
	}
	total := func(b *strings.Builder, n float64, filter, name string) {
		if !t.truncated {
			value(b, n, filter, name)
		} else if plan.sum {
			b.WriteString(pivotNumber(n) + " (partial)")
		} else {
			b.WriteString(pivotNumber(n) + `<span aria-hidden="true">+</span><span class="sr-only"> or more</span>`)
		}
	}

	var b strings.Builder
	c := charged{m: m}
	caption := esc(plan.caption)
	tabindex := 0
	if tooltip {
		// Omission alone can leave a scroll container in the tab order.
		tabindex = -1
	}
	// relative keeps the sr-only spans inside the scroll container, so they
	// cannot widen the page.
	fmt.Fprintf(&b, `<div class="mrql-pivot my-2"><div class="relative overflow-x-auto rounded-md %s" role="region" aria-label="%s" tabindex="%d">`, pivotFocus, caption, tabindex)
	fmt.Fprintf(&b, `<table class="min-w-full text-sm border border-stone-200"><caption class="text-left text-sm font-semibold px-1 pb-2">%s</caption>`, caption)
	fmt.Fprintf(&b, `<thead class="bg-stone-100 text-stone-700"><tr><th scope="col" class="%s text-left">%s</th>`, pivotTh, esc(plan.rows.label))
	for _, col := range shownCols {
		fmt.Fprintf(&b, `<th scope="col" class="%s text-right">%s</th>`, pivotTh, esc(col.label))
	}
	if showTotalCol {
		fmt.Fprintf(&b, `<th scope="col" class="%s text-right">Total</th>`, pivotTh)
	}
	b.WriteString(`</tr></thead><tbody class="bg-white text-stone-900 divide-y divide-stone-100">`)
	if !c.flush(&b) {
		return ""
	}

	for _, row := range shownRows {
		if !m.tick() {
			return ""
		}
		fmt.Fprintf(&b, `<tr><th scope="row" class="%s text-left">%s</th>`, pivotTh, esc(row.label))
		for _, col := range shownCols {
			if !m.tick() {
				return ""
			}
			n, present := t.cells[[2]int{row.order, col.order}]
			switch {
			case present:
				b.WriteString(`<td class="` + pivotTd + `">`)
				filter := ""
				if rowLinks && colLinks && row.literal != "" && col.literal != "" {
					filter = base + clause(plan.rows, row) + clause(plan.cols, col)
				}
				value(&b, n, filter, plan.rows.label+" "+row.label+", "+plan.cols.label+" "+col.label)
			case t.truncated:
				b.WriteString(`<td class="` + pivotTd + ` text-stone-500"><span aria-hidden="true">—</span><span class="sr-only">not counted</span>`)
			default:
				b.WriteString(`<td class="` + pivotTd + ` text-stone-500">0`)
			}
			b.WriteString(`</td>`)
		}
		if showTotalCol {
			b.WriteString(`<td class="` + pivotTd + ` font-semibold">`)
			filter := ""
			if rowLinks && row.literal != "" {
				filter = base + clause(plan.rows, row)
			}
			total(&b, row.total, filter, "total for "+plan.rows.label+" "+row.label)
			b.WriteString(`</td>`)
		}
		b.WriteString(`</tr>`)
		if !c.flush(&b) {
			return ""
		}
	}
	b.WriteString(`</tbody>`)

	if showFooter {
		fmt.Fprintf(&b, `<tfoot class="bg-stone-50 text-stone-700"><tr><th scope="row" class="%s text-left">Total</th>`, pivotTh)
		for _, col := range shownCols {
			if !m.tick() {
				return ""
			}
			b.WriteString(`<td class="` + pivotTd + ` font-semibold">`)
			filter := ""
			if colLinks && col.literal != "" {
				filter = base + clause(plan.cols, col)
			}
			total(&b, col.total, filter, "total for "+plan.cols.label+" "+col.label)
			b.WriteString(`</td>`)
		}
		if showTotalCol {
			b.WriteString(`<td class="` + pivotTd + ` font-semibold">`)
			total(&b, t.grand, base, "total of all")
			b.WriteString(`</td>`)
		}
		b.WriteString(`</tr></tfoot>`)
	}
	b.WriteString(`</table></div>`)

	if hiddenRows > 0 || hiddenCols > 0 {
		var parts []string
		if hiddenRows > 0 {
			parts = append(parts, pivotMore(hiddenRows, plan.rows.label))
		}
		if hiddenCols > 0 {
			parts = append(parts, pivotMore(hiddenCols, plan.cols.label))
		}
		verb := " are"
		if len(parts) == 1 && (hiddenRows+hiddenCols) == 1 {
			verb = " is"
		}
		note := strings.Join(parts, " and ") + verb + " not shown"
		if (hiddenRows > 0 && showFooter) || (hiddenCols > 0 && showTotalCol) {
			note += "; the totals include them"
		}
		fmt.Fprintf(&b, `<p class="text-xs mt-1">%s.</p>`, esc(note))
	}
	if t.truncated {
		fmt.Fprintf(&b, `<p class="text-xs mt-1">Only the %d combinations with the most items were counted. Cells marked — were not counted. `, pivotFetchLimit)
		if plan.sum {
			b.WriteString("Totals are partial sums. ")
		} else {
			b.WriteString("Totals marked + are incomplete. ")
		}
		b.WriteString("Narrow the query.</p>")
	}
	b.WriteString(`</div>`)
	if !c.flush(&b) {
		return ""
	}
	return b.String()
}

// pivotMore is "12 more status values" or "1 more status value".
func pivotMore(n int, label string) string {
	if n == 1 {
		return "1 more " + label + " value"
	}
	return strconv.Itoa(n) + " more " + label + " values"
}

// lintPivot reports the renderer's own refusals, word for word, at query=.
// A missing query, rows or cols is left to the required-attribute check, so
// it is reported once.
func lintPivot(w *schemaWork, tk token, reloadSpans [][2]int, add func(start, end int, severity, msg string)) {
	if insideSpans(tk.start, reloadSpans) {
		add(tk.start, tk.end, SeverityError, "[pivot] cannot be used inside a [reload] button face")
	}
	for _, required := range []string{"query", "rows", "cols"} {
		if strings.TrimSpace(tk.attrs[required]) == "" {
			return
		}
	}
	if _, _, err := pivotQuery(tk.attrs); err != nil {
		start, end := attrOffsetW(w, tk, "query")
		add(start, end, SeverityError, err.Error())
	}
	if strings.TrimSpace(tk.attrs["field"]) != "" && strings.TrimSpace(tk.attrs["value"]) != "sum" {
		add(tk.start, tk.end, SeverityWarning, `[pivot] field is read only with value="sum"; it is ignored`)
	}
	if scope, set := tk.attrs["scope"]; set && !pivotScopeKnown(scope) {
		add(tk.start, tk.end, SeverityWarning, `[pivot] scope must be entity, parent, root, global or a group ID; "`+scope+`" counts in the entity's own group`)
	}
}

// pivotScopeKnown reports whether resolveScopeKeyword reads scope as asked
// rather than falling back to the entity's own group.
func pivotScopeKnown(scope string) bool {
	switch scope {
	case "", "entity", "parent", "root", "global":
		return true
	}
	_, err := strconv.ParseUint(scope, 10, 64)
	return err == nil
}
