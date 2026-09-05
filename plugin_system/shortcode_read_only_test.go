package plugin_system

import (
	"context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/shortcodes"
	"testing"
)

func TestShortcodeReadOnlyCapability(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "readonly", `plugin={name="readonly",version="1"}
 function init() mah.shortcode({name="capability",label="Capability",render=function(ctx) return tostring(ctx.read_only)..":"..tostring(ctx.can_write) end}) end`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer pm.Close()
	if err := pm.EnablePlugin("readonly"); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		principal *auth.Principal
		force     bool
		want      string
	}{
		{"missing", nil, false, "true:false"},
		{"guest", &auth.Principal{Role: models.RoleGuest}, false, "true:false"},
		{"writer", &auth.Principal{Role: models.RoleUser}, false, "false:true"},
		{"share", &auth.Principal{SuperUser: true}, true, "true:false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := auth.WithPrincipal(context.Background(), tc.principal)
			got, err := pm.RenderShortcodeContext(ctx, "readonly", "plugin:readonly:capability", shortcodes.MetaShortcodeContext{ForceReadOnly: tc.force}, nil, "", false)
			if err != nil || got != tc.want {
				t.Fatalf("got %q %v, want %q", got, err, tc.want)
			}
		})
	}
}
