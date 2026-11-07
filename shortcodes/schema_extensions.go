package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mahresources/mrql"
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
	// Context, when set, cancels the lint. A lint that is cancelled, or whose
	// schema expands past the work budget, reports one error issue instead of
	// its warnings.
	Context context.Context
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
	if len(schema) > MaxLintSourceBytes {
		return lintTooLarge("schema")
	}
	var root any
	if err := json.Unmarshal([]byte(schema), &root); err != nil {
		return nil
	}
	l := &schemaLinter{opts: opts, root: root, refSiblings: refSiblingSemantics(root), w: newSchemaWork(opts.Context)}
	if l.w.poll() {
		l.walk(root, "#", true, 0)
	}
	if !l.w.poll() {
		return []LintIssue{{Severity: SeverityError, Message: "The schema is too complex to check: it expands past the lint's work or size allowance, or was cancelled. Simplify the $ref and composition structure."}}
	}
	return l.issues
}

// maxLintWalkDepth bounds the recursion of the ordinary tree walk. Real schemas
// nest a few levels; the cap only stops a document built to exhaust the stack.
const maxLintWalkDepth = 128

type schemaLinter struct {
	opts        MetaSchemaLintOptions
	root        any
	refSiblings refSiblingMode
	issues      []LintIssue
	w           *schemaWork
	displaySet  map[string]bool // opts.DisplayTypes as a set, built once
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
	// Reserve the message's bytes before formatting it: its arguments carry
	// schema-controlled names and paths, and %q can lengthen them.
	n := len(format)
	for _, a := range args {
		if str, ok := a.(string); ok {
			n += 4 * len(str)
		} else {
			n += 16
		}
	}
	if !l.w.chargeBytes(n) {
		return
	}
	l.issues = append(l.issues, LintIssue{Severity: SeverityWarning, Message: fmt.Sprintf(format, args...)})
}

// childPath builds path+sep+key after reserving its bytes, so a schema-controlled
// name cannot make the lint allocate paths past the allowance. ok is false once
// the work is spent.
func (l *schemaLinter) childPath(path, sep, key string) (string, bool) {
	if !l.w.chargeBytes(len(path) + len(sep) + len(key)) {
		return "", false
	}
	return path + sep + key, true
}

