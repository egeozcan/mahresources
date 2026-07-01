package fts

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseSearchQuery parses user input into a structured query
// Supported syntax:
//   - "hello" -> prefix matching (if length >= 3)
//   - "hi" -> exact matching (if length < 3)
//   - "typ*" -> explicit prefix matching
//   - "=test" -> explicit exact matching (equals prefix)
//   - "\"test\"" -> explicit exact matching (quotes)
//   - "~test" -> fuzzy matching with edit distance 1
//   - "~2test" -> fuzzy matching with edit distance 2
func ParseSearchQuery(input string) ParsedQuery {
	input = strings.TrimSpace(input)

	if input == "" {
		return ParsedQuery{Term: "", Mode: ModeExact}
	}

	// Check for explicit exact match with quotes
	if len(input) >= 2 && strings.HasPrefix(input, "\"") && strings.HasSuffix(input, "\"") {
		term := input[1 : len(input)-1]
		return ParsedQuery{
			Term:    sanitizeSearchTerm(term),
			RawTerm: sanitizeSearchTermKeepHyphen(term),
			Mode:    ModeExact,
		}
	}

	// Check for explicit exact match with =
	if strings.HasPrefix(input, "=") {
		term := strings.TrimPrefix(input, "=")
		return ParsedQuery{
			Term:    sanitizeSearchTerm(term),
			RawTerm: sanitizeSearchTermKeepHyphen(term),
			Mode:    ModeExact,
		}
	}

	// Check for prefix search: ends with *
	if strings.HasSuffix(input, "*") {
		term := strings.TrimSuffix(input, "*")
		return ParsedQuery{
			Term:    sanitizeSearchTerm(term),
			RawTerm: sanitizeSearchTermKeepHyphen(term),
			Mode:    ModePrefix,
		}
	}

	// Check for fuzzy search: starts with ~
	if strings.HasPrefix(input, "~") {
		rest := strings.TrimPrefix(input, "~")
		dist := 1 // default edit distance

		// Check for explicit distance like ~2word
		if len(rest) > 0 && unicode.IsDigit(rune(rest[0])) {
			dist = int(rest[0] - '0')
			if dist < 1 {
				dist = 1
			}
			if dist > 3 {
				dist = 3 // cap at 3 for performance
			}
			rest = rest[1:]
		}

		return ParsedQuery{
			Term:      sanitizeSearchTerm(rest),
			RawTerm:   sanitizeSearchTermKeepHyphen(rest),
			Mode:      ModeFuzzy,
			FuzzyDist: dist,
		}
	}

	term := sanitizeSearchTerm(input)
	rawTerm := sanitizeSearchTermKeepHyphen(input)

	// Short terms default to exact match to avoid performance issues and noise
	if utf8.RuneCountInString(term) < 3 {
		return ParsedQuery{
			Term:    term,
			RawTerm: rawTerm,
			Mode:    ModeExact,
		}
	}

	// Default: prefix match (partial match)
	return ParsedQuery{
		Term:    term,
		RawTerm: rawTerm,
		Mode:    ModePrefix,
	}
}

// sanitizeSearchTerm removes potentially dangerous characters for FTS queries
// Keeps alphanumeric, spaces, and common punctuation
func sanitizeSearchTerm(term string) string {
	var result strings.Builder
	result.Grow(len(term))

	for _, r := range term {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '_' || r == '.' {
			result.WriteRune(r)
		} else if r == '-' {
			// Replace hyphens with spaces — FTS5 interprets bare hyphens as
			// the NOT operator (column-scope syntax), causing query errors.
			result.WriteRune(' ')
		}
	}

	return strings.TrimSpace(result.String())
}

// sanitizeSearchTermKeepHyphen is like sanitizeSearchTerm but PRESERVES hyphens.
// The Postgres provider needs the original hyphenation so a query term tokenizes
// identically to the stored tsvector (generated from the raw column value);
// collapsing hyphens to spaces there splits a compound like "2024-q3" into terms
// that Postgres' parser does not produce for the stored value, making the row
// unfindable by its own name. SQLite FTS5 continues to use the hyphen-stripped
// Term because a bare hyphen is FTS5 column-scope/NOT syntax. The kept character
// set is otherwise identical to sanitizeSearchTerm, so the value remains safe to
// pass as a bind parameter to to_tsvector().
func sanitizeSearchTermKeepHyphen(term string) string {
	var result strings.Builder
	result.Grow(len(term))

	for _, r := range term {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == ' ' || r == '_' || r == '.' || r == '-' {
			result.WriteRune(r)
		}
	}

	return strings.TrimSpace(result.String())
}

// EscapeForFTS escapes special characters for FTS queries
// Different databases have different special characters, so this is a basic escape
func EscapeForFTS(term string) string {
	// Replace single quotes with escaped version
	term = strings.ReplaceAll(term, "'", "''")
	return term
}
