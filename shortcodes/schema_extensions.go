package shortcodes

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// This file is the Go source of truth for the JSON Schema extensions the app's
// editors and renderers understand. The facts are copied from the TypeScript
// that implements them; schema_extensions_drift_test.go fails when a copy and
// its source disagree.

// DisplayRenderer is one built-in x-display renderer. Shapes are the
// alternative key sets a value must carry for the renderer to draw it, tried
// in order. A schema must declare every key of at least one set for a forced
// renderer to have anything to draw.
type DisplayRenderer struct {
	Name   string
	Shapes [][]string
}

// builtinDisplayRenderers is src/schema-editor/display-renderers.ts
// builtinRenderers, in detection order (first match wins).
var builtinDisplayRenderers = []DisplayRenderer{
	{Name: "url", Shapes: [][]string{{"href", "host"}, {"href", "hostname"}}},
	{Name: "geo", Shapes: [][]string{{"latitude", "longitude"}, {"lat", "lng"}}},
	{Name: "daterange", Shapes: [][]string{{"start", "end"}}},
	{Name: "dimensions", Shapes: [][]string{{"width", "height"}}},
}

// BuiltinDisplayRenderers lists the built-in x-display renderers in detection
// order. The result is a copy.
func BuiltinDisplayRenderers() []DisplayRenderer {
	out := make([]DisplayRenderer, len(builtinDisplayRenderers))
	for i, r := range builtinDisplayRenderers {
		out[i] = DisplayRenderer{Name: r.Name}
		for _, shape := range r.Shapes {
			out[i].Shapes = append(out[i].Shapes, append([]string(nil), shape...))
		}
	}
	return out
}

// DisplayOptOutValues are the x-display values that skip shape detection
// (display-mode.ts and meta-shortcode.ts both honour them).
var DisplayOptOutValues = []string{"raw", "none"}

// LabeledEnumComplexKeys are the keys that stop a oneOf entry from counting as
// a labeled-enum entry (schema-core.ts isLabeledEnum).
var LabeledEnumComplexKeys = []string{"type", "properties", "items", "oneOf", "anyOf", "allOf", "if", "$ref", "enum"}

// schemaFormatsWithUI are the string formats the form gives their own input
// (form-mode.ts) and the display panel its own rendering (display-mode.ts).
var schemaFormatsWithUI = []string{"date", "date-time", "email", "uri", "url"}

// SchemaFormatsWithUI lists the string formats with their own control.
func SchemaFormatsWithUI() []string { return append([]string(nil), schemaFormatsWithUI...) }

func shapeText(shape []string) string { return strings.Join(shape, " and ") }

// shapeAlternatives renders a renderer's shapes as "a and b (or c and d)".
func shapeAlternatives(r DisplayRenderer) string {
	text := shapeText(r.Shapes[0])
	if len(r.Shapes) > 1 {
		rest := make([]string, 0, len(r.Shapes)-1)
		for _, shape := range r.Shapes[1:] {
			rest = append(rest, shapeText(shape))
		}
		text += " (or " + strings.Join(rest, ", or ") + ")"
	}
	return text
}

// SchemaExtensionReference is the prompt block describing the extensions the
// app understands. displayTypes are the enabled plugins' x-display values; the
// block is constant when there are none.
func SchemaExtensionReference(displayTypes []string) string {
	var b strings.Builder
	b.WriteString("Schema extensions this app understands. Use only these; any other custom keyword is ignored.\n")
	b.WriteString("- \"x-display\" on a property chooses how the detail page draws its value. Built-in values, each for a property of type object that declares these keys in \"properties\" and lists them in \"required\":\n")
	for _, r := range builtinDisplayRenderers {
		fmt.Fprintf(&b, "  - %q: %s\n", r.Name, shapeAlternatives(r))
	}
	b.WriteString("  Types: href, host and hostname are strings; latitude, longitude, lat, lng, width and height are numbers; start and end are ISO 8601 date strings (YYYY-MM-DD or a date-time).\n")
	b.WriteString("  An object value with one of these shapes is drawn that way without x-display. " + quoteJoin(DisplayOptOutValues, " or ") + " opts out of that and shows a plain key/value grid. A value that lacks the shape is drawn as a plain object.\n")
	if len(displayTypes) > 0 {
		b.WriteString("  Enabled plugin renderers, usable as the x-display value on any property: " + strings.Join(displayTypes, ", ") + ". Use no other plugin: value.\n")
	}
	b.WriteString("- \"format\" with its own input and display: " + strings.Join(schemaFormatsWithUI, ", ") + ". Other formats validate only.\n")
	b.WriteString("- Labeled enum: \"oneOf\" whose every entry is {\"const\": value, \"title\": label} with an optional \"x-color\" (a hex color) and \"description\". An entry that also has any of " + strings.Join(LabeledEnumComplexKeys, ", ") + " is a plain subschema and loses its labels. Use \"enum\" for choices without labels.\n")
	b.WriteString("- Property names are meta paths and MRQL fields: use letters, digits and underscores only (a dot means nesting).\n")
	b.WriteString("- \"$ref\" may point only inside the document (\"#/$defs/name\").\n")
	return b.String()
}

