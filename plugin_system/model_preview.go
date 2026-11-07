package plugin_system

import (
	"context"

	lua "github.com/yuin/gopher-lua"
)

// A model preview is a shortcode render the template-generation harness runs on
// a model's behalf (docs/plans/2026-11-07-template-generation-harness.md
// section 2). Every harness Tool is read-only, so while the mark is set the
// plugin may only call the read-only host functions below. A capability
// allow-list cannot do this: media.trim, the job reporters and registrations
// are all reachable from capabilities that look harmless.

type modelPreviewKey struct{}

// WithModelPreview marks ctx as a model preview. The mark rides the request
// context into the plugin's Lua context and into its Invocation.
func WithModelPreview(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, modelPreviewKey{}, true)
}

// IsModelPreview reports whether ctx carries the model-preview mark.
func IsModelPreview(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	marked, _ := ctx.Value(modelPreviewKey{}).(bool)
	return marked
}

func (pm *PluginManager) isModelPreview(L *lua.LState) bool {
	return IsModelPreview(pm.luaContext(L))
}

// modelPreviewAllowed is the read-only allow-list, by dotted name under mah.
// mah.log is on it only because it is a no-op under the mark. mah.doc is on it
// because its registration is refused by stateMayRegisterLocked. mrql_query is
// not: it reaches the database through its own executor, not querierFor, so it
// is outside "the readers built on querierFor". The drift-guard test fails on
// an entry that no longer exists and on any live function that is neither here
// nor refused.
var modelPreviewAllowed = map[string]bool{
	"db.count_groups": true, "db.count_notes": true, "db.count_resources": true,
	"db.each_resource_json": true, "db.get_category": true, "db.get_group": true,
	"db.get_note": true, "db.get_note_type": true, "db.get_resource": true,
	"db.get_resource_category": true, "db.get_resource_data": true, "db.get_series": true,
	"db.get_tag": true, "db.list_categories": true, "db.list_note_types": true,
	"db.list_resource_categories": true, "db.list_tags": true, "db.query_groups": true,
	"db.query_notes": true, "db.query_resources": true,

	"json.array": true, "json.decode": true, "json.encode": true,

	"util.base64.decode": true, "util.base64.encode": true, "util.hex.decode": true,
	"util.hex.encode": true, "util.hmac_sha256": true, "util.now": true,
	"util.now_iso": true, "util.secure_compare": true, "util.sha256": true,
	"util.today": true,

	"html_escape": true, "doc": true, "get_setting": true, "log": true,

	"image.pad_to_aspect_ratio": true, "image.to_png": true,
}

// guardMahTable wraps every Go function in the mah table, recursively, so that
// under the model-preview mark a function off the allow-list raises instead of
// running. It runs once, after the whole table is built, so a function added to
// registerMahModule later is refused by default.
func (pm *PluginManager) guardMahTable(L *lua.LState, tbl *lua.LTable, prefix string) {
	type entry struct {
		key lua.LValue
		val lua.LValue
	}
	var entries []entry
	tbl.ForEach(func(k, v lua.LValue) { entries = append(entries, entry{k, v}) })
	for _, e := range entries {
		name := prefix + e.key.String()
		switch v := e.val.(type) {
		case *lua.LTable:
			pm.guardMahTable(L, v, name+".")
		case *lua.LFunction:
			if v.GFunction == nil {
				continue
			}
			inner := v.GFunction
			allowed := modelPreviewAllowed[name]
			tbl.RawSet(e.key, L.NewFunction(func(L *lua.LState) int {
				if !allowed && pm.isModelPreview(L) {
					L.RaiseError("mah.%s is %s", name, "not allowed in a model preview")
					return 0
				}
				return inner(L)
			}))
		}
	}
}
