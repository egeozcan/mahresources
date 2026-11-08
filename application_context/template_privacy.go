package application_context

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Privacy mode for template generation (docs/plans/2026-11-08-template-privacy-mode.md,
// ADR 0010): every Entity value is replaced by a Placeholder before it reaches
// an AI provider, and a guard on the transport fails the run closed when a
// real value is still in a request. Placeholders only ever go one way: nothing
// maps them back.

var (
	// ErrTemplatePrivacyLeak is a request the guard refused to send: it carried
	// an Entity value, or the masker had hit a limit and could not vouch for it.
	// Its text never contains the value.
	ErrTemplatePrivacyLeak = errors.New("Privacy mode stopped a request that contained an entity value. Nothing was sent")
	// ErrTemplatePrivacyLimit is the sticky error of a masker past its limits.
	ErrTemplatePrivacyLimit = errors.New("too much entity data for Privacy mode")
)

// Limits of one masker (P16).
const (
	templatePrivacyMaxValues      = 20000
	templatePrivacyMaxLegendBytes = 4 << 20
	templatePrivacyMaxValueBytes  = 256 << 10
)

// templatePrivacyStandIn is what every time.Time of a masked entity copy becomes
// (P2): a time.Time cannot hold a Placeholder string.
var templatePrivacyStandIn = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// TemplatePrivacyStandInDate is the stand-in instant as the Seed's privacy note
// quotes it.
const TemplatePrivacyStandInDate = "2001-02-03T04:05:06Z"

// templatePrivacyKey keys the Placeholder tag. It is random per process, so a
// tag says nothing about the data and a restart changes every tag (the
// grounding hash then differs and the UI re-runs the dry run).
var templatePrivacyKey = func() []byte {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err)
	}
	return key
}()

// TemplatePrivacyTag is the two-character tag of a request's Placeholders:
// deterministic for one account, entity type and carrier within a process, so
// the dry run and the request that follows it build the same Seed bytes.
func TemplatePrivacyTag(accountKey, entityType string, carrierID uint) string {
	mac := hmac.New(sha256.New, templatePrivacyKey)
	mac.Write([]byte(accountKey + "|" + entityType + "|" + strconv.FormatUint(uint64(carrierID), 10)))
	sum := mac.Sum(nil)
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:2]))[:2]
}

// TemplatePrivacyLegendEntry maps one Placeholder back to its value, for the
// browser. The legend is never stored, logged or sent to a provider.
type TemplatePrivacyLegendEntry struct {
	Placeholder string `json:"placeholder"`
	Value       string `json:"value"`
}

type privacyIssued struct {
	placeholder string
	alias       string
	value       string
}

// privacyGuardValue is one guarded value: its lower-cased spellings, and
// whether readable text already carries it.
type privacyGuardValue struct {
	// spellings are the lower-cased forms of the COMPLETE value: raw, HTML-escaped,
	// and the JSON-escaped forms of both. The guard detects and exempts complete
	// values only, so readable text that equals a prefix of a long value is
	// sendable and a readable prefix never exempts the rest.
	spellings []string
	exempt    bool
}

// TemplatePrivacy is one request's masker and guard. nil means Privacy mode is
// off. It is safe for concurrent use: the Tools of a Turn run in parallel (P13).
type TemplatePrivacy struct {
	mu   sync.Mutex
	tag  string
	next int
	// byValue is keyed by the real string, or "\x00date:<instant>" for a date.
	byValue map[string]*privacyIssued
	// protected are originals enrolled in the guard without a Placeholder.
	protected map[string]struct{}
	// enrolled is every original ever enrolled in the guard, protected or issued,
	// so enrolling it again (a re-read date, the same instant on another entity)
	// is free whatever key its Placeholder is stored under.
	enrolled map[string]struct{}
	issued   []*privacyIssued
	bytes    int
	err      error

	guard privacyGuard
}

// NewTemplatePrivacy returns an empty masker whose Placeholders carry tag.
func NewTemplatePrivacy(tag string) *TemplatePrivacy {
	p := &TemplatePrivacy{tag: tag, byValue: map[string]*privacyIssued{}, protected: map[string]struct{}{}, enrolled: map[string]struct{}{}, guard: newPrivacyGuard()}
	// The stand-in instant is deliberately sent in every rendered date; a real
	// date that equals one of its spellings is therefore not a leak.
	// true, false and null are JSON's own words in every Tool result and Seed
	// sample, so a string value that spells one cannot tell them from a leak.
	for _, literal := range []string{"true", "false", "null"} {
		p.guard.addCorpus(literal)
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05", time.RFC1123, time.RFC1123Z, time.RFC822, time.RFC850, time.ANSIC, time.Kitchen, "Jan 2, 2006", "January 2, 2006", "02/01/2006", "01/02/2006", "2006/01/02", "15:04:05"} {
		p.guard.addCorpus(templatePrivacyStandIn.Format(layout))
	}
	return p
}

