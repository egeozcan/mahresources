package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"reflect"
	"strings"
	"sync"
)

// MetaShortcodeContext holds the entity context needed to render [meta] shortcodes.
type MetaShortcodeContext struct {
	EntityType    string // "group", "resource", "note"
	EntityID      uint
	Meta          json.RawMessage // entity's full Meta JSON
	MetaSchema    string          // category's MetaSchema JSON string (may be empty)
	Entity        any             // the full entity object (Group, Resource, Note, etc.)
	ScopeGroupID  uint            // resolved "entity" scope (owning group or sentinel for ownerless)
	ParentGroupID uint            // resolved "parent" scope (owner's owner)
	RootGroupID   uint            // resolved "root" scope (top of chain)
	// Presentation names accompany the resolved IDs so plugin templates can
	// render ownership breadcrumbs without issuing one lookup per card. They are
	// populated from preloaded associations on native pages and from the MRQL
	// renderer's request-local ancestry batch on query-result surfaces.
	ScopeGroupName     string
	ScopeCategoryName  string
	ParentGroupName    string
	ParentCategoryName string
	RootGroupName      string
	RootCategoryName   string
	// decodedMeta memoizes the parsed Meta for every reader of it — the [meta]
	// widget's value, its schema slice, inline values, and the paths [each] and
	// [conditional] resolve. It is a pointer because this struct travels by value
	// through every handler, so all the copies share one decode: a slot carrying
	// N Meta reads, rendered on a list of M cards, would otherwise unmarshal each
	// card's blob N times. The tree it holds is shared and must be treated
	// read-only (TestDecodedMetaIsSharedReadOnly).
	// processWithDepth populates it per render; a zero value simply decodes
	// locally, which is what a handler called directly (a test, another entry
	// point) gets. mrql_handler builds a fresh context per result item, so the
	// memo never outlives the entity it belongs to.
	decodedMeta *decodedMetaCache
	// ForceReadOnly forces every [meta] to render non-editable regardless of the
	// shortcode's editable attr. Set by the share renderer: the public /s/<token>
	// page must never emit an edit affordance that would POST to the primary server.
	ForceReadOnly bool
	// UntrustedSource marks a render that started in text a less trusted
	// account wrote — a Description or a note text block with -auth on (#92).
	// That text is sanitized before shortcodes expand, so what a shortcode emits
	// is never checked; raw="true" ([property], [meta inline], [item]) is
	// therefore ignored and the value escaped. It carries to every context the
	// render derives (an [mrql] or [using] result, a deferred body's token), so
	// an admin template reached from user content is escaped too.
	UntrustedSource bool
	// outer is the context a [using] block switched away from, nil at the top
	// of a render and on every MRQL result. Nothing reads it yet; it is carried
	// so that reaching the enclosing entity (ctx="outer") can be added without
	// reshaping how [using] builds its child.
	outer *MetaShortcodeContext
	// meter is the render this context belongs to, set by processWithDepth so
	// handlers that take no request context (the schema slice) still reach the
	// budget and the cancellation. Nil for a handler called directly.
	meter *renderMeter
	// scalarObserver is the render's WithScalarFormatObserver hook, copied from
	// the request context by processWithDepth.
	scalarObserver func(string)
}

// RenderMetaShortcode expands a [meta] shortcode into a <meta-shortcode> custom
// element, or — with inline="true" — into the bare value.
func RenderMetaShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	return renderMetaWidget(sc, ctx, "")
}

// RenderDateTimeShortcode uses the shared field editor with date/time presentation.
func RenderDateTimeShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	attrs := fmt.Sprintf(` data-date-time="true" data-layout="%s" data-input-layout="%s"`,
		html.EscapeString(sc.Attrs["layout"]), html.EscapeString(sc.Attrs["input-layout"]))
	return renderMetaWidget(sc, ctx, attrs)
}

