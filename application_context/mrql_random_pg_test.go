//go:build postgres

package application_context

import (
	"context"
	"fmt"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/mrql"
)

func TestMRQLRandomPostgresExecutionAndExplain(t *testing.T) {
	ctx := newPostgresPluginContext(t, nil)
	for i := 0; i < 80; i++ {
		r := models.Resource{Name: fmt.Sprintf("random-resource-%d", i), Meta: types.JSON(`{"score":10}`)}
		if err := ctx.db.Create(&r).Error; err != nil {
			t.Fatal(err)
		}
	}
	source := `type = resource AND meta.score = 10 ORDER BY RANDOM() LIMIT 50`
	result, err := ctx.ExecuteMRQL(context.Background(), source, 0, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 50 {
		t.Fatalf("got %d resources, want 50", len(result.Resources))
	}
	seen := map[uint]bool{}
	for _, resource := range result.Resources {
		if seen[resource.ID] {
			t.Fatalf("duplicate resource %d", resource.ID)
		}
		seen[resource.ID] = true
		if resource.Name == "" || string(resource.Meta) == "" {
			t.Fatalf("missing payload: %+v", resource)
		}
	}
	parsed, err := mrql.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	explained, err := ctx.ExplainMRQLWithOptions(context.Background(), parsed, MRQLExplainOptions{NativePlan: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(explained.Statements) != 1 || explained.ExecutionShape.PlannedStatements != 1 {
		t.Fatalf("random selection must remain one statement: %+v", explained.ExecutionShape)
	}
}
