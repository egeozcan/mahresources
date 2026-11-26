package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// Limits of one [inspect] render. A panel lists at most maxInspectRows paths
// across its sections, a preview shows at most maxInspectPanels panels (an
// [inspect] inside an [each] or [mrql] body runs once per element or item), a
// sample is cut at inspectSampleRunes, and the Meta walk stops at
// inspectMaxDepth path segments.
const (
	maxInspectPanels   = 10
	maxInspectRows     = 200
	inspectSampleRunes = 80
	inspectMaxDepth    = 6
)

type inspectKey struct{}

// inspectState counts the panels one preview request has rendered. It is a
// pointer in the context so every nested render of the request shares it.
// It is atomic because renders sharing one context may overlap.
type inspectState struct{ panels atomic.Int64 }

// WithInspect returns a context in which [inspect] renders its panel. Without
// it [inspect] renders the empty string — not a comment, not a marker — so a
// page reveals nothing about whether the shortcode is there.
//
// The template preview (GetPreviewTemplateHandler) is the only production
// caller, because it is the only render whose reader is known to be the
// template's author: the preview routes are gated at the capability that saves
// the template. Display pages, deferred bodies, share pages, MRQL API renders,
// hover cards, the lightbox and the AI template run tools never set it, and
// must not: the panel prints raw Meta values, which the run tools' privacy mode
// would otherwise have masked.
func WithInspect(ctx context.Context) context.Context {
	return context.WithValue(ctx, inspectKey{}, &inspectState{})
}

func inspectStateFrom(ctx context.Context) *inspectState {
	if ctx == nil {
		return nil
	}
	s, _ := ctx.Value(inspectKey{}).(*inspectState)
	return s
}

// RenderInspectShortcode renders [inspect] outside an [each] splice: the panel
// for the current entity context, or "" when the render is not a preview.
func RenderInspectShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string {
	return renderInspectGated(reqCtx, sc, ctx, nil, false)
}

// renderInspectGated applies the gate, the [reload]-face refusal and the panel
// cap, in that order, then renders the panel. The gate comes first so a
// non-author never sees even an error marker.
func renderInspectGated(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, elem any, hasElem bool) string {
	state := inspectStateFrom(reqCtx)
	if state == nil || ctx.UntrustedSource {
		return ""
	}
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("inspect", "[inspect] cannot be used inside a [reload] button face")
	}
	panel := int(state.panels.Add(1))
	switch {
	case panel == maxInspectPanels+1:
		return `<p class="inspect-shortcode__note">Further &#91;inspect] panels omitted (limit ` + strconv.Itoa(maxInspectPanels) + ` per preview).</p>`
	case panel > maxInspectPanels+1:
		return ""
	}
	return renderInspectPanel(ctx, sc, elem, hasElem, panel)
}

// inspectEscape escapes s for HTML text or a quoted attribute value, and writes
// "[" as a character reference so nothing the panel prints — a Meta value that
// holds shortcode text, or a snippet, which is shortcode text — could ever be
// parsed as a shortcode again. The browser decodes it for display and copying.
func inspectEscape(s string) string {
	return strings.ReplaceAll(html.EscapeString(s), "[", "&#91;")
}

type inspectRow struct {
	path    string
	typ     string
	sample  string
	snippet string // empty: no snippet cell input
	note    string // shown in the Shortcode cell when snippet is empty
}

// inspectRows collects a panel's rows under one budget shared by its sections.
type inspectRows struct {
	left    int
	omitted int
}

func (r *inspectRows) take() bool {
	if r.left > 0 {
		r.left--
		return true
	}
	r.omitted++
	return false
}

