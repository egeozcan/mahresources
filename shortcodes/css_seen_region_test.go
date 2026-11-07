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

// Shortcode output can be replaced later ([autorefresh] here; a [reload]
// region or a note text block on the page), so a stylesheet it emits never
// stands in for content after it, and the page set is left for the cards.
func TestResultStylesheetDoesNotDedupeContentAfterIt(t *testing.T) {
	var calls []string
	seen := map[string]bool{}
	reqCtx := WithCSSSeen(WithRefreshSigner(context.Background(), fakeSigner(&calls)), seen)
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7}

	out := Process(reqCtx, `[autorefresh][mrql query="x"][/autorefresh][mrql query="x"]`, ctx, nil, mockExecutor(cssResult(), nil))

	assert.Equal(t, 2, strings.Count(out, `data-mr-custom-css="resource:7"`), out)
	region := out[:strings.Index(out, "</autorefresh-shortcode>")]
	assert.Contains(t, region, `data-mr-custom-css="resource:7"`, "the region carries its own copy")
	assert.False(t, seen["resource:7"], "shortcode output never records into the page set")
}

// A stylesheet a card wrote stays on the page, so any result, inside a region
// or not, still skips that category.
func TestResultsSkipAStylesheetACardWrote(t *testing.T) {
	var calls []string
	reqCtx := WithCSSSeen(WithRefreshSigner(context.Background(), fakeSigner(&calls)), map[string]bool{"resource:7": true})
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7}

	out := Process(reqCtx, `[mrql query="x"][autorefresh][mrql query="x"][/autorefresh]`, ctx, nil, mockExecutor(cssResult(), nil))

	assert.Equal(t, 0, strings.Count(out, `data-mr-custom-css="resource:7"`), out)
}
