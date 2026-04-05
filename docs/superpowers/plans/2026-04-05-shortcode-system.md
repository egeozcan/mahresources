# Shortcode System Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add shortcode parsing to category custom render locations so users can write `[meta path="cooking.time" editable=true]` to display/edit metadata inline, with plugin extensibility via `mah.shortcode()`.

**Architecture:** Server-side Go parses shortcodes at template render time via a pongo2 tag. Built-in `[meta]` shortcode expands to a `<meta-shortcode>` Lit web component that reuses schema-editor rendering. Plugin shortcodes render server-side via Lua. A new `editMeta` endpoint handles deep-merge-by-path meta updates.

**Tech Stack:** Go (pongo2, GORM), TypeScript/Lit, Lua (gopher-lua), Playwright (E2E tests)

**Spec:** `docs/superpowers/specs/2026-04-05-shortcode-system-design.md`

**Note on pongo2 filter vs tag:** The spec mentions a pongo2 "filter," but filters don't have access to the execution context (needed for `_pluginManager`). This plan uses a pongo2 **tag** instead — `{% process_shortcodes expr entity %}` — which matches the `{% plugin_slot %}` pattern already in the codebase. The tag also eliminates the need for `{% autoescape off %}` wrappers since tags control their own output.

---

## File Map

| File | Action | Responsibility |
|------|--------|----------------|
| `shortcodes/parser.go` | Create | Parse `[name attr="val"]` syntax, return structured shortcode list |
| `shortcodes/parser_test.go` | Create | Unit tests for parser |
| `shortcodes/meta_handler.go` | Create | Built-in `[meta]` shortcode: extract schema slice, value, emit `<meta-shortcode>` HTML |
| `shortcodes/meta_handler_test.go` | Create | Unit tests for meta handler |
| `shortcodes/processor.go` | Create | `ProcessShortcodes()` orchestrator: dispatches to built-in + plugin handlers |
| `shortcodes/processor_test.go` | Create | Unit tests for processor |
| `server/template_handlers/template_filters/shortcode_tag.go` | Create | Pongo2 tag `{% process_shortcodes %}` — extracts entity + PM from context, calls processor |
| `server/api_handlers/meta_edit_handler.go` | Create | `GetEditMetaHandler` — deep-merge-by-path meta editing |
| `server/api_handlers/meta_edit_handler_test.go` | Create | Unit tests for editMeta handler |
| `plugin_system/shortcodes.go` | Create | `PluginShortcode` type, `parseShortcodeTable()`, `RenderShortcode()` |
| `plugin_system/shortcodes_test.go` | Create | Unit tests for plugin shortcode registration + rendering |
| `src/webcomponents/meta-shortcode.ts` | Create | `<meta-shortcode>` Lit web component |
| `src/main.js` | Modify | Add import for `meta-shortcode.ts` |
| `plugin_system/manager.go` | Modify | Add `shortcodes` field, `mah.shortcode()` Lua function, cleanup on disable/close |
| `server/routes.go` | Modify | Add `editMeta` routes for group, resource, note |
| `server/interfaces/generic_interfaces.go` | Modify | Add `MetaEditor` interface |
| `application_context/basic_entity_context.go` | Modify | Add `UpdateMeta()` method to `EntityWriter` |
| Templates (14 locations) | Modify | Replace `{% autoescape off %}{{ custom_field }}{% endautoescape %}` with `{% process_shortcodes custom_field entity %}` |
| `e2e/tests/shortcodes.spec.ts` | Create | E2E tests for shortcode display and editing |
| `e2e/helpers/api-client.ts` | Modify | Add helper methods for creating entities with meta and categories with schemas |

---

### Task 1: Shortcode Parser

**Files:**
- Create: `shortcodes/parser.go`
- Create: `shortcodes/parser_test.go`

- [ ] **Step 1: Write failing tests for the parser**

```go
// shortcodes/parser_test.go
package shortcodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEmpty(t *testing.T) {
	result := Parse("")
	assert.Empty(t, result)
}

func TestParseNoShortcodes(t *testing.T) {
	result := Parse("just some plain text with [brackets]")
	assert.Empty(t, result)
}

func TestParseMetaShortcode(t *testing.T) {
	result := Parse(`[meta path="cooking.time"]`)
	require.Len(t, result, 1)
	assert.Equal(t, "meta", result[0].Name)
	assert.Equal(t, "cooking.time", result[0].Attrs["path"])
	assert.Equal(t, `[meta path="cooking.time"]`, result[0].Raw)
	assert.Equal(t, 0, result[0].Start)
	assert.Equal(t, 26, result[0].End)
}

func TestParseMultipleAttributes(t *testing.T) {
	result := Parse(`[meta path="cooking.time" editable=true hide-empty=false]`)
	require.Len(t, result, 1)
	assert.Equal(t, "cooking.time", result[0].Attrs["path"])
	assert.Equal(t, "true", result[0].Attrs["editable"])
	assert.Equal(t, "false", result[0].Attrs["hide-empty"])
}

func TestParseUnquotedValues(t *testing.T) {
	result := Parse(`[meta path="a.b" editable=true]`)
	require.Len(t, result, 1)
	assert.Equal(t, "true", result[0].Attrs["editable"])
}

func TestParseMultipleShortcodes(t *testing.T) {
	result := Parse(`before [meta path="a"] middle [meta path="b"] after`)
	require.Len(t, result, 2)
	assert.Equal(t, "a", result[0].Attrs["path"])
	assert.Equal(t, "b", result[1].Attrs["path"])
}

func TestParsePluginShortcode(t *testing.T) {
	result := Parse(`[plugin:my-plugin:rating max="5"]`)
	require.Len(t, result, 1)
	assert.Equal(t, "plugin:my-plugin:rating", result[0].Name)
	assert.Equal(t, "5", result[0].Attrs["max"])
}

func TestParsePreservesHTMLAround(t *testing.T) {
	result := Parse(`<div class="flex">[meta path="a"]</div>`)
	require.Len(t, result, 1)
	assert.Equal(t, 20, result[0].Start)
}

func TestParseIgnoresUnrecognizedBrackets(t *testing.T) {
	// Only "meta" and "plugin:*" are valid shortcode names
	result := Parse(`see [this page] for details`)
	assert.Empty(t, result)
}

func TestParseSingleQuotedValues(t *testing.T) {
	result := Parse(`[meta path='cooking.time']`)
	require.Len(t, result, 1)
	assert.Equal(t, "cooking.time", result[0].Attrs["path"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./shortcodes/... -v -run TestParse`
Expected: FAIL — package does not exist yet

- [ ] **Step 3: Implement the parser**

```go
// shortcodes/parser.go
package shortcodes

import (
	"regexp"
	"strings"
)

// Shortcode represents a parsed shortcode occurrence in text.
type Shortcode struct {
	Name  string            // e.g., "meta" or "plugin:my-plugin:rating"
	Attrs map[string]string // e.g., {"path": "cooking.time", "editable": "true"}
	Raw   string            // original matched text including brackets
	Start int               // byte offset in input
	End   int               // byte offset end (exclusive)
}

// shortcodePattern matches [name ...attrs] where name is "meta" or "plugin:word:word".
// It captures the name and the attribute string separately.
var shortcodePattern = regexp.MustCompile(
	`\[(meta|plugin:[a-z][a-z0-9-]*:[a-z][a-z0-9-]*)\s*([^\]]*)\]`,
)

// attrPattern matches key="value", key='value', or key=value pairs.
var attrPattern = regexp.MustCompile(
	`([a-zA-Z][a-zA-Z0-9_-]*)=(?:"([^"]*)"|'([^']*)'|(\S+))`,
)

// Parse scans input for shortcode patterns and returns all matches.
// Only recognized shortcode names are matched: "meta" and "plugin:*:*".
// Unrecognized [bracket] patterns are left untouched.
func Parse(input string) []Shortcode {
	matches := shortcodePattern.FindAllStringSubmatchIndex(input, -1)
	if len(matches) == 0 {
		return nil
	}

	result := make([]Shortcode, 0, len(matches))
	for _, m := range matches {
		fullStart, fullEnd := m[0], m[1]
		name := input[m[2]:m[3]]
		attrStr := ""
		if m[4] >= 0 && m[5] >= 0 {
			attrStr = input[m[4]:m[5]]
		}

		attrs := parseAttrs(strings.TrimSpace(attrStr))

		result = append(result, Shortcode{
			Name:  name,
			Attrs: attrs,
			Raw:   input[fullStart:fullEnd],
			Start: fullStart,
			End:   fullEnd,
		})
	}

	return result
}

