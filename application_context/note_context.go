package application_context

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/server/interfaces"
)

func (ctx *MahresourcesContext) CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error) {
	if noteQuery.Name == "" {
		return nil, errors.New("note name needed")
	}

	var note models.Note

	if noteQuery.Meta == "" {
		noteQuery.Meta = "{}"
	}

	var noteTypeId *uint
	if noteQuery.NoteTypeId != 0 {
		noteTypeId = &noteQuery.NoteTypeId
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
			NoteTypeId:  noteTypeId,
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
		note.NoteTypeId = noteTypeId

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
		groups := BuildAssociationSlice(noteQuery.Groups, GroupFromID)

		if createGroupsErr := tx.Model(&note).Association("Groups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if len(noteQuery.Resources) > 0 {
		resources := BuildAssociationSlice(noteQuery.Resources, ResourceFromID)

		if createResourcesErr := tx.Model(&note).Association("Resources").Append(&resources); createResourcesErr != nil {
			tx.Rollback()
			return nil, createResourcesErr
		}
	}

	if len(noteQuery.Tags) > 0 {
		tags := BuildAssociationSlice(noteQuery.Tags, TagFromID)

		if createTagsErr := tx.Model(&note).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	if noteQuery.ID == 0 {
		ctx.Logger().Info(models.LogActionCreate, "note", &note.ID, note.Name, "Created note", nil)
	} else {
		ctx.Logger().Info(models.LogActionUpdate, "note", &note.ID, note.Name, "Updated note", nil)
	}

	ctx.InvalidateSearchCacheByType(EntityTypeNote)
	return &note, nil
}

func (ctx *MahresourcesContext) GetNote(id uint) (*models.Note, error) {
	var note models.Note

	return &note, ctx.db.Preload(clause.Associations, pageLimit).
		Preload("Blocks", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		First(&note, id).Error
}

func (ctx *MahresourcesContext) GetNotes(offset, maxResults int, query *query_models.NoteQuery) (*[]models.Note, error) {
	var notes []models.Note
	noteScope := database_scopes.NoteQuery(query, false)

	return &notes, ctx.db.Scopes(noteScope).Limit(maxResults).Offset(offset).Preload("Tags").Preload("NoteType").Find(&notes).Error
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
	// Load note name before deletion for audit log
	var note models.Note
	if err := ctx.db.First(&note, noteId).Error; err != nil {
		return err
	}
	noteName := note.Name

	err := ctx.db.Select(clause.Associations).Delete(&note).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "note", &noteId, noteName, "Deleted note", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeNote)
	}
	return err
}

func (ctx *MahresourcesContext) NoteMetaKeys() (*[]interfaces.MetaKey, error) {
	return metaKeys(ctx, "notes")
}

func (ctx *MahresourcesContext) GetNoteType(id uint) (*models.NoteType, error) {
	var noteType models.NoteType
	return &noteType, ctx.db.Preload(clause.Associations).First(&noteType, id).Error
}

func (ctx *MahresourcesContext) GetNoteTypes(query *query_models.NoteTypeQuery, offset, maxResults int) (*[]models.NoteType, error) {
	var noteTypes []models.NoteType
	err := ctx.db.Scopes(database_scopes.NoteTypeQuery(query)).Limit(maxResults).Offset(offset).Find(&noteTypes).Error
	return &noteTypes, err
}

func (ctx *MahresourcesContext) GetNoteTypesWithIds(ids []uint) (*[]models.NoteType, error) {
	var noteTypes []models.NoteType
	if len(ids) == 0 || (len(ids) == 1 && ids[0] == 0) {
		return &noteTypes, nil
	}
	return &noteTypes, ctx.db.Find(&noteTypes, ids).Error
}

func (ctx *MahresourcesContext) GetNoteTypesCount(query *query_models.NoteTypeQuery) (int64, error) {
	var noteType models.NoteType
	var count int64
	return count, ctx.db.Scopes(database_scopes.NoteTypeQuery(query)).Model(&noteType).Count(&count).Error
}

func (ctx *MahresourcesContext) CreateOrUpdateNoteType(query *query_models.NoteTypeEditor) (*models.NoteType, error) {
	if strings.TrimSpace(query.Name) == "" {
		return nil, errors.New("note type name must be non-empty")
	}
	isNew := query.ID == 0
	var noteType models.NoteType
	if query.ID != 0 {
		if err := ctx.db.First(&noteType, query.ID).Error; err != nil {
			return nil, err
		}
	}
	noteType.Name = query.Name
	noteType.Description = query.Description
	noteType.CustomHeader = query.CustomHeader
	noteType.CustomSidebar = query.CustomSidebar
	noteType.CustomSummary = query.CustomSummary
	noteType.CustomAvatar = query.CustomAvatar
	if err := ctx.db.Save(&noteType).Error; err != nil {
		return nil, err
	}

	if isNew {
		ctx.Logger().Info(models.LogActionCreate, "noteType", &noteType.ID, noteType.Name, "Created note type", nil)
	} else {
		ctx.Logger().Info(models.LogActionUpdate, "noteType", &noteType.ID, noteType.Name, "Updated note type", nil)
	}

	ctx.InvalidateSearchCacheByType(EntityTypeNoteType)
	return &noteType, nil
}

func (ctx *MahresourcesContext) DeleteNoteType(noteTypeId uint) error {
	// Load note type name before deletion for audit log
	var noteType models.NoteType
	if err := ctx.db.First(&noteType, noteTypeId).Error; err != nil {
		return err
	}
	noteTypeName := noteType.Name

	err := ctx.db.Select(clause.Associations).Delete(&noteType).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "noteType", &noteTypeId, noteTypeName, "Deleted note type", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeNoteType)
	}
	return err
}
