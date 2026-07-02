package hash_worker

import (
	"math/bits"
	"testing"
)

func TestSplitChunks_RoundTrip(t *testing.T) {
	cases := []uint64{
		0,
		0xFFFFFFFFFFFFFFFF,
		0x0123456789ABCDEF,
		0x7F00FF00FF00FF00,
	}
	for _, h := range cases {
		c := SplitChunks(h)
		got := uint64(c[0]) | uint64(c[1])<<16 | uint64(c[2])<<32 | uint64(c[3])<<48
		if got != h {
			t.Errorf("SplitChunks(%#x) round-trip = %#x", h, got)
		}
	}
}

func TestChunkNeighbors_Count(t *testing.T) {
	// Count must equal sum of C(16,k) for k in 0..radius.
	wantCounts := map[int]int{0: 1, 1: 17, 2: 137}
	for radius, want := range wantCounts {
		got := len(ChunkNeighbors(0xABCD, radius))
		if got != want {
			t.Errorf("radius %d: got %d neighbours, want %d", radius, got, want)
		}
	}
}

func TestChunkNeighbors_NoDuplicates(t *testing.T) {
	for radius := 0; radius <= 2; radius++ {
		neighbors := ChunkNeighbors(0x1234, radius)
		seen := make(map[uint16]bool, len(neighbors))
		for _, v := range neighbors {
			if seen[v] {
				t.Fatalf("radius %d: duplicate value %#x", radius, v)
			}
			seen[v] = true
		}
	}
}

func TestChunkNeighbors_AllWithinRadius(t *testing.T) {
	// Every enumerated value must be within `radius` bits of the chunk.
	const chunk uint16 = 0x8001
	for radius := 0; radius <= 2; radius++ {
		for _, v := range ChunkNeighbors(chunk, radius) {
			d := bits.OnesCount16(chunk ^ v)
			if d > radius {
				t.Errorf("radius %d: value %#x is distance %d from %#x", radius, v, d, chunk)
			}
		}
	}
}

func TestChunkNeighbors_ExhaustiveCompleteness(t *testing.T) {
	// Brute-force: every 16-bit value within Hamming distance r of the chunk
	// must appear in the enumeration (completeness of the pigeonhole prefilter).
	const chunk uint16 = 0x5A5A
	for radius := 0; radius <= 2; radius++ {
		enumerated := make(map[uint16]bool)
		for _, v := range ChunkNeighbors(chunk, radius) {
			enumerated[v] = true
		}
		for i := 0; i <= 0xFFFF; i++ {
			v := uint16(i)
			if bits.OnesCount16(chunk^v) <= radius && !enumerated[v] {
				t.Fatalf("radius %d: value %#x within distance but not enumerated", radius, v)
			}
		}
	}
}

// TestPigeonholeInvariant verifies the correctness foundation of the prefilter:
// any two 64-bit hashes within MaxStoredPDistance must share at least one chunk
// that differs by <= ChunkRadius.
func TestPigeonholeInvariant(t *testing.T) {
	check := func(a, b uint64) {
		if HammingDistance(a, b) > MaxStoredPDistance {
			return
		}
		ca, cb := SplitChunks(a), SplitChunks(b)
		found := false
		for i := 0; i < 4; i++ {
			if bits.OnesCount16(ca[i]^cb[i]) <= ChunkRadius {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("hashes %#x and %#x within %d but no chunk within %d",
				a, b, MaxStoredPDistance, ChunkRadius)
		}
	}
	// Deterministic spread of pairs at exactly the boundary distance.
	base := uint64(0x0123456789ABCDEF)
	for mask := 0; mask < 4096; mask++ {
		// Flip up to 11 bits at deterministic positions.
		var b uint64 = base
		flips := 0
		for pos := 0; pos < 64 && flips < 11; pos++ {
			if mask&(1<<(pos%12)) != 0 {
				b ^= 1 << uint(pos)
				flips++
			}
		}
		check(base, b)
	}
}
