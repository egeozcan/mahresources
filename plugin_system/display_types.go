package plugin_system

import (
	"fmt"
	"regexp"

	lua "github.com/yuin/gopher-lua"
)

// PluginDisplayType holds a plugin-defined display renderer.
type PluginDisplayType struct {
	PluginName string
	TypeName   string // full namespaced: plugin:<pluginName>:<type>
	Label      string
	Render     *lua.LFunction
	State      *lua.LState
}

var validDisplayTypeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,49}$`)

// parseDisplayTypeTable parses a Lua table from mah.display_type({...}) into a PluginDisplayType.
// Required fields: type, label, render.
func parseDisplayTypeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginDisplayType, error) {
	dt := &PluginDisplayType{
		PluginName: pluginName,
	}

	// Required: type
	if v := tbl.RawGetString("type"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'type'")
	} else if str, ok := v.(lua.LString); !ok {
		return nil, fmt.Errorf("'type' must be a string, got %s", v.Type())
	} else {
		raw := string(str)
		if !validDisplayTypeName.MatchString(raw) {
			return nil, fmt.Errorf("invalid type name %q: must match [a-z][a-z0-9-]{0,49}", raw)
		}
		dt.TypeName = "plugin:" + pluginName + ":" + raw
	}

	// Required: label
	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		dt.Label = v.String()
	}

	// Required: render (must be a function)
	if v := tbl.RawGetString("render"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render' must be a function")
	} else {
		dt.Render = fn
	}

	return dt, nil
}
