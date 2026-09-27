package mrql

import "fmt"

// SimilarPairDistance is the perceptual distance of one stored similarity pair:
// the v2 pHash distance, or the legacy dHash distance for a pair stored before
// v2. qualifier names the resource_similarities table or its alias.
func SimilarPairDistance(qualifier string) string {
	return fmt.Sprintf("COALESCE(%s.p_distance, %s.hamming_distance)", qualifier, qualifier)
}

// SimilarPairPredicate is the one rule every read of the stored similarity pairs
// applies, so SIMILAR TO, a resource's similar list and a Resource Reduction's
// Near-Identical tier cannot disagree about which images match: the distance is
// within `within`, and when aHashThreshold is nonzero the pair's aHash distance
// is within it too (the secondary guard). A legacy pair with no aHash distance
// passes the guard. Both bounds are integers, inlined as literals.
func SimilarPairPredicate(qualifier string, within int, aHashThreshold uint64) string {
	predicate := fmt.Sprintf("%s <= %d", SimilarPairDistance(qualifier), within)
	if aHashThreshold > 0 {
		predicate += fmt.Sprintf(" AND (%s.a_distance IS NULL OR %s.a_distance <= %d)", qualifier, qualifier, aHashThreshold)
	}
	return predicate
}
