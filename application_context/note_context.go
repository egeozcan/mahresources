package application_context

import (
	"errors"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

func (ctx *MahresourcesContext) CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error) {
	if noteQuery.Name == "" {
		return nil, errors.New("note name needed")
	}

	var note models.Note

	if noteQuery.Meta == "" {
		noteQuery.Meta = "{}"
	}

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if noteQuery.ID == 0 {

		note = models.Note{
			Name:        noteQuery.Name,
			Description: noteQuery.Description,
			Meta:        []byte(noteQuery.Meta),
			OwnerId:     &noteQuery.OwnerId,
			StartDate:   parseHTMLTime(noteQuery.StartDate),
			EndDate:     parseHTMLTime(noteQuery.EndDate),
		}

		if err := tx.Create(&note).Error; err != nil {
			tx.Rollback()
			return nil, err
		}

	} else {
		if err := tx.First(&note, noteQuery.ID).Error; err != nil {
			tx.Rollback()
			return nil, err
		}

		note.Name = noteQuery.Name
		note.Description = noteQuery.Description
		note.Meta = []byte(noteQuery.Meta)
		note.OwnerId = &noteQuery.OwnerId
		note.StartDate = parseHTMLTime(noteQuery.StartDate)
		note.EndDate = parseHTMLTime(noteQuery.EndDate)

		if err := tx.Save(&note).Error; err != nil {
			tx.Rollback()
			return nil, err
		}

		if err := tx.Model(&note).Association("Groups").Clear(); err != nil {
			tx.Rollback()
			return nil, err
		}

		if err := tx.Model(&note).Association("Tags").Clear(); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if len(noteQuery.Groups) > 0 {
		groups := make([]models.Group, len(noteQuery.Groups))

		for i, v := range noteQuery.Groups {
			groups[i] = models.Group{
				ID: v,
			}
		}

		createGroupsErr := tx.Model(&note).Association("Groups").Append(&groups)

		if createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if len(noteQuery.Resources) > 0 {
		resources := make([]models.Resource, len(noteQuery.Groups))

		for i, v := range noteQuery.Resources {
			resources[i] = models.Resource{
				ID: v,
			}
		}

		createGroupsErr := tx.Model(&note).Association("Resources").Append(&resources)

		if createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if len(noteQuery.Tags) > 0 {
		tags := make([]models.Tag, len(noteQuery.Tags))

		for i, v := range noteQuery.Tags {
			tags[i] = models.Tag{
				ID: v,
			}
		}

		if createTagsErr := tx.Model(&note).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	return &note, tx.Commit().Error
}

func (ctx *MahresourcesContext) GetNote(id uint) (*models.Note, error) {
	var note models.Note

	return &note, ctx.db.Preload(clause.Associations, pageLimit).First(&note, id).Error
}

func (ctx *MahresourcesContext) GetNotes(offset, maxResults int, query *query_models.NoteQuery) (*[]models.Note, error) {
	var notes []models.Note
	noteScope := database_scopes.NoteQuery(query, false)

	return &notes, ctx.db.Scopes(noteScope).Limit(maxResults).Offset(offset).Preload("Tags").Find(&notes).Error
}

func (ctx *MahresourcesContext) GetNotesWithIds(ids *[]uint) (*[]*models.Note, error) {
	var notes []*models.Note

	if len(*ids) == 0 {
		return &notes, nil
	}

	return &notes, ctx.db.Find(&notes, ids).Error
}

func (ctx *MahresourcesContext) GetNoteCount(query *query_models.NoteQuery) (int64, error) {
	var note models.Note
	var count int64

	return count, ctx.db.Scopes(database_scopes.NoteQuery(query, true)).Model(&note).Count(&count).Error
}

func (ctx *MahresourcesContext) DeleteNote(noteId uint) error {
	note := models.Note{ID: noteId}

	return ctx.db.Select(clause.Associations).Delete(&note).Error
}

func (ctx *MahresourcesContext) NoteMetaKeys() (*[]fieldResult, error) {
	return metaKeys(ctx, "notes")
}
