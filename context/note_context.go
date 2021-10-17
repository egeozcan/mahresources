package context

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/database_scopes"
	"mahresources/http_query"
	"mahresources/models"
)

func (ctx *MahresourcesContext) CreateOrUpdateNote(noteQuery *http_query.NoteEditor) (*models.Note, error) {
	if noteQuery.Name == "" {
		return nil, errors.New("note name needed")
	}

	var note models.Note

	if noteQuery.ID == 0 {
		note = models.Note{
			Name:        noteQuery.Name,
			Description: noteQuery.Description,
			Meta:        noteQuery.Meta,
			OwnerId:     noteQuery.OwnerId,
		}
		ctx.db.Create(&note)
	} else {
		ctx.db.First(&note, noteQuery.ID)
		note.Name = noteQuery.Name
		note.Description = noteQuery.Description
		note.Meta = noteQuery.Meta
		note.OwnerId = noteQuery.OwnerId
		ctx.db.Save(&note)
		err := ctx.db.Model(&note).Association("Groups").Clear()
		if err != nil {
			return nil, err
		}
		err = ctx.db.Model(&note).Association("Tags").Clear()
		if err != nil {
			return nil, err
		}
	}

	if len(noteQuery.Groups) > 0 {
		groups := make([]models.Group, len(noteQuery.Groups))
		for i, v := range noteQuery.Groups {
			groups[i] = models.Group{
				Model: gorm.Model{ID: v},
			}
		}
		createGroupsErr := ctx.db.Model(&note).Association("Groups").Append(&groups)

		if createGroupsErr != nil {
			return nil, createGroupsErr
		}
	}

	if len(noteQuery.Tags) > 0 {
		tags := make([]models.Tag, len(noteQuery.Tags))
		for i, v := range noteQuery.Tags {
			tags[i] = models.Tag{
				Model: gorm.Model{ID: v},
			}
		}
		createTagsErr := ctx.db.Model(&note).Association("Tags").Append(&tags)

		if createTagsErr != nil {
			return nil, createTagsErr
		}
	}

	return &note, nil
}

func (ctx *MahresourcesContext) GetNote(id uint) (*models.Note, error) {
	var note models.Note
	ctx.db.Preload(clause.Associations).First(&note, id)

	if note.ID == 0 {
		return nil, errors.New("could not load note")
	}

	return &note, nil
}

func (ctx *MahresourcesContext) GetNotes(offset, maxResults int, query *http_query.NoteQuery) (*[]models.Note, error) {
	var notes []models.Note

	ctx.db.Scopes(database_scopes.NoteQuery(query)).Limit(maxResults).Offset(int(offset)).Preload("Tags").Find(&notes)

	return &notes, nil
}

func (ctx *MahresourcesContext) GetNotesWithIds(ids *[]uint) (*[]*models.Note, error) {
	var notes []*models.Note

	if len(*ids) == 0 {
		return &notes, nil
	}

	ctx.db.Find(&notes, ids)

	return &notes, nil
}

func (ctx *MahresourcesContext) GetNoteCount(query *http_query.NoteQuery) (int64, error) {
	var note models.Note
	var count int64
	ctx.db.Scopes(database_scopes.NoteQuery(query)).Model(&note).Count(&count)

	return count, nil
}

func (ctx *MahresourcesContext) GetTagsForNotes() (*[]models.Tag, error) {
	var tags []models.Tag
	ctx.db.Raw(`SELECT
					  Count(*)
					  , id
					  , name
					from tags t
					join note_tags at on t.id = at.tag_id
					group by t.name, t.id
					order by count(*) desc
	`).Scan(&tags)

	return &tags, nil
}
