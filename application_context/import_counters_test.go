package application_context

import (
	"testing"
)

// TestImportApplyResult_NewCounters documents the expected fields added by BH-016.
// The counters are only meaningful once apply_import wires them — the compile
// check alone is enough to confirm the struct is extended.
func TestImportApplyResult_NewCounters(t *testing.T) {
	r := ImportApplyResult{}

	// BH-016: new counters — merged (GUID-collision policy=merge)
	_ = r.MergedGroups
	_ = r.MergedResources
	_ = r.MergedNotes

	// BH-016: new counters — linked by GUID (re-link path: existing GUID row
	// referenced by an incoming payload that targets the same entity)
	_ = r.LinkedByGUIDGroups
	_ = r.LinkedByGUIDResources
	_ = r.LinkedByGUIDNotes

	// BH-016: new counters — skipped by GUID-collision policy=skip
	_ = r.SkippedByPolicyGroups
	_ = r.SkippedByPolicyResources
	_ = r.SkippedByPolicyNotes
}

// TestHasMutations_NewCounters ensures the new counters participate in the
// "did this import actually change anything" check, so retry-safety logic
// treats a merge-only import as a mutation too.
func TestHasMutations_NewCounters(t *testing.T) {
	mergeOnly := ImportApplyResult{MergedGroups: 1}
	if !mergeOnly.HasMutations() {
		t.Fatal("expected HasMutations=true when MergedGroups>0")
	}
	linkOnly := ImportApplyResult{LinkedByGUIDResources: 1}
	if !linkOnly.HasMutations() {
		t.Fatal("expected HasMutations=true when LinkedByGUIDResources>0")
	}
	// SkippedByPolicy is NOT a mutation — the row existed before.
	skipOnly := ImportApplyResult{SkippedByPolicyNotes: 2}
	if skipOnly.HasMutations() {
		t.Fatal("expected HasMutations=false when only SkippedByPolicy>0")
	}
}
