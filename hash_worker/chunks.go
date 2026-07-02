package hash_worker

// Image similarity v2 chunk math.
//
// A 64-bit pHash is split into four 16-bit chunks stored in indexed integer
// columns. The pigeonhole principle guarantees that if two hashes are within
// Hamming distance MaxStoredPDistance (11) overall, at least one of the four
// chunks differs by at most 2 (worst case 3+3+3+2 = 11). So enumerating every
// value within Hamming radius 2 of each of a probe's four chunks and querying
// the chunk indexes for matches yields a candidate superset of all true
// neighbours, which are then verified exactly with a full-width popcount.

// MaxStoredPDistance is the maximum pHash Hamming distance stored in
// resource_similarities. Chosen so radius-2 chunk enumeration (137 values per
// chunk) suffices for the pigeonhole prefilter. Storing further would require a
// larger enumeration radius and far more candidate rows.
const MaxStoredPDistance = 11

// ChunkRadius is the per-chunk Hamming radius enumerated for the prefilter.
// It follows from MaxStoredPDistance via the pigeonhole principle:
// floor(MaxStoredPDistance / 4) = 2.
const ChunkRadius = 2

// SplitChunks splits a 64-bit hash into four 16-bit chunks.
// chunk[0] holds bits 0-15, chunk[1] bits 16-31, chunk[2] bits 32-47,
// chunk[3] bits 48-63.
func SplitChunks(h uint64) [4]uint16 {
	return [4]uint16{
		uint16(h),
		uint16(h >> 16),
		uint16(h >> 32),
		uint16(h >> 48),
	}
}

// ChunkNeighbors returns every 16-bit value within Hamming distance `radius`
// of chunk (inclusive), including chunk itself. The result has no duplicates.
// For radius 0..2 the counts are C(16,0)=1, +C(16,1)=17, +C(16,2)=137.
func ChunkNeighbors(chunk uint16, radius int) []uint16 {
	if radius < 0 {
		return nil
	}
	if radius > 16 {
		radius = 16
	}

	// Preallocate to the exact count (sum of C(16,k) for k in 0..radius).
	total := 0
	for k := 0; k <= radius; k++ {
		total += binomial(16, k)
	}
	out := make([]uint16, 0, total)

	// Enumerate all combinations of up to `radius` bit positions to flip.
	// Strictly increasing bit indices guarantee no duplicate values.
	var recurse func(start, remaining int, val uint16)
	recurse = func(start, remaining int, val uint16) {
		out = append(out, val)
		if remaining == 0 {
			return
		}
		for i := start; i < 16; i++ {
			recurse(i+1, remaining-1, val^(1<<uint(i)))
		}
	}
	recurse(0, radius, chunk)
	return out
}

// binomial computes C(n, k) for small n, k without overflow concerns.
func binomial(n, k int) int {
	if k < 0 || k > n {
		return 0
	}
	if k > n-k {
		k = n - k
	}
	result := 1
	for i := 0; i < k; i++ {
		result = result * (n - i) / (i + 1)
	}
	return result
}
