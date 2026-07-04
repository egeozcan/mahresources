package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- extractRawValueAtPath tests ---

func TestExtractRawValueAtPathString(t *testing.T) {
	meta := json.RawMessage(`{"status":"active"}`)
	result := extractRawValueAtPath(meta, "status")
	assert.Equal(t, "active", result)
}

func TestExtractRawValueAtPathNumber(t *testing.T) {
	meta := json.RawMessage(`{"count":42}`)
	result := extractRawValueAtPath(meta, "count")
	assert.Equal(t, float64(42), result)
}

func TestExtractRawValueAtPathNested(t *testing.T) {
	meta := json.RawMessage(`{"a":{"b":"deep"}}`)
	result := extractRawValueAtPath(meta, "a.b")
	assert.Equal(t, "deep", result)
}

func TestExtractRawValueAtPathMissing(t *testing.T) {
	meta := json.RawMessage(`{"a":"b"}`)
	result := extractRawValueAtPath(meta, "missing")
	assert.Nil(t, result)
}

func TestExtractRawValueAtPathEmpty(t *testing.T) {
	result := extractRawValueAtPath(nil, "x")
	assert.Nil(t, result)
}

func TestExtractRawValueAtPathBool(t *testing.T) {
	meta := json.RawMessage(`{"featured":true}`)
	result := extractRawValueAtPath(meta, "featured")
	assert.Equal(t, true, result)
}

// --- RenderConditionalShortcode tests ---

func makeMetaJSON(t *testing.T, v map[string]any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestConditionalEq(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "<b>yes</b>",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "<b>yes</b>", result)
}

func TestConditionalEqFalse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "<b>yes</b>",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "inactive"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalNeq(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "neq": "draft"},
		InnerContent: "published",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "published", result)
}

func TestConditionalGt(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "score", "gt": "50"},
		InnerContent: "high",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(75)})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "high", result)
}

func TestConditionalLt(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "score", "lt": "50"},
		InnerContent: "low",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(25)})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "low", result)
}

func TestConditionalGtNonNumericReturnsFalse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "gt": "50"},
		InnerContent: "yes",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"name": "hello"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalLtNonNumericReturnsFalse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "lt": "50"},
		InnerContent: "yes",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"name": "hello"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalContains(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "contains": "test"},
		InnerContent: "has test",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"name": "my test item"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "has test", result)
}

func TestConditionalEmpty(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "missing", "empty": "true"},
		InnerContent: "is empty",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "is empty", result)
}

func TestConditionalNotEmpty(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "not-empty": "true"},
		InnerContent: "exists",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"name": "hello"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "exists", result)
}

func TestConditionalElseBranch(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "yes[else]no",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "inactive"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "no", result)
}

func TestConditionalElseBranchTrue(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "yes[else]no",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "yes", result)
}

func TestConditionalSelfClosingRendersMarker(t *testing.T) {
	sc := Shortcode{
		Name:    "conditional",
		Attrs:   map[string]string{"path": "status", "eq": "active"},
		Raw:     `[conditional path="status" eq="active"]`,
		IsBlock: false, // self-closing: no IsBlock
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	// A conditional with no closing tag can't gate anything: surface an
	// author-facing marker rather than leaking the raw tag.
	assert.Contains(t, result, `class="shortcode-error`)
	assert.Contains(t, result, "closing [/conditional]")
	assert.NotContains(t, result, `[conditional path="status" eq="active"]`)
}

func TestProcessUnmatchedConditionalRendersMarker(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       makeMetaJSON(t, map[string]any{"status": "inactive"}),
	}
	// Unmatched [conditional] without closing tag — the tag becomes a marker and
	// the (ungated) trailing text stays put, so the author sees the broken tag.
	input := `[conditional path="status" eq="active"]SECRET`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, result, `class="shortcode-error`)
	assert.NotContains(t, result, `[conditional path="status" eq="active"]`)
	assert.Contains(t, result, "SECRET") // trailing literal text is outside the tag
}

