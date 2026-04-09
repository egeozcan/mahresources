package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mockExecutor(result *QueryResult, err error) QueryExecutor {
	return func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		return result, err
	}
}

func TestMRQLShortcodeFlat(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Photo A"}, Meta: []byte(`{}`)},
			{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "Photo B"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "type = 'resource'", "limit": "10"}, Raw: `[mrql query="type = 'resource'" limit="10"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "Photo A")
	assert.Contains(t, html, "Photo B")
	assert.Contains(t, html, "mrql-results")
}

func TestMRQLShortcodeCustomTemplate(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType:       "resource",
				EntityID:         1,
				Entity:           testEntity{ID: 1, Name: "My Photo"},
				Meta:             []byte(`{"rating": 5}`),
				CustomMRQLResult: `<div class="card">[property path="Name"]</div>`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "type = 'resource'"}, Raw: `[mrql query="type = 'resource'"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<div class="card">My Photo</div>`)
}

func TestMRQLShortcodeFormatOverridesCustom(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType:       "resource",
				EntityID:         1,
				Entity:           testEntity{ID: 1, Name: "Entity"},
				Meta:             []byte(`{}`),
				CustomMRQLResult: `<div>CUSTOM</div>`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	// Explicit format="table" overrides the custom template
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test", "format": "table"}, Raw: `[mrql query="test" format="table"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.NotContains(t, html, "CUSTOM")
	assert.Contains(t, html, "<table")
}

func TestMRQLShortcodeAggregated(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "aggregated",
		Rows: []map[string]any{
			{"category": "photo", "count": float64(10)},
			{"category": "video", "count": float64(5)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<table")
	assert.Contains(t, html, "photo")
	assert.Contains(t, html, "video")
}

func TestMRQLShortcodeBucketed(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "bucketed",
		Groups: []QueryResultGroup{
			{
				Key: map[string]any{"category": "photo"},
				Items: []QueryResultItem{
					{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Sunset"}, Meta: []byte(`{}`)},
				},
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "Sunset")
	assert.Contains(t, html, "photo")
}

func TestMRQLShortcodeExecutorError(t *testing.T) {
	executor := mockExecutor(nil, fmt.Errorf("query failed"))
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "bad query"}, Raw: `[mrql query="bad query"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "query failed")
}

func TestMRQLShortcodeNoQueryOrSaved(t *testing.T) {
	executor := mockExecutor(nil, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{}, Raw: `[mrql]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", html)
}

func TestMRQLShortcodeRecursionDepthCap(t *testing.T) {
	callCount := 0
	var executor QueryExecutor
	executor = func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		callCount++
		return &QueryResult{
			EntityType: "resource",
			Mode:       "flat",
			Items: []QueryResultItem{
				{
					EntityType:       "resource",
					EntityID:         1,
					Entity:           testEntity{ID: 1, Name: "Nested"},
					Meta:             []byte(`{}`),
					CustomMRQLResult: `[mrql query="type = 'resource'"]`, // recursive!
				},
			},
		}, nil
	}

	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	// Depth 0 → executes, custom template contains [mrql] → depth 1 executes,
	// that custom template also contains [mrql] → depth 2 hits cap, left as raw
	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, 2, callCount) // should execute exactly twice (depth 0 and 1)
	assert.Contains(t, html, `[mrql query="type = 'resource'"]`) // depth-2 shortcode left raw
}

func TestMRQLShortcodeSavedQuery(t *testing.T) {
	var capturedSaved string
	executor := func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		capturedSaved = savedName
		return &QueryResult{EntityType: "resource", Mode: "flat", Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Saved Result"}, Meta: []byte(`{}`)},
		}}, nil
	}
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"saved": "my-query"}, Raw: `[mrql saved="my-query"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "my-query", capturedSaved)
	assert.Contains(t, html, "Saved Result")
}

func TestMRQLShortcodeDefaultLimits(t *testing.T) {
	var capturedLimit, capturedBuckets int
	executor := func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		capturedLimit = limit
		capturedBuckets = buckets
		return &QueryResult{EntityType: "resource", Mode: "flat"}, nil
	}
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, 20, capturedLimit)
	assert.Equal(t, 5, capturedBuckets)
}

// Ensure the json import is used (Meta field uses json.RawMessage in the test struct)
var _ = json.RawMessage{}
