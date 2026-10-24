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
	// FieldTypes is what each shape key must hold for the renderer's selector
	// to accept the value: "number", "string", or "date" (an ISO 8601 string).
	FieldTypes map[string]string
}

// builtinDisplayRenderers is src/schema-editor/display-renderers.ts
// builtinRenderers, in detection order (first match wins).
var builtinDisplayRenderers = []DisplayRenderer{
	{Name: "url", Shapes: [][]string{{"href", "host"}, {"href", "hostname"}},
		FieldTypes: map[string]string{"href": "string", "host": "string", "hostname": "string"}},
	{Name: "geo", Shapes: [][]string{{"latitude", "longitude"}, {"lat", "lng"}},
		FieldTypes: map[string]string{"latitude": "number", "longitude": "number", "lat": "number", "lng": "number"}},
	{Name: "daterange", Shapes: [][]string{{"start", "end"}},
		FieldTypes: map[string]string{"start": "date", "end": "date"}},
	{Name: "dimensions", Shapes: [][]string{{"width", "height"}},
		FieldTypes: map[string]string{"width": "number", "height": "number"}},
}

// BuiltinDisplayRenderers lists the built-in x-display renderers in detection
// order. The result is a copy.
func BuiltinDisplayRenderers() []DisplayRenderer {
	out := make([]DisplayRenderer, len(builtinDisplayRenderers))
	for i, r := range builtinDisplayRenderers {
		out[i] = DisplayRenderer{Name: r.Name, FieldTypes: map[string]string{}}
		for k, v := range r.FieldTypes {
			out[i].FieldTypes[k] = v
		}
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
// app understands. It is constant; the enabled plugins' x-display values are
// plugin-authored and travel separately, fenced.
func SchemaExtensionReference() string {
	var b strings.Builder
	b.WriteString("Schema extensions this app understands. Use only these; any other custom keyword is ignored.\n")
	b.WriteString("- \"x-display\" on a property chooses how the detail page draws its value. Built-in values, each for a property of type object that declares these keys in \"properties\" and lists them in \"required\":\n")
	for _, r := range builtinDisplayRenderers {
		fmt.Fprintf(&b, "  - %q: %s\n", r.Name, shapeAlternatives(r))
	}
	b.WriteString("  Types: href, host and hostname are strings; latitude, longitude, lat, lng, width and height are numbers; start and end are ISO 8601 date strings (YYYY-MM-DD or a date-time).\n")
	b.WriteString("  An object value with one of these shapes is drawn that way without x-display. " + quoteJoin(DisplayOptOutValues, " or ") + " opts out of that and shows a plain key/value grid. A value that lacks the shape is drawn as a plain object.\n")
	b.WriteString("  An x-display that is not a string is ignored. A value starting with plugin: names an enabled plugin's renderer; the request lists them when there are any.\n")
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
	compositionKeys     = map[string]bool{"allOf": true, "anyOf": true, "oneOf": true, "not": true, "if": true, "then": true, "else": true}
	// rendererDraws says what a matching value looks like, for the messages.
	rendererDraws = map[string]string{"url": "a link with its host", "geo": "a map link", "daterange": "a formatted date range", "dimensions": "a width \u00d7 height size"}
)

// LintMetaSchema advises on constructs in a MetaSchema that the app's editors
// and renderers handle differently from what the author probably meant. None
// of them breaks a page: a renderer that does not fit a value falls back to the
// plain display, and an annotation of the wrong type is ignored. Every issue is
// therefore a warning that says what will be drawn instead. The schema must
// already compile; text that is not a JSON document gives no issues, because
// the compile step reports that.
func LintMetaSchema(schema string, opts MetaSchemaLintOptions) []LintIssue {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var root any
	if err := json.Unmarshal([]byte(schema), &root); err != nil {
		return nil
	}
	l := &schemaLinter{opts: opts, root: root, refSiblings: refSiblingSemantics(root)}
	l.walk(root, "#", true)
	return l.issues
}

type schemaLinter struct {
	opts        MetaSchemaLintOptions
	root        any
	refSiblings refSiblingMode
	issues      []LintIssue
}

// refSiblingMode says whether keywords next to a $ref apply: not up to
// draft-07, yes from 2019-09, and unknown for any other or no $schema.
type refSiblingMode int

const (
	refSiblingsUnknown refSiblingMode = iota
	refSiblingsIgnored
	refSiblingsApply
)

func refSiblingSemantics(root any) refSiblingMode {
	obj, _ := root.(map[string]any)
	uri, _ := obj["$schema"].(string)
	for _, draft := range []string{"draft-04", "draft-06", "draft-07"} {
		if strings.Contains(uri, "json-schema.org/"+draft) || strings.Contains(uri, "json-schema.org/draft/"+draft) {
			return refSiblingsIgnored
		}
	}
	for _, draft := range []string{"2019-09", "2020-12"} {
		if strings.Contains(uri, "json-schema.org/draft/"+draft) {
			return refSiblingsApply
		}
	}
	return refSiblingsUnknown
}

// composesOthers reports whether a node's own composition can add to the
// schemas of its properties and items from outside them.
func composesOthers(obj map[string]any) bool {
	for _, k := range []string{"$ref", "allOf", "anyOf", "oneOf", "if", "then", "else", "not"} {
		if _, has := obj[k]; has {
			return true
		}
	}
	return false
}

func (l *schemaLinter) warn(format string, args ...any) {
	l.issues = append(l.issues, LintIssue{Severity: SeverityWarning, Message: fmt.Sprintf(format, args...)})
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// walk visits a schema node. local is true while the node's effective schema is
// decided by the path from the root alone: no enclosing composition, reference
// or definition can add to it.
func (l *schemaLinter) walk(node any, path string, local bool) {
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	if ref, ok := obj["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
		l.warn("%s: $ref %q is not local; use a \"#/$defs/name\" reference inside the document.", path, ref)
	}
	if v, present := obj["x-display"]; present {
		l.lintDisplay(obj, v, path, local)
	}
	l.lintLabeledEnum(obj, path)

	childLocal := local && !composesOthers(obj)
	if props, ok := obj["properties"].(map[string]any); ok {
		for _, key := range sortedKeys(props) {
			l.lintPropertyKey(key, path)
			l.walk(props[key], path+"/properties/"+key, childLocal)
		}
	}
	for _, name := range schemaSchemaMaps {
		if m, ok := obj[name].(map[string]any); ok {
			for _, key := range sortedKeys(m) {
				l.walk(m[key], path+"/"+name+"/"+key, false)
			}
		}
	}
	for _, name := range schemaSubschemaKeys {
		l.walk(obj[name], path+"/"+name, childLocal && !compositionKeys[name])
	}
	for _, name := range schemaSchemaLists {
		if list, ok := obj[name].([]any); ok {
			for i, entry := range list {
				l.walk(entry, path+"/"+name+"/"+strconv.Itoa(i), childLocal && !compositionKeys[name])
			}
		}
	}
}

func (l *schemaLinter) lintPropertyKey(key, path string) {
	if metaPropertyKey.MatchString(key) {
		return
	}
	if strings.Contains(key, ".") {
		l.warn("%s: property name %q contains a dot, which meta paths read as nesting; use letters, digits and underscores only.", path, key)
		return
	}
	l.warn("%s: property name %q should use letters, digits and underscores only; other characters break meta paths and MRQL field names.", path, key)
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
		l.warn("%s: this oneOf looks like a labeled enum, but entry %d has %q, so the app treats it as a plain subschema and the titles are lost; keep only const, title, x-color and description in each entry.", path, complexAt, complexKey)
	}
}

func (l *schemaLinter) lintDisplay(obj map[string]any, value any, path string, local bool) {
	name, ok := value.(string)
	if !ok {
		l.warn("%s: \"x-display\" must be a string, not %s; it is ignored and the value is drawn by shape or by its type.", path, jsonTypeName(value))
		return
	}
	for _, v := range DisplayOptOutValues {
		if name == v {
			return
		}
	}
	for _, r := range builtinDisplayRenderers {
		if name == r.Name {
			if local {
				l.lintBuiltinDisplay(obj, r, path)
			}
			return
		}
	}
	if strings.HasPrefix(name, "plugin:") {
		switch {
		case !pluginDisplayValue.MatchString(name):
			l.warn("%s: x-display %q is not of the form plugin:<plugin>:<type>; the value is drawn without it.", path, name)
		case !l.opts.DisplayTypesTruncated && !containsString(l.opts.DisplayTypes, name):
			l.warn("%s: no enabled plugin provides x-display %q, so the value is drawn without it.", path, name)
		}
		return
	}
	l.warn("%s: unknown x-display %q suppresses shape detection and applies no renderer, so an object value is drawn as a plain object; use %s, %s or an enabled plugin value.", path, name, builtinNames(), quoteJoin(DisplayOptOutValues, " or "))
}

func builtinNames() string {
	names := make([]string, len(builtinDisplayRenderers))
	for i, r := range builtinDisplayRenderers {
		names[i] = strconv.Quote(r.Name)
	}
	return strings.Join(names, ", ")
}

func (l *schemaLinter) lintBuiltinDisplay(obj map[string]any, r DisplayRenderer, path string) {
	eff, ok := l.effective(obj, 0, map[string]bool{})
	if !ok {
		return // anyOf/oneOf alternatives or an unresolvable $ref: do not guess
	}
	draws := rendererDraws[r.Name]
	for _, types := range eff.typeLists {
		if !containsString(types, "object") {
			l.warn("%s: x-display %q is for an object property, but its type is %s, so its value is drawn as that type instead of %s.", path, r.Name, strings.Join(types, " or "), draws)
			return
		}
	}
	if len(eff.props) == 0 {
		return
	}
	var declared, usable [][]string
	for _, shape := range r.Shapes {
		if allIn(shape, func(k string) bool { _, ok := eff.props[k]; return ok }) {
			declared = append(declared, shape)
		}
	}
	if len(declared) == 0 {
		l.warn("%s: x-display %q draws only an object with %s, but that property does not declare them, so its value is drawn as a plain object instead of %s.", path, r.Name, shapeAlternatives(r), draws)
		return
	}
	for _, shape := range declared {
		if allIn(shape, func(k string) bool { return l.fieldTypeFits(eff, k, r.FieldTypes[k]) }) {
			usable = append(usable, shape)
		}
	}
	if len(usable) == 0 {
		l.warn("%s: x-display %q needs %s typed %s, but they are declared with other types, so no value matches and it is drawn as a plain object instead of %s.", path, r.Name, shapeText(declared[0]), fieldTypeText(r.FieldTypes[declared[0][0]]), draws)
		return
	}
	for _, shape := range usable {
		if allIn(shape, func(k string) bool { return eff.required[k] }) {
			return
		}
	}
	var missing []string
	for _, k := range usable[0] {
		if !eff.required[k] {
			missing = append(missing, k)
		}
	}
	l.warn("%s: x-display %q draws only when %s are present; add %s to \"required\", or a value without them is drawn as a plain object instead of %s.", path, r.Name, shapeText(usable[0]), strings.Join(missing, " and "), draws)
}

func fieldTypeText(kind string) string {
	switch kind {
	case "number":
		return "number"
	case "date":
		return "string (an ISO 8601 date)"
	}
	return "string"
}

// fieldTypeFits reports whether the property's declared type can hold the kind
// of value the renderer's selector accepts. An undeclared or unresolvable type
// fits: the check refuses only what is certainly unusable.
func (l *schemaLinter) fieldTypeFits(eff *effectiveSchema, key, kind string) bool {
	acceptable := []string{"string"}
	if kind == "number" {
		acceptable = []string{"number", "integer"}
	}
	for _, sub := range eff.props[key] {
		e, ok := l.effective(sub, 0, map[string]bool{})
		if !ok {
			return true
		}
		for _, types := range e.typeLists {
			fits := false
			for _, t := range types {
				if containsString(acceptable, t) {
					fits = true
				}
			}
			if !fits {
				return false
			}
		}
	}
	return true
}

// effectiveSchema is what the editors' resolveSchema makes of a node for the
// facts the lint needs: its allOf entries and local $ref targets merged in.
type effectiveSchema struct {
	typeLists [][]string // one list per declaration; the value must satisfy each
	props     map[string][]any
	required  map[string]bool
}

// effective resolves obj through local $ref and allOf. It reports false when
// the answer is not one schema: anyOf/oneOf alternatives, a ref that is not
// local or does not resolve, a cycle, or too deep a chain.
func (l *schemaLinter) effective(node any, depth int, active map[string]bool) (*effectiveSchema, bool) {
	obj, ok := node.(map[string]any)
	if !ok || depth > 12 {
		return nil, false
	}
	if _, has := obj["anyOf"]; has {
		return nil, false
	}
	if _, has := obj["oneOf"]; has {
		return nil, false
	}
	eff := &effectiveSchema{props: map[string][]any{}, required: map[string]bool{}}
	merge := func(sub *effectiveSchema) {
		eff.typeLists = append(eff.typeLists, sub.typeLists...)
		for k, v := range sub.props {
			eff.props[k] = append(eff.props[k], v...)
		}
		for k := range sub.required {
			eff.required[k] = true
		}
	}
	if ref, has := obj["$ref"]; has {
		pointer, isString := ref.(string)
		if !isString || active[pointer] {
			return nil, false
		}
		target, ok := localRef(l.root, pointer)
		if !ok {
			return nil, false
		}
		active[pointer] = true
		sub, ok := l.effective(target, depth+1, active)
		delete(active, pointer)
		if !ok {
			return nil, false
		}
		merge(sub)
		if hasAnyKey(obj, "type", "properties", "required", "allOf") {
			switch l.refSiblings {
			case refSiblingsIgnored:
				return eff, true // up to draft-07 the siblings of $ref do not apply
			case refSiblingsUnknown:
				return nil, false
			}
		}
	}
	if list, has := obj["allOf"].([]any); has {
		for _, entry := range list {
			sub, ok := l.effective(entry, depth+1, active)
			if !ok {
				return nil, false
			}
			merge(sub)
		}
	}
	switch t := obj["type"].(type) {
	case string:
		eff.typeLists = append(eff.typeLists, []string{t})
	case []any:
		var types []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				types = append(types, s)
			}
		}
		if len(types) > 0 {
			eff.typeLists = append(eff.typeLists, types)
		}
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		for k, v := range props {
			eff.props[k] = append(eff.props[k], v)
		}
	}
	if req, ok := obj["required"].([]any); ok {
		for _, k := range req {
			if s, ok := k.(string); ok {
				eff.required[s] = true
			}
		}
	}
	return eff, true
}

// localRef follows a "#" or "#/a/b" JSON pointer inside the document.
func localRef(root any, ref string) (any, bool) {
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	node := root
	for _, part := range strings.Split(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch n := node.(type) {
		case map[string]any:
			child, ok := n[part]
			if !ok {
				return nil, false
			}
			node = child
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(n) {
				return nil, false
			}
			node = n[i]
		default:
			return nil, false
		}
	}
	return node, true
}

func hasAnyKey(obj map[string]any, keys ...string) bool {
	for _, k := range keys {
		if _, has := obj[k]; has {
			return true
		}
	}
	return false
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
