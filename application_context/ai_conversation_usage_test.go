package application_context

import "testing"

func TestChargeableTokens(t *testing.T) {
	for _, tc := range []struct {
		name      string
		u         ConversationUsage
		est, maxT int
		want      int
	}{
		{"plan example, no miss counter", ConversationUsage{Prompt: intp(10000), CacheHit: intp(8000), Completion: intp(1000)}, 999, 999, 5000},
		{"miss counter is ignored when prompt and hit are known", ConversationUsage{Prompt: intp(10000), CacheHit: intp(8000), CacheMiss: intp(1), Completion: intp(1000)}, 0, 0, 5000},
		{"no cache info: whole prompt uncached", ConversationUsage{Prompt: intp(10000), Completion: intp(1000)}, 0, 0, 11000},
		{"missing prompt uses the estimate, missing completion the max", ConversationUsage{}, 4000, 2500, 6500},
		{"missing usage with cache fields absent", ConversationUsage{Completion: intp(10)}, 100, 0, 110},
		{"only a miss counter: hit is derived", ConversationUsage{Prompt: intp(1000), CacheMiss: intp(200), Completion: intp(0)}, 0, 0, 200 + 800/4},
		{"hit larger than prompt is clamped", ConversationUsage{Prompt: intp(100), CacheHit: intp(500), Completion: intp(0)}, 0, 0, 25},
		{"anthropic shape", ConversationUsage{Prompt: intp(2300), CacheHit: intp(2000), CacheMiss: intp(300), Completion: intp(50)}, 0, 0, 300 + 50 + 500},
	} {
		if got := ChargeableTokens(tc.u, tc.est, tc.maxT); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
