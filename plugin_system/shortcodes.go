package plugin_system

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const luaShortcodeRenderTimeout = 5 * time.Second

// ShortcodeDocAttr describes a single shortcode attribute for documentation.
type ShortcodeDocAttr struct {
	Name        string
	Type        string
	Required    bool
	Default     string
	Description string
}

// ShortcodeDocExample is a usage example for shortcode documentation.
type ShortcodeDocExample struct {
	Title       string
	Code        string
	Notes       string
	ExampleData map[string]any // optional mock meta values for live preview on docs pages
}

// PluginDoc is a general documentation entry registered via mah.doc().
// It lets plugins document any feature (actions, pages, settings, etc.).
type PluginDoc struct {
	PluginName  string
	Name        string // URL slug, e.g. "colorize"
	Label       string
	Description string
	Category    string // e.g. "Action", "Page", "" for custom
	Attrs       []ShortcodeDocAttr
	Examples    []ShortcodeDocExample
	Notes       []string
}

type PluginShortcode struct {
	PluginName string
	TypeName   string // full: plugin:<pluginName>:<name>
	Label      string
	Render     *lua.LFunction
	State      *lua.LState
	// Documentation (optional). A shortcode is "documented" when Description is non-empty.
	Description string
	Attrs       []ShortcodeDocAttr
	Examples    []ShortcodeDocExample
	Notes       []string
}

