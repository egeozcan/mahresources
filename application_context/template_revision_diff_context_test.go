package application_context

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
)

// A Generation run diffs revisions under the bounded table: a field past it
// takes the coarse delete-then-insert form instead of a 25M-cell table, and a
// cancelled run stops the diff.
func TestDiffTemplateRevisionsContextIsBoundedAndCancellable(t *testing.T) {
	ctx := createTestContext(t)
	var a, b []string
	for i := 0; i < 1100; i++ {
		a = append(a, fmt.Sprintf("line %d", i))
		if i%2 == 0 {
			b = append(b, fmt.Sprintf("line %d", i))
		} else {
			b = append(b, fmt.Sprintf("changed %d", i))
		}
	}
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "big", CustomHeader: strings.Join(a, "\n")})
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) { e.CustomHeader = strings.Join(b, "\n") }); err != nil {
		t.Fatal(err)
	}
	count := func(slots []models.SlotDiff) int {
		n := 0
		for _, s := range slots {
			for _, f := range s.Fields {
				n += len(f.Hunks)
			}
		}
		return n
	}
	full, err := ctx.DiffTemplateRevisions(models.TemplateCarrierCategory, cat.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	bounded, err := ctx.DiffTemplateRevisionsContext(context.Background(), models.TemplateCarrierCategory, cat.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if count(full.Slots) < 100 || count(bounded.Slots) > 3 {
		t.Fatalf("hunks: full %d, bounded %d; want a line diff and the coarse form", count(full.Slots), count(bounded.Slots))
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ctx.DiffTemplateRevisionsContext(cancelled, models.TemplateCarrierCategory, cat.ID, 1, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled diff err = %v", err)
	}
}
