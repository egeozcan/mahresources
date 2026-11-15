package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Param is one OpenAPI parameter of an operation.
type Param struct {
	Name     string
	In       string // "query", "path" or "header"
	Required bool
	Schema   map[string]any
}

// Body describes an operation's request body.
type Body struct {
	ContentType string
	Schema      map[string]any // resolved, no $ref
	// FileFields are multipart fields whose schema is a binary string (or an
	// array of them). Callers pass them as {name, contentBase64} objects.
	FileFields map[string]bool // value: true when the field takes several files
	// Nested is true when the body is passed under the "body" argument instead
	// of being flattened next to the parameters (a name collision, or a body
	// that is not an object).
	Nested bool
}

// ResponseKind says how a successful response is returned to the client.
type ResponseKind int

const (
	// ResponseJSON responses are returned as structured content.
	ResponseJSON ResponseKind = iota
	// ResponseText responses (text/html, text/plain) are returned as text.
	ResponseText
	// ResponseImage responses are dispatched with a small cap and inlined.
	ResponseImage
	// ResponseLink responses are never dispatched; the tool returns a
	// resource_link to the guarded URL instead.
	ResponseLink
)

// linkOnly operations return file bodies of any size.
var linkOnly = map[string]bool{
	"viewResource":           true,
	"getVersionFile":         true,
	"downloadGroupExport":    true,
	"openCanonicalJobOutput": true,
}

// inlineImages operations return thumbnails small enough to inline.
var inlineImages = map[string]bool{
	"getResourcePreview":  true,
	"getVersionThumbnail": true,
}

// Operation is one API operation, as MCP sees it.
type Operation struct {
	ID          string
	Method      string
	Path        string
	Summary     string
	Description string
	Tags        []string
	Safety      Safety
	OpenWorld   bool
	Params      []Param
	Body        *Body
	Response    ResponseKind
	// InputSchema is the JSON Schema of the arguments call_operation takes.
	InputSchema map[string]any
}

// Catalog indexes operations by ID.
type Catalog struct {
	ops   map[string]*Operation
	order []string
}

// Get returns an exposed operation. Excluded operations are not returned.
func (c *Catalog) Get(id string) (*Operation, bool) {
	op, ok := c.ops[id]
	if !ok || op.Safety == Excluded {
		return nil, false
	}
	return op, true
}

// Lookup returns any operation, excluded or not.
func (c *Catalog) Lookup(id string) (*Operation, bool) {
	op, ok := c.ops[id]
	return op, ok
}

// Exposed returns every operation MCP exposes, sorted by ID.
func (c *Catalog) Exposed() []*Operation {
	var out []*Operation
	for _, id := range c.order {
		if op := c.ops[id]; op.Safety != Excluded {
			out = append(out, op)
		}
	}
	return out
}

// All returns every operation, excluded or not, sorted by ID.
func (c *Catalog) All() []*Operation {
	out := make([]*Operation, 0, len(c.order))
	for _, id := range c.order {
		out = append(out, c.ops[id])
	}
	return out
}

// BuildCatalog converts the generated OpenAPI spec into a catalog. It fails
// when a non-GET operation has no safety class, so an unclassified route can
// never become reachable by accident.
func BuildCatalog(spec *openapi3.T) (*Catalog, error) {
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("marshal spec: %w", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal spec: %w", err)
	}
	components, _ := mapAt(doc, "components", "schemas")
	r := &resolver{schemas: components}

	paths, _ := mapAt(doc, "paths")
	c := &Catalog{ops: map[string]*Operation{}}
	var unclassified []string
	for path, item := range paths {
		methods, _ := item.(map[string]any)
		for method, rawOp := range methods {
			m := strings.ToUpper(method)
			switch m {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			default:
				continue
			}
			o, _ := rawOp.(map[string]any)
			id, _ := o["operationId"].(string)
			if id == "" {
				return nil, fmt.Errorf("%s %s has no operationId", m, path)
			}
			if _, dup := c.ops[id]; dup {
				return nil, fmt.Errorf("duplicate operationId %q", id)
			}
			op, err := buildOperation(r, id, m, path, o)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", id, err)
			}
			if op.Safety == 0 {
				unclassified = append(unclassified, id)
			}
			c.ops[id] = op
			c.order = append(c.order, id)
		}
	}
	sort.Strings(c.order)
	if len(unclassified) > 0 {
		sort.Strings(unclassified)
		return nil, fmt.Errorf("operations without an MCP safety class (add them to server/mcpserver/safety.go): %s",
			strings.Join(unclassified, ", "))
	}
	return c, nil
}

