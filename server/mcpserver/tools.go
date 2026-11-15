package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"mahresources/auth"
)

const (
	// thumbnailLimit caps an inlined thumbnail; a larger one becomes a link.
	thumbnailLimit = 512 << 10
	// previewLookupLimit caps the per-ID lookups a confirmation preview makes.
	previewLookupLimit = 25
	// previewArgsLimit caps the arguments of a destructive call, which its
	// confirmation shows in full.
	previewArgsLimit = 16 << 10
	// confirmKey is the input request ID of a confirmation.
	confirmKey = "confirm"
	// protocolStateless is the first protocol version whose clients carry
	// their capabilities per request and fulfil input requests themselves.
	protocolStateless = "2026-07-28"
)

// caller runs tools for one /mcp HTTP request.
type caller struct {
	h *Handler
	r *http.Request
}

// namedTool is a convenience tool that resolves to one catalog operation.
type namedTool struct {
	tool    *mcp.Tool
	resolve func(args map[string]any) (*Operation, map[string]any, error)
}

func (c *caller) named(t namedTool) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decodeArgs(req.Params.Arguments)
		if err != nil {
			return toolError(err.Error()), nil
		}
		op, opArgs, err := t.resolve(args)
		if err != nil {
			return toolError(err.Error()), nil
		}
		return c.run(ctx, req, t.tool.Name, op, opArgs), nil
	}
}

func (c *caller) generic(name string) mcp.ToolHandler {
	return func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args, err := decodeArgs(req.Params.Arguments)
		if err != nil {
			return toolError(err.Error()), nil
		}
		switch name {
		case "search_operations":
			return c.searchOperations(args), nil
		case "describe_operation":
			id, _ := args["operationId"].(string)
			op, ok := c.h.catalog.Get(id)
			if !ok {
				return toolError(fmt.Sprintf("unknown operation %q; use search_operations to find one", id)), nil
			}
			return jsonResult(describe(op)), nil
		case "call_operation":
			id, _ := args["operationId"].(string)
			op, ok := c.h.catalog.Get(id)
			if !ok {
				return toolError(fmt.Sprintf("unknown operation %q; use search_operations to find one", id)), nil
			}
			opArgs := map[string]any{}
			if a, ok := args["arguments"]; ok && a != nil {
				m, ok := a.(map[string]any)
				if !ok {
					return toolError(`"arguments" must be an object`), nil
				}
				opArgs = m
			}
			return c.run(ctx, req, name, op, opArgs), nil
		}
		return toolError("unknown tool " + name), nil
	}
}

// run executes one operation, behind a confirmation when it is destructive.
func (c *caller) run(_ context.Context, req *mcp.CallToolRequest, toolName string, op *Operation, args map[string]any) *mcp.CallToolResult {
	if op.Safety == Excluded {
		return toolError(fmt.Sprintf("%s is not available over MCP", op.ID))
	}
	// Handlers decode JSON case-insensitively, so every guard, hash and
	// preview below must see each argument under its one declared name.
	args, err := canonicalArgs(op, args)
	if err != nil {
		return toolError(err.Error())
	}
	if err := refuseInexpressible(op, args); err != nil {
		return toolError(err.Error())
	}
	if effectiveSafety(op, args) != Destructive {
		return c.execute(op, args)
	}

	if err := refuseUnpinnedSelection(op, args); err != nil {
		return toolError(err.Error())
	}
	if err := refuseBodyShadowing(op, args); err != nil {
		return toolError(err.Error())
	}
	argsHash, err := hashArgs(args)
	if err != nil {
		return toolError(err.Error())
	}
	// The confirmation shows every argument it covers, unclipped, so it can
	// only cover what fits in one.
	if raw, _ := json.Marshal(args); len(raw) > previewArgsLimit {
		return toolError(fmt.Sprintf("These arguments are too large to show in a confirmation (%d bytes, limit %d), so the call was refused. Split it into smaller calls.", len(raw), previewArgsLimit))
	}
	want := challenge{
		UserID:   principalID(c.r),
		AuthHash: hashString(c.r.Header.Get("Authorization")),
		Tool:     toolName,
		OpID:     op.ID,
		ArgsHash: argsHash,
	}

	if req.Params.RequestState != "" || len(req.Params.InputResponses) > 0 {
		answer, _ := req.Params.InputResponses[confirmKey].(*mcp.ElicitResult)
		if answer == nil {
			return toolError("the confirmation response is missing")
		}
		if answer.Action != "accept" || answer.Content["confirm"] != true {
			return textResult(fmt.Sprintf("Cancelled: %s was not run, and nothing was changed.", op.ID))
		}
		if err := c.h.confirmer.redeem(req.Params.RequestState, want); err != nil {
			return toolError(err.Error())
		}
		return c.execute(op, args)
	}

	preview := c.preview(op, args)
	if !canConfirm(req) {
		return toolError(preview + "\n\nThis action needs a confirmation this client cannot show, so it was refused and nothing was changed. " +
			"Use an MCP client that supports elicitation (protocol " + protocolStateless + "), the `mr mcp` bridge, or the REST API.")
	}
	state, err := c.h.confirmer.issue(want)
	if err != nil {
		return toolError(err.Error())
	}
	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			confirmKey: &mcp.ElicitParams{
				Mode:    "form",
				Message: preview,
				RequestedSchema: &jsonschema.Schema{
					Type: "object",
					Properties: map[string]*jsonschema.Schema{
						"confirm": {
							Type:        "boolean",
							Title:       "Run this action",
							Description: "Tick to run it. It cannot be undone from here.",
						},
					},
					Required: []string{"confirm"},
				},
			},
		},
		RequestState: state,
	}
}

