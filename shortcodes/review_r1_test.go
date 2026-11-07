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
	in := strings.Repeat("[meta]", 1_000_000)
	var issues []LintIssue
	grown := allocDuring(func() { issues = Lint(in, LintOptions{Known: KnownFromBuiltins()}) })
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
	assert.Less(t, grown, int64(64<<20))

	loose := strings.Repeat("[x", 1_000_000)
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
	for i := 0; i < 3000; i++ {
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
	// 3,000,000 retained entries cost well over 200 MB; the reservation stops at 100,000.
	assert.Less(t, grown, int64(120<<20))
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
