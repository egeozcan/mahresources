package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func unavailableExecutor(calls *int) QueryExecutor {
	return func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		*calls++
		return nil, NewQueryUnavailable("the query parses; it would run against groups")
	}
}

func TestQueryUnavailableMRQLRendersInertComment(t *testing.T) {
	var calls int
	got := Process(context.Background(), `[mrql query="type = group"]`, MetaShortcodeContext{}, nil, unavailableExecutor(&calls))
	assert.Contains(t, got, "<!--")
	assert.Contains(t, got, "mrql queries are unavailable")
	assert.Contains(t, got, "it would run against groups")
	assert.NotContains(t, got, "mrql-error")
	assert.NotContains(t, got, "<div")
}

func TestQueryUnavailableInlineValue(t *testing.T) {
	var calls int
	got := Process(context.Background(), `n=[mrql query="type = group" value="count"]`, MetaShortcodeContext{}, nil, unavailableExecutor(&calls))
	assert.Contains(t, got, "<!--")
	assert.NotContains(t, got, "mrql-error")
}

func TestQueryUnavailableUsingDoesNotTakeElse(t *testing.T) {
	var calls int
	got := Process(context.Background(), `[using query="type = group"]BODY[else]ELSE[/using]`, MetaShortcodeContext{}, nil, unavailableExecutor(&calls))
	assert.Contains(t, got, "<!--")
	assert.NotContains(t, got, "BODY")
	assert.NotContains(t, got, "ELSE")
}

func TestQueryUnavailableConditionalTakesNoBranch(t *testing.T) {
	var calls int
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"a":"1"}`)}
	in := `[conditional mrql="type = group" eq="1"]IF[elseif path="a" eq="1"]ELSEIF[else]ELSE[/conditional]`
	got := Process(context.Background(), in, ctx, nil, unavailableExecutor(&calls))
	assert.Contains(t, got, "<!--")
	assert.NotContains(t, got, "IF")
	assert.NotContains(t, got, "ELSE")
	// Numbered condition, second source unavailable.
	in = `[conditional path="a" eq="1" mrql2="type = group" eq2="1"]IF[else]ELSE[/conditional]`
	got = Process(context.Background(), in, ctx, nil, unavailableExecutor(&calls))
	assert.Contains(t, got, "<!--")
	assert.NotContains(t, got, "ELSE")
	// [elseif] arm reached first: its mrql source is unavailable too.
	in = `[conditional path="a" eq="2"]IF[elseif mrql="type = group" eq="1"]ELSEIF[else]ELSE[/conditional]`
	got = Process(context.Background(), in, ctx, nil, unavailableExecutor(&calls))
	assert.Contains(t, got, "<!--")
	assert.NotContains(t, got, "ELSE")
}

func TestQueryUnavailableNotRecordedAsDiagnostic(t *testing.T) {
	var calls int
	res := ProcessWithDiagnostics(context.Background(), `[mrql query="type = group"]`, MetaShortcodeContext{}, nil, unavailableExecutor(&calls))
	assert.Empty(t, res.Errors)
}

func TestQueryUnavailableErrorIs(t *testing.T) {
	err := NewQueryUnavailable("v")
	assert.True(t, errors.Is(err, ErrQueryUnavailable))
}

// 36,000 {link-all} in a slot over a ~32 KiB query would build about 1 GiB; the
// budget must refuse before the replacement is built.
func TestMRQLSlotPlaceholderExpansionIsAdmitted(t *testing.T) {
	longQuery := "type = group AND name ~ " + strings.Repeat("a", 32000)
	exec := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		return &QueryResult{
			Mode:           "flat",
			EffectiveQuery: longQuery,
			Items:          []QueryResultItem{{EntityType: "group", EntityID: 1, Entity: testEntity{ID: 1, Name: "x"}, Meta: []byte(`{}`)}},
		}, nil
	}
	for _, slot := range []string{"header", "footer"} {
		in := `[mrql query="q"][` + slot + `]` + strings.Repeat("{link-all}", 36000) + `[/` + slot + `][/mrql]`
		bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 256 << 10, ParsedSourceBytes: 512 << 10})
		var before, after runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&before)
		_, err := ProcessChecked(bctx, in, MetaShortcodeContext{}, nil, exec)
		runtime.ReadMemStats(&after)
		require.Error(t, err, slot)
		assert.True(t, errors.Is(err, ErrRenderBudgetExceeded), slot)
		assert.Less(t, int64(after.TotalAlloc-before.TotalAlloc), int64(64<<20), slot)
	}
	// else slot, with an empty result.
	execEmpty := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		return &QueryResult{Mode: "flat", EffectiveQuery: longQuery}, nil
	}
	in := `[mrql query="q"][else]` + strings.Repeat("{link-all}", 36000) + `[/mrql]`
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 256 << 10, ParsedSourceBytes: 512 << 10})
	_, err := ProcessChecked(bctx, in, MetaShortcodeContext{}, nil, execEmpty)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded), "else")
}

func TestSubstitutePlaceholdersMatchesReplacerWhenAdmitted(t *testing.T) {
	result := &QueryResult{Mode: "flat", EffectiveQuery: "q & r", Items: make([]QueryResultItem, 3)}
	in := `a{count}b{total}c{link-all}d{count}{nope}`
	want := substitutePlaceholders(in, result)
	got, ok := substitutePlaceholdersM(nil, in, result)
	require.True(t, ok)
	assert.Equal(t, want, got)
	assert.Equal(t, "a3b3c/mrql?q=q+%26+rd3{nope}", got)
}
