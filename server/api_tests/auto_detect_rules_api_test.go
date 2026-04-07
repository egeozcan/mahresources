package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"mahresources/models"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateResourceCategory_ValidAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	body := map[string]any{"Name": "Photos", "AutoDetectRules": `{"contentTypes":["image/jpeg"],"priority":5}`}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", body)
	assert.Equal(t, http.StatusOK, resp.Code)
	var result models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &result))
	assert.Equal(t, `{"contentTypes":["image/jpeg"],"priority":5}`, result.AutoDetectRules)
}

func TestCreateResourceCategory_InvalidAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": "Bad", "AutoDetectRules": `not json`})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateResourceCategory_MissingContentTypes(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": "No CT", "AutoDetectRules": `{"width":{"min":100}}`})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestCreateResourceCategory_UnknownField(t *testing.T) {
	tc := SetupTestEnv(t)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"Name": "Typo", "AutoDetectRules": `{"contentTypes":["image/png"],"contenTypes":["image/png"]}`})
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUpdateResourceCategory_PreservesAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	rc := &models.ResourceCategory{Name: "WithRules", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	tc.DB.Create(rc)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"ID": rc.ID, "Description": "Updated"})
	assert.Equal(t, http.StatusOK, resp.Code)
	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, `{"contentTypes":["image/jpeg"],"priority":5}`, check.AutoDetectRules)
}

func TestUpdateResourceCategory_ClearsAutoDetectRules(t *testing.T) {
	tc := SetupTestEnv(t)
	rc := &models.ResourceCategory{Name: "ClearRules", AutoDetectRules: `{"contentTypes":["image/jpeg"],"priority":5}`}
	tc.DB.Create(rc)
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", map[string]any{"ID": rc.ID, "AutoDetectRules": ""})
	assert.Equal(t, http.StatusOK, resp.Code)
	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, "", check.AutoDetectRules)
}

func createTestPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func TestUploadResource_AutoDetectsCategory(t *testing.T) {
	tc := SetupTestEnv(t)
	photosCat := &models.ResourceCategory{
		Name:            "Wide PNGs",
		AutoDetectRules: `{"contentTypes":["image/png"],"width":{"min":500},"priority":10}`,
	}
	tc.DB.Create(photosCat)

	imgBytes := createTestPNG(t, 800, 600)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("resource", "test.png")
	require.NoError(t, err)
	_, err = part.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("Name", "auto-detect test"))
	require.NoError(t, writer.Close())

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resources []models.Resource
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resources))
	require.Len(t, resources, 1)

	var res models.Resource
	tc.DB.First(&res, resources[0].ID)
	assert.Equal(t, photosCat.ID, res.ResourceCategoryId,
		"resource should be auto-detected into Wide PNGs category")
}

func TestUploadResource_ExplicitCategorySkipsDetection(t *testing.T) {
	tc := SetupTestEnv(t)
	photosCat := &models.ResourceCategory{
		Name:            "Wide PNGs",
		AutoDetectRules: `{"contentTypes":["image/png"],"width":{"min":500},"priority":10}`,
	}
	tc.DB.Create(photosCat)

	imgBytes := createTestPNG(t, 800, 600)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("resource", "test.png")
	require.NoError(t, err)
	_, err = part.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, writer.WriteField("Name", "explicit-category test"))
	require.NoError(t, writer.WriteField("ResourceCategoryId", fmt.Sprintf("%d", tc.AppCtx.DefaultResourceCategoryID)))
	require.NoError(t, writer.Close())

	req, _ := http.NewRequest(http.MethodPost, "/v1/resource", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()
	tc.Router.ServeHTTP(rr, req)
	require.Equal(t, http.StatusOK, rr.Code)

	var resources []models.Resource
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resources))
	require.Len(t, resources, 1)

	var res models.Resource
	tc.DB.First(&res, resources[0].ID)
	assert.Equal(t, tc.AppCtx.DefaultResourceCategoryID, res.ResourceCategoryId,
		"explicit category should not be overridden by auto-detect")
}
