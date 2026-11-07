package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func allocDuring(f func()) int64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	f()
	runtime.ReadMemStats(&after)
	return int64(after.TotalAlloc - before.TotalAlloc)
}

// P1-4: every sentinel in a URL attribute used to unescape and retain a growing
// prefix, quadratic in the value.
func TestLintURLPrefixCopiesAreBounded(t *testing.T) {
	in := `<a href="` + strings.Repeat(`&#65;[meta inline="true" path="x"]`, 10000) + `">x</a>`
	var issues []LintIssue
	grown := allocDuring(func() { issues = Lint(in, LintOptions{Known: KnownFromBuiltins()}) })
	assert.Less(t, grown, int64(600<<20), "quadratic prefix copies were ~1.7 GB")
	require.NotEmpty(t, issues)
}

// P1-5: tokens are bounded while they are collected, not after.
func TestLintTokenCollectionIsBounded(t *testing.T) {
	in := strings.Repeat("[meta]", 160_000)
	var issues []LintIssue
	grown := allocDuring(func() { issues = Lint(in, LintOptions{Known: KnownFromBuiltins()}) })
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
	assert.Less(t, grown, int64(64<<20))

	loose := strings.Repeat("[x", 500_000)
	issues = Lint(loose, LintOptions{})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
}

// P2-6: built-in layouts reserve each row; a few huge rows stop the render.
func TestBuiltinMRQLLayoutsChargeRows(t *testing.T) {
	big := strings.Repeat("n", 200<<10)
	items := make([]QueryResultItem, 6)
	for i := range items {
		items[i] = QueryResultItem{EntityType: "group", EntityID: uint(i + 1), Entity: testEntity{ID: uint(i + 1), Name: big}, Meta: []byte(`{}`)}
	}
	exec := func(ctx context.Context, q string, o QueryOptions) (*QueryResult, error) {
		return &QueryResult{Mode: "flat", Items: items}, nil
	}
	for _, format := range []string{"", "table", "list", "compact"} {
		in := `[mrql query="q" format="` + format + `"]`
		grown := allocDuring(func() {
			_, err := ProcessChecked(budgeted(), in, MetaShortcodeContext{}, nil, exec)
			assert.ErrorIs(t, err, ErrRenderBudgetExceeded, format)
		})
		assert.Less(t, grown, int64(16<<20), format)
	}
	rows := []map[string]any{{"a": big}, {"a": big}, {"a": big}, {"a": big}}
	execAgg := func(ctx context.Context, q string, o QueryOptions) (*QueryResult, error) {
		return &QueryResult{Mode: "aggregated", Rows: rows}, nil
	}
	_, err := ProcessChecked(budgeted(), `[mrql query="q"]`, MetaShortcodeContext{}, nil, execAgg)
	assert.ErrorIs(t, err, ErrRenderBudgetExceeded)
}

// P2-8: after exhaustion the schema lint builds no more child paths.
func TestSchemaLintStopsBuildingPathsAfterExhaustion(t *testing.T) {
	name := strings.Repeat("k", 10000)
	all := make([]any, 3000)
	for i := range all {
		all[i] = map[string]any{}
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{name: map[string]any{"allOf": all}}})
	var issues []LintIssue
	grown := allocDuring(func() { issues = LintMetaSchema(string(b), MetaSchemaLintOptions{}) })
	require.Len(t, issues, 1)
	assert.Less(t, grown, int64(8<<20))
}

// P2-9: escaping lengthens a description far past its raw length.
func TestSchemaByteBudgetCountsEscaping(t *testing.T) {
	desc := strings.Repeat("&", 10000)
	schema := fanSchema(10, 2, desc) // 100 leaves of 10,000 ampersands: ~6 MB once escaped
	_, err := ProcessChecked(budgeted(), `[meta path="root"]`, schemaCtx(schema), nil, nil)
	assert.ErrorIs(t, err, ErrRenderBudgetExceeded)
	assert.Equal(t, 6*10000+10, escapedSize(desc))
}

// P2-14: a cancelled context is seen by operations too small to reach the
// amortized check.
func TestSmallInputsSeeCancellation(t *testing.T) {
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	issues := Lint("plain", LintOptions{Context: cctx})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
	issues = LintMetaSchema("{}", MetaSchemaLintOptions{Context: cctx})
	require.Len(t, issues, 1)
}

// P2-15: a cancellation that stops schema expansion keeps its typed cause.
func TestSchemaCancellationKeepsTypedCause(t *testing.T) {
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := schemaCtx(fanSchema(20, 3, "d"))
	ctx.meter = newRenderMeter(cctx)
	_ = ctx.schemaSlice("root")
	err := ctx.meter.Err()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	assert.True(t, errors.Is(err, context.Canceled))
}

