package application_context

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"mahresources/shortcodes"
)

// The DB-free half of the Tool executor (plan section 2): argument validation
// and result shaping. The executor that touches the database is
// server/api_handlers/template_run_tools.go.

// Typed arguments of the read Tools. submit is lane C's.
type (
	GetEntityArgs struct {
		Type string
		ID   uint
	}
	ListRelatedArgs struct {
		Type      string
		ID        uint
		Direction string
		Kind      string
		Offset    int
	}
	GetTemplateArgs struct {
		CarrierType string
		ID          uint
		Fields      []string
	}
	ListCarriersArgs struct {
		CarrierType string
		NameQuery   string
		Offset      int
	}
	ListMembersArgs struct {
		CarrierType string
		ID          uint
		Offset      int
	}
	// RevisionArgs serves list_revisions, get_revision and diff_revisions.
	// CarrierType and ID are zero when the call means the carrier being edited.
	RevisionArgs struct {
		CarrierType string
		ID          uint
		Offset      int
		Number      uint
		Field       string
		From        uint
		To          *uint
	}
	LintArgs struct {
		Field   string
		Content *string
	}
	CheckMRQLArgs struct {
		Query   string
		Params  map[string]string
		Limit   int
		Buckets int
	}
	PreviewArgs struct {
		Field      string
		Content    *string
		CSS        *string
		EntityType string
		EntityID   uint
	}
	NameArgs struct{ Name string }
)

const (
	toolEntityTypes  = "group, resource, note"
	toolCarrierTypes = "category, resourceCategory, noteType"
	// Bounds on the free-text arguments, so a model cannot make argument
	// handling itself expensive. content/css have their own source limit.
	maxToolNameBytes   = 200
	maxToolQueryBytes  = 32 << 10
	maxToolParamCount  = 32
	maxToolParamBytes  = 32 << 10
	maxToolFieldsCount = 40
)

var (
	toolEntityTypeSet  = map[string]bool{"group": true, "resource": true, "note": true}
	toolCarrierTypeSet = map[string]bool{"category": true, "resourceCategory": true, "noteType": true}
	toolDirectionSet   = map[string]bool{"up": true, "down": true, "across": true}
)

// toolArgs reads one decoded argument object, remembering the first problem.
type toolArgs struct {
	m   map[string]any
	err error
}

func (a *toolArgs) fail(format string, args ...any) {
	if a.err == nil {
		a.err = fmt.Errorf(format, args...)
	}
}

func (a *toolArgs) str(key string, required bool, enum map[string]bool, enumList string, maxBytes int) string {
	v, ok := a.m[key]
	if !ok || v == nil {
		if required {
			a.fail("%s is required", key)
		}
		return ""
	}
	s, isStr := v.(string)
	if !isStr {
		a.fail("%s must be a string", key)
		return ""
	}
	if required && s == "" {
		a.fail("%s must not be empty", key)
	}
	if enum != nil && s != "" && !enum[s] {
		a.fail("%s must be one of: %s", key, enumList)
	}
	if maxBytes > 0 && len(s) > maxBytes {
		a.fail("%s is %d bytes, the limit is %d", key, len(s), maxBytes)
	}
	return s
}

// optStr is a string that may be absent, distinguishing absent from empty.
func (a *toolArgs) optStr(key string) *string {
	v, ok := a.m[key]
	if !ok || v == nil {
		return nil
	}
	s, isStr := v.(string)
	if !isStr {
		a.fail("%s must be a string", key)
		return nil
	}
	return &s
}

func (a *toolArgs) intArg(key string, required bool, min int) int {
	v, ok := a.m[key]
	if !ok || v == nil {
		if required {
			a.fail("%s is required", key)
		}
		return 0
	}
	n, isNum := v.(json.Number)
	if !isNum {
		a.fail("%s must be an integer", key)
		return 0
	}
	f, err := n.Float64()
	if err != nil || f != math.Trunc(f) || f > float64(math.MaxInt32) || f < float64(math.MinInt32) {
		a.fail("%s must be a whole number", key)
		return 0
	}
	if int(f) < min {
		a.fail("%s must be at least %d", key, min)
		return 0
	}
	return int(f)
}

func (a *toolArgs) id(key string, required bool) uint {
	return uint(a.intArg(key, required, 1))
}

func (a *toolArgs) strList(key string) []string {
	v, ok := a.m[key]
	if !ok || v == nil {
		return nil
	}
	list, isList := v.([]any)
	if !isList {
		a.fail("%s must be an array of strings", key)
		return nil
	}
	if len(list) > maxToolFieldsCount {
		a.fail("%s has %d entries, the limit is %d", key, len(list), maxToolFieldsCount)
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		s, isStr := item.(string)
		if !isStr {
			a.fail("%s must be an array of strings", key)
			return nil
		}
		if len(s) > maxToolNameBytes {
			a.fail("%s has a name over %d bytes", key, maxToolNameBytes)
			return nil
		}
		out = append(out, s)
	}
	return out
}

