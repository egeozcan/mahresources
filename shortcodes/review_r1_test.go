package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	_ = fmt.Sprint
}
