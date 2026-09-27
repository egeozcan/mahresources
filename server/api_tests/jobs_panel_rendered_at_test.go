package api_tests

import (
	"encoding/json"
	"net/http"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var jobsPanelRenderedAtMeta = regexp.MustCompile(`<meta name="x-jobs-panel-rendered-at" content="([^"]*)">`)

// The Jobs drawer refreshes a page's resource lists only for downloads that
// succeeded at or after the page's render began, so every page publishes that
// time, taken before its data is read. It is the drawer's, not part of a page's
// JSON.
func TestPagesPublishWhenTheirRenderBegan(t *testing.T) {
	tc := SetupTestEnv(t)

	before := time.Now().Truncate(time.Millisecond)
	resp := tc.MakeRequest(http.MethodGet, "/resources", nil)
	after := time.Now()
	require.Equal(t, http.StatusOK, resp.Code)

	match := jobsPanelRenderedAtMeta.FindStringSubmatch(resp.Body.String())
	require.NotNil(t, match, "the page publishes no render time")
	renderedAt, err := time.Parse(time.RFC3339Nano, match[1])
	require.NoError(t, err, "the render time %q is not a timestamp", match[1])
	assert.False(t, renderedAt.Before(before), "the render time %s predates the request (%s)", renderedAt, before)
	assert.False(t, renderedAt.After(after), "the render time %s postdates the response (%s)", renderedAt, after)

	jsonResp := tc.MakeRequest(http.MethodGet, "/resources.json", nil)
	require.Equal(t, http.StatusOK, jsonResp.Code)
	var body map[string]any
	require.NoError(t, json.Unmarshal(jsonResp.Body.Bytes(), &body))
	assert.NotContains(t, body, "jobsPanelRenderedAt")
}
