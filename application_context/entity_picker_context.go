package application_context

import (
	"fmt"
	"math"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/mrql"
)

// BrowseEntities queries at most one page plus a sentinel identity, then hydrates
// only the display page. Both reads use this context's principal/transaction.
func (ctx *MahresourcesContext) BrowseEntities(q *query_models.EntityPickerQuery) (*contracts.EntityPickerPage, error) {
	if q == nil || q.Page < 0 || q.Page > math.MaxInt/query_models.EntityPickerPageSize {
		return nil, fmt.Errorf("%w: invalid page", query_models.ErrEntityPickerInput)
	}
	page := max(q.Page, 1)
	db, table, err := ctx.pickerQuery(q.Entity, q.Filter, q.Constraints)
	if err != nil {
		return nil, err
	}
	var ids []uint
	// Pluck runs the scoped Query callbacks; Scan would bypass them.
	err = db.Distinct(table+".id").Order(clause.OrderByColumn{Column: clause.Column{Table: table, Name: "id"}}).
		Limit(query_models.EntityPickerPageSize+1).Offset((page-1)*query_models.EntityPickerPageSize).
		Pluck(table+".id", &ids).Error
	if err != nil {
		return nil, err
	}
	hasNext := len(ids) > query_models.EntityPickerPageSize
	if hasNext {
		ids = ids[:query_models.EntityPickerPageSize]
	}
	// Rebuild, rather than retaining the Pluck's projection/limit/statement state.
	// Reapplying predicates also drops rows whose eligibility changed meanwhile.
	db, table, err = ctx.pickerQuery(q.Entity, q.Filter, q.Constraints)
	if err != nil {
		return nil, err
	}
	rows, err := hydratePickerEntities(db, table, q.Entity, ids)
	if err != nil {
		return nil, err
	}
	return &contracts.EntityPickerPage{Items: rows, Page: page, HasNext: hasNext}, nil
}

// ResolvePickerEntities revalidates explicit pending choices without the
// browser's editable text/page filter. Missing or no-longer-visible rows are
// omitted, allowing the caller to refuse the whole confirmation atomically.
func (ctx *MahresourcesContext) ResolvePickerEntities(q *query_models.EntityPickerResolveQuery) ([]contracts.EntityPickerEntity, error) {
	if q == nil || len(q.IDs) == 0 || len(q.IDs) > query_models.EntityPickerPageSize {
		return nil, fmt.Errorf("%w: resolve requires 1 to 50 distinct ids", query_models.ErrEntityPickerInput)
	}
	seen := make(map[uint]bool, len(q.IDs))
	for _, id := range q.IDs {
		if id == 0 || seen[id] {
			return nil, fmt.Errorf("%w: ids must be nonzero and distinct", query_models.ErrEntityPickerInput)
		}
		seen[id] = true
	}
	db, table, err := ctx.pickerQuery(q.Entity, "", q.Constraints)
	if err != nil {
		return nil, err
	}
	return hydratePickerEntities(db, table, q.Entity, q.IDs)
}

func (ctx *MahresourcesContext) pickerQuery(entity, filter, constraints string) (*gorm.DB, string, error) {
	db, table, err := ctx.pickerFilteredDB(entity, filter)
	if err != nil {
		return nil, "", err
	}
	if constraints != "" {
		allowed, _, err := ctx.pickerFilteredDB(entity, constraints)
		if err != nil {
			return nil, "", err
		}
		// Independent predicates avoid duplicate join aliases and preserve AND
		// semantics even when both searches constrain the same property.
		db = db.Where(table+".id IN (?)", allowed.Select(table+".id"))
	}
	return db, table, nil
}

