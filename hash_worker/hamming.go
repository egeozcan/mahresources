package hash_worker

import "math/bits"

// HammingDistance returns the number of bit positions where two uint64 values differ.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}
