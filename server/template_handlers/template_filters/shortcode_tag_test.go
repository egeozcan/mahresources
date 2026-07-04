package template_filters

import (
	"testing"

	"mahresources/models"
)

// Carrier types (Category/ResourceCategory/NoteType) drive the CustomListHeader
// list-header slot. buildMetaContext must accept them, resolve global scope
// (0/0/0 — a carrier is not a group), and leave Meta empty.
func TestBuildMetaContextForCarrier(t *testing.T) {
	cases := []struct {
		name           string
		entity         any
		wantEntityType string
		wantID         uint
	}{
		{"category", &models.Category{ID: 5, Name: "Projects"}, "category", 5},
		{"resourceCategory", &models.ResourceCategory{ID: 7, Name: "Photos"}, "resource_category", 7},
		{"noteType", &models.NoteType{ID: 9, Name: "Meeting"}, "note_type", 9},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// appCtx is nil: carriers must not need DB resolution for scope.
			mctx := BuildMetaContextForEntity(tc.entity, nil)
			if mctx == nil {
				t.Fatalf("expected a meta context for carrier %s, got nil", tc.name)
			}
			if mctx.EntityType != tc.wantEntityType {
				t.Errorf("EntityType = %q, want %q", mctx.EntityType, tc.wantEntityType)
			}
			if mctx.EntityID != tc.wantID {
				t.Errorf("EntityID = %d, want %d", mctx.EntityID, tc.wantID)
			}
			// Global scope is the whole point: dashboard [mrql] must not scope to a group.
			if mctx.ScopeGroupID != 0 || mctx.ParentGroupID != 0 || mctx.RootGroupID != 0 {
				t.Errorf("scope = (%d,%d,%d), want (0,0,0)", mctx.ScopeGroupID, mctx.ParentGroupID, mctx.RootGroupID)
			}
			// Carriers carry no Meta; MetaSchema is not the carrier's own meta schema.
			if len(mctx.Meta) != 0 {
				t.Errorf("Meta = %q, want empty", string(mctx.Meta))
			}
			if mctx.MetaSchema != "" {
				t.Errorf("MetaSchema = %q, want empty", mctx.MetaSchema)
			}
			if mctx.Entity == nil {
				t.Error("Entity should be the carrier itself, got nil")
			}
		})
	}
}
