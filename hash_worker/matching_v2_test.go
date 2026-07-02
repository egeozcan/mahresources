package hash_worker

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/models"
)

// insertV2Hash inserts a v2 image_hashes row with the given pHash and legacy
// hashes, and returns nothing. status defaults to ok.
func insertV2Hash(t *testing.T, w *HashWorker, resourceID uint, pHash, dHash, aHash uint64, status string) {
	t.Helper()
	if status == "" {
		status = models.HashStatusOK
	}
	chunks := SplitChunks(pHash)
	c0, c1, c2, c3 := int32(chunks[0]), int32(chunks[1]), int32(chunks[2]), int32(chunks[3])
	p := int64(pHash)
	d := int64(dHash)
	a := int64(aHash)
	ver := HashVersionV2
	row := models.ImageHash{
		ResourceId:  &resourceID,
		HashVersion: &ver,
		PHashInt:    &p,
		DHashInt:    &d,
		AHashInt:    &a,
		PChunk0:     &c0,
		PChunk1:     &c1,
		PChunk2:     &c2,
		PChunk3:     &c3,
		Status:      status,
	}
	if err := w.db.Create(&row).Error; err != nil {
		t.Fatalf("insert v2 hash for resource %d: %v", resourceID, err)
	}
}

func testWorker(t *testing.T) *HashWorker {
	t.Helper()
	db := setupTestDB(t)
	return New(db, afero.NewMemMapFs(), nil, Config{
		WorkerCount:           1,
		BatchSize:             100,
		PollInterval:          time.Hour,
		SimilarityThresholdFn: func() int { return 10 },
		AHashThresholdFn:      func() uint64 { return 5 },
		CacheSize:             100,
	}, nil)
}

func TestFindSimilaritiesV2_Distances(t *testing.T) {
	w := testWorker(t)

	// Probe pHash = 0. Candidates crafted at known pHash distances.
	const probeID = 100
	insertV2Hash(t, w, probeID, 0, 0, 0, models.HashStatusOK)

	// near A: pDist 5 (bits 0-4), dDist 2, aDist 1.
	insertV2Hash(t, w, 1, 0x1F, 0x3, 0x1, models.HashStatusOK)
	// near B: pDist 11 (bits 0-10) — boundary, stored.
	insertV2Hash(t, w, 2, 0x7FF, 0x0, 0x0, models.HashStatusOK)
	// far C: pDist 12 concentrated (bits 0-11) — found by query but verified out.
	insertV2Hash(t, w, 3, 0xFFF, 0x0, 0x0, models.HashStatusOK)
	// far D: pDist 12 spread 3+3+3+3 across chunks — no chunk within radius 2,
	// so the prefilter never even surfaces it.
	spread := uint64(0x7) | uint64(0x7)<<16 | uint64(0x7)<<32 | uint64(0x7)<<48
	insertV2Hash(t, w, 4, spread, 0x0, 0x0, models.HashStatusOK)
	// flat F: identical pHash but flat — excluded.
	insertV2Hash(t, w, 5, 0x0, 0x0, 0x0, models.HashStatusFlat)
	// failed G: excluded.
	insertV2Hash(t, w, 6, 0x0, 0x0, 0x0, models.HashStatusFailed)

	w.findSimilaritiesV2(probeID, 0, 0, 0)

	var sims []models.ResourceSimilarity
	if err := w.db.Order("resource_id1, resource_id2").Find(&sims).Error; err != nil {
		t.Fatalf("query sims: %v", err)
	}

	// Only resources 1 and 2 should match (within distance 11, ok status).
	got := map[uint]models.ResourceSimilarity{}
	for _, s := range sims {
		other := s.ResourceID1
		if other == probeID {
			other = s.ResourceID2
		}
		got[other] = s
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 matches (resources 1,2), got %d: %+v", len(got), sims)
	}

	a, ok := got[1]
	if !ok {
		t.Fatal("resource 1 not matched")
	}
	if a.PDistance == nil || *a.PDistance != 5 {
		t.Errorf("resource 1 p_distance = %v, want 5", a.PDistance)
	}
	if a.ADistance == nil || *a.ADistance != 1 {
		t.Errorf("resource 1 a_distance = %v, want 1", a.ADistance)
	}
	if a.HammingDistance != 2 {
		t.Errorf("resource 1 hamming_distance = %d, want 2", a.HammingDistance)
	}

	b, ok := got[2]
	if !ok {
		t.Fatal("resource 2 not matched")
	}
	if b.PDistance == nil || *b.PDistance != 11 {
		t.Errorf("resource 2 p_distance = %v, want 11", b.PDistance)
	}

	// Ordering invariant.
	for _, s := range sims {
		if s.ResourceID1 >= s.ResourceID2 {
			t.Errorf("bad ordering: %d >= %d", s.ResourceID1, s.ResourceID2)
		}
	}
}