// parseAttrs extracts key=value pairs from an attribute string.
func parseAttrs(s string) map[string]string {
	attrs := make(map[string]string)
	if s == "" {
		return attrs
	}

	matches := attrPattern.FindAllStringSubmatch(s, -1)
	for _, m := range matches {
		key := m[1]
		// Value is in one of three capture groups: double-quoted, single-quoted, or unquoted.
		val := m[2]
		if val == "" {
			val = m[3]
		}
		if val == "" {
			val = m[4]
		}
		attrs[key] = val
	}

	return attrs
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./shortcodes/... -v -run TestParse`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add shortcodes/parser.go shortcodes/parser_test.go
git commit -m "feat(shortcodes): add shortcode syntax parser"
```

---

### Task 2: Built-in Meta Shortcode Handler

**Files:**
- Create: `shortcodes/meta_handler.go`
- Create: `shortcodes/meta_handler_test.go`

- [ ] **Step 1: Write failing tests for the meta handler**

```go
// shortcodes/meta_handler_test.go
package shortcodes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderMetaBasic(t *testing.T) {
	meta := map[string]any{"cooking": map[string]any{"time": 30}}
	metaJSON, _ := json.Marshal(meta)
	schema := `{"type":"object","properties":{"cooking":{"type":"object","properties":{"time":{"type":"integer","title":"Cooking Time"}}}}}`

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   42,
		Meta:       metaJSON,
		MetaSchema: schema,
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "cooking.time"},
	}, ctx)

	assert.Contains(t, result, `data-path="cooking.time"`)
	assert.Contains(t, result, `data-entity-type="group"`)
	assert.Contains(t, result, `data-entity-id="42"`)
	assert.Contains(t, result, `data-value="30"`)
	assert.Contains(t, result, `data-editable="false"`)
	assert.Contains(t, result, `data-hide-empty="false"`)
	// Schema slice for cooking.time should be just the integer schema
	assert.Contains(t, result, `"type":"integer"`)
	assert.Contains(t, result, `"title":"Cooking Time"`)
}

func TestRenderMetaEditable(t *testing.T) {
	meta := map[string]any{"name": "test"}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   7,
		Meta:       metaJSON,
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "name", "editable": "true"},
	}, ctx)

	assert.Contains(t, result, `data-editable="true"`)
	assert.Contains(t, result, `data-value="&#34;test&#34;"`)
}

func TestRenderMetaMissingPath(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{},
	}, ctx)

	// Missing required path attribute — return empty string
	assert.Equal(t, "", result)
}

func TestRenderMetaEmptyValue(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "nonexistent"},
	}, ctx)

	assert.Contains(t, result, `data-path="nonexistent"`)
	assert.Contains(t, result, `data-value=""`)
}

func TestRenderMetaHideEmpty(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "a", "hide-empty": "true"},
	}, ctx)

	assert.Contains(t, result, `data-hide-empty="true"`)
}

func TestRenderMetaObjectValue(t *testing.T) {
	meta := map[string]any{"loc": map[string]any{"lat": 1.5, "lng": 2.5}}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
		MetaSchema: "",
	}

	result := RenderMetaShortcode(Shortcode{
		Name:  "meta",
		Attrs: map[string]string{"path": "loc"},
	}, ctx)

	assert.Contains(t, result, `data-path="loc"`)
	// Value should be JSON-encoded object
	assert.Contains(t, result, `"lat"`)
}

func TestExtractSchemaSlice(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"object","properties":{"b":{"type":"string","title":"B Field"}}}}}`
	slice := extractSchemaSlice(schema, "a.b")
	require.NotEmpty(t, slice)
	var parsed map[string]any
	err := json.Unmarshal([]byte(slice), &parsed)
	require.NoError(t, err)
	assert.Equal(t, "string", parsed["type"])
	assert.Equal(t, "B Field", parsed["title"])
}

func TestExtractSchemaSliceNotFound(t *testing.T) {
	schema := `{"type":"object","properties":{"a":{"type":"string"}}}`
	slice := extractSchemaSlice(schema, "b.c")
	assert.Equal(t, "", slice)
}

func TestExtractSchemaSliceEmptySchema(t *testing.T) {
	slice := extractSchemaSlice("", "a.b")
	assert.Equal(t, "", slice)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./shortcodes/... -v -run "TestRenderMeta|TestExtractSchema"`
Expected: FAIL — functions not defined

- [ ] **Step 3: Implement the meta handler**

```go
// shortcodes/meta_handler.go
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

	// Extract the value at the given path from Meta
	valueJSON := extractValueAtPath(ctx.Meta, path)

	// Extract the schema slice for this path
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./shortcodes/... -v -run "TestRenderMeta|TestExtractSchema"`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add shortcodes/meta_handler.go shortcodes/meta_handler_test.go
git commit -m "feat(shortcodes): add built-in meta shortcode handler"
```

---

### Task 3: Shortcode Processor

**Files:**
- Create: `shortcodes/processor.go`
- Create: `shortcodes/processor_test.go`

- [ ] **Step 1: Write failing tests for the processor**

```go
// shortcodes/processor_test.go
package shortcodes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessNoShortcodes(t *testing.T) {
	result := Process("<div>hello</div>", MetaShortcodeContext{}, nil)
	assert.Equal(t, "<div>hello</div>", result)
}

func TestProcessMetaShortcode(t *testing.T) {
	meta := map[string]any{"name": "test"}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}

	result := Process(`before [meta path="name"] after`, ctx, nil)
	assert.Contains(t, result, "before ")
	assert.Contains(t, result, "<meta-shortcode")
	assert.Contains(t, result, " after")
	assert.NotContains(t, result, "[meta")
}

func TestProcessMixedHTMLAndShortcodes(t *testing.T) {
	meta := map[string]any{"a": 1, "b": 2}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}

	input := `<div class="flex gap-2">[meta path="a"]<span>sep</span>[meta path="b"]</div>`
	result := Process(input, ctx, nil)
	assert.Contains(t, result, `<div class="flex gap-2">`)
	assert.Contains(t, result, `<span>sep</span>`)
	assert.Contains(t, result, `data-path="a"`)
	assert.Contains(t, result, `data-path="b"`)
}

func TestProcessPluginShortcode(t *testing.T) {
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "<div>plugin output</div>", nil
	}

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}

	result := Process(`[plugin:test:widget size="large"]`, ctx, renderer)
	assert.Equal(t, "<div>plugin output</div>", result)
}

func TestProcessPluginShortcodeError(t *testing.T) {
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "", fmt.Errorf("render error")
	}

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}

	// On error, the original shortcode text is preserved
	result := Process(`[plugin:test:widget]`, ctx, renderer)
	assert.Equal(t, `[plugin:test:widget]`, result)
}
```

Add the `"fmt"` import to the test file.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./shortcodes/... -v -run TestProcess`
Expected: FAIL — `Process` not defined

- [ ] **Step 3: Implement the processor**

```go
// shortcodes/processor.go
package shortcodes

import (
	"strings"
)

// PluginRenderer is a callback that renders a plugin shortcode.
// It receives the plugin name (e.g., "test" from "plugin:test:widget"),
// the parsed shortcode, and the entity context.
// Returns rendered HTML or an error (in which case the original text is preserved).
type PluginRenderer func(pluginName string, sc Shortcode, ctx MetaShortcodeContext) (string, error)

// Process parses shortcodes in input and replaces them with rendered HTML.
// Built-in "meta" shortcodes are handled directly.
// Plugin shortcodes (starting with "plugin:") use the provided renderer callback.
// If renderer is nil, plugin shortcodes are left as-is.
func Process(input string, ctx MetaShortcodeContext, renderer PluginRenderer) string {
	shortcodes := Parse(input)
	if len(shortcodes) == 0 {
		return input
	}

	var b strings.Builder
	b.Grow(len(input) * 2)
	lastEnd := 0

	for _, sc := range shortcodes {
		// Write text before this shortcode
		b.WriteString(input[lastEnd:sc.Start])

		var replacement string

		if sc.Name == "meta" {
			replacement = RenderMetaShortcode(sc, ctx)
		} else if strings.HasPrefix(sc.Name, "plugin:") {
			if renderer != nil {
				// Extract plugin name: "plugin:my-plugin:type" -> "my-plugin"
				parts := strings.SplitN(sc.Name, ":", 3)
				if len(parts) == 3 {
					html, err := renderer(parts[1], sc, ctx)
					if err == nil {
						replacement = html
					} else {
						// On error, preserve original shortcode text
						replacement = sc.Raw
					}
				} else {
					replacement = sc.Raw
				}
			} else {
				replacement = sc.Raw
			}
		}

		b.WriteString(replacement)
		lastEnd = sc.End
	}

	// Write remaining text after last shortcode
	b.WriteString(input[lastEnd:])

	return b.String()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./shortcodes/... -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add shortcodes/processor.go shortcodes/processor_test.go
git commit -m "feat(shortcodes): add shortcode processor with plugin renderer support"
```

---

### Task 4: Pongo2 Tag — `{% process_shortcodes %}`

**Files:**
- Create: `server/template_handlers/template_filters/shortcode_tag.go`

This task depends on Tasks 1-3 (the shortcodes package).

- [ ] **Step 1: Implement the pongo2 tag**

The tag follows the `{% plugin_slot %}` pattern from `server/template_handlers/template_filters/plugin_slot.go`. It reads `_pluginManager` from the execution context and resolves entity type/ID/Meta/MetaSchema from the entity argument.

```go
// server/template_handlers/template_filters/shortcode_tag.go
package template_filters

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/flosch/pongo2/v4"
	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

type processShortcodesNode struct {
	contentExpr *pongo2.NodeExpression
	entityExpr  *pongo2.NodeExpression
}

func (node *processShortcodesNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	contentVal := node.contentExpr.Execute(ctx, ctx.Public)
	content := contentVal.String()
	if content == "" {
		return nil
	}

	entityVal := node.entityExpr.Execute(ctx, ctx.Public)
	entity := entityVal.Interface()
	if entity == nil {
		// No entity — just write content as-is
		_, _ = writer.WriteString(content)
		return nil
	}

	metaCtx := buildMetaContext(entity)
	if metaCtx == nil {
		_, _ = writer.WriteString(content)
		return nil
	}

	// Get plugin manager for plugin shortcode rendering
	var pluginRenderer shortcodes.PluginRenderer
	if pmVal, ok := ctx.Public["_pluginManager"]; ok && pmVal != nil {
		if pm, ok := pmVal.(*plugin_system.PluginManager); ok && pm != nil {
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs)
			}
		}
	}

	result := shortcodes.Process(content, *metaCtx, pluginRenderer)
	if _, err := writer.WriteString(result); err != nil {
		return ctx.Error(fmt.Sprintf("process_shortcodes: write error: %s", err), nil)
	}
	return nil
}