// toolKeys lists the argument names each Tool accepts; anything else is
// refused by name so the model learns the contract from the error.
var toolKeys = map[string][]string{
	ToolGetEntity:        {"type", "id"},
	ToolListRelated:      {"type", "id", "direction", "kind", "offset"},
	ToolGetTemplate:      {"carrierType", "id", "fields"},
	ToolListCarriers:     {"carrierType", "nameQuery", "offset"},
	ToolListMembers:      {"carrierType", "id", "offset"},
	ToolListRevisions:    {"carrierType", "id", "offset"},
	ToolGetRevision:      {"carrierType", "id", "number", "field"},
	ToolDiffRevisions:    {"carrierType", "id", "from", "to"},
	ToolLint:             {"field", "content"},
	ToolCheckMRQL:        {"query", "params", "limit", "buckets"},
	ToolPreview:          {"field", "content", "css", "entityType", "entityId"},
	ToolGetPartial:       {"name"},
	ToolGetShortcodeDocs: {"name"},
}

// DecodeToolArguments validates the argument text of a read Tool call and
// returns its typed form. The error text goes back to the model as a result.
func DecodeToolArguments(tool, raw string) (any, error) {
	keys, known := toolKeys[tool]
	if !known {
		return nil, fmt.Errorf("unknown Tool %q", tool)
	}
	m := map[string]any{}
	if strings.TrimSpace(raw) != "" {
		dec := json.NewDecoder(strings.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&m); err != nil {
			return nil, fmt.Errorf("the arguments are not valid JSON: %v", err)
		}
		// Anything after the object but whitespace is refused; More() would
		// accept a stray closing bracket.
		var extra json.RawMessage
		if err := dec.Decode(&extra); err != io.EOF {
			return nil, fmt.Errorf("the arguments are not valid JSON: trailing data after the object")
		}
		if m == nil {
			m = map[string]any{}
		}
	}
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	var unknown []string
	for k := range m {
		if !allowed[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		more := ""
		if len(unknown) > 3 {
			more = fmt.Sprintf(" (and %d more)", len(unknown)-3)
			unknown = unknown[:3]
		}
		for i, k := range unknown {
			unknown[i] = strconv.Quote(clipToolText(k, 40))
		}
		return nil, fmt.Errorf("unknown argument %s%s; %s accepts: %s", strings.Join(unknown, ", "), more, tool, strings.Join(keys, ", "))
	}

	a := &toolArgs{m: m}
	var out any
	switch tool {
	case ToolGetEntity:
		out = GetEntityArgs{
			Type: a.str("type", true, toolEntityTypeSet, toolEntityTypes, 0),
			ID:   a.id("id", true),
		}
	case ToolListRelated:
		out = ListRelatedArgs{
			Type:      a.str("type", true, toolEntityTypeSet, toolEntityTypes, 0),
			ID:        a.id("id", true),
			Direction: a.str("direction", true, toolDirectionSet, "up, down, across", 0),
			Kind:      a.str("kind", false, toolEntityTypeSet, toolEntityTypes, 0),
			Offset:    a.intArg("offset", false, 0),
		}
	case ToolGetTemplate:
		out = GetTemplateArgs{
			CarrierType: a.str("carrierType", true, toolCarrierTypeSet, toolCarrierTypes, 0),
			ID:          a.id("id", true),
			Fields:      a.strList("fields"),
		}
	case ToolListCarriers:
		out = ListCarriersArgs{
			CarrierType: a.str("carrierType", true, toolCarrierTypeSet, toolCarrierTypes, 0),
			NameQuery:   a.str("nameQuery", false, nil, "", maxToolNameBytes),
			Offset:      a.intArg("offset", false, 0),
		}
	case ToolListMembers:
		out = ListMembersArgs{
			CarrierType: a.str("carrierType", true, toolCarrierTypeSet, toolCarrierTypes, 0),
			ID:          a.id("id", true),
			Offset:      a.intArg("offset", false, 0),
		}
	case ToolListRevisions, ToolGetRevision, ToolDiffRevisions:
		r := RevisionArgs{
			CarrierType: a.str("carrierType", false, toolCarrierTypeSet, toolCarrierTypes, 0),
			ID:          a.id("id", false),
		}
		if tool == ToolListRevisions {
			r.Offset = a.intArg("offset", false, 0)
		}
		if tool == ToolGetRevision {
			r.Number = uint(a.intArg("number", true, 1))
			r.Field = a.str("field", false, nil, "", maxToolNameBytes)
		}
		if tool == ToolDiffRevisions {
			r.From = uint(a.intArg("from", true, 1))
			if _, ok := m["to"]; ok && m["to"] != nil {
				to := uint(a.intArg("to", false, 1))
				r.To = &to
			}
		}
		// Naming one half of a carrier is a mistake, not a default.
		if (r.CarrierType == "") != (r.ID == 0) {
			a.fail("carrierType and id go together; omit both for the carrier being edited")
		}
		out = r
	case ToolLint:
		out = LintArgs{
			Field:   a.str("field", true, nil, "", maxToolNameBytes),
			Content: a.optStr("content"),
		}
	case ToolCheckMRQL:
		c := CheckMRQLArgs{
			Query:   a.str("query", true, nil, "", maxToolQueryBytes),
			Limit:   a.intArg("limit", false, 1),
			Buckets: a.intArg("buckets", false, 1),
		}
		if v, ok := m["params"]; ok && v != nil {
			c.Params = a.params(v)
		}
		out = c
	case ToolPreview:
		p := PreviewArgs{
			Field:      a.str("field", true, nil, "", maxToolNameBytes),
			Content:    a.optStr("content"),
			CSS:        a.optStr("css"),
			EntityType: a.str("entityType", false, toolEntityTypeSet, toolEntityTypes, 0),
			EntityID:   a.id("entityId", false),
		}
		if (p.EntityType == "") != (p.EntityID == 0) {
			a.fail("entityType and entityId go together; omit both to preview against a default entity")
		}
		out = p
	case ToolGetPartial, ToolGetShortcodeDocs:
		out = NameArgs{Name: a.str("name", true, nil, "", maxToolNameBytes)}
	}
	if a.err != nil {
		return nil, a.err
	}
	return out, nil
}

// params reads check_mrql's parameter object: string, number and boolean
// values only, bounded in count and total size before anything binds them.
func (a *toolArgs) params(v any) map[string]string {
	obj, ok := v.(map[string]any)
	if !ok {
		a.fail("params must be an object of scalar values")
		return nil
	}
	if len(obj) > maxToolParamCount {
		a.fail("params has %d entries, the limit is %d", len(obj), maxToolParamCount)
		return nil
	}
	out := make(map[string]string, len(obj))
	total := 0
	for k, raw := range obj {
		var s string
		switch x := raw.(type) {
		case string:
			s = x
		case json.Number:
			s = x.String()
		case bool:
			s = fmt.Sprint(x)
		default:
			a.fail("params.%s must be a string, number or boolean", k)
			return nil
		}
		total += len(k) + len(s)
		if total > maxToolParamBytes {
			a.fail("params total more than %d bytes", maxToolParamBytes)
			return nil
		}
		out[k] = s
	}
	return out
}

// ToolResultCap is the byte cap of a Tool's result.
func ToolResultCap(tool string) int {
	switch tool {
	case ToolGetEntity:
		return ToolCapGetEntityBytes
	case ToolListRelated:
		return ToolCapListRelatedBytes
	case ToolGetTemplate:
		return ToolCapGetTemplateBytes
	case ToolListCarriers:
		return ToolCapListCarriersBytes
	case ToolListMembers:
		return ToolCapListMembersBytes
	case ToolListRevisions:
		return ToolCapListRevisionsBytes
	case ToolGetRevision:
		return ToolCapGetRevisionBytes
	case ToolDiffRevisions:
		return ToolCapDiffRevisionsBytes
	case ToolLint:
		return ToolCapLintBytes
	case ToolCheckMRQL:
		return ToolCapCheckMRQLBytes
	case ToolPreview:
		return ToolCapPreviewBytes
	case ToolGetPartial:
		return ToolCapGetPartialBytes
	case ToolGetShortcodeDocs:
		return ToolCapGetShortcodeDocsBytes
	case ToolSubmit:
		return ToolCapSubmitBytes
	}
	return 0
}

// ShapeToolResult caps body at capBytes, cut on a rune boundary, appends
// "[cut: N of M bytes shown]" when it cut, and fences the whole with the
// existing <<<BEGIN/END label>>> markers (defanging markers inside the body).
// The cut happens before fencing, so the notice sits inside the fence.
func ShapeToolResult(label, body string, capBytes int) TemplateToolOutput {
	full := len(body)
	truncated := false
	if capBytes > 0 && full > capBytes {
		cut := capBytes
		for cut > 0 && !utf8.RuneStart(body[cut]) {
			cut--
		}
		body = body[:cut] + fmt.Sprintf("\n[cut: %d of %d bytes shown]", cut, full)
		truncated = true
	}
	text := strings.Join(referenceBlock(label, body), "\n")
	return TemplateToolOutput{Text: text, Bytes: len(text), FullBytes: full, Truncated: truncated}
}

// maxToolErrorBytes bounds an error result's message.
const maxToolErrorBytes = 2 << 10

// clipToolText cuts s to at most limit bytes on a rune boundary, marking a cut.
func clipToolText(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// ToolErrorResult is an error result: plain text, not fenced. A message can
// echo model-controlled text (an argument name, a field), so it is capped and
// any fence marker in it is defanged like a fenced body.
func ToolErrorResult(msg string) TemplateToolOutput {
	msg = referenceMarkerPattern.ReplaceAllString(clipToolText(msg, maxToolErrorBytes), "<<$1")
	text := "error: " + msg
	return TemplateToolOutput{Text: text, IsError: true, Bytes: len(text), FullBytes: len(text)}
}

// ToolEntityUnavailable is the one error text for an entity that does not exist
// and one the account may not read, so scope cannot be probed.
const ToolEntityUnavailable = "no such entity, or it is not readable by this account"

// MarshalToolJSON marshals v for a Tool result without HTML escaping, indented
// by nothing: the model reads it, and escaping < and > would only add noise.
func MarshalToolJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return `{"error":"could not encode the result"}`
	}
	return strings.TrimRight(buf.String(), "\n")
}