func buildOperation(r *resolver, id, method, path string, o map[string]any) (*Operation, error) {
	op := &Operation{
		ID:        id,
		Method:    method,
		Path:      path,
		Safety:    classify(id, method),
		OpenWorld: openWorld[id],
	}
	op.Summary, _ = o["summary"].(string)
	op.Description, _ = o["description"].(string)
	if tags, ok := o["tags"].([]any); ok {
		for _, t := range tags {
			if s, ok := t.(string); ok {
				op.Tags = append(op.Tags, s)
			}
		}
	}

	if params, ok := o["parameters"].([]any); ok {
		for _, p := range params {
			pm, _ := r.deref(p).(map[string]any)
			name, _ := pm["name"].(string)
			in, _ := pm["in"].(string)
			if name == "" || (in != "query" && in != "path" && in != "header") {
				continue
			}
			req, _ := pm["required"].(bool)
			schema, _ := r.resolve(pm["schema"], 0).(map[string]any)
			if schema == nil {
				schema = map[string]any{"type": "string"}
			}
			if d, ok := pm["description"].(string); ok && d != "" {
				schema = withDescription(schema, d)
			}
			op.Params = append(op.Params, Param{Name: name, In: in, Required: req || in == "path", Schema: schema})
		}
	}

	if rb, ok := o["requestBody"].(map[string]any); ok {
		body, err := buildBody(r, rb)
		if err != nil {
			return nil, err
		}
		op.Body = body
	}

	op.Response = responseKind(id, o)
	op.InputSchema = inputSchema(op)
	return op, nil
}

func buildBody(r *resolver, rb map[string]any) (*Body, error) {
	content, _ := rb["content"].(map[string]any)
	for _, ct := range []string{"application/json", "application/x-www-form-urlencoded", "multipart/form-data"} {
		media, ok := content[ct].(map[string]any)
		if !ok {
			continue
		}
		schema, _ := r.resolve(media["schema"], 0).(map[string]any)
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		b := &Body{ContentType: ct, Schema: schema}
		if ct == "multipart/form-data" {
			b.FileFields = map[string]bool{}
			props, _ := schema["properties"].(map[string]any)
			for name, p := range props {
				ps, _ := p.(map[string]any)
				if isBinary(ps) {
					b.FileFields[name] = false
					props[name] = fileArgSchema(false)
				} else if items, ok := ps["items"].(map[string]any); ok && isBinary(items) {
					b.FileFields[name] = true
					props[name] = fileArgSchema(true)
				}
			}
		}
		return b, nil
	}
	if len(content) == 0 {
		return nil, nil
	}
	var types []string
	for ct := range content {
		types = append(types, ct)
	}
	sort.Strings(types)
	return nil, fmt.Errorf("unsupported request body content types %v", types)
}

func isBinary(s map[string]any) bool {
	if s == nil {
		return false
	}
	t, _ := s["type"].(string)
	f, _ := s["format"].(string)
	return t == "string" && f == "binary"
}

func fileArgSchema(multiple bool) map[string]any {
	file := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":          map[string]any{"type": "string", "description": "File name, including the extension"},
			"contentBase64": map[string]any{"type": "string", "description": "File content, base64-encoded"},
			"contentType":   map[string]any{"type": "string", "description": "MIME type (optional)"},
		},
		"required": []any{"name", "contentBase64"},
	}
	if multiple {
		return map[string]any{"type": "array", "items": file}
	}
	return file
}