// buildMetaContext uses reflection to extract entity type, ID, Meta, and MetaSchema
// from Group, Resource, or Note model structs.
func buildMetaContext(entity any) *shortcodes.MetaShortcodeContext {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	// Extract ID
	idField := v.FieldByName("ID")
	if !idField.IsValid() || idField.Kind() != reflect.Uint {
		return nil
	}
	id := uint(idField.Uint())

	// Extract Meta
	var metaJSON json.RawMessage
	metaField := v.FieldByName("Meta")
	if metaField.IsValid() {
		if raw, err := json.Marshal(metaField.Interface()); err == nil {
			metaJSON = raw
		}
	}

	// Determine entity type and MetaSchema based on struct type
	typeName := v.Type().Name()
	var entityType, metaSchema string

	switch typeName {
	case "Group":
		entityType = "group"
		metaSchema = extractCategorySchema(v, "Category")
	case "Resource":
		entityType = "resource"
		metaSchema = extractCategorySchema(v, "ResourceCategory")
	case "Note":
		entityType = "note"
		metaSchema = extractCategorySchema(v, "NoteType")
	default:
		return nil
	}

	return &shortcodes.MetaShortcodeContext{
		EntityType: entityType,
		EntityID:   id,
		Meta:       metaJSON,
		MetaSchema: metaSchema,
	}
}

// extractCategorySchema reads the MetaSchema field from a preloaded category/type relation.
func extractCategorySchema(entityVal reflect.Value, fieldName string) string {
	catField := entityVal.FieldByName(fieldName)
	if !catField.IsValid() || catField.Kind() != reflect.Ptr || catField.IsNil() {
		return ""
	}
	catVal := catField.Elem()
	schemaField := catVal.FieldByName("MetaSchema")
	if !schemaField.IsValid() || schemaField.Kind() != reflect.String {
		return ""
	}
	return schemaField.String()
}

func processShortcodesTagParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	// Parse: {% process_shortcodes <contentExpr> <entityExpr> %}
	contentExpr, err := arguments.ParseExpression()
	if err != nil {
		return nil, err
	}

	entityExpr, err := arguments.ParseExpression()
	if err != nil {
		return nil, arguments.Error("process_shortcodes tag requires two arguments: content and entity", nil)
	}

	if remaining := arguments.Remaining(); len(remaining) > 0 {
		return nil, arguments.Error("process_shortcodes tag takes exactly two arguments", nil)
	}

	return &processShortcodesNode{
		contentExpr: contentExpr,
		entityExpr:  entityExpr,
	}, nil
}

func init() {
	if err := pongo2.RegisterTag("process_shortcodes", processShortcodesTagParser); err != nil {
		fmt.Println("error when registering process_shortcodes tag:", err)
	}
}
```

- [ ] **Step 2: Verify the tag compiles**

Run: `go build --tags 'json1 fts5' ./...`
Expected: Build succeeds. Note: this import needs the shortcodes package and `pm.RenderShortcode` (which doesn't exist yet). To avoid a compile error, temporarily comment out the plugin renderer block or stub the method. We'll complete it in Task 8.

**Temporary stub in `plugin_system/shortcodes.go`** (will be fully implemented in Task 8):

```go
// plugin_system/shortcodes.go
package plugin_system

import "encoding/json"

// RenderShortcode renders a plugin-registered shortcode. Stub for now.
func (pm *PluginManager) RenderShortcode(pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string) (string, error) {
	return "", fmt.Errorf("plugin shortcodes not yet implemented")
}
```

Add `"fmt"` import to the stub.

- [ ] **Step 3: Run build to verify compilation**

Run: `go build --tags 'json1 fts5' ./...`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add server/template_handlers/template_filters/shortcode_tag.go plugin_system/shortcodes.go
git commit -m "feat(shortcodes): add process_shortcodes pongo2 tag"
```

---

### Task 5: `editMeta` API Endpoint

**Files:**
- Create: `server/api_handlers/meta_edit_handler.go`
- Modify: `server/interfaces/generic_interfaces.go`
- Modify: `application_context/basic_entity_context.go`
- Modify: `server/routes.go`

- [ ] **Step 1: Write failing API test**

Create `server/api_tests/meta_edit_test.go`:

```go
// server/api_tests/meta_edit_test.go
package api_tests

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/types"
)

func TestEditMetaGroup_SimpleField(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{Name: "Test", Meta: types.JSON(`{"existing": "value"}`)}
	tc.DB.Create(group)

	formData := url.Values{}
	formData.Set("path", "cooking.time")
	formData.Set("value", "30")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group/editMeta?id="+fmt.Sprint(group.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	err := json.Unmarshal(resp.Body.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, true, result["ok"])

	// Verify meta was updated
	var updated models.Group
	tc.DB.First(&updated, group.ID)
	var meta map[string]any
	json.Unmarshal(updated.Meta, &meta)
	cooking, _ := meta["cooking"].(map[string]any)
	assert.Equal(t, float64(30), cooking["time"])
	// Existing fields preserved
	assert.Equal(t, "value", meta["existing"])
}

func TestEditMetaGroup_DeepPath(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{Name: "Test", Meta: types.JSON(`{}`)}
	tc.DB.Create(group)

	formData := url.Values{}
	formData.Set("path", "a.b.c.d")
	formData.Set("value", `"deep"`)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group/editMeta?id="+fmt.Sprint(group.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code)

	var updated models.Group
	tc.DB.First(&updated, group.ID)
	var meta map[string]any
	json.Unmarshal(updated.Meta, &meta)

	a, _ := meta["a"].(map[string]any)
	b, _ := a["b"].(map[string]any)
	c, _ := b["c"].(map[string]any)
	assert.Equal(t, "deep", c["d"])
}

func TestEditMetaGroup_PreservesExistingNested(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{Name: "Test", Meta: types.JSON(`{"cooking":{"difficulty":"easy"}}`)}
	tc.DB.Create(group)

	formData := url.Values{}
	formData.Set("path", "cooking.time")
	formData.Set("value", "45")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group/editMeta?id="+fmt.Sprint(group.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code)

	var updated models.Group
	tc.DB.First(&updated, group.ID)
	var meta map[string]any
	json.Unmarshal(updated.Meta, &meta)
	cooking, _ := meta["cooking"].(map[string]any)
	assert.Equal(t, "easy", cooking["difficulty"]) // preserved
	assert.Equal(t, float64(45), cooking["time"])  // added
}

func TestEditMetaGroup_MissingID(t *testing.T) {
	tc := SetupTestEnv(t)
	formData := url.Values{}
	formData.Set("path", "a")
	formData.Set("value", "1")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group/editMeta", formData)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestEditMetaGroup_MissingPath(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{Name: "Test"}
	tc.DB.Create(group)

	formData := url.Values{}
	formData.Set("value", "1")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group/editMeta?id="+fmt.Sprint(group.ID), formData)
	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestEditMetaResource(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	resource := &models.Resource{Name: "Test", Meta: types.JSON(`{}`)}
	tc.DB.Create(resource)

	formData := url.Values{}
	formData.Set("path", "rating")
	formData.Set("value", "5")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/resource/editMeta?id="+fmt.Sprint(resource.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestEditMetaNote(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	note := tc.CreateDummyNote("Test")

	formData := url.Values{}
	formData.Set("path", "status")
	formData.Set("value", `"done"`)

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/note/editMeta?id="+fmt.Sprint(note.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code)
}

func TestEditMetaReturnsFullMeta(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	group := &models.Group{Name: "Test", Meta: types.JSON(`{"a":1}`)}
	tc.DB.Create(group)

	formData := url.Values{}
	formData.Set("path", "b")
	formData.Set("value", "2")

	resp := tc.MakeFormRequest(http.MethodPost, "/v1/group/editMeta?id="+fmt.Sprint(group.ID), formData)
	assert.Equal(t, http.StatusOK, resp.Code)

	var result map[string]any
	json.Unmarshal(resp.Body.Bytes(), &result)
	meta, ok := result["meta"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, float64(1), meta["a"])
	assert.Equal(t, float64(2), meta["b"])
}
```

