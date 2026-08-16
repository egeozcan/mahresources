package plugin_system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMRQLCacheHitAndMiss(t *testing.T) {
	ctx := context.Background()
	ctx = WithMRQLCache(ctx)

	cache := MRQLCacheFromContext(ctx)
	assert.NotNil(t, cache)

	key := MRQLCacheKey("type=resource", 0, 10, 5, nil, 0)

	result, ok := cache.Get(key)
	assert.False(t, ok)
	assert.Nil(t, result)

	expected := &MRQLResult{Mode: "flat", EntityType: "resource"}
	cache.Put(key, expected)

	result, ok = cache.Get(key)
	assert.True(t, ok)
	assert.Equal(t, expected, result)
}

// The executor answers the same query differently per principal now, so two
// actors must not share a cache entry.
func TestMRQLCacheKeySeparatesActors(t *testing.T) {
	base := MRQLCacheKey("type=resource", 0, 10, 5, nil, 0)
	assert.NotEqual(t, base, MRQLCacheKey("type=resource", 0, 10, 5, nil, 7))
	assert.NotEqual(t,
		MRQLCacheKey("type=resource", 0, 10, 5, nil, 7),
		MRQLCacheKey("type=resource", 0, 10, 5, nil, 8))
}

func TestMRQLCacheFromContextNil(t *testing.T) {
	cache := MRQLCacheFromContext(context.Background())
	assert.Nil(t, cache)
}
