package api_tests

import (
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ffmpegAvailable returns true if ffmpeg is on PATH.
func ffmpegAvailable() bool {
	_, err := exec.LookPath("ffmpeg")
	return err == nil
}

const testVideoPath = "../../test_data/pexels-thirdman-5862328.mp4"

// seedTrimVideoResource reads the test video from disk into the in-memory
// filesystem and creates a Resource row pointing at it.
// Skips if ffmpeg is not available (needed for the actual trim operation).
func seedTrimVideoResource(t *testing.T, tc *TestContext) *models.Resource {
	t.Helper()

	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available, skipping video trim test")
	}

	videoBytes, err := os.ReadFile(testVideoPath)
	require.NoError(t, err, "test video not found at %s", testVideoPath)
	require.True(t, len(videoBytes) > 0, "test video is empty")

	imgPath := "/resources/cc/op/pp/trimtest.mp4"
	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	require.NoError(t, err)
	f, err := fs.Create(imgPath)
	require.NoError(t, err)
	_, err = f.Write(videoBytes)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	owner := tc.CreateDummyGroup("trim-api-owner-" + t.Name())
	resource := &models.Resource{
		Name:        "trimtest.mp4",
		Hash:        "trim000000000000000000000000000000000000",
		HashType:    "SHA1",
		Location:    imgPath,
		ContentType: "video/mp4",
		FileSize:    int64(len(videoBytes)),
		Width:       640,
		Height:      360,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)
	return resource
}

func TestTrimVideo_MissingID_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/trim", url.Values{
		"Start": {"0"},
		"End":   {"1"},
	})

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestTrimVideo_NotFound_Returns404(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/trim", url.Values{
		"ID":    {"9999999"},
		"Start": {"0"},
		"End":   {"1"},
	})

	assert.Equal(t, http.StatusNotFound, resp.Code,
		"missing resource should map to 404")
}

func TestTrimVideo_NonVideo_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create an image resource
	owner := tc.CreateDummyGroup("trim-nonvideo-owner")
	resource := &models.Resource{
		Name:        "image.png",
		Hash:        "nontrim000000000000000000000000000000000",
		HashType:    "SHA1",
		ContentType: "image/png",
		FileSize:    100,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/trim", url.Values{
		"ID":    {strconv.Itoa(int(resource.ID))},
		"Start": {"0"},
		"End":   {"1"},
	})

	assert.Equal(t, http.StatusBadRequest, resp.Code,
		"non-video resource should return 400")
}

func TestTrimVideo_InvalidTimes_Returns400(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a video resource (just DB row, no file needed for validation test)
	owner := tc.CreateDummyGroup("trim-times-owner")
	resource := &models.Resource{
		Name:        "badvid.mp4",
		Hash:        "badt00000000000000000000000000000000000",
		HashType:    "SHA1",
		ContentType: "video/mp4",
		FileSize:    1000,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	cases := []struct {
		name  string
		start string
		end   string
	}{
		{"empty times", "", ""},
		{"negative start", "-1", "5"},
		{"end before start", "10", "5"},
		{"garbage format", "abc", "def"},
		{"only start", "5", ""},
		{"only end", "", "5"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/trim", url.Values{
				"ID":    {strconv.Itoa(int(resource.ID))},
				"Start": {c.start},
				"End":   {c.end},
			})
			assert.Equal(t, http.StatusBadRequest, resp.Code,
				"invalid times should return 400; got %d body=%q", resp.Code, resp.Body.String())
		})
	}
}

func TestTrimVideo_HappyPath_CreatesVersion(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available")
	}

	tc := SetupTestEnv(t)
	resource := seedTrimVideoResource(t, tc)

	// Trim from 1 to 3 seconds (video is ~6s)
	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/trim", url.Values{
		"ID":    {strconv.Itoa(int(resource.ID))},
		"Start": {"1"},
		"End":   {"3"},
		"Comment": {"test trim"},
	})

	assert.Equal(t, http.StatusOK, resp.Code, "trim should succeed; body=%q", resp.Body.String())

	// Verify a version was created
	var versions []models.ResourceVersion
	require.NoError(t, tc.DB.Where("resource_id = ?", resource.ID).
		Order("version_number asc").Find(&versions).Error)
	require.Len(t, versions, 2, "expected lazy v1 plus trimmed v2")
	assert.Equal(t, "Original (before trim)", versions[0].Comment)
	assert.Equal(t, "test trim", versions[1].Comment)
	assert.Equal(t, "SHA1", versions[1].HashType)
	assert.True(t, versions[1].FileSize > 0 && versions[1].FileSize < resource.FileSize,
		"trimmed video should be smaller than original (%d >= %d)", versions[1].FileSize, resource.FileSize)

	// Verify resource was updated
	var updated models.Resource
	require.NoError(t, tc.DB.First(&updated, resource.ID).Error)
	assert.NotNil(t, updated.CurrentVersionID)
	assert.Equal(t, "video/mp4", updated.ContentType)

	// Thumbnails should be cleared
	var previews []models.Preview
	require.NoError(t, tc.DB.Where("resource_id = ?", resource.ID).Find(&previews).Error)
	assert.Len(t, previews, 0, "thumbnails should be cleared after trim")
}

