# MRQL Shortcodes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `[property]` and `[mrql]` built-in shortcodes so MRQL query results and entity fields can be embedded inline, with custom per-category rendering templates.

**Architecture:** Extend the shortcode processor with two new built-in handlers and a `QueryExecutor` callback (same pattern as `PluginRenderer`). Add `CustomMRQLResult` field to category/type models. The MRQL page gains a `render=1` API parameter that returns server-rendered HTML for entities with custom templates.

**Tech Stack:** Go (shortcodes package, models, API handlers), Pongo2 templates, Alpine.js (MRQL page), Tailwind CSS

---

## File Map

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `shortcodes/processor.go` | Add `QueryExecutor` callback, `context.Context` to `Process()`, route `mrql` and `property` shortcodes |
| Modify | `shortcodes/meta_handler.go` | Add `Entity` field to `MetaShortcodeContext` |
| Create | `shortcodes/property_handler.go` | `[property]` shortcode: extract entity fields, HTML-escape by default |
| Create | `shortcodes/mrql_handler.go` | `[mrql]` shortcode: orchestrate query execution, format resolution, rendering |
| Create | `shortcodes/mrql_renderer.go` | HTML renderers for flat/aggregated/bucketed results in table/list/compact/custom/default formats |
| Modify | `shortcodes/parser.go` | Extend regex to recognize `mrql` and `property` shortcode names |
| Create | `shortcodes/property_handler_test.go` | Unit tests for `[property]` |
| Create | `shortcodes/mrql_handler_test.go` | Unit tests for `[mrql]` with mock executor |
| Modify | `shortcodes/processor_test.go` | Update existing tests for new `Process()` signature |
| Modify | `models/category_model.go` | Add `CustomMRQLResult` field |
| Modify | `models/resource_category_model.go` | Add `CustomMRQLResult` field |
| Modify | `models/note_type_model.go` | Add `CustomMRQLResult` field |
| Modify | `server/template_handlers/template_filters/shortcode_tag.go` | Pass `QueryExecutor` and `Entity` to `Process()` |
| Modify | `server/routes.go` | Pass `QueryExecutor` and `Entity` to `processShortcodesForJSON` |
| Modify | `server/api_handlers/mrql_api_handlers.go` | Add `render=1` parameter, preload categories, return `renderedHTML` |
| Modify | `templates/partials/description.tpl` | Add `process_shortcodes` to filter chain |
| Modify | `templates/createCategory.tpl` | Add `CustomMRQLResult` textarea |
| Modify | `templates/createResourceCategory.tpl` | Add `CustomMRQLResult` textarea |
| Modify | `templates/createNoteType.tpl` | Add `CustomMRQLResult` textarea |
| Modify | `templates/mrql.tpl` | Render `renderedHTML` when present on result entities |
| Modify | `src/components/mrqlEditor.js` | Pass `render=1` to API, use `renderedHTML` |

---

### Task 1: Extend the shortcode parser to recognize `mrql` and `property`

**Files:**
- Modify: `shortcodes/parser.go:23-25` (regex pattern)
- Modify: `shortcodes/parser_test.go`

- [ ] **Step 1: Write failing tests for the new shortcode names**

Add to `shortcodes/parser_test.go`:

```go
func TestParsePropertyShortcode(t *testing.T) {
	result := Parse(`before [property path="Name"] after`)
	assert.Len(t, result, 1)
	assert.Equal(t, "property", result[0].Name)
	assert.Equal(t, "Name", result[0].Attrs["path"])
	assert.Equal(t, `[property path="Name"]`, result[0].Raw)
}

func TestParsePropertyRawAttr(t *testing.T) {
	result := Parse(`[property path="Description" raw="true"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "property", result[0].Name)
	assert.Equal(t, "Description", result[0].Attrs["path"])
	assert.Equal(t, "true", result[0].Attrs["raw"])
}

