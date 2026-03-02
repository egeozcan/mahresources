package plugin_system

import (
	"fmt"
	"slices"

	lua "github.com/yuin/gopher-lua"
)

// SettingDefinition describes a single plugin setting.
type SettingDefinition struct {
	Name         string   `json:"name"`
	Type         string   `json:"type"` // string, password, boolean, number, select
	Label        string   `json:"label"`
	Required     bool     `json:"required,omitempty"`
	DefaultValue any      `json:"default,omitempty"`
	Options      []string `json:"options,omitempty"` // for type=select
}

// ValidationError describes a single setting validation failure.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// parseSettingsFromLua executes a Lua script string in a throwaway VM and
// extracts the plugin.settings table into Go structs. Only safe libraries
// are opened (base, table, string, math). The script's init() is NOT called.
func parseSettingsFromLua(script string) ([]SettingDefinition, error) {
	L := lua.NewState(lua.Options{SkipOpenLibs: true})
	defer L.Close()

	// Open only safe libraries.
	for _, pair := range []struct {
		name string
		fn   lua.LGFunction
	}{
		{lua.BaseLibName, lua.OpenBase},
		{lua.TabLibName, lua.OpenTable},
		{lua.StringLibName, lua.OpenString},
		{lua.MathLibName, lua.OpenMath},
	} {
		L.Push(L.NewFunction(pair.fn))
		L.Push(lua.LString(pair.name))
		L.Call(1, 0)
	}

	if err := L.DoString(script); err != nil {
		return nil, fmt.Errorf("executing lua script: %w", err)
	}

	return extractSettingsFromState(L), nil
}

// extractSettingsFromState reads plugin.settings from an already-executed Lua
// state and returns the parsed definitions. This is separate from
// parseSettingsFromLua so it can be reused by discoverPlugin (which has its
// own Lua state).
func extractSettingsFromState(L *lua.LState) []SettingDefinition {
	pluginGlobal := L.GetGlobal("plugin")
	pluginTable, ok := pluginGlobal.(*lua.LTable)
	if !ok {
		return nil
	}

	settingsVal := pluginTable.RawGetString("settings")
	settingsTable, ok := settingsVal.(*lua.LTable)
	if !ok {
		return nil
	}

	var defs []SettingDefinition
	settingsTable.ForEach(func(_ lua.LValue, value lua.LValue) {
		tbl, ok := value.(*lua.LTable)
		if !ok {
			return
		}

		def := SettingDefinition{}

		if v := tbl.RawGetString("name"); v != lua.LNil {
			def.Name = v.String()
		}
		if v := tbl.RawGetString("type"); v != lua.LNil {
			def.Type = v.String()
		}
		if v := tbl.RawGetString("label"); v != lua.LNil {
			def.Label = v.String()
		}
		if v := tbl.RawGetString("required"); v == lua.LTrue {
			def.Required = true
		}

		// Parse default value based on type.
		if v := tbl.RawGetString("default"); v != lua.LNil {
			switch {
			case v == lua.LTrue:
				def.DefaultValue = true
			case v == lua.LFalse:
				def.DefaultValue = false
			case v.Type() == lua.LTNumber:
				def.DefaultValue = float64(v.(lua.LNumber))
			default:
				def.DefaultValue = v.String()
			}
		}

		// Parse options for select type.
		if opts := tbl.RawGetString("options"); opts != lua.LNil {
			if optsTable, ok := opts.(*lua.LTable); ok {
				optsTable.ForEach(func(_ lua.LValue, val lua.LValue) {
					def.Options = append(def.Options, val.String())
				})
			}
		}

		defs = append(defs, def)
	})

	return defs
}

// ValidateSettings validates setting values against their definitions.
// It checks required fields, type correctness for boolean/number/select,
// and that select values match one of the declared options.
func ValidateSettings(defs []SettingDefinition, values map[string]any) []ValidationError {
	var errs []ValidationError

	for _, def := range defs {
		val, exists := values[def.Name]

		// Required check.
		if def.Required && (!exists || val == nil || val == "") {
			errs = append(errs, ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("%s is required", def.Label),
			})
			continue
		}

		// Skip further checks if value is absent or empty.
		if !exists || val == nil || val == "" {
			continue
		}

		switch def.Type {
		case "boolean":
			if _, ok := val.(bool); !ok {
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be a boolean", def.Label),
				})
			}

		case "number":
			switch val.(type) {
			case float64, int, int64:
				// valid
			default:
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be a number", def.Label),
				})
			}

		case "select":
			strVal, ok := val.(string)
			if !ok {
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be a string", def.Label),
				})
				continue
			}
			if !slices.Contains(def.Options, strVal) {
				errs = append(errs, ValidationError{
					Field:   def.Name,
					Message: fmt.Sprintf("%s must be one of the available options", def.Label),
				})
			}

		case "string", "password":
			// No type validation beyond the required check above.

		default:
			errs = append(errs, ValidationError{
				Field:   def.Name,
				Message: fmt.Sprintf("%s has unknown setting type %q", def.Label, def.Type),
			})
		}
	}

	return errs
}

// CheckRequiredSettings returns the labels of required settings that are
// missing or empty in the provided values map.
func CheckRequiredSettings(defs []SettingDefinition, values map[string]any) []string {
	var missing []string
	for _, def := range defs {
		if !def.Required {
			continue
		}
		val, exists := values[def.Name]
		if !exists || val == nil || val == "" {
			missing = append(missing, def.Label)
		}
	}
	return missing
}