// seedTrimWebmResource transcodes the mp4 test asset to a WebM (VP9/Opus) file,
// stores it in the in-memory filesystem, and creates a Resource row labelled
// video/webm with a .webm location.
func seedTrimWebmResource(t *testing.T, tc *TestContext) *models.Resource {
	t.Helper()

	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available, skipping video trim test")
	}

	tmp := filepath.Join(t.TempDir(), "trim_in.webm")
	cmd := exec.Command("ffmpeg", "-y",
		"-i", testVideoPath,
		"-t", "4",
		"-c:v", "libvpx-vp9", "-b:v", "200k",
		"-c:a", "libopus",
		tmp,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("ffmpeg cannot produce webm (missing vpx/opus codecs?): %v\n%s", err, out)
	}

	webmBytes, err := os.ReadFile(tmp)
	require.NoError(t, err)
	require.True(t, len(webmBytes) > 0, "transcoded webm is empty")

	webmPath := "/resources/cc/op/pp/trimtest.webm"
	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	require.NoError(t, err)
	f, err := fs.Create(webmPath)
	require.NoError(t, err)
	_, err = f.Write(webmBytes)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	owner := tc.CreateDummyGroup("trim-webm-owner-" + t.Name())
	resource := &models.Resource{
		Name:        "trimtest.webm",
		Hash:        "trimwebm0000000000000000000000000000000",
		HashType:    "SHA1",
		Location:    webmPath,
		ContentType: "video/webm",
		FileSize:    int64(len(webmBytes)),
		Width:       640,
		Height:      360,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)
	return resource
}

// TestTrimVideo_NonMp4_StoredAsMp4 guards against a format mismatch: ffmpeg
// always transcodes to an MP4 container, so the new version must be recorded as
// video/mp4 with a .mp4 location even when the source was WebM. Otherwise the
// static file server would serve MP4 bytes under a video/webm content type.
func TestTrimVideo_NonMp4_StoredAsMp4(t *testing.T) {
	if !ffmpegAvailable() {
		t.Skip("ffmpeg not available")
	}

	tc := SetupTestEnv(t)
	resource := seedTrimWebmResource(t, tc)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/trim", url.Values{
		"ID":    {strconv.Itoa(int(resource.ID))},
		"Start": {"0"},
		"End":   {"2"},
	})

	assert.Equal(t, http.StatusOK, resp.Code, "trim should succeed; body=%q", resp.Body.String())

	var versions []models.ResourceVersion
	require.NoError(t, tc.DB.Where("resource_id = ?", resource.ID).
		Order("version_number asc").Find(&versions).Error)
	require.Len(t, versions, 2, "expected lazy v1 (webm) plus trimmed v2 (mp4)")

	// v1 preserves the original webm metadata.
	assert.Equal(t, "video/webm", versions[0].ContentType)
	assert.True(t, strings.HasSuffix(versions[0].Location, ".webm"),
		"original version location should keep .webm, got %q", versions[0].Location)

	// v2 is the transcoded MP4 and must be labelled accordingly.
	assert.Equal(t, "video/mp4", versions[1].ContentType,
		"trimmed version must be video/mp4 since ffmpeg outputs an MP4 container")
	assert.True(t, strings.HasSuffix(versions[1].Location, ".mp4"),
		"trimmed version location should end in .mp4, got %q", versions[1].Location)

	var updated models.Resource
	require.NoError(t, tc.DB.First(&updated, resource.ID).Error)
	assert.Equal(t, "video/mp4", updated.ContentType,
		"resource content type should be updated to video/mp4 after trim")
	assert.True(t, strings.HasSuffix(updated.Location, ".mp4"),
		"resource location should end in .mp4, got %q", updated.Location)
}