Add `"fmt"` to the imports.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -v -run TestEditMeta`
Expected: FAIL — handler doesn't exist, route not registered

- [ ] **Step 3: Add MetaEditor interface**

In `server/interfaces/generic_interfaces.go`, add:

```go
// MetaEditor provides per-path meta editing for an entity type.
type MetaEditor interface {
	UpdateMetaAtPath(id uint, path string, value json.RawMessage) (json.RawMessage, error)
}
```

Add `"encoding/json"` to imports.

- [ ] **Step 4: Add UpdateMetaAtPath to EntityWriter**

In `application_context/basic_entity_context.go`, add:

```go
// UpdateMetaAtPath sets a single meta field at the given dot-notation path
// using deep merge semantics. Returns the full updated Meta.
func (w *EntityWriter[T]) UpdateMetaAtPath(id uint, path string, value json.RawMessage) (json.RawMessage, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("path must not be empty")
	}

	// Build nested JSON object from path + value
	nested, err := buildNestedJSON(path, value)
	if err != nil {
		return nil, fmt.Errorf("invalid value: %w", err)
	}

	entity := new(T)

	// Determine table name
	stmt := &gorm.Statement{DB: w.ctx.db}
	_ = stmt.Parse(entity)

	var metaExpr clause.Expr
	if w.ctx.Config.DbType == constants.DbTypePosgres {
		metaExpr = gorm.Expr("COALESCE(meta, '{}'::jsonb) || ?::jsonb", nested)
	} else {
		metaExpr = gorm.Expr("json_patch(COALESCE(meta, '{}'), ?)", nested)
	}

	result := w.ctx.db.Model(entity).Where("id = ?", id).Update("meta", metaExpr)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}

	// Read back the full updated meta
	var metaRow struct {
		Meta json.RawMessage
	}
	if err := w.ctx.db.Table(stmt.Table).Select("meta").Where("id = ?", id).Scan(&metaRow).Error; err != nil {
		return nil, err
	}

	return metaRow.Meta, nil
}

// buildNestedJSON constructs a nested JSON object from a dot-notation path and value.
// Example: path="a.b.c", value=30 -> {"a":{"b":{"c":30}}}
func buildNestedJSON(path string, value json.RawMessage) (string, error) {
	parts := strings.Split(path, ".")

	// Validate the value is valid JSON
	if !json.Valid(value) {
		return "", fmt.Errorf("value is not valid JSON")
	}

	// Build from inside out
	result := string(value)
	for i := len(parts) - 1; i >= 0; i-- {
		keyJSON, _ := json.Marshal(parts[i])
		result = fmt.Sprintf("{%s:%s}", string(keyJSON), result)
	}

	return result, nil
}
```

Add these imports: `"encoding/json"`, `"fmt"`, `"mahresources/constants"`, `"gorm.io/gorm/clause"`.

- [ ] **Step 5: Create the editMeta handler**

```go
// server/api_handlers/meta_edit_handler.go
package api_handlers

import (
	"encoding/json"
	"fmt"
	"mahresources/constants"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
)

// GetEditMetaHandler returns a handler for POST /v1/{entity}/editMeta.
// It reads path and value from form data, performs a deep-merge at that path,
// and returns the full updated meta.
func GetEditMetaHandler(ctx interfaces.MetaEditor, name string) func(http.ResponseWriter, *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := http_utils.GetUIntQueryParameter(request, "id", 0)
		if id == 0 {
			http_utils.HandleError(fmt.Errorf("missing or invalid %s ID", name), writer, request, http.StatusBadRequest)
			return
		}

		if err := request.ParseForm(); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		path := request.FormValue("path")
		if path == "" {
			http_utils.HandleError(fmt.Errorf("missing required field 'path'"), writer, request, http.StatusBadRequest)
			return
		}

		valueStr := request.FormValue("value")
		if valueStr == "" {
			http_utils.HandleError(fmt.Errorf("missing required field 'value'"), writer, request, http.StatusBadRequest)
			return
		}

		updatedMeta, err := ctx.UpdateMetaAtPath(id, path, json.RawMessage(valueStr))
		if err != nil {
			http_utils.HandleError(err, writer, request, statusCodeForError(err, http.StatusBadRequest))
			return
		}

		// Parse meta into a map for the response
		var metaMap any
		if err := json.Unmarshal(updatedMeta, &metaMap); err != nil {
			metaMap = string(updatedMeta)
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"ok":   true,
			"id":   id,
			"meta": metaMap,
		})
	}
}
```

- [ ] **Step 6: Register routes in `server/routes.go`**

After the existing `editDescription` routes for each entity type, add `editMeta` routes. Find the line with `router.Methods(http.MethodPost).Path("/v1/group/editDescription")` (around line 243) and add after it:

```go
router.Methods(http.MethodPost).Path("/v1/group/editMeta").HandlerFunc(api_handlers.GetEditMetaHandler(basicGroupWriter, "group"))
```

Similarly for notes (after line 196):
```go
router.Methods(http.MethodPost).Path("/v1/note/editMeta").HandlerFunc(api_handlers.GetEditMetaHandler(basicNoteWriter, "note"))
```

And for resources (find the resource editDescription line and add after it):
```go
router.Methods(http.MethodPost).Path("/v1/resource/editMeta").HandlerFunc(api_handlers.GetEditMetaHandler(basicResourceWriter, "resource"))
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./server/api_tests/... -v -run TestEditMeta`
Expected: All PASS

- [ ] **Step 8: Run existing tests to verify nothing broke**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All existing tests still pass

- [ ] **Step 9: Commit**

```bash
git add server/api_handlers/meta_edit_handler.go server/interfaces/generic_interfaces.go application_context/basic_entity_context.go server/routes.go
git commit -m "feat(shortcodes): add editMeta API endpoint with deep-merge-by-path"
```

---

### Task 6: `<meta-shortcode>` Web Component — Display Mode

**Files:**
- Create: `src/webcomponents/meta-shortcode.ts`
- Modify: `src/main.js`

- [ ] **Step 1: Create the web component with display mode**

```typescript
// src/webcomponents/meta-shortcode.ts
import { LitElement, html, nothing, type TemplateResult } from 'lit';
import { customElement, property, state } from 'lit/decorators.js';
import { detectShape, getBuiltinRenderer } from '../schema-editor/display-renderers';
import type { JSONSchema } from '../schema-editor/schema-core';
import { titleCase } from '../schema-editor/schema-core';

@customElement('meta-shortcode')
export class MetaShortcode extends LitElement {
  @property({ attribute: 'data-path' }) path = '';
  @property({ attribute: 'data-editable' }) editable = 'false';
  @property({ attribute: 'data-hide-empty' }) hideEmpty = 'false';
  @property({ attribute: 'data-entity-type' }) entityType = '';
  @property({ attribute: 'data-entity-id' }) entityId = '';
  @property({ attribute: 'data-schema' }) schemaStr = '';
  @property({ attribute: 'data-value' }) valueStr = '';

  @state() private _editing = false;
  @state() private _saving = false;
  @state() private _currentValue: any = undefined;
  @state() private _flash: 'success' | 'error' | null = null;
  @state() private _pluginHtml: string | null = null;
  @state() private _pluginError = false;

  // Light DOM to inherit Tailwind styles
  override createRenderRoot() {
    return this;
  }

  private get _schema(): JSONSchema | null {
    if (!this.schemaStr) return null;
    try {
      return JSON.parse(this.schemaStr);
    } catch {
      return null;
    }
  }

