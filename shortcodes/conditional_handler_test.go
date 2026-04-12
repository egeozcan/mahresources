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

func TestConditionalSelfClosingReturnsEmpty(t *testing.T) {
	sc := Shortcode{
		Name:    "conditional",
		Attrs:   map[string]string{"path": "status", "eq": "active"},
		IsBlock: false, // self-closing: no IsBlock
	}
	ctx := MetaShortcodeContext{Meta: makeMetaJSON(t, map[string]any{"status": "active"})}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
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
