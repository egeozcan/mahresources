package hash_worker

import (
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
)

// findSimilaritiesV2 finds and stores v2 similarity pairs for a freshly hashed
// resource using the DB-native pigeonhole prefilter over the four indexed pHash
// chunk columns, then verifying candidates with a full-width popcount in Go.
//
// pHash is the probe's v2 perceptual hash; legacyDHash/legacyAHash are its imgsim
// hashes (present on every v2 row) used to populate the legacy hamming_distance
// and the a_distance columns. Pairs are stored for every candidate within
// MaxStoredPDistance; the runtime threshold filters at read time.
func (w *HashWorker) findSimilaritiesV2(resourceID uint, pHash, legacyDHash, legacyAHash uint64) {
	FindSimilaritiesV2(w.db, resourceID, pHash, legacyDHash, legacyAHash)
}

// FindSimilaritiesV2 is the DB-only core of v2 matching, usable outside the
// worker (e.g. the admin recompute job) with any *gorm.DB handle.
func FindSimilaritiesV2(db *gorm.DB, resourceID uint, pHash, legacyDHash, legacyAHash uint64) {
	chunks := SplitChunks(pHash)

	// Build a UNION of one indexed lookup per chunk column. Radius-2 neighbour
	// lists are inlined as integer literals to sidestep SQLite's bind-variable
	// limit (548 values across four chunks). UNION dedups candidates that match
	// on more than one chunk.
	var sb strings.Builder
	for i := 0; i < 4; i++ {
		if i > 0 {
			sb.WriteString("\nUNION\n")
		}
		neighbors := ChunkNeighbors(chunks[i], ChunkRadius)
		fmt.Fprintf(&sb,
			"SELECT resource_id, p_hash_int, a_hash_int, d_hash_int FROM image_hashes "+
				"WHERE p_chunk%d IN (%s) AND resource_id <> %d AND status NOT IN ('failed','flat')",
			i, inList(neighbors), resourceID)
	}

	rows, err := db.Raw(sb.String()).Rows()
	if err != nil {
		log.Printf("Hash worker: v2 candidate query failed for resource %d: %v", resourceID, err)
		return
	}
	defer rows.Close()

	var similarities []models.ResourceSimilarity
	for rows.Next() {
		var otherID uint
		var otherPHash int64
		var otherAHash, otherDHash sql.NullInt64
		if err := rows.Scan(&otherID, &otherPHash, &otherAHash, &otherDHash); err != nil {
			log.Printf("Hash worker: v2 candidate scan failed for resource %d: %v", resourceID, err)
			return
		}
		if otherID == resourceID {
			continue
		}

		pDist := HammingDistance(pHash, uint64(otherPHash))
		if pDist > MaxStoredPDistance {
			continue
		}

		pd := uint8(pDist)
		ad := uint8(HammingDistance(legacyAHash, uint64(otherAHash.Int64)))
		hd := uint8(HammingDistance(legacyDHash, uint64(otherDHash.Int64)))

		id1, id2 := resourceID, otherID
		if id1 > id2 {
			id1, id2 = id2, id1
		}
		similarities = append(similarities, models.ResourceSimilarity{
			ResourceID1:     id1,
			ResourceID2:     id2,
			HammingDistance: hd,
			PDistance:       &pd,
			ADistance:       &ad,
		})
	}
	if err := rows.Err(); err != nil {
		log.Printf("Hash worker: v2 candidate rows error for resource %d: %v", resourceID, err)
		return
	}

	if len(similarities) == 0 {
		return
	}

	// Upsert: if the legacy path already inserted the pair (hamming_distance only),
	// fill in the v2 distances; otherwise insert the full row.
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "resource_id1"}, {Name: "resource_id2"}},
		DoUpdates: clause.AssignmentColumns([]string{"p_distance", "a_distance"}),
	}).Create(&similarities).Error; err != nil {
		log.Printf("Hash worker: error saving v2 similarities for resource %d: %v", resourceID, err)
	}
}

// inList renders a slice of 16-bit values as a comma-separated SQL integer list.
func inList(vals []uint16) string {
	var sb strings.Builder
	sb.Grow(len(vals) * 6)
	for i, v := range vals {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.Itoa(int(v)))
	}
	return sb.String()
}
