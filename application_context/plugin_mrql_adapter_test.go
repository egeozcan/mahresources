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