// sortedKeys returns m's keys in order, or nil once the work is spent. The
// count is charged before the slice is allocated.
func (l *schemaLinter) sortedKeys(m map[string]any) []string {
	if !l.w.charge(len(m)) {
		return nil
	}
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
func (l *schemaLinter) walk(node any, path string, local bool, depth int) {
	obj, ok := node.(map[string]any)
	if !ok {
		return
	}
	if depth > maxLintWalkDepth {
		l.w.fail()
		return
	}
	if !l.w.charge(1 + len(obj)) {
		return
	}
	if ref, ok := obj["$ref"].(string); ok && !strings.HasPrefix(ref, "#") {
		l.warn("%s: $ref %q is not local; use a \"#/$defs/name\" reference inside the document.", path, ref)
	}
	// An empty string is unset for both consumers, which then auto-detect.
	if v, present := obj["x-display"]; present && v != "" {
		l.lintDisplay(obj, v, path, local)
	}
	l.lintLabeledEnum(obj, path)

	childLocal := local && !composesOthers(obj) && onlyModelledKeywords(l.w, obj)
	if props, ok := obj["properties"].(map[string]any); ok {
		for _, key := range l.sortedKeys(props) {
			child, ok := l.childPath(path, "/properties/", key)
			if !ok {
				return
			}
			l.lintPropertyKey(key, path)
			l.walk(props[key], child, childLocal, depth+1)
			if !l.w.ok() {
				return
			}
		}
	}
	for _, name := range schemaSchemaMaps {
		if m, ok := obj[name].(map[string]any); ok {
			for _, key := range l.sortedKeys(m) {
				child, ok := l.childPath(path, "/"+name+"/", key)
				if !ok {
					return
				}
				l.walk(m[key], child, false, depth+1)
				if !l.w.ok() {
					return
				}
			}
		}
	}
	for _, name := range schemaSubschemaKeys {
		if _, present := obj[name]; !present {
			continue // no child, no path
		}
		child, ok := l.childPath(path, "/", name)
		if !ok {
			return
		}
		l.walk(obj[name], child, childLocal && !compositionKeys[name], depth+1)
		if !l.w.ok() {
			return
		}
	}
	for _, name := range schemaSchemaLists {
		if list, ok := obj[name].([]any); ok {
			if !l.w.charge(len(list)) {
				return
			}
			for i, entry := range list {
				child, ok := l.childPath(path, "/"+name+"/", strconv.Itoa(i))
				if !ok {
					return
				}
				l.walk(entry, child, childLocal && !compositionKeys[name], depth+1)
				if !l.w.ok() {
					return
				}
			}
		}
	}
}

// metaKeyAddressable reports whether MRQL can address the key as meta.<key>:
// the parser reads "meta.<key> = 1" as a comparison on exactly that field. A
// name that starts with a digit, is a reserved word (and, or, not, limit, ...)
// or has other characters does not.
func metaKeyAddressable(key string) bool {
	q, err := mrql.Parse("meta." + key + " = 1")
	if err != nil {
		return false
	}
	cmp, ok := q.Where.(*mrql.ComparisonExpr)
	return ok && len(cmp.Field.Parts) == 2 && cmp.Field.Parts[1].Value == key
}

func (l *schemaLinter) lintPropertyKey(key, path string) {
	if metaPropertyKey.MatchString(key) && metaKeyAddressable(key) {
		return
	}
	if strings.Contains(key, ".") {
		l.warn("%s: property name %q contains a dot, which meta paths read as nesting; use letters, digits and underscores only.", path, key)
		return
	}
	if metaPropertyKey.MatchString(key) {
		l.warn("%s: property name %q cannot be written as meta.%s in MRQL because it starts with a digit or is a reserved word; rename it.", path, key, key)
		return
	}
	l.warn("%s: property name %q should use letters, digits and underscores only; other characters break meta paths and MRQL field names.", path, key)
}

func (l *schemaLinter) lintLabeledEnum(obj map[string]any, path string) {
	entries, ok := obj["oneOf"].([]any)
	if !ok || len(entries) == 0 {
		return
	}
	if !l.w.charge(len(entries)) {
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
		case !l.opts.DisplayTypesTruncated && !l.hasDisplayType(name):
			l.warn("%s: no enabled plugin provides x-display %q, so the value is drawn without it.", path, name)
		}
		return
	}
	l.warn("%s: unknown x-display %q suppresses shape detection and applies no renderer, so an object value is drawn as a plain object; use %s, %s or an enabled plugin value.", path, name, builtinNames(), quoteJoin(DisplayOptOutValues, " or "))
}

