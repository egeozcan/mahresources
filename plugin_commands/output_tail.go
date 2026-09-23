package plugin_commands

import "sync"

const outputTailBytes = 64 << 10

// outputTail is a concurrency-safe bounded writer shared by stdout and stderr.
// It strips terminal control sequences before bytes enter the retained tail.
type outputTail struct {
	mu        sync.Mutex
	data      []byte
	start     int
	size      int
	escape    bool
	csi       bool
	osc       bool
	oscEscape bool
}

func newOutputTail() *outputTail {
	return newOutputTailWithCapacity(outputTailBytes)
}

// newOutputTailWithCapacity retains a bounded look-behind window before the
// runner redacts known secret values and trims the persisted tail to
// outputTailBytes. The ordinary constructor keeps the public output-tail
// bound used by callers that do not need that window.
func newOutputTailWithCapacity(capacity int) *outputTail {
	if capacity < outputTailBytes {
		capacity = outputTailBytes
	}
	return &outputTail{data: make([]byte, capacity)}
}

func (t *outputTail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, b := range p {
		if t.osc {
			if b == 0x07 || (t.oscEscape && b == '\\') {
				t.osc = false
				t.oscEscape = false
				continue
			}
			t.oscEscape = b == 0x1b
			continue
		}
		if t.csi {
			if b >= 0x40 && b <= 0x7e {
				t.csi = false
			}
			continue
		}
		if t.escape {
			t.escape = false
			switch b {
			case '[':
				t.csi = true
			case ']':
				t.osc = true
			}
			continue
		}
		if b == 0x1b {
			t.escape = true
			continue
		}
		if (b < 0x20 && b != '\n' && b != '\t') || b == 0x7f {
			continue
		}
		t.appendByte(b)
	}
	return len(p), nil
}

func (t *outputTail) appendByte(b byte) {
	if t.size < len(t.data) {
		t.data[(t.start+t.size)%len(t.data)] = b
		t.size++
		return
	}
	t.data[t.start] = b
	t.start = (t.start + 1) % len(t.data)
}

func (t *outputTail) String() string {
	return string(t.bytes())
}

func (t *outputTail) bytes() []byte {
	t.mu.Lock()
	defer t.mu.Unlock()
	result := make([]byte, t.size)
	if t.size == 0 {
		return result
	}
	first := copy(result, t.data[t.start:min(len(t.data), t.start+t.size)])
	copy(result[first:], t.data[:t.size-first])
	return result
}