func (ctx *MahresourcesContext) pickerFilteredDB(entity, filter string) (*gorm.DB, string, error) {
	decoded, err := query_models.DecodeEntityPickerFilter(entity, filter)
	if err != nil {
		return nil, "", err
	}
	var db *gorm.DB
	var table, expression string
	var mrqlEntity mrql.EntityType
	switch q := decoded.(type) {
	case *query_models.ResourceSearchQuery:
		db, table = ctx.db.Model(&models.Resource{}).Scopes(database_scopes.ResourceQuery(q, true, ctx.db)), "resources"
		expression, mrqlEntity = q.MRQL, mrql.EntityResource
	case *query_models.GroupQuery:
		db, table = ctx.db.Model(&models.Group{}).Scopes(database_scopes.GroupQuery(q, true, ctx.db)), "groups"
		expression, mrqlEntity = q.MRQL, mrql.EntityGroup
	case *query_models.NoteQuery:
		db, table = ctx.db.Model(&models.Note{}).Scopes(database_scopes.NoteQuery(q, true, ctx.db)), "notes"
		expression, mrqlEntity = q.MRQL, mrql.EntityNote
	case *query_models.CategoryQuery:
		db, table = ctx.db.Model(&models.Category{}).Scopes(database_scopes.CategoryQuery(q, true)), "categories"
	case *query_models.NoteTypeQuery:
		db, table = ctx.db.Model(&models.NoteType{}).Scopes(database_scopes.NoteTypeQuery(q)), "note_types"
	case *query_models.ResourceCategoryQuery:
		db, table = ctx.db.Model(&models.ResourceCategory{}).Scopes(database_scopes.ResourceCategoryQuery(q)), "resource_categories"
	case *query_models.TagQuery:
		db, table = ctx.db.Model(&models.Tag{}).Scopes(database_scopes.TagQuery(q, true)), "tags"
	case *query_models.QueryQuery:
		db, table = ctx.db.Model(&models.Query{}).Scopes(database_scopes.QueryQuery(q, true)), "queries"
	case *query_models.RelationshipTypeQuery:
		db, table = ctx.db.Model(&models.GroupRelationType{}).Scopes(database_scopes.RelationTypeQuery(q)), "group_relation_types"
	case *query_models.SeriesQuery:
		db, table = ctx.db.Model(&models.Series{}).Scopes(database_scopes.SeriesQuery(q, true)), "series"
	default:
		return nil, "", fmt.Errorf("%w: unsupported filter", query_models.ErrEntityPickerInput)
	}
	if expression != "" {
		db, err = ctx.applyMRQLFilter(db, mrqlEntity, expression)
		if err != nil {
			return nil, "", err
		}
	}
	return db, table, nil
}

func hydratePickerEntities(db *gorm.DB, table, entity string, ids []uint) ([]contracts.EntityPickerEntity, error) {
	if len(ids) == 0 {
		return []contracts.EntityPickerEntity{}, nil
	}
	db = db.Where(table+".id IN ?", ids)
	// Only bounded, identifying associations. In particular do not preload
	// series resources, note blocks or group descendants for a picker card.
	switch entity {
	case "resource":
		return pickerRows[models.Resource](db.Preload("Tags").Preload("Owner").Preload("ResourceCategory").Preload("Series"), ids)
	case "group":
		return pickerRows[models.Group](db.Preload("Tags").Preload("Owner.Category").Preload("Category"), ids)
	case "note":
		return pickerRows[models.Note](db.Preload("Tags").Preload("Owner").Preload("NoteType"), ids)
	case "category":
		return pickerRows[models.Category](db, ids)
	case "noteType":
		return pickerRows[models.NoteType](db, ids)
	case "resourceCategory":
		return pickerRows[models.ResourceCategory](db, ids)
	case "tag":
		return pickerRows[models.Tag](db, ids)
	case "query":
		return pickerRows[models.Query](db, ids)
	case "relationType":
		return pickerRows[models.GroupRelationType](db, ids)
	case "series":
		return pickerRows[models.Series](db, ids)
	default:
		return nil, fmt.Errorf("%w: unsupported entity type", query_models.ErrEntityPickerInput)
	}
}

func pickerRows[T interface {
	GetId() uint
	GetName() string
}](db *gorm.DB, ids []uint) ([]contracts.EntityPickerEntity, error) {
	var rows []T
	if err := db.Find(&rows).Error; err != nil {
		return nil, err
	}
	byID := make(map[uint]contracts.EntityPickerEntity, len(rows))
	for _, row := range rows {
		name := row.GetName()
		// Group.GetName truncates for legacy summaries. A picker must retain
		// the distinguishing suffix of a long, otherwise identical name.
		if group, ok := any(row).(models.Group); ok {
			name = group.Name
		}
		byID[row.GetId()] = contracts.EntityPickerEntity{ID: row.GetId(), Name: name, Raw: row}
	}
	ordered := make([]contracts.EntityPickerEntity, 0, len(rows))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
			ordered = append(ordered, row)
		}
	}
	return ordered, nil
}
