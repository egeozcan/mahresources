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
	schemaSlice := extractSchemaSlice(ctx.MetaSchema, path)

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
// Handles $ref (local JSON pointer refs like #/$defs/Foo or #/definitions/Foo)
// and allOf (merges all branches) so that composed schemas resolve correctly.
func extractSchemaSlice(schemaStr string, path string) string {
	if schemaStr == "" {
		return ""
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(schemaStr), &root); err != nil {
		return ""
	}

	parts := strings.Split(path, ".")
	current := root

	for _, part := range parts {
		resolved := resolveSchemaNode(current, root)
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
	}

	// Resolve the leaf node too (it might be a $ref)
	resolved := resolveSchemaNode(current, root)
	if resolved == nil {
		resolved = current
	}

	encoded, err := json.Marshal(resolved)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// resolveSchemaNode recursively resolves $ref, allOf, oneOf, and anyOf on a
// schema node. For allOf all branches are merged (all constraints apply).
// For oneOf/anyOf all branches' properties are merged so that any property
// reachable through any branch can be found — we don't have data to pick the
// "right" branch at schema-extraction time, and merging is safe because we
// only need to locate property definitions for display/edit rendering.
// Recursion is capped at depth 10 to prevent infinite $ref loops.
func resolveSchemaNode(node map[string]any, root map[string]any) map[string]any {
	return resolveSchemaNodeDepth(node, root, 0)
}

func resolveSchemaNodeDepth(node map[string]any, root map[string]any, depth int) map[string]any {
	if node == nil || depth > 10 {
		return node
	}

	// Resolve $ref first, then recurse on the result
	if ref, ok := node["$ref"].(string); ok {
		resolved := followRef(ref, root)
		if resolved == nil {
			return nil
		}
		merged := shallowMergeSchema(resolved, node)
		delete(merged, "$ref")
		return resolveSchemaNodeDepth(merged, root, depth+1)
	}

	// Resolve all composition keywords present on this node.
	resolved := false
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
		resolved = true
		for _, branch := range branches {
			branchMap, ok := branch.(map[string]any)
			if !ok {
				continue
			}
			r := resolveSchemaNodeDepth(branchMap, root, depth+1)
			if r == nil {
				r = branchMap
			}
			merged = shallowMergeSchema(merged, r)
		}
	}
	if resolved {
		return merged
	}

	return node
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
