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

	key := MRQLCacheKey("type=resource", 0, 10, 5)

	result, ok := cache.Get(key)
	assert.False(t, ok)
	assert.Nil(t, result)

	expected := &MRQLResult{Mode: "flat", EntityType: "resource"}
	cache.Put(key, expected)

	result, ok = cache.Get(key)
	assert.True(t, ok)
	assert.Equal(t, expected, result)
}

func TestMRQLCacheFromContextNil(t *testing.T) {
	cache := MRQLCacheFromContext(context.Background())
	assert.Nil(t, cache)
}
