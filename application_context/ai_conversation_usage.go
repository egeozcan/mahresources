package application_context

// Usage reporting and the charging rule for one Turn.
//
// Adapters report the provider's own numbers and leave a field nil when the
// provider did not send it. Anthropic is mapped onto the same shape as the
// others: Prompt is input_tokens + cache_creation_input_tokens +
// cache_read_input_tokens, CacheHit is cache_read_input_tokens and CacheMiss is
// input_tokens + cache_creation_input_tokens, so "Prompt - CacheHit" is the
// uncached part for every provider. Completion includes reasoning tokens
// everywhere (Reasoning is informational, never added again).

// ChargeableTokens is what a Turn costs against the run's token budget, with
// cached prompt tokens counted at a quarter.
//
//   - A missing Prompt falls back to estimate, a missing Completion to maxTokens.
//   - With a cache hit count known, the uncached part is Prompt - CacheHit
//     whether or not CacheMiss was reported. When only CacheMiss is known, the
//     hit is derived from it.
//   - With no cache information at all, the whole prompt is uncached.
//
// Prompt=10000, CacheHit=8000, Completion=1000 charges 1000 + 2000 + 8000/4 = 5000.
func ChargeableTokens(u ConversationUsage, estimate, maxTokens int) int {
	prompt, completion := estimate, maxTokens
	if u.Prompt != nil {
		prompt = *u.Prompt
	}
	if u.Completion != nil {
		completion = *u.Completion
	}
	hit := 0
	switch {
	case u.CacheHit != nil:
		hit = *u.CacheHit
	case u.CacheMiss != nil && u.Prompt != nil:
		hit = prompt - *u.CacheMiss
	}
	hit = max(0, min(hit, prompt))
	return (prompt - hit) + completion + hit/4
}
