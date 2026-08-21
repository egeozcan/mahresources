package groupio

import (
	"strings"
	"testing"

	"mahresources/archive"
	"mahresources/models"
)

// The shell-group `map_to_existing` destination is the other place a
// caller-chosen group id enters idMap, and idMap is what every later edge
// construction reads. The guard for it exists (apply_import.go); nothing
// pinned it, so deleting it left the whole suite green.
//
// That mattered more than the usual "untested guard" because the sibling
// dangling-ref guard IS mutation-tested, which made the pair look covered.
//
// Both directions are asserted here. The refusal direction is what was missing;
// the permissive direction is what stops the guard being "fixed" by refusing
// everything, and apply_import_test.go's two map_to_existing tests run under
// the allow-all testScope, so they would not notice.
func newShellGroupState(t *testing.T, base *opCtx, destination *models.Group, allowed map[uint]bool) *applyState {
	t.Helper()

	opctx := &opCtx{
		Service: base.Service,
		db:      base.db,
		scope:   restrictedScope{testScope: testScope{db: base.db}, allowed: allowed},
	}

	return &applyState{
		ctx: opctx,
		collector: &importDataCollector{
			// Shell is read off the payload, not off the plan item: applyGroups
			// looks the payload up first and branches on gp.Shell.
			groups: map[string]*archive.GroupPayload{
				"g-shell": {Name: "shell-group", Shell: true},
			},
		},
		plan: &ImportPlan{Items: []ImportPlanItem{{Kind: "group", ExportID: "g-shell"}}},
		decisions: &ImportDecisions{ShellGroupActions: map[string]ShellGroupAction{
			"g-shell": {Action: "map_to_existing", DestinationID: &destination.ID},
		}},
		idMap:      map[string]uint{},
		skippedM2M: map[string]bool{},
		result:     &ImportApplyResult{},
	}
}

func TestApplyGroups_ShellGroupDestinationOutsideScopeIsRefused(t *testing.T) {
	base := createTestContext(t)

	category := &models.Category{Name: "shell-scope-cat"}
	if err := base.db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	outside := &models.Group{Name: "shell-theirs", CategoryId: &category.ID}
	if err := base.db.Create(outside).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	// Nothing is visible: the destination is outside the importer's subtree.
	st := newShellGroupState(t, base, outside, map[uint]bool{})

	if err := st.applyGroups(); err != nil {
		t.Fatalf("applyGroups: %v", err)
	}

	// The id must not reach idMap — that is the containment, not the warning.
	if got, ok := st.idMap["g-shell"]; ok {
		t.Errorf("idMap gained the out-of-scope destination (%d); every later edge reads this map", got)
	}
	if st.result.MappedShellGroups != 0 {
		t.Errorf("MappedShellGroups is %d, want 0", st.result.MappedShellGroups)
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

func TestApplyGroups_ShellGroupDestinationInScopeIsMapped(t *testing.T) {
	base := createTestContext(t)

	category := &models.Category{Name: "shell-scope-cat-ok"}
	if err := base.db.Create(category).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	inside := &models.Group{Name: "shell-mine", CategoryId: &category.ID}
	if err := base.db.Create(inside).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	st := newShellGroupState(t, base, inside, map[uint]bool{inside.ID: true})

	if err := st.applyGroups(); err != nil {
		t.Fatalf("applyGroups: %v", err)
	}

	if st.idMap["g-shell"] != inside.ID {
		t.Errorf("idMap has %d for the shell group, want %d: the guard over-refuses", st.idMap["g-shell"], inside.ID)
	}
	if st.result.MappedShellGroups != 1 {
		t.Errorf("MappedShellGroups is %d, want 1", st.result.MappedShellGroups)
	}
}