// KeepScalar registers the text forms of a number or boolean that a Tool result
// passes through unmasked (IDs, counts, sizes).
func (p *TemplatePrivacy) KeepScalar(v any) { p.keepScalar(v) }

// keepScalar registers the text forms of a number or boolean that passes
// through unmasked, so a protected string equal to it (a Name "734928601" next
// to a FileSize of 734928601) is not mistaken for a leak.
func (p *TemplatePrivacy) keepScalar(v any) {
	if p == nil {
		return
	}
	var forms []string
	switch x := v.(type) {
	case json.Number:
		forms = []string{x.String()}
		if f, err := x.Float64(); err == nil {
			forms = append(forms, strconv.FormatFloat(f, 'g', -1, 64), strconv.FormatFloat(f, 'f', -1, 64))
		}
	case float64:
		forms = []string{strconv.FormatFloat(x, 'g', -1, 64), strconv.FormatFloat(x, 'f', -1, 64)}
	case float32:
		f := float64(x)
		forms = []string{strconv.FormatFloat(f, 'g', -1, 32), strconv.FormatFloat(f, 'f', -1, 32)}
	case bool:
		forms = []string{strconv.FormatBool(x)}
	default:
		switch rv := reflect.ValueOf(v); rv.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			forms = []string{strconv.FormatInt(rv.Int(), 10)}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			forms = []string{strconv.FormatUint(rv.Uint(), 10)}
		case reflect.Float32, reflect.Float64:
			forms = []string{strconv.FormatFloat(rv.Float(), 'g', -1, 64), strconv.FormatFloat(rv.Float(), 'f', -1, 64)}
		case reflect.Bool:
			forms = []string{strconv.FormatBool(rv.Bool())}
		}
	}
	p.mu.Lock()
	for _, f := range forms {
		p.guard.addCorpus(f)
	}
	p.mu.Unlock()
}

// Err is the sticky error of a masker that hit a limit. Every publisher checks
// it before its output leaves, and partial output is never published.
func (p *TemplatePrivacy) Err() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

func (p *TemplatePrivacy) withheld() string { return "⟦" + p.tag + "·withheld⟧" }

var (
	privacyDatePattern  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}([T ]\d{2}:\d{2}(:\d{2}(\.\d+)?)?(Z|[+-]\d{2}:?\d{2})?)?$`)
	privacyEmailPattern = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	privacyURLPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*://\S+$`)
)

func privacyKindOf(s string) string {
	switch {
	case privacyDatePattern.MatchString(s):
		return "date"
	case privacyURLPattern.MatchString(s):
		return "url"
	case privacyEmailPattern.MatchString(s):
		return "email"
	}
	return "text"
}

// privacyLengthBucket rounds a character count up to the shown size.
func privacyLengthBucket(n int) string {
	for _, bucket := range []int{10, 20, 50, 100, 200, 500, 1000} {
		if n <= bucket {
			return strconv.Itoa(bucket)
		}
	}
	return "1000+"
}

func (p *TemplatePrivacy) shapedPlaceholder(kind string, n int, value string) string {
	if kind == "date" {
		return fmt.Sprintf("⟦%s·date %d⟧", p.tag, n)
	}
	lines := "1 line"
	if strings.ContainsAny(value, "\n\r") {
		lines = "multi-line"
	}
	return fmt.Sprintf("⟦%s·%s %d · %s · ~%s chars⟧", p.tag, kind, n, lines, privacyLengthBucket(utf8.RuneCountInString(value)))
}

