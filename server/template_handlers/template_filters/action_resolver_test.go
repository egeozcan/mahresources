package template_filters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

// actionProbeLua registers one action per rule the [action] resolver applies.
const actionProbeLua = `
plugin = { name = "acts", version = "1.0", description = "[action] resolver probe" }

local function ok(ctx) return { success = true } end

function init()
    mah.action({ id = "plain", label = "Plain", confirm = "Sure?", entity = "resource", handler = ok })
    mah.action({ id = "later", label = "Later", entity = "resource", async = true, handler = ok })
    mah.action({ id = "on-group", label = "On group", entity = "group", handler = ok })
    mah.action({ id = "pngs", label = "PNGs", entity = "resource", filters = { content_types = { "image/png" } }, handler = ok })
    mah.action({ id = "cat7", label = "Cat 7", entity = "resource", filters = { category_ids = { 7 } }, handler = ok })
    mah.action({ id = "group-cat7", label = "Group cat 7", entity = "group", filters = { category_ids = { 7 } }, handler = ok })
    mah.action({ id = "type5", label = "Type 5", entity = "note", filters = { note_type_ids = { 5 } }, handler = ok })
    mah.action({
        id = "typed", label = "Typed", entity = "resource", handler = ok,
        params = {
            { name = "scale", type = "number", label = "Scale", default = 1, max = 4 },
            { name = "flag", type = "boolean", label = "Flag" },
            { name = "mode", type = "select", label = "Mode", options = { "a", "b" }, default = "a" },
            { name = "note", type = "text", label = "Note" },
            { name = "secret", type = "hidden", label = "Secret", default = "s" },
            { name = "extra", type = "text", label = "Extra", default = "x", show_when = { mode = "b" } },
            { name = "help", type = "info", label = "Help" },
            { name = "target", type = "entity_ref", entity = "resource", label = "Target" },
            { name = "targets", type = "entity_ref", entity = "resource", multi = true, label = "Targets" },
            { name = "picked", type = "entity_ref", entity = "resource", default = "selection", label = "Picked" },
            { name = "owner", type = "entity_ref", entity = "group", label = "Owner" },
        },
    })
    mah.action({
        id = "bulk-only", label = "Bulk only", entity = "resource", placement = { "bulk" }, handler = ok,
        params = { { name = "sel", type = "entity_ref", entity = "resource", multi = true, default = "selection", required = true, label = "Selection" } },
    })
    mah.action({
        id = "mixed-mode", label = "Mixed mode", entity = "resource", handler = ok,
        params = {
            { name = "mode", type = "select", options = { "single", "bulk" }, default = "single" },
            { name = "sel", type = "entity_ref", entity = "resource", default = "selection", required = true, show_when = { mode = "bulk" } },
        },
    })
    mah.action({
        id = "trigger-filter", label = "Trigger filter", entity = "resource", handler = ok,
        params = {
            { name = "advanced", type = "boolean", default = true },
            { name = "target", type = "entity_ref", entity = "resource", required = true, filters = { content_types = { "image/jpeg" } }, show_when = { advanced = true } },
        },
    })
    mah.action({
        id = "trigger-filter-needs-text", label = "Trigger filter needs text", entity = "resource", handler = ok,
        params = {
            { name = "target", type = "entity_ref", entity = "resource", filters = { content_types = { "image/jpeg" } } },
            { name = "why", type = "text", label = "Why", required = true },
        },
    })
    mah.action({
        id = "needs-text", label = "Needs text", entity = "resource", handler = ok,
        params = { { name = "why", type = "text", label = "Why", required = true } },
    })
end
`

func enableActionProbePlugin(t *testing.T) *plugin_system.PluginManager {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "acts"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "acts", "plugin.lua"), []byte(actionProbeLua), 0o644))
	pm, err := plugin_system.NewPluginManager(dir)
	require.NoError(t, err)
	t.Cleanup(pm.Close)
	require.NoError(t, pm.EnablePlugin("acts"))
	return pm
}

func allowAll(string) bool { return true }

func resourceCtx(r *models.Resource) shortcodes.MetaShortcodeContext {
	return shortcodes.MetaShortcodeContext{EntityType: "resource", EntityID: r.ID, Entity: r}
}

