package application_context

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestListTemplateRevisionsPage_BoundsTheQueryAndReportsMore(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "paged"})
	for i := 0; i < 59; i++ {
		n := i
		if err := updateCategory(ctx, cat.ID, func(ed *query_models.CategoryEditor) { ed.CustomHeader = fmt.Sprintf("h%d", n) }); err != nil {
			t.Fatal(err)
		}
	}

	var sqls []string
	name := "revpage-test:capture"
	if err := ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "template_revisions" {
			sqls = append(sqls, tx.Statement.SQL.String())
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })

	page, more, err := ctx.ListTemplateRevisionsPage(models.TemplateCarrierCategory, cat.ID, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 50 || !more {
		t.Fatalf("first page = %d more=%v, want 50 true", len(page), more)
	}
	if page[0].Number != 60 {
		t.Errorf("newest first: first number = %d, want 60", page[0].Number)
	}
	asserted := false
	for _, s := range sqls {
		if strings.Contains(s, "LIMIT") {
			asserted = true
			if !strings.Contains(s, "LIMIT 51") {
				t.Errorf("query is not bounded to limit+1: %s", s)
			}
		}
	}
	if !asserted {
		t.Fatalf("no LIMIT query captured: %v", sqls)
	}

	page, more, err = ctx.ListTemplateRevisionsPage(models.TemplateCarrierCategory, cat.ID, 50, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 10 || more {
		t.Fatalf("second page = %d more=%v, want 10 false", len(page), more)
	}
	if page[9].Number != 1 {
		t.Errorf("last number = %d, want 1", page[9].Number)
	}
}

func TestListTemplateRevisionsPage_KeepsTheCapabilityGateAndExistenceCheck(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})
	editor := ctx.WithPrincipal(&auth.Principal{UserID: 2, Role: models.RoleEditor})
	if _, _, err := editor.ListTemplateRevisionsPage(models.TemplateCarrierCategory, cat.ID, 50, 0); !errors.Is(err, ErrRoleCapability) {
		t.Errorf("editor on a category: err = %v, want ErrRoleCapability", err)
	}
	admin := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleAdmin})
	if _, _, err := admin.ListTemplateRevisionsPage(models.TemplateCarrierCategory, 99999, 50, 0); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Errorf("missing carrier: err = %v, want record not found", err)
	}
}
