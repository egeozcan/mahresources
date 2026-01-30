package template_filters

import (
	"github.com/flosch/pongo2/v4"
)

// lookupFilter looks up a value in a map using the provided key.
// Usage: {{ map|lookup:key }}
// Supports:
// - map[uint]string for resource hash lookups
// - map[float64]string for group name lookups (JSON numbers are float64)
// - map[string]interface{} for table row lookups
// - map[string]string for general string maps
func lookupFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	// Get the map
	m := in.Interface()

	// Get the key
	key := param.Interface()

	// Handle map[uint]string (resource hash map)
	if hashMap, ok := m.(map[uint]string); ok {
		var uintKey uint
		switch k := key.(type) {
		case int:
			uintKey = uint(k)
		case int64:
			uintKey = uint(k)
		case uint:
			uintKey = k
		case uint64:
			uintKey = uint(k)
		case float64:
			uintKey = uint(k)
		default:
			return pongo2.AsValue(""), nil
		}

		if val, exists := hashMap[uintKey]; exists {
			return pongo2.AsValue(val), nil
		}
	}

	// Handle map[float64]string (group name map - JSON numbers are float64)
	if floatMap, ok := m.(map[float64]string); ok {
		var floatKey float64
		switch k := key.(type) {
		case float64:
			floatKey = k
		case int:
			floatKey = float64(k)
		case int64:
			floatKey = float64(k)
		case uint:
			floatKey = float64(k)
		case uint64:
			floatKey = float64(k)
		default:
			return pongo2.AsValue(""), nil
		}

		if val, exists := floatMap[floatKey]; exists {
			return pongo2.AsValue(val), nil
		}
	}

	// Handle map[float64]interface{} (group data map with struct values)
	if floatAnyMap, ok := m.(map[float64]any); ok {
		var floatKey float64
		switch k := key.(type) {
		case float64:
			floatKey = k
		case int:
			floatKey = float64(k)
		case int64:
			floatKey = float64(k)
		case uint:
			floatKey = float64(k)
		case uint64:
			floatKey = float64(k)
		default:
			return pongo2.AsValue(nil), nil
		}

		if val, exists := floatAnyMap[floatKey]; exists {
			return pongo2.AsValue(val), nil
		}
	}

	// Handle map[string]interface{} (table rows, JSON objects)
	if strMap, ok := m.(map[string]interface{}); ok {
		strKey, ok := key.(string)
		if !ok {
			return pongo2.AsValue(""), nil
		}
		if val, exists := strMap[strKey]; exists {
			return pongo2.AsValue(val), nil
		}
	}

	// Handle map[string]string
	if strStrMap, ok := m.(map[string]string); ok {
		strKey, ok := key.(string)
		if !ok {
			return pongo2.AsValue(""), nil
		}
		if val, exists := strStrMap[strKey]; exists {
			return pongo2.AsValue(val), nil
		}
	}

	return pongo2.AsValue(""), nil
}