func TestConditionalFieldSource(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"field": "Name", "eq": "hello"},
		InnerContent: "matched",
		IsBlock:      true,
	}
	entity := testEntity{Name: "hello"}
	ctx := MetaShortcodeContext{Entity: entity}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "matched", result)
}

func TestConditionalBoolEq(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "featured", "eq": "true"},
		InnerContent: "featured",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"featured": true})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "featured", result)
}

// --- MRQL source tests ---

func TestConditionalMRQLSourceFlat(t *testing.T) {
	items := []QueryResultItem{
		{EntityType: "resource", EntityID: 1},
		{EntityType: "resource", EntityID: 2},
		{EntityType: "resource", EntityID: 3},
	}
	executor := mockExecutor(&QueryResult{Mode: "flat", Items: items}, nil)
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type='resource'", "gt": "2"},
		InnerContent: "many",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "many", result)
}

func TestConditionalMRQLSourceFlatFalse(t *testing.T) {
	items := []QueryResultItem{
		{EntityType: "resource", EntityID: 1},
	}
	executor := mockExecutor(&QueryResult{Mode: "flat", Items: items}, nil)
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type='resource'", "gt": "2"},
		InnerContent: "many",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", result)
}

func TestConditionalMRQLSourceAggregated(t *testing.T) {
	rows := []map[string]any{
		{"total": float64(100)},
	}
	executor := mockExecutor(&QueryResult{Mode: "aggregated", Rows: rows}, nil)
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "SELECT COUNT(*) AS total", "aggregate": "total", "gt": "50"},
		InnerContent: "high total",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "high total", result)
}

func TestConditionalMRQLSourceAggregatedNoAggregate(t *testing.T) {
	rows := []map[string]any{
		{"total": float64(100)},
	}
	executor := mockExecutor(&QueryResult{Mode: "aggregated", Rows: rows}, nil)
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "SELECT COUNT(*) AS total", "gt": "50"}, // no aggregate attr
		InnerContent: "high total",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, result, "mrql-error")
	assert.Contains(t, result, "aggregate")
}

func TestConditionalMRQLSourceExecutorError(t *testing.T) {
	executor := mockExecutor(nil, fmt.Errorf("query syntax error"))
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "bad query", "gt": "0"},
		InnerContent: "yes",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, result, "mrql-error")
	assert.Contains(t, result, "query syntax error")
}

func TestConditionalMRQLSourceBucketed(t *testing.T) {
	groups := []QueryResultGroup{
		{Key: map[string]any{"cat": "a"}, Items: nil},
		{Key: map[string]any{"cat": "b"}, Items: nil},
		{Key: map[string]any{"cat": "c"}, Items: nil},
		{Key: map[string]any{"cat": "d"}, Items: nil},
	}
	executor := mockExecutor(&QueryResult{Mode: "bucketed", Groups: groups}, nil)
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "GROUP BY cat", "gt": "3"},
		InnerContent: "many groups",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "many groups", result)
}

// --- Phase 2: new operators ---

func TestConditionalGte(t *testing.T) {
	for _, score := range []float64{50, 75} {
		sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "score", "gte": "50"}, InnerContent: "ok", IsBlock: true}
		ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": score})}
		assert.Equal(t, "ok", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0), "score=%v", score)
	}
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "score", "gte": "50"}, InnerContent: "ok", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(49)})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))
}

func TestConditionalLte(t *testing.T) {
	for _, score := range []float64{10, 50} {
		sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "score", "lte": "50"}, InnerContent: "ok", IsBlock: true}
		ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": score})}
		assert.Equal(t, "ok", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0), "score=%v", score)
	}
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "score", "lte": "50"}, InnerContent: "ok", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(51)})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))
}

func TestConditionalIn(t *testing.T) {
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "status", "in": "active, pending ,closed"}, InnerContent: "in", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "pending"})}
	assert.Equal(t, "in", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))

	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "archived"})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

func TestConditionalMatches(t *testing.T) {
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "sku", "matches": "^SKU-[0-9]+$"}, InnerContent: "valid", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"sku": "SKU-42"})}
	assert.Equal(t, "valid", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))

	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"sku": "nope"})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

