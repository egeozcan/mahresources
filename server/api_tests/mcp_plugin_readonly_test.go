package api_tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/application_context"
)

// A read-only MCP operation stays read-only when plugin code runs inside it.
// Template previews render plugin shortcodes, and a shortcode can call the
// mah.db writers, so before every read-only dispatch carried the model-preview
// mark this preview deleted a note with no confirmation (review round 12).
func TestMCP_ReadOnlyOperationsCannotRunPluginWrites(t *testing.T) {
	pluginDir := t.TempDir()
	sub := filepath.Join(pluginDir, "mcp-ro")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	lua := `
plugin = { name = "mcp-ro", version = "1.0", api_version = 1, capabilities = { "render", "db:read", "db:write" } }
function init()
  mah.shortcode({ name = "deleter", label = "deleter", render = function(ctx)
    mah.db.delete_note(tonumber(ctx.attrs.id))
    return "ran"
  end })
end
`
	if err := os.WriteFile(filepath.Join(sub, "plugin.lua"), []byte(lua), 0o644); err != nil {
		t.Fatal(err)
	}
	env := newMCPEnvOn(t, func(t *testing.T, mutate func(*application_context.MahresourcesConfig)) *TestContext {
		return setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
			mutate(c)
			c.PluginPath = pluginDir
		})
	}, false)
	pm := env.tc.AppCtx.PluginManager()
	if pm == nil {
		t.Fatal("no plugin manager")
	}
	if err := pm.EnablePlugin("mcp-ro"); err != nil {
		t.Fatalf("enable plugin: %v", err)
	}
	victim := env.tc.CreateDummyNote("victim")
	host := env.tc.CreateDummyNote("host")

	cs := env.connect(t, clientOpts{})
	res := callTool(t, cs, "call_operation", map[string]any{"operationId": "previewNoteTypeTemplate",
		"arguments": map[string]any{"entityId": host.ID, "content": fmt.Sprintf(`[plugin:mcp-ro:deleter id="%d"]`, victim.ID)}})
	if !env.noteExists(t, victim.ID) {
		t.Fatalf("a read-only preview let a plugin delete a note: %s", resultText(res))
	}
	if strings.Contains(resultText(res), ">ran<") {
		t.Fatalf("the writing shortcode rendered: %s", resultText(res))
	}
}
