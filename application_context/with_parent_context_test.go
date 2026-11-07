package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"mahresources/models"
)

func countGroups(c *MahresourcesContext) (int64, error) {
	var n int64
	err := c.db.Model(&models.Group{}).Count(&n).Error
	return n, err
}

func TestWithParentContextCancellationReachesQueries(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	parent, cancel := context.WithCancel(context.Background())
	derived := ctx.WithParentContext(parent)

	if _, err := countGroups(derived); err != nil {
		t.Fatalf("query before cancel: %v", err)
	}
	cancel()
	if _, err := countGroups(derived); !errors.Is(err, context.Canceled) {
		t.Fatalf("query after cancel = %v, want context.Canceled", err)
	}
	if _, err := countGroups(ctx); err != nil {
		t.Fatalf("the original context was affected by the copy's cancellation: %v", err)
	}
}

func TestWithParentContextKeepsScopeAndActor(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, _, _ := relationScopeFixture(t, ctx)
	principal.UserID = 42
	scoped := ctx.WithPrincipal(principal)

	total, err := countGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := countGroups(scoped)
	if err != nil {
		t.Fatal(err)
	}
	if want >= total {
		t.Fatalf("fixture does not confine: scoped %d, total %d", want, total)
	}

	derived := scoped.WithParentContext(context.Background())
	got, err := countGroups(derived)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("rebased scoped context sees %d groups, want %d (the scope filter was dropped)", got, want)
	}
	if id, ok := actingUserFromContext(derived.db.Statement.Context); !ok || id != 42 {
		t.Errorf("acting user = %d, %v; want 42", id, ok)
	}
	// The unscoped context stays unscoped.
	if n, _ := countGroups(ctx.WithParentContext(context.Background())); n != total {
		t.Errorf("unscoped rebase sees %d groups, want %d", n, total)
	}
}

func TestWithPrincipalContextBindsScopeAndReportsCancellation(t *testing.T) {
	ctx := createTestContextWithPlugins(t, t.TempDir())
	principal, _, _, _ := relationScopeFixture(t, ctx)
	principal.UserID = 7
	want, _ := countGroups(ctx.WithPrincipal(principal))

	bound, err := ctx.WithPrincipalContext(context.Background(), principal)
	if err != nil {
		t.Fatalf("WithPrincipalContext: %v", err)
	}
	if got, _ := countGroups(bound); got != want {
		t.Errorf("bound context sees %d groups, want %d", got, want)
	}
	if id, ok := actingUserFromContext(bound.db.Statement.Context); !ok || id != 7 {
		t.Errorf("acting user = %d, %v; want 7", id, ok)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	denied, err := ctx.WithPrincipalContext(cancelled, principal)
	if err == nil {
		t.Fatal("a scoped principal bound under a cancelled context returned no error")
	}
	// Fails closed: the copy still denies, it does not fall back to unscoped.
	// The count runs on a live context, so a query error cannot pass for zero.
	if all, err := countGroups(ctx); err != nil || all == 0 {
		t.Fatalf("fixture: unscoped count %d, %v; want groups to exist", all, err)
	}
	n, err := countGroups(denied.WithParentContext(context.Background()))
	if err != nil {
		t.Fatalf("count through the denied copy: %v", err)
	}
	if n != 0 {
		t.Errorf("a copy whose subtree could not be read sees %d groups, want 0", n)
	}
}

func TestTemplateRunToolCatalogueIsWellFormed(t *testing.T) {
	specs := TemplateRunToolSpecs()
	if len(specs) != 14 {
		t.Fatalf("%d tools, want 14", len(specs))
	}
	seen := map[string]bool{}
	for _, s := range specs {
		if seen[s.Name] || s.Description == "" {
			t.Errorf("tool %q duplicated or without a description", s.Name)
		}
		seen[s.Name] = true
		var schema struct {
			Type       string                    `json:"type"`
			Properties map[string]map[string]any `json:"properties"`
			Required   []string                  `json:"required"`
		}
		if err := json.Unmarshal(s.Parameters, &schema); err != nil || schema.Type != "object" {
			t.Fatalf("%s: parameters are not a JSON Schema object: %v", s.Name, err)
		}
		for _, r := range schema.Required {
			if _, ok := schema.Properties[r]; !ok {
				t.Errorf("%s: required %q is not a property", s.Name, r)
			}
		}
	}
}
