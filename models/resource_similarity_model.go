package models

// ResourceSimilarity stores pre-computed similarity pairs between resources.
// ResourceID1 is always less than ResourceID2 to avoid duplicate pairs.
type ResourceSimilarity struct {
	ID              uint      `gorm:"primarykey"`
	ResourceID1     uint      `gorm:"index:idx_sim_r1;uniqueIndex:idx_sim_pair;index:idx_sim_r1_dist,priority:1"`
	ResourceID2     uint      `gorm:"index:idx_sim_r2;uniqueIndex:idx_sim_pair;index:idx_sim_r2_dist,priority:1"`
	HammingDistance uint8 `gorm:"index:idx_sim_r1_dist,priority:2;index:idx_sim_r2_dist,priority:2"`
	// v2 distances (image similarity v2). Nullable so legacy pairs written by the
	// dHash path leave them NULL; the read path uses COALESCE(p_distance, hamming_distance).
	PDistance       *uint8    `gorm:"index"` // v2 pHash Hamming distance
	ADistance       *uint8    // v2 aHash Hamming distance
	Resource1       *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID1"`
	Resource2       *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;foreignKey:ResourceID2"`
}
