package application_context

import (
	"testing"
)

// TestEstimateJSONOverhead ensures small exports include a realistic estimate of
// the tar's JSON-payload overhead (manifest + per-entity JSONs + tar padding)
// so progressPercent doesn't overshoot 100% just because totalSize only counts
// blob bytes. BH-015.
func TestEstimateJSONOverhead(t *testing.T) {
	t.Run("empty plan returns a non-zero baseline for manifest", func(t *testing.T) {
		plan := &exportPlan{}
		got := estimateJSONOverhead(plan)
		if got < 1024 {
			t.Fatalf("expected >=1 KB manifest baseline, got %d", got)
		}
		if got > 8192 {
			t.Fatalf("expected <=8 KB manifest baseline, got %d", got)
		}
	})

	t.Run("plan with 10 resources + 5 notes + 3 groups produces linear overhead", func(t *testing.T) {
		plan := &exportPlan{
			groupIDs:    make([]uint, 3),
			noteIDs:     make([]uint, 5),
			resourceIDs: make([]uint, 10),
		}
		got := estimateJSONOverhead(plan)
		// 18 entities x ~1 KB + 2 KB manifest baseline = ~20 KB
		if got < 10_000 || got > 30_000 {
			t.Fatalf("expected 10-30 KB for 18 entities, got %d", got)
		}
	})

	t.Run("buildExportPlan sums overhead into totalBytes", func(t *testing.T) {
		// Integration-ish — confirms the helper is actually wired into the pipeline.
		// Skip if MahresourcesContext scaffolding is unavailable in this test package.
		t.Skip("covered by the E2E progress-cap test via real export")
	})
}
