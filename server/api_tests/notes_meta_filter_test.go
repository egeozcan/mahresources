package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNotesMetaFilter_DoesNotReturn500(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create a note with meta {"color": "blue"}
	note := &models.Note{
		Name:        "Blue Note",
		Description: "A note with color meta",
		Meta:        []byte(`{"color": "blue"}`),
	}
	tc.DB.Create(note)

	// Also create a tag and attach it to the note so the query JOINs with the
	// tags table (which also has a "meta" column), triggering the ambiguity.
	tag := &models.Tag{Name: "TestTag"}
	tc.DB.Create(tag)
	tc.DB.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", note.ID, tag.ID)

	// GET /v1/notes with meta filter via query params
	resp := tc.MakeRequest(http.MethodGet,
		`/v1/notes?MetaQuery.0=color:EQ:"blue"`, nil)

	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"Meta filter on notes must not return HTTP 500 (ambiguous column name: meta)")
	assert.Equal(t, http.StatusOK, resp.Code)

	var notes []models.Note
	err := json.Unmarshal(resp.Body.Bytes(), &notes)
	assert.NoError(t, err)
	assert.Len(t, notes, 1, "Should return the note matching meta color=blue")
	if len(notes) > 0 {
		assert.Equal(t, "Blue Note", notes[0].Name)
	}
}

func TestNotesMetaFilter_PopularTagsDoesNotError(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create a note with meta {"color": "blue"} and a tag.
	// GetPopularNoteTags JOINs notes with the tags table, and the unqualified
	// "meta" column reference becomes ambiguous because tags also has "meta".
	note := &models.Note{
		Name: "Blue Tagged Note",
		Meta: []byte(`{"color": "blue"}`),
	}
	tc.DB.Create(note)

	tag := &models.Tag{Name: "PopTag"}
	tc.DB.Create(tag)
	tc.DB.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", note.ID, tag.ID)

	query := &query_models.NoteQuery{
		MetaQuery: []query_models.ColumnMeta{
			{Key: "color", Value: "blue", Operation: "EQ"},
		},
	}

	tags, err := tc.AppCtx.GetPopularNoteTags(query)
	assert.NoError(t, err,
		"GetPopularNoteTags with meta filter must not fail with 'ambiguous column name: meta'")
	assert.Len(t, tags, 1, "Should find one popular tag for the matching note")
}

func TestNotesMetaFilter_TemplateRoute(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create a note with meta {"color": "blue"}
	note := &models.Note{
		Name:        "Blue Template Note",
		Description: "A note with color meta",
		Meta:        []byte(`{"color": "blue"}`),
	}
	tc.DB.Create(note)

	// Attach a tag to ensure the tags JOIN is triggered
	tag := &models.Tag{Name: "TemplateTag"}
	tc.DB.Create(tag)
	tc.DB.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?)", note.ID, tag.ID)

	// Hit the template route (HTML) which triggers the JOIN with tags
	resp := tc.MakeRequest(http.MethodGet,
		`/notes?MetaQuery.0=color:EQ:"blue"`, nil)

	assert.NotEqual(t, http.StatusInternalServerError, resp.Code,
		"Template route with meta filter must not return HTTP 500 (ambiguous column name: meta)")
}
