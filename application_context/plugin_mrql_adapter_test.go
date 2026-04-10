//go:build json1 && fts5

package application_context

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/plugin_system"
)

func TestPluginMRQLAdapterFlat(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	result, err := adapter.ExecuteMRQL(context.Background(), "type=resource", plugin_system.MRQLExecOptions{
		Limit: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "flat", result.Mode)
	assert.Equal(t, "resource", result.EntityType)
	// Items may or may not be present depending on shared DB state
	// (other tests may populate resources in the shared in-memory DB).
	// We only verify the structural correctness of the result.
}

func TestPluginMRQLAdapterFlatNotes(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	result, err := adapter.ExecuteMRQL(context.Background(), "type=note", plugin_system.MRQLExecOptions{
		Limit: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "flat", result.Mode)
	assert.Equal(t, "note", result.EntityType)
}

func TestPluginMRQLAdapterFlatGroups(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	result, err := adapter.ExecuteMRQL(context.Background(), "type=group", plugin_system.MRQLExecOptions{
		Limit: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "flat", result.Mode)
	assert.Equal(t, "group", result.EntityType)
}

func TestPluginMRQLAdapterAggregated(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	result, err := adapter.ExecuteMRQL(context.Background(), "type=resource GROUP BY contentType COUNT()", plugin_system.MRQLExecOptions{
		Limit:   10,
		Buckets: 5,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	// Result mode depends on data — "aggregated" or "bucketed"
	assert.Contains(t, []string{"aggregated", "bucketed"}, result.Mode)
}

func TestPluginMRQLAdapterScoped(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	// ScopeID=999999 should match nothing (empty result, no error)
	result, err := adapter.ExecuteMRQL(context.Background(), "type=resource", plugin_system.MRQLExecOptions{
		Limit:   10,
		ScopeID: 999999,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Empty(t, result.Items)
}

func TestPluginMRQLAdapterScopeResolution(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	// Test scope resolution: looking up parent of a nonexistent entity
	// should return a sentinel (max uint >> 1) that matches nothing in DB.
	// NOT 0, because 0 means "no scope filter" = global fan-out.
	scopeID := adapter.resolveScope("parent", 999999, "group")
	assert.Equal(t, ^uint(0)>>1, scopeID)

	// scope="global" always returns 0
	scopeID = adapter.resolveScope("global", 1, "group")
	assert.Equal(t, uint(0), scopeID)

	// scope="entity" returns the entity ID itself
	scopeID = adapter.resolveScope("entity", 42, "group")
	assert.Equal(t, uint(42), scopeID)

	// scope="" (empty) defaults to entity
	scopeID = adapter.resolveScope("", 42, "group")
	assert.Equal(t, uint(42), scopeID)

	// scope="entity" on a resource uses sentinel (not the raw resource ID,
	// which would collide with an unrelated group sharing the same numeric ID)
	scopeID = adapter.resolveScope("entity", 42, "resource")
	assert.Equal(t, ^uint(0)>>1, scopeID, "ownerless resource entity scope should be sentinel")

	// scope="entity" on a note — same: sentinel for ownerless note
	scopeID = adapter.resolveScope("entity", 42, "note")
	assert.Equal(t, ^uint(0)>>1, scopeID, "ownerless note entity scope should be sentinel")

	// scope="root" on an ownerless resource — sentinel, not the raw ID
	scopeID = adapter.resolveScope("root", 42, "resource")
	assert.Equal(t, ^uint(0)>>1, scopeID, "ownerless resource root scope should be sentinel")

	// scope="root" on an ownerless note — sentinel
	scopeID = adapter.resolveScope("root", 42, "note")
	assert.Equal(t, ^uint(0)>>1, scopeID, "ownerless note root scope should be sentinel")
}

func TestPluginMRQLAdapterRequiresEntityType(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	// Query without type= should fail
	_, err := adapter.ExecuteMRQL(context.Background(), "name~test", plugin_system.MRQLExecOptions{
		Limit: 10,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "entity type")
}

func TestPluginMRQLAdapterInvalidQuery(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginMRQLAdapter{ctx: ctx}

	_, err := adapter.ExecuteMRQL(context.Background(), "!!!invalid!!!", plugin_system.MRQLExecOptions{})
	require.Error(t, err)
}