// canConfirm reports whether the client can show the user a confirmation and
// answer it with a retry. That needs the 2026-07-28 protocol (a stateless
// server cannot send an older client a request) and form elicitation.
func canConfirm(req *mcp.CallToolRequest) bool {
	meta := req.Params.GetMeta()
	v, _ := meta[mcp.MetaKeyProtocolVersion].(string)
	if v < protocolStateless {
		return false
	}
	caps := req.ClientCapabilities()
	if caps == nil || caps.Elicitation == nil {
		return false
	}
	// An empty elicitation capability means form mode.
	return caps.Elicitation.Form != nil || caps.Elicitation.URL == nil
}

// canonicalArgs renames each argument to the operation's declared name for
// it, matched case-insensitively, the way encoding/json matches struct
// fields. Two arguments that name the same field ("Id" and "id") are refused:
// which one a handler would read is a decoder detail, and a confirmation must
// not depend on it. Undeclared arguments keep their names but are held to the
// same one-spelling rule.
func canonicalArgs(op *Operation, args map[string]any) (map[string]any, error) {
	props, _ := op.InputSchema["properties"].(map[string]any)
	out, err := canonicalKeys(props, args)
	if err != nil {
		return nil, err
	}
	if op.Body != nil && op.Body.Nested {
		if body, ok := out["body"].(map[string]any); ok {
			bodyProps, _ := op.Body.Schema["properties"].(map[string]any)
			if out["body"], err = canonicalKeys(bodyProps, body); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func canonicalKeys(props map[string]any, args map[string]any) (map[string]any, error) {
	declared := map[string]string{} // folded -> declared name; "" when two declared names fold together
	for name := range props {
		folded := foldKey(name)
		if prev, ok := declared[folded]; ok && prev != name {
			declared[folded] = ""
			continue
		}
		declared[folded] = name
	}
	out := make(map[string]any, len(args))
	from := map[string]string{} // canonical name -> the spelling it came from
	for key, v := range args {
		name := key
		if _, exact := props[key]; !exact {
			if d, ok := declared[foldKey(key)]; ok {
				if d == "" {
					return nil, fmt.Errorf("argument %q is ambiguous: the operation declares several fields with that name in different cases; use the exact name", key)
				}
				name = d
			}
		}
		folded := foldKey(name)
		if prev, dup := from[folded]; dup {
			return nil, fmt.Errorf("argument %q is given twice (as %q and %q); pass it once", name, prev, key)
		}
		from[folded] = key
		out[name] = v
	}
	return out, nil
}

// foldKey maps s to one representative of its Unicode simple-fold class,
// rune by rune: two names get the same key exactly when strings.EqualFold
// reports them equal, which is how encoding/json matches object keys to
// struct fields. ASCII case folding alone is not enough: "Loſers" (U+017F)
// and the Kelvin sign both reach ASCII fields.
func foldKey(s string) string {
	var b strings.Builder
	for _, r := range s {
		lowest := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			if f < lowest {
				lowest = f
			}
		}
		b.WriteRune(lowest)
	}
	return b.String()
}

// effectiveSafety upgrades an operation whose safety depends on its
// arguments.
//
//   - createOrUpdateGroup with an ID updates a group, and every update
//     re-derives the group's category and deletes the relations that no
//     longer fit it. A create touches no existing relation.
func effectiveSafety(op *Operation, args map[string]any) Safety {
	switch op.ID {
	case "createOrUpdateGroup":
		if v, ok := args["ID"]; ok && !isZeroArg(v) {
			return Destructive
		}
	}
	return op.Safety
}

func isZeroArg(v any) bool {
	if v == nil {
		return true
	}
	s, err := scalarString(v)
	return err == nil && (s == "0" || s == "")
}

// refuseInexpressible refuses arguments the dispatched request cannot carry
// with the meaning the caller gave them. editResource tells "SeriesId: 0"
// (leave the series) from an omitted SeriesId only in a form body, and
// dispatch sends JSON, where the handler keeps the current series and reports
// success.
func refuseInexpressible(op *Operation, args map[string]any) error {
	if op.ID == "editResource" {
		if v, ok := args["SeriesId"]; ok && isZeroArg(v) {
			return errors.New("editResource cannot remove a resource from its series over MCP; call removeResourceFromSeries with the resource's id instead")
		}
	}
	return nil
}

// refuseBodyShadowing refuses a destructive call whose "body" passthrough
// carries a field that names another argument. Both reach the handler, and
// the handler may prefer the body copy (deleteNote reads a body ID before its
// Id parameter), so the confirmation would name one target and the call
// delete another. Non-destructive calls may shadow on purpose: a saved
// query's bind value can be called "name" like runQuery's lookup parameter.
// args must be canonical.
func refuseBodyShadowing(op *Operation, args map[string]any) error {
	if op.Body != nil {
		return nil
	}
	body, ok := args["body"].(map[string]any)
	if !ok {
		return nil
	}
	props, _ := op.InputSchema["properties"].(map[string]any)
	top := map[string]string{}
	for k := range args {
		if k != "body" {
			top[foldKey(k)] = k
		}
	}
	for name := range props {
		if name != "body" {
			top[foldKey(name)] = name
		}
	}
	for k := range body {
		if other, dup := top[foldKey(k)]; dup {
			return fmt.Errorf("argument %q inside \"body\" duplicates %q; pass it once, outside \"body\"", k, other)
		}
	}
	_, err := canonicalKeys(nil, body)
	return err
}

// refuseUnpinnedSelection refuses destructive calls whose targets would be
// resolved again at execution time. A confirmation covers the sealed
// arguments, so the targets must be in them. args must be canonical.
func refuseUnpinnedSelection(op *Operation, args map[string]any) error {
	if !strings.HasPrefix(op.ID, "massEdit") {
		return nil
	}
	if target, _ := args["Target"].(string); target != "" && !strings.EqualFold(target, "ids") {
		return fmt.Errorf("%s over MCP must name its targets with ID; Target %q is refused, because the matching set could change between confirmation and execution. Resolve the IDs first (list or MRQL), then pass them as ID", op.ID, target)
	}
	for _, k := range []string{"Filter", "MRQLQuery", "MRQLSnapshot", "MRQLParams"} {
		if v, ok := args[k]; ok && v != nil && v != "" {
			return fmt.Errorf("%s over MCP must name its targets with ID; %s selection is refused, because the matching set could change between confirmation and execution. Resolve the IDs first (list or MRQL), then pass them as ID", op.ID, k)
		}
	}
	if ids, _ := args["ID"].([]any); len(ids) == 0 {
		return fmt.Errorf("%s over MCP needs a non-empty ID list", op.ID)
	}
	return nil
}

func principalID(r *http.Request) uint {
	p := auth.PrincipalFromContext(r.Context())
	if p == nil || p.SuperUser {
		return 0
	}
	return p.UserID
}

// execute dispatches op and turns the response into a tool result.
func (c *caller) execute(op *Operation, args map[string]any) *mcp.CallToolResult {
	switch op.Response {
	case ResponseLink:
		return c.linkResult(op, args)
	case ResponseImage:
		resp, err := dispatch(c.h.opts.Dispatch, c.r, op, args, thumbnailLimit)
		if err != nil {
			return toolError(err.Error())
		}
		ct := resp.Header.Get("Content-Type")
		if resp.Truncated || resp.Status >= 300 || !strings.HasPrefix(ct, "image/") {
			if resp.Status >= 400 {
				return httpError(resp)
			}
			return c.linkResult(op, args)
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.ImageContent{Data: resp.Body, MIMEType: ct}}}
	}

	resp, err := dispatch(c.h.opts.Dispatch, c.r, op, args, c.h.opts.MaxResponse)
	if err != nil {
		return toolError(err.Error())
	}
	if resp.Truncated {
		return toolError(fmt.Sprintf("The response is larger than %d bytes, so it was not returned. Narrow the request (a smaller page or limit, or a filter).", c.h.opts.MaxResponse))
	}
	if resp.Status >= 300 {
		return httpError(resp)
	}
	body := bytes.TrimSpace(resp.Body)
	if len(body) == 0 {
		return textResult(fmt.Sprintf("Done (HTTP %d).", resp.Status))
	}
	if op.Response == ResponseText || !json.Valid(body) {
		return textResult(string(body))
	}
	return jsonBytesResult(body)
}

// linkResult points at the guarded URL instead of returning a file body.
func (c *caller) linkResult(op *Operation, args map[string]any) *mcp.CallToolResult {
	req, err := buildRequest(context.Background(), c.r, op, args)
	if err != nil {
		return toolError(err.Error())
	}
	scheme := "http"
	if c.r.TLS != nil || strings.EqualFold(c.r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	u := url.URL{Scheme: scheme, Host: c.r.Host, Path: req.URL.Path, RawQuery: req.URL.RawQuery}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.ResourceLink{URI: u.String(), Name: op.ID, Description: op.Summary},
		&mcp.TextContent{Text: "Fetch this URL with the same Authorization header to get the file: " + u.String()},
	}}
}