// RenderPillsShortcode presents a single Meta value as a segmented selector.
func RenderPillsShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	attrs := fmt.Sprintf(` data-pills="true" data-options="%s"`, html.EscapeString(sc.Attrs["options"]))
	return renderMetaWidget(sc, ctx, attrs)
}

func renderMetaWidget(sc Shortcode, ctx MetaShortcodeContext, extraAttrs string) string {
	path := sc.Attrs["path"]
	if path == "" {
		return ""
	}

	if extraAttrs == "" && sc.Attrs["inline"] == "true" {
		return renderInlineMetaValue(sc, ctx)
	}

	editable := sc.Attrs["editable"] == "true"
	if ctx.ForceReadOnly {
		editable = false
	}
	hideEmpty := sc.Attrs["hide-empty"] == "true"
	defaultVal := sc.Attrs["default"]

	valueJSON := ctx.valueJSONAtPath(path)
	schemaSlice := ctx.schemaSlice(path)

	return fmt.Sprintf(
		`<meta-shortcode data-path="%s" data-editable="%t" data-hide-empty="%t" data-default="%s" data-entity-type="%s" data-entity-id="%d" data-schema="%s" data-value="%s"%s></meta-shortcode>`,
		html.EscapeString(path),
		editable,
		hideEmpty,
		html.EscapeString(defaultVal),
		html.EscapeString(ctx.EntityType),
		ctx.EntityID,
		html.EscapeString(schemaSlice),
		html.EscapeString(valueJSON),
		extraAttrs,
	)
}

// renderInlineMetaValue renders [meta inline="true"]: the value itself rather
// than the editing widget, formatted with the same format=/layout=/default=
// helpers as [property] and [item], and HTML-escaped unless raw="true".
//
// It exists because the widget cannot be used where only text is legal. A
// <meta-shortcode> element inside an attribute value is broken markup twice
// over: an element cannot nest in an attribute at all, and the element's own
// quotes close the attribute early.
//
// Escaping is therefore the default, and its guarantee is narrow: it covers
// & < > ' " and nothing else, which keeps a value inside a QUOTED attribute and
// stops nothing that happens after the browser parses that value back out. A
// javascript: URL in href still runs, an unquoted attribute still gains
// neighbours from a value containing a space, an on* handler still executes a
// value containing a quote (the HTML parser undoes the escaping before the JS
// parser sees it), and style still takes CSS. Those four shapes are what
// unsafeAttributeContexts in lint.go warns about, because the boundary is real:
// the template is written by an admin or editor, the Meta value in it by anyone
// who can edit the entity.
//
// editable= is ignored: inline output is text, so there is nothing to edit, and
// honouring it would put an edit affordance inside whatever attribute the author
// was building. hide-empty= is honoured here rather than client-side, because
// inline mode emits no element for a client to act on.
func renderInlineMetaValue(sc Shortcode, ctx MetaShortcodeContext) string {
	value, _ := navigateJSONValue(ctx.decodeMeta(), sc.Attrs["path"])
	text := formatJSONScalarObserved(ctx, value, sc.Attrs["format"], sc.Attrs["layout"])

	// Trimmed for the emptiness decision only — the widget's client side treats
	// a whitespace-only value as empty (meta-shortcode.ts), and inline mode must
	// agree or the same Meta renders three spaces here and the default there.
	// The value itself is emitted untrimmed when it is not empty.
	if strings.TrimSpace(text) == "" {
		if sc.Attrs["hide-empty"] == "true" {
			return ""
		}
		text = sc.Attrs["default"]
	}

	if sc.Attrs["raw"] == "true" && !ctx.UntrustedSource {
		return text
	}
	return html.EscapeString(text)
}