func TestParseMRQLQueryShortcode(t *testing.T) {
	result := Parse(`[mrql query="type = 'resource'" limit="10" format="table"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "mrql", result[0].Name)
	assert.Equal(t, "type = 'resource'", result[0].Attrs["query"])
	assert.Equal(t, "10", result[0].Attrs["limit"])
	assert.Equal(t, "table", result[0].Attrs["format"])
}

func TestParseMRQLSavedShortcode(t *testing.T) {
	result := Parse(`[mrql saved="my-query" format="custom"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "mrql", result[0].Name)
	assert.Equal(t, "my-query", result[0].Attrs["saved"])
	assert.Equal(t, "custom", result[0].Attrs["format"])
}

func TestParseMRQLBucketsAttr(t *testing.T) {
	result := Parse(`[mrql query="type = 'resource' GROUP BY category" buckets="3" limit="5"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "3", result[0].Attrs["buckets"])
	assert.Equal(t, "5", result[0].Attrs["limit"])
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run "TestParseProperty|TestParseMRQL" -v`
Expected: FAIL — `mrql` and `property` don't match the current regex.

- [ ] **Step 3: Update the regex to recognize the new names**

In `shortcodes/parser.go`, change the `shortcodePattern` regex from:

```go
var shortcodePattern = regexp.MustCompile(
	`\[(meta|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\s*([^\]]*)\]`,
)
```

to:

```go
var shortcodePattern = regexp.MustCompile(
	`\[(meta|property|mrql|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\s*([^\]]*)\]`,
)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: All tests PASS including existing parser tests.

- [ ] **Step 5: Commit**

```bash
git add shortcodes/parser.go shortcodes/parser_test.go
git commit -m "feat: extend shortcode parser to recognize [mrql] and [property]"
```

---

### Task 2: Add `Entity` field to `MetaShortcodeContext` and update `Process()` signature

**Files:**
- Modify: `shortcodes/meta_handler.go:13-18` (context struct)
- Modify: `shortcodes/processor.go` (signature + routing)
- Modify: `shortcodes/processor_test.go` (update existing tests)

- [ ] **Step 1: Write a test verifying the new `Process()` signature compiles**

Add to `shortcodes/processor_test.go`:

```go
func TestProcessWithNilExecutor(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}
	// With nil executor (and nil renderer), plain text passes through
	result := Process(context.Background(), "<p>hello</p>", ctx, nil, nil)
	assert.Equal(t, "<p>hello</p>", result)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestProcessWithNilExecutor -v`
Expected: Compilation error — `Process()` doesn't accept `context.Context` or 5th argument yet.

- [ ] **Step 3: Add `Entity` field to `MetaShortcodeContext`**

In `shortcodes/meta_handler.go`, change the struct from:

```go
type MetaShortcodeContext struct {
	EntityType string // "group", "resource", "note"
	EntityID   uint
	Meta       json.RawMessage // entity's full Meta JSON
	MetaSchema string          // category's MetaSchema JSON string (may be empty)
}
```

to:

```go
type MetaShortcodeContext struct {
	EntityType string          // "group", "resource", "note"
	EntityID   uint
	Meta       json.RawMessage // entity's full Meta JSON
	MetaSchema string          // category's MetaSchema JSON string (may be empty)
	Entity     any             // full model struct (Group/Resource/Note), for [property] shortcode
}
```

- [ ] **Step 4: Add `QueryExecutor` type and update `Process()` signature**

In `shortcodes/processor.go`, add the new types and update the signature. Replace the entire file:

```go
package shortcodes

import (
	"context"
	"encoding/json"
	"strings"
)

// PluginRenderer is a callback that renders a plugin shortcode.
// It receives the plugin name (e.g., "test" from "plugin:test:widget"),
// the parsed shortcode, and the entity context.
// Returns rendered HTML or an error (in which case the original text is preserved).
type PluginRenderer func(pluginName string, sc Shortcode, ctx MetaShortcodeContext) (string, error)

// QueryExecutor is a callback that executes an MRQL query.
// It receives the request context for timeout/cancellation, the query string or
// saved query name (one will be empty), and limit/buckets caps.
// Returns structured results or an error.
type QueryExecutor func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error)

// QueryResult holds the results of a query executed via QueryExecutor.
type QueryResult struct {
	EntityType string
	Mode       string              // "flat", "aggregated", or "bucketed"
	Items      []QueryResultItem   // flat mode
	Rows       []map[string]any    // aggregated mode (GROUP BY with aggregates)
	Groups     []QueryResultGroup  // bucketed mode (GROUP BY without aggregates)
}

// QueryResultItem is a single entity in a query result.
type QueryResultItem struct {
	EntityType       string
	EntityID         uint
	Entity           any
	Meta             json.RawMessage
	MetaSchema       string
	CustomMRQLResult string
}

// QueryResultGroup is a single group/bucket of entities.
type QueryResultGroup struct {
	Key   map[string]any
	Items []QueryResultItem
}

// maxRecursionDepth caps nested [mrql] shortcode processing to prevent infinite loops.
const maxRecursionDepth = 2

// Process parses shortcodes in input and replaces them with rendered HTML.
// Built-in "meta", "property", and "mrql" shortcodes are handled directly.
// Plugin shortcodes (starting with "plugin:") use the provided renderer callback.
// If renderer is nil, plugin shortcodes are left as-is.
// If executor is nil, mrql shortcodes are left as-is.
func Process(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor) string {
	return processWithDepth(reqCtx, input, ctx, renderer, executor, 0)
}

func processWithDepth(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	shortcodes := Parse(input)
	if len(shortcodes) == 0 {
		return input
	}

	var b strings.Builder
	b.Grow(len(input) * 2)
	lastEnd := 0

	for _, sc := range shortcodes {
		b.WriteString(input[lastEnd:sc.Start])

		var replacement string

		switch {
		case sc.Name == "meta":
			replacement = RenderMetaShortcode(sc, ctx)
		case sc.Name == "property":
			replacement = RenderPropertyShortcode(sc, ctx)
		case sc.Name == "mrql":
			if executor != nil && depth < maxRecursionDepth {
				replacement = RenderMRQLShortcode(reqCtx, sc, ctx, renderer, executor, depth)
			} else {
				replacement = sc.Raw
			}
		case strings.HasPrefix(sc.Name, "plugin:"):
			if renderer != nil {
				parts := strings.SplitN(sc.Name, ":", 3)
				if len(parts) == 3 {
					html, err := renderer(parts[1], sc, ctx)
					if err == nil {
						replacement = html
					} else {
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

	b.WriteString(input[lastEnd:])

	return b.String()
}
```

- [ ] **Step 5: Update all existing tests to use the new `Process()` signature**

In `shortcodes/processor_test.go`, add the `"context"` import and update every `Process()` call. Change each call from `Process(input, ctx, renderer)` to `Process(context.Background(), input, ctx, renderer, nil)`. For example:

```go
func TestProcessNoShortcodes(t *testing.T) {
	result := Process(context.Background(), "<div>hello</div>", MetaShortcodeContext{}, nil, nil)
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

	result := Process(context.Background(), `before [meta path="name"] after`, ctx, nil, nil)
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
	result := Process(context.Background(), input, ctx, nil, nil)
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

	result := Process(context.Background(), `[plugin:test:widget size="large"]`, ctx, renderer, nil)
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
	result := Process(context.Background(), `[plugin:test:widget]`, ctx, renderer, nil)
	assert.Equal(t, `[plugin:test:widget]`, result)
}
```

- [ ] **Step 6: Run all shortcode tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: All tests PASS. Compilation succeeds with new signature. Note: `RenderPropertyShortcode` and `RenderMRQLShortcode` don't exist yet — this will fail until Task 3. **If it fails to compile**, add temporary stubs to allow the test run:

Create `shortcodes/property_handler.go`:
```go
package shortcodes

// RenderPropertyShortcode expands a [property] shortcode into the entity field value.
func RenderPropertyShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	return sc.Raw // stub
}
```

Create `shortcodes/mrql_handler.go`:
```go
package shortcodes

import "context"

// RenderMRQLShortcode expands an [mrql] shortcode into rendered query results.
func RenderMRQLShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	return sc.Raw // stub
}
```

Re-run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: All tests PASS.

- [ ] **Step 7: Commit**

```bash
git add shortcodes/processor.go shortcodes/meta_handler.go shortcodes/processor_test.go shortcodes/property_handler.go shortcodes/mrql_handler.go
git commit -m "feat: add QueryExecutor callback and Entity field to shortcode context"
```

---

### Task 3: Implement the `[property]` shortcode handler

**Files:**
- Modify: `shortcodes/property_handler.go` (replace stub)
- Create: `shortcodes/property_handler_test.go`

- [ ] **Step 1: Write failing tests for `[property]`**

Create `shortcodes/property_handler_test.go`:

```go
package shortcodes

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// testEntity is a minimal struct mimicking a model entity for testing.
type testEntity struct {
	ID          uint
	Name        string
	Description string
	CreatedAt   time.Time
	Tags        []string
	Meta        json.RawMessage
}

func TestPropertyShortcodeStringField(t *testing.T) {
	entity := testEntity{ID: 1, Name: "My Resource"}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "My Resource", result)
}

func TestPropertyShortcodeHTMLEscaped(t *testing.T) {
	entity := testEntity{ID: 1, Name: `<script>alert("xss")</script>`}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;", result)
	assert.NotContains(t, result, "<script>")
}

func TestPropertyShortcodeRawAttribute(t *testing.T) {
	entity := testEntity{ID: 1, Description: "<b>bold</b> text"}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Description", "raw": "true"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "<b>bold</b> text", result)
}

func TestPropertyShortcodeUintField(t *testing.T) {
	entity := testEntity{ID: 42, Name: "Test"}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   42,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "ID"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "42", result)
}

func TestPropertyShortcodeSliceField(t *testing.T) {
	entity := testEntity{ID: 1, Tags: []string{"photo", "landscape"}}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "photo, landscape", result)
}

func TestPropertyShortcodeSliceHTMLEscaped(t *testing.T) {
	entity := testEntity{ID: 1, Tags: []string{"<b>bold</b>", "normal"}}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "&lt;b&gt;bold&lt;/b&gt;, normal", result)
}

func TestPropertyShortcodeTimeField(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	entity := testEntity{ID: 1, CreatedAt: ts}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Contains(t, result, "2026")
}

func TestPropertyShortcodeMissingPath(t *testing.T) {
	entity := testEntity{ID: 1, Name: "Test"}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodeInvalidField(t *testing.T) {
	entity := testEntity{ID: 1, Name: "Test"}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "NonExistent"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodeNilEntity(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     nil,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodePointerEntity(t *testing.T) {
	entity := &testEntity{ID: 1, Name: "Pointer Entity"}
	ctx := MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   1,
		Entity:     entity,
	}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "Pointer Entity", result)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestPropertyShortcode -v`
Expected: FAIL — the stub returns `sc.Raw`.

- [ ] **Step 3: Implement `RenderPropertyShortcode`**

Replace the contents of `shortcodes/property_handler.go`:

```go
package shortcodes

import (
	"encoding/json"
	"fmt"
	"html"
	"reflect"
	"strings"
	"time"
)

// RenderPropertyShortcode expands a [property] shortcode by extracting the named
// field from ctx.Entity using reflection. Output is HTML-escaped by default;
// use raw="true" to opt into unescaped output.
func RenderPropertyShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	path := sc.Attrs["path"]
	if path == "" || ctx.Entity == nil {
		return ""
	}

	raw := sc.Attrs["raw"] == "true"

	v := reflect.ValueOf(ctx.Entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return ""
	}

	field := v.FieldByName(path)
	if !field.IsValid() {
		return ""
	}

	text := formatFieldValue(field)

	if raw {
		return text
	}
	return html.EscapeString(text)
}

// formatFieldValue converts a reflect.Value to its string representation.
func formatFieldValue(v reflect.Value) string {
	if !v.IsValid() {
		return ""
	}

	// Handle interface values by unwrapping
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	// Handle pointer types
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return ""
		}
		v = v.Elem()
	}

	iface := v.Interface()

	// Special types
	switch val := iface.(type) {
	case time.Time:
		return val.Format(time.RFC3339)
	case json.RawMessage:
		return string(val)
	}

	switch v.Kind() {
	case reflect.String:
		return v.String()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", v.Float())
	case reflect.Bool:
		return fmt.Sprintf("%t", v.Bool())
	case reflect.Slice:
		parts := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			parts[i] = formatFieldValue(v.Index(i))
		}
		return strings.Join(parts, ", ")
	default:
		// Fallback: JSON-encode complex types
		encoded, err := json.Marshal(iface)
		if err != nil {
			return fmt.Sprintf("%v", iface)
		}
		return string(encoded)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestPropertyShortcode -v`
Expected: All PASS.

- [ ] **Step 5: Run all shortcode tests**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add shortcodes/property_handler.go shortcodes/property_handler_test.go
git commit -m "feat: implement [property] shortcode with HTML-escape-by-default"
```

---

### Task 4: Implement `[mrql]` shortcode handler and renderers

**Files:**
- Modify: `shortcodes/mrql_handler.go` (replace stub)
- Create: `shortcodes/mrql_renderer.go`
- Create: `shortcodes/mrql_handler_test.go`

- [ ] **Step 1: Write failing tests for `[mrql]`**

Create `shortcodes/mrql_handler_test.go`:

```go
package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func mockExecutor(result *QueryResult, err error) QueryExecutor {
	return func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		return result, err
	}
}

func TestMRQLShortcodeFlat(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Photo A"}, Meta: []byte(`{}`)},
			{EntityType: "resource", EntityID: 2, Entity: testEntity{ID: 2, Name: "Photo B"}, Meta: []byte(`{}`)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "type = 'resource'", "limit": "10"}, Raw: `[mrql query="type = 'resource'" limit="10"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "Photo A")
	assert.Contains(t, html, "Photo B")
	assert.Contains(t, html, "mrql-results")
}

