package application_context

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/interfaces"
)

func (ctx *MahresourcesContext) CreateOrUpdateNote(noteQuery *query_models.NoteEditor) (*models.Note, error) {
	noteQuery.Name = strings.TrimSpace(noteQuery.Name)
	if noteQuery.Name == "" {
		return nil, errors.New("note name needed")
	}

	if err := ValidateEntityName(noteQuery.Name, "note"); err != nil {
		return nil, err
	}

	var note models.Note

	if noteQuery.Meta == "" {
		noteQuery.Meta = "{}"
	}

	if err := ValidateMeta(noteQuery.Meta); err != nil {
		return nil, err
	}

	var noteTypeId *uint
	if noteQuery.NoteTypeId != 0 {
		var ntCheck models.NoteType
		if err := ctx.db.Select("id").First(&ntCheck, noteQuery.NoteTypeId).Error; err != nil {
			return nil, errors.New("note type not found")
		}
		noteTypeId = &noteQuery.NoteTypeId
	}

	var ownerId *uint
	if noteQuery.OwnerId != 0 {
		var ownerCheck models.Group
		if err := ctx.db.Select("id").First(&ownerCheck, noteQuery.OwnerId).Error; err != nil {
			return nil, errors.New("owner group not found")
		}
		ownerId = &noteQuery.OwnerId
	}

	// Determine hook event based on whether an ID was supplied.
	// Note: ID != 0 is treated as an update, but if the caller passes a
	// non-existent ID the DB lookup will fail later — the hook event may
	// not reflect the actual outcome.
	hookEvent := "before_note_create"
	if noteQuery.ID != 0 {
		hookEvent = "before_note_update"
	}
	hookData := map[string]any{
		"id":          float64(noteQuery.ID),
		"name":        noteQuery.Name,
		"description": noteQuery.Description,
		"meta":        noteQuery.Meta,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks(hookEvent, hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		noteQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		noteQuery.Description = desc
	}
	if hMeta, ok := hookData["meta"].(string); ok {
		noteQuery.Meta = hMeta
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
			OwnerId:     ownerId,
			StartDate:   parseHTMLTime(noteQuery.StartDate),
			EndDate:     parseHTMLTime(noteQuery.EndDate),
			NoteTypeId:  noteTypeId,
		}

		if err := tx.Create(&note).Error; err != nil {
			tx.Rollback()
			if isForeignKeyError(err) {
				return nil, errors.New("referenced note type or owner does not exist")
			}
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
		note.OwnerId = ownerId
		note.StartDate = parseHTMLTime(noteQuery.StartDate)
		note.EndDate = parseHTMLTime(noteQuery.EndDate)
		note.NoteTypeId = noteTypeId

		if err := tx.Save(&note).Error; err != nil {
			tx.Rollback()
			if isForeignKeyError(err) {
				return nil, errors.New("referenced note type or owner does not exist")
			}
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

		if err := tx.Model(&note).Association("Resources").Clear(); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if len(noteQuery.Groups) > 0 {
		if err := ValidateAssociationIDs[models.Group](tx, noteQuery.Groups, "groups"); err != nil {
			tx.Rollback()
			return nil, err
		}
		groups := BuildAssociationSlice(noteQuery.Groups, GroupFromID)

		if createGroupsErr := tx.Model(&note).Association("Groups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if len(noteQuery.Resources) > 0 {
		if err := ValidateAssociationIDs[models.Resource](tx, noteQuery.Resources, "resources"); err != nil {
			tx.Rollback()
			return nil, err
		}
		resources := BuildAssociationSlice(noteQuery.Resources, ResourceFromID)

		if createResourcesErr := tx.Model(&note).Association("Resources").Append(&resources); createResourcesErr != nil {
			tx.Rollback()
			return nil, createResourcesErr
		}
	}

	if len(noteQuery.Tags) > 0 {
		if err := ValidateAssociationIDs[models.Tag](tx, noteQuery.Tags, "tags"); err != nil {
			tx.Rollback()
			return nil, err
		}
		tags := BuildAssociationSlice(noteQuery.Tags, TagFromID)

		if createTagsErr := tx.Model(&note).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	// Sync description to first text block if blocks exist (backward compatibility)
	if noteQuery.ID != 0 {
		var blocks []models.NoteBlock
		if err := tx.Where("note_id = ? AND type = ?", note.ID, "text").Order("position ASC").Limit(1).Find(&blocks).Error; err == nil && len(blocks) > 0 {
			content, _ := json.Marshal(map[string]string{"text": noteQuery.Description})
			tx.Model(&blocks[0]).Update("content", content)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, err
	}

	ctx.syncMentionsForNote(&note)

	if noteQuery.ID == 0 {
		ctx.Logger().Info(models.LogActionCreate, "note", &note.ID, note.Name, "Created note", nil)
	} else {
		ctx.Logger().Info(models.LogActionUpdate, "note", &note.ID, note.Name, "Updated note", nil)
	}

	afterEvent := "after_note_create"
	if noteQuery.ID != 0 {
		afterEvent = "after_note_update"
	}
	ctx.RunAfterPluginHooks(afterEvent, map[string]any{
		"id":          float64(note.ID),
		"name":        note.Name,
		"description": note.Description,
		"meta":        string(note.Meta),
	})

	ctx.InvalidateSearchCacheByType(EntityTypeNote)
	return &note, nil
}

func (ctx *MahresourcesContext) GetNote(id uint) (*models.Note, error) {
	var note models.Note

	return &note, ctx.db.Preload(clause.Associations).
		Preload("Blocks", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		First(&note, id).Error
}

func (ctx *MahresourcesContext) GetNotes(offset, maxResults int, query *query_models.NoteQuery) ([]models.Note, error) {
	var notes []models.Note
	noteScope := database_scopes.NoteQuery(query, false, ctx.db)

	return notes, ctx.db.Scopes(noteScope).Limit(maxResults).Offset(offset).Preload("Tags").Preload("NoteType").Find(&notes).Error
}

func (ctx *MahresourcesContext) GetNotesWithIds(ids *[]uint) ([]*models.Note, error) {
	var notes []*models.Note

	if len(*ids) == 0 {
		return notes, nil
	}

	return notes, ctx.db.Find(&notes, ids).Error
}

func (ctx *MahresourcesContext) GetNoteCount(query *query_models.NoteQuery) (int64, error) {
	var note models.Note
	var count int64

	return count, ctx.db.Scopes(database_scopes.NoteQuery(query, true, ctx.db)).Model(&note).Count(&count).Error
}

func (ctx *MahresourcesContext) GetPopularNoteTags(query *query_models.NoteQuery) ([]PopularTag, error) {
	var res []PopularTag

	db := ctx.db.Table("notes").
		Scopes(database_scopes.NoteQuery(query, true, ctx.db)).
		Joins("INNER JOIN note_tags pt ON pt.note_id = notes.id").
		Joins("INNER JOIN tags t ON t.id = pt.tag_id").
		Select("t.id AS id, t.name AS name, count(*) AS count").
		Group("t.id, t.name").
		Order("count DESC").
		Limit(20)

	return res, db.Scan(&res).Error
}

func (ctx *MahresourcesContext) DeleteNote(noteId uint) error {
	_, hookErr := ctx.RunBeforePluginHooks("before_note_delete", map[string]any{"id": float64(noteId)})
	if hookErr != nil {
		return hookErr
	}

	// Load note name before deletion for audit log
	var note models.Note
	if err := ctx.db.First(&note, noteId).Error; err != nil {
		return err
	}
	noteName := note.Name

	err := ctx.db.Select(clause.Associations).Delete(&note).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "note", &noteId, noteName, "Deleted note", nil)
		ctx.RunAfterPluginHooks("after_note_delete", map[string]any{"id": float64(noteId), "name": noteName})
		ctx.InvalidateSearchCacheByType(EntityTypeNote)
	}
	return err
}

func (ctx *MahresourcesContext) ShareNote(noteId uint) (string, error) {
	var note models.Note
	if err := ctx.db.First(&note, noteId).Error; err != nil {
		return "", err
	}

	// If already shared, return existing token
	if note.ShareToken != nil {
		return *note.ShareToken, nil
	}

	token := lib.GenerateShareToken()
	if err := ctx.db.Model(&note).Update("share_token", token).Error; err != nil {
		return "", err
	}

	ctx.Logger().Info(models.LogActionUpdate, "note", &noteId, note.Name, "Created share token", nil)
	return token, nil
}

func (ctx *MahresourcesContext) UnshareNote(noteId uint) error {
	var note models.Note
	if err := ctx.db.First(&note, noteId).Error; err != nil {
		return err
	}

	if err := ctx.db.Model(&note).Update("share_token", nil).Error; err != nil {
		return err
	}

	ctx.Logger().Info(models.LogActionUpdate, "note", &noteId, note.Name, "Removed share token", nil)
	return nil
}

func (ctx *MahresourcesContext) GetNoteByShareToken(token string) (*models.Note, error) {
	if token == "" {
		return nil, errors.New("share token required")
	}

	var note models.Note
	err := ctx.db.
		Preload("Blocks", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		Preload("Resources").
		Preload("NoteType").
		Where("share_token = ?", token).
		First(&note).Error

	if err != nil {
		return nil, err
	}

	return &note, nil
}

func (ctx *MahresourcesContext) NoteMetaKeys() ([]interfaces.MetaKey, error) {
	return metaKeys(ctx, "notes")
}

func (ctx *MahresourcesContext) GetNoteType(id uint) (*models.NoteType, error) {
	var noteType models.NoteType
	return &noteType, ctx.db.Preload(clause.Associations).First(&noteType, id).Error
}

func (ctx *MahresourcesContext) GetNoteTypes(query *query_models.NoteTypeQuery, offset, maxResults int) ([]models.NoteType, error) {
	var noteTypes []models.NoteType
	err := ctx.db.Scopes(database_scopes.NoteTypeQuery(query)).Limit(maxResults).Offset(offset).Find(&noteTypes).Error
	return noteTypes, err
}

func (ctx *MahresourcesContext) GetNoteTypesWithIds(ids []uint) ([]models.NoteType, error) {
	var noteTypes []models.NoteType
	if len(ids) == 0 || (len(ids) == 1 && ids[0] == 0) {
		return noteTypes, nil
	}
	return noteTypes, ctx.db.Find(&noteTypes, ids).Error
}

func (ctx *MahresourcesContext) GetNoteTypesCount(query *query_models.NoteTypeQuery) (int64, error) {
	var noteType models.NoteType
	var count int64
	return count, ctx.db.Scopes(database_scopes.NoteTypeQuery(query)).Model(&noteType).Count(&count).Error
}

func (ctx *MahresourcesContext) CreateOrUpdateNoteType(query *query_models.NoteTypeEditor) (*models.NoteType, error) {
	isNew := query.ID == 0
	var noteType models.NoteType
	if query.ID != 0 {
		if err := ctx.db.First(&noteType, query.ID).Error; err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(query.Name) != "" {
		if err := ValidateEntityName(query.Name, "note type"); err != nil {
			return nil, err
		}
		noteType.Name = query.Name
	} else if isNew {
		return nil, errors.New("note type name must be non-empty")
	}
	noteType.Description = query.Description
	noteType.CustomHeader = query.CustomHeader
	noteType.CustomSidebar = query.CustomSidebar
	noteType.CustomSummary = query.CustomSummary
	noteType.CustomAvatar = query.CustomAvatar
	noteType.CustomMRQLResult = query.CustomMRQLResult
	noteType.MetaSchema = query.MetaSchema
	if query.SectionConfig != "" {
		noteType.SectionConfig = types.JSON(query.SectionConfig)
	}
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

	// Do NOT use Select(clause.Associations) here — NoteType's only association
	// is Notes, and deleting a type must SET NULL on notes (not cascade-delete them).
	// Explicitly clear NoteTypeId since SQLite's PRAGMA foreign_keys is a no-op
	// inside transactions, so FK constraints (OnDelete:SET NULL) don't fire reliably.
	if err := ctx.db.Model(&models.Note{}).Where("note_type_id = ?", noteTypeId).Update("note_type_id", nil).Error; err != nil {
		return err
	}

	err := ctx.db.Delete(&noteType).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "noteType", &noteTypeId, noteTypeName, "Deleted note type", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeNoteType)
	}
	return err
}

// AddTagsToNote appends tags to a note by ID.
func (ctx *MahresourcesContext) AddTagsToNote(noteId uint, tagIds []uint) error {
	if err := ValidateAssociationIDs[models.Tag](ctx.db, tagIds, "tags"); err != nil {
		return err
	}
	note := models.Note{ID: noteId}
	tags := BuildAssociationSlice(tagIds, TagFromID)
	return ctx.db.Model(&note).Association("Tags").Append(&tags)
}

// RemoveTagsFromNote removes tags from a note by ID.
func (ctx *MahresourcesContext) RemoveTagsFromNote(noteId uint, tagIds []uint) error {
	note := models.Note{ID: noteId}
	tags := BuildAssociationSlice(tagIds, TagFromID)
	return ctx.db.Model(&note).Association("Tags").Delete(&tags)
}

// AddGroupsToNote appends groups to a note by ID.
func (ctx *MahresourcesContext) AddGroupsToNote(noteId uint, groupIds []uint) error {
	if err := ValidateAssociationIDs[models.Group](ctx.db, groupIds, "groups"); err != nil {
		return err
	}
	note := models.Note{ID: noteId}
	groups := BuildAssociationSlice(groupIds, GroupFromID)
	return ctx.db.Model(&note).Association("Groups").Append(&groups)
}

// RemoveGroupsFromNote removes groups from a note by ID.
func (ctx *MahresourcesContext) RemoveGroupsFromNote(noteId uint, groupIds []uint) error {
	note := models.Note{ID: noteId}
	groups := BuildAssociationSlice(groupIds, GroupFromID)
	return ctx.db.Model(&note).Association("Groups").Delete(&groups)
}

// AddResourcesToNote appends resources to a note by ID.
func (ctx *MahresourcesContext) AddResourcesToNote(noteId uint, resourceIds []uint) error {
	if err := ValidateAssociationIDs[models.Resource](ctx.db, resourceIds, "resources"); err != nil {
		return err
	}
	note := models.Note{ID: noteId}
	resources := BuildAssociationSlice(resourceIds, ResourceFromID)
	return ctx.db.Model(&note).Association("Resources").Append(&resources)
}

// RemoveResourcesFromNote removes resources from a note by ID.
func (ctx *MahresourcesContext) RemoveResourcesFromNote(noteId uint, resourceIds []uint) error {
	note := models.Note{ID: noteId}
	resources := BuildAssociationSlice(resourceIds, ResourceFromID)
	return ctx.db.Model(&note).Association("Resources").Delete(&resources)
}
