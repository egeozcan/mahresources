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

	if n < alphabetSize {
		// Single character positions with even spacing
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

	// Determine how many characters we need
	digits := 1
	capacity := alphabetSize
	for capacity < total {
		digits++
		capacity *= alphabetSize
	}

	// Map index to evenly spaced slot in the capacity
	slot := int(float64(index+1) * float64(capacity) / float64(total+1))

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