func TestMRQLShortcodeCustomTemplate(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType:       "resource",
				EntityID:         1,
				Entity:           testEntity{ID: 1, Name: "My Photo"},
				Meta:             []byte(`{"rating": 5}`),
				CustomMRQLResult: `<div class="card">[property path="Name"]</div>`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "type = 'resource'"}, Raw: `[mrql query="type = 'resource'"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, `<div class="card">My Photo</div>`)
}

func TestMRQLShortcodeFormatOverridesCustom(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "flat",
		Items: []QueryResultItem{
			{
				EntityType:       "resource",
				EntityID:         1,
				Entity:           testEntity{ID: 1, Name: "Entity"},
				Meta:             []byte(`{}`),
				CustomMRQLResult: `<div>CUSTOM</div>`,
			},
		},
	}
	executor := mockExecutor(result, nil)
	// Explicit format="table" overrides the custom template
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test", "format": "table"}, Raw: `[mrql query="test" format="table"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.NotContains(t, html, "CUSTOM")
	assert.Contains(t, html, "<table")
}

func TestMRQLShortcodeAggregated(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "aggregated",
		Rows: []map[string]any{
			{"category": "photo", "count": float64(10)},
			{"category": "video", "count": float64(5)},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "<table")
	assert.Contains(t, html, "photo")
	assert.Contains(t, html, "video")
}

func TestMRQLShortcodeBucketed(t *testing.T) {
	result := &QueryResult{
		EntityType: "resource",
		Mode:       "bucketed",
		Groups: []QueryResultGroup{
			{
				Key: map[string]any{"category": "photo"},
				Items: []QueryResultItem{
					{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Sunset"}, Meta: []byte(`{}`)},
				},
			},
		},
	}
	executor := mockExecutor(result, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "Sunset")
	assert.Contains(t, html, "photo")
}

func TestMRQLShortcodeExecutorError(t *testing.T) {
	executor := mockExecutor(nil, fmt.Errorf("query failed"))
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "bad query"}, Raw: `[mrql query="bad query"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Contains(t, html, "query failed")
}

func TestMRQLShortcodeNoQueryOrSaved(t *testing.T) {
	executor := mockExecutor(nil, nil)
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{}, Raw: `[mrql]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", html)
}

func TestMRQLShortcodeRecursionDepthCap(t *testing.T) {
	callCount := 0
	var executor QueryExecutor
	executor = func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		callCount++
		return &QueryResult{
			EntityType: "resource",
			Mode:       "flat",
			Items: []QueryResultItem{
				{
					EntityType:       "resource",
					EntityID:         1,
					Entity:           testEntity{ID: 1, Name: "Nested"},
					Meta:             []byte(`{}`),
					CustomMRQLResult: `[mrql query="type = 'resource'"]`, // recursive!
				},
			},
		}, nil
	}

	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	// Depth 0 → executes, custom template contains [mrql] → depth 1 executes,
	// that custom template also contains [mrql] → depth 2 hits cap, left as raw
	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, 2, callCount) // should execute exactly twice (depth 0 and 1)
	assert.Contains(t, html, `[mrql query="type = 'resource'"]`) // depth-2 shortcode left raw
}

func TestMRQLShortcodeSavedQuery(t *testing.T) {
	var capturedSaved string
	executor := func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		capturedSaved = savedName
		return &QueryResult{EntityType: "resource", Mode: "flat", Items: []QueryResultItem{
			{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Saved Result"}, Meta: []byte(`{}`)},
		}}, nil
	}
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"saved": "my-query"}, Raw: `[mrql saved="my-query"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	html := RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "my-query", capturedSaved)
	assert.Contains(t, html, "Saved Result")
}

func TestMRQLShortcodeDefaultLimits(t *testing.T) {
	var capturedLimit, capturedBuckets int
	executor := func(ctx context.Context, query string, savedName string, limit int, buckets int) (*QueryResult, error) {
		capturedLimit = limit
		capturedBuckets = buckets
		return &QueryResult{EntityType: "resource", Mode: "flat"}, nil
	}
	sc := Shortcode{Name: "mrql", Attrs: map[string]string{"query": "test"}, Raw: `[mrql query="test"]`}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}

	RenderMRQLShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, 20, capturedLimit)
	assert.Equal(t, 5, capturedBuckets)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestMRQLShortcode -v`
Expected: FAIL — the stub returns `sc.Raw`.

- [ ] **Step 3: Implement the MRQL renderers**

Create `shortcodes/mrql_renderer.go`:

