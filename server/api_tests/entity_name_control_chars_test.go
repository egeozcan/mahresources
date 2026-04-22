package api_tests

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

// BH-019: entity names must not accept NUL bytes, Unicode directional
// overrides/isolates, or embedded newlines. These characters cause UI
// spoofing, CSV/log corruption, and C-library truncation (e.g. ffmpeg
// shelling to a path containing NUL).

func TestTagCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-nul\x00byte")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "NUL-byte name must be rejected with 400, got body=%s", resp.Body.String())
}

func TestTagCreate_RejectsDirectionalOverrideInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19\u202erotated")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "RTL override in name must be rejected, got body=%s", resp.Body.String())
}

func TestTagCreate_RejectsEmbeddedNewlineInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19\nwith newline")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "Newline in name must be rejected, got body=%s", resp.Body.String())
}

func TestTagCreate_AcceptsNormalName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-ordinary-tag-name")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/tag", form)
	assert.Equal(t, http.StatusOK, resp.Code, "Ordinary name must still succeed, got body=%s", resp.Body.String())
}

func TestGroupCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-group\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "got body=%s", resp.Body.String())
}

func TestNoteCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-note\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "got body=%s", resp.Body.String())
}

func TestCategoryCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-cat\x00null")
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/category", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "got body=%s", resp.Body.String())
}

func TestNoteTypeCreate_RejectsNullByteInName(t *testing.T) {
	tc := SetupTestEnv(t)
	form := url.Values{}
	form.Set("Name", "bh19-nt\x00null")
	// NoteType create route is /v1/note/noteType.
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note/noteType", form)
	assert.Equal(t, http.StatusBadRequest, resp.Code, "got body=%s", resp.Body.String())
}