// P2-10: numbered conditions share one param collection (behavior unchanged).
func TestConditionalNumberedMRQLSharesParams(t *testing.T) {
	var seen []map[string]string
	exec := func(ctx context.Context, q string, o QueryOptions) (*QueryResult, error) {
		seen = append(seen, o.Params)
		return &QueryResult{Mode: "flat"}, nil
	}
	in := `[conditional mrql="type = group AND name = $t" gte="0" mrql2="type = group AND name = $t" gte2="0" param-t="x"]Y[/conditional]`
	Process(context.Background(), in, MetaShortcodeContext{}, nil, exec)
	require.Len(t, seen, 2)
	assert.Equal(t, map[string]string{"t": "x"}, seen[0])
	assert.Equal(t, map[string]string{"t": "x"}, seen[1])
	// One collection: both numbered sources receive the same map.
	assert.Equal(t, reflect.ValueOf(seen[0]).Pointer(), reflect.ValueOf(seen[1]).Pointer())

	// Lint: the options for every numbered query carry one shared map.
	var got []map[string]string
	opts := LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: func(q string, o MRQLCheckOptions) error {
		got = append(got, o.Params)
		return nil
	}}
	Lint(in, opts)
	require.Len(t, got, 2)
	assert.Equal(t, reflect.ValueOf(got[0]).Pointer(), reflect.ValueOf(got[1]).Pointer())
	_ = fmt.Sprint
}

// Round 2: aggregate attribute entries are reserved before the maps are built.
func TestLintAggregateAttributeRetentionIsBounded(t *testing.T) {
	var sb strings.Builder
	for i := 0; i < 140; i++ { // ~1 MB, under MaxLintSourceBytes: 140,000 entries past the 100,000 allowance
		sb.WriteString("[meta")
		for j := 0; j < 1000; j++ {
			fmt.Fprintf(&sb, " a%d=1", j)
		}
		sb.WriteString("]")
	}
	in := sb.String()
	var issues []LintIssue
	grown := allocDuring(func() { issues = Lint(in, LintOptions{Known: KnownFromBuiltins()}) })
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
	assert.Less(t, grown, int64(60<<20))
}

// Round 2: the estimate is an upper bound of the real JSON, and of the real
// JSON once HTML-escaped into an attribute, for the shapes that lengthen.
func TestEscapedSizeBoundsRealSerialization(t *testing.T) {
	samples := []string{
		"", "plain", `quo"te`, `back\slash`, "it's", "<b>&amp;</b>", "tab\tnl\n\x00\x1f",
		"line sep ", strings.Repeat(" ", 100), "café 世界 \U0001F600",
		"bad\xffutf8\xc3", strings.Repeat("&", 1000), "mixed   \" < \xff \U0001F600",
	}
	for _, s := range samples {
		real, _ := json.Marshal(s)
		embedded := len(htmlEscape(string(real)))
		assert.GreaterOrEqual(t, escapedSize(s), embedded, "%q", s)
	}
}

func htmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return r.Replace(s)
}

func TestValueBytesBoundsRealSerializationOfValues(t *testing.T) {
	v := map[string]any{
		"description": "a b \"q\" <t>",
		"n":           -1.2345678901234567e-308,
		"i":           float64(12),
		"b":           true,
		"nil":         nil,
		"list":        []any{"x", 1.5, map[string]any{"k ": "&"}},
		"k\"ey":       "v",
	}
	real, _ := json.Marshal(v)
	w := newSchemaWork(nil)
	require.True(t, w.valueBytes(v))
	assert.GreaterOrEqual(t, w.bytes, len(htmlEscape(string(real))))
}

// Round 2 (P2-10): admission precedes marshaling. Called directly, the schema
// work is exhausted and nothing is serialized.
func TestExtractSchemaSliceRefusesBeforeMarshaling(t *testing.T) {
	for name, desc := range map[string]string{
		"ampersands": strings.Repeat("&", 10000),
		"unicode":    strings.Repeat(" ", 3000),
	} {
		schema := fanSchema(10, 2, desc) // 100 leaves
		w := newSchemaWork(nil)
		var out string
		grown := allocDuring(func() { out = extractSchemaSliceW(w, schema, "root", nil) })
		assert.Empty(t, out, name)
		assert.ErrorIs(t, w.Err(), errSchemaTooComplex, name)
		assert.Less(t, grown, int64(4<<20), "%s: a marshal of the expansion would allocate several MB", name)
	}
	// Repeated numeric defaults: 8 bytes each was the old charge.
	props := map[string]any{}
	for i := 0; i < 40000; i++ {
		props[fmt.Sprintf("p%d", i)] = map[string]any{"type": "number", "default": -1.2345678901234567e-308}
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{"root": map[string]any{"type": "object", "properties": props}}})
	w := newSchemaWork(nil)
	assert.Empty(t, extractSchemaSliceW(w, string(b), "root", nil))
	assert.Error(t, w.Err())
}

