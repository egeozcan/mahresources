//go:build postgres

package application_context

import (
	"context"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/mrql"
	"testing"
)

func TestMRQLMixedListPagePostgres(t *testing.T) {
	ctx := newPostgresPluginContext(t, nil)
	for _, entity := range []any{
		&models.Note{Name: "list-pg A"}, &models.Group{Name: "list-pg B"}, &models.Resource{Name: "list-pg C"},
	} {
		require.NoError(t, ctx.db.Create(entity).Error)
	}
	parsed, err := mrql.Parse(`name ~ "list-pg" ORDER BY name ASC LIMIT 2 OFFSET 1`)
	require.NoError(t, err)
	result, total, page, err := ctx.ExecuteMRQLListPage(context.Background(), parsed, 2, 1)
	require.NoError(t, err)
	require.Equal(t, 2, total)
	require.Equal(t, 2, page)
	require.Len(t, result.Resources, 1)
	require.Empty(t, result.Groups)
	require.Empty(t, result.Notes)
}

func TestMRQLListTiedPagesPostgres(t *testing.T) {
	ctx := newPostgresPluginContext(t, nil)
	notes := make([]models.Note, 100)
	for i := range notes {
		notes[i].Name = "tie-page"
	}
	require.NoError(t, ctx.db.Create(&notes).Error)
	parsed, err := mrql.Parse(`type = note AND name = "tie-page" ORDER BY name DESC LIMIT 10 OFFSET 3`)
	require.NoError(t, err)
	require.NoError(t, mrql.Validate(parsed))
	whole, err := ctx.ExecuteMRQLParsed(context.Background(), parsed, 0, 0)
	require.NoError(t, err)
	var pagedIDs []uint
	for page := 1; page <= 2; page++ {
		result, total, actualPage, err := ctx.ExecuteMRQLListPage(context.Background(), parsed, page, 5)
		require.NoError(t, err)
		require.Equal(t, 10, total)
		require.Equal(t, page, actualPage)
		for _, note := range result.Notes {
			pagedIDs = append(pagedIDs, note.ID)
		}
	}
	expected := make([]uint, len(whole.Notes))
	for i, note := range whole.Notes {
		expected[i] = note.ID
	}
	require.Equal(t, expected, pagedIDs, "display pages must preserve the query's bounded membership and order")
}

func TestMRQLBucketListTiedPagesPostgres(t *testing.T) {
	ctx := newPostgresPluginContext(t, nil)
	notes := make([]models.Note, 100)
	for i := range notes {
		notes[i].Name = "tie-bucket"
	}
	require.NoError(t, ctx.db.Create(&notes).Error)
	parsed, err := mrql.Parse(`type = note AND name ~ "tie-bucket" GROUP BY noteType ORDER BY name DESC LIMIT 10`)
	require.NoError(t, err)
	require.NoError(t, mrql.Validate(parsed))
	parsed.EntityType = mrql.ExtractEntityType(parsed)
	whole, err := ctx.ExecuteMRQLGrouped(context.Background(), parsed)
	require.NoError(t, err)
	require.Len(t, whole.Groups, 1)
	var pagedIDs []uint
	itemOffset := 0
	for page := 1; page <= 2; page++ {
		result, err := ctx.ExecuteMRQLGrouped(WithMRQLCardPage(context.Background(), 5, itemOffset), parsed)
		require.NoError(t, err)
		require.Len(t, result.Groups, 1)
		for _, note := range result.Groups[0].Items.([]models.Note) {
			pagedIDs = append(pagedIDs, note.ID)
		}
		itemOffset = result.NextItemOffset
		if page == 1 {
			require.NotNil(t, result.NextOffset)
			require.Equal(t, 0, *result.NextOffset)
			require.Equal(t, 5, itemOffset)
		} else {
			require.Nil(t, result.NextOffset)
		}
	}
	wholeNotes := whole.Groups[0].Items.([]models.Note)
	expected := make([]uint, len(wholeNotes))
	for i, note := range wholeNotes {
		expected[i] = note.ID
	}
	require.Equal(t, expected, pagedIDs, "bucket continuations must preserve the query's bounded membership and order")
}
