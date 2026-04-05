package plugin_system

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderDisplayWithObjectValue(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "display-obj", `
plugin = { name = "display-obj", version = "1.0" }
function init()
    mah.display_type({
        type = "badge",
        label = "Badge",
        render = function(ctx)
            return "<span>" .. tostring(ctx.value.label) .. "</span>"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("display-obj"))

	html, err := pm.RenderDisplay("display-obj", "plugin:display-obj:badge", DisplayRenderContext{
		Value:      map[string]any{"label": "OK"},
		FieldPath:  "status",
		FieldLabel: "Status",
	})
	require.NoError(t, err)
	assert.Equal(t, "<span>OK</span>", html)
}

func TestRenderDisplayWithScalarValue(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "display-scalar", `
plugin = { name = "display-scalar", version = "1.0" }
function init()
    mah.display_type({
        type = "stars",
        label = "Stars",
        render = function(ctx)
            local n = tonumber(ctx.value) or 0
            local s = ""
            for i = 1, n do s = s .. "★" end
            return "<span>" .. s .. "</span>"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("display-scalar"))

	// Scalar number value — must not fail
	html, err := pm.RenderDisplay("display-scalar", "plugin:display-scalar:stars", DisplayRenderContext{
		Value:      float64(3),
		FieldPath:  "rating",
		FieldLabel: "Rating",
	})
	require.NoError(t, err)
	assert.Equal(t, "<span>★★★</span>", html)
}

func TestRenderDisplayWithStringValue(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "display-str", `
plugin = { name = "display-str", version = "1.0" }
function init()
    mah.display_type({
        type = "upper",
        label = "Uppercase",
        render = function(ctx)
            return "<b>" .. string.upper(tostring(ctx.value)) .. "</b>"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("display-str"))

	html, err := pm.RenderDisplay("display-str", "plugin:display-str:upper", DisplayRenderContext{
		Value:      "hello",
		FieldPath:  "name",
		FieldLabel: "Name",
	})
	require.NoError(t, err)
	assert.Equal(t, "<b>HELLO</b>", html)
}

func TestRenderDisplayWithNullValue(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "display-null", `
plugin = { name = "display-null", version = "1.0" }
function init()
    mah.display_type({
        type = "nullable",
        label = "Nullable",
        render = function(ctx)
            if ctx.value == nil then return "<em>none</em>" end
            return "<span>" .. tostring(ctx.value) .. "</span>"
        end
    })
end
`)
	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()
	require.NoError(t, pm.EnablePlugin("display-null"))

	html, err := pm.RenderDisplay("display-null", "plugin:display-null:nullable", DisplayRenderContext{
		Value:      nil,
		FieldPath:  "opt",
		FieldLabel: "Optional",
	})
	require.NoError(t, err)
	assert.Equal(t, "<em>none</em>", html)
}