// Round 2: the cache-key reservation covers the quoted size.
func TestCacheKeyReservationCoversQuotedSize(t *testing.T) {
	nul := strings.Repeat("\x00", 1<<20)
	m := newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 1 << 20}))
	opts := QueryOptions{SavedName: "s\x00", Params: map[string]string{"p\x01": nul}}
	key, ok := budgetCacheKeyMetered(m, "q\x00\x00", opts)
	require.True(t, ok)
	assert.GreaterOrEqual(t, m.keyWork, int64(len(key)), "reserved less than the key it built")

	// With 4 MiB reserved per key, the 8 MiB allowance ends on the third call.
	m = newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 1 << 20}))
	calls := 0
	for ; calls < 10; calls++ {
		if _, ok := budgetCacheKeyMetered(m, "q", opts); !ok {
			break
		}
	}
	assert.LessOrEqual(t, calls, 2)
}

// Round 2 (P2-9): the layout helpers called directly with a meter stop after
// the first row that crosses the allowance; the old code formatted all fifty.
func TestLayoutHelpersStopAtTheFirstOverBudgetRow(t *testing.T) {
	big := strings.Repeat("n", 2<<20)
	items := make([]QueryResultItem, 50)
	for i := range items {
		items[i] = QueryResultItem{EntityType: "group", EntityID: uint(i + 1), Entity: testEntity{ID: uint(i + 1), Name: big}}
	}
	rows := make([]map[string]any, 50)
	for i := range rows {
		rows[i] = map[string]any{"a": big, "b": big}
	}
	layouts := map[string]func(m *renderMeter) string{
		"default":    func(m *renderMeter) string { return renderFlatDefault(m, items) },
		"table":      func(m *renderMeter) string { return renderFlatTable(m, items) },
		"list":       func(m *renderMeter) string { return renderFlatList(m, items) },
		"compact":    func(m *renderMeter) string { return renderFlatCompact(m, items) },
		"aggregated": func(m *renderMeter) string { return renderAggregatedTable(m, rows) },
	}
	for name, render := range layouts {
		m := newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 256 << 10, ParsedSourceBytes: 1 << 20}))
		var out string
		grown := allocDuring(func() { out = render(m) })
		assert.Empty(t, out, name)
		assert.ErrorIs(t, m.Err(), ErrRenderBudgetExceeded, name)
		assert.Less(t, grown, int64(40<<20), "%s formatted past the first over-budget row", name)
	}
}

// Round 3: numbers are measured. -0.0000010000000000000002 is 25 bytes.
func TestValueBytesMeasuresNumbers(t *testing.T) {
	for _, f := range []float64{-0.0000010000000000000002, -1.2345678901234567e-308, 1e21, 0, 12, 1.5, -123456789.123456789, 1e-7, 5e-324} {
		real, _ := json.Marshal(f)
		w := newSchemaWork(nil)
		require.True(t, w.valueBytes(f))
		assert.GreaterOrEqual(t, w.bytes, len(real), "%v", f)
	}
}

// Repeated numeric data reaches the byte allowance without first exhausting the
// node-work allowance: 100 properties reference one definition holding 450
// numbers of 25 bytes (~1.1 MB once expanded).
func TestExtractSchemaSliceByteAdmissionOnNumbers(t *testing.T) {
	nums := make([]any, 450)
	for i := range nums {
		nums[i] = -0.0000010000000000000002
	}
	props := map[string]any{}
	for i := 0; i < 100; i++ {
		props[fmt.Sprintf("p%d", i)] = map[string]any{"$ref": "#/$defs/n"}
	}
	b, _ := json.Marshal(map[string]any{
		"type":       "object",
		"$defs":      map[string]any{"n": map[string]any{"type": "array", "default": nums}},
		"properties": map[string]any{"root": map[string]any{"type": "object", "properties": props}},
	})
	w := newSchemaWork(nil)
	assert.Empty(t, extractSchemaSliceW(w, string(b), "root", nil))
	assert.ErrorIs(t, w.Err(), errSchemaTooComplex)
	assert.Less(t, w.units, schemaWorkUnits, "must stop on bytes, not on node work")
}