// preview describes a destructive call for its confirmation. Every lookup is
// dispatched as the caller, so the preview can only name what the caller can
// already see.
func (c *caller) preview(op *Operation, args map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Confirm: %s\nOperation: %s (%s %s)\n", op.Summary, op.ID, op.Method, op.Path)

	lookup, entity := lookupFor(op.Path)
	for _, key := range []string{"Id", "ID", "id", "Ids", "Winner", "Losers"} {
		ids := idList(args[key])
		if len(ids) == 0 {
			continue
		}
		label := map[string]string{"Winner": "Keep", "Losers": "Merge into it and remove"}[key]
		if label == "" {
			label = "Targets"
		}
		fmt.Fprintf(&b, "%s (%d %s):\n", label, len(ids), entity)
		for i, id := range ids {
			if i == previewLookupLimit {
				fmt.Fprintf(&b, "  ... and %d more\n", len(ids)-previewLookupLimit)
				break
			}
			fmt.Fprintf(&b, "  #%s %s\n", id, c.nameOf(lookup, id))
		}
	}

	switch op.ID {
	case "applyResourceReduction":
		b.WriteString(c.reductionPreview(args))
	case "applyGroupImport":
		b.WriteString(c.importPreview(args))
	}

	if strings.HasPrefix(op.ID, "massEdit") {
		dry := make(map[string]any, len(args)+1)
		for k, v := range args {
			dry[k] = v
		}
		dry["DryRun"] = true
		if resp, err := dispatch(c.h.opts.Dispatch, c.r, op, dry, 64<<10); err == nil && resp.Status < 300 {
			fmt.Fprintf(&b, "Dry run: %s\n", clip(string(bytes.TrimSpace(resp.Body)), 1500))
		}
	}

	if raw, err := json.Marshal(args); err == nil && len(args) > 0 {
		fmt.Fprintf(&b, "Arguments: %s\n", raw)
	}
	return strings.TrimRight(b.String(), "\n")
}