  private get _value(): any {
    if (this._currentValue !== undefined) return this._currentValue;
    if (!this.valueStr) return undefined;
    try {
      return JSON.parse(this.valueStr);
    } catch {
      return this.valueStr;
    }
  }

  private get _isEmpty(): boolean {
    const v = this._value;
    return v === undefined || v === null || (typeof v === 'string' && v.trim() === '');
  }

  private get _label(): string {
    const schema = this._schema;
    if (schema?.title) return schema.title;
    // Use last path segment in title case
    const parts = this.path.split('.');
    return titleCase(parts[parts.length - 1]);
  }

  private get _isEditable(): boolean {
    return this.editable === 'true';
  }

  override render(): TemplateResult | typeof nothing {
    if (this._isEmpty && this.hideEmpty === 'true' && !this._editing) {
      return nothing;
    }

    const flashClass = this._flash === 'success'
      ? 'bg-green-100 transition-colors duration-300'
      : this._flash === 'error'
        ? 'bg-red-100 transition-colors duration-300'
        : '';

    return html`
      <span class="meta-shortcode inline-flex items-center gap-1 ${flashClass}">
        ${this._editing ? this._renderEditMode() : this._renderDisplayMode()}
      </span>
    `;
  }

  private _renderDisplayMode(): TemplateResult {
    const value = this._value;
    const schema = this._schema;
    const editButton = this._isEditable
      ? html`<button
          type="button"
          class="inline-flex items-center p-0.5 border-0 bg-transparent cursor-pointer"
          aria-label="Edit ${this._label}"
          @click=${this._enterEditMode}
        >${this._pencilIcon()}</button>`
      : nothing;

    if (this._isEmpty) {
      return html`
        <span class="text-stone-400 text-sm">${this._label}: —</span>
        ${editButton}
      `;
    }

    return html`
      <span class="text-sm">${this._renderValue(value, schema)}</span>
      ${editButton}
    `;
  }

  private _renderValue(value: any, schema: JSONSchema | null): TemplateResult | string {
    // Check for x-display plugin renderer
    const xDisplay = schema?.['x-display'] as string | undefined;
    if (xDisplay?.startsWith('plugin:')) {
      return this._renderPluginDisplay(value, xDisplay);
    }

    // Check for forced built-in renderer
    if (xDisplay) {
      const renderer = getBuiltinRenderer(xDisplay);
      if (renderer) {
        return html`<span>${renderer.render(value)}</span>`;
      }
    }

    // Auto shape detection for objects
    if (value != null && typeof value === 'object' && !Array.isArray(value)) {
      const shape = detectShape(value);
      if (shape) {
        return html`<span>${shape.render(value)}</span>`;
      }
    }

    // Type-based rendering
    if (schema) {
      const type = Array.isArray(schema.type)
        ? schema.type.find((t: string) => t !== 'null') || 'string'
        : schema.type || 'string';

      if (type === 'boolean') return value ? 'Yes' : 'No';
      if (type === 'integer' || type === 'number') {
        return html`<span class="font-mono">${value}</span>`;
      }
    }

    // Fallback: primitive as string, object as JSON
    if (typeof value === 'object') {
      return JSON.stringify(value);
    }
    return String(value);
  }

  private _renderPluginDisplay(value: any, xDisplay: string): TemplateResult {
    if (this._pluginHtml !== null) {
      const wrapper = document.createElement('span');
      wrapper.innerHTML = this._pluginHtml;
      return html`${wrapper}`;
    }
    if (this._pluginError) {
      return html`<span class="text-stone-400 text-xs italic">Render error</span>`;
    }

    // Fetch from plugin
    const parts = xDisplay.split(':');
    if (parts.length >= 3) {
      this._fetchPluginDisplay(parts[1], parts[2], value);
    }
    return html`<span class="text-stone-400 text-xs animate-pulse">Loading...</span>`;
  }

  private async _fetchPluginDisplay(pluginName: string, typeName: string, value: any) {
    try {
      const resp = await fetch(`/v1/plugins/${pluginName}/display/render`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          type: typeName,
          value,
          schema: this._schema || {},
          field_path: this.path,
          field_label: this._label,
        }),
      });
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      this._pluginHtml = await resp.text();
    } catch {
      this._pluginError = true;
    }
  }

  private _renderEditMode(): TemplateResult {
    const schema = this._schema;
    const value = this._value;

    return html`
      <div class="meta-shortcode-edit border border-stone-300 rounded p-2 my-1">
        ${schema
          ? html`<schema-editor
              mode="form"
              .schema=${JSON.stringify(schema)}
              .value=${JSON.stringify(value ?? this._defaultValue(schema))}
              name="_meta_shortcode_value"
              @value-change=${this._onFormValueChange}
            ></schema-editor>`
          : html`<input
              type="text"
              class="border border-stone-300 rounded px-2 py-1 text-sm w-full"
              .value=${value != null ? (typeof value === 'object' ? JSON.stringify(value) : String(value)) : ''}
              @input=${this._onInputChange}
            />`
        }
        <div class="flex gap-2 mt-2">
          <button
            type="button"
            class="px-3 py-1 text-sm bg-indigo-600 text-white rounded hover:bg-indigo-700 disabled:opacity-50"
            ?disabled=${this._saving}
            @click=${this._save}
          >${this._saving ? 'Saving...' : 'Save'}</button>
          <button
            type="button"
            class="px-3 py-1 text-sm bg-stone-200 text-stone-700 rounded hover:bg-stone-300"
            ?disabled=${this._saving}
            @click=${this._cancelEdit}
          >Cancel</button>
        </div>
      </div>
    `;
  }

  private _editValue: any = undefined;

  private _onFormValueChange(e: CustomEvent) {
    this._editValue = e.detail.value;
  }

  private _onInputChange(e: Event) {
    const input = e.target as HTMLInputElement;
    // Try parsing as JSON, fall back to string
    try {
      this._editValue = JSON.parse(input.value);
    } catch {
      this._editValue = input.value;
    }
  }

  private _defaultValue(schema: JSONSchema): any {
    const type = schema.type;
    if (type === 'object') return {};
    if (type === 'array') return [];
    if (type === 'string') return '';
    if (type === 'number' || type === 'integer') return 0;
    if (type === 'boolean') return false;
    return null;
  }

  private _enterEditMode() {
    this._editValue = this._value;
    this._editing = true;
  }

  private _cancelEdit() {
    this._editing = false;
    this._editValue = undefined;
  }

  private async _save() {
    this._saving = true;

    const value = this._editValue !== undefined ? this._editValue : this._value;
    const valueJSON = JSON.stringify(value);

    const formData = new FormData();
    formData.append('path', this.path);
    formData.append('value', valueJSON);

    try {
      const resp = await fetch(
        `/v1/${this.entityType}/editMeta?id=${this.entityId}`,
        { method: 'POST', body: formData }
      );

      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);

      const result = await resp.json();
      // Update our value from the returned full meta
      if (result.meta) {
        const parts = this.path.split('.');
        let current: any = result.meta;
        for (const part of parts) {
          if (current == null || typeof current !== 'object') {
            current = undefined;
            break;
          }
          current = current[part];
        }
        this._currentValue = current;
        this.valueStr = current !== undefined ? JSON.stringify(current) : '';
      }

      this._editing = false;
      this._flash = 'success';
      setTimeout(() => { this._flash = null; }, 1000);
    } catch (err) {
      console.error('Meta shortcode save error:', err);
      this._flash = 'error';
      setTimeout(() => { this._flash = null; }, 1000);
    } finally {
      this._saving = false;
    }
  }

  private _pencilIcon(): TemplateResult {
    return html`
      <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="text-stone-400 hover:text-stone-600">
        <path d="M17 3a2.85 2.83 0 1 1 4 4L7.5 20.5 2 22l1.5-5.5Z"/>
        <path d="m15 5 4 4"/>
      </svg>
    `;
  }
}
```

- [ ] **Step 2: Add import to main.js**

In `src/main.js`, add alongside the existing web component imports (around line 83-85):

```javascript
import './webcomponents/meta-shortcode.ts';
```

- [ ] **Step 3: Build to verify compilation**

Run: `npm run build-js`
Expected: Build succeeds

- [ ] **Step 4: Commit**

```bash
git add src/webcomponents/meta-shortcode.ts src/main.js
git commit -m "feat(shortcodes): add meta-shortcode web component"
```

---

### Task 7: Template Updates

**Files:**
- Modify: `templates/displayGroup.tpl`
- Modify: `templates/displayResource.tpl`
- Modify: `templates/displayNote.tpl`
- Modify: `templates/displayNoteText.tpl`
- Modify: `templates/partials/group.tpl`
- Modify: `templates/partials/resource.tpl`
- Modify: `templates/partials/note.tpl`

- [ ] **Step 1: Update displayGroup.tpl**

Replace lines 6-9:
```html
<!-- Before -->
    <div x-data="{ entity: {{ group|json }} }" ...>
        {% autoescape off %}
            {{ group.Category.CustomHeader }}
        {% endautoescape %}
    </div>