// The fixed part of a cache key is measured at the numeric extrema.
func TestCacheKeyReservationCoversNumericExtrema(t *testing.T) {
	m := newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 1 << 20}))
	opts := QueryOptions{ScopeGroupID: ^uint(0), Limit: int(^uint(0) >> 1), Buckets: int(^uint(0) >> 1)}
	key, ok := budgetCacheKeyMetered(m, "q", opts)
	require.True(t, ok)
	assert.GreaterOrEqual(t, m.keyWork, int64(len(key)))
}

// Round 3: bucket headers are reserved as they are written; fifty buckets with
// a 2 MiB key value used to retain ~100 MiB of header HTML before any charge.
func TestBucketedRenderingStopsAtTheFirstOverBudgetHeader(t *testing.T) {
	big := strings.Repeat("k", 2<<20)
	groups := make([]QueryResultGroup, 50)
	for i := range groups {
		groups[i] = QueryResultGroup{Key: map[string]any{"cat": big}, Items: []QueryResultItem{{EntityType: "group", EntityID: 1}}}
	}
	rctx := context.WithValue(budgeted(), renderMeterKey{}, newRenderMeter(budgeted()))
	var out string
	grown := allocDuring(func() { out = renderBucketed(rctx, groups, "", MetaShortcodeContext{}, nil, nil, 0) })
	assert.Empty(t, out)
	assert.ErrorIs(t, meterFrom(rctx).Err(), ErrRenderBudgetExceeded)
	assert.Less(t, grown, int64(24<<20))

	// Small buckets render exactly as before.
	small := []QueryResultGroup{{Key: map[string]any{"cat": "a<b"}, Items: []QueryResultItem{{EntityType: "group", EntityID: 2}}}}
	got := renderBucketed(context.Background(), small, "", MetaShortcodeContext{}, nil, nil, 0)
	assert.Contains(t, got, `<span class="font-semibold text-stone-700">a&lt;b</span>`)
	assert.Contains(t, got, "1 items")
}

func elseifBomb(dividers, attrs int) string {
	var sb strings.Builder
	sb.WriteString(`[conditional path="a" eq="1" mrql="type = group"]Y`)
	for i := 0; i < dividers; i++ {
		sb.WriteString("[elseif path=\"a\" eq=\"2\"")
		for j := 0; j < attrs; j++ {
			fmt.Fprintf(&sb, " b%d=1", j)
		}
		sb.WriteString("]Z")
	}
	sb.WriteString(`[/conditional]`)
	return sb.String()
}

// Round 3: [elseif] is a divider, not a token, so its attribute maps were
// outside the aggregate reservation. 1,500 dividers x 1,000 attributes retained
// 1.5 million entries.
func TestElseifAttributeMapsAreAdmittedInLintAndRender(t *testing.T) {
	in := elseifBomb(150, 900)

	var issues []LintIssue
	opts := LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: func(string, MRQLCheckOptions) error { return nil }}
	grown := allocDuring(func() { issues = Lint(in, opts) })
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
	assert.Less(t, grown, int64(150<<20))

	grown = allocDuring(func() {
		_, err := ProcessChecked(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 16 << 20}), in, MetaShortcodeContext{Meta: []byte(`{"a":"1"}`)}, nil, nil)
		assert.ErrorIs(t, err, ErrRenderBudgetExceeded)
	})
	assert.Less(t, grown, int64(150<<20))

	// An ordinary [elseif] chain is unchanged.
	got := Process(context.Background(), `[conditional path="a" eq="0"]A[elseif path="a" eq="1"]B[else]C[/conditional]`, MetaShortcodeContext{Meta: []byte(`{"a":"1"}`)}, nil, nil)
	assert.Equal(t, "B", got)
}

// Cancellation is seen while the dividers are collected.
func TestElseifSplitStopsOnCancellation(t *testing.T) {
	cc := &errCountingCtx{Context: context.Background(), after: 3}
	m := newRenderMeter(WithRenderBudget(cc, RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 16 << 20}))
	inner := strings.Repeat(`x[elseif path="a" eq="1"]`, 20000)
	splitBranchesM(m, inner)
	assert.ErrorIs(t, m.Err(), context.Canceled)
	assert.Less(t, cc.calls, 200)
}