// reductionPreview states what applying a Resource Reduction destroys: the
// checked Clusters across the whole plan and the Resources they would remove,
// read from the reduction page's JSON as the caller.
func (c *caller) reductionPreview(args map[string]any) string {
	id := idList(args["id"])
	if len(id) != 1 {
		return ""
	}
	page := &Operation{ID: "reductionPage", Method: http.MethodGet, Path: "/reduction.json",
		Params: []Param{{Name: "id", In: "query", Required: true}}}
	resp, err := dispatch(c.h.opts.Dispatch, c.r, page, map[string]any{"id": json.Number(id[0])}, c.h.opts.MaxResponse)
	if err != nil || resp.Truncated || resp.Status >= 300 {
		return "Reduction: not found or not visible\n"
	}
	var v struct {
		Raw struct {
			Name string `json:"name"`
		} `json:"raw"`
		Review struct {
			CheckedCount      int
			CheckedLoserCount int
		} `json:"review"`
	}
	if json.Unmarshal(resp.Body, &v) != nil {
		return ""
	}
	return fmt.Sprintf("Reduction %q: %d checked clusters; %d resources would be removed (merged into each cluster's winner).\n",
		clip(v.Raw.Name, 120), v.Review.CheckedCount, v.Review.CheckedLoserCount)
}