func quoteJoin(values []string, sep string) string {
	quoted := make([]string, len(values))
	for i, v := range values {
		quoted[i] = strconv.Quote(v)
	}
	return strings.Join(quoted, sep)
}

// MetaSchemaLintOptions carries the per-request facts the lint needs.
type MetaSchemaLintOptions struct {
	// DisplayTypes are the enabled plugins' x-display values.
	DisplayTypes []string
	// DisplayTypesTruncated is true when DisplayTypes is a cut-off list, so
	// an unlisted plugin value may still exist and is not reported.
	DisplayTypesTruncated bool
}

var (
	metaPropertyKey     = regexp.MustCompile(`^[A-Za-z0-9_]+$`)
	pluginDisplayValue  = regexp.MustCompile(`^plugin:[^:\s]+:[a-z][a-z0-9-]{0,49}$`)
	schemaSubschemaKeys = []string{"not", "if", "then", "else", "contains", "propertyNames", "additionalProperties", "unevaluatedProperties", "unevaluatedItems", "items", "additionalItems"}
	schemaSchemaLists   = []string{"allOf", "anyOf", "oneOf", "prefixItems", "items"}
	schemaSchemaMaps    = []string{"$defs", "definitions", "patternProperties", "dependentSchemas"}
)

// LintMetaSchema reports constructs in a MetaSchema that the app's editors and
// renderers mishandle. The schema must already compile; text that is not a JSON
// document gives no issues, because the compile step reports that.
func LintMetaSchema(schema string, opts MetaSchemaLintOptions) []LintIssue {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var root any
	if err := json.Unmarshal([]byte(schema), &root); err != nil {
		return nil
	}
	l := &schemaLinter{opts: opts}
	l.walk(root, "#")
	return l.issues
}

type schemaLinter struct {
	opts   MetaSchemaLintOptions
	issues []LintIssue
}

