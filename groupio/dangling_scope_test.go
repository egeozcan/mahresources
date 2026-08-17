package groupio

import (
	"strings"
	"testing"

	"mahresources/models"
)

// restrictedScope allows exactly the ids it is given, so a test can express
// "this group is outside the importer's subtree" without building a principal —
// which is the point of ScopeResolver existing as an interface at all.
type restrictedScope struct {
	testScope
	allowed map[uint]bool
}

func (s restrictedScope) VisibleGroupIDs(ids []uint) map[uint]bool {
	visible := make(map[uint]bool, len(ids))
	for _, id := range ids {
		if s.allowed[id] {
			visible[id] = true
		}
	}
	return visible
}

// A dangling "group_relation" decision names its destination group by raw id,
// chosen by whoever submitted the decisions. group_relations is the one table on
// this path that scopeColumn does not map, so no GORM callback stands between
// that id and an INSERT — unlike the related_group branch beside it, whose
// Group stub the create callback rejects.
//
// Left unguarded, a group-limited user importing an archive could mint a typed
// edge from a group in its own subtree to any group in the database, and could
// tell a real id from an invented one by whether the insert failed on the
// foreign key.
func TestApplyDanglingDecisions_GroupRelationDestinationMustBeInScope(t *testing.T) {
	base := createTestContext(t)
	db := base.db

	category := &models.Category{Name: "dangling-scope-cat"}
	if err := db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	from := &models.Group{Name: "dangling-mine", CategoryId: &category.ID}
	dest := &models.Group{Name: "dangling-theirs", CategoryId: &category.ID}
	for _, g := range []*models.Group{from, dest} {
		if err := db.Create(g).Error; err != nil {
			t.Fatalf("create group: %v", err)
		}
	}

	// from is visible; dest is not. That is the whole fixture.
	opctx := &opCtx{
		Service: base.Service,
		db:      db,
		scope:   restrictedScope{testScope: testScope{db: db}, allowed: map[uint]bool{from.ID: true}},
	}

	st := &applyState{
		ctx: opctx,
		plan: &ImportPlan{DanglingRefs: []DanglingRefPlan{{
			ID: "d1", Kind: "group_relation", FromExportID: "g-from", RelationTypeName: "rt",
		}}},
		decisions: &ImportDecisions{DanglingActions: map[string]DanglingAction{
			"d1": {Action: "map", DestinationID: &dest.ID},
		}},
		idMap:  map[string]uint{"g-from": from.ID},
		result: &ImportApplyResult{},
	}

	if err := st.applyDanglingDecisions(); err != nil {
		t.Fatalf("applyDanglingDecisions: %v", err)
	}

	var count int64
	if err := db.Model(&models.GroupRelation{}).
		Where("from_group_id = ? AND to_group_id = ?", from.ID, dest.ID).
		Count(&count).Error; err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if count != 0 {
		t.Error("an edge was minted to a group outside the importer's scope")
	}

	found := false
	for _, w := range st.result.Warnings {
		if strings.Contains(w, "outside your scope") {
			found = true
		}
	}
	if !found {
		t.Errorf("the refusal is silent; warnings were %v", st.result.Warnings)
	}
}

// The permissive direction needs no test of its own: every other groupio test
// drives this same branch through the unrestricted testScope, so a guard that
// refused in-scope destinations would break them rather than pass unnoticed.
