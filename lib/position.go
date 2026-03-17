package lib

const (
	minChar = 'a'
	maxChar = 'z'
)

// PositionBetween returns a string that sorts between before and after.
// Uses lexicographic ordering with lowercase letters a-z.
func PositionBetween(before, after string) string {
	if before == "" && after == "" {
		return "n" // middle of alphabet
	}
	if before == "" {
		before = string(minChar)
	}
	if after == "" {
		after = string(maxChar + 1) // Use a character just past 'z' conceptually
	}
	return generateBetween(before, after)
}

func generateBetween(before, after string) string {
	result := make([]byte, 0, max(len(before), len(after))+1)

	i := 0
	for {
		// Get character at position i, or boundaries if past string length
		var prevChar, nextChar byte
		if i < len(before) {
			prevChar = before[i]
		} else {
			prevChar = minChar
		}
		if i < len(after) {
			nextChar = after[i]
		} else {
			nextChar = maxChar + 1
		}

		// When past the end of 'before', we're already guaranteed > before.
		// We just need something < after[i:]. If after[i] > minChar, we can
		// pick a character in [minChar, after[i]). If after[i] == minChar,
		// there's no room — append minChar and continue deeper.
		if i >= len(before) && i < len(after) {
			if after[i] > minChar {
				midChar := midpoint(minChar, after[i])
				if midChar >= after[i] {
					midChar = after[i] - 1
				}
				if midChar >= minChar {
					result = append(result, midChar)
					return string(result)
				}
			}
			// after[i] == minChar: no room, append 'a' and go deeper
			result = append(result, minChar)
			i++
			continue
		}

		// When past BOTH strings, we've built a prefix that equals 'after'.
		// Any extension would be > after. Return what we have (equals after,
		// which is the closest we can get for adjacent inputs).
		if i >= len(before) && i >= len(after) {
			if len(result) == 0 {
				return string(minChar)
			}
			return string(result)
		}

		if prevChar == nextChar {
			// Characters are equal, add to result and continue
			result = append(result, prevChar)
			i++
			continue
		}

		// Try to find a character between prevChar and nextChar
		midChar := midpoint(prevChar, nextChar)
		if midChar > prevChar && midChar < nextChar {
			result = append(result, midChar)
			return string(result)
		}

		// No room between characters
		// Add prevChar and look for space in the next position
		result = append(result, prevChar)
		i++

		// Now we need to find something > before[i:] and < after (conceptually 'z...')
		// The simplest approach: append a character in the middle of the remaining range
		for {
			if i < len(before) {
				prevChar = before[i]
			} else {
				prevChar = minChar - 1 // Below 'a' conceptually
			}

			// We want something > prevChar
			if prevChar < maxChar {
				midChar = midpoint(prevChar+1, maxChar+1)
				result = append(result, midChar)
				return string(result)
			}

			// prevChar is 'z', we need to extend further
			result = append(result, prevChar)
			i++
		}
	}
}

func midpoint(a, b byte) byte {
	return (a + b) / 2
}

// FirstPosition returns the initial position for the first block
func FirstPosition() string {
	return "n"
}

// GenerateEvenPositions returns n evenly distributed position strings.
// For n items, it divides the alphabet space evenly to maximize room for insertions.
// For example, with 5 items: ["d", "h", "l", "p", "t"]
// For n > 26, multi-character strings are generated to ensure uniqueness.
func GenerateEvenPositions(n int) []string {
	if n <= 0 {
		return []string{}
	}
	if n == 1 {
		return []string{"n"} // middle of alphabet
	}

	const alphabetSize = int(maxChar-minChar) + 1 // 26

	positions := make([]string, n)

	if n < alphabetSize-1 {
		// Single character positions with even spacing.
		// n <= 24 ensures step >= 1.0, so the first position is at least 'b'
		// and the last is at most 'y', leaving room for insertion at both ends.
		step := float64(maxChar-minChar) / float64(n+1)
		for i := 0; i < n; i++ {
			charCode := minChar + byte(step*float64(i+1))
			positions[i] = string(charCode)
		}
	} else {
		// Multi-character positions: treat indices as base-26 numbers
		// to guarantee strictly ascending, unique strings
		for i := 0; i < n; i++ {
			positions[i] = indexToPosition(i, n)
		}
	}

	return positions
}

// indexToPosition converts an index (0..n-1) to an evenly spaced position string.
// It maps the index into a fixed-width base-26 representation using letters a-z.
func indexToPosition(index, total int) string {
	const alphabetSize = int(maxChar-minChar) + 1

	// Determine how many characters we need.
	// Require capacity >= total+2 so that positions never land on the
	// absolute min ("aa…a") or max ("zz…z"), leaving room for insertions
	// before the first and after the last generated position.
	digits := 1
	capacity := alphabetSize
	for capacity < total+2 {
		digits++
		capacity *= alphabetSize
	}

	// Map index into [1, capacity-2] to avoid boundaries
	slot := 1 + int(float64(index+1)*float64(capacity-2)/float64(total+1))

	// Convert slot to base-26 string of fixed width
	result := make([]byte, digits)
	for d := digits - 1; d >= 0; d-- {
		result[d] = minChar + byte(slot%alphabetSize)
		slot /= alphabetSize
	}
	return string(result)
}

// NeedsRebalancing checks if any position string exceeds the threshold length.
// Position strings grow when many insertions happen at the same point.
// A threshold of 4-5 characters is reasonable (allows ~26^4 = 456,976 distinct positions).
func NeedsRebalancing(positions []string, threshold int) bool {
	for _, pos := range positions {
		if len(pos) > threshold {
			return true
		}
	}
	return false
}