func TestConditionalMatchesInvalidRegexIsFalse(t *testing.T) {
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "sku", "matches": "([unterminated"}, InnerContent: "valid", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"sku": "anything"})}
	// Invalid regex must evaluate to false, never an error box.
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
	assert.NotContains(t, result, "mrql-error")
}

// --- Phase 2: multi-operator AND / combine ---

func TestConditionalMultiOperatorAND(t *testing.T) {
	// Range: gte AND lte must both pass.
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "score", "gte": "1", "lte": "10"}, InnerContent: "in range", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(5)})}
	assert.Equal(t, "in range", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))

	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(20)})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

func TestConditionalCombineAny(t *testing.T) {
	// combine=any: OR across present operators.
	sc := Shortcode{Name: "conditional", Attrs: map[string]string{"path": "score", "lt": "1", "gt": "10", "combine": "any"}, InnerContent: "outside", IsBlock: true}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(20)})}
	assert.Equal(t, "outside", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))

	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"score": float64(5)})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

// --- Phase 2: numbered-suffix multi-value conditions ---

func TestConditionalMultiValueAND(t *testing.T) {
	// Default combine=all: both path (status=active) and path2 (score>=5) must hold.
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active", "path2": "score", "gte2": "5"},
		InnerContent: "both",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active", "score": float64(8)})}
	assert.Equal(t, "both", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))

	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active", "score": float64(2)})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

func TestConditionalMultiValueOR(t *testing.T) {
	// combine=any across conditions: either status=active OR score>=5.
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active", "path2": "score", "gte2": "5", "combine": "any"},
		InnerContent: "either",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "closed", "score": float64(8)})}
	assert.Equal(t, "either", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))

	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "closed", "score": float64(2)})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

// --- Phase 2: [elseif] chains ---

func TestConditionalElseIfChain(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "tier", "eq": "gold"},
		InnerContent: `G[elseif path="tier" eq="silver"]S[elseif path="tier" eq="bronze"]B[else]none`,
		IsBlock:      true,
	}
	cases := map[string]string{"gold": "G", "silver": "S", "bronze": "B", "wood": "none"}
	for tier, want := range cases {
		ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"tier": tier})}
		assert.Equal(t, want, RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0), "tier=%s", tier)
	}
}

func TestConditionalElseIfFirstMatchWins(t *testing.T) {
	// Both the if and an elseif would match; the if branch wins.
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "n", "gt": "0"},
		InnerContent: `positive[elseif path="n" gt="-100"]big[else]other`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"n": float64(5)})}
	assert.Equal(t, "positive", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))
}

func TestConditionalElseIfNoMatchNoElse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "tier", "eq": "gold"},
		InnerContent: `G[elseif path="tier" eq="silver"]S`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"tier": "bronze"})}
	assert.Equal(t, "", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))
}

func TestConditionalElseIfNestedBlockIgnored(t *testing.T) {
	// An [elseif] inside a nested conditional block must not split the outer one.
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "a", "eq": "1"},
		InnerContent: `[conditional path="b" eq="2"]inner[elseif path="b" eq="3"]innerB[/conditional]OUT[else]elsebranch`,
		IsBlock:      true,
	}
	// a=1 → if branch renders; nested conditional b=3 → its elseif renders "innerB", then "OUT".
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"a": "1", "b": "3"})}
	assert.Equal(t, "innerBOUT", RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0))
	// a=9 → else branch.
	ctx2 := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"a": "9", "b": "3"})}
	assert.Equal(t, "elsebranch", RenderConditionalShortcode(context.Background(), sc, ctx2, nil, nil, 0))
}

func TestConditionalMRQLSourcePriorityOverPath(t *testing.T) {
	// executor returns 0 items → gt="0" fails (0 > 0 is false)
	executor := mockExecutor(&QueryResult{Mode: "flat", Items: []QueryResultItem{}}, nil)
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type='resource'", "path": "status", "eq": "active", "gt": "0"},
		InnerContent: "yes",
		IsBlock:      true,
	}
	// Even though path=status eq=active would match, mrql takes priority
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", result)
}