// navigateJSONValue walks a dot-separated path into a decoded JSON value,
// taking a map[string]any at each step. It is the single walker behind [item],
// [meta], [each] and [conditional]; found reports whether the path resolved,
// which is what lets the widget tell an explicit null (JSON "null") apart from a
// missing key (no value at all).
//
// An empty path returns current unchanged. What that means is the caller's to
// decide, and the three callers decide differently on purpose: [item] with no
// path= renders the element itself, while a [meta] or [conditional] with no
// path= names no value and says so before calling here.
func navigateJSONValue(current any, path string) (value any, found bool) {
	if path == "" {
		return current, true
	}
	for _, part := range strings.Split(path, ".") {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = obj[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// valueJSONAtPath navigates the entity's Meta by dot-notation path and returns
// the JSON-encoded value there, or "" when the path resolves to nothing. An
// empty path is "nothing" too rather than the whole blob — RenderMetaShortcode
// returns before it could ask, and a widget bound to the entire Meta is not a
// thing this shortcode can build.
//
// The re-marshal is not a round trip added here: the widget's data-value
// attribute has always carried JSON text, and the sub-value has to be encoded
// from something. Taking it from the memo means the blob is parsed once per
// render rather than once per [meta] occurrence per card.
func (ctx MetaShortcodeContext) valueJSONAtPath(path string) string {
	if path == "" {
		return ""
	}

	value, found := navigateJSONValue(ctx.decodeMeta(), path)
	if !found {
		return ""
	}

	encoded, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	return string(encoded)
}

// rawValueAtPath resolves a Meta dot-path to its decoded value, the source
// [conditional] and [each] read. An empty path names no value: it resolves to
// nil, so a [conditional path=""] tests nothing and an [each path=""] renders
// its [else], rather than testing or iterating the whole blob. A miss and an
// explicit null are both nil, which is what empty= means here.
func (ctx MetaShortcodeContext) rawValueAtPath(path string) any {
	if path == "" {
		return nil
	}
	value, _ := navigateJSONValue(ctx.decodeMeta(), path)
	return value
}

// schemaSlice is the sub-schema for path, bounded by one schemaWork. A schema
// too complex to expand yields no slice; on a budgeted or cancelled render it
// also ends the render with the budget error, so a model cannot get a quiet
// empty widget out of a schema the preview refused to expand.
func (ctx MetaShortcodeContext) schemaSlice(path string) string {
	if ctx.MetaSchema == "" {
		return ""
	}
	var reqCtx context.Context
	if ctx.meter != nil {
		reqCtx = ctx.meter.ctx
	}
	w := newSchemaWork(reqCtx)
	out := extractSchemaSliceW(w, ctx.MetaSchema, path, ctx.decodeMetaObject())
	if err := w.Err(); err != nil && ctx.meter != nil {
		if ctx.meter.limited() || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			ctx.meter.failErr(err)
		}
	}
	return out
}

// extractSchemaSlice navigates a JSON Schema by dot-notation path through
// nested "properties" and returns the JSON-encoded sub-schema, or "" if not found.
// Handles $ref, allOf, oneOf, anyOf, and if/then/else (using metaValue to
// evaluate conditions). metaValue is the entity's decoded Meta and may be nil if
// no value context is available; it is read, never written — see
// TestDecodedMetaIsSharedReadOnly, since callers hand it the memoized tree.
func extractSchemaSlice(schemaStr string, path string, metaValue map[string]any) string {
	return extractSchemaSliceW(newSchemaWork(nil), schemaStr, path, metaValue)
}

func extractSchemaSliceW(w *schemaWork, schemaStr string, path string, metaValue map[string]any) string {
	if schemaStr == "" {
		return ""
	}

	var root map[string]any
	if err := json.Unmarshal([]byte(schemaStr), &root); err != nil {
		return ""
	}

	parts := strings.Split(path, ".")
	current := root
	currentValue := metaValue
	// rawValue tracks the value at the current path as any (including primitives)
	var rawValue any = metaValue

	for _, part := range parts {
		if !w.charge(1) {
			return ""
		}
		resolved := resolveSchemaNodeWithValue(w, current, root, currentValue)
		if resolved == nil || !w.ok() {
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
	resolved := resolveLeafSchema(w, current, root, leafValue, 0)
	if !w.ok() {
		return ""
	}
	if resolved == nil {
		resolved = current
	}

	// Admit the serialized size (every repeat counted, escaping weighted)
	// before marshaling it.
	if !w.valueBytes(resolved) {
		return ""
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
func resolveLeafSchema(w *schemaWork, node map[string]any, root map[string]any, value map[string]any, depth int) map[string]any {
	if node == nil || w.deep(depth) || !w.charge(1+len(node)) {
		return nil
	}
	// Resolve $ref
	if ref, ok := node["$ref"].(string); ok {
		// A ref already being expanded on this path is a cycle: stop with the
		// node as it is rather than follow it again.
		if !w.enterRef(ref) {
			return node
		}
		defer w.leaveRef(ref)
		resolved := followRef(w, ref, root)
		if resolved == nil {
			return nil
		}
		merged := shallowMergeSchema(w, resolved, node, 0)
		if merged == nil {
			return nil
		}
		delete(merged, "$ref")
		return resolveLeafSchema(w, merged, root, value, depth+1)
	}
	// Recursively resolve $ref everywhere in the leaf (composition branches,
	// properties, items) so the client doesn't need the root $defs.
	node = inlineRefsDeep(w, node, root, depth+1)
	if !w.ok() {
		return nil
	}

	// Resolve if/then/else
	if _, ok := node["if"].(map[string]any); ok {
		if !w.charge(len(node)) {
			return nil
		}
		base := make(map[string]any)
		for k, v := range node {
			if k != "if" && k != "then" && k != "else" {
				base[k] = v
			}
		}
		matched, supported := tryEvaluateCondition(w, node["if"].(map[string]any), value)
		base = mergeConditionBranches(w, base, node, matched, supported)
		if base == nil {
			return nil
		}
		// Re-resolve in case the merged branch introduced $ref or composition.
		return resolveLeafSchema(w, base, root, value, depth+1)
	}
	return node
}

// mergeConditionBranches merges the active if/then/else branch into base, or
// both when the condition was not evaluable. nil means the work was exhausted.
func mergeConditionBranches(w *schemaWork, base, node map[string]any, matched, supported bool) map[string]any {
	merge := func(key string) {
		if base == nil {
			return
		}
		if branch, ok := node[key].(map[string]any); ok {
			base = shallowMergeSchema(w, base, branch, 0)
		}
	}
	switch {
	case !supported:
		// Condition uses unsupported features — merge both branches so all
		// possible properties are discoverable.
		merge("then")
		merge("else")
	case matched:
		merge("then")
	default:
		merge("else")
	}
	return base
}

// inlineRefsDeep resolves all $ref in a schema node recursively — in
// composition branches (allOf/oneOf/anyOf), properties, and items. A $ref that
// is already being expanded on the current path (a recursive schema) is left in
// place, and every visit and copy is charged to w.
func inlineRefsDeep(w *schemaWork, node map[string]any, root map[string]any, depth int) map[string]any {
	if w.deep(depth) || !w.charge(1+len(node)) {
		return node
	}
	// Resolve direct $ref
	if ref, ok := node["$ref"].(string); ok {
		if w.enterRef(ref) {
			defer w.leaveRef(ref)
			if refTarget := followRef(w, ref, root); refTarget != nil {
				merged := shallowMergeSchema(w, refTarget, node, 0)
				if merged == nil {
					return node
				}
				delete(merged, "$ref")
				return inlineRefsDeep(w, merged, root, depth+1)
			}
		} else {
			return node
		}
	}

	result := make(map[string]any, len(node))
	for k, v := range node {
		result[k] = v
	}

	// Recurse into composition branches
	for _, keyword := range []string{"allOf", "oneOf", "anyOf"} {
		if branches, ok := result[keyword].([]any); ok {
			if !w.charge(len(branches)) {
				return result
			}
			newBranches := make([]any, len(branches))
			for i, branch := range branches {
				if branchMap, ok := branch.(map[string]any); ok {
					newBranches[i] = inlineRefsDeep(w, branchMap, root, depth+1)
				} else {
					newBranches[i] = branch
				}
				if !w.ok() {
					return result
				}
			}
			result[keyword] = newBranches
		}
	}

	// Recurse into properties
	if props, ok := result["properties"].(map[string]any); ok {
		if !w.charge(len(props)) {
			return result
		}
		newProps := make(map[string]any, len(props))
		for k, v := range props {
			if propSchema, ok := v.(map[string]any); ok {
				newProps[k] = inlineRefsDeep(w, propSchema, root, depth+1)
			} else {
				newProps[k] = v
			}
			if !w.ok() {
				return result
			}
		}
		result["properties"] = newProps
	}

	// Recurse into items
	if items, ok := result["items"].(map[string]any); ok {
		result["items"] = inlineRefsDeep(w, items, root, depth+1)
	}

	// Recurse into if/then/else branches
	for _, keyword := range []string{"if", "then", "else"} {
		if branch, ok := result[keyword].(map[string]any); ok {
			result[keyword] = inlineRefsDeep(w, branch, root, depth+1)
		}
	}

	return result
}

// resolveSchemaNodeWithValue recursively resolves $ref, allOf, oneOf, anyOf,
// and if/then/else on a schema node. The value parameter carries the entity
// data at the current schema level for evaluating if/then/else conditions.
// It may be nil if no value context is available.
func resolveSchemaNodeWithValue(w *schemaWork, node map[string]any, root map[string]any, value map[string]any) map[string]any {
	return resolveSchemaNodeImpl(w, node, root, value, 0)
}

func resolveSchemaNodeImpl(w *schemaWork, node map[string]any, root map[string]any, value map[string]any, depth int) map[string]any {
	if node == nil || depth > 10 {
		return node
	}
	// Every visit is work; a failure here is read by the caller through w.
	if !w.charge(1 + len(node)) {
		return nil
	}

	// Resolve $ref first, then recurse on the result.
	if ref, ok := node["$ref"].(string); ok {
		if !w.enterRef(ref) {
			return node
		}
		defer w.leaveRef(ref)
		resolved := followRef(w, ref, root)
		if resolved == nil {
			return nil
		}
		merged := shallowMergeSchema(w, resolved, node, 0)
		if merged == nil {
			return nil
		}
		delete(merged, "$ref")
		return resolveSchemaNodeImpl(w, merged, root, value, depth+1)
	}

	// Resolve if/then/else using the entity value.
	if _, ok := node["if"].(map[string]any); ok {
		base := make(map[string]any)
		for k, v := range node {
			if k != "if" && k != "then" && k != "else" {
				base[k] = v
			}
		}
		matched, supported := tryEvaluateCondition(w, node["if"].(map[string]any), value)
		base = mergeConditionBranches(w, base, node, matched, supported)
		if base == nil {
			return nil
		}
		return resolveSchemaNodeImpl(w, base, root, value, depth+1)
	}

	// Resolve all composition keywords present on this node.
	composed := false
	if !w.charge(len(node)) {
		return nil
	}
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
			r := resolveSchemaNodeImpl(w, branchMap, root, value, depth+1)
			if !w.ok() {
				return nil
			}
			if r == nil {
				r = branchMap
			}
			merged = shallowMergeSchema(w, merged, r, 0)
			if merged == nil {
				return nil
			}
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
func tryEvaluateCondition(w *schemaWork, ifSchema map[string]any, value map[string]any) (matched bool, supported bool) {
	if value == nil || !w.charge(1+len(ifSchema)) {
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
		types := normalizeTypeList(w, typeRaw)
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
			if !w.charge(len(enumVal)) {
				return false, false
			}
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
	if !w.charge(len(props)) {
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
			if !w.charge(len(enumVal)) {
				return false, false
			}
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
			types := normalizeTypeList(w, typeRaw)
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
func normalizeTypeList(w *schemaWork, typeRaw any) []string {
	if s, ok := typeRaw.(string); ok {
		return []string{s}
	}
	if arr, ok := typeRaw.([]any); ok {
		if !w.charge(len(arr)) {
			return nil
		}
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
// "#/definitions/Address" within the root schema. Each pointer segment is
// charged to w before it is decoded.
func followRef(w *schemaWork, ref string, root map[string]any) map[string]any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}
	if !w.charge(1+strings.Count(ref, "/")) || !w.chargeBytes(len(ref)) {
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

// shallowMergeSchema deep-merges src into dst. "properties" maps are merged
// recursively so that overlapping property keys combine their children
// rather than one branch replacing the other. The entries about to be copied
// are charged first, and the recursion shares the depth cap; nil means w ran
// out.
func shallowMergeSchema(w *schemaWork, dst, src map[string]any, depth int) map[string]any {
	if w.deep(depth) || !w.charge(len(dst)+len(src)) {
		return nil
	}
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
				if !w.charge(len(dstProps) + len(srcProps)) {
					return nil
				}
				merged := make(map[string]any, len(dstProps)+len(srcProps))
				for pk, pv := range dstProps {
					merged[pk] = pv
				}
				for pk, pv := range srcProps {
					// If both sides define the same property as objects,
					// recursively merge so nested children from both survive.
					if existing, ok := merged[pk].(map[string]any); ok {
						if srcChild, ok := pv.(map[string]any); ok {
							child := shallowMergeSchema(w, existing, srcChild, depth+1)
							if child == nil {
								return nil
							}
							merged[pk] = child
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

// decodedMetaCache holds one parse of an entity's Meta, shared by every by-value
// copy of the context that carries it.
type decodedMetaCache struct {
	once  sync.Once
	value any
}

// decodeMetaObject returns the memoized Meta as a JSON object, or nil when the
// blob is absent, malformed, or not an object at its top level — the three cases
// the previous per-call json.Unmarshal into a map[string]any also produced nil
// for.
// MetaObject is decodeMetaObject for readers outside this package: the parsed
// Meta shared by every reader of this render, so callers must not modify it.
// The plugin bridge reads it rather than unmarshalling the blob again per slot.
func (ctx MetaShortcodeContext) MetaObject() map[string]any {
	return ctx.decodeMetaObject()
}

func (ctx MetaShortcodeContext) decodeMetaObject() map[string]any {
	obj, _ := ctx.decodeMeta().(map[string]any)
	return obj
}

// decodeMeta returns the parsed Meta, decoding at most once when the context
// carries a cache and every call when it does not. A malformed blob decodes to
// nil, which navigates to nothing and falls through to default= — the same
// answer as a missing path, and what the widget's client side also does.
//
// A blob that only *partly* decodes is nil too, and that is the whole of the
// error branch below. encoding/json keeps going after a semantic error: a number
// too large for a float64 leaves that one key nil and populates the rest, so
// half a Meta would render while the failed key rendered as an explicit null —
// and an editable widget bound to it would offer that null back for writing.
// The three non-inline readers each rejected the whole decode on error before
// they shared this memo, so this is also what keeps them answering as they did.
//
// The tree is shared by every reader on every by-value copy of the context, so
// callers must treat it as read-only.
func (ctx MetaShortcodeContext) decodeMeta() any {
	if len(ctx.Meta) == 0 {
		return nil
	}
	decode := func() any {
		var v any
		if err := json.Unmarshal(ctx.Meta, &v); err != nil {
			return nil
		}
		return v
	}
	if ctx.decodedMeta == nil {
		return decode()
	}
	ctx.decodedMeta.once.Do(func() { ctx.decodedMeta.value = decode() })
	return ctx.decodedMeta.value
}