// hasDisplayType is a membership test on the enabled plugins' x-display values,
// over a set built once per lint (and charged when built) instead of a scan of
// the list for every plugin-valued property.
func (l *schemaLinter) hasDisplayType(name string) bool {
	if l.displaySet == nil {
		l.w.charge(len(l.opts.DisplayTypes))
		l.displaySet = make(map[string]bool, len(l.opts.DisplayTypes))
		for _, t := range l.opts.DisplayTypes {
			l.displaySet[t] = true
		}
	}
	return l.displaySet[name]
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
	// A closed object whose minProperties reaches its declared count is only
	// valid with every declared key present.
	if min, ok := obj["minProperties"].(float64); ok && obj["additionalProperties"] == false && !hasAnyKey(obj, "$ref", "allOf") && int(min) >= len(eff.props) {
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
		if !l.w.charge(1) {
			return true // out of work: the walk reports it; do not guess a mismatch
		}
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
// Merged declarations are deduplicated (a type list by its text, a property
// subschema by identity), so repeated references add nothing.
type effectiveSchema struct {
	typeLists [][]string // one list per declaration; the value must satisfy each
	props     map[string][]any
	required  map[string]bool
	seenTypes map[string]bool
	seenProps map[string]map[uintptr]bool
}

func newEffective() *effectiveSchema {
	return &effectiveSchema{props: map[string][]any{}, required: map[string]bool{},
		seenTypes: map[string]bool{}, seenProps: map[string]map[uintptr]bool{}}
}

func (e *effectiveSchema) addTypes(w *schemaWork, types []string) {
	if !w.charge(1 + len(types)) {
		return
	}
	key := strings.Join(types, "\x00")
	if e.seenTypes[key] {
		return
	}
	e.seenTypes[key] = true
	e.typeLists = append(e.typeLists, types)
}

func (e *effectiveSchema) addProp(w *schemaWork, k string, v any) {
	if !w.charge(1) {
		return
	}
	if m, ok := v.(map[string]any); ok {
		id := reflect.ValueOf(m).Pointer()
		if e.seenProps[k] == nil {
			e.seenProps[k] = map[uintptr]bool{}
		}
		if e.seenProps[k][id] {
			return
		}
		e.seenProps[k][id] = true
	}
	e.props[k] = append(e.props[k], v)
}

// effective resolves obj through local $ref and allOf. It reports false when
// the answer is not one schema: anyOf/oneOf alternatives, a ref that is not
// local or does not resolve, a cycle, too deep a chain, or work spent.
func (l *schemaLinter) effective(node any, depth int, active map[string]bool) (*effectiveSchema, bool) {
	obj, ok := node.(map[string]any)
	if !ok || depth > 12 || !l.w.charge(1+len(obj)) {
		return nil, false
	}
	if !onlyModelledKeywords(l.w, obj) {
		return nil, false
	}
	eff := newEffective()
	merge := func(sub *effectiveSchema) {
		for _, t := range sub.typeLists {
			eff.addTypes(l.w, t)
		}
		for k, v := range sub.props {
			for _, e := range v {
				eff.addProp(l.w, k, e)
			}
		}
		if !l.w.charge(len(sub.required)) {
			return
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
		target, ok := l.localRef(pointer)
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
				return eff, l.w.ok() // up to draft-07 the siblings of $ref do not apply
			case refSiblingsUnknown:
				return nil, false
			}
		}
	}
	if list, has := obj["allOf"].([]any); has {
		if !l.w.charge(len(list)) {
			return nil, false
		}
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
		eff.addTypes(l.w, []string{t})
	case []any:
		if !l.w.charge(len(t)) {
			return nil, false
		}
		var types []string
		for _, e := range t {
			if s, ok := e.(string); ok {
				types = append(types, s)
			}
		}
		if len(types) > 0 {
			eff.addTypes(l.w, types)
		}
	}
	if props, ok := obj["properties"].(map[string]any); ok {
		if !l.w.charge(len(props)) {
			return nil, false
		}
		for k, v := range props {
			eff.addProp(l.w, k, v)
		}
	}
	if req, ok := obj["required"].([]any); ok {
		if !l.w.charge(len(req)) {
			return nil, false
		}
		for _, k := range req {
			if s, ok := k.(string); ok {
				eff.required[s] = true
			}
		}
	}
	return eff, l.w.ok()
}

// modelledKeywords are the keywords that cannot change which object shape a
// schema has: the ones effective reads (type, properties, required, $ref,
// allOf), annotations, and value constraints. Any other keyword (dependent*,
// if/then/else, not, anyOf/oneOf, patternProperties, propertyNames,
// unevaluated*, minProperties, a schema-valued additionalProperties, ...) may
// change what is valid or drawn, so a schema using one is not decided.
var modelledKeywords = map[string]bool{
	"type": true, "properties": true, "required": true, "$ref": true, "allOf": true,
	"$schema": true, "$id": true, "id": true, "$comment": true, "$defs": true, "definitions": true, "$anchor": true,
	"title": true, "description": true, "default": true, "examples": true, "readOnly": true, "writeOnly": true, "deprecated": true,
	"format": true, "enum": true, "const": true, "pattern": true,
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"minLength": true, "maxLength": true, "items": true, "prefixItems": true, "minItems": true, "maxItems": true, "uniqueItems": true,
	"contentMediaType": true, "contentEncoding": true, "minProperties": true, "maxProperties": true,
}

func onlyModelledKeywords(w *schemaWork, obj map[string]any) bool {
	if !w.charge(len(obj)) {
		return false
	}
	for k, v := range obj {
		switch {
		case modelledKeywords[k], strings.HasPrefix(k, "x-"):
		case k == "additionalProperties":
			if _, isBool := v.(bool); !isBool {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// localRef follows a "#" or "#/a/b" JSON pointer inside the document, charging
// its segments and bytes before they are split and decoded.
func (l *schemaLinter) localRef(ref string) (any, bool) {
	return localRef(l.w, l.root, ref)
}

func localRef(w *schemaWork, root any, ref string) (any, bool) {
	if ref == "#" {
		return root, true
	}
	if !strings.HasPrefix(ref, "#/") {
		return nil, false
	}
	if !w.charge(1+strings.Count(ref, "/")) || !w.chargeBytes(len(ref)) {
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