var validShortcodeName = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,49}$`)

func parseShortcodeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginShortcode, error) {
	sc := &PluginShortcode{PluginName: pluginName}

	if v := tbl.RawGetString("name"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'name'")
	} else if str, ok := v.(lua.LString); !ok {
		return nil, fmt.Errorf("'name' must be a string, got %s", v.Type())
	} else {
		raw := string(str)
		if !validShortcodeName.MatchString(raw) {
			return nil, fmt.Errorf("invalid shortcode name %q: must match [a-z][a-z0-9-]{0,49}", raw)
		}
		sc.TypeName = "plugin:" + pluginName + ":" + raw
	}

	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		sc.Label = v.String()
	}

	if v := tbl.RawGetString("render"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render' must be a function")
	} else {
		sc.Render = fn
	}

	// Optional documentation fields.
	if v := tbl.RawGetString("description"); v != lua.LNil {
		sc.Description = v.String()
	}

	if v := tbl.RawGetString("attrs"); v != lua.LNil {
		if attrsTbl, ok := v.(*lua.LTable); ok {
			sc.Attrs = parseDocAttrs(attrsTbl)
		}
	}

	if v := tbl.RawGetString("examples"); v != lua.LNil {
		if exTbl, ok := v.(*lua.LTable); ok {
			sc.Examples = parseDocExamples(exTbl)
		}
	}

	if v := tbl.RawGetString("notes"); v != lua.LNil {
		if notesTbl, ok := v.(*lua.LTable); ok {
			notesTbl.ForEach(func(_, val lua.LValue) {
				if s, ok := val.(lua.LString); ok {
					sc.Notes = append(sc.Notes, string(s))
				}
			})
		}
	}

	return sc, nil
}

func parseDocAttrs(tbl *lua.LTable) []ShortcodeDocAttr {
	var attrs []ShortcodeDocAttr
	tbl.ForEach(func(_, val lua.LValue) {
		row, ok := val.(*lua.LTable)
		if !ok {
			return
		}
		attr := ShortcodeDocAttr{}
		if v := row.RawGetString("name"); v != lua.LNil {
			attr.Name = v.String()
		}
		if v := row.RawGetString("type"); v != lua.LNil {
			attr.Type = v.String()
		}
		if v := row.RawGetString("required"); v != lua.LNil {
			if b, ok := v.(lua.LBool); ok {
				attr.Required = bool(b)
			}
		}
		if v := row.RawGetString("default"); v != lua.LNil {
			attr.Default = v.String()
		}
		if v := row.RawGetString("description"); v != lua.LNil {
			attr.Description = v.String()
		}
		attrs = append(attrs, attr)
	})
	return attrs
}

func parseDocExamples(tbl *lua.LTable) []ShortcodeDocExample {
	var examples []ShortcodeDocExample
	tbl.ForEach(func(_, val lua.LValue) {
		row, ok := val.(*lua.LTable)
		if !ok {
			return
		}
		ex := ShortcodeDocExample{}
		if v := row.RawGetString("title"); v != lua.LNil {
			ex.Title = v.String()
		}
		if v := row.RawGetString("code"); v != lua.LNil {
			ex.Code = v.String()
		}
		if v := row.RawGetString("notes"); v != lua.LNil {
			ex.Notes = v.String()
		}
		if v := row.RawGetString("example_data"); v != lua.LNil {
			if dataTbl, ok := v.(*lua.LTable); ok {
				ex.ExampleData = luaTableToGoMap(dataTbl)
			}
		}
		examples = append(examples, ex)
	})
	return examples
}

func (pm *PluginManager) GetPluginShortcode(fullTypeName string) *PluginShortcode {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, scs := range pm.shortcodes {
		for _, sc := range scs {
			if sc.TypeName == fullTypeName {
				return sc
			}
		}
	}
	return nil
}

func (pm *PluginManager) RenderShortcode(reqCtx context.Context, pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string, entity any) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	sc := pm.GetPluginShortcode(fullTypeName)
	if sc == nil {
		return "", fmt.Errorf("shortcode %q not found", fullTypeName)
	}
	if sc.PluginName != pluginName {
		return "", fmt.Errorf("shortcode %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	fn := sc.Render
	if fn == nil {
		return "", fmt.Errorf("no render function for shortcode %q", fullTypeName)
	}

	L := sc.State
	mu := pm.VMLock(L)
	if mu == nil {
		return "", fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	mu.Lock()
	defer mu.Unlock()

	var metaMap map[string]any
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &metaMap)
	}
	if metaMap == nil {
		metaMap = map[string]any{}
	}

	attrsMap := make(map[string]any, len(attrs))
	for k, v := range attrs {
		attrsMap[k] = v
	}

	settings := pm.GetPluginSettings(pluginName)
	if settings == nil {
		settings = map[string]any{}
	}

	ctxData := map[string]any{
		"entity_type": entityType,
		"entity_id":   float64(entityID),
		"value":       metaMap,
		"attrs":       attrsMap,
		"settings":    settings,
	}

	if entity != nil {
		ctxData["entity"] = entityToMap(entity)
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(reqCtx, luaShortcodeRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: shortcode render %q/%q returned error: %v", pluginName, fullTypeName, err)
		return "", fmt.Errorf("shortcode render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", fmt.Errorf("shortcode %q render function must return a string, got %s", fullTypeName, ret.Type())
}

// entityToMap converts an entity struct to a map[string]any using reflection.
func entityToMap(entity any) map[string]any {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	result := make(map[string]any)
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if !field.IsExported() {
			continue
		}
		fv := v.Field(i)
		val := entityFieldValue(fv)
		if val != nil {
			result[field.Name] = val
		}
	}
	return result
}

// entityFieldValue extracts a Lua-compatible value from a reflect.Value.
func entityFieldValue(fv reflect.Value) any {
	if fv.Kind() == reflect.Ptr {
		if fv.IsNil() {
			return nil
		}
		fv = fv.Elem()
	}

	iface := fv.Interface()

	if t, ok := iface.(time.Time); ok {
		return t.Format(time.RFC3339)
	}
	if raw, ok := iface.(json.RawMessage); ok {
		return string(raw)
	}
	if v, ok := iface.(driver.Valuer); ok {
		if dbVal, err := v.Value(); err == nil {
			if s, ok := dbVal.(string); ok {
				return s
			}
		}
	}

	switch fv.Kind() {
	case reflect.String:
		return fv.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(fv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(fv.Uint())
	case reflect.Float32, reflect.Float64:
		return fv.Float()
	case reflect.Bool:
		return fv.Bool()
	default:
		return nil
	}
}

// renderShortcodeForDocs renders a shortcode in documentation preview mode.
// It uses entityType "group" with entityID 0 and sets preview=true in the
// context so plugins can disable side effects (e.g. meta-editors skip saves).
func (pm *PluginManager) renderShortcodeForDocs(pluginName, fullTypeName string, meta json.RawMessage, attrs map[string]string) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	sc := pm.GetPluginShortcode(fullTypeName)
	if sc == nil {
		return "", fmt.Errorf("shortcode %q not found", fullTypeName)
	}
	if sc.PluginName != pluginName {
		return "", fmt.Errorf("shortcode %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	fn := sc.Render
	if fn == nil {
		return "", fmt.Errorf("no render function for shortcode %q", fullTypeName)
	}

	L := sc.State
	mu := pm.VMLock(L)
	if mu == nil {
		return "", fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	mu.Lock()
	defer mu.Unlock()

	var metaMap map[string]any
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &metaMap)
	}
	if metaMap == nil {
		metaMap = map[string]any{}
	}

	attrsMap := make(map[string]any, len(attrs))
	for k, v := range attrs {
		attrsMap[k] = v
	}

	settings := pm.GetPluginSettings(pluginName)
	if settings == nil {
		settings = map[string]any{}
	}

	ctxData := map[string]any{
		"entity_type": "group",
		"entity_id":   float64(0),
		"value":       metaMap,
		"attrs":       attrsMap,
		"settings":    settings,
		"preview":     true,
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaShortcodeRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		return "", fmt.Errorf("shortcode preview render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", fmt.Errorf("shortcode %q render function must return a string, got %s", fullTypeName, ret.Type())
}
