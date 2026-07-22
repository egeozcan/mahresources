package application_context

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/mrql"
)

func TestExplainMRQLForcedScopeOverridesExplicitScope(t *testing.T) {
	ctx, db := setupMRQLRenderDataTest(t)
	root := models.Group{Name: "forced-root"}
	outside := models.Group{Name: "outside"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&outside).Error; err != nil {
		t.Fatal(err)
	}
	scoped := ctx.WithMRQLPrincipal(context.Background(), &auth.Principal{Role: models.RoleUser, ScopeGroupID: &root.ID})
	parsed, err := mrql.Parse(`type = "resource" SCOPE ` + fmt.Sprint(outside.ID) + ` LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if err := mrql.Validate(parsed); err != nil {
		t.Fatal(err)
	}
	result, err := scoped.ExplainMRQL(context.Background(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Statements) != 1 {
		t.Fatalf("statements = %#v", result.Statements)
	}
	if !strings.Contains(strings.ToUpper(result.Statements[0].SQL), "RECURSIVE") {
		t.Fatalf("forced scope CTE missing: %s", result.Statements[0].SQL)
	}
	foundRoot, foundOutside := false, false
	for _, value := range result.Statements[0].Vars {
		text := fmt.Sprint(value)
		foundRoot = foundRoot || text == fmt.Sprint(root.ID)
		foundOutside = foundOutside || text == fmt.Sprint(outside.ID)
	}
	if !foundRoot || foundOutside {
		t.Fatalf("scope vars = %#v, root=%d outside=%d", result.Statements[0].Vars, root.ID, outside.ID)
	}
}

func TestExplainMRQLFingerprintDistinguishesEffectiveExplicitScope(t *testing.T) {
	ctx, db := setupMRQLRenderDataTest(t)
	group := models.Group{Name: "scoped"}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	explain := func(source string) string {
		parsed, err := mrql.Parse(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := mrql.Validate(parsed); err != nil {
			t.Fatal(err)
		}
		result, err := ctx.ExplainMRQL(context.Background(), parsed)
		if err != nil {
			t.Fatal(err)
		}
		return result.QueryFingerprint
	}
	unscoped := explain(`type = "resource" SCOPE 0 LIMIT 10`)
	scoped := explain(`type = "resource" SCOPE ` + fmt.Sprint(group.ID) + ` LIMIT 10`)
	if unscoped == scoped {
		t.Fatal("explicit unscoped and scoped effective queries share a fingerprint")
	}
}

func TestExplainMRQLDeniedScopeIsExplicitlyEmpty(t *testing.T) {
	ctx, _ := setupMRQLRenderDataTest(t)
	scoped := ctx.WithMRQLPrincipal(context.Background(), &auth.Principal{Role: models.RoleGuest})
	parsed, err := mrql.Parse(`type = "resource" LIMIT 10`)
	if err != nil {
		t.Fatal(err)
	}
	if err := mrql.Validate(parsed); err != nil {
		t.Fatal(err)
	}

	result, err := scoped.ExplainMRQLWithOptions(context.Background(), parsed, MRQLExplainOptions{NativePlan: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Statements == nil || len(result.Statements) != 0 {
		t.Fatalf("statements = %#v, want a non-nil empty slice", result.Statements)
	}
	if result.ExecutionShape.Strategy != "denied" || result.ExecutionShape.PlannedStatements != 0 || result.ExecutionShape.MinimumStatements != 0 || result.ExecutionShape.MaximumStatements != 0 {
		t.Fatalf("unexpected denied execution shape: %#v", result.ExecutionShape)
	}
	if !strings.HasPrefix(result.QueryFingerprint, mrql.QueryShapeFingerprintVersion+":") {
		t.Fatalf("missing denied-query fingerprint: %q", result.QueryFingerprint)
	}
	if len(result.Warnings) == 0 {
		t.Fatal("expected denied-scope warning")
	}
}
