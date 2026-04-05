package shortcodes

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"
)

// MetaShortcodeContext holds the entity context needed to render [meta] shortcodes.
type MetaShortcodeContext struct {
	EntityType string // "group", "resource", "note"
	EntityID   uint
	Meta       json.RawMessage // entity's full Meta JSON
	MetaSchema string          // category's MetaSchema JSON string (may be empty)
}

// RenderMetaShortcode expands a [meta] shortcode into a <meta-shortcode> custom element.
func RenderMetaShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	path := sc.Attrs["path"]
	if path == "" {
		return ""
	}

	editable := sc.Attrs["editable"] == "true"
	hideEmpty := sc.Attrs["hide-empty"] == "true"

	valueJSON := extractValueAtPath(ctx.Meta, path)
	schemaSlice := extractSchemaSlice(ctx.MetaSchema, path, ctx.Meta)

	return fmt.Sprintf(
		`<meta-shortcode data-path="%s" data-editable="%t" data-hide-empty="%t" data-entity-type="%s" data-entity-id="%d" data-schema="%s" data-value="%s"></meta-shortcode>`,
		html.EscapeString(path),
		editable,
		hideEmpty,
		html.EscapeString(ctx.EntityType),
		ctx.EntityID,
		html.EscapeString(schemaSlice),
		html.EscapeString(valueJSON),
	)
}

// extractValueAtPath navigates a JSON object by dot-notation path
// and returns the JSON-encoded value at that path, or "" if not found.
func extractValueAtPath(metaRaw json.RawMessage, path string) string {
	if len(metaRaw) == 0 {
		return ""
	}

	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return ""
	}

	parts := strings.Split(path, ".")
	var current any = meta

	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current, ok = obj[part]
		if !ok {
			return ""
		}
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// extractSchemaSlice navigates a JSON Schema by dot-notation path through
// nested "properties" and returns the JSON-encoded sub-schema, or "" if not found.
// Handles $ref, allOf, oneOf, anyOf, and if/then/else (using entityMeta to
// evaluate conditions). entityMeta may be nil if no value context is available.
func extractSchemaSlice(schemaStr string, path string, entityMeta json.RawMessage) string {
	if schemaStr == "" {
		return ""
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(schemaStr), &root); err != nil {
		return ""
	}

	// Parse entity meta for if/then/else condition evaluation.
	var metaValue map[string]any
	if len(entityMeta) > 0 {
		_ = json.Unmarshal(entityMeta, &metaValue)
	}

	parts := strings.Split(path, ".")
	current := root
	currentValue := metaValue

	for _, part := range parts {
		resolved := resolveSchemaNodeWithValue(current, root, currentValue)
		if resolved == nil {
			return ""
		}
		props, ok := resolved["properties"].(map[string]any)
		if !ok {
			return ""
		}
		sub, ok := props[part].(map[string]any)
		if !ok {
			return ""
		}
		current = sub
		// Descend into the value too for next-level condition evaluation
		if currentValue != nil {
			if nested, ok := currentValue[part].(map[string]any); ok {
				currentValue = nested
			} else {
				currentValue = nil
			}
		}
	}

	// Resolve the leaf node too (it might be a $ref or conditional)
	resolved := resolveSchemaNodeWithValue(current, root, currentValue)
	if resolved == nil {
		resolved = current
	}

	encoded, err := json.Marshal(resolved)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// resolveSchemaNodeWithValue recursively resolves $ref, allOf, oneOf, anyOf,
// and if/then/else on a schema node. The value parameter carries the entity
// data at the current schema level for evaluating if/then/else conditions.
// It may be nil if no value context is available.
func resolveSchemaNodeWithValue(node map[string]any, root map[string]any, value map[string]any) map[string]any {
	return resolveSchemaNodeImpl(node, root, value, 0)
}

func resolveSchemaNodeImpl(node map[string]any, root map[string]any, value map[string]any, depth int) map[string]any {
	if node == nil || depth > 10 {
		return node
	}

	// Resolve $ref first, then recurse on the result.
	if ref, ok := node["$ref"].(string); ok {
		resolved := followRef(ref, root)
		if resolved == nil {
			return nil
		}
		merged := shallowMergeSchema(resolved, node)
		delete(merged, "$ref")
		return resolveSchemaNodeImpl(merged, root, value, depth+1)
	}

	// Resolve if/then/else using the entity value.
	if ifSchema, ok := node["if"].(map[string]any); ok && value != nil {
		base := make(map[string]any)
		for k, v := range node {
			if k != "if" && k != "then" && k != "else" {
				base[k] = v
			}
		}
		if evaluateSimpleCondition(ifSchema, value) {
			if thenSchema, ok := node["then"].(map[string]any); ok {
				base = shallowMergeSchema(base, thenSchema)
			}
		} else {
			if elseSchema, ok := node["else"].(map[string]any); ok {
				base = shallowMergeSchema(base, elseSchema)
			}
		}
		return resolveSchemaNodeImpl(base, root, value, depth+1)
	}

	// Resolve all composition keywords present on this node.
	composed := false
	merged := make(map[string]any)
	for k, v := range node {
		if k == "allOf" || k == "oneOf" || k == "anyOf" {
			continue
		}
		merged[k] = v
	}
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		branches, ok := node[keyword].([]any)
		if !ok {
			continue
		}
		composed = true
		for _, branch := range branches {
			branchMap, ok := branch.(map[string]any)
			if !ok {
				continue
			}
			r := resolveSchemaNodeImpl(branchMap, root, value, depth+1)
			if r == nil {
				r = branchMap
			}
			merged = shallowMergeSchema(merged, r)
		}
	}
	if composed {
		return merged
	}

	return node
}

