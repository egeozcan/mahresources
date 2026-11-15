package mcpserver

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"sort"
	"strings"
)

// Response is what a dispatched operation produced.
type Response struct {
	Status    int
	Header    http.Header
	Body      []byte
	Truncated bool
}

// forwardedHeaders are copied from the /mcp request onto each dispatched
// request, so authentication, client-IP trust and rate limits see the same
// caller the REST API would.
var forwardedHeaders = []string{
	"Authorization",
	"User-Agent",
	"X-Forwarded-For",
	"X-Forwarded-Proto",
	"X-Forwarded-Host",
	"X-Real-Ip",
}

// dispatch runs op through handler as the caller of parent and records at most
// limit bytes of the response. A response that would exceed the limit is cut
// off and its handler's context cancelled.
func dispatch(handler http.Handler, parent *http.Request, op *Operation, args map[string]any, limit int64) (*Response, error) {
	ctx, cancel := context.WithCancel(parent.Context())
	defer cancel()
	req, err := buildRequest(ctx, parent, op, args)
	if err != nil {
		return nil, err
	}
	if req.URL.Path == mcpPath {
		return nil, errors.New("refusing to dispatch to the MCP endpoint itself")
	}
	rec := &boundedRecorder{header: http.Header{}, limit: limit, cancel: cancel}
	handler.ServeHTTP(rec, req)
	if rec.status == 0 {
		rec.status = http.StatusOK
	}
	return &Response{Status: rec.status, Header: rec.header, Body: rec.buf.Bytes(), Truncated: rec.truncated}, nil
}

