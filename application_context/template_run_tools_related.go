package application_context

import (
	"errors"
	"fmt"

	"gorm.io/gorm"

	"mahresources/models"
)

// Bounded readers behind the list_related and list_members Tools (plan section
// 2). They query through Find, Pluck and Count on the entity models, never Scan,
// so a scoped context's subtree filter applies to every row they return. Every
// page is fetched with LIMIT n+1, so "more" costs one extra row and no count of
// the whole table.

// ErrTemplateRunEntityUnavailable is returned for an entity that does not exist
// and for one the account may not read. The two are one error so a model cannot
// probe the subtree boundary.
var ErrTemplateRunEntityUnavailable = errors.New("entity unavailable")

// TemplateRelatedEntry is one related entity, with its carrier (category,
// resource category or note type) when it has one.
type TemplateRelatedEntry struct {
	Type        string
	ID          uint
	Name        string
	CarrierID   uint
	CarrierName string
}

// relatedSource is one query of a paged walk: a model and the predicate that
// selects its rows. Sources are visited in order, so one offset spans them all.
type relatedSource struct {
	kind  string
	where func(db *gorm.DB) *gorm.DB
	order string
}

func relatedModel(kind string) any {
	switch kind {
	case "group":
		return &models.Group{}
	case "resource":
		return &models.Resource{}
	}
	return &models.Note{}
}

// templateRunEntityExists reports whether the context can read entity (typ, id).
func (ctx *MahresourcesContext) templateRunEntityExists(typ string, id uint) (bool, error) {
	var ids []uint
	if err := ctx.db.Model(relatedModel(typ)).Where("id = ?", id).Limit(1).Pluck("id", &ids).Error; err != nil {
		return false, err
	}
	return len(ids) == 1, nil
}

// ListTemplateRunRelated walks one step from an entity. up is the owner group;
// down is what a group owns (groups, then resources, then notes; resources and
// notes own nothing); across is links: related groups and typed group
// relations in either direction, related resources and notes of a group, and
// the groups and notes a resource is attached to, or the groups and resources
// a note is attached to. kind narrows the result to one entity type.
func (ctx *MahresourcesContext) ListTemplateRunRelated(typ string, id uint, direction, kind string, offset, limit int) ([]TemplateRelatedEntry, bool, error) {
	ok, err := ctx.templateRunEntityExists(typ, id)
	if err != nil {
		return nil, false, err
	}
	if !ok {
		return nil, false, ErrTemplateRunEntityUnavailable
	}
	ownerOf := func(db *gorm.DB) *gorm.DB {
		return db.Where("id IN (?)", ctx.db.Model(relatedModel(typ)).Select("owner_id").Where("id = ?", id))
	}
	ownedBy := func(db *gorm.DB) *gorm.DB { return db.Where("owner_id = ?", id) }
	var sources []relatedSource
	switch direction {
	case "up":
		sources = []relatedSource{{kind: "group", where: ownerOf, order: "id ASC"}}
	case "down":
		if typ == "group" {
			sources = []relatedSource{
				{kind: "group", where: ownedBy, order: "id ASC"},
				{kind: "resource", where: ownedBy, order: "id ASC"},
				{kind: "note", where: ownedBy, order: "id ASC"},
			}
		}
	case "across":
		in := func(query string) func(*gorm.DB) *gorm.DB {
			return func(db *gorm.DB) *gorm.DB { return db.Where("id IN ("+query+")", id) }
		}
		switch typ {
		case "group":
			sources = []relatedSource{
				{kind: "group", order: "id ASC", where: func(db *gorm.DB) *gorm.DB {
					return db.Where("id IN (SELECT related_group_id FROM group_related_groups WHERE group_id = ? "+
						"UNION SELECT to_group_id FROM group_relations WHERE from_group_id = ? "+
						"UNION SELECT from_group_id FROM group_relations WHERE to_group_id = ?)", id, id, id)
				}},
				{kind: "resource", order: "id ASC", where: in("SELECT resource_id FROM groups_related_resources WHERE group_id = ?")},
				{kind: "note", order: "id ASC", where: in("SELECT note_id FROM groups_related_notes WHERE group_id = ?")},
			}
		case "resource":
			sources = []relatedSource{
				{kind: "group", order: "id ASC", where: in("SELECT group_id FROM groups_related_resources WHERE resource_id = ?")},
				{kind: "note", order: "id ASC", where: in("SELECT note_id FROM resource_notes WHERE resource_id = ?")},
			}
		case "note":
			sources = []relatedSource{
				{kind: "group", order: "id ASC", where: in("SELECT group_id FROM groups_related_notes WHERE note_id = ?")},
				{kind: "resource", order: "id ASC", where: in("SELECT resource_id FROM resource_notes WHERE note_id = ?")},
			}
		}
	default:
		return nil, false, fmt.Errorf("unknown direction %q", direction)
	}
	if kind != "" {
		filtered := sources[:0:0]
		for _, s := range sources {
			if s.kind == kind {
				filtered = append(filtered, s)
			}
		}
		sources = filtered
	}
	return ctx.pageRelatedSources(sources, offset, limit)
}

