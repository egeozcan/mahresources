package application_context

import (
	"context"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/mrql"
)

func TestNestedMRQLRequestedScopesIntersectPrincipalScope(t *testing.T) {
	ctx, db := setupMRQLRenderDataTest(t)
	if err := db.Create(&models.ResourceCategory{ID: 1, Name: "default"}).Error; err != nil {
		t.Fatal(err)
	}
	root := models.Group{Name: "root"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	rootID := root.ID
	child := models.Group{Name: "child", OwnerId: &rootID}
	outside := models.Group{Name: "outside"}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&outside).Error; err != nil {
		t.Fatal(err)
	}
	childID, outsideID := child.ID, outside.ID
	if err := db.Create(&models.Resource{Name: "inside", OwnerId: &childID, ResourceCategoryId: 1}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Resource{Name: "secret", OwnerId: &outsideID, ResourceCategoryId: 1}).Error; err != nil {
		t.Fatal(err)
	}

	scoped := ctx.WithMRQLPrincipal(context.Background(), &auth.Principal{Role: models.RoleUser, ScopeGroupID: &rootID})
	flat, err := mrql.Parse(`type = "resource" LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if err := mrql.Validate(flat); err != nil {
		t.Fatal(err)
	}
	result, err := scoped.ExecuteMRQLScoped(context.Background(), flat, outside.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 0 {
		t.Fatalf("out-of-scope nested flat query returned %d resources", len(result.Resources))
	}
	total, err := scoped.CountMRQLScoped(context.Background(), flat, outside.ID)
	if err != nil {
		t.Fatal(err)
	}
	if total != 0 {
		t.Fatalf("out-of-scope nested total = %d, want 0", total)
	}

	grouped, err := mrql.Parse(`type = "resource" GROUP BY name LIMIT 1`)
	if err != nil {
		t.Fatal(err)
	}
	if err := mrql.Validate(grouped); err != nil {
		t.Fatal(err)
	}
	grouped.EntityType = mrql.EntityResource
	groupedResult, err := scoped.ExecuteMRQLGroupedWithScope(context.Background(), grouped, outside.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(groupedResult.Groups) != 0 {
		t.Fatalf("out-of-scope nested grouped query returned %d groups", len(groupedResult.Groups))
	}

	inside, err := scoped.ExecuteMRQLScoped(context.Background(), flat, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inside.Resources) != 1 || inside.Resources[0].Name != "inside" {
		t.Fatalf("in-scope nested query = %#v, want inside resource", inside.Resources)
	}
}