func TestActionResolver_Unavailable(t *testing.T) {
	pm := enableActionProbePlugin(t)
	res := resourceCtx(&models.Resource{ID: 9, ContentType: "image/png"})

	cases := []struct {
		name        string
		resolver    shortcodes.ActionResolver
		plugin, act string
		ctx         shortcodes.MetaShortcodeContext
	}{
		{"access refused", BuildActionResolver(pm, func(string) bool { return false }), "acts", "plain", res},
		{"nil access", BuildActionResolver(pm, nil), "acts", "plain", res},
		{"nil plugin manager", BuildActionResolver(nil, allowAll), "acts", "plain", res},
		{"unknown plugin", BuildActionResolver(pm, allowAll), "nope", "plain", res},
		{"unknown action", BuildActionResolver(pm, allowAll), "acts", "nope", res},
		{"entity mismatch", BuildActionResolver(pm, allowAll), "acts", "on-group", res},
		{"carrier context", BuildActionResolver(pm, allowAll), "acts", "plain", shortcodes.MetaShortcodeContext{EntityType: "resource_category", EntityID: 3}},
		{"no entity id", BuildActionResolver(pm, allowAll), "acts", "plain", shortcodes.MetaShortcodeContext{EntityType: "resource"}},
		{"content type mismatch", BuildActionResolver(pm, allowAll), "acts", "pngs", resourceCtx(&models.Resource{ID: 9, ContentType: "application/pdf"})},
		{"filters with no entity data", BuildActionResolver(pm, allowAll), "acts", "pngs", shortcodes.MetaShortcodeContext{EntityType: "resource", EntityID: 9}},
		{"category mismatch", BuildActionResolver(pm, allowAll), "acts", "cat7", resourceCtx(&models.Resource{ID: 9, ResourceCategoryId: 8})},
	}
	for _, tc := range cases {
		_, err := tc.resolver(tc.plugin, tc.act, nil, tc.ctx)
		assert.True(t, errors.Is(err, shortcodes.ErrActionUnavailable), "%s: got %v", tc.name, err)
	}
}

func TestActionResolver_FiltersReadPointerAndValueEntities(t *testing.T) {
	pm := enableActionProbePlugin(t)
	resolve := BuildActionResolver(pm, allowAll)
	seven, five := uint(7), uint(5)

	ok := func(name, act, entityType string, entity any) {
		t.Helper()
		_, err := resolve("acts", act, nil, shortcodes.MetaShortcodeContext{EntityType: entityType, EntityID: 1, Entity: entity})
		assert.NoError(t, err, name)
	}
	ok("png pointer", "pngs", "resource", &models.Resource{ID: 1, ContentType: "image/png"})
	ok("png value", "pngs", "resource", models.Resource{ID: 1, ContentType: "image/png"})
	ok("resource category pointer", "cat7", "resource", &models.Resource{ID: 1, ResourceCategoryId: 7})
	ok("resource category value", "cat7", "resource", models.Resource{ID: 1, ResourceCategoryId: 7})
	ok("group category pointer", "group-cat7", "group", &models.Group{ID: 1, CategoryId: &seven})
	ok("group category value", "group-cat7", "group", models.Group{ID: 1, CategoryId: &seven})
	ok("note type pointer", "type5", "note", &models.Note{ID: 1, NoteTypeId: &five})
	ok("note type value", "type5", "note", models.Note{ID: 1, NoteTypeId: &five})

	_, err := resolve("acts", "type5", nil, shortcodes.MetaShortcodeContext{EntityType: "note", EntityID: 1, Entity: models.Note{ID: 1}})
	assert.ErrorIs(t, err, shortcodes.ErrActionUnavailable, "a note with no type fails a note-type filter")
}

func TestActionResolver_CopiesTheRegistration(t *testing.T) {
	pm := enableActionProbePlugin(t)
	resolve := BuildActionResolver(pm, allowAll)
	res := resourceCtx(&models.Resource{ID: 9})

	button, err := resolve("acts", "plain", nil, res)
	require.NoError(t, err)
	assert.Equal(t, shortcodes.ActionButton{Label: "Plain", Confirm: "Sure?", Params: map[string]any{}}, button)

	button, err = resolve("acts", "later", nil, res)
	require.NoError(t, err)
	assert.True(t, button.Async)
	assert.Empty(t, button.Confirm)
}

