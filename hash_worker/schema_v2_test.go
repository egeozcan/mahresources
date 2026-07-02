package hash_worker

import (
	"testing"

	"mahresources/models"
)

// TestV2Schema_RoundTrip verifies AutoMigrate adds the v2 columns and a v2 row
// (version, pHash, chunks, status) plus a v2 similarity pair round-trip cleanly.
func TestV2Schema_RoundTrip(t *testing.T) {
	db := setupTestDB(t)

	resource := models.Resource{Name: "v2"}
	if err := db.Create(&resource).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	pHash := uint64(0x0123456789ABCDEF)
	chunks := SplitChunks(pHash)
	ver := 2
	pSigned := int64(pHash)
	c0, c1, c2, c3 := int32(chunks[0]), int32(chunks[1]), int32(chunks[2]), int32(chunks[3])

	h := models.ImageHash{
		ResourceId:  &resource.ID,
		HashVersion: &ver,
		PHashInt:    &pSigned,
		PChunk0:     &c0,
		PChunk1:     &c1,
		PChunk2:     &c2,
		PChunk3:     &c3,
		Status:      models.HashStatusOK,
	}
	if err := db.Create(&h).Error; err != nil {
		t.Fatalf("create v2 hash: %v", err)
	}

	var got models.ImageHash
	if err := db.First(&got, h.ID).Error; err != nil {
		t.Fatalf("load hash: %v", err)
	}
	if !got.IsV2() {
		t.Errorf("expected IsV2 true")
	}
	if got.GetPHash() != pHash {
		t.Errorf("GetPHash = %#x, want %#x", got.GetPHash(), pHash)
	}
	if got.PChunk0 == nil || *got.PChunk0 != int32(chunks[0]) {
		t.Errorf("PChunk0 round-trip failed")
	}
	if got.Status != models.HashStatusOK {
		t.Errorf("Status = %q, want ok", got.Status)
	}

	// A legacy v1 row leaves the new columns NULL / default.
	legacy := models.ImageHash{ResourceId: nil, DHash: "abc"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy hash: %v", err)
	}
	var gotLegacy models.ImageHash
	if err := db.First(&gotLegacy, legacy.ID).Error; err != nil {
		t.Fatalf("load legacy: %v", err)
	}
	if gotLegacy.IsV2() {
		t.Errorf("legacy row should not be v2")
	}
	if gotLegacy.PHashInt != nil {
		t.Errorf("legacy PHashInt should be nil")
	}

	// v2 similarity pair with distances round-trips.
	pd, ad := uint8(3), uint8(2)
	sim := models.ResourceSimilarity{
		ResourceID1:     1,
		ResourceID2:     2,
		HammingDistance: 4,
		PDistance:       &pd,
		ADistance:       &ad,
	}
	if err := db.Create(&sim).Error; err != nil {
		t.Fatalf("create similarity: %v", err)
	}
	var gotSim models.ResourceSimilarity
	if err := db.First(&gotSim, sim.ID).Error; err != nil {
		t.Fatalf("load similarity: %v", err)
	}
	if gotSim.PDistance == nil || *gotSim.PDistance != 3 {
		t.Errorf("PDistance round-trip failed: %v", gotSim.PDistance)
	}
	if gotSim.ADistance == nil || *gotSim.ADistance != 2 {
		t.Errorf("ADistance round-trip failed: %v", gotSim.ADistance)
	}
}
