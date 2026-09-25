package plugin_commands

import (
	"bytes"
	"crypto/sha256"
	"strings"
)

const (
	maxCommandOutputInputLinePatterns  = 256
	maxCommandOutputInputTokenPatterns = 256
	maxCommandOutputInputFields        = maxCommandOutputInputTokenPatterns + 2
	minCommandOutputTokenBytes         = 8
)

// commandOutputSecrets returns the values the host knows are secret: exact
// declared-sensitive parameter values, plus bounded patterns from supplied
// inputs. Input patterns include the whole file, its first 256 nonempty lines,
// and up to 256 recognized credential values per file. If those bounds leave
// any input content without line coverage or a credential parser limit is
// reached, the caller discards the captured tail. Patterns pass through the
// same terminal-control filter as captured output. The copies survive the
// runner's input-memory cleanup until the tail is sanitized.
func commandOutputSecrets(run QueuedRun) ([][]byte, bool) {
	secrets := make([][]byte, 0, len(run.Request.Declaration.SensitiveParams)+len(run.Inputs))
	seen := make(map[[sha256.Size]byte][]int)
	incomplete := false
	addNormalized := func(value []byte) {
		if len(value) == 0 {
			return
		}
		digest := sha256.Sum256(value)
		for _, index := range seen[digest] {
			if bytes.Equal(secrets[index], value) {
				return
			}
		}
		seen[digest] = append(seen[digest], len(secrets))
		secrets = append(secrets, append([]byte(nil), value...))
	}
	add := func(value []byte) {
		if len(value) == 0 {
			return
		}
		filtered := newOutputTailWithCapacity(len(value))
		_, _ = filtered.Write(value)
		normalized := filtered.bytes()
		clear(filtered.data)
		addNormalized(normalized)
		clear(normalized)
	}
	for _, name := range run.Request.Declaration.SensitiveParams {
		if value, ok := run.Request.Params[name]; ok {
			add([]byte(value))
		}
	}
	for _, input := range run.Inputs {
		normalized := normalizeCommandOutputBytes(input.Content)
		addNormalized(normalized)
		linePatterns, tokenPatterns := 0, 0
		start := 0
		for start < len(normalized) && (linePatterns < maxCommandOutputInputLinePatterns || tokenPatterns < maxCommandOutputInputTokenPatterns) {
			end := start + bytes.IndexByte(normalized[start:], '\n')
			if end < start {
				end = len(normalized)
			}
			line := bytes.TrimSpace(normalized[start:end])
			if len(line) > 0 {
				if linePatterns < maxCommandOutputInputLinePatterns {
					add(line)
					linePatterns++
				} else {
					incomplete = true
				}
				if tokenPatterns < maxCommandOutputInputTokenPatterns && hasCommandOutputCredentialSyntax(line) {
					added, fieldLimitReached := addCommandOutputInputTokens(line, maxCommandOutputInputTokenPatterns-tokenPatterns, add)
					tokenPatterns += added
					if fieldLimitReached {
						incomplete = true
					}
					// A full budget may have stopped in the middle of this line.
					if tokenPatterns >= maxCommandOutputInputTokenPatterns {
						incomplete = true
					}
				}
			}
			if end == len(normalized) {
				start = len(normalized)
				break
			}
			start = end + 1
		}
		if start < len(normalized) {
			// Both pattern budgets were exhausted before all input was scanned.
			incomplete = true
		}
		clear(normalized)
	}
	return secrets, incomplete
}

func hasCommandOutputCredentialSyntax(line []byte) bool {
	if bytes.IndexByte(line, '=') >= 0 || bytes.IndexByte(line, ':') >= 0 || bytes.IndexByte(line, '\t') >= 0 {
		return true
	}
	firstEnd := bytes.IndexAny(line, " \t")
	if firstEnd < 0 {
		firstEnd = len(line)
	}
	return isAuthScheme(bytes.TrimSpace(line[:firstEnd]))
}

func normalizeCommandOutputBytes(value []byte) []byte {
	filtered := newOutputTailWithCapacity(len(value))
	_, _ = filtered.Write(value)
	normalized := filtered.bytes()
	clear(filtered.data)
	return normalized
}

