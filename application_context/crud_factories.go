// Package application_context provides business logic and data access operations.
//
// This file contains factory methods for creating generic CRUD components.
//
// Migration status:
//   - Tag, Category, Query, NoteType: Full CRUD (read + write) via factories
//   - Note, Group: Read-only via factories; writes remain in dedicated context files
//     due to complex association management and transaction handling
//   - Resource: Not migrated; file upload handling is too specialized
//
// The old methods (GetTags, CreateTag, etc.) are preserved for backward compatibility
// with template context providers and other code that depends on them.
package application_context

import (
	"errors"
	"strings"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

// TagCRUD returns generic CRUD components for tags.
func (ctx *MahresourcesContext) TagCRUD() (
	*CRUDReader[models.Tag, *query_models.TagQuery],
	*CRUDWriter[models.Tag, *query_models.TagCreator],
) {
	reader := NewCRUDReader[models.Tag, *query_models.TagQuery](ctx.db, CRUDReaderConfig[*query_models.TagQuery]{
		ScopeFn:       ScopeWithIgnoreSort(database_scopes.TagQuery),
		ScopeFnNoSort: ScopeWithIgnoreSortForCount(database_scopes.TagQuery),
		PreloadAssoc:  true,
	})

	writer := NewCRUDWriter[models.Tag, *query_models.TagCreator](
		ctx.db,
		buildTag,
		"tag",
	)

	return reader, writer
}

func buildTag(creator *query_models.TagCreator) (models.Tag, error) {
	if strings.TrimSpace(creator.Name) == "" {
		return models.Tag{}, errors.New("tag name must be non-empty")
	}
	return models.Tag{
		ID:          creator.ID,
		Name:        creator.Name,
		Description: creator.Description,
	}, nil
}

// CategoryCRUD returns generic CRUD components for categories.
func (ctx *MahresourcesContext) CategoryCRUD() (
	*CRUDReader[models.Category, *query_models.CategoryQuery],
	*CRUDWriter[models.Category, *query_models.CategoryCreator],
) {
	reader := NewCRUDReader[models.Category, *query_models.CategoryQuery](ctx.db, CRUDReaderConfig[*query_models.CategoryQuery]{
		ScopeFn:      database_scopes.CategoryQuery,
		PreloadAssoc: true,
	})

	writer := NewCRUDWriter[models.Category, *query_models.CategoryCreator](
		ctx.db,
		buildCategory,
		"category",
	)

	return reader, writer
}

func buildCategory(creator *query_models.CategoryCreator) (models.Category, error) {
	if strings.TrimSpace(creator.Name) == "" {
		return models.Category{}, errors.New("category name must be non-empty")
	}
	return models.Category{
		Name:          creator.Name,
		Description:   creator.Description,
		CustomHeader:  creator.CustomHeader,
		CustomSidebar: creator.CustomSidebar,
		CustomSummary: creator.CustomSummary,
		CustomAvatar:  creator.CustomAvatar,
		MetaSchema:    creator.MetaSchema,
	}, nil
}

// ResourceCategoryCRUD returns generic CRUD components for resource categories.
func (ctx *MahresourcesContext) ResourceCategoryCRUD() (
	*CRUDReader[models.ResourceCategory, *query_models.ResourceCategoryQuery],
	*CRUDWriter[models.ResourceCategory, *query_models.ResourceCategoryCreator],
) {
	reader := NewCRUDReader[models.ResourceCategory, *query_models.ResourceCategoryQuery](ctx.db, CRUDReaderConfig[*query_models.ResourceCategoryQuery]{
		ScopeFn:      database_scopes.ResourceCategoryQuery,
		PreloadAssoc: true,
	})

	writer := NewCRUDWriter[models.ResourceCategory, *query_models.ResourceCategoryCreator](
		ctx.db,
		buildResourceCategory,
		"resourceCategory",
	)

	return reader, writer
}

func buildResourceCategory(creator *query_models.ResourceCategoryCreator) (models.ResourceCategory, error) {
	if strings.TrimSpace(creator.Name) == "" {
		return models.ResourceCategory{}, errors.New("resource category name must be non-empty")
	}
	return models.ResourceCategory{
		Name:          creator.Name,
		Description:   creator.Description,
		CustomHeader:  creator.CustomHeader,
		CustomSidebar: creator.CustomSidebar,
		CustomSummary: creator.CustomSummary,
		CustomAvatar:  creator.CustomAvatar,
		MetaSchema:    creator.MetaSchema,
	}, nil
}

// QueryCRUD returns generic CRUD components for queries.
func (ctx *MahresourcesContext) QueryCRUD() (
	*CRUDReader[models.Query, *query_models.QueryQuery],
	*CRUDWriter[models.Query, *query_models.QueryCreator],
) {
	reader := NewCRUDReader[models.Query, *query_models.QueryQuery](ctx.db, CRUDReaderConfig[*query_models.QueryQuery]{
		ScopeFn:      database_scopes.QueryQuery,
		PreloadAssoc: false, // Query model doesn't have associations to preload
	})

	writer := NewCRUDWriter[models.Query, *query_models.QueryCreator](
		ctx.db,
		buildQuery,
		"query",
	)

	return reader, writer
}

func buildQuery(creator *query_models.QueryCreator) (models.Query, error) {
	if strings.TrimSpace(creator.Name) == "" {
		return models.Query{}, errors.New("query name must be non-empty")
	}
	return models.Query{
		Name:     creator.Name,
		Text:     creator.Text,
		Template: creator.Template,
	}, nil
}

// NoteTypeCRUD returns generic CRUD components for note types.
func (ctx *MahresourcesContext) NoteTypeCRUD() (
	*CRUDReader[models.NoteType, *query_models.NoteTypeQuery],
	*CRUDWriter[models.NoteType, *query_models.NoteTypeEditor],
) {
	reader := NewCRUDReader[models.NoteType, *query_models.NoteTypeQuery](ctx.db, CRUDReaderConfig[*query_models.NoteTypeQuery]{
		ScopeFn:      database_scopes.NoteTypeQuery,
		PreloadAssoc: true,
	})

	writer := NewCRUDWriter[models.NoteType, *query_models.NoteTypeEditor](
		ctx.db,
		buildNoteType,
		"noteType",
	)

	return reader, writer
}

func buildNoteType(editor *query_models.NoteTypeEditor) (models.NoteType, error) {
	if strings.TrimSpace(editor.Name) == "" {
		return models.NoteType{}, errors.New("note type name must be non-empty")
	}
	return models.NoteType{
		ID:            editor.ID,
		Name:          editor.Name,
		Description:   editor.Description,
		CustomHeader:  editor.CustomHeader,
		CustomSidebar: editor.CustomSidebar,
		CustomSummary: editor.CustomSummary,
		CustomAvatar:  editor.CustomAvatar,
	}, nil
}

// SeriesCRUD returns generic CRUD components for series.
func (ctx *MahresourcesContext) SeriesCRUD() (
	*CRUDReader[models.Series, *query_models.SeriesQuery],
	*CRUDWriter[models.Series, *query_models.SeriesCreator],
) {
	reader := NewCRUDReader[models.Series, *query_models.SeriesQuery](ctx.db, CRUDReaderConfig[*query_models.SeriesQuery]{
		ScopeFn:       ScopeWithIgnoreSort(database_scopes.SeriesQuery),
		ScopeFnNoSort: ScopeWithIgnoreSortForCount(database_scopes.SeriesQuery),
		PreloadAssoc:  false,
	})

	writer := NewCRUDWriter[models.Series, *query_models.SeriesCreator](
		ctx.db,
		buildSeries,
		"series",
	)

	return reader, writer
}

func buildSeries(creator *query_models.SeriesCreator) (models.Series, error) {
	name := strings.TrimSpace(creator.Name)
	if name == "" {
		return models.Series{}, errors.New("series name must be non-empty")
	}
	return models.Series{
		Name: name,
		Slug: name,
		Meta: []byte("{}"),
	}, nil
}

// NoteCRUDReader returns a read-only CRUD reader for notes.
// Note writes are complex (associations, transactions) and remain in note_context.go.
func (ctx *MahresourcesContext) NoteCRUDReader() *CRUDReader[models.Note, *query_models.NoteQuery] {
	return NewCRUDReader[models.Note, *query_models.NoteQuery](ctx.db, CRUDReaderConfig[*query_models.NoteQuery]{
		ScopeFn: func(query *query_models.NoteQuery) func(db *gorm.DB) *gorm.DB {
			return database_scopes.NoteQuery(query, false, ctx.db)
		},
		ScopeFnNoSort: func(query *query_models.NoteQuery) func(db *gorm.DB) *gorm.DB {
			return database_scopes.NoteQuery(query, true, ctx.db)
		},
		PreloadAssoc:   true,
		PreloadClauses: []string{"Tags", "NoteType"},
	})
}

// GroupCRUDReader returns a read-only CRUD reader for groups.
// Group writes are complex (hierarchical, associations) and remain in group_crud_context.go.
func (ctx *MahresourcesContext) GroupCRUDReader() *CRUDReader[models.Group, *query_models.GroupQuery] {
	// Note: GroupQuery scope requires originalDB parameter and ignoreSort which we handle specially
	return NewCRUDReader[models.Group, *query_models.GroupQuery](ctx.db, CRUDReaderConfig[*query_models.GroupQuery]{
		ScopeFn: func(query *query_models.GroupQuery) func(db *gorm.DB) *gorm.DB {
			return database_scopes.GroupQuery(query, false, ctx.db)
		},
		ScopeFnNoSort: func(query *query_models.GroupQuery) func(db *gorm.DB) *gorm.DB {
			return database_scopes.GroupQuery(query, true, ctx.db)
		},
		PreloadAssoc:   true,
		PreloadClauses: []string{"Tags", "Category"},
	})
}
