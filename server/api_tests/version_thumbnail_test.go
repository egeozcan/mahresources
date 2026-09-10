package api_tests

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

func TestVersionThumbnail(t *testing.T) {
	tc := SetupTestEnv(t)
	res := newCompareResource(t, tc, "Thumbnail", "thumbnail-source")
	v := addCompareVersion(t, tc, res.ID, 2, "image/png", "")
	var source bytes.Buffer
	require.NoError(t, png.Encode(&source, image.NewRGBA(image.Rect(0, 0, 1200, 800))))
	require.NoError(t, afero.WriteFile(tc.Fs, v.Location, source.Bytes(), 0600))
	url := fmt.Sprintf("/v1/resource/version/preview?versionId=%d&width=900&height=900&v=%s", v.ID, v.Hash)
	rr := doReq(tc, http.MethodGet, url, nil, nil, nil)
	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "image/jpeg", rr.Header().Get("Content-Type"))
	cfg, err := jpeg.DecodeConfig(rr.Body)
	require.NoError(t, err)
	require.LessOrEqual(t, cfg.Width, 600)
	require.LessOrEqual(t, cfg.Height, 600)
	require.Equal(t, "max-age=31536000, immutable", rr.Header().Get("Cache-Control"))
	require.Equal(t, fmt.Sprintf(`"%s-600-600"`, v.Hash), rr.Header().Get("ETag"))
	cached := doReq(tc, http.MethodGet, url, map[string]string{"If-None-Match": rr.Header().Get("ETag")}, nil, nil)
	require.Equal(t, http.StatusNotModified, cached.Code)
	var count int64
	require.NoError(t, tc.DB.Model(&models.Preview{}).Where("resource_id = ?", res.ID).Count(&count).Error)
	require.Zero(t, count)
}

func TestVersionThumbnailPlaceholderAndScope(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	headers := map[string]string{"Authorization": f.bearer}
	for _, contentType := range []string{"application/pdf", "image/png"} {
		v := addCompareVersion(t, tc, f.rInID, 3+len(contentType), contentType, "")
		require.NoError(t, afero.WriteFile(tc.Fs, v.Location, []byte("undecodable bytes"), 0600))
		rr := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/resource/version/preview?versionId=%d&height=64", v.ID), headers, nil, nil)
		require.Equal(t, http.StatusTemporaryRedirect, rr.Code)
		require.Equal(t, "/public/placeholders/file.jpg", rr.Header().Get("Location"))
		require.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))
		require.Empty(t, rr.Header().Get("ETag"))
	}
	outside := addCompareVersion(t, tc, f.rOutID, 2, "image/png", "")
	rr := doReq(tc, http.MethodGet, fmt.Sprintf("/v1/resource/version/preview?versionId=%d&height=64", outside.ID), headers, nil, nil)
	require.Equal(t, http.StatusNotFound, rr.Code)
}