// addCommandOutputInputTokens recognizes common credential fields in input
// lines: name=value pairs, credential headers, Bearer/Basic values, and
// Netscape cookie rows. It returns the number of candidates consumed and
// whether the field parser reached its ceiling.
func addCommandOutputInputTokens(line []byte, budget int, add func([]byte)) (int, bool) {
	if budget <= 0 {
		return 0, false
	}
	initialBudget := budget
	fields := commandOutputInputFields(line, maxCommandOutputInputFields, true)
	fieldLimitReached := len(fields) == maxCommandOutputInputFields
	if cookieValue, ok := netscapeCookieValue(line); ok {
		add(cookieValue)
		budget--
		if budget == 0 {
			return initialBudget, fieldLimitReached
		}
	}

	trimmedLine := bytes.TrimSpace(line)
	cookieHeader := hasPrefixFold(trimmedLine, "cookie:")
	setCookieHeader := hasPrefixFold(trimmedLine, "set-cookie:")
	used := 0
	for i := 0; i < len(fields) && used < budget; i++ {
		field := trimCommandOutputToken(fields[i])
		if len(field) == 0 {
			continue
		}

		if isAuthScheme(field) && i+1 < len(fields) {
			value := trimCommandOutputToken(fields[i+1])
			if len(value) > 0 {
				add(value)
				used++
				i++
			}
			continue
		}

		if separator := bytes.IndexByte(field, '='); separator > 0 {
			key := trimCommandOutputToken(field[:separator])
			value := trimCommandOutputToken(field[separator+1:])
			allowShort := cookieHeader || isCredentialKey(key)
			if setCookieHeader && used == 0 {
				allowShort = true
			}
			if len(value) > 0 && (allowShort || len(value) >= minCommandOutputTokenBytes) {
				add(value)
				used++
			}
			continue
		}

		if separator := bytes.IndexByte(field, ':'); separator > 0 {
			key := trimCommandOutputToken(field[:separator])
			value := trimCommandOutputToken(field[separator+1:])
			if isCredentialKey(key) {
				if isAuthScheme(value) && i+1 < len(fields) {
					next := trimCommandOutputToken(fields[i+1])
					if len(next) > 0 {
						add(next)
						used++
						i++
					}
				} else if len(value) > 0 {
					add(value)
					used++
				} else if i+1 < len(fields) && !isAuthScheme(trimCommandOutputToken(fields[i+1])) && bytes.IndexByte(fields[i+1], '=') < 0 {
					next := trimCommandOutputToken(fields[i+1])
					if len(next) > 0 {
						add(next)
						used++
						i++
					}
				}
			}
		}
	}
	return initialBudget - budget + used, fieldLimitReached
}

func commandOutputInputFields(line []byte, limit int, splitCookieDelimiters bool) [][]byte {
	fields := make([][]byte, 0, min(limit, 16))
	for start := 0; start < len(line) && len(fields) < limit; {
		for start < len(line) && isCommandOutputFieldSeparator(line[start], splitCookieDelimiters) {
			start++
		}
		if start == len(line) {
			break
		}
		end := start
		for end < len(line) && !isCommandOutputFieldSeparator(line[end], splitCookieDelimiters) {
			end++
		}
		fields = append(fields, line[start:end])
		start = end
	}
	return fields
}

func isCommandOutputFieldSeparator(value byte, splitCookieDelimiters bool) bool {
	if value == ' ' || value == '\t' || value == '\r' || value == '\n' {
		return true
	}
	return splitCookieDelimiters && (value == ';' || value == '&' || value == ',')
}

func trimCommandOutputToken(value []byte) []byte {
	return bytes.Trim(value, "\"'`{}[]()<>,;")
}

func hasPrefixFold(value []byte, prefix string) bool {
	return len(value) >= len(prefix) && bytes.EqualFold(value[:len(prefix)], []byte(prefix))
}

func isAuthScheme(value []byte) bool {
	return bytes.EqualFold(value, []byte("bearer")) || bytes.EqualFold(value, []byte("basic"))
}

