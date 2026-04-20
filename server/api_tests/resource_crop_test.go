package api_tests

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// seedCropImageResource writes a PNG to the in-memory fs and creates a
// Resource row pointing at it.
func seedCropImageResource(t *testing.T, tc *TestContext, w, h int) *models.Resource {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))

	imgPath := "/resources/cc/op/pp/cropapi" + strconv.Itoa(w) + "x" + strconv.Itoa(h) + ".png"
	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	require.NoError(t, err)
	require.NoError(t, fs.MkdirAll(filepath.Dir(imgPath), 0755))
	f, err := fs.Create(imgPath)
	require.NoError(t, err)
	_, err = f.Write(buf.Bytes())
	require.NoError(t, err)
	require.NoError(t, f.Close())

	owner := tc.CreateDummyGroup("crop-api-owner-" + t.Name())
	resource := &models.Resource{
		Name:        "crop-api.png",
		Hash:        "ccoppp000000000000000000000000000000" + strconv.Itoa(w),
		HashType:    "SHA1",
		Location:    imgPath,
		ContentType: "image/png",
		FileSize:    int64(buf.Len()),
		Width:       uint(w),
		Height:      uint(h),
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)
	return resource
}

func TestCropResource_HappyPath_CreatesVersion(t *testing.T) {
	tc := SetupTestEnv(t)
	resource := seedCropImageResource(t, tc, 100, 80)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/crop", url.Values{
		"ID":     {strconv.Itoa(int(resource.ID))},
		"X":      {"10"},
		"Y":      {"20"},
		"Width":  {"40"},
		"Height": {"30"},
	})

	assert.Equal(t, http.StatusOK, resp.Code, "crop should succeed")

	var versions []models.ResourceVersion
	require.NoError(t, tc.DB.Where("resource_id = ?", resource.ID).Find(&versions).Error)
	assert.Len(t, versions, 2, "expected lazy v1 plus cropped v2")

	var updated models.Resource
	require.NoError(t, tc.DB.First(&updated, resource.ID).Error)
	assert.Equal(t, uint(40), updated.Width)
	assert.Equal(t, uint(30), updated.Height)
	require.NotNil(t, updated.CurrentVersionID)
}

func TestCropResource_MissingID_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/crop", url.Values{
		"X":      {"0"},
		"Y":      {"0"},
		"Width":  {"10"},
		"Height": {"10"},
	})

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCropResource_BadRect_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)
	resource := seedCropImageResource(t, tc, 50, 50)

	cases := []struct {
		name string
		form url.Values
	}{
		{"zero width", url.Values{"ID": {strconv.Itoa(int(resource.ID))}, "X": {"0"}, "Y": {"0"}, "Width": {"0"}, "Height": {"10"}}},
		{"out of bounds", url.Values{"ID": {strconv.Itoa(int(resource.ID))}, "X": {"40"}, "Y": {"0"}, "Width": {"20"}, "Height": {"10"}}},
		{"negative origin", url.Values{"ID": {strconv.Itoa(int(resource.ID))}, "X": {"-1"}, "Y": {"0"}, "Width": {"10"}, "Height": {"10"}}},
	}

	for _, tcase := range cases {
		t.Run(tcase.name, func(t *testing.T) {
			resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/crop", tcase.form)
			assert.Equal(t, http.StatusBadRequest, resp.Code,
				"bad rect should map to 400 via the 'must be' / 'cannot be' validation pattern; got %d body=%q", resp.Code, resp.Body.String())
		})
	}
}

func TestCropResource_NotFound_Returns404(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/crop", url.Values{
		"ID":     {"9999999"},
		"X":      {"0"},
		"Y":      {"0"},
		"Width":  {"10"},
		"Height": {"10"},
	})

	assert.Equal(t, http.StatusNotFound, resp.Code,
		"missing resource should map to 404 via the 'not found' pattern")
}
