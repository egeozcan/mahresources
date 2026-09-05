package plugin_system

import (
	"testing"

	"github.com/stretchr/testify/require"
	lua "github.com/yuin/gopher-lua"
)

func TestPluginBlockScriptsRegistration(t *testing.T) {
	L := lua.NewState()
	defer L.Close()
	require.NoError(t, L.DoString(`config = {type="test",label="Test",scripts={"core.js","controls/editor.js"},render_view=function() return "" end,render_edit=function() return "" end}`))
	block, err := parseBlockTypeTable(L, L.GetGlobal("config").(*lua.LTable), "sample")
	require.NoError(t, err)
	require.Equal(t, []string{"/plugins/sample/public/core.js", "/plugins/sample/public/controls/editor.js"}, block.Scripts)

	for _, script := range []string{"../other.js", "nested/../../other.js", "/outside.js", "https://example.com/a.js", "//example.com/a.js", "a.js?redirect=1", "a.js#hash", "a%2f.js", `a\b.js`} {
		t.Run(script, func(t *testing.T) {
			_, err := NewPluginBlockType(PluginBlockTypeConfig{PluginName: "sample", Scripts: []string{script}})
			require.Error(t, err)
		})
	}
	for _, declaration := range []string{`"core.js"`, `{[2]="core.js"}`, `{[1.5]="core.js"}`, `{true}`} {
		require.NoError(t, L.DoString(`config.scripts = `+declaration))
		_, err := parseBlockTypeTable(L, L.GetGlobal("config").(*lua.LTable), "sample")
		require.Error(t, err)
	}
}
