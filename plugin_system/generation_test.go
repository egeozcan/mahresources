package plugin_system

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPluginGenerationChangesAcrossDisableAndReenable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "generation", `
plugin = {
  name = "generation",
  version = "1.0.0",
  description = "generation test",
  api_version = 1,
  capabilities = {},
}
function init() end
`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	t.Cleanup(pm.Close)

	require.NoError(t, pm.EnablePlugin("generation"))
	firstState := stateForPlugin(t, pm, "generation")
	first, ok := pm.GenerationForState(firstState)
	require.True(t, ok)
	require.NotZero(t, first)
	require.True(t, pm.GenerationActive("generation", first))
	require.Equal(t, first, pm.Plugins()[0].Generation)

	require.NoError(t, pm.DisablePlugin("generation"))
	_, ok = pm.GenerationForState(firstState)
	require.False(t, ok, "revoked VMs must no longer resolve a generation")
	require.False(t, pm.GenerationActive("generation", first))

	require.NoError(t, pm.EnablePlugin("generation"))
	secondState := stateForPlugin(t, pm, "generation")
	second, ok := pm.GenerationForState(secondState)
	require.True(t, ok)
	require.Greater(t, second, first)
	require.False(t, pm.GenerationActive("generation", first))
	require.True(t, pm.GenerationActive("generation", second))
	require.Equal(t, second, pm.Plugins()[0].Generation)
}

func TestPluginGenerationLookupRejectsMismatchedPlugin(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "one", `plugin={name="one",version="1",api_version=1,capabilities={}}`)
	writePlugin(t, dir, "two", `plugin={name="two",version="1",api_version=1,capabilities={}}`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	t.Cleanup(pm.Close)
	require.NoError(t, pm.EnablePlugin("one"))
	require.NoError(t, pm.EnablePlugin("two"))

	generation, ok := pm.GenerationForState(stateForPlugin(t, pm, "one"))
	require.True(t, ok)
	require.False(t, pm.GenerationActive("two", generation))
	require.False(t, pm.GenerationActive("one", generation+1))
}
