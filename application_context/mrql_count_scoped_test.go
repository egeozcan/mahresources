package application_context

import (
	"context"
	"fmt"
	"testing"

	"mahresources/models"
	"mahresources/mrql"
)

// TestCountMRQLScopedIgnoresLimit verifies CountMRQLScoped returns the true row
// count regardless of the query's LIMIT, while ExecuteMRQLScoped honors it — the
// two share the same WHERE so the count is the total the "view all" link leads to.
func TestCountMRQLScopedIgnoresLimit(t *testing.T) {
	ctx := setupSharedCacheTestContext(t)

	for i := 0; i < 5; i++ {
		if err := ctx.db.Create(&models.Resource{Name: fmt.Sprintf("cnt-res-%d", i)}).Error; err != nil {
			t.Fatalf("seed resource: %v", err)
		}
	}

	parsed, err := mrql.Parse(`type = "resource" AND name ~ "cnt-res-" LIMIT 2`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := mrql.Validate(parsed); err != nil {
		t.Fatalf("validate: %v", err)
	}

	res, err := ctx.ExecuteMRQLScoped(context.Background(), parsed, 0)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(res.Resources) != 2 {
		t.Fatalf("expected 2 limited resources, got %d", len(res.Resources))
	}

	// Re-parse for the count (execution mutates parsed.EntityType/Limit).
	countParsed, _ := mrql.Parse(`type = "resource" AND name ~ "cnt-res-" LIMIT 2`)
	total, err := ctx.CountMRQLScoped(context.Background(), countParsed, 0)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 5 {
		t.Fatalf("expected true total 5 ignoring LIMIT, got %d", total)
	}
}

// TestCountMRQLScopedRespectsScope verifies the count applies the same scope
// subtree filter as scoped execution, so a scoped list and its total agree.
func TestCountMRQLScopedRespectsScope(t *testing.T) {
	ctx := setupSharedCacheTestContext(t)

	scope := &models.Group{Name: "scope-root"}
	if err := ctx.db.Create(scope).Error; err != nil {
		t.Fatalf("seed scope group: %v", err)
	}
	// Three resources owned by the scope group, two outside it.
	for i := 0; i < 3; i++ {
		if err := ctx.db.Create(&models.Resource{Name: fmt.Sprintf("in-%d", i), OwnerId: &scope.ID}).Error; err != nil {
			t.Fatalf("seed in-scope resource: %v", err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := ctx.db.Create(&models.Resource{Name: fmt.Sprintf("out-%d", i)}).Error; err != nil {
			t.Fatalf("seed out-of-scope resource: %v", err)
		}
	}

	countParsed, _ := mrql.Parse(`type = "resource"`)
	scoped, err := ctx.CountMRQLScoped(context.Background(), countParsed, scope.ID)
	if err != nil {
		t.Fatalf("scoped count: %v", err)
	}
	if scoped != 3 {
		t.Fatalf("expected 3 in-scope resources, got %d", scoped)
	}
}
