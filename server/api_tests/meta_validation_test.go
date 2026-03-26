package api_tests

import (
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMetaValidation(t *testing.T) {
	tc := SetupTestEnv(t)

	t.Run("Note with scalar meta is rejected", func(t *testing.T) {
		payload := query_models.NoteEditor{}
		payload.Name = "Note with bad meta"
		payload.Meta = "42"

		resp := tc.MakeRequest(http.MethodPost, "/v1/note", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Note with array meta is rejected", func(t *testing.T) {
		payload := query_models.NoteEditor{}
		payload.Name = "Note with array meta"
		payload.Meta = "[1,2,3]"

		resp := tc.MakeRequest(http.MethodPost, "/v1/note", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Group with scalar meta is rejected", func(t *testing.T) {
		payload := query_models.GroupCreator{
			Name: "Group with bad meta",
			Meta: "42",
		}

		resp := tc.MakeRequest(http.MethodPost, "/v1/group", payload)
		assert.Equal(t, http.StatusBadRequest, resp.Code)
	})

	t.Run("Note with valid object meta succeeds", func(t *testing.T) {
		payload := query_models.NoteEditor{}
		payload.Name = "Note with good meta"
		payload.Meta = `{"k":"v"}`

		resp := tc.MakeRequest(http.MethodPost, "/v1/note", payload)
		assert.Equal(t, http.StatusOK, resp.Code)
	})
}