// importPreview states what applying a parsed import creates and, under the
// "replace" policy, overwrites.
func (c *caller) importPreview(args map[string]any) string {
	jobID, _ := args["jobId"].(string)
	op, ok := c.h.catalog.Get("getImportPlan")
	if jobID == "" || !ok {
		return ""
	}
	resp, err := dispatch(c.h.opts.Dispatch, c.r, op, map[string]any{"jobId": jobID}, c.h.opts.MaxResponse)
	if err != nil || resp.Truncated || resp.Status >= 300 {
		return "Import plan: not found or not visible\n"
	}
	var plan struct {
		Counts struct {
			Groups, Notes, Resources, Series int
		} `json:"counts"`
		Conflicts struct {
			GUIDMatches         int `json:"guid_matches"`
			ResourceGUIDMatches int `json:"resource_guid_matches"`
			ResourceHashMatches int `json:"resource_hash_matches"`
		} `json:"conflicts"`
	}
	if json.Unmarshal(resp.Body, &plan) != nil {
		return ""
	}
	policy, _ := args["guid_collision_policy"].(string)
	if policy == "" {
		policy = "(default)"
	}
	return fmt.Sprintf("Import: %d groups, %d notes, %d resources, %d series.\nExisting entities matched by GUID: %d (resources: %d by GUID, %d by hash). GUID collision policy: %s; \"replace\" overwrites the matched entities.\n",
		plan.Counts.Groups, plan.Counts.Notes, plan.Counts.Resources, plan.Counts.Series,
		plan.Conflicts.GUIDMatches, plan.Conflicts.ResourceGUIDMatches, plan.Conflicts.ResourceHashMatches, policy)
}

// lookupFor names the single-item GET operation for the entity family a path
// belongs to. Families without one are listed by ID only.
func lookupFor(path string) (opID, entity string) {
	seg := strings.Split(strings.TrimPrefix(path, "/v1/"), "/")
	first := seg[0]
	if first == "note" && len(seg) > 1 && (seg[1] == "block" || seg[1] == "noteType") {
		return "", seg[1]
	}
	switch first {
	case "resource", "resources":
		return "getResource", "resources"
	case "note", "notes":
		return "getNote", "notes"
	case "group", "groups":
		return "getGroup", "groups"
	case "query":
		return "getQuery", "queries"
	case "series":
		return "getSeries", "series"
	}
	return "", first
}

func (c *caller) nameOf(lookupID, id string) string {
	if lookupID == "" {
		return ""
	}
	op, ok := c.h.catalog.Get(lookupID)
	if !ok {
		return ""
	}
	resp, err := dispatch(c.h.opts.Dispatch, c.r, op, map[string]any{"id": json.Number(id)}, 256<<10)
	if err != nil || resp.Truncated || resp.Status >= 300 {
		return "(not found or not visible)"
	}
	var v map[string]any
	if json.Unmarshal(resp.Body, &v) != nil {
		return ""
	}
	for _, k := range []string{"Name", "name"} {
		if s, ok := v[k].(string); ok && s != "" {
			return fmt.Sprintf("%q", clip(s, 120))
		}
	}
	return ""
}