// renderInspectPanel renders the disclosure for ctx, plus the current [each]
// element's section when hasElem. panel is its 1-based number in the preview. It reads only what the context already holds
// and issues no query.
func renderInspectPanel(ctx MetaShortcodeContext, sc Shortcode, elem any, hasElem bool, panel int) string {
	// A preview can show several panels; the second and later number their
	// regions so no two landmarks on the page share a name.
	region := func(name string) string {
		if panel > 1 {
			return name + ", panel " + strconv.Itoa(panel)
		}
		return name
	}
	budget := &inspectRows{left: maxInspectRows}
	path := sc.Attrs["path"]

	var b strings.Builder
	b.WriteString(`<details class="inspect-shortcode"><summary>`)
	b.WriteString(inspectEscape(inspectSummary(ctx, path)))
	b.WriteString(`</summary><div class="inspect-shortcode__body">`)
	b.WriteString(`<p class="inspect-shortcode__note">Author aid: shown only in the template preview; renders nothing on pages.</p>`)

	// The element section comes first inside a loop: it is what the author is
	// most likely asking about there.
	if hasElem {
		writeInspectTable(&b, "Current [each] element", region("Each element paths"), inspectElementRows(elem, budget), "")
	}

	metaRows, metaEmpty := inspectMetaRows(ctx, path, budget)
	writeInspectTable(&b, "Meta", region("Meta paths"), metaRows, metaEmpty)

	if ctx.Entity != nil {
		if rows, ok := inspectPropertyRows(ctx.Entity, budget); ok {
			writeInspectTable(&b, "Properties", region("Property fields"), rows, "This entity has no readable fields.")
		}
	}

	if budget.omitted > 0 {
		fmt.Fprintf(&b, `<p class="inspect-shortcode__note">%d more paths not shown. Narrow with path=.</p>`, budget.omitted)
	}
	b.WriteString(`</div></details>`)
	return b.String()
}

func inspectSummary(ctx MetaShortcodeContext, path string) string {
	s := "Inspect: "
	if ctx.EntityType != "" {
		s += ctx.EntityType
		if ctx.EntityID != 0 {
			s += " #" + strconv.FormatUint(uint64(ctx.EntityID), 10)
		}
	} else {
		s += "no entity"
	}
	if path != "" {
		s += " · Meta at “" + path + "”"
	}
	if ctx.outer != nil {
		s += " · inside [using]"
	}
	return s
}

func writeInspectTable(b *strings.Builder, caption, region string, rows []inspectRow, empty string) {
	if len(rows) == 0 {
		if empty != "" {
			b.WriteString(`<p class="inspect-shortcode__empty">` + inspectEscape(empty) + `</p>`)
		}
		return
	}
	b.WriteString(`<div class="inspect-shortcode__scroll" role="region" aria-label="` + inspectEscape(region) + `" tabindex="0"><table><caption>`)
	b.WriteString(inspectEscape(caption))
	b.WriteString(`</caption><thead><tr><th scope="col">Path</th><th scope="col">Type</th><th scope="col">Sample</th><th scope="col">Shortcode</th></tr></thead><tbody>`)
	for _, r := range rows {
		b.WriteString(`<tr><th scope="row"><code>`)
		b.WriteString(inspectEscape(r.path))
		b.WriteString(`</code></th><td>`)
		b.WriteString(inspectEscape(r.typ))
		b.WriteString(`</td><td>`)
		b.WriteString(inspectEscape(r.sample))
		b.WriteString(`</td><td>`)
		if r.snippet != "" {
			b.WriteString(`<input type="text" readonly class="inspect-shortcode__snippet" value="`)
			b.WriteString(inspectEscape(r.snippet))
			b.WriteString(`" aria-label="`)
			b.WriteString(inspectEscape("Shortcode for " + r.path))
			b.WriteString(`">`)
		} else {
			b.WriteString(inspectEscape(r.note))
		}
		b.WriteString(`</td></tr>`)
	}
	b.WriteString(`</tbody></table></div>`)
}

// inspectKeyAddressable reports whether a Meta key can be written as one
// segment of a path= value: the path splits on ".", and the attribute is
// quoted, so neither may appear, nor may brackets, which would end the token.
func inspectKeyAddressable(k string) bool {
	return k != "" && !strings.ContainsAny(k, `."'[]`)
}

