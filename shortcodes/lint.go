package shortcodes

import (
	"regexp"
	"sort"
	"strings"
)

// Lint severity levels.
const (
	SeverityError   = "error"
	SeverityWarning = "warning"
	SeverityInfo    = "info"
)

// LintIssue is a single diagnostic anchored to a byte range in the input.
type LintIssue struct {
	Start    int    `json:"start"`
	End      int    `json:"end"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

// KnownShortcode is what the linter knows about one shortcode name.
type KnownShortcode struct {
	Name  string
	Block BlockCapability
	// Attrs is keyed by exact attribute name. Wildcard families are stored under
	// their prefix (e.g. "param-") with Wildcard=true on the DocAttr.
	Attrs map[string]DocAttr
	// Documented is true when the attribute set is authoritative (built-ins and
	// documented plugins). When false, attribute-level checks are skipped so an
	// undocumented plugin shortcode is not flagged for "unknown" attributes.
	Documented bool
}

// KnownShortcodes maps a shortcode name to its descriptor.
type KnownShortcodes map[string]KnownShortcode

// LintOptions configures a Lint run.
type LintOptions struct {
	// Known is the shortcode catalogue (built-ins plus enabled plugins). When
	// nil, only structural checks that don't need the catalogue run.
	Known KnownShortcodes
	// ValidateMRQL, when non-nil, validates the query/mrql attribute values and
	// its error is surfaced as a lint issue. nil skips MRQL syntax checks.
	ValidateMRQL func(query string) error
	// PartialName, when non-empty, is the name of the partial whose content is
	// being linted. A [partial name="<PartialName>"] reference inside it is a
	// direct self-reference and is flagged as a warning.
	PartialName string
	// PartialExists, when non-nil, reports whether a template partial with that
	// name exists. A [partial] reference to a name it rejects is flagged
	// (finding 155: a reference to a partial that does not exist rendered nothing
	// at all and produced no diagnostic, while an invalid [mrql] query right
	// beside it was reported). nil disables the check, so a caller with no way to
	// resolve names behaves as before rather than reporting every partial as
	// missing. Lint memoizes it, so a template referencing one partial ten times
	// asks once.
	PartialExists func(name string) bool
}

// KnownFromBuiltins builds a KnownShortcodes catalogue seeded with the built-in
// shortcodes. Callers add plugin shortcodes on top. Keeping this
// derived from BuiltinDocs keeps lint in sync with the docs endpoint.
func KnownFromBuiltins() KnownShortcodes {
	k := make(KnownShortcodes)
	for _, d := range BuiltinDocs() {
		attrs := make(map[string]DocAttr, len(d.Attrs))
		for _, a := range d.Attrs {
			attrs[a.Name] = a
		}
		k[d.Name] = KnownShortcode{Name: d.Name, Block: d.IsBlock, Attrs: attrs, Documented: true}
	}
	return k
}

// conditionalOperators are the operator attributes on [conditional]; at least
// one must be present. Every operator present must pass (or any, with
// combine="any"). Shared with the conditional handler so lint and evaluation
// agree on the operator vocabulary.
var conditionalOperators = []string{"eq", "neq", "gt", "lt", "gte", "lte", "in", "contains", "matches", "empty", "not-empty"}

// builtinBaseNames is used for near-miss detection of misspelled shortcodes.
var builtinBaseNames = []string{"meta", "property", "mrql", "conditional", "link", "each", "item", "partial", "lazy", "details", "reload"}

// looseBracketPattern finds bracket expressions that lead with an identifier,
// used to detect shortcode-looking brackets that did not parse as real
// shortcodes (typos, malformed plugin refs).
var looseBracketPattern = regexp.MustCompile(`\[/?([a-zA-Z][a-zA-Z0-9_:-]*)`)

// Lint parses shortcode markup and returns diagnostics. It never executes any
// shortcode: all checks are pure parsing plus (optionally) an MRQL syntax check
// on query attributes. Issues are returned sorted by start offset.
func Lint(input string, opts LintOptions) []LintIssue {
	var issues []LintIssue
	add := func(start, end int, severity, msg string) {
		issues = append(issues, LintIssue{Start: start, End: end, Severity: severity, Message: msg})
	}

	// Finding 155: memoized so a template that references the same partial many
	// times resolves it once per lint run.
	var partialExists func(string) bool
	if opts.PartialExists != nil {
		seen := map[string]bool{}
		partialExists = func(name string) bool {
			if got, ok := seen[name]; ok {
				return got
			}
			got := opts.PartialExists(name)
			seen[name] = got
			return got
		}
	}

	tokens := matchTokens(input)

	// Track real token start offsets so the loose-bracket scan can skip them.
	realStarts := make(map[int]bool, len(tokens))
	for _, tk := range tokens {
		realStarts[tk.start] = true
	}

	// Inner content per conditional block (opener.start -> inner text), for
	// counting [else] dividers.
	condInner := conditionalInnerRanges(input, tokens)

	// Inner byte spans of every [each] block, so [item] outside any [each] can
	// be flagged.
	eachSpans := namedBlockSpans(tokens, "each")
	reloadSpans := namedBlockSpans(tokens, "reload")

	// --- Structural checks over the token stream ---
	for _, tk := range tokens {
		known, isKnown := opts.Known[tk.name]

		if tk.closing {
			if isKnown && known.Block == BlockNo {
				add(tk.start, tk.end, SeverityError,
					"["+tk.name+"] is an inline shortcode and cannot have a closing tag")
				continue
			}
			if !tk.matched {
				add(tk.start, tk.end, SeverityError,
					"orphan closing tag [/"+tk.name+"] has no matching opener")
			}
			continue
		}

		// Opener: a block-required shortcode with no closer is unclosed.
		if isKnown && known.Block == BlockRequired && !tk.matched {
			add(tk.start, tk.end, SeverityError,
				"["+tk.name+"] must be a block: ["+tk.name+"]…[/"+tk.name+"]")
		}
	}

	// One markup scan for every inline [meta] in the template, rather than one
	// per shortcode: the scan is forward-only from the start of the document, so
	// doing it per token made a template with many of them quadratic.
	var inlineMetaPositions []int
	for _, tk := range tokens {
		if !tk.closing && tk.name == "meta" && tk.attrs["inline"] == "true" {
			inlineMetaPositions = append(inlineMetaPositions, tk.start)
		}
	}
	sort.Ints(inlineMetaPositions)
	inlineMetaContexts := attributeContextsAt(input, inlineMetaPositions)

	// --- Attribute and semantic checks over opener tokens ---
	for _, tk := range tokens {
		if tk.closing {
			continue
		}
		known, isKnown := opts.Known[tk.name]
		if !isKnown || !known.Documented {
			continue
		}

		// Missing required attributes.
		for name, a := range known.Attrs {
			if a.Wildcard || !a.Required {
				continue
			}
			if v, ok := tk.attrs[name]; !ok || strings.TrimSpace(v) == "" {
				add(tk.start, tk.end, SeverityError,
					"["+tk.name+"] is missing required attribute \""+name+"\"")
			}
		}

		// Unknown attributes (warning — documented shortcodes only).
		for attrName := range tk.attrs {
			if knownAttr(known, attrName) {
				continue
			}
			// [conditional] accepts numbered-suffix source/operator attrs
			// (path2, eq2, …) for multi-value conditions.
			if tk.name == "conditional" {
				if base := stripTrailingDigits(attrName); base != attrName && knownAttr(known, base) {
					continue
				}
			}
			add(tk.start, tk.end, SeverityWarning,
				"unknown attribute \""+attrName+"\" on ["+tk.name+"]")
		}

		// Escaping the value keeps it from breaking out of a quoted attribute,
		// and nothing more. Three surrounding contexts defeat it entirely, and
		// they matter because the boundary is real: category templates are
		// authored by admins/editors, but the Meta values they interpolate are
		// written by ordinary users.
		if tk.name == "meta" && tk.attrs["inline"] == "true" {
			for _, msg := range unsafeAttributeContexts(inlineMetaContexts[tk.start], tk.attrs["raw"] == "true") {
				add(tk.start, tk.end, SeverityWarning, msg)
			}
		}

		// Name-specific semantic checks.
		switch tk.name {
		case "item":
			if !insideSpans(tk.start, eachSpans) {
				add(tk.start, tk.end, SeverityWarning,
					"[item] is only meaningful inside an [each] block")
			}
		case "reload":
			// A <button> may not contain another button. The renderer refuses the
			// whole thing rather than emit invalid interactive nesting, so say so
			// here while the author can still see which one is at fault.
			if insideSpans(tk.start, reloadSpans) {
				add(tk.start, tk.end, SeverityError,
					"[reload] cannot contain another [reload]")
			}
		case "lazy", "details":
			// Both emit a block, and a button takes phrasing content only. The
			// renderer refuses them here for the same reason.
			if insideSpans(tk.start, reloadSpans) {
				add(tk.start, tk.end, SeverityError,
					"["+tk.name+"] cannot be used inside a [reload] button face")
			}
		case "partial":
			refName := strings.TrimSpace(tk.attrs["name"])
			if opts.PartialName != "" && tk.attrs["name"] == opts.PartialName {
				add(tk.start, tk.end, SeverityWarning,
					"[partial] references itself; this recursion stops at the depth limit")
			} else if partialExists != nil && refName != "" && !partialExists(refName) {
				// Finding 155. A warning rather than an error: the partial may be
				// about to be created, and lint must never block a save.
				add(tk.start, tk.end, SeverityWarning,
					`no template partial named "`+refName+`" exists; this renders nothing`)
			}
		case "meta":
			if strings.TrimSpace(tk.attrs["default"]) != "" && tk.attrs["hide-empty"] == "true" {
				add(tk.start, tk.end, SeverityWarning,
					`[meta] has both hide-empty and default; hide-empty wins and the default is never shown`)
			}
		case "mrql":
			if strings.TrimSpace(tk.attrs["query"]) == "" && strings.TrimSpace(tk.attrs["saved"]) == "" {
				add(tk.start, tk.end, SeverityError,
					"[mrql] requires a \"query\" or \"saved\" attribute")
			}
			// Inline scalar mode (value=) and a block body are mutually exclusive:
			// value= renders a single value, so the body is silently ignored.
			if strings.TrimSpace(tk.attrs["value"]) != "" && tk.matched {
				add(tk.start, tk.end, SeverityError,
					"[mrql value=…] renders a single value and cannot have a block body; the body is ignored")
			}
		case "conditional":
			if strings.TrimSpace(tk.attrs["path"]) == "" &&
				strings.TrimSpace(tk.attrs["field"]) == "" &&
				strings.TrimSpace(tk.attrs["mrql"]) == "" {
				add(tk.start, tk.end, SeverityError,
					"[conditional] needs a \"path\", \"field\", or \"mrql\" attribute to test")
			}
			if !hasAnyOperator(tk.attrs) {
				add(tk.start, tk.end, SeverityError,
					"[conditional] needs a comparison operator (eq, neq, gt, lt, gte, lte, in, contains, matches, empty, not-empty)")
			}
			// Invalid regex in any matches operator (matches, matches2, …) is an
			// error at edit time — at render it silently evaluates to false.
			for attrName, attrVal := range tk.attrs {
				if stripTrailingDigits(attrName) == "matches" {
					if _, err := regexp.Compile(attrVal); err != nil {
						start, end := attrOffset(tk, attrName)
						add(start, end, SeverityError,
							"invalid regular expression in "+attrName+": "+err.Error())
					}
				}
			}
			if inner, ok := condInner[tk.start]; ok && countTopLevelElse(inner) > 1 {
				add(tk.start, tk.end, SeverityError,
					"[conditional] has more than one [else] divider")
			}
		}

		// MRQL syntax check on query-bearing attributes.
		if opts.ValidateMRQL != nil {
			for _, attr := range []string{"query", "mrql"} {
				q, ok := tk.attrs[attr]
				if !ok || strings.TrimSpace(q) == "" {
					continue
				}
				if err := opts.ValidateMRQL(q); err != nil {
					start, end := attrOffset(tk, attr)
					add(start, end, SeverityError, "MRQL error in "+attr+": "+err.Error())
				}
			}
		}
	}

	// --- Loose bracket scan for shortcode-looking typos (info) ---
	for _, m := range looseBracketPattern.FindAllStringSubmatchIndex(input, -1) {
		pos := m[0]
		if realStarts[pos] {
			continue
		}
		name := input[m[2]:m[3]]
		if name == "else" || name == "elseif" {
			continue
		}
		if strings.HasPrefix(name, "plugin:") {
			add(pos, m[1], SeverityInfo,
				"malformed plugin shortcode; expected [plugin:<plugin>:<name> …]")
			continue
		}
		if suggestion := nearMissBuiltin(name); suggestion != "" {
			if suggestion == name {
				add(pos, m[1], SeverityInfo,
					"["+name+"…] looks like an incomplete or malformed shortcode")
			} else {
				add(pos, m[1], SeverityInfo,
					"unknown shortcode \"["+name+"]\"; did you mean ["+suggestion+"]?")
			}
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].Start != issues[j].Start {
			return issues[i].Start < issues[j].Start
		}
		return issues[i].End < issues[j].End
	})
	return issues
}

// knownAttr reports whether attrName is a recognized attribute on known,
// honoring wildcard prefix families (e.g. "param-tag" matches "param-").
func knownAttr(known KnownShortcode, attrName string) bool {
	if _, ok := known.Attrs[attrName]; ok {
		return true
	}
	for _, a := range known.Attrs {
		if a.Wildcard && len(attrName) > len(a.Name) && strings.HasPrefix(attrName, a.Name) {
			return true
		}
	}
	return false
}

// stripTrailingDigits removes a trailing run of ASCII digits, so "eq2" → "eq"
// and "path10" → "path". Used to recognize numbered-suffix conditional attrs.
func stripTrailingDigits(s string) string {
	end := len(s)
	for end > 0 && s[end-1] >= '0' && s[end-1] <= '9' {
		end--
	}
	return s[:end]
}

func hasAnyOperator(attrs map[string]string) bool {
	for _, op := range conditionalOperators {
		if _, ok := attrs[op]; ok {
			return true
		}
	}
	return false
}

// attrOffset returns a best-effort byte range for the given attribute within a
// token, falling back to the whole token span. Matches must sit on an attribute
// boundary (preceded by whitespace) so "query=" does not anchor inside a longer
// attribute like "param-query=".
func attrOffset(tk token, attr string) (int, int) {
	needle := attr + "="
	for from := 0; ; {
		idx := strings.Index(tk.raw[from:], needle)
		if idx < 0 {
			break
		}
		idx += from
		if idx > 0 {
			switch tk.raw[idx-1] {
			case ' ', '\t', '\n', '\r':
				return tk.start + idx, tk.end
			}
		}
		from = idx + 1
	}
	return tk.start, tk.end
}

// nearMissBuiltin returns a suggested built-in name for a misspelled bracket
// identifier, or "" if none is close. An exact match returns the name itself
// (signaling a malformed-but-recognizable bracket). Names that contain a
// built-in as a prefix never reach here — the shortcode regex parses them as a
// real shortcode (the extra characters become attributes) — so the reachable
// near-misses are proper abbreviations of a built-in or single-character typos.
func nearMissBuiltin(name string) string {
	for _, base := range builtinBaseNames {
		if name == base {
			return base
		}
	}
	if len(name) < 3 {
		return ""
	}
	for _, base := range builtinBaseNames {
		// Abbreviation: "condition" / "prop" for a longer built-in.
		if len(name) >= 4 && strings.HasPrefix(base, name) {
			return base
		}
		// Single-character typo (substitution / insertion / deletion).
		if editDistanceAtMost1(name, base) {
			return base
		}
	}
	return ""
}

// editDistanceAtMost1 reports whether a and b differ by at most one single-
// character edit (substitution, insertion, or deletion). Cheaper and less
// noisy than a full Levenshtein threshold for typo detection.
func editDistanceAtMost1(a, b string) bool {
	la, lb := len(a), len(b)
	if la > lb {
		a, b = b, a
		la, lb = lb, la
	}
	if lb-la > 1 {
		return false
	}
	if la == lb {
		diff := 0
		for i := 0; i < la; i++ {
			if a[i] != b[i] {
				diff++
				if diff > 1 {
					return false
				}
			}
		}
		return diff == 1
	}
	// Lengths differ by exactly 1: check b is a with one extra character.
	i, j := 0, 0
	edited := false
	for i < la && j < lb {
		if a[i] == b[j] {
			i++
			j++
			continue
		}
		if edited {
			return false
		}
		edited = true
		j++ // skip the extra character in the longer string
	}
	return true
}

// conditionalInnerRanges returns, for each conditional block, the inner content
// keyed by the opener token's start offset. Nested conditionals are included.
func conditionalInnerRanges(input string, tokens []token) map[int]string {
	result := make(map[int]string)
	var stack []int // indices into tokens of open conditional openers
	for i := range tokens {
		tk := tokens[i]
		if tk.name != "conditional" {
			continue
		}
		if !tk.closing {
			stack = append(stack, i)
			continue
		}
		// Closing conditional — pair with the nearest unclosed opener.
		if len(stack) == 0 {
			continue
		}
		openIdx := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		innerStart := tokens[openIdx].end
		innerEnd := tk.start
		if innerEnd > innerStart {
			result[tokens[openIdx].start] = input[innerStart:innerEnd]
		} else {
			result[tokens[openIdx].start] = ""
		}
	}
	return result
}

// namedBlockSpans returns the inner-content byte ranges [openerEnd, closerStart)
// of every block with the given name, pairing openers with closers by depth over
// the token stream. Used to place a token relative to a block: [item] tokens
// sitting outside any [each], [reload] tokens sitting inside another [reload].
// (Distinct from blockInnerSpans in split_else.go, which scans raw text for any
// block rather than the token stream for one name.)
func namedBlockSpans(tokens []token, name string) [][2]int {
	var spans [][2]int
	var stack []int
	for i := range tokens {
		if tokens[i].name != name {
			continue
		}
		if !tokens[i].closing {
			stack = append(stack, i)
			continue
		}
		if len(stack) == 0 {
			continue
		}
		openIdx := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		spans = append(spans, [2]int{tokens[openIdx].end, tokens[i].start})
	}
	return spans
}

// countTopLevelElse counts [else] dividers in content that are not nested
// inside a block shortcode, mirroring SplitElse's skip logic.
func countTopLevelElse(content string) int {
	blocks := ParseWithBlocks(content)
	count := 0
	i := 0
	for i < len(content) {
		if content[i] == '[' && strings.HasPrefix(content[i:], elseTag) {
			inside := false
			for _, b := range blocks {
				if !b.IsBlock {
					continue
				}
				openEnd := strings.Index(content[b.Start:], "]")
				if openEnd < 0 {
					continue
				}
				innerStart := b.Start + openEnd + 1
				closingTag := "[/" + b.Name + "]"
				innerEnd := b.End - len(closingTag)
				if i >= innerStart && i < innerEnd {
					inside = true
					break
				}
			}
			if !inside {
				count++
				i += len(elseTag)
				continue
			}
		}
		i++
	}
	return count
}

// attrContext is where one position falls in the markup around it.
type attrContext struct {
	attr       string // lowercased attribute name; "" when not in a value
	quote      byte   // the quote that opened the value, 0 when unquoted
	valueSoFar string // the value text before the position
	inValue    bool
}

// unsafeAttributeContexts reports the ways an inline [meta] value is still
// dangerous where it is being placed.
//
// html.EscapeString covers & < > ' " and nothing else — exactly enough to keep a
// value inside a quoted attribute, and nothing at all once the browser re-parses
// that value. The distinction has a privilege boundary behind it: an admin or
// editor writes the template, but the Meta value it interpolates is written by
// whoever can edit the entity, which includes the plain user role.
//
// raw disables even that much, so with it every attribute position is unsafe.
func unsafeAttributeContexts(ctx attrContext, raw bool) []string {
	if !ctx.inValue || ctx.attr == "" {
		return nil
	}
	attr := ctx.attr

	var out []string
	if raw {
		out = append(out, `[meta inline raw] is not escaped at all, so placing it in the "`+attr+`" attribute lets the value close the attribute and add its own. Drop raw= here.`)
	}
	if ctx.quote == 0 {
		out = append(out, `[meta inline] sits in an unquoted attribute value, where escaping does not stop a value containing a space from adding attributes of its own. Quote the attribute.`)
	}
	if strings.HasPrefix(attr, "on") {
		out = append(out, `[meta inline] sits in the "`+attr+`" event handler, where the HTML parser undoes the escaping before the script is parsed, so a value containing a quote can execute. Do not interpolate Meta into a handler.`)
	}
	if attr == "style" {
		out = append(out, `[meta inline] sits in a "style" attribute, where escaping does not prevent CSS injection.`)
	}
	if alwaysUnsafeAttrs[attr] {
		// srcdoc is decoded and then parsed as a whole document, so escaping
		// buys nothing wherever in the value the interpolation lands.
		out = append(out, `[meta inline] sits in a "`+attr+`" attribute, whose value the browser decodes and parses as HTML, so escaping does not prevent script injection anywhere in it.`)
	}
	if urlBearingAttrs[attr] && !urlSchemeAlreadyFixed(ctx.valueSoFar) {
		out = append(out, `[meta inline] can still choose the scheme of the "`+attr+`" URL, and escaping does not stop a "javascript:" value. Put a path or a complete scheme in front of it, e.g. href="/x/[meta ...]".`)
	}
	return out
}

// urlSchemeAlreadyFixed reports whether the text before the interpolation has
// already decided what kind of URL this is. A ":" means a scheme is chosen, and
// "/", "?" or "#" mean the URL is relative and can no longer become one.
//
// Emptiness is the wrong test: href="java[meta ...]" has a non-empty prefix and
// a Meta value of "script:alert(1)" still completes a javascript: URL.
func urlSchemeAlreadyFixed(prefix string) bool {
	return strings.ContainsAny(prefix, ":/?#")
}

// rawTextElements have bodies the HTML parser does not read as markup, so a "<"
// inside them opens nothing. Scanning into them desynchronizes everything after:
// a <script> containing the text `'<x a="'` would otherwise open an attribute
// value that swallows the next real tag.
var rawTextElements = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true, "xmp": true,
}

// attributeContextsAt answers, in one forward pass, where each of the given
// positions falls. positions must be ascending, which the caller has because
// shortcode tokens come out of the parser in source order.
//
// It scans forward rather than backwards from each position, because no single
// character marks a boundary reliably: onclick="if (x > 0) [meta …]" contains a
// ">" that does not end the tag, and onclick="if (x < 1) [meta …]" contains a
// "<" that does not start one. A backwards search mistakes both and goes quiet
// on exactly the placement that matters most. One pass for all positions also
// keeps a template with many shortcodes from being quadratic.
func attributeContextsAt(input string, positions []int) map[int]attrContext {
	out := make(map[int]attrContext, len(positions))
	if len(positions) == 0 {
		return out
	}
	next := 0
	// record consumes every position up to end, attributing ctx to those inside
	// [start, end] and "not in a value" to the rest.
	record := func(start, end int, ctx func(pos int) attrContext) {
		for next < len(positions) && positions[next] <= end {
			p := positions[next]
			if p >= start {
				out[p] = ctx(p)
			} else {
				out[p] = attrContext{}
			}
			next++
		}
	}

	i := 0
	for i < len(input) && next < len(positions) {
		if input[i] != '<' {
			i++
			continue
		}
		// "<" only opens a tag when a name, a slash, or a markup declaration
		// follows it; "a < b" is text.
		if i+1 >= len(input) || !isTagNameStart(input[i+1]) {
			i++
			continue
		}
		i++ // past '<'
		if input[i] == '!' || input[i] == '?' {
			for i < len(input) && input[i] != '>' {
				i++
			}
			continue
		}
		closing := input[i] == '/'
		if closing {
			i++
		}
		nameStart := i
		for i < len(input) && !isASCIISpace(input[i]) && input[i] != '>' && input[i] != '/' {
			i++
		}
		element := strings.ToLower(input[nameStart:i])

		// Attributes.
		for i < len(input) {
			for i < len(input) && (isASCIISpace(input[i]) || input[i] == '/') {
				i++
			}
			if i >= len(input) || input[i] == '>' {
				break
			}
			attrStart := i
			for i < len(input) && input[i] != '=' && !isASCIISpace(input[i]) && input[i] != '>' {
				i++
			}
			name := strings.ToLower(input[attrStart:i])
			record(0, attrStart-1, func(int) attrContext { return attrContext{} })
			for i < len(input) && isASCIISpace(input[i]) {
				i++
			}
			if i >= len(input) || input[i] != '=' {
				continue // valueless attribute
			}
			i++ // past '='
			for i < len(input) && isASCIISpace(input[i]) {
				i++
			}
			var q byte
			if i < len(input) && (input[i] == '"' || input[i] == '\'') {
				q = input[i]
				i++
			}
			valStart := i
			for i < len(input) {
				if q != 0 && input[i] == q {
					break
				}
				if q == 0 && (isASCIISpace(input[i]) || input[i] == '>') {
					break
				}
				i++
			}
			valEnd := i
			record(valStart, valEnd, func(pos int) attrContext {
				return attrContext{attr: name, quote: q, valueSoFar: input[valStart:pos], inValue: true}
			})
			if i < len(input) {
				i++ // past the closing quote or delimiter
			}
		}
		if i < len(input) && input[i] == '>' {
			i++
		}
		// A raw-text element's body is text, not markup: skip to its end tag so
		// a "<" in a string literal cannot open an attribute value.
		if !closing && rawTextElements[element] {
			if end := strings.Index(strings.ToLower(input[i:]), "</"+element); end >= 0 {
				record(0, i+end-1, func(int) attrContext { return attrContext{} })
				i += end
			} else {
				record(0, len(input), func(int) attrContext { return attrContext{} })
				i = len(input)
			}
		}
	}
	for ; next < len(positions); next++ {
		out[positions[next]] = attrContext{}
	}
	return out
}

func isTagNameStart(c byte) bool {
	return c == '/' || c == '!' || c == '?' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// alwaysUnsafeAttrs have values the browser decodes and then parses as markup,
// so escaping protects nothing wherever the interpolation lands in them.
var alwaysUnsafeAttrs = map[string]bool{"srcdoc": true}

// urlBearingAttrs are the attributes whose value the browser resolves as a URL,
// so a value in one can choose the scheme until a path or scheme fixes it.
var urlBearingAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"data": true, "poster": true, "xlink:href": true, "ping": true,
	"background": true, "cite": true, "longdesc": true, "manifest": true,
}