func idList(v any) []string {
	var out []string
	add := func(x any) {
		if s, err := scalarString(x); err == nil && s != "" && s != "0" {
			out = append(out, s)
		}
	}
	switch t := v.(type) {
	case nil:
	case []any:
		for _, x := range t {
			add(x)
		}
	default:
		add(t)
	}
	return out
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func (c *caller) searchOperations(args map[string]any) *mcp.CallToolResult {
	q := strings.ToLower(strings.TrimSpace(stringArg(args, "query")))
	tag := strings.ToLower(strings.TrimSpace(stringArg(args, "tag")))
	safety := strings.ToLower(strings.TrimSpace(stringArg(args, "safety")))
	var lines []string
	for _, op := range c.h.catalog.Exposed() {
		if tag != "" && !containsTag(op.Tags, tag) {
			continue
		}
		if safety != "" && op.Safety.String() != safety {
			continue
		}
		if q != "" {
			hay := strings.ToLower(op.ID + " " + op.Path + " " + op.Summary + " " + strings.Join(op.Tags, " "))
			match := true
			for _, word := range strings.Fields(q) {
				if !strings.Contains(hay, word) {
					match = false
					break
				}
			}
			if !match {
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("%s | %s %s | %s | %s", op.ID, op.Method, op.Path, op.Safety, op.Summary))
	}
	if len(lines) == 0 {
		return textResult("No operations match. Try fewer words, or no query to list everything.")
	}
	return textResult(fmt.Sprintf("%d operations (operationId | method path | safety | summary):\n%s", len(lines), strings.Join(lines, "\n")))
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if strings.ToLower(t) == tag {
			return true
		}
	}
	return false
}

func describe(op *Operation) map[string]any {
	kind := map[ResponseKind]string{ResponseJSON: "json", ResponseText: "text", ResponseImage: "image", ResponseLink: "link"}[op.Response]
	d := map[string]any{
		"operationId": op.ID,
		"method":      op.Method,
		"path":        op.Path,
		"summary":     op.Summary,
		"safety":      op.Safety.String(),
		"openWorld":   op.OpenWorld,
		"response":    kind,
		"inputSchema": op.InputSchema,
		"tags":        op.Tags,
	}
	if op.Description != "" {
		d["description"] = op.Description
	}
	if op.Safety == Destructive {
		d["confirmation"] = "required: the client asks the user before the operation runs"
	}
	return d
}

func stringArg(args map[string]any, k string) string {
	s, _ := args[k].(string)
	return s
}

func decodeArgs(raw json.RawMessage) (map[string]any, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return map[string]any{}, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var args map[string]any
	if err := dec.Decode(&args); err != nil {
		return nil, fmt.Errorf("arguments must be a JSON object: %w", err)
	}
	if args == nil {
		args = map[string]any{}
	}
	return args, nil
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

func textResult(s string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}
}

func httpError(resp *Response) *mcp.CallToolResult {
	msg := strings.TrimSpace(string(resp.Body))
	var v map[string]any
	if json.Unmarshal(resp.Body, &v) == nil {
		for _, k := range []string{"error", "message", "Error"} {
			if s, ok := v[k].(string); ok && s != "" {
				msg = s
				break
			}
		}
	}
	if msg == "" {
		msg = http.StatusText(resp.Status)
	}
	return toolError(fmt.Sprintf("HTTP %d: %s", resp.Status, clip(msg, 2000)))
}

func jsonResult(v any) *mcp.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError(err.Error())
	}
	return jsonBytesResult(b)
}

// jsonBytesResult returns JSON as text and, when it is an object, also as
// structured content. A top-level array is wrapped as {"items": [...]} for
// the structured copy, because structuredContent must be an object.
func jsonBytesResult(b []byte) *mcp.CallToolResult {
	res := &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
	var v any
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if dec.Decode(&v) == nil {
		switch t := v.(type) {
		case map[string]any:
			res.StructuredContent = t
		case []any:
			res.StructuredContent = map[string]any{"items": t}
		}
	}
	return res
}

func annotationsFor(s Safety, openWorld bool, method string) *mcp.ToolAnnotations {
	destructive := s == Destructive
	return &mcp.ToolAnnotations{
		ReadOnlyHint:    s == ReadOnly,
		DestructiveHint: &destructive,
		IdempotentHint:  method == http.MethodGet,
		OpenWorldHint:   &openWorld,
	}
}