// MaxTemplateToolShortcodeTokens bounds the shortcode tokens in one lint,
// preview or submit source, checked before any parsing.
const MaxTemplateToolShortcodeTokens = 2000

// CheckTemplateToolSource refuses a source over the byte or shortcode-token
// limit before it is parsed. The token count is a count of "[" bytes, which
// over-counts prose but never under-counts a shortcode, and costs one pass.
func CheckTemplateToolSource(src string) error {
	if len(src) > MaxTemplateGeneratedContentLength {
		return fmt.Errorf("content is %d bytes, the limit is %d", len(src), MaxTemplateGeneratedContentLength)
	}
	if strings.Count(src, "[") > MaxTemplateToolShortcodeTokens {
		return fmt.Errorf("content has more than %d shortcode tokens", MaxTemplateToolShortcodeTokens)
	}
	return nil
}

// TemplateRunValidation is the harness validator's grounding: the same options
// validateTemplateContent takes, plus a partial-existence check that sees the
// caller's context. lint, submit and the final Draft all validate through
// ValidateTemplateRunContent, so one input gives one verdict.
type TemplateRunValidation struct {
	Known                 shortcodes.KnownShortcodes
	ValidateMRQL          func(query string, opts shortcodes.MRQLCheckOptions) error
	DisplayTypes          []string
	DisplayTypesTruncated bool
	PartialExists         func(ctx context.Context, name string) (bool, error)
}