// issueLocked returns the entry for key, issuing it on first sight. It
// returns nil past a limit, with the sticky error set.
func (p *TemplatePrivacy) issueLocked(key, value, forceKind string) *privacyIssued {
	if existing, ok := p.byValue[key]; ok {
		return existing
	}
	// An original enrolled earlier (a Date's instant, a stripped content type) is
	// already guarded and already charged: issuing it moves it from protected to
	// issued without recharging its bytes or adding its patterns again.
	_, transfer := p.protected[value]
	if !transfer && (len(value) > templatePrivacyMaxValueBytes || p.bytes+len(value) > templatePrivacyMaxLegendBytes || len(p.issued)+len(p.protected) >= templatePrivacyMaxValues) {
		if p.err == nil {
			p.err = ErrTemplatePrivacyLimit
		}
		return nil
	}
	kind := forceKind
	if kind == "" {
		kind = privacyKindOf(value)
	}
	p.next++
	entry := &privacyIssued{placeholder: p.shapedPlaceholder(kind, p.next, value), value: value}
	p.byValue[key] = entry
	p.issued = append(p.issued, entry)
	if transfer {
		delete(p.protected, value)
	} else {
		p.bytes += len(value)
		p.guard.addValue(value)
		p.enrolled[value] = struct{}{}
	}
	// A Placeholder is text this run generated, never a stored value, so it is
	// readable even when some other stored value happens to equal it.
	p.guard.addCorpus(entry.placeholder)
	return entry
}

// Text masks a string. The kind is read from its shape (url, email, date or
// text); equal strings share one Placeholder. It always treats its input as
// raw: a string that happens to equal an issued Placeholder gets its own.
func (p *TemplatePrivacy) Text(s string) string {
	if p == nil || s == "" {
		return s
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.issueLocked(s, s, "")
	if entry == nil {
		return p.withheld()
	}
	return entry.placeholder
}

// Date masks an instant: ⟦tag·date N⟧, keyed by the instant. The legend value
// is the instant as RFC 3339 in UTC.
func (p *TemplatePrivacy) Date(t time.Time) string {
	if p == nil {
		return t.UTC().Format(time.RFC3339)
	}
	p.protectTime(t)
	value := t.UTC().Format(time.RFC3339Nano)
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.issueLocked("\x00date:"+value, t.UTC().Format(time.RFC3339), "date")
	if entry == nil {
		return p.withheld()
	}
	return entry.placeholder
}

// URLAlias masks a URL as one value (P6) and returns its ASCII alias,
// https://placeholder.invalid/<tag>-url-<n>, which is the same Placeholder in a
// spelling net/url leaves alone. The legend lists both spellings.
func (p *TemplatePrivacy) URLAlias(s string) string {
	if p == nil || s == "" {
		return s
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	entry := p.issueLocked(s, s, "url")
	if entry == nil {
		return "https://placeholder.invalid/" + p.tag + "-withheld"
	}
	if entry.alias == "" {
		entry.alias = "https://placeholder.invalid/" + p.tag + "-url-" + p.numberOf(entry)
		p.guard.addCorpus(entry.alias)
	}
	return entry.alias
}

// numberOf is the n of an entry, read back from its Placeholder.
func (p *TemplatePrivacy) numberOf(e *privacyIssued) string {
	start := strings.Index(e.placeholder, "·")
	rest := e.placeholder[start+len("·"):]
	fields := strings.Fields(strings.TrimSuffix(rest, "⟧"))
	return fields[1]
}

// protect enrolls a stored value in the guard without issuing a Placeholder,
// for originals the source replaces by something that is not a Placeholder
// (a stand-in instant, a content type with its parameters stripped).
func (p *TemplatePrivacy) protect(value string) {
	if p == nil || value == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.enrolled[value]; ok {
		return
	}
	if _, ok := p.byValue[value]; ok {
		return // already guarded as an issued value
	}
	// The same limits as an issued value (P16), through the sticky error.
	if len(value) > templatePrivacyMaxValueBytes || p.bytes+len(value) > templatePrivacyMaxLegendBytes || len(p.issued)+len(p.protected) >= templatePrivacyMaxValues {
		if p.err == nil {
			p.err = ErrTemplatePrivacyLimit
		}
		return
	}
	p.protected[value] = struct{}{}
	p.enrolled[value] = struct{}{}
	p.bytes += len(value)
	p.guard.addValue(value)
}

// protectTime enrolls the wire spellings of an original instant: RFC 3339 with
// and without fractional seconds, in UTC and in the zone it was stored in.
func (p *TemplatePrivacy) protectTime(t time.Time) {
	if p == nil || t.IsZero() {
		return
	}
	for _, v := range []time.Time{t, t.UTC()} {
		p.protect(v.Format(time.RFC3339Nano))
		p.protect(v.Format(time.RFC3339))
	}
}

// Keep marks a string as readable. It is returned unchanged and joins the
// guard's corpus, so a masked value that also appears in it is not a leak.
func (p *TemplatePrivacy) Keep(s string) string {
	if p == nil || s == "" {
		return s
	}
	p.mu.Lock()
	p.guard.addCorpus(s)
	p.mu.Unlock()
	return s
}

// Legend lists every issued Placeholder with its value, in issue order.
func (p *TemplatePrivacy) Legend() []TemplatePrivacyLegendEntry {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]TemplatePrivacyLegendEntry, 0, len(p.issued))
	for _, e := range p.issued {
		out = append(out, TemplatePrivacyLegendEntry{Placeholder: e.placeholder, Value: e.value})
		// A URL has two spellings (P6), each its own entry with the same value.
		if e.alias != "" {
			out = append(out, TemplatePrivacyLegendEntry{Placeholder: e.alias, Value: e.value})
		}
	}
	return out
}