func TestActionResolver_ParamDefaults(t *testing.T) {
	pm := enableActionProbePlugin(t)
	button, err := BuildActionResolver(pm, allowAll)("acts", "typed", nil, resourceCtx(&models.Resource{ID: 9}))
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"scale":   float64(1),
		"flag":    false,
		"mode":    "a",
		"note":    "",
		"secret":  "s",
		"target":  float64(9),        // entity_ref defaults to the trigger: this entity
		"targets": []any{float64(9)}, // and as a one-element list when multi
		"picked":  nil,               // a selection default has nothing to select here
		"owner":   nil,               // a group ref on a resource action gets no id
	}, button.Params, "extra is hidden by show_when and stripped; help is info and has no value")
}

func TestActionResolver_ParamCoercion(t *testing.T) {
	pm := enableActionProbePlugin(t)
	resolve := BuildActionResolver(pm, allowAll)
	res := resourceCtx(&models.Resource{ID: 9})

	button, err := resolve("acts", "typed", map[string]string{"scale": " 2.5 ", "flag": "true", "mode": "b", "note": "<b>hi</b>", "extra": "shown"}, res)
	require.NoError(t, err)
	assert.Equal(t, 2.5, button.Params["scale"])
	assert.Equal(t, true, button.Params["flag"])
	assert.Equal(t, "b", button.Params["mode"])
	assert.Equal(t, "<b>hi</b>", button.Params["note"], "text is taken literally; the handler escapes on output")
	assert.Equal(t, "shown", button.Params["extra"], "visible once mode=b")

	bad := []struct {
		attrs map[string]string
		want  string
	}{
		{map[string]string{"scale": "abc"}, "scale: must be a number"},
		{map[string]string{"scale": "Inf"}, "scale: must be a number"},
		{map[string]string{"scale": "NaN"}, "scale: must be a number"},
		{map[string]string{"scale": "5"}, "scale: Scale must be at most 4"},
		{map[string]string{"flag": "yes"}, "flag: must be true or false"},
		{map[string]string{"mode": "c"}, "mode: Mode must be one of the available options"},
		{map[string]string{"nope": "1"}, "nope: no such parameter"},
		{map[string]string{"target": "3"}, "target: cannot be set from a template"},
		{map[string]string{"help": "x"}, "help: cannot be set from a template"},
	}
	for _, tc := range bad {
		_, err := resolve("acts", "typed", tc.attrs, res)
		require.Error(t, err, "%v", tc.attrs)
		assert.False(t, errors.Is(err, shortcodes.ErrActionUnavailable), "%v is an author error", tc.attrs)
		assert.Equal(t, tc.want, err.Error())
	}

	_, err = resolve("acts", "needs-text", nil, res)
	assert.EqualError(t, err, "why: Why is required")
	_, err = resolve("acts", "needs-text", map[string]string{"why": "because"}, res)
	assert.NoError(t, err)
}

func TestActionResolver_SelectionOnlyAction(t *testing.T) {
	pm := enableActionProbePlugin(t)
	_, err := BuildActionResolver(pm, allowAll)("acts", "bulk-only", nil, resourceCtx(&models.Resource{ID: 9}))
	assert.EqualError(t, err, "sel: needs a bulk selection and cannot run from a template")
}

func TestActionResolver_SelectionRequiredOnlyWhenVisible(t *testing.T) {
	resolve := BuildActionResolver(enableActionProbePlugin(t), allowAll)
	res := resourceCtx(&models.Resource{ID: 9})

	button, err := resolve("acts", "mixed-mode", nil, res)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"mode": "single"}, button.Params)

	_, err = resolve("acts", "mixed-mode", map[string]string{"mode": "bulk"}, res)
	assert.EqualError(t, err, "sel: needs a bulk selection and cannot run from a template")
}