// buildRequest turns tool arguments into the HTTP request the REST API expects.
func buildRequest(ctx context.Context, parent *http.Request, op *Operation, args map[string]any) (*http.Request, error) {
	remaining := make(map[string]any, len(args))
	for k, v := range args {
		remaining[k] = v
	}

	path, rawPath := op.Path, op.Path
	query := url.Values{}
	header := http.Header{}
	for _, p := range op.Params {
		v, ok := remaining[p.Name]
		delete(remaining, p.Name)
		if !ok || v == nil {
			if p.Required {
				return nil, fmt.Errorf("missing required argument %q", p.Name)
			}
			continue
		}
		switch p.In {
		case "path":
			s, err := scalarString(v)
			if err != nil {
				return nil, fmt.Errorf("argument %q: %w", p.Name, err)
			}
			// A path segment must stay one segment: a slash or a dot
			// segment would let an argument address a different route.
			if s == "" || s == "." || s == ".." || strings.ContainsAny(s, "/\\") {
				return nil, fmt.Errorf("argument %q is not a valid path segment", p.Name)
			}
			path = strings.ReplaceAll(path, "{"+p.Name+"}", s)
			rawPath = strings.ReplaceAll(rawPath, "{"+p.Name+"}", url.PathEscape(s))
		case "header":
			s, err := scalarString(v)
			if err != nil {
				return nil, fmt.Errorf("argument %q: %w", p.Name, err)
			}
			header.Set(p.Name, s)
		default:
			if err := checkStringItems(p, v); err != nil {
				return nil, err
			}
			if err := addValues(query, p.Name, v); err != nil {
				return nil, fmt.Errorf("argument %q: %w", p.Name, err)
			}
		}
	}

	// What is left belongs to the body. A nested body arrives whole under
	// "body"; anything else left over is passed through, because some registry
	// entries omit inputs their handlers read.
	var bodyArgs map[string]any
	explicitBody := false
	if op.Body != nil && op.Body.Nested {
		if b, ok := remaining["body"]; ok {
			delete(remaining, "body")
			bm, ok := b.(map[string]any)
			if !ok {
				return nil, errors.New(`argument "body" must be an object`)
			}
			bodyArgs = bm
		}
		for k, v := range remaining {
			if bodyArgs == nil {
				bodyArgs = map[string]any{}
			}
			bodyArgs[k] = v
		}
	} else {
		bodyArgs = remaining
		// An operation with no declared body takes explicit body fields under
		// "body" (see inputSchema); they join the leftovers. An explicit body
		// is sent even when empty: {} can mean "clear every field".
		if b, ok := remaining["body"]; ok && op.Body == nil && op.Method != http.MethodGet {
			explicitBody = true
			bm, ok := b.(map[string]any)
			if !ok {
				return nil, errors.New(`argument "body" must be an object`)
			}
			bodyArgs = make(map[string]any, len(remaining)+len(bm))
			for k, v := range remaining {
				if k != "body" {
					bodyArgs[k] = v
				}
			}
			for k, v := range bm {
				if _, dup := bodyArgs[k]; dup {
					return nil, fmt.Errorf("argument %q is given both inside and outside \"body\"", k)
				}
				bodyArgs[k] = v
			}
		}
	}

	// MRQL reads its named bind values from "param.<name>" keys in the query
	// string or a form body, never from JSON, so for the MRQL routes they
	// always travel in the query (the JSON form is the "params" object). Other
	// operations keep such keys where they are: a plugin setting may be
	// named "param.limit".
	if strings.HasPrefix(op.Path, "/v1/mrql") {
		// A fresh map: bodyArgs can be the caller's own "body" object, which
		// a confirmation preview and the real dispatch both read.
		withoutParams := make(map[string]any, len(bodyArgs))
		for k, v := range bodyArgs {
			if strings.HasPrefix(k, "param.") {
				if err := addValues(query, k, v); err != nil {
					return nil, fmt.Errorf("argument %q: %w", k, err)
				}
				continue
			}
			withoutParams[k] = v
		}
		bodyArgs = withoutParams
	}

	var body []byte
	contentType := ""
	switch {
	case op.Method == http.MethodGet || op.Body == nil && len(bodyArgs) == 0 && !explicitBody:
		// A GET has no body. Leftover GET arguments are filters the registry
		// did not declare, which the query decoders read.
		for k, v := range bodyArgs {
			if err := addValues(query, k, v); err != nil {
				return nil, fmt.Errorf("argument %q: %w", k, err)
			}
		}
	case op.Body == nil:
		// No declared body, but arguments are left: the registry entry omits
		// the handler's body type. tryFillStructValuesFromRequest, which nearly
		// every handler uses, decodes JSON, and JSON keeps nested objects. A
		// DELETE's declared parameters are in the query already; what is left
		// is body, such as resetRuntimeSetting's audit reason.
		b, err := json.Marshal(bodyArgs)
		if err != nil {
			return nil, err
		}
		body, contentType = b, "application/json"
	case op.Body.ContentType == "application/json":
		b, err := json.Marshal(bodyArgs)
		if err != nil {
			return nil, err
		}
		body, contentType = b, "application/json"
	case op.Body.ContentType == "multipart/form-data":
		b, ct, err := multipartBody(op.Body, bodyArgs)
		if err != nil {
			return nil, err
		}
		body, contentType = b, ct
	default:
		form := url.Values{}
		for k, v := range bodyArgs {
			if err := addValues(form, k, v); err != nil {
				return nil, fmt.Errorf("argument %q: %w", k, err)
			}
		}
		body, contentType = []byte(form.Encode()), "application/x-www-form-urlencoded"
	}

	u := &url.URL{Path: path, RawPath: rawPath, RawQuery: query.Encode()}
	req, err := http.NewRequestWithContext(ctx, op.Method, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Host = parent.Host
	req.RemoteAddr = parent.RemoteAddr
	req.TLS = parent.TLS
	for _, h := range forwardedHeaders {
		if v := parent.Header.Values(h); len(v) > 0 {
			req.Header[textproto.CanonicalMIMEHeaderKey(h)] = append([]string(nil), v...)
		}
	}
	for k, v := range header {
		req.Header[k] = v
	}
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

// checkStringItems refuses an object where a query parameter's schema asks
// for string items, rather than sending its JSON text for the handler to
// misread as a filter.
func checkStringItems(p Param, v any) error {
	items, _ := p.Schema["items"].(map[string]any)
	if p.Schema["type"] != "array" || items["type"] != "string" {
		return nil
	}
	list, ok := v.([]any)
	if !ok {
		list = []any{v}
	}
	for _, item := range list {
		if _, isObj := item.(map[string]any); isObj {
			return fmt.Errorf("argument %q takes strings, not objects; see its schema for the format", p.Name)
		}
	}
	return nil
}

func multipartBody(b *Body, args map[string]any) ([]byte, string, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := args[k]
		if multiple, isFile := b.FileFields[k]; isFile {
			var files []any
			if multiple {
				list, ok := v.([]any)
				if !ok {
					return nil, "", fmt.Errorf("argument %q must be a list of files", k)
				}
				files = list
			} else {
				files = []any{v}
			}
			for i, f := range files {
				if err := writeFilePart(w, k, f); err != nil {
					return nil, "", fmt.Errorf("argument %q[%d]: %w", k, i, err)
				}
			}
			continue
		}
		vals := url.Values{}
		if err := addValues(vals, k, v); err != nil {
			return nil, "", fmt.Errorf("argument %q: %w", k, err)
		}
		for _, s := range vals[k] {
			if err := w.WriteField(k, s); err != nil {
				return nil, "", err
			}
		}
	}
	if err := w.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), w.FormDataContentType(), nil
}

