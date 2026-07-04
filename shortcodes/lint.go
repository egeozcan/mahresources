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
var builtinBaseNames = []string{"meta", "property", "mrql", "conditional", "link", "each", "item", "partial"}

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
	eachSpans := eachInnerSpans(tokens)

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

		// Name-specific semantic checks.
		switch tk.name {
		case "item":
			if !insideSpans(tk.start, eachSpans) {
				add(tk.start, tk.end, SeverityWarning,
					"[item] is only meaningful inside an [each] block")
			}
		case "partial":
			if opts.PartialName != "" && tk.attrs["name"] == opts.PartialName {
				add(tk.start, tk.end, SeverityWarning,
					"[partial] references itself; this recursion stops at the depth limit")
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

// eachInnerSpans returns the inner-content byte ranges [openerEnd, closerStart)
// of every [each] block, pairing openers with closers by depth over the token
// stream. Used to detect [item] tokens sitting outside any [each].
func eachInnerSpans(tokens []token) [][2]int {
	var spans [][2]int
	var stack []int
	for i := range tokens {
		if tokens[i].name != "each" {
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