// Round 4: the lint source cap, and the helper reservations beneath it.
func TestLintRefusesSourceOverTheCap(t *testing.T) {
	issues := Lint(strings.Repeat("a", MaxLintSourceBytes+1), LintOptions{})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too large")
	assert.Empty(t, Lint(strings.Repeat("a", MaxLintSourceBytes), LintOptions{}))
	issues = LintMetaSchema(`{"description":"`+strings.Repeat("a", MaxLintSourceBytes)+`"}`, MetaSchemaLintOptions{})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too large")
}

func limitedMeter(ctx context.Context) *renderMeter {
	return &renderMeter{ctx: ctx, budget: RenderBudget{ParsedSourceBytes: 1 << 62}}
}

// An [mrql] body of plain text: the origin array is 8 bytes a byte, reserved
// first. Under a tight scratch allowance the helper allocates nothing.
func TestMRQLSlotSpansReserveOriginStorage(t *testing.T) {
	inner := strings.Repeat("x", 1<<20) + "[header]h[/header]"
	m := limitedMeter(nil)
	m.scratch = maxScratchBytes - 1<<20 // leaves 1 MiB; the origin array needs 8 MiB
	var out [][2]int
	grown := allocDuring(func() { out = mrqlSlotTagSpansM(m, inner) })
	assert.Nil(t, out)
	assert.ErrorIs(t, m.Err(), ErrRenderBudgetExceeded)
	assert.Less(t, grown, int64(2<<20), "the origin array was allocated before the reservation")

	// With room it still returns the spans.
	m = limitedMeter(nil)
	spans := mrqlSlotTagSpansM(m, "a[header]h[/header]b")
	assert.Len(t, spans, 2)
}

// Empty dividers are records too: 1.3 million [elseif] used to allocate a
// branch, a map and a span each.
func TestEmptyDividersAreReserved(t *testing.T) {
	inner := strings.Repeat("[elseif]", 200_000)
	m := limitedMeter(nil)
	var branches []Branch
	grown := allocDuring(func() { branches = splitBranchesM(m, inner) })
	assert.ErrorIs(t, m.Err(), ErrRenderBudgetExceeded)
	assert.LessOrEqual(t, len(branches), maxDividerRecords+1)
	assert.Less(t, grown, int64(40<<20))

	m = limitedMeter(nil)
	spans := branchDividerSpansM(m, strings.Repeat("[else]", 200_000))
	assert.ErrorIs(t, m.Err(), ErrRenderBudgetExceeded)
	assert.LessOrEqual(t, len(spans), maxDividerRecords+1)

	// Through lint, at a size under the cap.
	in := `[conditional path="a" eq="1"]` + strings.Repeat("[elseif]", 120_000) + `[/conditional]`
	issues := Lint(in, LintOptions{Known: KnownFromBuiltins(), ValidateMRQL: func(string, MRQLCheckOptions) error { return nil }})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
}

// The helper scans are cancellable: each returns promptly on a done context.
func TestSlotHelpersCheckTheMeter(t *testing.T) {
	cc := &errCountingCtx{Context: context.Background(), after: 0}
	body := strings.Repeat("x", 1<<20)
	for name, f := range map[string]func(m *renderMeter){
		"firstSlotSpan":     func(m *renderMeter) { firstSlotSpanM(m, body, "header") },
		"firstElseIndex":    func(m *renderMeter) { firstElseIndexM(m, body) },
		"branchDivider":     func(m *renderMeter) { branchDividerSpansM(m, body) },
		"countTopLevelElse": func(m *renderMeter) { countTopLevelElseM(m, body) },
		"splitElse":         func(m *renderMeter) { splitElseM(m, body) },
	} {
		cc.calls = 0
		m := limitedMeter(cc)
		f(m)
		assert.Error(t, m.Err(), name)
		assert.Less(t, cc.calls, 50, name)
	}
}

// markItems preflights the marked source: 49,000 bare [item] expand to ~2 MB.
func TestMarkItemsPreflightsTheMarkedSource(t *testing.T) {
	branch := strings.Repeat("[item]", 40000)
	m := newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 512 << 10}))
	var out string
	parseOnly := allocDuring(func() {
		parseWithBlocksM(newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 512 << 10})), branch)
	})
	grown := allocDuring(func() { out, _ = markItems(m, branch) })
	assert.Empty(t, out)
	assert.ErrorIs(t, m.Err(), ErrRenderBudgetExceeded)
	assert.Less(t, grown, parseOnly+int64(512<<10), "the marked source (~1.6 MB) was built after the parse")
	// Within the allowance it marks as before.
	m = newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 512 << 10}))
	marked, items := markItems(m, "a[item]b[item path=\"x\"]c")
	assert.Len(t, items, 2)
	assert.Contains(t, marked, itemSentinelPrefix)
}
