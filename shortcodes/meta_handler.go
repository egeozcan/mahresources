package shortcodes

import (
	"encoding/json"
	"fmt"
	"html"
	"math"
	"reflect"
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
	// rawValue tracks the value at the current path as any (including primitives)
	var rawValue any = metaValue

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
		// Descend into the value for next-level condition evaluation
		if currentValue != nil {
			rawValue = currentValue[part]
			if nested, ok := currentValue[part].(map[string]any); ok {
				currentValue = nested
			} else {
				currentValue = nil
			}
		} else {
			rawValue = nil
		}
	}

	// Resolve the leaf node for $ref and if/then/else only.
	// Do NOT resolve oneOf/anyOf/allOf on the leaf — the client needs those
	// intact for labeled-enum detection and display rendering.
	leafValue := currentValue
	if leafValue == nil && rawValue != nil {
		leafValue = map[string]any{"_self": rawValue}
	}
	resolved := resolveLeafSchema(current, root, leafValue)
	if resolved == nil {
		resolved = current
	}

	encoded, err := json.Marshal(resolved)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// resolveLeafSchema resolves only $ref and if/then/else on a leaf schema node,
// preserving oneOf/anyOf/allOf so the client can detect labeled enums and other
// composition-based display patterns.
func resolveLeafSchema(node map[string]any, root map[string]any, value map[string]any) map[string]any {
	if node == nil {
		return nil
	}
	// Resolve $ref
	if ref, ok := node["$ref"].(string); ok {
		resolved := followRef(ref, root)
		if resolved == nil {
			return nil
		}
		merged := shallowMergeSchema(resolved, node)
		delete(merged, "$ref")
		return resolveLeafSchema(merged, root, value)
	}
	// Resolve $ref inside composition branches so the client doesn't need
	// the root $defs. The branches themselves are preserved (not merged).
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		branches, ok := node[keyword].([]any)
		if !ok {
			continue
		}
		changed := false
		resolved := make([]any, len(branches))
		for i, branch := range branches {
			if branchMap, ok := branch.(map[string]any); ok {
				if ref, ok := branchMap["$ref"].(string); ok {
					if refTarget := followRef(ref, root); refTarget != nil {
						merged := shallowMergeSchema(refTarget, branchMap)
						delete(merged, "$ref")
						resolved[i] = merged
						changed = true
						continue
					}
				}
			}
			resolved[i] = branch
		}
		if changed {
			// Copy node to avoid mutating the original schema tree
			copy := make(map[string]any, len(node))
			for k, v := range node {
				copy[k] = v
			}
			copy[keyword] = resolved
			node = copy
		}
	}

	// Resolve if/then/else
	if _, ok := node["if"].(map[string]any); ok {
		base := make(map[string]any)
		for k, v := range node {
			if k != "if" && k != "then" && k != "else" {
				base[k] = v
			}
		}
		matched, supported := tryEvaluateCondition(node["if"].(map[string]any), value)
		if supported {
			if matched {
				if thenSchema, ok := node["then"].(map[string]any); ok {
					base = shallowMergeSchema(base, thenSchema)
				}
			} else {
				if elseSchema, ok := node["else"].(map[string]any); ok {
					base = shallowMergeSchema(base, elseSchema)
				}
			}
		} else {
			if thenSchema, ok := node["then"].(map[string]any); ok {
				base = shallowMergeSchema(base, thenSchema)
			}
			if elseSchema, ok := node["else"].(map[string]any); ok {
				base = shallowMergeSchema(base, elseSchema)
			}
		}
		// Re-resolve in case the merged branch introduced $ref or composition.
		return resolveLeafSchema(base, root, value)
	}
	return node
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
	if _, ok := node["if"].(map[string]any); ok {
		base := make(map[string]any)
		for k, v := range node {
			if k != "if" && k != "then" && k != "else" {
				base[k] = v
			}
		}
		matched, supported := tryEvaluateCondition(node["if"].(map[string]any), value)
		if supported {
			// Condition was evaluable — pick the active branch.
			if matched {
				if thenSchema, ok := node["then"].(map[string]any); ok {
					base = shallowMergeSchema(base, thenSchema)
				}
			} else {
				if elseSchema, ok := node["else"].(map[string]any); ok {
					base = shallowMergeSchema(base, elseSchema)
				}
			}
		} else {
			// Condition uses unsupported features — merge both branches
			// so all possible properties are discoverable.
			if thenSchema, ok := node["then"].(map[string]any); ok {
				base = shallowMergeSchema(base, thenSchema)
			}
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

// supportedIfKeys lists top-level keys in an if-schema that this evaluator
// understands. Any other key (required, $ref, allOf, not, etc.) triggers
// the unsupported fallback.
var supportedIfKeys = map[string]bool{
	"properties": true,
	"const":      true,
	"enum":       true,
	"type":       true, // ignored for evaluation but not unsupported
}

// tryEvaluateCondition attempts to evaluate an if-schema condition against
// the current value. Returns (matched, true) if the condition was evaluable,
// or (false, false) if it uses unsupported features (in which case the caller
// should merge both branches as a safe fallback).
//
// Supported: direct const/enum on the value, or properties with const/enum
// checks. Unsupported: required, minimum/maximum, pattern, $ref, allOf, etc.
func tryEvaluateCondition(ifSchema map[string]any, value map[string]any) (matched bool, supported bool) {
	if value == nil {
		return false, false
	}

	// Check for unsupported top-level keys — if any are present alongside
	// supported ones, we can't trust our partial evaluation.
	for k := range ifSchema {
		if !supportedIfKeys[k] {
			return false, false
		}
	}

	// Top-level type check: if: {type: "string"} or {type: ["string","null"]}.
	if typeRaw, hasType := ifSchema["type"]; hasType {
		types := normalizeTypeList(typeRaw)
		if len(types) > 0 {
			selfVal, hasSelf := value["_self"]
			if hasSelf {
				if !jsonValueMatchesAnyType(selfVal, types) {
					return false, true
				}
			} else {
				if !jsonValueMatchesAnyType(value, types) {
					return false, true
				}
			}
		}
	}

	// Direct const check (leaf-level conditional: if: {const: "draft"}).
	// For leaf values, the caller wraps the primitive as {"_self": value}.
	if constVal, ok := ifSchema["const"]; ok {
		selfVal, hasSelf := value["_self"]
		if hasSelf {
			return reflect.DeepEqual(selfVal, constVal), true
		}
		// const at object level — not standard, unsupported
		return false, false
	}

	// Direct enum check (leaf-level conditional).
	if enumVal, ok := ifSchema["enum"].([]any); ok {
		selfVal, hasSelf := value["_self"]
		if hasSelf {
			for _, e := range enumVal {
				if reflect.DeepEqual(selfVal, e) {
					return true, true
				}
			}
			return false, true
		}
		return false, false
	}

	// Properties-based check.
	props, ok := ifSchema["properties"].(map[string]any)
	if !ok {
		return false, false
	}
	for key, constraint := range props {
		constraintMap, ok := constraint.(map[string]any)
		if !ok {
			return false, false
		}

		// Check for unsupported property-level keywords. If anything
		// beyond const/enum/type is present, we can't trust partial eval.
		for ck := range constraintMap {
			if ck != "const" && ck != "enum" && ck != "type" {
				return false, false
			}
		}

		actual, exists := value[key]
		if !exists {
			// Per JSON Schema, absent properties match vacuously —
			// only "required" (a top-level keyword, not in properties)
			// makes absence a failure, and we already reject required
			// via the supportedIfKeys whitelist.
			continue
		}
		if constVal, hasConst := constraintMap["const"]; hasConst {
			if !reflect.DeepEqual(actual, constVal) {
				return false, true
			}
		}
		if enumVal, hasEnum := constraintMap["enum"].([]any); hasEnum {
			found := false
			for _, e := range enumVal {
				if reflect.DeepEqual(actual, e) {
					found = true
					break
				}
			}
			if !found {
				return false, true
			}
		}
		if typeRaw, hasType := constraintMap["type"]; hasType {
			types := normalizeTypeList(typeRaw)
			if len(types) > 0 && !jsonValueMatchesAnyType(actual, types) {
				return false, true
			}
		}
	}
	return true, true
}

// jsonValueMatchesType checks whether a json.Unmarshal value matches a JSON
// Schema type keyword. Values from json.Unmarshal are: string, float64, bool,
// nil, map[string]any, []any.
func jsonValueMatchesType(v any, schemaType string) bool {
	switch schemaType {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := v.(float64)
		return ok
	case "integer":
		f, ok := v.(float64)
		return ok && f == math.Trunc(f)
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "null":
		return v == nil
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "array":
		_, ok := v.([]any)
		return ok
	}
	return false
}

// normalizeTypeList converts a JSON Schema type value (string or []any)
// into a []string of type names.
func normalizeTypeList(typeRaw any) []string {
	if s, ok := typeRaw.(string); ok {
		return []string{s}
	}
	if arr, ok := typeRaw.([]any); ok {
		types := make([]string, 0, len(arr))
		for _, v := range arr {
			if s, ok := v.(string); ok {
				types = append(types, s)
			}
		}
		return types
	}
	return nil
}

// jsonValueMatchesAnyType returns true if v matches any of the given types.
func jsonValueMatchesAnyType(v any, types []string) bool {
	for _, t := range types {
		if jsonValueMatchesType(v, t) {
			return true
		}
	}
	return false
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