func inspectJSONType(v any) string {
	switch t := v.(type) {
	case nil:
		return "null"
	case string:
		return "string"
	case bool:
		return "boolean"
	case float64, json.Number:
		return "number"
	case map[string]any:
		return fmt.Sprintf("object (%d keys)", len(t))
	case []any:
		return fmt.Sprintf("array (%d items)", len(t))
	default:
		return "value"
	}
}

func inspectJSONSample(v any) string {
	switch t := v.(type) {
	case string:
		return `"` + truncateRunes(t, inspectSampleRunes) + `"`
	case map[string]any, []any, nil:
		return ""
	default:
		return truncateRunes(formatJSONScalar(t, "", ""), inspectSampleRunes)
	}
}

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	count := 0
	for i := range s {
		if count == n {
			return s[:i] + "…"
		}
		count++
	}
	return s
}

// walkInspectJSON emits one row per key of obj, depth-first in sorted key
// order, descending into objects until a path has inspectMaxDepth segments.
// Arrays are leaves: no path= indexes into one. A key that cannot be written as
// a path is listed without a snippet, and so is everything beneath it.
func walkInspectJSON(obj map[string]any, prefix string, segments int, addressable bool, budget *inspectRows, snippet func(path string, v any) string, rows *[]inspectRow) {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := obj[k]
		p := k
		if prefix != "" {
			p = prefix + "." + k
		}
		ok := addressable && inspectKeyAddressable(k)
		if budget.take() {
			*rows = append(*rows, inspectJSONRow(p, v, ok, snippet))
		}
		if child, isObj := v.(map[string]any); isObj && segments+1 < inspectMaxDepth {
			walkInspectJSON(child, p, segments+1, ok, budget, snippet, rows)
		}
	}
}

func inspectJSONRow(p string, v any, addressable bool, snippet func(path string, v any) string) inspectRow {
	row := inspectRow{path: p, typ: inspectJSONType(v), sample: inspectJSONSample(v)}
	if addressable {
		row.snippet = snippet(p, v)
	} else {
		row.note = "not addressable by path="
	}
	return row
}

func inspectMetaSnippet(p string, v any) string {
	if _, isArr := v.([]any); isArr {
		return `[each path="` + p + `"][item][/each]`
	}
	return `[meta path="` + p + `"]`
}

// inspectMetaRows lists the Meta tree, or the subtree at path. empty is the
// message to show when there are no rows.
func inspectMetaRows(ctx MetaShortcodeContext, path string, budget *inspectRows) (rows []inspectRow, empty string) {
	root := ctx.decodeMeta()
	if path == "" {
		obj, _ := root.(map[string]any)
		if len(obj) == 0 {
			return nil, "This entity has no Meta."
		}
		walkInspectJSON(obj, "", 0, true, budget, inspectMetaSnippet, &rows)
		return rows, ""
	}

	v, found := navigateJSONValue(root, path)
	if !found {
		return nil, `No Meta value at path "` + path + `".`
	}
	addressable := true
	for _, seg := range strings.Split(path, ".") {
		addressable = addressable && inspectKeyAddressable(seg)
	}
	segments := len(strings.Split(path, "."))
	obj, isObj := v.(map[string]any)
	// A path already at the depth limit lists itself, not its children, so no
	// row is ever deeper than inspectMaxDepth segments.
	if !isObj || segments >= inspectMaxDepth {
		if budget.take() {
			rows = append(rows, inspectJSONRow(path, v, addressable, inspectMetaSnippet))
		}
		return rows, ""
	}
	if len(obj) == 0 {
		return nil, `The object at path "` + path + `" is empty.`
	}
	walkInspectJSON(obj, path, segments, addressable, budget, inspectMetaSnippet, &rows)
	return rows, ""
}

func inspectItemSnippet(p string, _ any) string {
	return `[item path="` + p + `"]`
}

// inspectElementRows lists the current [each] element: its position, then the
// element itself when it is not an object, or its paths when it is.
func inspectElementRows(elem any, budget *inspectRows) []inspectRow {
	var rows []inspectRow
	if budget.take() {
		rows = append(rows, inspectRow{path: "(position)", typ: "position", sample: "1", snippet: `[item index="true"]`})
	}
	obj, isObj := elem.(map[string]any)
	if !isObj {
		if budget.take() {
			rows = append(rows, inspectRow{path: "(element)", typ: inspectJSONType(elem), sample: inspectJSONSample(elem), snippet: "[item]"})
		}
		return rows
	}
	walkInspectJSON(obj, "", 0, true, budget, inspectItemSnippet, &rows)
	return rows
}