func isCredentialKey(key []byte) bool {
	name := strings.ToLower(string(trimCommandOutputToken(key)))
	for _, marker := range []string{"token", "cookie", "session", "auth", "secret", "password", "passwd", "credential", "csrf", "api_key", "apikey"} {
		if strings.Contains(name, marker) {
			return true
		}
	}
	return name == "key" || name == "sid" || strings.HasSuffix(name, "_sid") || strings.HasSuffix(name, "-sid")
}

func netscapeCookieValue(line []byte) ([]byte, bool) {
	fields := commandOutputInputFields(line, 8, false)
	if len(fields) != 7 || (line[0] == '#' && !bytes.HasPrefix(line, []byte("#HttpOnly_"))) {
		return nil, false
	}
	if !isCookieBoolean(fields[1]) || !bytes.HasPrefix(fields[2], []byte("/")) || !isCookieBoolean(fields[3]) || len(fields[4]) == 0 || len(fields[5]) == 0 || len(fields[6]) == 0 {
		return nil, false
	}
	for _, digit := range fields[4] {
		if digit < '0' || digit > '9' {
			return nil, false
		}
	}
	return fields[6], true
}

func isCookieBoolean(value []byte) bool {
	return bytes.EqualFold(value, []byte("TRUE")) || bytes.EqualFold(value, []byte("FALSE"))
}

func maxCommandOutputSecretLength(secrets [][]byte) int {
	maximum := 0
	for _, secret := range secrets {
		if len(secret) > maximum {
			maximum = len(secret)
		}
	}
	return maximum
}

// redactCommandOutputTail scrubs exact known values from the terminal-filtered
// capture, then applies the public tail limit. When input scanning was
// incomplete, it discards the whole tail because an uncollected value could be
// echoed partially. The runner retains at most one tail plus the longest known
// value, so a secret crossing the final truncation boundary is still present
// in full when this function scans it.
func redactCommandOutputTail(output string, secrets [][]byte, inputCoverageIncomplete bool) string {
	if inputCoverageIncomplete {
		return redactedValue
	}
	remaining := []byte(output)
	defer clear(remaining)
	if len(secrets) > 0 {
		redacted := make([]byte, 0, len(remaining))
		defer func() { clear(redacted) }()
		cursor := 0
		marker := []byte(redactedValue)
		for cursor < len(remaining) {
			matchStart, matchLength := -1, 0
			for _, secret := range secrets {
				if len(secret) == 0 {
					continue
				}
				relative := bytes.Index(remaining[cursor:], secret)
				if relative < 0 {
					continue
				}
				start := cursor + relative
				if matchStart < 0 || start < matchStart || (start == matchStart && len(secret) > matchLength) {
					matchStart, matchLength = start, len(secret)
				}
			}
			if matchStart < 0 {
				redacted = append(redacted, remaining[cursor:]...)
				break
			}
			redacted = append(redacted, remaining[cursor:matchStart]...)
			redacted = append(redacted, marker...)
			// Keep at least the matched byte count. That way replacing a long
			// secret cannot pull older, partially captured bytes into the final
			// tail after truncation.
			if matchLength > len(marker) {
				redacted = append(redacted, bytes.Repeat([]byte{' '}, matchLength-len(marker))...)
			}
			cursor = matchStart + matchLength
		}
		remaining = redacted
	}
	if len(remaining) > outputTailBytes {
		remaining = remaining[len(remaining)-outputTailBytes:]
	}
	return string(remaining)
}

// redactProgressText applies the tail's redaction to one short progress field.
// The tail pads a replaced secret with spaces to keep its length, which a label
// has no use for.
func redactProgressText(value string, secrets [][]byte, inputCoverageIncomplete bool) string {
	if value == "" {
		return value
	}
	return strings.TrimRight(redactCommandOutputTail(value, secrets, inputCoverageIncomplete), " ")
}

func cloneCommandOutputSecrets(secrets [][]byte) [][]byte {
	out := make([][]byte, len(secrets))
	for i, secret := range secrets {
		out[i] = append([]byte(nil), secret...)
	}
	return out
}

func clearCommandOutputSecrets(secrets [][]byte) {
	for _, secret := range secrets {
		clear(secret)
	}
}