// ListTemplateRunMembers lists the entities that use a carrier, newest first.
// The carrier's type names the member type: categories hold groups, resource
// categories resources, note types notes.
func (ctx *MahresourcesContext) ListTemplateRunMembers(carrierType string, carrierID uint, offset, limit int) ([]TemplateRelatedEntry, bool, error) {
	var kind, column string
	switch carrierType {
	case models.TemplateCarrierCategory:
		kind, column = "group", "category_id"
	case models.TemplateCarrierResourceCategory:
		kind, column = "resource", "resource_category_id"
	case models.TemplateCarrierNoteType:
		kind, column = "note", "note_type_id"
	default:
		return nil, false, fmt.Errorf("unknown carrier type %q", carrierType)
	}
	if err := ctx.requireCarrierExists(carrierType, carrierID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, ErrTemplateRunEntityUnavailable
		}
		return nil, false, err
	}
	return ctx.pageRelatedSources([]relatedSource{{
		kind:  kind,
		order: "created_at DESC, id DESC",
		where: func(db *gorm.DB) *gorm.DB { return db.Where(column+" = ?", carrierID) },
	}}, offset, limit)
}

// pageRelatedSources returns up to limit entries starting at offset across the
// sources in order, and whether another entry exists. Each source is counted
// only to decide how much of the offset it absorbs; rows are fetched with
// LIMIT remaining+1 so the extra row answers "more".
func (ctx *MahresourcesContext) pageRelatedSources(sources []relatedSource, offset, limit int) ([]TemplateRelatedEntry, bool, error) {
	var out []TemplateRelatedEntry
	skip := offset
	for _, src := range sources {
		need := limit + 1 - len(out)
		if need <= 0 {
			break
		}
		if skip > 0 {
			var n int64
			if err := src.where(ctx.db.Model(relatedModel(src.kind))).Count(&n).Error; err != nil {
				return nil, false, err
			}
			if int64(skip) >= n {
				skip -= int(n)
				continue
			}
		}
		rows, err := ctx.fetchRelatedRows(src, need, skip)
		if err != nil {
			return nil, false, err
		}
		skip = 0
		out = append(out, rows...)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	if err := ctx.fillRelatedCarriers(out); err != nil {
		return nil, false, err
	}
	return out, more, nil
}

func (ctx *MahresourcesContext) fetchRelatedRows(src relatedSource, limit, offset int) ([]TemplateRelatedEntry, error) {
	q := src.where(ctx.db.Model(relatedModel(src.kind))).Order(src.order).Limit(limit).Offset(offset)
	var entries []TemplateRelatedEntry
	switch src.kind {
	case "group":
		var rows []models.Group
		if err := q.Select("id", "name", "category_id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			e := TemplateRelatedEntry{Type: "group", ID: r.ID, Name: r.Name}
			if r.CategoryId != nil {
				e.CarrierID = *r.CategoryId
			}
			entries = append(entries, e)
		}
	case "resource":
		var rows []models.Resource
		if err := q.Select("id", "name", "resource_category_id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			entries = append(entries, TemplateRelatedEntry{Type: "resource", ID: r.ID, Name: r.Name, CarrierID: r.ResourceCategoryId})
		}
	default:
		var rows []models.Note
		if err := q.Select("id", "name", "note_type_id").Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			e := TemplateRelatedEntry{Type: "note", ID: r.ID, Name: r.Name}
			if r.NoteTypeId != nil {
				e.CarrierID = *r.NoteTypeId
			}
			entries = append(entries, e)
		}
	}
	return entries, nil
}

// fillRelatedCarriers names each entry's carrier with one query per type. The
// carrier tables are global and readable by every role.
func (ctx *MahresourcesContext) fillRelatedCarriers(entries []TemplateRelatedEntry) error {
	ids := map[string][]uint{}
	for _, e := range entries {
		if e.CarrierID != 0 {
			ids[e.Type] = append(ids[e.Type], e.CarrierID)
		}
	}
	names := map[string]map[uint]string{}
	load := func(kind string, model any) error {
		if len(ids[kind]) == 0 {
			return nil
		}
		type row struct {
			ID   uint
			Name string
		}
		var rows []row
		if err := ctx.db.Model(model).Select("id", "name").Where("id IN ?", ids[kind]).Find(&rows).Error; err != nil {
			return err
		}
		names[kind] = map[uint]string{}
		for _, r := range rows {
			names[kind][r.ID] = r.Name
		}
		return nil
	}
	if err := load("group", &models.Category{}); err != nil {
		return err
	}
	if err := load("resource", &models.ResourceCategory{}); err != nil {
		return err
	}
	if err := load("note", &models.NoteType{}); err != nil {
		return err
	}
	for i := range entries {
		entries[i].CarrierName = names[entries[i].Type][entries[i].CarrierID]
	}
	return nil
}
