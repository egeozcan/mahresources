//go:build postgres && json1 && fts5

package application_context

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
)

// SQLite has one writer, so two saves can never be inside their transactions at
// once there; on Postgres they can, and only the carrier row lock makes the
// second save read the first one's committed content as its "before". Without
// it both read the same pre-save state, and one revision's changed slots (and,
// under a real race, its number) would be computed against a state that was
// already gone.
func TestRevision_ConcurrentSavesChainOnPostgres(t *testing.T) {
	ctx := newPostgresUserJobContext(t)
	cat, err := ctx.CreateCategory(&query_models.CategoryCreator{Name: "race"})
	if err != nil {
		t.Fatal(err)
	}

	const savers = 6
	var wg sync.WaitGroup
	start := make(chan struct{})
	errs := make(chan error, savers)
	for i := 0; i < savers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			// Each save posts a full editor state that differs from every other.
			_, err := ctx.UpdateCategory(&query_models.CategoryEditor{ID: cat.ID, CategoryCreator: query_models.CategoryCreator{
				Name:          "race",
				CustomHeader:  fmt.Sprintf("header-%d", i),
				CustomSidebar: fmt.Sprintf("sidebar-%d", i%2),
			}})
			errs <- err
		}(i)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a concurrent save failed: %v", err)
		}
	}

	revs := listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)
	if len(revs) != savers+1 {
		t.Fatalf("got %d revisions, want %d (create plus one per changing save): %s", len(revs), savers+1, revisionSummary(revs))
	}
	var prev models.TemplateSnapshot
	for i, r := range revs {
		if r.Number != uint(i+1) {
			t.Fatalf("numbers must be 1..n without gaps, got %d at position %d", r.Number, i)
		}
		snap, err := models.ParseTemplateSnapshot(r.Snapshot)
		if err != nil {
			t.Fatal(err)
		}
		if i > 0 {
			var stored []string
			_ = json.Unmarshal(r.ChangedSlots, &stored)
			want := models.ChangedTemplateSlots(models.TemplateCarrierCategory, prev, snap)
			if fmt.Sprint(stored) != fmt.Sprint(want) {
				t.Errorf("revision #%d changed slots %v do not chain from #%d's content (want %v)", r.Number, stored, r.Number-1, want)
			}
		}
		prev = snap
	}
	final, _ := ctx.GetCategory(cat.ID)
	if last, _ := models.ParseTemplateSnapshot(revs[len(revs)-1].Snapshot); last["CustomHeader"] != final.CustomHeader {
		t.Errorf("the last revision (%v) must describe the stored state (%q)", last["CustomHeader"], final.CustomHeader)
	}
}
