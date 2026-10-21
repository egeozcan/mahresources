package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

func popularTagNames(tags []application_context.PopularTag) map[string]int {
	out := map[string]int{}
	for _, tag := range tags {
		out[tag.Name] = tag.Count
	}
	return out
}

// The tag sidebar of a scope-limited user counts only what they can see. The
// aggregate used to end in Scan, which runs GORM's Row chain and so skipped the
// subtree callback: a tag used only outside the subtree was listed, and so was
// its exact count.
func TestPopularTagsStopAtTheSubtree(t *testing.T) {
	tc := SetupTestEnv(t)

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)
	child, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Child", OwnerId: mine.ID})
	require.NoError(t, err)

	visible, err := tc.AppCtx.CreateTag(&query_models.TagCreator{Name: "visible-tag"})
	require.NoError(t, err)
	hidden, err := tc.AppCtx.CreateTag(&query_models.TagCreator{Name: "hidden-tag"})
	require.NoError(t, err)

	inside := addResourceWithBody(t, tc, "inside.txt", "inside")
	outside := addResourceWithBody(t, tc, "outside.txt", "outside")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", inside.ID).Update("owner_id", mine.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", outside.ID).Update("owner_id", theirs.ID).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO resource_tags (resource_id, tag_id) VALUES (?, ?), (?, ?)",
		inside.ID, visible.ID, outside.ID, hidden.ID).Error)

	insideNote := models.Note{Name: "inside note", OwnerId: &mine.ID}
	outsideNote := models.Note{Name: "outside note", OwnerId: &theirs.ID}
	require.NoError(t, tc.DB.Create(&insideNote).Error)
	require.NoError(t, tc.DB.Create(&outsideNote).Error)
	require.NoError(t, tc.DB.Exec("INSERT INTO note_tags (note_id, tag_id) VALUES (?, ?), (?, ?)",
		insideNote.ID, visible.ID, outsideNote.ID, hidden.ID).Error)

	require.NoError(t, tc.DB.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (?, ?), (?, ?)",
		child.ID, visible.ID, theirs.ID, hidden.ID).Error)

	scoped := scopedTo(t, tc, mine.ID)

	resourceTags, err := scoped.GetPopularResourceTags(&query_models.ResourceSearchQuery{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"visible-tag": 1}, popularTagNames(resourceTags), "resources")

	noteTags, err := scoped.GetPopularNoteTags(&query_models.NoteQuery{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"visible-tag": 1}, popularTagNames(noteTags), "notes")

	groupTags, err := scoped.GetPopularGroupTags(&query_models.GroupQuery{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"visible-tag": 1}, popularTagNames(groupTags), "groups")

	// An unscoped context still counts everything.
	all, err := tc.AppCtx.GetPopularResourceTags(&query_models.ResourceSearchQuery{})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"visible-tag": 1, "hidden-tag": 1}, popularTagNames(all))
}
