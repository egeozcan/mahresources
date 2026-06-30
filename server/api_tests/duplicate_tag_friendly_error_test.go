package api_tests

import (
	"encoding/json"
	"mahresources/application_context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Tier 0 / Item 3: creating a tag with a name that already exists is idempotent for
// programmatic/JSON callers. Instead of a 4xx "already exists" error it returns the existing
// tag (200). This lets the lightbox tag autocompleter (and the mr CLI — both send
// Content-Type: application/json) resolve an "Add" of a name that already exists but sits
// beyond the suggestion window to the real tag instead of failing with a "Could not add" toast.
//
// The explicit /tag/new BROWSER form is the exception: an HTML submission (Accept: text/html)
// of a duplicate name gets a friendly "already exists" error with its input preserved via
// Post-Redirect-Get (BH-006), rather than silently adopting the existing tag. Either way the
// raw DB constraint message must never leak.

func TestDuplicateTagCreationIsIdempotent(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
		"Name": "unique-test-tag",
	})
	require.Equal(t, http.StatusOK, resp.Code, "first tag creation should succeed")

	var first struct {
		ID   uint
		Name string
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &first), "first response should be the tag object")
	require.NotZero(t, first.ID, "first create should return a real ID")

	// Creating the same name again returns the existing tag, not an error.
	resp = tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{
		"Name": "unique-test-tag",
	})
	require.Equal(t, http.StatusOK, resp.Code,
		"duplicate create should be idempotent, got %d: %s", resp.Code, resp.Body.String())

	var second struct {
		ID   uint
		Name string
	}
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &second))
	assert.Equal(t, first.ID, second.ID, "duplicate create should return the existing tag's ID")
	assert.Equal(t, "unique-test-tag", second.Name, "returned tag should be the existing one")

	// The raw DB constraint error must never leak, even on the idempotent path.
	body := resp.Body.String()
	assert.NotContains(t, body, "UNIQUE constraint failed", "raw DB constraint error should not leak")
	assert.NotContains(t, body, "tags.name", "raw DB table/column name should not leak")
}

// A JSON-accepting client (the mr CLI posts urlencoded edits but JSON creates; this covers a
// non-browser urlencoded create) stays idempotent — it does not get the HTML form error.
func TestDuplicateTagCreationViaFormIsIdempotent(t *testing.T) {
	tc := SetupTestEnv(t)

	formData := url.Values{"Name": {"form-dup-tag"}}
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", formData)
	require.Equal(t, http.StatusOK, resp.Code, "first tag creation should succeed")

	// MakeFormRequest sends Accept: application/json, so this is the programmatic path and
	// stays idempotent.
	resp = tc.MakeFormRequest(http.MethodPost, "/v1/tag", formData)
	assert.Equal(t, http.StatusOK, resp.Code, "duplicate non-HTML form create should be idempotent, got %d", resp.Code)

	bodyStr := resp.Body.String()
	assert.NotContains(t, bodyStr, "UNIQUE constraint failed", "raw DB constraint error should not leak")
}

// The explicit /tag/new browser form (Accept: text/html) surfaces a friendly "already exists"
// error and preserves the typed name via Post-Redirect-Get (BH-006), instead of silently
// redirecting to the existing tag.
func TestDuplicateTagCreationViaBrowserFormShowsFriendlyError(t *testing.T) {
	tc := SetupTestEnv(t)

	postBrowserForm := func(name string) *httptest.ResponseRecorder {
		body := strings.NewReader(url.Values{"Name": {name}}.Encode())
		req, _ := http.NewRequest(http.MethodPost, "/v1/tag", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)
		return rr
	}

	resp := postBrowserForm("browser-dup-tag")
	require.Equal(t, http.StatusSeeOther, resp.Code, "first create should redirect to the new tag")

	// Duplicate browser submission is NOT idempotent: it redirects back to the form (PRG)
	// carrying the error and the preserved name.
	resp = postBrowserForm("browser-dup-tag")
	require.Equal(t, http.StatusFound, resp.Code, "duplicate browser create should redirect back to the form")

	location := resp.Header().Get("Location")
	assert.Contains(t, location, "/tag/new", "should redirect back to the create form")
	assert.Contains(t, location, "error=", "redirect should carry the error for the banner")
	assert.Contains(t, strings.ToLower(location), "already+exists", "error should be the friendly duplicate message")
	assert.Contains(t, location, "Name=browser-dup-tag", "the typed name should be preserved")
	assert.NotContains(t, location, "UNIQUE+constraint", "raw DB constraint error should not leak")
}

// A before_tag_create plugin hook can normalize the submitted name (e.g. lowercase +
// hyphenate) before it is persisted. The browser-form duplicate check must detect a
// collision caused by that normalization, not just a literal match on the raw input --
// otherwise CreateTag's idempotent resolve silently redirects to the existing tag
// instead of showing the friendly preserved-input error.
func TestDuplicateTagCreationViaBrowserFormDetectsHookNormalizedDuplicate(t *testing.T) {
	pluginDir := t.TempDir()
	pluginName := "tag-normalizer"
	pluginSubDir := filepath.Join(pluginDir, pluginName)
	require.NoError(t, os.MkdirAll(pluginSubDir, 0755))
	lua := `
plugin = { name = "tag-normalizer", version = "1.0", description = "normalizes tag names" }

function before_create(data)
    data.name = string.lower(data.name)
    data.name = string.gsub(data.name, " ", "-")
    return data
end

function init()
    mah.on("before_tag_create", before_create)
end
`
	require.NoError(t, os.WriteFile(filepath.Join(pluginSubDir, "plugin.lua"), []byte(lua), 0644))

	tc := setupTestEnvWithConfig(t, func(cfg *application_context.MahresourcesConfig) {
		cfg.PluginPath = pluginDir
	})
	require.NotNil(t, tc.AppCtx.PluginManager(), "plugin manager should be initialized")
	require.NoError(t, tc.AppCtx.PluginManager().EnablePlugin(pluginName))

	// Pre-create the tag that the hook will normalize "New Camera" into.
	existing := tc.MakeRequest(http.MethodPost, "/v1/tag", map[string]any{"Name": "new-camera"})
	require.Equal(t, http.StatusOK, existing.Code)

	postBrowserForm := func(name string) *httptest.ResponseRecorder {
		body := strings.NewReader(url.Values{"Name": {name}}.Encode())
		req, _ := http.NewRequest(http.MethodPost, "/v1/tag", body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "text/html")
		rr := httptest.NewRecorder()
		tc.Router.ServeHTTP(rr, req)
		return rr
	}

	// "New Camera" doesn't literally match "new-camera", but the before_tag_create hook
	// normalizes it to the same name. The friendly duplicate error must still fire instead
	// of CreateTag's idempotent silent-resolve.
	resp := postBrowserForm("New Camera")
	require.Equal(t, http.StatusFound, resp.Code,
		"hook-induced duplicate should redirect back to the form, not silently to the existing tag, got %d", resp.Code)

	location := resp.Header().Get("Location")
	assert.Contains(t, location, "/tag/new", "should redirect back to the create form")
	assert.Contains(t, location, "error=", "redirect should carry the error for the banner")
	assert.Contains(t, strings.ToLower(location), "already+exists", "error should be the friendly duplicate message")
	assert.Contains(t, location, "Name=New+Camera", "the originally typed name should be preserved, not the hook-normalized one")
}