var (
	inspectTimeType       = reflect.TypeOf(time.Time{})
	inspectRawMessageType = reflect.TypeOf(json.RawMessage(nil))
)

// inspectPropertyRows lists the entity's exported fields, one level deep and in
// declaration order. It never reads through an association: a struct, slice,
// array or map field is listed by name and kind only, with no value, count or
// nested field. A confined user's subtree root preloads its Owner, which lies
// outside the subtree, and the panel must not print it. Meta and OwnMeta are
// the Meta section's, and credential fields are always left out, whatever the
// render's credentialsVisible says: an inspector gets screenshotted.
func inspectPropertyRows(entity any, budget *inspectRows) ([]inspectRow, bool) {
	v := reflect.ValueOf(entity)
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil, false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}

	var rows []inspectRow
	for _, sf := range reflect.VisibleFields(v.Type()) {
		if sf.Anonymous || !sf.IsExported() {
			continue
		}
		name := sf.Name
		if name == "Meta" || name == "OwnMeta" || credentialFields[name] {
			continue
		}
		f, err := v.FieldByIndexErr(sf.Index)
		if err != nil || !f.IsValid() || !f.CanInterface() {
			continue
		}
		if !budget.take() {
			continue
		}
		rows = append(rows, inspectPropertyRow(name, sf.Type, f))
	}
	return rows, true
}

func inspectPropertyRow(name string, ft reflect.Type, f reflect.Value) inspectRow {
	base := ft
	for base.Kind() == reflect.Ptr {
		base = base.Elem()
	}
	row := inspectRow{path: name}

	switch {
	case base == inspectTimeType:
		row.typ = "time"
		row.sample = inspectPropertySample(f)
		row.snippet = `[property path="` + name + `" format="date"]`
		return row
	case base == inspectRawMessageType || base.Kind() == reflect.Map ||
		(base.Kind() == reflect.Slice && base.Elem().Kind() == reflect.Uint8):
		row.typ = "json"
		row.note = "no snippet"
		return row
	case base.Kind() == reflect.Struct:
		row.typ = "struct"
		if _, hasID := base.FieldByName("ID"); hasID {
			row.typ = "relation"
		}
		row.note = "association: values not shown"
		return row
	case base.Kind() == reflect.Slice || base.Kind() == reflect.Array:
		row.typ = "list"
		row.note = "association: values not shown"
		return row
	}

	switch base.Kind() {
	case reflect.String:
		row.typ = "string"
	case reflect.Bool:
		row.typ = "boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		row.typ = "integer"
	case reflect.Float32, reflect.Float64:
		row.typ = "number"
	default:
		// An interface, func or channel: nothing a template reads.
		row.typ = "value"
		row.note = "no snippet"
		return row
	}

	sample := inspectPropertySample(f)
	if name == "Description" {
		sample = stripTemplateComments(sample)
	}
	if row.typ == "string" && !isNilPointer(f) {
		sample = `"` + truncateRunes(sample, inspectSampleRunes) + `"`
	} else {
		sample = truncateRunes(sample, inspectSampleRunes)
	}
	row.sample = sample
	row.snippet = `[property path="` + name + `"]`
	if name == "FileSize" && row.typ == "integer" {
		row.snippet = `[property path="FileSize" format="filesize"]`
	}
	return row
}

func isNilPointer(f reflect.Value) bool {
	for f.Kind() == reflect.Ptr || f.Kind() == reflect.Interface {
		if f.IsNil() {
			return true
		}
		f = f.Elem()
	}
	return false
}

func inspectPropertySample(f reflect.Value) string {
	if isNilPointer(f) {
		return "(none)"
	}
	return formatPropertyValue(f, "", "")
}
