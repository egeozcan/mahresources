package api_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models/query_models"
)

func TestCreateGroupWhitespaceNameRejected(t *testing.T) {
	tc := SetupTestEnv(t)

	payload := query_models.GroupCreator{
		Name: "   ",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", payload)
	assert.GreaterOrEqual(t, resp.Code, 400,
		"creating a group with whitespace-only name should be rejected")
}

func TestCreateNoteWhitespaceNameRejected(t *testing.T) {
	tc := SetupTestEnv(t)

	payload := query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name: "   ",
		},
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", payload)
	assert.GreaterOrEqual(t, resp.Code, 400,
		"creating a note with whitespace-only name should be rejected")
}
