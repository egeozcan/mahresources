package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// BH-P05: `.json` error responses must not leak `_appContext` / `_requestContext`.
// Those keys serialize the entire server config (DbDsn, FfmpegPath, FileSavePath,
// AltFileSystems, plugin manager, etc.) on any error-rendered .json route.

func TestJsonErrorDoesNotLeakAppContext(t *testing.T) {
	tc := SetupTestEnv(t)

	// Non-existent id on .json route renders an error with a populated template context.
	resp := tc.MakeRequest(http.MethodGet, "/resource.json?id=99999", nil)
	assert.GreaterOrEqual(t, resp.Code, 400, "response should be an error status")

	var body map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &body)
	require.NoError(t, err, "response should be valid JSON")

	// These must NOT leak — they exposed full server config and nested context.
	assert.NotContains(t, body, "_appContext", "_appContext must not leak in JSON error response")
	assert.NotContains(t, body, "_requestContext", "_requestContext must not leak in JSON error response")

	// Sanity: the user-facing error must still be present.
	assert.Contains(t, body, "errorMessage", "errorMessage should be present in error response")
}

func TestJsonErrorDoesNotLeakAppContextAcrossEntities(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{
		"/note.json?id=99999",
		"/group.json?id=99999",
		"/tag.json?id=99999",
		"/resource.json?id=99999",
	} {
		resp := tc.MakeRequest(http.MethodGet, path, nil)
		assert.GreaterOrEqual(t, resp.Code, 400, "%s: expected error status", path)

		var body map[string]any
		err := json.Unmarshal(resp.Body.Bytes(), &body)
		require.NoError(t, err, "%s: response should be valid JSON", path)

		assert.NotContains(t, body, "_appContext", "%s: _appContext must not leak", path)
		assert.NotContains(t, body, "_requestContext", "%s: _requestContext must not leak", path)
	}
}