func writeFilePart(w *multipart.Writer, field string, f any) error {
	fm, ok := f.(map[string]any)
	if !ok {
		return errors.New("a file must be an object with name and contentBase64")
	}
	name, _ := fm["name"].(string)
	enc, _ := fm["contentBase64"].(string)
	if name == "" {
		return errors.New("file name is required")
	}
	data, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return fmt.Errorf("contentBase64 is not valid base64: %w", err)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, name))
	ct, _ := fm["contentType"].(string)
	if ct == "" {
		ct = "application/octet-stream"
	}
	h.Set("Content-Type", ct)
	part, err := w.CreatePart(h)
	if err != nil {
		return err
	}
	_, err = part.Write(data)
	return err
}

// addValues encodes v the way gorilla/schema and the query-param handlers
// decode it: a list repeats the key, an object is sent as JSON text.
func addValues(vals url.Values, key string, v any) error {
	switch t := v.(type) {
	case nil:
		return nil
	case []any:
		for _, item := range t {
			if _, isObj := item.(map[string]any); isObj {
				b, err := json.Marshal(item)
				if err != nil {
					return err
				}
				vals.Add(key, string(b))
				continue
			}
			s, err := scalarString(item)
			if err != nil {
				return err
			}
			vals.Add(key, s)
		}
		return nil
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		vals.Add(key, string(b))
		return nil
	default:
		s, err := scalarString(v)
		if err != nil {
			return err
		}
		vals.Add(key, s)
		return nil
	}
}

func scalarString(v any) (string, error) {
	switch t := v.(type) {
	case string:
		return t, nil
	case json.Number:
		return t.String(), nil
	case bool:
		if t {
			return "true", nil
		}
		return "false", nil
	case float64:
		return fmt.Sprintf("%v", t), nil
	case int:
		return fmt.Sprintf("%d", t), nil
	case uint:
		return fmt.Sprintf("%d", t), nil
	}
	return "", fmt.Errorf("expected a scalar value, got %T", v)
}

var errResponseTooLarge = errors.New("response exceeds the MCP capture limit")

// boundedRecorder records a response up to limit bytes. Past the limit it
// stops accepting writes and cancels the handler's context.
type boundedRecorder struct {
	header    http.Header
	status    int
	buf       bytes.Buffer
	limit     int64
	truncated bool
	cancel    context.CancelFunc
}

func (r *boundedRecorder) Header() http.Header { return r.header }

func (r *boundedRecorder) WriteHeader(code int) {
	if r.status == 0 {
		r.status = code
	}
}

func (r *boundedRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	if r.truncated {
		return 0, errResponseTooLarge
	}
	if int64(r.buf.Len()+len(p)) > r.limit {
		r.truncated = true
		r.cancel()
		return 0, errResponseTooLarge
	}
	return r.buf.Write(p)
}

// Flush satisfies http.Flusher for handlers that stream; the recorder has
// nothing to flush.
func (r *boundedRecorder) Flush() {}
