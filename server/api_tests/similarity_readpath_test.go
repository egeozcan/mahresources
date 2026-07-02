package api_tests

import (
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// seedSimPair inserts a resource-similarity pair with explicit distances.
func seedSimPair(t *testing.T, tc *TestContext, id1, id2 uint, pDist, aDist *uint8, hamming uint8) {
	t.Helper()
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	sim := models.ResourceSimilarity{
		ResourceID1:     id1,
		ResourceID2:     id2,
		HammingDistance: hamming,
		PDistance:       pDist,
		ADistance:       aDist,
	}
	if err := tc.DB.Create(&sim).Error; err != nil {
		t.Fatalf("seed sim pair: %v", err)
	}
}

func u8(v uint8) *uint8 { return &v }

// TestSimilarity_ReadTimeThreshold: the similarity threshold filters at read time,
// so changing it changes the similar-resources list with no rehash.
func TestSimilarity_ReadTimeThreshold(t *testing.T) {
	tc := SetupTestEnv(t)

	r1 := &models.Resource{Name: "r1"}
	r2 := &models.Resource{Name: "r2"}
	if err := tc.DB.Create(r1).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Create(r2).Error; err != nil {
		t.Fatal(err)
	}

	// v2 pair at p_distance 8 (a_distance NULL so aHash filter never excludes it).
	seedSimPair(t, tc, r1.ID, r2.ID, u8(8), nil, 8)

	// Default threshold is 10: the pair surfaces.
	sims, err := tc.AppCtx.GetSimilarResources(r1.ID)
	if err != nil {
		t.Fatalf("GetSimilarResources: %v", err)
	}
	if len(sims) != 1 {
		t.Fatalf("threshold 10: expected 1 similar resource, got %d", len(sims))
	}
	if sims[0].SimilarityDistance == nil || *sims[0].SimilarityDistance != 8 {
		t.Errorf("expected SimilarityDistance 8, got %v", sims[0].SimilarityDistance)
	}

	// Lower the threshold below the pair distance — no rehash, list empties.
	if err := tc.AppCtx.Settings().Set(application_context.KeyHashSimilarityThreshold, "5", "test", "tester"); err != nil {
		t.Fatalf("set threshold: %v", err)
	}
	sims, err = tc.AppCtx.GetSimilarResources(r1.ID)
	if err != nil {
		t.Fatalf("GetSimilarResources after lowering: %v", err)
	}
	if len(sims) != 0 {
		t.Fatalf("threshold 5: expected 0 similar resources (8 > 5), got %d", len(sims))
	}

	// Raise it back — the same stored pair reappears (still no rehash).
	if err := tc.AppCtx.Settings().Set(application_context.KeyHashSimilarityThreshold, "10", "test", "tester"); err != nil {
		t.Fatalf("reset threshold: %v", err)
	}
	sims, _ = tc.AppCtx.GetSimilarResources(r1.ID)
	if len(sims) != 1 {
		t.Fatalf("threshold 10 restored: expected 1 similar resource, got %d", len(sims))
	}
}

// TestSimilarity_PreferPDistanceAndOrdering: COALESCE(p_distance, hamming_distance)
// drives both the filter and the ordering; legacy pairs (p_distance NULL) fall back
// to hamming_distance.
func TestSimilarity_PreferPDistanceAndOrdering(t *testing.T) {
	tc := SetupTestEnv(t)

	base := &models.Resource{Name: "base"}
	near := &models.Resource{Name: "near"}
	mid := &models.Resource{Name: "mid"}
	legacy := &models.Resource{Name: "legacy"}
	for _, r := range []*models.Resource{base, near, mid, legacy} {
		if err := tc.DB.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}

	// near: p_distance 1. mid: p_distance 6. legacy: no p_distance, hamming 3.
	seedSimPair(t, tc, base.ID, near.ID, u8(1), nil, 1)
	seedSimPair(t, tc, base.ID, mid.ID, u8(6), nil, 6)
	seedSimPair(t, tc, base.ID, legacy.ID, nil, nil, 3)

	sims, err := tc.AppCtx.GetSimilarResources(base.ID)
	if err != nil {
		t.Fatalf("GetSimilarResources: %v", err)
	}
	if len(sims) != 3 {
		t.Fatalf("expected 3 similar resources, got %d", len(sims))
	}
	// Ascending by effective distance: near(1), legacy(3), mid(6).
	wantOrder := []uint{near.ID, legacy.ID, mid.ID}
	for i, want := range wantOrder {
		if sims[i].ID != want {
			t.Errorf("position %d: got resource %d, want %d", i, sims[i].ID, want)
		}
	}
}

// TestSimilarity_AHashFilter: when the aHash threshold is nonzero, pairs whose
// a_distance exceeds it are filtered, but legacy pairs (a_distance NULL) survive.
func TestSimilarity_AHashFilter(t *testing.T) {
	tc := SetupTestEnv(t) // aHash threshold defaults to 5

	base := &models.Resource{Name: "base"}
	highA := &models.Resource{Name: "highA"}
	lowA := &models.Resource{Name: "lowA"}
	legacy := &models.Resource{Name: "legacy"}
	for _, r := range []*models.Resource{base, highA, lowA, legacy} {
		if err := tc.DB.Create(r).Error; err != nil {
			t.Fatal(err)
		}
	}

	seedSimPair(t, tc, base.ID, highA.ID, u8(2), u8(9), 2) // a_distance 9 > 5 → excluded
	seedSimPair(t, tc, base.ID, lowA.ID, u8(2), u8(3), 2)  // a_distance 3 <= 5 → kept
	seedSimPair(t, tc, base.ID, legacy.ID, nil, nil, 2)    // a_distance NULL → kept

	sims, err := tc.AppCtx.GetSimilarResources(base.ID)
	if err != nil {
		t.Fatalf("GetSimilarResources: %v", err)
	}
	got := map[uint]bool{}
	for _, s := range sims {
		got[s.ID] = true
	}
	if got[highA.ID] {
		t.Errorf("highA (a_distance 9) should be filtered by aHash threshold 5")
	}
	if !got[lowA.ID] {
		t.Errorf("lowA (a_distance 3) should be kept")
	}
	if !got[legacy.ID] {
		t.Errorf("legacy pair (a_distance NULL) should be kept")
	}
}