func (l *schemaLinter) add(severity, format string, args ...any) {
	l.issues = append(l.issues, LintIssue{Severity: severity, Message: fmt.Sprintf(format, args...)})
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (l *schemaLinter) walk(node any, path string) {
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	if ref, ok := obj["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
		l.add(SeverityWarning, "%s: $ref %q is not local; use a \"#/$defs/name\" reference inside the document.", path, ref)
	}
	if v, present := obj["x-display"]; present {
		l.lintDisplay(obj, v, path)
	}
	l.lintLabeledEnum(obj, path)

	if props, ok := obj["properties"].(map[string]any); ok {
		for _, key := range sortedKeys(props) {
			l.lintPropertyKey(key, path)
			l.walk(props[key], path+"/properties/"+key)
		}
	}
	for _, name := range schemaSchemaMaps {
		if m, ok := obj[name].(map[string]any); ok {
			for _, key := range sortedKeys(m) {
				l.walk(m[key], path+"/"+name+"/"+key)
			}
		}
	}
	for _, name := range schemaSubschemaKeys {
		l.walk(obj[name], path+"/"+name)
	}
	for _, name := range schemaSchemaLists {
		if list, ok := obj[name].([]any); ok {
			for i, entry := range list {
				l.walk(entry, path+"/"+name+"/"+strconv.Itoa(i))
			}
		}
	}
}

func (l *schemaLinter) lintPropertyKey(key, path string) {
	if metaPropertyKey.MatchString(key) {
		return
	}
	if strings.Contains(key, ".") {
		l.add(SeverityWarning, "%s: property name %q contains a dot, which meta paths read as nesting; use letters, digits and underscores only.", path, key)
		return
	}
	l.add(SeverityWarning, "%s: property name %q should use letters, digits and underscores only; other characters break meta paths and MRQL field names.", path, key)
}

func (l *schemaLinter) lintLabeledEnum(obj map[string]any, path string) {
	entries, ok := obj["oneOf"].([]any)
	if !ok || len(entries) == 0 {
		return
	}
	complexAt, complexKey := -1, ""
	for i, entry := range entries {
		e, ok := entry.(map[string]any)
		if !ok {
			return
		}
		if _, hasConst := e["const"]; !hasConst {
			return
		}
		if complexAt < 0 {
			for _, k := range LabeledEnumComplexKeys {
				if _, has := e[k]; has {
					complexAt, complexKey = i, k
					break
				}
			}
		}
	}
	if complexAt >= 0 {
		l.add(SeverityWarning, "%s: this oneOf looks like a labeled enum, but entry %d has %q, so the app treats it as a plain subschema and the titles are lost; keep only const, title, x-color and description in each entry.", path, complexAt, complexKey)
	}
}

func (l *schemaLinter) lintDisplay(obj map[string]any, value any, path string) {
	name, ok := value.(string)
	if !ok {
		l.add(SeverityError, "%s: \"x-display\" must be a string, not %s.", path, jsonTypeName(value))
		return
	}
	for _, v := range DisplayOptOutValues {
		if name == v {
			return
		}
	}
	for _, r := range builtinDisplayRenderers {
		if name == r.Name {
			l.lintBuiltinDisplay(obj, r, path)
			return
		}
	}
	if strings.HasPrefix(name, "plugin:") {
		switch {
		case !pluginDisplayValue.MatchString(name):
			l.add(SeverityWarning, "%s: x-display %q is not of the form plugin:<plugin>:<type>.", path, name)
		case !l.opts.DisplayTypesTruncated && !containsString(l.opts.DisplayTypes, name):
			l.add(SeverityWarning, "%s: no enabled plugin provides x-display %q, so the value is drawn without it.", path, name)
		}
		return
	}
	l.add(SeverityWarning, "%s: unknown x-display %q; use %s, %s or an enabled plugin value.", path, name, builtinNames(), quoteJoin(DisplayOptOutValues, " or "))
}

func builtinNames() string {
	names := make([]string, len(builtinDisplayRenderers))
	for i, r := range builtinDisplayRenderers {
		names[i] = strconv.Quote(r.Name)
	}
	return strings.Join(names, ", ")
}

func (l *schemaLinter) lintBuiltinDisplay(obj map[string]any, r DisplayRenderer, path string) {
	if t, declared := obj["type"]; declared && !typeIncludes(t, "object") {
		l.add(SeverityError, "%s: x-display %q needs an object property, but its type is %s; a forced renderer throws on any other value.", path, r.Name, typeText(t))
		return
	}
	props, hasProps := obj["properties"].(map[string]any)
	if !hasProps {
		return
	}
	var declared [][]string
	for _, shape := range r.Shapes {
		if allIn(shape, func(k string) bool { _, ok := props[k]; return ok }) {
			declared = append(declared, shape)
		}
	}
	if len(declared) == 0 {
		l.add(SeverityError, "%s: x-display %q needs properties %s declared under \"properties\"; a forced renderer throws on a value without them.", path, r.Name, shapeAlternatives(r))
		return
	}
	required := map[string]bool{}
	if list, ok := obj["required"].([]any); ok {
		for _, k := range list {
			if s, ok := k.(string); ok {
				required[s] = true
			}
		}
	}
	for _, shape := range declared {
		if allIn(shape, func(k string) bool { return required[k] }) {
			return
		}
	}
	var missing []string
	for _, k := range declared[0] {
		if !required[k] {
			missing = append(missing, k)
		}
	}
	l.add(SeverityWarning, "%s: x-display %q draws only when %s are present; add %s to \"required\", or a value without them is drawn as a plain object.", path, r.Name, shapeText(declared[0]), strings.Join(missing, " and "))
}

func allIn(keys []string, ok func(string) bool) bool {
	for _, k := range keys {
		if !ok(k) {
			return false
		}
	}
	return true
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func typeIncludes(t any, want string) bool {
	switch v := t.(type) {
	case string:
		return v == want
	case []any:
		for _, e := range v {
			if s, ok := e.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func typeText(t any) string {
	b, _ := json.Marshal(t)
	return string(b)
}

func jsonTypeName(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "a boolean"
	case float64:
		return "a number"
	case []any:
		return "an array"
	case map[string]any:
		return "an object"
	}
	return "another type"
}
