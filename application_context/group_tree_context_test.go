package application_context

import (
	"testing"

	"mahresources/models"
)

func TestGetGroupTreeRoots(t *testing.T) {
	ctx := createTestContext(t)

	// Use unique names to avoid collision with other tests via shared-cache SQLite.
	root1 := &models.Group{Name: "TreeTestRootA"}
	root2 := &models.Group{Name: "TreeTestRootB"}
	ctx.db.Create(root1)
	ctx.db.Create(root2)

	// Create a child group under root1
	child := &models.Group{Name: "TreeTestChildOfA", OwnerId: &root1.ID}
	ctx.db.Create(child)

	roots, err := ctx.GetGroupTreeRoots(200)
	if err != nil {
		t.Fatalf("GetGroupTreeRoots() error: %v", err)
	}

	// Find our specific roots by name (shared DB may contain other tests' groups)
	if len(roots) < 2 {
		t.Fatalf("expected at least 2 roots, got %d", len(roots))
	}

	var foundA, foundB bool
	for _, r := range roots {
		switch r.Name {
		case "TreeTestRootA":
			foundA = true
			if r.ChildCount != 1 {
				t.Errorf("TreeTestRootA childCount = %d, want 1", r.ChildCount)
			}
		case "TreeTestRootB":
			foundB = true
			if r.ChildCount != 0 {
				t.Errorf("TreeTestRootB childCount = %d, want 0", r.ChildCount)
			}
		}
	}
	if !foundA {
		t.Error("TreeTestRootA not found in roots")
	}
	if !foundB {
		t.Error("TreeTestRootB not found in roots")
	}
}

func TestGetGroupTreeChildren(t *testing.T) {
	ctx := createTestContext(t)

	parent := &models.Group{Name: "Parent"}
	ctx.db.Create(parent)

	child1 := &models.Group{Name: "Child 1", OwnerId: &parent.ID}
	child2 := &models.Group{Name: "Child 2", OwnerId: &parent.ID}
	ctx.db.Create(child1)
	ctx.db.Create(child2)

	// Create a grandchild under child1
	grandchild := &models.Group{Name: "Grandchild", OwnerId: &child1.ID}
	ctx.db.Create(grandchild)

	children, err := ctx.GetGroupTreeChildren(parent.ID, 50)
	if err != nil {
		t.Fatalf("GetGroupTreeChildren() error: %v", err)
	}

	if len(children) != 2 {
		t.Fatalf("expected 2 children, got %d", len(children))
	}

	// Child 1 should have 1 child (grandchild)
	found := false
	for _, c := range children {
		if c.Name == "Child 1" {
			found = true
			if c.ChildCount != 1 {
				t.Errorf("Child 1 childCount = %d, want 1", c.ChildCount)
			}
		}
	}
	if !found {
		t.Error("Child 1 not found in results")
	}
}

func TestGetGroupTreeDown(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "Root"}
	ctx.db.Create(root)

	child := &models.Group{Name: "Level 1", OwnerId: &root.ID}
	ctx.db.Create(child)

	grandchild := &models.Group{Name: "Level 2", OwnerId: &child.ID}
	ctx.db.Create(grandchild)

	rows, err := ctx.GetGroupTreeDown(root.ID, 3, 50)
	if err != nil {
		t.Fatalf("GetGroupTreeDown() error: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}

	// Verify levels
	for _, row := range rows {
		switch row.Name {
		case "Root":
			if row.Level != 0 {
				t.Errorf("Root level = %d, want 0", row.Level)
			}
		case "Level 1":
			if row.Level != 1 {
				t.Errorf("Level 1 level = %d, want 1", row.Level)
			}
		case "Level 2":
			if row.Level != 2 {
				t.Errorf("Level 2 level = %d, want 2", row.Level)
			}
		}
	}
}

func TestGetGroupTreeChildren_ReturnsEmptySliceNotNil(t *testing.T) {
	ctx := createTestContext(t)

	// Query children for a non-existent parent ID
	children, err := ctx.GetGroupTreeChildren(99999, 50)
	if err != nil {
		t.Fatalf("GetGroupTreeChildren() error: %v", err)
	}

	// Should return an empty slice, not nil
	if children == nil {
		t.Error("GetGroupTreeChildren should return empty slice, not nil, for non-existent parent")
	}

	if len(children) != 0 {
		t.Errorf("expected 0 children, got %d", len(children))
	}
}

func TestGetGroupTreeRoots_ReturnsNonNilSlice(t *testing.T) {
	ctx := createTestContext(t)

	// Query roots - even if shared DB has groups from other tests,
	// the result must be a non-nil slice (not nil) for proper JSON marshaling
	roots, err := ctx.GetGroupTreeRoots(50)
	if err != nil {
		t.Fatalf("GetGroupTreeRoots() error: %v", err)
	}

	// Should return a non-nil slice so json.Marshal produces [] not null
	if roots == nil {
		t.Error("GetGroupTreeRoots should return non-nil slice, not nil")
	}
}

func TestGetGroupTreeDown_RespectsMaxLevels(t *testing.T) {
	ctx := createTestContext(t)

	root := &models.Group{Name: "Root"}
	ctx.db.Create(root)

	child := &models.Group{Name: "Level 1", OwnerId: &root.ID}
	ctx.db.Create(child)

	grandchild := &models.Group{Name: "Level 2", OwnerId: &child.ID}
	ctx.db.Create(grandchild)

	// maxLevels=1 should only include root and direct children
	rows, err := ctx.GetGroupTreeDown(root.ID, 1, 50)
	if err != nil {
		t.Fatalf("GetGroupTreeDown() error: %v", err)
	}

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (root + 1 level), got %d", len(rows))
	}

	for _, row := range rows {
		if row.Name == "Level 2" {
			t.Error("grandchild should be excluded with maxLevels=1")
		}
	}
}