```go
package shortcodes

import (
	"fmt"
	"html"
	"strings"
)

// renderFlatDefault renders flat result items using the default card layout.
func renderFlatDefault(items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">`)
	for _, item := range items {
		name := extractEntityName(item)
		desc := extractEntityDescription(item)
		b.WriteString(fmt.Sprintf(
			`<a href="/%s?id=%d" class="block p-3 bg-white border border-stone-200 rounded-md hover:border-amber-400 hover:shadow-sm transition-colors"><div class="min-w-0"><p class="text-sm font-medium text-stone-900 truncate">%s</p>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
		))
		if desc != "" {
			b.WriteString(fmt.Sprintf(
				`<p class="text-xs text-stone-500 mt-0.5 line-clamp-2">%s</p>`,
				html.EscapeString(desc),
			))
		}
		b.WriteString(`</div></a>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderFlatTable renders flat result items as an HTML table.
func renderFlatTable(items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="overflow-x-auto"><table class="min-w-full text-sm border border-stone-200 rounded-md">`)
	b.WriteString(`<thead class="bg-stone-100"><tr>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Name</th>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Type</th>`)
	b.WriteString(`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">Description</th>`)
	b.WriteString(`</tr></thead><tbody class="divide-y divide-stone-100">`)

	for _, item := range items {
		name := extractEntityName(item)
		desc := extractEntityDescription(item)
		b.WriteString(fmt.Sprintf(
			`<tr class="hover:bg-stone-50"><td class="px-3 py-2"><a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a></td><td class="px-3 py-2 text-stone-500">%s</td><td class="px-3 py-2 text-stone-500 truncate max-w-xs">%s</td></tr>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
			html.EscapeString(item.EntityType),
			html.EscapeString(desc),
		))
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// renderFlatList renders flat result items as a vertical list.
func renderFlatList(items []QueryResultItem) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<ul class="divide-y divide-stone-200 border border-stone-200 rounded-md bg-white">`)
	for _, item := range items {
		name := extractEntityName(item)
		desc := extractEntityDescription(item)
		b.WriteString(fmt.Sprintf(
			`<li class="px-3 py-2 hover:bg-stone-50"><a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
		))
		if desc != "" {
			b.WriteString(fmt.Sprintf(
				` <span class="text-xs text-stone-500">— %s</span>`,
				html.EscapeString(desc),
			))
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)
	return b.String()
}

// renderFlatCompact renders flat result items as inline comma-separated links.
func renderFlatCompact(items []QueryResultItem) string {
	if len(items) == 0 {
		return ""
	}

	parts := make([]string, len(items))
	for i, item := range items {
		name := extractEntityName(item)
		parts[i] = fmt.Sprintf(
			`<a href="/%s?id=%d" class="text-amber-700 hover:text-amber-900 underline">%s</a>`,
			html.EscapeString(item.EntityType),
			item.EntityID,
			html.EscapeString(name),
		)
	}
	return strings.Join(parts, ", ")
}

// renderAggregatedTable renders aggregated GROUP BY rows as an HTML table.
func renderAggregatedTable(rows []map[string]any) string {
	if len(rows) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	// Collect column keys from the first row
	keys := make([]string, 0, len(rows[0]))
	for k := range rows[0] {
		keys = append(keys, k)
	}

	var b strings.Builder
	b.WriteString(`<div class="overflow-x-auto"><table class="min-w-full text-sm font-mono border border-stone-200 rounded-md">`)
	b.WriteString(`<thead class="bg-stone-100"><tr>`)
	for _, k := range keys {
		b.WriteString(fmt.Sprintf(
			`<th class="px-3 py-2 text-left text-xs font-semibold text-stone-600 uppercase border-b border-stone-200">%s</th>`,
			html.EscapeString(k),
		))
	}
	b.WriteString(`</tr></thead><tbody class="divide-y divide-stone-100">`)

	for _, row := range rows {
		b.WriteString(`<tr class="hover:bg-stone-50">`)
		for _, k := range keys {
			val := row[k]
			b.WriteString(fmt.Sprintf(
				`<td class="px-3 py-2 text-stone-800 whitespace-nowrap">%s</td>`,
				html.EscapeString(fmt.Sprintf("%v", val)),
			))
		}
		b.WriteString(`</tr>`)
	}

	b.WriteString(`</tbody></table></div>`)
	return b.String()
}

// renderBucketHeader renders the header bar for a bucketed group.
func renderBucketHeader(key map[string]any, itemCount int) string {
	var parts []string
	for k, v := range key {
		parts = append(parts, fmt.Sprintf(
			`<span class="text-stone-500">%s:</span> <span class="font-semibold text-stone-700">%v</span>`,
			html.EscapeString(k),
			html.EscapeString(fmt.Sprintf("%v", v)),
		))
	}
	return fmt.Sprintf(
		`<div class="bg-stone-100 px-3 py-2 flex items-center gap-2 text-xs font-mono">%s<span class="ml-auto text-stone-400">%d items</span></div>`,
		strings.Join(parts, " "),
		itemCount,
	)
}

// extractEntityName gets the Name field from the entity via reflection, falling back to the entity type + ID.
func extractEntityName(item QueryResultItem) string {
	if item.Entity == nil {
		return fmt.Sprintf("%s #%d", item.EntityType, item.EntityID)
	}
	ctx := MetaShortcodeContext{Entity: item.Entity}
	sc := Shortcode{Attrs: map[string]string{"path": "Name", "raw": "true"}}
	name := RenderPropertyShortcode(sc, ctx)
	if name == "" {
		return fmt.Sprintf("%s #%d", item.EntityType, item.EntityID)
	}
	return name
}

// extractEntityDescription gets the Description field from the entity via reflection.
func extractEntityDescription(item QueryResultItem) string {
	if item.Entity == nil {
		return ""
	}
	ctx := MetaShortcodeContext{Entity: item.Entity}
	sc := Shortcode{Attrs: map[string]string{"path": "Description", "raw": "true"}}
	return RenderPropertyShortcode(sc, ctx)
}
```

- [ ] **Step 4: Implement `RenderMRQLShortcode`**

Replace the contents of `shortcodes/mrql_handler.go`:

```go
package shortcodes

import (
	"context"
	"fmt"
	"html"
	"strconv"
	"strings"
)

const (
	defaultMRQLShortcodeLimit   = 20
	defaultMRQLShortcodeBuckets = 5
)

// RenderMRQLShortcode expands an [mrql] shortcode into rendered query results.
// The depth parameter tracks recursion level for custom templates that may
// contain nested [mrql] shortcodes.
func RenderMRQLShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	query := sc.Attrs["query"]
	saved := sc.Attrs["saved"]
	if query == "" && saved == "" {
		return ""
	}

	limit := parseIntAttr(sc.Attrs["limit"], defaultMRQLShortcodeLimit)
	buckets := parseIntAttr(sc.Attrs["buckets"], defaultMRQLShortcodeBuckets)
	format := sc.Attrs["format"] // "" means auto-resolve

	result, err := executor(reqCtx, query, saved, limit, buckets)
	if err != nil {
		return fmt.Sprintf(
			`<div class="mrql-results mrql-error text-sm text-red-700 bg-red-50 border border-red-200 rounded-md p-3 font-mono">%s</div>`,
			html.EscapeString(err.Error()),
		)
	}

	if result == nil {
		return ""
	}

	var inner string

	switch result.Mode {
	case "aggregated":
		inner = renderAggregatedTable(result.Rows)
	case "bucketed":
		inner = renderBucketed(reqCtx, result.Groups, format, ctx, renderer, executor, depth)
	default: // "flat" or empty
		inner = renderFlat(reqCtx, result.Items, format, ctx, renderer, executor, depth)
	}

	return fmt.Sprintf(`<div class="mrql-results">%s</div>`, inner)
}

// renderFlat renders flat result items using the resolved format.
func renderFlat(reqCtx context.Context, items []QueryResultItem, format string, parentCtx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	switch format {
	case "table":
		return renderFlatTable(items)
	case "list":
		return renderFlatList(items)
	case "compact":
		return renderFlatCompact(items)
	case "custom":
		return renderFlatWithCustom(reqCtx, items, renderer, executor, depth, true)
	default:
		// Auto-resolve: try custom templates, fall back to default
		return renderFlatWithCustom(reqCtx, items, renderer, executor, depth, false)
	}
}

// renderFlatWithCustom renders items, using custom templates where available.
// If forceCustom is true (explicit format="custom"), items without templates use default rendering.
// If forceCustom is false (auto-resolve), items without templates also use default rendering.
func renderFlatWithCustom(reqCtx context.Context, items []QueryResultItem, renderer PluginRenderer, executor QueryExecutor, depth int, forceCustom bool) string {
	if len(items) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	// Check if any item has a custom template
	hasAnyCustom := false
	for _, item := range items {
		if item.CustomMRQLResult != "" {
			hasAnyCustom = true
			break
		}
	}

	// If no custom templates and not forced, use default
	if !hasAnyCustom && !forceCustom {
		return renderFlatDefault(items)
	}

	var b strings.Builder
	for _, item := range items {
		if item.CustomMRQLResult != "" {
			childCtx := MetaShortcodeContext{
				EntityType: item.EntityType,
				EntityID:   item.EntityID,
				Meta:       item.Meta,
				MetaSchema: item.MetaSchema,
				Entity:     item.Entity,
			}
			rendered := processWithDepth(reqCtx, item.CustomMRQLResult, childCtx, renderer, executor, depth+1)
			b.WriteString(rendered)
		} else {
			// Fall back to default single-item rendering
			b.WriteString(renderFlatDefault([]QueryResultItem{item}))
		}
	}
	return b.String()
}

// renderBucketed renders bucketed GROUP BY results.
func renderBucketed(reqCtx context.Context, groups []QueryResultGroup, format string, parentCtx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if len(groups) == 0 {
		return `<p class="text-sm text-stone-500 font-mono py-2 text-center">No results.</p>`
	}

	var b strings.Builder
	b.WriteString(`<div class="space-y-4">`)
	for _, group := range groups {
		b.WriteString(`<div class="border border-stone-200 rounded-md overflow-hidden">`)
		b.WriteString(renderBucketHeader(group.Key, len(group.Items)))
		b.WriteString(`<div class="p-3">`)
		b.WriteString(renderFlat(reqCtx, group.Items, format, parentCtx, renderer, executor, depth))
		b.WriteString(`</div></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func parseIntAttr(s string, defaultVal int) int {
	if s == "" {
		return defaultVal
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return defaultVal
	}
	return v
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: All PASS.

- [ ] **Step 6: Commit**

```bash
git add shortcodes/mrql_handler.go shortcodes/mrql_renderer.go shortcodes/mrql_handler_test.go
git commit -m "feat: implement [mrql] shortcode with flat/aggregated/bucketed/custom rendering"
```

---

### Task 5: Add `CustomMRQLResult` field to category/type models

**Files:**
- Modify: `models/category_model.go:24`
- Modify: `models/resource_category_model.go:24`
- Modify: `models/note_type_model.go:22`

- [ ] **Step 1: Add the field to `Category`**

In `models/category_model.go`, add after line 24 (after `CustomAvatar`):

```go
	// CustomMRQLResult is a template (HTML + shortcodes) for rendering entities of this
	// category in MRQL query results. When set, used as default rendering in [mrql] shortcodes
	// and on the MRQL results page.
	CustomMRQLResult string `gorm:"type:text"`
```

- [ ] **Step 2: Add the field to `ResourceCategory`**

In `models/resource_category_model.go`, add after line 24 (after `CustomAvatar`):

```go
	// CustomMRQLResult is a template (HTML + shortcodes) for rendering resources of this
	// category in MRQL query results.
	CustomMRQLResult string `gorm:"type:text"`
```

- [ ] **Step 3: Add the field to `NoteType`**

In `models/note_type_model.go`, add after line 22 (after `CustomAvatar`):

```go
	// CustomMRQLResult is a template (HTML + shortcodes) for rendering notes of this
	// type in MRQL query results.
	CustomMRQLResult string `gorm:"type:text"`
```

- [ ] **Step 4: Verify the application builds**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds. GORM will auto-migrate the new column on startup.

- [ ] **Step 5: Commit**

```bash
git add models/category_model.go models/resource_category_model.go models/note_type_model.go
git commit -m "feat: add CustomMRQLResult field to Category, ResourceCategory, NoteType"
```

---

### Task 6: Add `CustomMRQLResult` textarea to category/type editor forms

**Files:**
- Modify: `templates/createCategory.tpl:80`
- Modify: `templates/createResourceCategory.tpl:80`
- Modify: `templates/createNoteType.tpl:80`

- [ ] **Step 1: Add textarea to Category form**

In `templates/createCategory.tpl`, after line 80 (after the `CustomAvatar` include), add:

```html
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom MRQL Result" name="CustomMRQLResult" value=category.CustomMRQLResult %}
```

- [ ] **Step 2: Add textarea to ResourceCategory form**

In `templates/createResourceCategory.tpl`, after line 80 (after the `CustomAvatar` include), add:

```html
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom MRQL Result" name="CustomMRQLResult" value=resourceCategory.CustomMRQLResult %}
```

- [ ] **Step 3: Add textarea to NoteType form**

In `templates/createNoteType.tpl`, after line 80 (after the `CustomAvatar` include), add:

```html
        {% include "/partials/form/createFormTextareaInput.tpl" with title="Custom MRQL Result" name="CustomMRQLResult" value=noteType.CustomMRQLResult %}
```

- [ ] **Step 4: Verify the application builds**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add templates/createCategory.tpl templates/createResourceCategory.tpl templates/createNoteType.tpl
git commit -m "feat: add CustomMRQLResult textarea to category/type editor forms"
```

---

### Task 7: Wire up `QueryExecutor` and `Entity` in the template filter and routes

**Files:**
- Modify: `server/template_handlers/template_filters/shortcode_tag.go`
- Modify: `server/routes.go:164-222`

This task updates both call sites of `shortcodes.Process()` to pass the new `QueryExecutor` callback and populate `Entity` on the context. It also needs to import `context` and the application context for query execution.

- [ ] **Step 1: Update the template filter (`shortcode_tag.go`)**

The template filter needs to:
1. Accept `context.Context` — extract from the pongo2 public context (the HTTP request context).
2. Pass `Entity` on the `MetaShortcodeContext`.
3. Build and pass a `QueryExecutor` callback.

Replace the `Execute` method and `buildMetaContext` function. The full updated file:

```go
package template_filters

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

type processShortcodesNode struct {
	contentExpr pongo2.IEvaluator
	entityExpr  pongo2.IEvaluator
}

func (node *processShortcodesNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	contentVal, err := node.contentExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	content := contentVal.String()
	if content == "" {
		return nil
	}

	entityVal, err := node.entityExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	entity := entityVal.Interface()
	if entity == nil {
		_, _ = writer.WriteString(content)
		return nil
	}

	metaCtx := buildMetaContext(entity)
	if metaCtx == nil {
		_, _ = writer.WriteString(content)
		return nil
	}

	var pluginRenderer shortcodes.PluginRenderer
	if pmVal, ok := ctx.Public["_pluginManager"]; ok && pmVal != nil {
		if pm, ok := pmVal.(*plugin_system.PluginManager); ok && pm != nil {
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs)
			}
		}
	}

	var executor shortcodes.QueryExecutor
	if appCtxVal, ok := ctx.Public["_appContext"]; ok && appCtxVal != nil {
		if appCtx, ok := appCtxVal.(*application_context.MahresourcesContext); ok && appCtx != nil {
			executor = BuildQueryExecutor(appCtx)
		}
	}

	// Use request context if available, otherwise background
	reqCtx := context.Background()
	if reqCtxVal, ok := ctx.Public["_requestContext"]; ok && reqCtxVal != nil {
		if rc, ok := reqCtxVal.(context.Context); ok {
			reqCtx = rc
		}
	}

	result := shortcodes.Process(reqCtx, content, *metaCtx, pluginRenderer, executor)
	if _, writeErr := writer.WriteString(result); writeErr != nil {
		return ctx.Error(fmt.Sprintf("process_shortcodes: write error: %s", writeErr), nil)
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

	idField := v.FieldByName("ID")
	if !idField.IsValid() || idField.Kind() != reflect.Uint {
		return nil
	}
	id := uint(idField.Uint())

	var metaJSON json.RawMessage
	metaField := v.FieldByName("Meta")
	if metaField.IsValid() {
		if raw, err := json.Marshal(metaField.Interface()); err == nil {
			metaJSON = raw
		}
	}

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
		Entity:     entity,
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
	contentExpr, err := arguments.ParseExpression()
	if err != nil {
		return nil, err
	}

	entityExpr, err := arguments.ParseExpression()
	if err != nil {
		return nil, arguments.Error("process_shortcodes tag requires two arguments: content and entity", nil)
	}

	if arguments.Remaining() > 0 {
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

- [ ] **Step 2: Create `buildQueryExecutor` helper**

Create `server/template_handlers/template_filters/shortcode_query_executor.go`:

```go
package template_filters

import (
	"context"
	"encoding/json"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/shortcodes"
)

// BuildQueryExecutor creates a QueryExecutor callback that uses the application
// context to execute MRQL queries. It preloads categories on result entities to
// extract CustomMRQLResult templates.
func BuildQueryExecutor(appCtx *application_context.MahresourcesContext) shortcodes.QueryExecutor {
	return func(reqCtx context.Context, query string, savedName string, limit int, buckets int) (*shortcodes.QueryResult, error) {
		return executeMRQLForShortcode(reqCtx, appCtx, query, savedName, limit, buckets)
	}
}

// executeMRQLForShortcode runs an MRQL query and converts the result into shortcode types.
func executeMRQLForShortcode(reqCtx context.Context, appCtx *application_context.MahresourcesContext, query string, savedName string, limit int, buckets int) (*shortcodes.QueryResult, error) {
	// Resolve saved query name to query string
	actualQuery := query
	if savedName != "" && query == "" {
		saved, err := appCtx.GetSavedMRQLQueryByName(savedName)
		if err != nil {
			return nil, err
		}
		actualQuery = saved.Query
	}

	result, err := appCtx.ExecuteMRQL(reqCtx, actualQuery, limit, 0)
	if err != nil {
		return nil, err
	}

	// Convert to shortcode result types
	qr := &shortcodes.QueryResult{
		EntityType: result.EntityType,
		Mode:       "flat",
	}

	// Collect all entities and preload their categories
	items := convertResultItems(result, appCtx)
	qr.Items = items

	return qr, nil
}

// convertResultItems converts MRQLResult entities into QueryResultItems with
// category information preloaded for CustomMRQLResult templates.
func convertResultItems(result *application_context.MRQLResult, appCtx *application_context.MahresourcesContext) []shortcodes.QueryResultItem {
	var items []shortcodes.QueryResultItem

	for i := range result.Resources {
		r := &result.Resources[i]
		// Preload category if not already loaded
		if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
			cat, err := appCtx.GetResourceCategory(r.ResourceCategoryId)
			if err == nil {
				r.ResourceCategory = cat
			}
		}
		item := shortcodes.QueryResultItem{
			EntityType: "resource",
			EntityID:   r.ID,
			Entity:     r,
			Meta:       json.RawMessage(r.Meta),
		}
		if r.ResourceCategory != nil {
			item.MetaSchema = r.ResourceCategory.MetaSchema
			item.CustomMRQLResult = r.ResourceCategory.CustomMRQLResult
		}
		items = append(items, item)
	}

	for i := range result.Notes {
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
			nt, err := appCtx.GetNoteType(*n.NoteTypeId)
			if err == nil {
				n.NoteType = nt
			}
		}
		item := shortcodes.QueryResultItem{
			EntityType: "note",
			EntityID:   n.ID,
			Entity:     n,
			Meta:       json.RawMessage(n.Meta),
		}
		if n.NoteType != nil {
			item.MetaSchema = n.NoteType.MetaSchema
			item.CustomMRQLResult = n.NoteType.CustomMRQLResult
		}
		items = append(items, item)
	}

	for i := range result.Groups {
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
			cat, err := appCtx.GetCategory(*g.CategoryId)
			if err == nil {
				g.Category = cat
			}
		}
		item := shortcodes.QueryResultItem{
			EntityType: "group",
			EntityID:   g.ID,
			Entity:     g,
			Meta:       json.RawMessage(g.Meta),
		}
		if g.Category != nil {
			item.MetaSchema = g.Category.MetaSchema
			item.CustomMRQLResult = g.Category.CustomMRQLResult
		}
		items = append(items, item)
	}

	return items
}

// convertGroupedResultItems converts MRQLGroupedResult into QueryResult.
func convertGroupedResultItems(result *application_context.MRQLGroupedResult, appCtx *application_context.MahresourcesContext) *shortcodes.QueryResult {
	qr := &shortcodes.QueryResult{
		EntityType: result.EntityType,
	}

	if result.Mode == "aggregated" {
		qr.Mode = "aggregated"
		qr.Rows = result.Rows
		return qr
	}

	// Bucketed mode
	qr.Mode = "bucketed"
	for _, bucket := range result.Groups {
		group := shortcodes.QueryResultGroup{
			Key: bucket.Key,
		}
		// Convert bucket items — they are typed entities
		switch items := bucket.Items.(type) {
		case []models.Resource:
			for i := range items {
				r := &items[i]
				if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
					cat, _ := appCtx.GetResourceCategory(r.ResourceCategoryId)
					if cat != nil {
						r.ResourceCategory = cat
					}
				}
				item := shortcodes.QueryResultItem{
					EntityType: "resource",
					EntityID:   r.ID,
					Entity:     r,
					Meta:       json.RawMessage(r.Meta),
				}
				if r.ResourceCategory != nil {
					item.MetaSchema = r.ResourceCategory.MetaSchema
					item.CustomMRQLResult = r.ResourceCategory.CustomMRQLResult
				}
				group.Items = append(group.Items, item)
			}
		case []models.Note:
			for i := range items {
				n := &items[i]
				if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
					nt, _ := appCtx.GetNoteType(*n.NoteTypeId)
					if nt != nil {
						n.NoteType = nt
					}
				}
				item := shortcodes.QueryResultItem{
					EntityType: "note",
					EntityID:   n.ID,
					Entity:     n,
					Meta:       json.RawMessage(n.Meta),
				}
				if n.NoteType != nil {
					item.MetaSchema = n.NoteType.MetaSchema
					item.CustomMRQLResult = n.NoteType.CustomMRQLResult
				}
				group.Items = append(group.Items, item)
			}
		case []models.Group:
			for i := range items {
				g := &items[i]
				if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
					cat, _ := appCtx.GetCategory(*g.CategoryId)
					if cat != nil {
						g.Category = cat
					}
				}
				item := shortcodes.QueryResultItem{
					EntityType: "group",
					EntityID:   g.ID,
					Entity:     g,
					Meta:       json.RawMessage(g.Meta),
				}
				if g.Category != nil {
					item.MetaSchema = g.Category.MetaSchema
					item.CustomMRQLResult = g.Category.CustomMRQLResult
				}
				group.Items = append(group.Items, item)
			}
		}
		qr.Groups = append(qr.Groups, group)
	}

	return qr
}
```

- [ ] **Step 3: Check that `_appContext` and `_requestContext` are available in the pongo2 context**

Search for where the pongo2 template context is populated:

Run: `grep -rn "_appContext\|_requestContext" server/`

If these keys are not currently set in the template context, they need to be added. Check `server/routes.go` or `server/template_handlers/` for where the pongo2 context is built. Look for where `_pluginManager` is set — `_appContext` and `_requestContext` should be set in the same place.

If `_appContext` is not already set, find the function that sets `_pluginManager` and add:

```go
ctx["_appContext"] = appContext
```

Similarly for `_requestContext`, find where the HTTP request is available and add:

```go
ctx["_requestContext"] = request.Context()
```

These must be set wherever the template context is built for entity display pages.

- [ ] **Step 4: Update `processShortcodesForJSON` in `server/routes.go`**

Update the function to pass `context.Background()`, `Entity`, and `QueryExecutor`. The `processShortcodesForJSON` function needs the application context. Check its call site to see if `appContext` is available.

Update `server/routes.go`. Change the function signature and body:

```go
// processShortcodesForJSON processes shortcode markup in Custom* fields of
// entity categories/types so that JSON API consumers (e.g., the lightbox)
// receive expanded HTML instead of raw [meta ...] shortcode text.
// Only called for JSON responses — HTML responses use the process_shortcodes template tag.
func processShortcodesForJSON(ctx pongo2.Context, pm *plugin_system.PluginManager, appCtx *application_context.MahresourcesContext) {
	mainEntity := ctx["mainEntity"]
	entityType, _ := ctx["mainEntityType"].(string)
	if mainEntity == nil || entityType == "" {
		return
	}

	var pluginRenderer shortcodes.PluginRenderer
	if pm != nil {
		pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
			return pm.RenderShortcode(pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs)
		}
	}

	var executor shortcodes.QueryExecutor
	if appCtx != nil {
		executor = template_filters.BuildQueryExecutor(appCtx)
	}

	reqCtx := context.Background() // routes.go doesn't have the HTTP request here; use Background

	switch entityType {
	case "resource":
		if r, ok := mainEntity.(*models.Resource); ok && r.ResourceCategory != nil {
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType: "resource",
				EntityID:   r.ID,
				Meta:       json.RawMessage(r.Meta),
				MetaSchema: r.ResourceCategory.MetaSchema,
				Entity:     r,
			}
			r.ResourceCategory.CustomHeader = shortcodes.Process(reqCtx, r.ResourceCategory.CustomHeader, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomSidebar = shortcodes.Process(reqCtx, r.ResourceCategory.CustomSidebar, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomSummary = shortcodes.Process(reqCtx, r.ResourceCategory.CustomSummary, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomAvatar = shortcodes.Process(reqCtx, r.ResourceCategory.CustomAvatar, metaCtx, pluginRenderer, executor)
		}
	case "group":
		if g, ok := mainEntity.(*models.Group); ok && g.Category != nil {
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType: "group",
				EntityID:   g.ID,
				Meta:       json.RawMessage(g.Meta),
				MetaSchema: g.Category.MetaSchema,
				Entity:     g,
			}
			g.Category.CustomHeader = shortcodes.Process(reqCtx, g.Category.CustomHeader, metaCtx, pluginRenderer, executor)
			g.Category.CustomSidebar = shortcodes.Process(reqCtx, g.Category.CustomSidebar, metaCtx, pluginRenderer, executor)
			g.Category.CustomSummary = shortcodes.Process(reqCtx, g.Category.CustomSummary, metaCtx, pluginRenderer, executor)
			g.Category.CustomAvatar = shortcodes.Process(reqCtx, g.Category.CustomAvatar, metaCtx, pluginRenderer, executor)
		}
	case "note":
		if n, ok := mainEntity.(*models.Note); ok && n.NoteType != nil {
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType: "note",
				EntityID:   n.ID,
				Meta:       json.RawMessage(n.Meta),
				Entity:     n,
			}
			n.NoteType.CustomHeader = shortcodes.Process(reqCtx, n.NoteType.CustomHeader, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomSidebar = shortcodes.Process(reqCtx, n.NoteType.CustomSidebar, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomSummary = shortcodes.Process(reqCtx, n.NoteType.CustomSummary, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomAvatar = shortcodes.Process(reqCtx, n.NoteType.CustomAvatar, metaCtx, pluginRenderer, executor)
		}
	}
}
```

Then update all call sites of `processShortcodesForJSON` to pass the additional `appCtx` parameter. Search for calls with: `grep -rn "processShortcodesForJSON" server/`

For each call site, add the `appCtx` parameter. The application context should be available in the same scope where `pm` (plugin manager) is accessed.

- [ ] **Step 5: Verify the application builds**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds. Fix any missing imports or incorrect references.

- [ ] **Step 6: Commit**

```bash
git add server/template_handlers/template_filters/shortcode_tag.go server/template_handlers/template_filters/shortcode_query_executor.go server/routes.go
git commit -m "feat: wire QueryExecutor and Entity into both shortcode call sites"
```

---

### Task 8: Add `process_shortcodes` to the description partial

**Files:**
- Modify: `templates/partials/description.tpl:11`

- [ ] **Step 1: Update the description partial**

In `templates/partials/description.tpl`, change line 11 from:

```html
                    {% if !preview %}{{ description|markdown2|render_mentions }}{% endif %}
```

to:

```html
                    {% if !preview %}{% process_shortcodes description entity %}{% endif %}
```

Wait — `process_shortcodes` is a tag, not a filter, and the description content needs markdown processing first. The shortcode processing should happen _after_ markdown conversion. But `process_shortcodes` takes raw text input, not pre-filtered output.

Check how `process_shortcodes` is used in other templates (e.g., `displayResource.tpl` line 6) — it takes a field directly. For descriptions, we need shortcodes processed on the raw text _before_ markdown, since shortcodes produce HTML that shouldn't be markdown-escaped.

Change line 11 to:

```html
                    {% if !preview %}{% process_shortcodes description descriptionEntity %}{% endif %}
```

This means the caller needs to pass `descriptionEntity` when including the partial. Check how the partial is included. Search: `grep -rn "description.tpl" templates/`

The `description.tpl` partial is included with variables like `description=note.Description`. The entity variable must also be passed. For each include site, add the entity. For example:

Note display: add `descriptionEntity=note`
Group display: add `descriptionEntity=group`
Resource display: add `descriptionEntity=resource`

However, `markdown2` and `render_mentions` filters are still needed. Since `process_shortcodes` replaces shortcodes with HTML, we want:
1. Process shortcodes first (produces HTML from shortcodes)
2. The remaining text gets markdown-processed

But shortcodes output raw HTML, and markdown will escape it. So the flow should be: render markdown first, then process shortcodes on the HTML output. But `process_shortcodes` is a block tag, not a filter.

The cleanest approach: apply `markdown2|render_mentions` first to get HTML, then process shortcodes on that HTML. But `process_shortcodes` is a tag that writes directly. We need a two-step approach.

Actually, looking at the existing usage: `{% process_shortcodes resource.ResourceCategory.CustomHeader resource %}` — the CustomHeader field contains raw HTML + shortcodes, not markdown. For descriptions, the content is markdown + shortcodes.

The simplest approach: process shortcodes first (they produce HTML), then the markdown processor will leave the HTML through (markdown passes through inline HTML). Actually, pongo2 filter chains and tags don't easily compose this way.

Better approach: add a `process_shortcodes` filter (not just tag) that can be chained:

```html
{% if !preview %}{{ description|process_shortcodes:entity|markdown2|render_mentions }}{% endif %}
```

But the existing implementation is a tag, not a filter. Creating a filter version would be simpler for this use case. However, looking at how Pongo2 filters work, they only take a value and optional arg — the entity context can't easily be passed.

**Simplest working approach:** Keep description rendering as `markdown2|render_mentions`, and add a separate `process_shortcodes` call on the result. Since `process_shortcodes` is a tag that writes output, we can pipe through a template variable:

Actually, revisiting: shortcodes in descriptions will produce HTML snippets (like `<div class="mrql-results">...</div>`). Markdown processors typically pass through raw HTML blocks. So the order should be: shortcodes first → markdown second.

But the current `process_shortcodes` tag doesn't apply markdown. And description content is typically markdown.

**Recommended approach for now:** Add `process_shortcodes` after markdown processing. The shortcode markers `[mrql ...]` and `[property ...]` look like bracket text and will pass through markdown essentially unchanged (they don't look like markdown links because they lack `(url)` syntax). Then process_shortcodes expands them in the HTML output.

Update line 11:

```html
                    {% if !preview %}{% process_shortcodes description descriptionEntity %}{% endif %}
```

But this loses the `markdown2|render_mentions` processing. We need both. The simplest approach is to NOT use `process_shortcodes` on the raw description, but instead create a custom combined processing.

**Actually, the cleanest approach:** Since `process_shortcodes` is already a template tag in pongo2, and the description needs markdown + shortcodes, the simplest fix is to add shortcode processing inside the markdown filter, or apply them in sequence. Let's look at what `markdown2` does — it converts markdown to HTML. Shortcodes in the text like `[mrql query="..."]` would be left as plain text by markdown (they're not valid markdown links). Then `process_shortcodes` on the HTML output would expand them.

So the approach is: apply markdown first, then process shortcodes on the output. We need a way to capture the output of `markdown2|render_mentions` and pass it to `process_shortcodes`.

In pongo2, we can use `{% with %}`:

```html
{% if !preview %}
    {% with processed_desc=description|markdown2|render_mentions %}
        {% process_shortcodes processed_desc descriptionEntity %}
    {% endwith %}
{% endif %}
```

Hmm, but `process_shortcodes` takes a variable expression, and `with` creates a new variable in scope. Let's verify pongo2 supports this pattern.

Actually, looking at the `process_shortcodes` tag parser, it parses expressions — so a variable set by `with` should work. But `{% with %}` in pongo2 uses `{% set %}` syntax. Let's use the pattern directly in the template.

**Final approach — use nested template tags:**

Change line 11 from:
```html
{% if !preview %}{{ description|markdown2|render_mentions }}{% endif %}
```

to:
```html
{% if !preview %}{% process_shortcodes description|markdown2|render_mentions descriptionEntity %}{% endif %}
```

The `process_shortcodes` tag parser calls `arguments.ParseExpression()` which should handle filter chaining on the first argument. If pongo2 supports filter expressions as tag arguments (which it does — pongo2 expression parsing handles pipes), this will work: the `description|markdown2|render_mentions` evaluates to the markdown-rendered HTML, and then `process_shortcodes` processes shortcodes on that output.

- [ ] **Step 2: Test that `process_shortcodes` tag handles filter expressions**

Before making the template change, verify this works by checking pongo2 docs or existing similar usage in the codebase. Search: `grep -rn "process_shortcodes.*|" templates/`

If pongo2 tag arguments support filter chains (they do — pongo2's `ParseExpression` handles the full expression grammar including filters), proceed with the change.

- [ ] **Step 3: Update description.tpl**

In `templates/partials/description.tpl`, change line 11 from:

```html
                    {% if !preview %}{{ description|markdown2|render_mentions }}{% endif %}
```

to:

```html
                    {% if !preview %}{% process_shortcodes description|markdown2|render_mentions descriptionEntity %}{% endif %}
```

- [ ] **Step 4: Update all includes of the description partial to pass `descriptionEntity`**

Search for all includes: `grep -rn "description.tpl" templates/`

For each include that passes `description=`, add the entity variable. Examples:

If the include is:
```html
{% include "/partials/description.tpl" with description=note.Description descriptionEditUrl="/v1/note/editDescription" %}
```

Change to:
```html
{% include "/partials/description.tpl" with description=note.Description descriptionEntity=note descriptionEditUrl="/v1/note/editDescription" %}
```

Repeat for all entity types (note, group, resource). For list views (preview mode), pass the entity too so it works if preview is ever expanded.

If some includes don't have the entity available (e.g., in list partials where only description is passed), pass `nil` or omit — the `process_shortcodes` tag handles nil entities by writing content as-is.

- [ ] **Step 5: Verify the application builds**

Run: `go build --tags 'json1 fts5'`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add templates/partials/description.tpl templates/
git commit -m "feat: add shortcode processing to description partial"
```

---

### Task 9: Add `render=1` support to the MRQL API for custom template rendering

**Files:**
- Modify: `server/api_handlers/mrql_api_handlers.go`
- Modify: `src/components/mrqlEditor.js`
- Modify: `templates/mrql.tpl`

- [ ] **Step 1: Add `render` parameter to `mrqlExecuteRequest`**

In `server/api_handlers/mrql_api_handlers.go`, add to the `mrqlExecuteRequest` struct:

```go
type mrqlExecuteRequest struct {
	Query   string `json:"query" schema:"query"`
	Limit   int    `json:"limit" schema:"limit"`
	Buckets int    `json:"buckets" schema:"buckets"`
	Page    int    `json:"page" schema:"page"`
	Offset  int    `json:"offset" schema:"offset"`
	Render  bool   `json:"render" schema:"render"` // render=1 enables server-side custom template rendering
}
```

- [ ] **Step 2: Add rendering logic to `GetExecuteMRQLHandler`**

After the JSON encoding of results in `GetExecuteMRQLHandler`, but before writing the response, add logic to process `CustomMRQLResult` templates when `render=1`. This requires access to the application context (already available as `ctx`) and the shortcode processor.

Add a helper function in the same file or import from the template filters package:

```go
// renderCustomTemplates processes CustomMRQLResult templates on result entities
// when the render parameter is set. It adds a "renderedHTML" key to each entity
// in the JSON response.
func renderCustomTemplates(appCtx *application_context.MahresourcesContext, result *application_context.MRQLResult, reqCtx context.Context) {
	executor := template_filters.BuildQueryExecutor(appCtx)

	for i := range result.Resources {
		r := &result.Resources[i]
		if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
			cat, _ := appCtx.GetResourceCategory(r.ResourceCategoryId)
			if cat != nil {
				r.ResourceCategory = cat
			}
		}
		if r.ResourceCategory != nil && r.ResourceCategory.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "resource",
				EntityID:   r.ID,
				Meta:       json.RawMessage(r.Meta),
				MetaSchema: r.ResourceCategory.MetaSchema,
				Entity:     r,
			}
			rendered := shortcodes.Process(reqCtx, r.ResourceCategory.CustomMRQLResult, mctx, nil, executor)
			// Attach rendered HTML — we'll need a response wrapper
		}
	}
	// Similar for Notes and Groups...
}
```

Actually, the cleaner approach is to wrap the response. Create a new response struct that includes `renderedHTML` per entity. When `render=1`:

1. Preload categories on all result entities
2. For entities with `CustomMRQLResult`, process the template
3. Return a modified JSON response with `renderedHTML` added

Since modifying the model structs to add a transient `RenderedHTML` field is the simplest approach:

Add to each model (or use a wrapper):

In `models/resource_model.go`, add a transient field:
```go
RenderedHTML string `gorm:"-" json:"renderedHTML,omitempty"`
```

Similarly for Note and Group models.

Then in the handler, after executing the query and when `render=1`, loop through results and set `RenderedHTML`.

- [ ] **Step 3: Add `RenderedHTML` transient field to entity models**

In `models/resource_model.go`, add after the last field before the closing brace:
```go
	// RenderedHTML is a transient field populated by the API when render=1 is set.
	// Contains server-side rendered custom template HTML, if the entity's category
	// defines a CustomMRQLResult template.
	RenderedHTML string `gorm:"-" json:"renderedHTML,omitempty"`
```

Add the same field to `models/note_model.go` and `models/group_model.go`.

- [ ] **Step 4: Add rendering logic to the MRQL handler**

In `server/api_handlers/mrql_api_handlers.go`, in `GetExecuteMRQLHandler`, add rendering after the query executes but before JSON encoding. After `result, err := ctx.ExecuteMRQL(...)`:

```go
		if req.Render {
			renderMRQLCustomTemplates(ctx, result, request.Context())
		}
```

Add the helper function:

```go
// renderMRQLCustomTemplates populates RenderedHTML on result entities that have
// a CustomMRQLResult template defined on their category/type.
func renderMRQLCustomTemplates(appCtx *application_context.MahresourcesContext, result *application_context.MRQLResult, reqCtx context.Context) {
	executor := template_filters.BuildQueryExecutor(appCtx)

	for i := range result.Resources {
		r := &result.Resources[i]
		if r.ResourceCategory == nil && r.ResourceCategoryId > 0 {
			cat, _ := appCtx.GetResourceCategory(r.ResourceCategoryId)
			if cat != nil {
				r.ResourceCategory = cat
			}
		}
		if r.ResourceCategory != nil && r.ResourceCategory.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "resource",
				EntityID:   r.ID,
				Meta:       json.RawMessage(r.Meta),
				MetaSchema: r.ResourceCategory.MetaSchema,
				Entity:     r,
			}
			r.RenderedHTML = shortcodes.Process(reqCtx, r.ResourceCategory.CustomMRQLResult, mctx, nil, executor)
		}
	}

	for i := range result.Notes {
		n := &result.Notes[i]
		if n.NoteType == nil && n.NoteTypeId != nil && *n.NoteTypeId > 0 {
			nt, _ := appCtx.GetNoteType(*n.NoteTypeId)
			if nt != nil {
				n.NoteType = nt
			}
		}
		if n.NoteType != nil && n.NoteType.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "note",
				EntityID:   n.ID,
				Meta:       json.RawMessage(n.Meta),
				Entity:     n,
			}
			n.RenderedHTML = shortcodes.Process(reqCtx, n.NoteType.CustomMRQLResult, mctx, nil, executor)
		}
	}

	for i := range result.Groups {
		g := &result.Groups[i]
		if g.Category == nil && g.CategoryId != nil && *g.CategoryId > 0 {
			cat, _ := appCtx.GetCategory(*g.CategoryId)
			if cat != nil {
				g.Category = cat
			}
		}
		if g.Category != nil && g.Category.CustomMRQLResult != "" {
			mctx := shortcodes.MetaShortcodeContext{
				EntityType: "group",
				EntityID:   g.ID,
				Meta:       json.RawMessage(g.Meta),
				Entity:     g,
			}
			g.RenderedHTML = shortcodes.Process(reqCtx, g.Category.CustomMRQLResult, mctx, nil, executor)
		}
	}
}
```

Do the same for grouped results in the grouped handler path.

Note: `BuildQueryExecutor` needs to be exported (capital B) in `shortcode_query_executor.go`.

- [ ] **Step 5: Update `mrqlEditor.js` to pass `render=1`**

In `src/components/mrqlEditor.js`, update the `execute` method to always pass `render: true` in the request body (line 302):

```javascript
        const resp = await fetch('/v1/mrql', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ query, render: true }),
        });
```

- [ ] **Step 6: Update `templates/mrql.tpl` to display `renderedHTML`**

In `templates/mrql.tpl`, for each entity rendering template (resource, note, group results around lines 397-453), add a check for `renderedHTML`. For example, in the resource results section (line 401), wrap the entity rendering:

Change the resource entity `<a>` block:

```html
<template x-for="entity in result.resources" :key="entity.ID">
    <template x-if="entity.renderedHTML">
        <div x-html="entity.renderedHTML"></div>
    </template>
    <template x-if="!entity.renderedHTML">
        <a :href="'/resource?id=' + entity.ID" ...existing markup...>
            ...existing content...
        </a>
    </template>
</template>
```

Apply the same pattern to note and group result sections, and to bucketed result items.

- [ ] **Step 7: Build and verify**

Run: `npm run build && go build --tags 'json1 fts5'`
Expected: Both JS and Go build succeed.

- [ ] **Step 8: Commit**

```bash
git add server/api_handlers/mrql_api_handlers.go src/components/mrqlEditor.js templates/mrql.tpl models/resource_model.go models/note_model.go models/group_model.go
git commit -m "feat: add render=1 API param for server-side custom template rendering on MRQL page"
```

---

### Task 10: Run all tests and fix breakage

**Files:** Various (depends on what breaks)

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS. Fix any compilation errors from the `Process()` signature change. Any file that calls `shortcodes.Process()` directly (test files, other packages) needs the updated signature.

Common fix: any call site like `shortcodes.Process(input, ctx, renderer)` must become `shortcodes.Process(context.Background(), input, ctx, renderer, nil)`.

- [ ] **Step 2: Run E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: PASS. The new shortcodes should not break existing functionality since they only activate when `[mrql]` or `[property]` markers are present.

- [ ] **Step 3: Fix any failures**

Address any test failures. Common issues:
- Template rendering errors from missing `descriptionEntity` variable
- Compilation errors from updated `Process()` signature
- Missing imports

- [ ] **Step 4: Commit fixes**

```bash
git add -A
git commit -m "fix: resolve test breakage from MRQL shortcode integration"
```

---

### Task 11: Run Postgres tests

**Files:** None (test-only)

- [ ] **Step 1: Run Go Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
Expected: PASS.

- [ ] **Step 2: Run E2E Postgres tests**

Run: `cd e2e && npm run test:with-server:postgres`
Expected: PASS.

- [ ] **Step 3: Fix any Postgres-specific failures**

The `CustomMRQLResult` column migration should work identically on Postgres. If there are issues, they'll likely be in the MRQL query execution path.

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: resolve Postgres test issues for MRQL shortcodes"
```
