package shortcodes

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func cssResult() *QueryResult {
	return &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, CategoryID: 7, Entity: testEntity{ID: 1, Name: "A"}, Meta: []byte(`{}`),
				CustomMRQLResult: `<div class="rc">x</div>`, CustomCSS: `.rc {}`},
		},
	}
}

// An [autorefresh] body is replaced whenever it refreshes, so a stylesheet it
// emits cannot stand in for the rest of the page: content after the region
// still emits its own copy, or it would lose its styling when the refreshed
// body no longer shows that category.
func TestRegionStylesheetDoesNotDedupeThePageAfterIt(t *testing.T) {
	var calls []string
	seen := map[string]bool{}
	reqCtx := WithCSSSeen(WithRefreshSigner(context.Background(), fakeSigner(&calls)), seen)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7}

	out := Process(reqCtx, `[autorefresh][mrql query="x"][/autorefresh][mrql query="x"]`, ctx, nil, mockExecutor(cssResult(), nil))

	assert.Equal(t, 2, strings.Count(out, `data-mr-custom-css="resource:7"`), out)
	region := out[:strings.Index(out, "</autorefresh-shortcode>")]
	assert.Contains(t, region, `data-mr-custom-css="resource:7"`, "the region carries its own copy")
	assert.True(t, seen["resource:7"], "the page-level copy is recorded for what follows")
}

// A stylesheet the page wrote outside any region stays, so a region after it
// still skips that category.
func TestRegionSkipsAStylesheetThePageAlreadyWrote(t *testing.T) {
	var calls []string
	seen := map[string]bool{}
	reqCtx := WithCSSSeen(WithRefreshSigner(context.Background(), fakeSigner(&calls)), seen)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7}

	out := Process(reqCtx, `[mrql query="x"][autorefresh][mrql query="x"][/autorefresh]`, ctx, nil, mockExecutor(cssResult(), nil))

	assert.Equal(t, 1, strings.Count(out, `data-mr-custom-css="resource:7"`), out)
}
