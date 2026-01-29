// lib/position.go
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