// IsIssuedPlaceholder reports whether s is exactly a Placeholder this masker
// issued. A nil masker has issued none.
func (p *TemplatePrivacy) IsIssuedPlaceholder(s string) bool {
	if p == nil || !strings.HasPrefix(s, "⟦") {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.isIssuedPlaceholder(s)
}

// ContainsIssued reports the first Placeholder this run issued, in either
// spelling, that s contains (P11).
func (p *TemplatePrivacy) ContainsIssued(s string) (string, bool) {
	if p == nil || s == "" {
		return "", false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	best, bestAt := "", -1
	consider := func(token string, at int) {
		if bestAt < 0 || at < bestAt {
			best, bestAt = token, at
		}
	}
	opening := "⟦" + p.tag + "·"
	for from := 0; from < len(s); {
		at := strings.Index(s[from:], opening)
		if at < 0 {
			break
		}
		at += from
		end := strings.Index(s[at:], "⟧")
		if end < 0 {
			break
		}
		token := s[at : at+end+len("⟧")]
		if token == p.withheld() || p.isIssuedPlaceholder(token) {
			consider(token, at)
			break
		}
		from = at + len(opening)
	}
	// An issued alias counts wherever it is contained, even when more URL text
	// follows it (...-url-1backup).
	for _, e := range p.issued {
		if e.alias == "" {
			continue
		}
		if at := strings.Index(s, e.alias); at >= 0 {
			consider(e.alias, at)
		}
	}
	return best, bestAt >= 0
}

func (p *TemplatePrivacy) isIssuedPlaceholder(token string) bool {
	for _, e := range p.issued {
		if e.placeholder == token {
			return true
		}
	}
	return false
}

func (p *TemplatePrivacy) isIssuedAlias(token string) bool {
	for _, e := range p.issued {
		if e.alias != "" && e.alias == token {
			return true
		}
	}
	return false
}

// privacyContextKey carries the request's masker to the transport guard.
type privacyContextKey struct{}

// WithTemplatePrivacy returns ctx carrying p; the guard in the provider
// transport reads it. A nil p leaves ctx unchanged.
func WithTemplatePrivacy(ctx context.Context, p *TemplatePrivacy) context.Context {
	if p == nil {
		return ctx
	}
	return context.WithValue(ctx, privacyContextKey{}, p)
}

// TemplatePrivacyFrom is the masker WithTemplatePrivacy stored, or nil.
func TemplatePrivacyFrom(ctx context.Context) *TemplatePrivacy {
	if ctx == nil {
		return nil
	}
	p, _ := ctx.Value(privacyContextKey{}).(*TemplatePrivacy)
	return p
}

// Meta masks the string leaves of a Meta document, keeping its keys. A string
// equal to an enum or const of its own field in schema stays readable (P12).
// raw that is not JSON is masked as one string.
func (p *TemplatePrivacy) Meta(raw []byte, schema string) []byte {
	return p.metaWalk(nil, raw, schema)
}

// privacyWalk counts the nodes of every masking walk (reflection, JSON, query
// rows) and checks the context at bounded intervals, so a large field cannot
// outrun cancellation. A nil walk checks nothing.
type privacyWalk struct {
	ctx context.Context
	n   int
	err error
}

func (w *privacyWalk) tick() bool {
	if w == nil {
		return true
	}
	if w.err != nil {
		return false
	}
	w.n++
	if w.n%privacyCheckEvery == 0 {
		if err := w.ctx.Err(); err != nil {
			w.err = err
			return false
		}
	}
	return true
}

func (p *TemplatePrivacy) metaWalk(w *privacyWalk, raw []byte, schema string) []byte {
	if p == nil {
		return raw
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		// Nothing to mask, but the copy must not alias the original's bytes.
		return append([]byte(nil), raw...)
	}
	decoded, err := decodeJSONNumber(raw)
	if err != nil {
		quoted, _ := json.Marshal(p.Text(string(raw)))
		return quoted
	}
	out, err := encodeJSONNoEscape(p.valueWalk(w, decoded, parseSchemaRoot(schema)))
	if err != nil {
		quoted, _ := json.Marshal(p.Text(string(raw)))
		return quoted
	}
	return out
}

func decodeJSONNumber(raw []byte) (any, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}

func encodeJSONNoEscape(v any) ([]byte, error) {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return []byte(strings.TrimRight(sb.String(), "\n")), nil
}

func parseSchemaRoot(schema string) map[string]any {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var root map[string]any
	if json.Unmarshal([]byte(schema), &root) != nil {
		return nil
	}
	return root
}

// Value masks a decoded JSON value. schema is a decoded root schema
// (map[string]any) or nil.
func (p *TemplatePrivacy) Value(v any, schema any) any {
	return p.valueWalk(nil, v, schema)
}

func (p *TemplatePrivacy) valueWalk(w *privacyWalk, v any, schema any) any {
	if p == nil {
		return v
	}
	root, _ := schema.(map[string]any)
	var nodes []map[string]any
	if root != nil {
		nodes = expandSchemaNode(root, root)
	}
	return p.maskJSON(w, v, root, nodes)
}

func (p *TemplatePrivacy) maskJSON(w *privacyWalk, v any, root map[string]any, nodes []map[string]any) any {
	if !w.tick() {
		return v
	}
	switch x := v.(type) {
	case string:
		if schemaAllows(nodes, x) {
			return p.Keep(x)
		}
		return p.Text(x)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make(map[string]any, len(x))
		for _, k := range keys {
			p.Keep(k) // a preserved key is vocabulary, and it travels in the request
			out[k] = p.maskJSON(w, x[k], root, schemaChildNodes(root, nodes, k))
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = p.maskJSON(w, e, root, schemaItemNodes(root, nodes, i))
		}
		return out
	}
	p.keepScalar(v)
	return v
}

// resolveSchemaRef follows a local $ref one level: #/$defs/x or
// #/definitions/x. An unresolvable $ref gives nil.
func resolveSchemaRef(root, node map[string]any) map[string]any {
	ref, ok := node["$ref"].(string)
	if !ok {
		return node
	}
	for _, prefix := range []string{"#/$defs/", "#/definitions/"} {
		if name, found := strings.CutPrefix(ref, prefix); found {
			bucket := "$defs"
			if prefix == "#/definitions/" {
				bucket = "definitions"
			}
			defs, _ := root[bucket].(map[string]any)
			if target, ok := defs[name].(map[string]any); ok {
				return target
			}
		}
	}
	return nil
}

// expandSchemaNode is a schema node plus the branches of its allOf, anyOf and
// oneOf, each with a local $ref resolved one level.
func expandSchemaNode(root, node map[string]any) []map[string]any {
	resolved := resolveSchemaRef(root, node)
	if resolved == nil {
		return nil
	}
	out := []map[string]any{resolved}
	for _, keyword := range []string{"allOf", "anyOf", "oneOf"} {
		branches, _ := resolved[keyword].([]any)
		for _, b := range branches {
			if m, ok := b.(map[string]any); ok {
				if r := resolveSchemaRef(root, m); r != nil {
					out = append(out, r)
				}
			}
		}
	}
	return out
}

func schemaChildNodes(root map[string]any, nodes []map[string]any, key string) []map[string]any {
	var out []map[string]any
	for _, n := range nodes {
		props, _ := n["properties"].(map[string]any)
		if child, ok := props[key].(map[string]any); ok {
			out = append(out, expandSchemaNode(root, child)...)
		}
	}
	return out
}

func schemaItemNodes(root map[string]any, nodes []map[string]any, index int) []map[string]any {
	var out []map[string]any
	for _, n := range nodes {
		switch items := n["items"].(type) {
		case map[string]any:
			out = append(out, expandSchemaNode(root, items)...)
		case []any:
			if index < len(items) {
				if m, ok := items[index].(map[string]any); ok {
					out = append(out, expandSchemaNode(root, m)...)
				}
			}
		}
	}
	return out
}

func schemaAllows(nodes []map[string]any, s string) bool {
	for _, n := range nodes {
		if c, ok := n["const"].(string); ok && c == s {
			return true
		}
		if enum, ok := n["enum"].([]any); ok {
			for _, e := range enum {
				if es, ok := e.(string); ok && es == s {
					return true
				}
			}
		}
	}
	return false
}
