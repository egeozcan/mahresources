package plugin_system

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

type mrqlCacheKey struct{}

// MRQLCache is a per-request cache for MRQL query results.
type MRQLCache struct {
	mu    sync.Mutex
	store map[string]*MRQLResult
}

// WithMRQLCache returns a new context with an empty MRQL cache attached.
func WithMRQLCache(ctx context.Context) context.Context {
	return context.WithValue(ctx, mrqlCacheKey{}, &MRQLCache{
		store: make(map[string]*MRQLResult),
	})
}

// MRQLCacheFromContext retrieves the MRQL cache from the context, or nil.
func MRQLCacheFromContext(ctx context.Context) *MRQLCache {
	v := ctx.Value(mrqlCacheKey{})
	if v == nil {
		return nil
	}
	return v.(*MRQLCache)
}

// MRQLCacheKey builds a deterministic cache key from query parameters. params
// bindings are folded in (sorted) so two calls that differ only by parameter
// value do not collide.
func MRQLCacheKey(query string, scopeID uint, limit, buckets int, params map[string]string) string {
	base := fmt.Sprintf("%s|%d|%d|%d", query, scopeID, limit, buckets)
	if len(params) == 0 {
		return base
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(base)
	for _, k := range keys {
		// %q-quote so separator characters in names/values cannot collide with
		// the key structure (e.g. {"a": "1|b=2"} vs {"a": "1", "b": "2"}).
		fmt.Fprintf(&b, "|%q=%q", k, params[k])
	}
	return b.String()
}

// Get returns a cached result and true, or nil and false.
func (c *MRQLCache) Get(key string) (*MRQLResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, ok := c.store[key]
	return r, ok
}

// Put stores a result in the cache.
func (c *MRQLCache) Put(key string, result *MRQLResult) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.store[key] = result
}
