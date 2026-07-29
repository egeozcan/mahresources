package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// UI bug hunt 2026-07-29, finding 152 (WS2): the inline name editor shows a
// generic "Could not save name" because the server hands it a raw driver
// string. `/v1/tag/editName` answered a duplicate rename with
// {"error":"UNIQUE constraint failed: tags.name"} — a message the UI cannot
// show a user even if it stops swallowing it. The client half of the finding
// (rendering the message visibly) is only worth doing once the message itself
// is worth reading, so the server is fixed first.
//
// `EntityWriter.UpdateName` backs every /v1/<entity>/editName route, so the
// entity label has to be derived from the model rather than hardcoded.
func TestEditName_DuplicateReportsFriendlyMessage(t *testing.T) {
	tc := SetupTestEnv(t)

	// Every entity here has a unique index on Name; each is created through its
	// own endpoint so the test exercises the real routes.
	cases := []struct {
		label      string // the human word expected in the message
		createPath string
		editPath   string
		create     func(name string) any
	}{
		{
			label:      "tag",
			createPath: "/v1/tag",
			editPath:   "/v1/tag/editName",
			create:     func(name string) any { return query_models.TagCreator{Name: name} },
		},
		{
			label:      "category",
			createPath: "/v1/category",
			editPath:   "/v1/category/editName",
			create:     func(name string) any { return query_models.CategoryCreator{Name: name} },
		},
		{
			label:      "resource category",
			createPath: "/v1/resourceCategory",
			editPath:   "/v1/resourceCategory/editName",
			create:     func(name string) any { return query_models.CategoryCreator{Name: name} },
		},
	}

	for _, tt := range cases {
		t.Run(tt.label, func(t *testing.T) {
			occupied := fmt.Sprintf("ws2 %s occupied", tt.label)
			victim := fmt.Sprintf("ws2 %s victim", tt.label)

			resp := tc.MakeRequest(http.MethodPost, tt.createPath, tt.create(occupied))
			require.Equal(t, http.StatusOK, resp.Code, "creating %q: %s", occupied, resp.Body.String())

			resp = tc.MakeRequest(http.MethodPost, tt.createPath, tt.create(victim))
			require.Equal(t, http.StatusOK, resp.Code, "creating %q: %s", victim, resp.Body.String())
			var created struct {
				ID uint `json:"ID"`
			}
			require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
			require.NotZero(t, created.ID)

			form := url.Values{}
			form.Set("name", occupied)
			resp = tc.MakeFormRequest(http.MethodPost, fmt.Sprintf("%s?id=%d", tt.editPath, created.ID), form)

			require.Equal(t, http.StatusBadRequest, resp.Code,
				"renaming onto an existing name should be a 4xx, got: %s", resp.Body.String())

			var body map[string]string
			require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body), "response was %s", resp.Body.String())
			msg := body["error"]

			assert.NotContains(t, strings.ToLower(msg), "unique constraint",
				"the driver's constraint text must not reach the UI: %q", msg)
			assert.NotContains(t, strings.ToLower(msg), "duplicate key",
				"the driver's constraint text must not reach the UI: %q", msg)
			assert.Contains(t, msg, tt.label,
				"the message should name the entity: %q", msg)
			assert.Contains(t, msg, occupied,
				"the message should quote the name that is taken: %q", msg)
		})
	}
}

// Positive control for the test above: without it, an endpoint that rejected
// every rename would satisfy the "no constraint text" assertions forever.
// Also pins the empty-name message finding 88 relies on being surfaced.
func TestEditName_SucceedsAndReportsEmptyName(t *testing.T) {
	tc := SetupTestEnv(t)

	resp := tc.MakeRequest(http.MethodPost, "/v1/tag", query_models.TagCreator{Name: "ws2 control original"})
	require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
	var tag models.Tag
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &tag))
	require.NotZero(t, tag.ID)

	form := url.Values{}
	form.Set("name", "ws2 control renamed")
	resp = tc.MakeFormRequest(http.MethodPost, fmt.Sprintf("/v1/tag/editName?id=%d", tag.ID), form)
	require.Equal(t, http.StatusOK, resp.Code, "a free name must still be accepted: %s", resp.Body.String())

	var reloaded models.Tag
	require.NoError(t, tc.DB.First(&reloaded, tag.ID).Error)
	assert.Equal(t, "ws2 control renamed", reloaded.Name, "the rename must persist")

	form = url.Values{}
	form.Set("name", "   ")
	resp = tc.MakeFormRequest(http.MethodPost, fmt.Sprintf("/v1/tag/editName?id=%d", tag.ID), form)
	require.Equal(t, http.StatusBadRequest, resp.Code, resp.Body.String())
	var body map[string]string
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "name must not be empty",
		"the empty-name message is what the inline editor is expected to render")
}