func genericTools() []*mcp.Tool {
	t, f := true, false
	return []*mcp.Tool{
		{
			Name:  "search_operations",
			Title: "Search API operations",
			Description: "List the mahresources API operations reachable through call_operation. Every word of query must appear in the " +
				"operation ID, path, summary or tags. With no arguments, lists every operation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":  map[string]any{"type": "string", "description": "Words to match, e.g. \"resource version\""},
					"tag":    map[string]any{"type": "string", "description": "Only operations with this tag, e.g. resources, notes, groups, jobs"},
					"safety": map[string]any{"type": "string", "enum": []any{"read-only", "mutating", "destructive"}},
				},
			},
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &f, IdempotentHint: true, OpenWorldHint: &f},
		},
		{
			Name:        "describe_operation",
			Title:       "Describe an API operation",
			Description: "Return an operation's arguments (as a JSON Schema), safety class and documentation.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"operationId": map[string]any{"type": "string"}},
				"required":   []any{"operationId"},
			},
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: &f, IdempotentHint: true, OpenWorldHint: &f},
		},
		{
			Name:  "call_operation",
			Title: "Call an API operation",
			Description: "Run any mahresources API operation as the signed-in user. arguments follows the operation's inputSchema " +
				"(see describe_operation). Destructive operations ask the user to confirm first.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"operationId": map[string]any{"type": "string"},
					"arguments":   map[string]any{"type": "object", "additionalProperties": true},
				},
				"required": []any{"operationId"},
			},
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &t, OpenWorldHint: &t},
		},
	}
}

// passThrough names a tool that exposes one operation's own arguments.
type passThrough struct {
	name, title, opID, description string
}

var passThroughTools = []passThrough{
	{"search", "Search everything", "globalSearch", "Full-text search across resources, notes, groups, tags and more."},
	{"list_resources", "List resources", "listResources", "List resources (files) with filters. Paginated with page."},
	{"get_resource", "Get a resource", "getResource", "Get one resource by id."},
	{"list_notes", "List notes", "listNotes", "List notes with filters. Paginated with page."},
	{"get_note", "Get a note", "getNote", "Get one note by id."},
	{"list_groups", "List groups", "listGroups", "List groups with filters. Paginated with page."},
	{"get_group", "Get a group", "getGroup", "Get one group by id."},
	{"list_tags", "List tags", "listTags", "List tags with filters. Paginated with page."},
	{"create_note", "Create or update a note", "createOrUpdateNote", "Create a note, or update one when ID is set."},
	{"create_group", "Create or update a group", "createOrUpdateGroup", "Create a group, or update one when ID is set."},
	{"edit_resource", "Edit a resource", "editResource", "Edit a resource's fields."},
	{"upload_resource", "Upload a resource", "createResource", "Upload files as new resources. Each file is {name, contentBase64}. Extra fields (Name, Tags, Groups, OwnerId, ...) are passed through."},
	{"add_resource_from_url", "Add a resource from a URL", "addRemoteResource", "Fetch a URL on the server and store it as a resource."},
	{"get_job", "Get a job", "getCanonicalJob", "Get one background job (download, export, import, plugin action) by id."},
	{"list_jobs", "List jobs", "listCanonicalJobs", "List background jobs."},
}

