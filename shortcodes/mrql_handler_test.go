package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mockExecutor(result *QueryResult, err error) QueryExecutor {
	return func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
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
	var executor QueryExecutor = func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
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

	// Depth 0 → executes, custom template contains [mrql] → depth 1 executes, …
	// repeats until depth maxRecursionDepth-1, then the shortcode is left as raw.
	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, maxRecursionDepth, callCount) // executes exactly maxRecursionDepth times
	assert.Contains(t, html, `[mrql query="type = 'resource'"]`) // depth-cap shortcode left raw
}

func TestMRQLShortcodeSavedQuery(t *testing.T) {
	var capturedSaved string
	executor := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		capturedSaved = opts.SavedName
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
	executor := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		capturedLimit = opts.Limit
		capturedBuckets = opts.Buckets
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

func TestMRQLBlockFlatUsesInnerTemplate(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Alpha"}, Meta: []byte(`{}`)},
			{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "Beta"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<span class="item">[property path="Name"]</span>[/mrql]`,
		InnerContent: `<span class="item">[property path="Name"]</span>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<span class="item">Alpha</span>`)
	assert.Contains(t, html, `<span class="item">Beta</span>`)
}

func TestMRQLBlockOverridesCustomMRQLResult(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType:       "resource",
				EntityID:         1,
				Entity:           testEntity{ID: 1, Name: "Item"},
				Meta:             []byte(`{}`),
				CustomMRQLResult: `<div class="category-template">CATEGORY</div>`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<p class="block-tpl">[property path="Name"]</p>[/mrql]`,
		InnerContent: `<p class="block-tpl">[property path="Name"]</p>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<p class="block-tpl">Item</p>`)
	assert.NotContains(t, html, "CATEGORY")
}

func TestMRQLBlockOverridesFormatAttr(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Row"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test", "format": "table"},
		Raw:          `[mrql query="test" format="table"]<b>[property path="Name"]</b>[/mrql]`,
		InnerContent: `<b>[property path="Name"]</b>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<b>Row</b>")
	assert.NotContains(t, html, "<table")
}

func TestMRQLBlockBucketedAppliesTemplate(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "bucketed",
		Groups: []QueryResultGroup{
			{
				Key: map[string]any{"type": "photo"},
				Items: []QueryResultItem{
					{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Sunset"}, Meta: []byte(`{}`)},
					{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "Mountain"}, Meta: []byte(`{}`)},
				},
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<em>[property path="Name"]</em>[/mrql]`,
		InnerContent: `<em>[property path="Name"]</em>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<em>Sunset</em>")
	assert.Contains(t, html, "<em>Mountain</em>")
	assert.Contains(t, html, "photo")
}

func TestMRQLBlockAggregatedIgnoresInnerContent(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "aggregated",
		Rows: []map[string]any{
			{"category": "photo", "count": float64(10)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]SHOULD NOT APPEAR[/mrql]`,
		InnerContent: `SHOULD NOT APPEAR`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.NotContains(t, html, "SHOULD NOT APPEAR")
	assert.Contains(t, html, "<table")
	assert.Contains(t, html, "photo")
}

func TestMRQLBlockWhitespaceOnlyFallsBack(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Fallback"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          "[mrql query=\"test\"]\n  \n[/mrql]",
		InnerContent: "\n  \n",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "Fallback")
	assert.Contains(t, html, `href="/resource?id=1"`)
}