// ValidateTemplateRunContent lints one field's content. A field ending in CSS
// is a stylesheet; MetaSchema is JSON Schema; anything else is markup. A partial
// lookup that fails or is cancelled counts as existing, with a warning that it
// could not be checked, so a cancelled check never reports a real partial as
// missing.
func ValidateTemplateRunContent(ctx context.Context, v TemplateRunValidation, field, content string) []shortcodes.LintIssue {
	if err := CheckTemplateToolSource(content); err != nil {
		return []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: err.Error()}}
	}
	if field == "MetaSchema" {
		return validateMetaSchemaJSON(content, shortcodes.MetaSchemaLintOptions{
			DisplayTypes:          v.DisplayTypes,
			DisplayTypesTruncated: v.DisplayTypesTruncated,
		})
	}
	var unchecked []string
	seen := map[string]bool{}
	opts := shortcodes.LintOptions{
		Known:        v.Known,
		ValidateMRQL: v.ValidateMRQL,
		CSSMode:      strings.HasSuffix(field, "CSS"),
	}
	if v.PartialExists != nil {
		opts.PartialExists = func(name string) bool {
			if err := ctx.Err(); err != nil {
				return noteUnchecked(&unchecked, seen, name)
			}
			ok, err := v.PartialExists(ctx, name)
			if err != nil {
				return noteUnchecked(&unchecked, seen, name)
			}
			return ok
		}
	}
	issues := shortcodes.Lint(content, opts)
	for _, name := range unchecked {
		issues = append(issues, shortcodes.LintIssue{
			Severity: shortcodes.SeverityWarning,
			Message:  fmt.Sprintf("could not check that partial %s exists", name),
		})
	}
	if issues == nil {
		return []shortcodes.LintIssue{}
	}
	return issues
}

func noteUnchecked(list *[]string, seen map[string]bool, name string) bool {
	if !seen[name] {
		seen[name] = true
		*list = append(*list, name)
	}
	return true
}