func buildNamedTools(c *Catalog) ([]namedTool, error) {
	var out []namedTool
	for _, p := range passThroughTools {
		op, ok := c.Get(p.opID)
		if !ok {
			return nil, fmt.Errorf("named tool %s: operation %s is not exposed", p.name, p.opID)
		}
		out = append(out, namedTool{
			tool: &mcp.Tool{
				Name:        p.name,
				Title:       p.title,
				Description: p.description + " (operation " + op.ID + ")",
				InputSchema: op.InputSchema,
				Annotations: annotationsFor(op.Safety, op.OpenWorld, op.Method),
			},
			resolve: func(args map[string]any) (*Operation, map[string]any, error) { return op, args, nil },
		})
	}

	mrql, ok := c.Get("executeMRQL")
	if !ok {
		return nil, errors.New("named tool mrql_query: executeMRQL is not exposed")
	}
	out = append(out, namedTool{
		tool: &mcp.Tool{
			Name:  "mrql_query",
			Title: "Run an MRQL query",
			Description: "Run a Mahresources Query Language query, e.g. `type = resource AND tags = \"photo\" ORDER BY created DESC LIMIT 20`. " +
				"Use validateMRQL through call_operation to check syntax, and the docs at describe_operation executeMRQL.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":  map[string]any{"type": "string"},
					"limit":  map[string]any{"type": "integer"},
					"page":   map[string]any{"type": "integer"},
					"offset": map[string]any{"type": "integer"},
				},
				"required": []any{"query"},
			},
			Annotations: annotationsFor(ReadOnly, false, http.MethodGet),
		},
		resolve: func(args map[string]any) (*Operation, map[string]any, error) { return mrql, args, nil },
	})

	entityTool := func(name, title, desc string, safety Safety, ops map[string]string, schema map[string]any,
		mapArgs func(args map[string]any) (map[string]any, error)) error {
		resolved := map[string]*Operation{}
		var entities []any
		for entity, id := range ops {
			op, ok := c.Get(id)
			if !ok {
				return fmt.Errorf("named tool %s: operation %s is not exposed", name, id)
			}
			resolved[entity] = op
			entities = append(entities, entity)
		}
		sort.Slice(entities, func(i, j int) bool { return entities[i].(string) < entities[j].(string) })
		props := schema["properties"].(map[string]any)
		props["entity"] = map[string]any{"type": "string", "enum": entities}
		schema["required"] = append([]any{"entity"}, schema["required"].([]any)...)
		out = append(out, namedTool{
			tool: &mcp.Tool{
				Name: name, Title: title, Description: desc, InputSchema: schema,
				Annotations: annotationsFor(safety, false, http.MethodPost),
			},
			resolve: func(args map[string]any) (*Operation, map[string]any, error) {
				entity, _ := args["entity"].(string)
				op, ok := resolved[entity]
				if !ok {
					return nil, nil, fmt.Errorf("entity must be one of %v", entities)
				}
				opArgs, err := mapArgs(args)
				return op, opArgs, err
			},
		})
		return nil
	}
	intList := map[string]any{"type": "array", "items": map[string]any{"type": "integer"}}

	tagArgs := func(args map[string]any) (map[string]any, error) {
		return map[string]any{"ID": args["ids"], "EditedId": args["tags"]}, nil
	}
	tagSchema := func() map[string]any {
		return map[string]any{"type": "object", "properties": map[string]any{
			"ids":  intList,
			"tags": map[string]any{"type": "array", "items": map[string]any{"type": "integer"}, "description": "Tag IDs"},
		}, "required": []any{"ids", "tags"}}
	}
	if err := entityTool("add_tags", "Add tags", "Add tags to resources, notes or groups.", Mutating,
		map[string]string{"resource": "bulkAddTagsToResources", "note": "bulkAddTagsToNotes", "group": "bulkAddTagsToGroups"},
		tagSchema(), tagArgs); err != nil {
		return nil, err
	}
	if err := entityTool("remove_tags", "Remove tags", "Remove tags from resources, notes or groups.", Mutating,
		map[string]string{"resource": "bulkRemoveTagsFromResources", "note": "bulkRemoveTagsFromNotes", "group": "bulkRemoveTagsFromGroups"},
		tagSchema(), tagArgs); err != nil {
		return nil, err
	}
	if err := entityTool("delete", "Delete", "Delete one resource, note, group or tag. The user is asked to confirm first.", Destructive,
		map[string]string{"resource": "deleteResource", "note": "deleteNote", "group": "deleteGroup", "tag": "deleteTag"},
		map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}}, "required": []any{"id"}},
		func(args map[string]any) (map[string]any, error) { return map[string]any{"Id": args["id"]}, nil }); err != nil {
		return nil, err
	}
	if err := entityTool("merge", "Merge", "Merge resources, groups or tags into one winner; the losers are removed. The user is asked to confirm first.", Destructive,
		map[string]string{"resource": "mergeResources", "group": "mergeGroups", "tag": "mergeTags"},
		map[string]any{"type": "object", "properties": map[string]any{
			"winner": map[string]any{"type": "integer"},
			"losers": intList,
		}, "required": []any{"winner", "losers"}},
		func(args map[string]any) (map[string]any, error) {
			return map[string]any{"Winner": args["winner"], "Losers": args["losers"]}, nil
		}); err != nil {
		return nil, err
	}
	return out, nil
}