func TestMRQLBlockEmptyResultsShowsDefault(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items:      []QueryResultItem{},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"]<b>[property path="Name"]</b>[/mrql]`,
		InnerContent: `<b>[property path="Name"]</b>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "No results.")
}

func TestMRQLBlockChildContextWithMeta(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType: "resource",
				EntityID:   1,
				Entity:     testEntity{ID: 1, Name: "Item A"},
				Meta:       []byte(`{"rating": 5}`),
				MetaSchema: `{"type":"object","properties":{"rating":{"type":"integer"}}}`,
			},
			{
				EntityType: "resource",
				EntityID:   2,
				Entity:     testEntity{ID: 2, Name: "Item B"},
				Meta:       []byte(`{"rating": 3}`),
				MetaSchema: `{"type":"object","properties":{"rating":{"type":"integer"}}}`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "test"},
		Raw:          `[mrql query="test"][property path="Name"] [meta path="rating"][/mrql]`,
		InnerContent: `[property path="Name"] [meta path="rating"]`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "Item A")
	assert.Contains(t, html, "Item B")
	assert.Contains(t, html, `data-path="rating"`)
	assert.Contains(t, html, `data-entity-id="1"`)
	assert.Contains(t, html, `data-entity-id="2"`)
}

// Inline [mrql] shortcode injects each category's CustomCSS once (deduped), raw, but only for
// categories whose items render a custom card (CustomMRQLResult set).
func TestMRQLShortcodeCustomCSS(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			// Two items of resource category 7 -> CSS emitted once.
			{EntityType: "resource", EntityID: 1, CategoryID: 7, Entity: testEntity{ID: 1, Name: "A"}, Meta: []byte(`{}`),
				CustomMRQLResult: `<div class="rc">[property path="Name"]</div>`, CustomCSS: `.rc > b { color: red }`},
			{EntityType: "resource", EntityID: 2, CategoryID: 7, Entity: testEntity{ID: 2, Name: "B"}, Meta: []byte(`{}`),
				CustomMRQLResult: `<div class="rc">[property path="Name"]</div>`, CustomCSS: `.rc > b { color: red }`},
			// Note type 7 (different entity type, same numeric id) -> its own block.
			{EntityType: "note", EntityID: 3, CategoryID: 7, Entity: testEntity{ID: 3, Name: "N"}, Meta: []byte(`{}`),
				CustomMRQLResult: `<p>[property path="Name"]</p>`, CustomCSS: `.nt {}`},
			// Group category 9 has CustomCSS but NO custom card -> skipped (default card, no hook).
			{EntityType: "group", EntityID: 4, CategoryID: 9, Entity: testEntity{ID: 4, Name: "G"}, Meta: []byte(`{}`),
				CustomCSS: `.skip {}`},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "x"}, Raw: `[mrql query="x"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)

	// Raw, unescaped CSS, deduped to one block for resource category 7.
	assert.Contains(t, html, `<style data-mr-custom-css="resource:7">.rc > b { color: red }</style>`)
	assert.Equal(t, 1, strings.Count(html, `data-mr-custom-css="resource:7"`), "resource category 7 CSS should appear once")
	assert.NotContains(t, html, "&gt;")
	// Distinct entity type with the same numeric id gets its own block.
	assert.Contains(t, html, `data-mr-custom-css="note:7"`)
	// Category 9 has no custom card, so its CustomCSS is not emitted.
	assert.NotContains(t, html, ".skip {}")
}

// --- Work item 1: inline scalar value mode ---

func TestMRQLInlineValueCountFlat(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "A"}, Meta: []byte(`{}`)},
			{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "B"}, Meta: []byte(`{}`)},
			{EntityType: "resource", EntityID: 3, Entity: testEntity{ID: 3, Name: "C"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "resources", "value": "count"}, Raw: `[mrql query="resources" value="count"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	// No wrapper div: bare escaped text.
	assert.Equal(t, "3", html)
}

func TestMRQLInlineValueCountBucketed(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "bucketed",
		Groups: []QueryResultGroup{
			{Key: map[string]any{"t": "a"}},
			{Key: map[string]any{"t": "b"}},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "x group by t", "value": "count"}, Raw: `[mrql query="x" value="count"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "2", html)
}

func TestMRQLInlineValueColumnAggregated(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "aggregated",
		Rows: []map[string]any{
			{"total": float64(42), "category": "photo"},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "x group by category count()", "value": "total"}, Raw: `[mrql query="x" value="total"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "42", html)
}

func TestMRQLInlineValueNoWrapperOrCSS(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, CategoryID: 7, Entity: testEntity{ID: 1, Name: "A"}, Meta: []byte(`{}`),
				CustomMRQLResult: `<div>x</div>`, CustomCSS: `.rc {}`},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "resources", "value": "count"}, Raw: `[mrql query="resources" value="count"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "1", html)
	assert.NotContains(t, html, "mrql-results")
	assert.NotContains(t, html, "<style")
}