<!-- After -->
    <div x-data="{ entity: {{ group|json }} }" ...>
        {% process_shortcodes group.Category.CustomHeader group %}
    </div>
```

Replace the CustomSidebar section (around line 58-62):
```html
<!-- Before -->
            {% autoescape off %} {# KAN-6: by design — ... #}
                {{ group.Category.CustomSidebar }}
            {% endautoescape %}

<!-- After -->
            {% process_shortcodes group.Category.CustomSidebar group %}
```

- [ ] **Step 2: Update displayResource.tpl**

Replace CustomHeader (lines 6-9):
```html
<!-- Before -->
        {% autoescape off %}
            {{ resource.ResourceCategory.CustomHeader }}
        {% endautoescape %}

<!-- After -->
        {% process_shortcodes resource.ResourceCategory.CustomHeader resource %}
```

Replace CustomSidebar (around line 234-238):
```html
<!-- Before -->
            {% autoescape off %} {# KAN-6: ... #}
                {{ resource.ResourceCategory.CustomSidebar }}
            {% endautoescape %}

<!-- After -->
            {% process_shortcodes resource.ResourceCategory.CustomSidebar resource %}
```

- [ ] **Step 3: Update displayNote.tpl**

Replace CustomHeader (around line 6-9):
```html
<!-- Before -->
        {% autoescape off %}
            {{ note.NoteType.CustomHeader }}
        {% endautoescape %}

<!-- After -->
        {% process_shortcodes note.NoteType.CustomHeader note %}
```

Replace CustomSidebar (around line 46-50):
```html
<!-- Before -->
            {% autoescape off %} {# KAN-6: ... #}
                {{ note.NoteType.CustomSidebar }}
            {% endautoescape %}

<!-- After -->
            {% process_shortcodes note.NoteType.CustomSidebar note %}
```

- [ ] **Step 4: Update displayNoteText.tpl**

Replace CustomSidebar (around line 23-27):
```html
<!-- Before -->
            {% autoescape off %} {# KAN-6: ... #}
                {{ note.NoteType.CustomSidebar }}
            {% endautoescape %}

<!-- After -->
            {% process_shortcodes note.NoteType.CustomSidebar note %}
```

- [ ] **Step 5: Update partials/group.tpl**

Replace CustomSummary (around line 58-62):
```html
<!-- Before -->
        {% autoescape off %}
            {{ entity.Category.CustomSummary }}
        {% endautoescape %}

<!-- After -->
        {% process_shortcodes entity.Category.CustomSummary entity %}
```

- [ ] **Step 6: Update partials/resource.tpl**

Replace CustomAvatar (around line 37):
```html
<!-- Before -->
                        {% autoescape off %}{{ entity.ResourceCategory.CustomAvatar }}{% endautoescape %}

<!-- After -->
                        {% process_shortcodes entity.ResourceCategory.CustomAvatar entity %}
```

Replace CustomSummary (around line 44-48):
```html
<!-- Before -->
            {% autoescape off %}
                {{ entity.ResourceCategory.CustomSummary }}
            {% endautoescape %}

<!-- After -->
            {% process_shortcodes entity.ResourceCategory.CustomSummary entity %}
```

- [ ] **Step 7: Update partials/note.tpl**

Replace CustomAvatar (around line 7):
```html
<!-- Before -->
            {% autoescape off %}{{ entity.NoteType.CustomAvatar }}{% endautoescape %}

<!-- After -->
            {% process_shortcodes entity.NoteType.CustomAvatar entity %}
```

Replace CustomSummary (around line 15-19):
```html
<!-- Before -->
            {% autoescape off %}
                {{ entity.NoteType.CustomSummary }}
            {% endautoescape %}

<!-- After -->
            {% process_shortcodes entity.NoteType.CustomSummary entity %}
```

- [ ] **Step 8: Build and verify**

Run: `npm run build && go build --tags 'json1 fts5'`
Expected: Build succeeds

- [ ] **Step 9: Run existing Go tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass

- [ ] **Step 10: Commit**

```bash
git add templates/displayGroup.tpl templates/displayResource.tpl templates/displayNote.tpl templates/displayNoteText.tpl templates/partials/group.tpl templates/partials/resource.tpl templates/partials/note.tpl
git commit -m "feat(shortcodes): update templates to use process_shortcodes tag"
```

---

### Task 8: Plugin `mah.shortcode()` API

**Files:**
- Modify: `plugin_system/shortcodes.go` (replace stub from Task 4)
- Create: `plugin_system/shortcodes_test.go`
- Modify: `plugin_system/manager.go`

- [ ] **Step 1: Write failing tests for plugin shortcode registration and rendering**

```go
// plugin_system/shortcodes_test.go
package plugin_system

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShortcodeRegistration(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-test", `
		plugin = { name = "sc-test", version = "1.0" }
		function init()
			mah.shortcode({
				name = "greeting",
				label = "Greeting",
				render = function(ctx)
					return "<span>Hello from plugin!</span>"
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-test"))

	// Verify shortcode was registered
	sc := pm.GetPluginShortcode("plugin:sc-test:greeting")
	require.NotNil(t, sc)
	assert.Equal(t, "Greeting", sc.Label)
	assert.Equal(t, "sc-test", sc.PluginName)
}

func TestShortcodeRendering(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-render", `
		plugin = { name = "sc-render", version = "1.0" }
		function init()
			mah.shortcode({
				name = "stars",
				label = "Star Rating",
				render = function(ctx)
					local max = tonumber(ctx.attrs.max) or 5
					local stars = ""
					for i = 1, max do stars = stars .. "★" end
					return "<span>" .. stars .. "</span>"
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-render"))

	html, err := pm.RenderShortcode(
		"sc-render",
		"plugin:sc-render:stars",
		"group", 1,
		json.RawMessage(`{"rating": 4}`),
		map[string]string{"max": "3"},
	)
	require.NoError(t, err)
	assert.Equal(t, "<span>★★★</span>", html)
}

func TestShortcodeRenderContext(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-ctx", `
		plugin = { name = "sc-ctx", version = "1.0" }
		function init()
			mah.shortcode({
				name = "info",
				label = "Info",
				render = function(ctx)
					return ctx.entity_type .. ":" .. tostring(ctx.entity_id)
				end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-ctx"))

	html, err := pm.RenderShortcode(
		"sc-ctx",
		"plugin:sc-ctx:info",
		"resource", 42,
		json.RawMessage(`{}`),
		map[string]string{},
	)
	require.NoError(t, err)
	assert.Equal(t, "resource:42", html)
}

func TestShortcodeDuplicate(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-dup", `
		plugin = { name = "sc-dup", version = "1.0" }
		function init()
			mah.shortcode({
				name = "test",
				label = "Test",
				render = function(ctx) return "a" end
			})
			mah.shortcode({
				name = "test",
				label = "Test2",
				render = function(ctx) return "b" end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("sc-dup")
	assert.Error(t, err) // Should fail on duplicate
}

func TestShortcodeInvalidName(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-bad", `
		plugin = { name = "sc-bad", version = "1.0" }
		function init()
			mah.shortcode({
				name = "INVALID",
				label = "Bad",
				render = function(ctx) return "" end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	err = pm.EnablePlugin("sc-bad")
	assert.Error(t, err)
}

func TestShortcodeCleanupOnDisable(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "sc-cleanup", `
		plugin = { name = "sc-cleanup", version = "1.0" }
		function init()
			mah.shortcode({
				name = "temp",
				label = "Temp",
				render = function(ctx) return "temp" end
			})
		end
	`)

	pm, err := NewPluginManager(dir)
	require.NoError(t, err)
	defer pm.Close()

	require.NoError(t, pm.EnablePlugin("sc-cleanup"))
	assert.NotNil(t, pm.GetPluginShortcode("plugin:sc-cleanup:temp"))

	require.NoError(t, pm.DisablePlugin("sc-cleanup"))
	assert.Nil(t, pm.GetPluginShortcode("plugin:sc-cleanup:temp"))
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./plugin_system/... -v -run TestShortcode`
Expected: FAIL — `mah.shortcode` not registered, methods missing

- [ ] **Step 3: Implement the full shortcodes.go**

Replace the stub from Task 4:

```go
// plugin_system/shortcodes.go
package plugin_system

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const luaShortcodeRenderTimeout = 5 * time.Second

// PluginShortcode represents a plugin-registered shortcode.
type PluginShortcode struct {
	PluginName string
	TypeName   string         // full: plugin:<pluginName>:<name>
	Label      string
	Render     *lua.LFunction
	State      *lua.LState
}

var validShortcodeName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,49}$`)

// parseShortcodeTable parses a Lua table into a PluginShortcode.
func parseShortcodeTable(L *lua.LState, tbl *lua.LTable, pluginName string) (*PluginShortcode, error) {
	sc := &PluginShortcode{
		PluginName: pluginName,
	}

	if v := tbl.RawGetString("name"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'name'")
	} else if str, ok := v.(lua.LString); !ok {
		return nil, fmt.Errorf("'name' must be a string, got %s", v.Type())
	} else {
		raw := string(str)
		if !validShortcodeName.MatchString(raw) {
			return nil, fmt.Errorf("invalid shortcode name %q: must match [a-z][a-z0-9-]{0,49}", raw)
		}
		sc.TypeName = "plugin:" + pluginName + ":" + raw
	}

	if v := tbl.RawGetString("label"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'label'")
	} else {
		sc.Label = v.String()
	}

	if v := tbl.RawGetString("render"); v == lua.LNil {
		return nil, fmt.Errorf("missing required field 'render'")
	} else if fn, ok := v.(*lua.LFunction); !ok {
		return nil, fmt.Errorf("'render' must be a function")
	} else {
		sc.Render = fn
	}

	return sc, nil
}

// GetPluginShortcode returns a specific plugin shortcode by full type name, or nil.
func (pm *PluginManager) GetPluginShortcode(fullTypeName string) *PluginShortcode {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	for _, scs := range pm.shortcodes {
		for _, sc := range scs {
			if sc.TypeName == fullTypeName {
				return sc
			}
		}
	}
	return nil
}

// RenderShortcode executes the Lua render function for a plugin shortcode.
func (pm *PluginManager) RenderShortcode(pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string) (string, error) {
	if pm.closed.Load() {
		return "", fmt.Errorf("plugin manager is closed")
	}

	sc := pm.GetPluginShortcode(fullTypeName)
	if sc == nil {
		return "", fmt.Errorf("shortcode %q not found", fullTypeName)
	}
	if sc.PluginName != pluginName {
		return "", fmt.Errorf("shortcode %q does not belong to plugin %q", fullTypeName, pluginName)
	}

	fn := sc.Render
	if fn == nil {
		return "", fmt.Errorf("no render function for shortcode %q", fullTypeName)
	}

	L := sc.State
	mu := pm.VMLock(L)
	if mu == nil {
		return "", fmt.Errorf("plugin %q is no longer available", pluginName)
	}
	mu.Lock()
	defer mu.Unlock()

	// Build context table
	var metaMap map[string]any
	if len(meta) > 0 {
		_ = json.Unmarshal(meta, &metaMap)
	}
	if metaMap == nil {
		metaMap = map[string]any{}
	}

	attrsMap := make(map[string]any, len(attrs))
	for k, v := range attrs {
		attrsMap[k] = v
	}

	// Fetch plugin settings
	settings := pm.GetPluginSettings(pluginName)
	if settings == nil {
		settings = map[string]any{}
	}

	ctxData := map[string]any{
		"entity_type": entityType,
		"entity_id":   float64(entityID),
		"value":       metaMap,
		"attrs":       attrsMap,
		"settings":    settings,
	}

	tbl := goToLuaTable(L, ctxData)

	timeoutCtx, cancel := context.WithTimeout(context.Background(), luaShortcodeRenderTimeout)
	L.SetContext(timeoutCtx)

	err := L.CallByParam(lua.P{
		Fn:      fn,
		NRet:    1,
		Protect: true,
	}, tbl)

	L.RemoveContext()
	cancel()

	if err != nil {
		log.Printf("[plugin] warning: shortcode render %q/%q returned error: %v", pluginName, fullTypeName, err)
		return "", fmt.Errorf("shortcode render error: %w", err)
	}

	ret := L.Get(-1)
	L.Pop(1)

	if str, ok := ret.(lua.LString); ok {
		return string(str), nil
	}

	return "", nil
}
```

- [ ] **Step 4: Add shortcodes field and Lua registration to manager.go**

Add `shortcodes` field to the `PluginManager` struct (after line 89):

```go
shortcodes   map[string][]*PluginShortcode    // pluginName -> shortcodes
```

Initialize it in `NewPluginManager` (after line 134):

```go
shortcodes:      make(map[string][]*PluginShortcode),
```

Add `mah.shortcode()` Lua function registration (after the `display_type` block, around line 519):

```go
mahMod.RawSetString("shortcode", L.NewFunction(func(L *lua.LState) int {
	tbl := L.CheckTable(1)
	sc, err := parseShortcodeTable(L, tbl, *pluginNamePtr)
	if err != nil {
		L.ArgError(1, err.Error())
		return 0
	}
	sc.State = L

	pm.mu.Lock()
	for _, existing := range pm.shortcodes[*pluginNamePtr] {
		if existing.TypeName == sc.TypeName {
			pm.mu.Unlock()
			L.ArgError(1, fmt.Sprintf("duplicate shortcode %q", sc.TypeName))
			return 0
		}
	}
	pm.shortcodes[*pluginNamePtr] = append(pm.shortcodes[*pluginNamePtr], sc)
	pm.mu.Unlock()
	return 0
}))
```

Add cleanup in `DisablePlugin` (after line 836 `delete(pm.displayTypes, name)`):

```go
delete(pm.shortcodes, name)
```

Add cleanup in `Close` (after line 1010 `pm.displayTypes = nil`):

```go
pm.shortcodes = nil
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./plugin_system/... -v -run TestShortcode`
Expected: All PASS

- [ ] **Step 6: Run all plugin system tests**

Run: `go test ./plugin_system/... -v`
Expected: All pass

- [ ] **Step 7: Commit**

```bash
git add plugin_system/shortcodes.go plugin_system/shortcodes_test.go plugin_system/manager.go
git commit -m "feat(shortcodes): add mah.shortcode() plugin API"
```

---

### Task 9: E2E Tests

**Files:**
- Create: `e2e/tests/shortcodes.spec.ts`
- Modify: `e2e/helpers/api-client.ts`

- [ ] **Step 1: Add helper methods to ApiClient**

In `e2e/helpers/api-client.ts`, add methods for creating categories with MetaSchema and groups with Meta:

```typescript
async createCategoryWithSchema(name: string, description: string, metaSchema: object): Promise<Category> {
  const formData = new URLSearchParams();
  formData.append('Name', name);
  formData.append('Description', description);
  formData.append('MetaSchema', JSON.stringify(metaSchema));

  return this.postRetry<Category>(`${this.baseUrl}/v1/category`, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    data: formData.toString(),
  });
}

async createGroupWithMeta(opts: { name: string; categoryId: number; meta: object }): Promise<Group> {
  const formData = new URLSearchParams();
  formData.append('Name', opts.name);
  formData.append('CategoryId', String(opts.categoryId));
  formData.append('Meta', JSON.stringify(opts.meta));

  return this.postRetry<Group>(`${this.baseUrl}/v1/group`, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    data: formData.toString(),
  });
}

async updateCategoryCustomFields(id: number, fields: {
  customHeader?: string;
  customSidebar?: string;
  customSummary?: string;
}): Promise<Category> {
  const formData = new URLSearchParams();
  formData.append('ID', String(id));
  if (fields.customHeader !== undefined) formData.append('CustomHeader', fields.customHeader);
  if (fields.customSidebar !== undefined) formData.append('CustomSidebar', fields.customSidebar);
  if (fields.customSummary !== undefined) formData.append('CustomSummary', fields.customSummary);

  return this.postRetry<Category>(`${this.baseUrl}/v1/category`, {
    headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
    data: formData.toString(),
  });
}
```

- [ ] **Step 2: Write E2E tests**

```typescript
// e2e/tests/shortcodes.spec.ts
import { test, expect } from '../fixtures/base.fixture';

test.describe('Shortcode system', () => {
  let categoryId: number;
  let groupId: number;

  const metaSchema = {
    type: 'object',
    properties: {
      cooking: {
        type: 'object',
        properties: {
          time: { type: 'integer', title: 'Cooking Time (min)' },
          difficulty: { type: 'string', title: 'Difficulty', enum: ['easy', 'medium', 'hard'] },
        },
      },
    },
  };

  test.beforeAll(async ({ apiClient }) => {
    const category = await apiClient.createCategoryWithSchema(
      'Recipe',
      'A recipe category',
      metaSchema
    );
    categoryId = category.ID;

    // Set shortcode in CustomSidebar
    await apiClient.updateCategoryCustomFields(categoryId, {
      customSidebar: '[meta path="cooking.time"] [meta path="cooking.difficulty"]',
      customSummary: '[meta path="cooking.time" hide-empty=true]',
    });

    const group = await apiClient.createGroupWithMeta({
      name: 'Pasta Recipe',
      categoryId,
      meta: { cooking: { time: 30, difficulty: 'easy' } },
    });
    groupId = group.ID;
  });

  test('meta shortcode renders value in sidebar', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // The meta-shortcode element should be present and display the value
    const timeShortcode = page.locator('meta-shortcode[data-path="cooking.time"]').first();
    await expect(timeShortcode).toBeVisible({ timeout: 5000 });
    await expect(timeShortcode).toContainText('30');
  });

  test('meta shortcode renders difficulty value', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const difficultyShortcode = page.locator('meta-shortcode[data-path="cooking.difficulty"]').first();
    await expect(difficultyShortcode).toBeVisible({ timeout: 5000 });
    await expect(difficultyShortcode).toContainText('easy');
  });

  test('meta shortcode with hide-empty hides when value is absent', async ({ apiClient, page }) => {
    // Create a group with no cooking.time
    const emptyGroup = await apiClient.createGroupWithMeta({
      name: 'Empty Recipe',
      categoryId,
      meta: {},
    });

    await page.goto(`/groups`);
    await page.waitForLoadState('load');

    // In the list, the CustomSummary has [meta path="cooking.time" hide-empty=true]
    // For the empty group, nothing should render
    const card = page.locator(`a[href="/group?id=${emptyGroup.ID}"]`).first();
    await expect(card).toBeVisible({ timeout: 5000 });

    // The meta-shortcode for cooking.time should not be visible (hide-empty=true)
    const shortcode = card.locator('meta-shortcode[data-path="cooking.time"]');
    await expect(shortcode).toHaveCount(0);

    // Cleanup
    await apiClient.deleteGroup(emptyGroup.ID);
  });

  test('editable meta shortcode shows pencil and allows editing', async ({ apiClient, page }) => {
    // Update category to make cooking.time editable
    await apiClient.updateCategoryCustomFields(categoryId, {
      customSidebar: '[meta path="cooking.time" editable=true]',
    });

    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    const shortcode = page.locator('meta-shortcode[data-path="cooking.time"]').first();
    await expect(shortcode).toBeVisible({ timeout: 5000 });

    // Pencil button should be present
    const editButton = shortcode.locator('button[aria-label*="Edit"]');
    await expect(editButton).toBeVisible();

    // Click pencil to enter edit mode
    await editButton.click();

    // Form should appear
    const saveButton = shortcode.locator('button:has-text("Save")');
    await expect(saveButton).toBeVisible({ timeout: 3000 });

    // Cancel to return to display mode
    const cancelButton = shortcode.locator('button:has-text("Cancel")');
    await cancelButton.click();

    // Back to display mode
    await expect(saveButton).not.toBeVisible();
    await expect(shortcode).toContainText('30');
  });

  test('editMeta API creates deep path', async ({ apiClient, page }) => {
    // Create a group with empty meta
    const group = await apiClient.createGroupWithMeta({
      name: 'Deep Path Test',
      categoryId,
      meta: {},
    });

    // Call editMeta directly
    const formData = new URLSearchParams();
    formData.append('path', 'cooking.time');
    formData.append('value', '45');

    const response = await apiClient.request.post(
      `${(apiClient as any).baseUrl}/v1/group/editMeta?id=${group.ID}`,
      {
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        data: formData.toString(),
      }
    );
    expect(response.ok()).toBeTruthy();

    const result = await response.json();
    expect(result.ok).toBe(true);
    expect(result.meta.cooking.time).toBe(45);

    await apiClient.deleteGroup(group.ID);
  });

  test('regular HTML in custom fields still works alongside shortcodes', async ({ apiClient, page }) => {
    await apiClient.updateCategoryCustomFields(categoryId, {
      customSidebar: '<strong>Info:</strong> [meta path="cooking.time"]',
    });

    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // Both HTML and shortcode should render
    const sidebar = page.locator('.sidebar-group').first();
    await expect(sidebar.locator('strong')).toContainText('Info:');
    await expect(sidebar.locator('meta-shortcode')).toBeVisible();
  });

  test.afterAll(async ({ apiClient }) => {
    if (groupId) await apiClient.deleteGroup(groupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });
});
```

- [ ] **Step 3: Run full build**

Run: `npm run build`
Expected: Build succeeds

- [ ] **Step 4: Run E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Shortcode"`
Expected: All tests pass

- [ ] **Step 5: Commit**

```bash
git add e2e/tests/shortcodes.spec.ts e2e/helpers/api-client.ts
git commit -m "test(e2e): add shortcode system tests"
```

---

### Task 10: Lightbox API Preprocessing

**Files:**
- Modify: `server/api_handlers/resource_api_handlers.go` (or relevant handler that returns resource details for lightbox)

The lightbox (`partials/lightbox.tpl` line 763) renders `CustomSidebar` client-side via Alpine's `x-html` from API-fetched resource data. The pongo2 tag doesn't apply there.

**Solution:** When the API returns a resource with its ResourceCategory, pre-process shortcodes in the Custom* fields. The expanded `<meta-shortcode>` elements in the HTML string are recognized as custom elements when `x-html` inserts them into the DOM.

- [ ] **Step 1: Find the resource detail API handler that serves lightbox data**

Check `server/api_handlers/resource_api_handlers.go` — look for the handler that returns the resource JSON used by `$store.lightbox.resourceDetails`. This is the `GetResourceHandler` that returns the resource with preloaded `ResourceCategory`.

- [ ] **Step 2: Add shortcode processing to the response**

After fetching the resource, process shortcodes in the resource's `ResourceCategory.CustomHeader`, `CustomSidebar`, `CustomSummary`, and `CustomAvatar` fields before serializing to JSON. This requires the plugin manager to be available in the handler context.

If the plugin manager is not easily accessible in the API handler, an alternative approach: add a post-processing step in the JSON serialization that the lightbox endpoint uses, or process shortcodes in a middleware for resource detail endpoints.

The simplest approach: process the Custom* fields directly on the model before JSON encoding. Since the API handler already has access to the resource (with preloaded ResourceCategory), modify the Custom* fields in-place:

```go
// Process shortcodes in ResourceCategory custom fields for client-side rendering
if resource.ResourceCategory != nil {
    metaCtx := shortcodes.MetaShortcodeContext{
        EntityType: "resource",
        EntityID:   resource.ID,
        Meta:       resource.Meta,
        MetaSchema: resource.ResourceCategory.MetaSchema,
    }
    resource.ResourceCategory.CustomHeader = shortcodes.Process(resource.ResourceCategory.CustomHeader, metaCtx, pluginRenderer)
    resource.ResourceCategory.CustomSidebar = shortcodes.Process(resource.ResourceCategory.CustomSidebar, metaCtx, pluginRenderer)
    resource.ResourceCategory.CustomSummary = shortcodes.Process(resource.ResourceCategory.CustomSummary, metaCtx, pluginRenderer)
    resource.ResourceCategory.CustomAvatar = shortcodes.Process(resource.ResourceCategory.CustomAvatar, metaCtx, pluginRenderer)
}
```

Note: The exact implementation depends on how the plugin manager is wired into API handlers. Check how `GetResourceHandler` is constructed and whether it has access to the plugin manager. If not, pass it as an additional parameter or use a middleware approach.

- [ ] **Step 3: Test the lightbox displays shortcode content**

Write an E2E test that opens the lightbox for a resource whose ResourceCategory has shortcodes in CustomSidebar, and verify the rendered content appears.

- [ ] **Step 4: Commit**

```bash
git add server/api_handlers/resource_api_handlers.go
git commit -m "feat(shortcodes): pre-process shortcodes in resource API for lightbox"
```

---

### Task 11: Full Test Suite Verification

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All pass

- [ ] **Step 2: Run all E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: All pass

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: All pass

- [ ] **Step 4: Fix any failures**

If any tests fail, investigate and fix before proceeding.

- [ ] **Step 5: Final commit (if fixes were needed)**

```bash
git add -A
git commit -m "fix: resolve test issues from shortcode implementation"
```

---

## Dependencies

```
Task 1 (Parser) ─────┐
                      ├── Task 3 (Processor) ── Task 4 (Pongo2 Tag) ── Task 7 (Templates) ── Task 10 (Lightbox)
Task 2 (Meta Handler)┘                                                                              │
                                                                                                     │
Task 5 (editMeta API) ─────────────────────────────────────────────────────────────────────┐          │
                                                                                           ├── Task 9 (E2E) ── Task 11 (Full Tests)
Task 6 (Web Component) ───────────────────────────────────────────────────────────────────┘          │
                                                                                                     │
Task 8 (Plugin mah.shortcode()) ────────────────────────────────────────────────────────────────────┘
```

Tasks 1+2, 5, 6, and 8 can run in parallel. Tasks 3, 4, 7 are sequential. Tasks 9-11 run after all others.