func responseKind(id string, o map[string]any) ResponseKind {
	if linkOnly[id] {
		return ResponseLink
	}
	if inlineImages[id] {
		return ResponseImage
	}
	responses, _ := o["responses"].(map[string]any)
	sawText := false
	for code, resp := range responses {
		if !strings.HasPrefix(code, "2") {
			continue
		}
		rm, _ := resp.(map[string]any)
		content, _ := rm["content"].(map[string]any)
		for ct := range content {
			if strings.Contains(ct, "json") {
				return ResponseJSON
			}
			if strings.HasPrefix(ct, "text/") {
				sawText = true
			}
		}
	}
	if sawText {
		return ResponseText
	}
	return ResponseJSON
}

// inputSchema merges the parameters and the request body into one object
// schema. Body fields are flattened next to the parameters unless a name
// collides or the body is not an object; then the body goes under "body".
func inputSchema(op *Operation) map[string]any {
	props := map[string]any{}
	var required []any
	for _, p := range op.Params {
		props[p.Name] = p.Schema
		if p.Required {
			required = append(required, p.Name)
		}
	}
	if op.Body != nil {
		bodyProps, isObject := op.Body.Schema["properties"].(map[string]any)
		collides := false
		for name := range bodyProps {
			if _, ok := props[name]; ok || name == "body" {
				collides = true
			}
		}
		if !isObject || collides {
			op.Body.Nested = true
			props["body"] = op.Body.Schema
		} else {
			for name, s := range bodyProps {
				props[name] = s
			}
			if req, ok := op.Body.Schema["required"].([]any); ok {
				required = append(required, req...)
			}
		}
	}
	schema := map[string]any{
		"type":       "object",
		"properties": props,
		// Some registry entries omit inputs their handlers read, so extra
		// arguments are passed through rather than refused.
		"additionalProperties": true,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

// resolver inlines $ref schemas from components/schemas and rewrites the
// OpenAPI 3.0 "nullable" keyword into a JSON Schema type union.
type resolver struct {
	schemas map[string]any
}

const maxSchemaDepth = 12

func (r *resolver) deref(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	if ref, ok := m["$ref"].(string); ok {
		name := strings.TrimPrefix(ref, "#/components/schemas/")
		if target, ok := r.schemas[name]; ok {
			return target
		}
	}
	return m
}

func (r *resolver) resolve(v any, depth int) any {
	if depth > maxSchemaDepth {
		return map[string]any{"type": "object"}
	}
	switch t := v.(type) {
	case map[string]any:
		if ref, ok := t["$ref"].(string); ok {
			name := strings.TrimPrefix(ref, "#/components/schemas/")
			target, ok := r.schemas[name]
			if !ok {
				return map[string]any{"type": "object"}
			}
			return r.resolve(target, depth+1)
		}
		out := make(map[string]any, len(t))
		for k, val := range t {
			switch k {
			case "properties", "patternProperties":
				if pm, ok := val.(map[string]any); ok {
					props := make(map[string]any, len(pm))
					for name, ps := range pm {
						props[name] = r.resolve(ps, depth+1)
					}
					out[k] = props
					continue
				}
			case "nullable", "example", "xml", "externalDocs", "discriminator":
				continue
			}
			out[k] = r.resolve(val, depth+1)
		}
		if nullable, _ := t["nullable"].(bool); nullable {
			if typ, ok := out["type"].(string); ok {
				out["type"] = []any{typ, "null"}
			}
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = r.resolve(val, depth+1)
		}
		return out
	default:
		return v
	}
}

func withDescription(schema map[string]any, d string) map[string]any {
	out := make(map[string]any, len(schema)+1)
	for k, v := range schema {
		out[k] = v
	}
	if _, ok := out["description"]; !ok {
		out["description"] = d
	}
	return out
}

func mapAt(doc map[string]any, keys ...string) (map[string]any, bool) {
	cur := doc
	for _, k := range keys {
		next, ok := cur[k].(map[string]any)
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}