func TestMRQLInlineValueColumnOnFlatIsEmpty(t *testing.T) {
	// A column value has no meaning on a flat (non-aggregated) result.
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "A"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "resources", "value": "total"}, Raw: `[mrql query="resources" value="total"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", html)
}

func TestMRQLInlineValueError(t *testing.T) {
	executor := mockExecutor(nil, fmt.Errorf("boom"))
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "bad", "value": "count"}, Raw: `[mrql query="bad" value="count"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	// Inline error is a span (does not break surrounding block layout), not a div.
	assert.Contains(t, html, "<span")
	assert.Contains(t, html, "mrql-error")
	assert.Contains(t, html, "boom")
	assert.NotContains(t, html, "<div")
}

func TestMRQLInlineValueIgnoresBlockBody(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items:      []QueryResultItem{{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "A"}, Meta: []byte(`{}`)}},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{
		Name:         "mrql",
		Attrs:        map[string]string{"query": "resources", "value": "count"},
		Raw:          `[mrql query="resources" value="count"]<b>[property path="Name"]</b>[/mrql]`,
		InnerContent: `<b>[property path="Name"]</b>`,
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "1", html)
	assert.NotContains(t, html, "<b>")
}

func TestMRQLInlineValueFormatFilesize(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "aggregated",
		Rows:       []map[string]any{{"bytes": int64(2048)}},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "x", "value": "bytes", "format": "filesize"}, Raw: `[mrql query="x" value="bytes" format="filesize"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "2.0 KB", html)
}

// --- Work items 2 & 3: header/footer/else slots, totals, link-all ---

func flatResult(names ...string) *QueryResult {
	r := &QueryResult{EntityType: "resource", Mode: "flat"}
	for i, n := range names {
		r.Items = append(r.Items, QueryResultItem{EntityType: "resource", EntityID: uint(i + 1), Entity: testEntity{ID: uint(i + 1), Name: n}, Meta: []byte(`{}`)})
	}
	return r
}

func blockMRQL(inner string, attrs map[string]string) Shortcode {
	if attrs == nil {
		attrs = map[string]string{}
	}
	if attrs["query"] == "" {
		attrs["query"] = "resources"
	}
	return Shortcode{Name: "mrql", Attrs: attrs, IsBlock: true, InnerContent: inner, Raw: "[mrql]" + inner + "[/mrql]"}
}

func TestMRQLHeaderFooterWrapResults(t *testing.T) {
	executor := mockExecutor(flatResult("Alpha", "Beta"), nil)
	sc := blockMRQL(`[header]<h4>Items ({count})</h4>[/header]<li>[property path="Name"]</li>[footer]<p>done</p>[/footer]`, nil)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<h4>Items (2)</h4>`)
	assert.Contains(t, html, `<li>Alpha</li>`)
	assert.Contains(t, html, `<li>Beta</li>`)
	assert.Contains(t, html, `<p>done</p>`)
	// Header appears before the items, footer after.
	assert.Less(t, strings.Index(html, "Items (2)"), strings.Index(html, "Alpha"))
	assert.Less(t, strings.Index(html, "Beta"), strings.Index(html, "done"))
}