func TestActionResolver_VisibleTriggerReferenceMustMatchItsOwnFilter(t *testing.T) {
	resolve := BuildActionResolver(enableActionProbePlugin(t), allowAll)
	png := resourceCtx(&models.Resource{ID: 9, ContentType: "image/png"})
	jpeg := resourceCtx(&models.Resource{ID: 10, ContentType: "image/jpeg"})

	_, err := resolve("acts", "trigger-filter", nil, png)
	assert.ErrorIs(t, err, shortcodes.ErrActionUnavailable)

	button, err := resolve("acts", "trigger-filter", nil, jpeg)
	require.NoError(t, err)
	assert.Equal(t, float64(10), button.Params["target"])

	button, err = resolve("acts", "trigger-filter", map[string]string{"advanced": "false"}, png)
	require.NoError(t, err)
	assert.NotContains(t, button.Params, "target")
}

func TestActionResolver_TriggerReferenceEligibilityPrecedesRequiredParams(t *testing.T) {
	resolve := BuildActionResolver(enableActionProbePlugin(t), allowAll)
	reqCtx := shortcodes.WithActionResolver(context.Background(), resolve)
	sc := shortcodes.Shortcode{Attrs: map[string]string{"plugin": "acts", "name": "trigger-filter-needs-text"}}
	png := resourceCtx(&models.Resource{ID: 9, ContentType: "image/png"})
	jpeg := resourceCtx(&models.Resource{ID: 10, ContentType: "image/jpeg"})

	_, err := resolve("acts", "trigger-filter-needs-text", nil, png)
	assert.ErrorIs(t, err, shortcodes.ErrActionUnavailable)
	assert.Equal(t, "<!-- mr:action unavailable in this context -->", shortcodes.RenderActionShortcode(reqCtx, sc, png))
	assert.Contains(t, shortcodes.RenderActionShortcode(reqCtx, sc, jpeg), "[action] why: Why is required")
}

// renderActionTag pushes content through the process_shortcodes tag with no
// _pluginAccess on the context, so the tag has to fall back to its own rule.
func renderActionTag(t *testing.T, pm *plugin_system.PluginManager, p *auth.Principal, content string, entity any) string {
	t.Helper()
	tpl, err := pongo2.FromString(`{% process_shortcodes content entity %}|{% custom_css entity %}`)
	require.NoError(t, err)
	out, err := tpl.Execute(pongo2.Context{
		"content":         content,
		"entity":          entity,
		"_pluginManager":  pm,
		"_requestContext": auth.WithPrincipal(context.Background(), p),
	})
	require.NoError(t, err)
	return out
}

func TestActionShortcodeTag_FollowsTheRunEndpointRule(t *testing.T) {
	pm := enableActionProbePlugin(t)
	scope := uint(3)
	entity := &models.Resource{ID: 9, ResourceCategory: &models.ResourceCategory{ID: 2, CustomCSS: `/*[action plugin="acts" name="plain"]*/`}}
	const content = `[action plugin="acts" name="plain"]`

	cases := []struct {
		name   string
		p      *auth.Principal
		button bool
	}{
		{"admin", &auth.Principal{UserID: 1, Role: models.RoleAdmin}, true},
		{"unscoped user", &auth.Principal{UserID: 2, Role: models.RoleUser}, true},
		{"group-limited user, no _pluginAccess", &auth.Principal{UserID: 3, Role: models.RoleUser, ScopeGroupID: &scope}, false},
		{"guest", &auth.Principal{UserID: 4, Role: models.RoleGuest, ScopeGroupID: &scope}, false},
	}
	for _, tc := range cases {
		out := renderActionTag(t, pm, tc.p, content, entity)
		body, css, _ := strings.Cut(out, "|")
		if tc.button {
			assert.Contains(t, body, `<action-shortcode data-plugin="acts" data-action="plain" data-entity-type="resource" data-entity-id="9"`, tc.name)
		} else {
			assert.Equal(t, "<!-- mr:action unavailable in this context -->", body, tc.name)
		}
		// The resolver never reaches the custom_css tag, which reuses the
		// render context the first tag stored.
		assert.Contains(t, css, "<!-- mr:action unavailable in this context -->", tc.name)
		assert.NotContains(t, css, "<action-shortcode", tc.name)
	}
}
