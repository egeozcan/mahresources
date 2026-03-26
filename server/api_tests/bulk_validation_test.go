package api_tests

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
)

func TestBulkNoteValidation(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "Bulk Test Tag"}
	tc.DB.Create(tag)
	note := tc.CreateDummyNote("Bulk Test Note")
	group := tc.CreateDummyGroup("Bulk Test Group")

	t.Run("addTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addTags",
			url.Values{"EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addTags with no TagID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addTags",
			url.Values{"ID": {fmt.Sprint(note.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no tag IDs provided")
	})

	t.Run("addTags with nonexistent note IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addTags",
			url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when note IDs don't exist")
	})

	t.Run("removeTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/removeTags",
			url.Values{"EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("removeTags with no TagID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/removeTags",
			url.Values{"ID": {fmt.Sprint(note.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no tag IDs provided")
	})

	t.Run("addGroups with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addGroups",
			url.Values{"EditedId": {fmt.Sprint(group.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addGroups with no GroupID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/addGroups",
			url.Values{"ID": {fmt.Sprint(note.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no group IDs provided")
	})

	t.Run("addMeta with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/notes/addMeta", map[string]any{
			"Meta": `{"key":"val"}`,
		})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addMeta with nonexistent IDs returns error", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/notes/addMeta", map[string]any{
			"ID":   []uint{999999},
			"Meta": `{"key":"val"}`,
		})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when note IDs don't exist")
	})

	t.Run("delete with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/notes/delete", url.Values{})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})
}

func TestBulkGroupValidation(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "Bulk Group Test Tag"}
	tc.DB.Create(tag)
	group := tc.CreateDummyGroup("Bulk Test Group")

	t.Run("addTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/addTags",
			url.Values{"EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addTags with no TagID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/addTags",
			url.Values{"ID": {fmt.Sprint(group.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no tag IDs provided")
	})

	t.Run("addTags with nonexistent group IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/addTags",
			url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when group IDs don't exist")
	})

	t.Run("removeTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/removeTags",
			url.Values{"EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("removeTags with no TagID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/removeTags",
			url.Values{"ID": {fmt.Sprint(group.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no tag IDs provided")
	})

	t.Run("addMeta with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/groups/addMeta", map[string]any{
			"Meta": `{"key":"val"}`,
		})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addMeta with nonexistent IDs returns error", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/groups/addMeta", map[string]any{
			"ID":   []uint{999999},
			"Meta": `{"key":"val"}`,
		})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when group IDs don't exist")
	})

	t.Run("delete with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/groups/delete", url.Values{})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})
}

func TestBulkResourceValidation(t *testing.T) {
	tc := SetupTestEnv(t)
	tag := &models.Tag{Name: "Bulk Resource Test Tag"}
	tc.DB.Create(tag)
	group := tc.CreateDummyGroup("Bulk Resource Test Group")

	// Create a minimal resource for testing
	resource := &models.Resource{Name: "Bulk Test Resource", Hash: "abc123", HashType: "SHA1", Location: "/test/file.txt"}
	tc.DB.Create(resource)

	t.Run("addTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addTags",
			url.Values{"EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addTags with no TagID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addTags",
			url.Values{"ID": {fmt.Sprint(resource.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no tag IDs provided")
	})

	t.Run("addTags with nonexistent resource IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addTags",
			url.Values{"ID": {"999999"}, "EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when resource IDs don't exist")
	})

	t.Run("removeTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/removeTags",
			url.Values{"EditedId": {fmt.Sprint(tag.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("removeTags with no TagID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/removeTags",
			url.Values{"ID": {fmt.Sprint(resource.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no tag IDs provided")
	})

	t.Run("replaceTags with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/replaceTags", url.Values{})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addGroups with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addGroups",
			url.Values{"EditedId": {fmt.Sprint(group.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addGroups with no GroupID returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/addGroups",
			url.Values{"ID": {fmt.Sprint(resource.ID)}})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no group IDs provided")
	})

	t.Run("addMeta with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/resources/addMeta", map[string]any{
			"Meta": `{"key":"val"}`,
		})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})

	t.Run("addMeta with nonexistent IDs returns error", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodPost, "/v1/resources/addMeta", map[string]any{
			"ID":   []uint{999999},
			"Meta": `{"key":"val"}`,
		})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when resource IDs don't exist")
	})

	t.Run("delete with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/resources/delete", url.Values{})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})
}

func TestBulkTagValidation(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("delete with no IDs returns error", func(t *testing.T) {
		resp := tc.MakeFormRequest(http.MethodPost, "/v1/tags/delete", url.Values{})
		assert.NotEqual(t, http.StatusOK, resp.Code, "should not return 200 when no IDs provided")
	})
}
