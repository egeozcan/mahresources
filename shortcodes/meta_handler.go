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
		`<meta-shortcode data-path="%s" data-editable="%t" data-hide-empty="%t" data-entity-type="%s" data-entity-id="%d" data-schema='%s' data-value='%s'></meta-shortcode>`,
		html.EscapeString(path),
		editable,
		hideEmpty,
		html.EscapeString(ctx.EntityType),
		ctx.EntityID,
		schemaSlice,
		valueJSON,
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
func extractSchemaSlice(schemaStr string, path string) string {
	if schemaStr == "" {
		return ""
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(schemaStr), &schema); err != nil {
		return ""
	}

	parts := strings.Split(path, ".")
	current := schema

	for _, part := range parts {
		props, ok := current["properties"].(map[string]any)
		if !ok {
			return ""
		}
		sub, ok := props[part].(map[string]any)
		if !ok {
			return ""
		}
		current = sub
	}

	encoded, err := json.Marshal(current)
	if err != nil {
		return ""
	}
	return string(encoded)
}