func TestMRQLElseBranchOnEmpty(t *testing.T) {
	executor := mockExecutor(&QueryResult{EntityType: "resource", Mode: "flat"}, nil)
	sc := blockMRQL(`[header]<h4>H</h4>[/header]<li>[property path="Name"]</li>[footer]F[/footer][else]<p>Nothing 🎉</p>`, nil)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<p>Nothing 🎉</p>`)
	// Header/footer are NOT rendered on the empty branch.
	assert.NotContains(t, html, "<h4>H</h4>")
	assert.NotContains(t, html, ">F<")
	assert.NotContains(t, html, "No results.")
}

func TestMRQLNoElseEmptyKeepsDefault(t *testing.T) {
	executor := mockExecutor(&QueryResult{EntityType: "resource", Mode: "flat"}, nil)
	sc := blockMRQL(`<li>[property path="Name"]</li>`, nil)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "No results.")
}

func TestMRQLHeaderNotRenderedOnEmptyWithoutElse(t *testing.T) {
	// Header/footer are chrome around results and are suppressed on an empty
	// result even without an [else]; only the "No results." placeholder shows.
	executor := mockExecutor(&QueryResult{EntityType: "resource", Mode: "flat"}, nil)
	sc := blockMRQL(`[header]<h4>H</h4>[/header]<li>[property path="Name"]</li>[footer]<p>F</p>[/footer]`, map[string]string{"query": "resources", "link-all": "true"})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.NotContains(t, html, "<h4>H</h4>")
	assert.NotContains(t, html, "<p>F</p>")
	assert.NotContains(t, html, "mrql-view-all")
	assert.Contains(t, html, "No results.")
}

func TestMRQLTotalPlaceholderSetsWantTotalAndRenders(t *testing.T) {
	var capturedWantTotal bool
	total := int64(150)
	executor := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		capturedWantTotal = opts.WantTotal
		r := flatResult("A", "B")
		r.Total = &total
		return r, nil
	}
	sc := blockMRQL(`[header]{count} of {total}[/header]<li>[property path="Name"]</li>`, nil)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.True(t, capturedWantTotal, "{total} in a slot must set WantTotal")
	assert.Contains(t, html, "2 of 150")
}

func TestMRQLNoTotalPlaceholderLeavesWantTotalOff(t *testing.T) {
	var capturedWantTotal bool
	executor := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		capturedWantTotal = opts.WantTotal
		return flatResult("A"), nil
	}
	sc := blockMRQL(`[header]{count} items[/header]<li>[property path="Name"]</li>`, nil)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.False(t, capturedWantTotal)
}

func TestMRQLLinkAllDefaultLinkInline(t *testing.T) {
	r := flatResult("A")
	r.EffectiveQuery = "resources where tag = 'x'"
	executor := mockExecutor(r, nil)
	sc := blockMRQL(`<li>[property path="Name"]</li>`, map[string]string{"query": "resources", "link-all": "true"})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "mrql-view-all")
	assert.Contains(t, html, "/mrql?q=resources+where+tag+%3D+%27x%27")
	assert.Contains(t, html, "View all")
}

func TestMRQLLinkAllDefaultLinkSaved(t *testing.T) {
	r := flatResult("A")
	r.SavedID = 9
	executor := mockExecutor(r, nil)
	sc := blockMRQL(`<li>[property path="Name"]</li>`, map[string]string{"query": "", "saved": "rep", "link-all": "true"})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `href="/mrql?saved=9"`)
}

func TestMRQLLinkAllBeforeFooter(t *testing.T) {
	r := flatResult("A")
	r.EffectiveQuery = "resources"
	executor := mockExecutor(r, nil)
	sc := blockMRQL(`<li>[property path="Name"]</li>[footer]<p>myfoot</p>[/footer]`, map[string]string{"query": "resources", "link-all": "true"})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Less(t, strings.Index(html, "mrql-view-all"), strings.Index(html, "myfoot"))
}

func TestMRQLLinkAllPlaceholderInFooter(t *testing.T) {
	r := flatResult("A")
	r.EffectiveQuery = "resources"
	executor := mockExecutor(r, nil)
	sc := blockMRQL(`<li>[property path="Name"]</li>[footer]<a href="{link-all}">more</a>[/footer]`, map[string]string{"query": "resources"})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<a href="/mrql?q=resources">more</a>`)
	// Without link-all="true", no default link is injected.
	assert.NotContains(t, html, "mrql-view-all")
}

func TestMRQLBucketedElseOnZeroBuckets(t *testing.T) {
	executor := mockExecutor(&QueryResult{EntityType: "resource", Mode: "bucketed"}, nil)
	sc := blockMRQL(`<li>[property path="Name"]</li>[else]<p>no buckets</p>`, map[string]string{"query": "x group by t"})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<p>no buckets</p>")
}