// TestFindSimilaritiesV2_UpsertFillsLegacy verifies that when the legacy path
// inserted a pair with only hamming_distance, the v2 path fills p_distance/a_distance.
func TestFindSimilaritiesV2_UpsertFillsLegacy(t *testing.T) {
	w := testWorker(t)

	const probeID = 100
	insertV2Hash(t, w, probeID, 0, 0, 0, models.HashStatusOK)
	insertV2Hash(t, w, 1, 0x1F, 0x3, 0x1, models.HashStatusOK)

	// Legacy path already stored the pair (1,100) with hamming only.
	legacy := models.ResourceSimilarity{ResourceID1: 1, ResourceID2: probeID, HammingDistance: 2}
	if err := w.db.Create(&legacy).Error; err != nil {
		t.Fatalf("seed legacy pair: %v", err)
	}

	w.findSimilaritiesV2(probeID, 0, 0, 0)

	var sims []models.ResourceSimilarity
	if err := w.db.Find(&sims).Error; err != nil {
		t.Fatalf("query sims: %v", err)
	}
	if len(sims) != 1 {
		t.Fatalf("expected 1 pair (upserted, not duplicated), got %d", len(sims))
	}
	s := sims[0]
	if s.PDistance == nil || *s.PDistance != 5 {
		t.Errorf("p_distance = %v, want 5 (filled by v2 upsert)", s.PDistance)
	}
	if s.ADistance == nil || *s.ADistance != 1 {
		t.Errorf("a_distance = %v, want 1", s.ADistance)
	}
	if s.HammingDistance != 2 {
		t.Errorf("hamming_distance = %d, want 2 (preserved)", s.HammingDistance)
	}
}

// TestFindSimilaritiesV2_UsesChunkIndex verifies the candidate query hits a chunk
// index rather than full-scanning image_hashes (SQLite EXPLAIN QUERY PLAN).
func TestFindSimilaritiesV2_UsesChunkIndex(t *testing.T) {
	w := testWorker(t)

	// Seed enough rows that the planner prefers the index over a scan.
	for i := uint(1); i <= 2000; i++ {
		insertV2Hash(t, w, i, uint64(i)*2654435761, 0, 0, models.HashStatusOK)
	}

	neighbors := ChunkNeighbors(0x1234, ChunkRadius)
	q := fmt.Sprintf("EXPLAIN QUERY PLAN SELECT resource_id FROM image_hashes WHERE p_chunk0 IN (%s)", inList(neighbors))
	rows, err := w.db.Raw(q).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()

	var plan strings.Builder
	for rows.Next() {
		cols, _ := rows.Columns()
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		for _, v := range vals {
			plan.WriteString(fmt.Sprintf("%v ", v))
		}
		plan.WriteString("\n")
	}
	planStr := plan.String()
	if !strings.Contains(planStr, "idx_ih_pchunk0") {
		t.Errorf("expected query plan to use idx_ih_pchunk0, got:\n%s", planStr)
	}
}
