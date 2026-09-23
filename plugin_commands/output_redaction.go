package plugin_commands

import "bytes"

// commandOutputSecrets returns the values the host knows are secret:
// parameters explicitly marked sensitive by the declaration and supplied
// input-file contents. Patterns pass through the same terminal-control filter
// as captured output so filtering cannot expose their remaining bytes. The
// copies survive the runner's input-memory cleanup until the tail is sanitized.
func commandOutputSecrets(run QueuedRun) [][]byte {
	secrets := make([][]byte, 0, len(run.Request.Declaration.SensitiveParams)+len(run.Inputs))
	add := func(value []byte) {
		if len(value) == 0 {
			return
		}
		filtered := newOutputTailWithCapacity(len(value))
		_, _ = filtered.Write(value)
		normalized := filtered.bytes()
		clear(filtered.data)
		if len(normalized) == 0 {
			clear(normalized)
			return
		}
		for _, previous := range secrets {
			if bytes.Equal(previous, normalized) {
				clear(normalized)
				return
			}
		}
		secrets = append(secrets, normalized)
	}
	for _, name := range run.Request.Declaration.SensitiveParams {
		if value, ok := run.Request.Params[name]; ok {
			add([]byte(value))
		}
	}
	for _, input := range run.Inputs {
		add(input.Content)
	}
	return secrets
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
// capture, then applies the public tail limit. The runner retains at most one
// tail plus the longest known value, so a secret crossing the final truncation
// boundary is still present in full when this function scans it.
func redactCommandOutputTail(output string, secrets [][]byte) string {
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

func clearCommandOutputSecrets(secrets [][]byte) {
	for _, secret := range secrets {
		clear(secret)
	}
}