// evaluateSimpleCondition checks whether an if-schema's property constraints
// match the current value. Supports properties with const and enum checks,
// which covers the vast majority of if/then/else usage in practice.
// Comparisons are type-aware: JSON numbers (float64), strings, and booleans
// are compared by Go type and value, matching the TypeScript evaluateCondition
// semantics where 1 !== "1".
func evaluateSimpleCondition(ifSchema map[string]any, value map[string]any) bool {
	props, ok := ifSchema["properties"].(map[string]any)
	if !ok {
		return false
	}
	for key, constraint := range props {
		constraintMap, ok := constraint.(map[string]any)
		if !ok {
			return false
		}
		actual, exists := value[key]
		if !exists {
			return false
		}
		// Check const — type-aware comparison via reflect.DeepEqual
		if constVal, ok := constraintMap["const"]; ok {
			if !jsonValuesEqual(actual, constVal) {
				return false
			}
		}
		// Check enum — actual must match at least one enum value
		if enumVal, ok := constraintMap["enum"].([]any); ok {
			found := false
			for _, e := range enumVal {
				if jsonValuesEqual(actual, e) {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}
	return true
}

// jsonValuesEqual compares two values from json.Unmarshal using Go type
// equality. This ensures "1" (string) != 1 (float64) != true (bool),
// matching JSON Schema and the TypeScript evaluateCondition semantics.
func jsonValuesEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Both values come from json.Unmarshal, so their types are limited to:
	// string, float64, bool, nil, map[string]any, []any.
	// Direct == works for string, float64, bool. For maps/slices we'd need
	// deep comparison, but const/enum values are almost always primitives.
	return a == b
}

// followRef resolves a local JSON pointer ref like "#/$defs/Address" or
// "#/definitions/Address" within the root schema.
func followRef(ref string, root map[string]any) map[string]any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	segments := strings.Split(ref[2:], "/")
	var current any = root
	for _, seg := range segments {
		// Unescape JSON Pointer encoding (~0 = ~, ~1 = /)
		seg = strings.ReplaceAll(seg, "~1", "/")
		seg = strings.ReplaceAll(seg, "~0", "~")
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[seg]
		if !ok {
			return nil
		}
	}
	result, ok := current.(map[string]any)
	if !ok {
		return nil
	}
	return result
}

// mergeSchemas deep-merges src into dst. "properties" maps are merged
// recursively so that overlapping property keys combine their children
// rather than one branch replacing the other.
func shallowMergeSchema(dst, src map[string]any) map[string]any {
	result := make(map[string]any, len(dst)+len(src))
	for k, v := range dst {
		result[k] = v
	}
	for k, v := range src {
		if k == "properties" {
			dstProps, _ := result["properties"].(map[string]any)
			srcProps, _ := v.(map[string]any)
			if dstProps == nil {
				result["properties"] = v
			} else if srcProps != nil {
				merged := make(map[string]any, len(dstProps)+len(srcProps))
				for pk, pv := range dstProps {
					merged[pk] = pv
				}
				for pk, pv := range srcProps {
					// If both sides define the same property as objects,
					// recursively merge so nested children from both survive.
					if existing, ok := merged[pk].(map[string]any); ok {
						if srcChild, ok := pv.(map[string]any); ok {
							merged[pk] = shallowMergeSchema(existing, srcChild)
							continue
						}
					}
					merged[pk] = pv
				}
				result["properties"] = merged
			}
		} else {
			result[k] = v
		}
	}
	return result
}
