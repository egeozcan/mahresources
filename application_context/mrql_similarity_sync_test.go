package application_context

import (
	"testing"

	"mahresources/hash_worker"
	"mahresources/mrql"
)

// The mrql package must not import hash_worker, so the WITHIN cap is mirrored
// as mrql.MaxSimilarityDistance. This test keeps the two constants in sync:
// pairs are only stored up to hash_worker.MaxStoredPDistance, so accepting a
// larger WITHIN would silently under-match.
func TestSimilarityDistanceCapMatchesStoredPairs(t *testing.T) {
	if mrql.MaxSimilarityDistance != hash_worker.MaxStoredPDistance {
		t.Fatalf("mrql.MaxSimilarityDistance (%d) != hash_worker.MaxStoredPDistance (%d); update the mrql constant",
			mrql.MaxSimilarityDistance, hash_worker.MaxStoredPDistance)
	}
}
