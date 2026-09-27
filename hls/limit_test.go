package hls

import (
	"errors"
	"strings"
	"sync/atomic"
	"testing"
)

// A stream refused because it is over a limit the deployment configures is told
// apart from one this server refuses outright: raising the limit makes the same
// download possible, so the first is worth retrying and the second is not.
func TestAConfiguredLimitIsToldApartFromAnOutrightRefusal(t *testing.T) {
	var spent atomic.Int64
	reader := &budgetReader{r: strings.NewReader(strings.Repeat("x", 64)), total: &spent, limit: 16}
	_, err := reader.Read(make([]byte, 64))
	var refused *ErrNotSupported
	if !errors.As(err, &refused) || !refused.Limit || !errors.Is(err, errBudgetExceeded) {
		t.Fatalf("the byte budget answered %v, want a limit refusal that is still errBudgetExceeded", err)
	}
	if !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("the refusal does not name the limit: %v", err)
	}

	_, _, err = parse(strings.Repeat("#EXTINF:1,\nseg.ts\n", 5)+"#EXT-X-ENDLIST\n", "https://example.com/v.m3u8",
		Options{MaxSegments: 2}, 0, new(string))
	if !errors.As(err, &refused) || !refused.Limit {
		t.Fatalf("a playlist over the segment limit answered %v, want a limit refusal", err)
	}

	_, _, err = parse("#EXTM3U\n#EXTINF:1,\nseg.ts\n", "https://example.com/live.m3u8", Options{MaxSegments: 100}, 0, new(string))
	if !errors.As(err, &refused) || refused.Limit {
		t.Fatalf("a live stream answered %v, want an outright refusal", err)
	}
}
