package shortcodes

import (
	"regexp"
	"sort"
	"strconv"
	"strings"

	"mahresources/mrql"
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

// MRQLCheckOptions are what a shortcode's own attributes do to its query at
// render time: param-<name> binds $name, limit replaces any LIMIT in the
// query, and buckets caps the buckets of a GROUP BY.
type MRQLCheckOptions struct {
	Params  map[string]string
	Limit   int
	Buckets int
	// ParseOnly asks only whether the query parses. A plugin shortcode's
	// query attribute is the plugin's own, so the built-in execution rules do
	// not apply to it.
	ParseOnly bool
}

// mrqlCheckOptions reads a query source's options the way the runtime does
// (RenderMRQLShortcode, resolveConditionalValue): suffix is "" for the base
// source and "2", "3", … for a conditional's numbered sources.
func mrqlCheckOptions(attrs map[string]string, suffix string) MRQLCheckOptions {
	return MRQLCheckOptions{
		Params:  collectShortcodeParams(attrs),
		Limit:   parseIntAttr(attrs["limit"+suffix], defaultMRQLShortcodeLimit),
		Buckets: parseIntAttr(attrs["buckets"+suffix], defaultMRQLShortcodeBuckets),
	}
}

// KnownShortcodes maps a shortcode name to its descriptor.
type KnownShortcodes map[string]KnownShortcode

// LintOptions configures a Lint run.
type LintOptions struct {
	// Known is the shortcode catalogue (built-ins plus enabled plugins). When
	// nil, only structural checks that don't need the catalogue run.
	Known KnownShortcodes
	// ValidateMRQL, when non-nil, validates every query the runtime would
	// execute, with the options the runtime applies to it, and its error is
	// surfaced as a lint issue. nil skips MRQL checks.
	ValidateMRQL func(query string, opts MRQLCheckOptions) error
	// PartialName, when non-empty, is the name of the partial whose content is
	// being linted. A [partial name="<PartialName>"] reference inside it is a
	// direct self-reference and is flagged as a warning.
	PartialName string
	// CSSMode marks content that is a stylesheet rather than markup, which is
	// what a CustomCSS slot holds. Nothing in the text says so — it carries no
	// <style> wrapper of its own — so an inline [meta] there has to be judged as
	// landing in CSS rather than as ordinary text.
	CSSMode bool
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
var builtinBaseNames = []string{"meta", "datetime", "pills", "property", "mrql", "conditional", "link", "each", "item", "partial", "lazy", "details", "reload", "using"}

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
	usingSpans := namedBlockSpans(tokens, "using")

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

	// One markup scan for every bare value in the template, rather than one per
	// shortcode: the scan is forward-only from the start of the document, so
	// doing it per token made a template with many of them quadratic.
	// Every shortcode that emits a bare value gets the placement analysis, not
	// just [meta inline]: the danger is the value landing where escaping does
	// not reach, and [property], [item] and [mrql value=] put values in exactly
	// the same places. [property path="Description" raw="true"] is in this
	// repo's own reference panel.
	var valueSpans []bareValueSpan
	for _, tk := range tokens {
		if !tk.closing && emitsBareValue(tk.name, tk.attrs) {
			valueSpans = append(valueSpans, bareValueSpan{start: tk.start, end: tk.end})
		}
	}
	valueContexts, valueContextsOff := attributeContextsFor(input, valueSpans)

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
		if emitsBareValue(tk.name, tk.attrs) {
			label := "[" + tk.name + "]"
			if tk.name == "meta" {
				label = "[meta inline]"
			}
			raw := tk.attrs["raw"] == "true"
			seen := map[string]bool{}
			for _, msg := range unsafeAttributeContexts(valueContexts[tk.start], raw, opts.CSSMode, label) {
				if seen[msg] {
					continue
				}
				seen[msg] = true
				add(tk.start, tk.end, SeverityWarning, msg)
			}
			// The scripting-disabled reading, for the occurrences whose two
			// browser readings differ. Its warnings carry the scripting-mode
			// qualifier; identical strings are not repeated.
			if off, ok := valueContextsOff[tk.start]; ok {
				for _, msg := range unsafeAttributeContexts(off, raw, opts.CSSMode, label) {
					if seen[msg] {
						continue
					}
					seen[msg] = true
					add(tk.start, tk.end, SeverityWarning, msg)
				}
			}
		}

		// Name-specific semantic checks.
		switch tk.name {
		case "item":
			if !insideSpans(tk.start, eachSpans) {
				add(tk.start, tk.end, SeverityWarning,
					"[item] is only meaningful inside an [each] block")
			} else if innermostSpanStart(tk.start, usingSpans) > innermostSpanStart(tk.start, eachSpans) {
				add(tk.start, tk.end, SeverityWarning,
					"[item] inside a [using] block renders nothing: [using] switches to another entity, so the [each] element is not available there")
			}
		case "using":
			lintUsing(tk, add)
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
			// raw, format and layout are documented attributes of [meta], so
			// they no longer draw an "unknown attribute" warning — but only the
			// inline renderer reads them (RenderMetaShortcode branches on
			// inline=="true"), so without it they do nothing and, until this,
			// nothing said so. raw is the one that matters: an author who wrote
			// it believes escaping is off when it is not.
			//
			// raw="false" is deliberately quiet. It is a no-op the author asked
			// for and is the default anyway, so reporting it is noise; only a
			// raw= that was meant to change something is worth a line.
			if tk.attrs["inline"] != "true" {
				if tk.attrs["raw"] == "true" {
					add(tk.start, tk.end, SeverityWarning,
						`[meta] has raw="true" without inline="true"; only the inline form reads it, so it is ignored and the value is still escaped`)
				}
				// Not TrimSpace: formatTimeValue reads layout != "", so
				// layout=" " is an active layout that formats a date as one
				// space. An attribute that does something is worth a line even
				// when what it does is silly.
				for _, attr := range []string{"format", "layout"} {
					if tk.attrs[attr] != "" {
						add(tk.start, tk.end, SeverityWarning,
							`[meta] has `+attr+`= without inline="true"; only the inline form reads it, so the formatting you asked for is ignored`)
					}
				}
			} else if tk.attrs["editable"] == "true" {
				// The mirror of the same rule: inline emits the bare value, so
				// there is no widget for editable to turn on.
				add(tk.start, tk.end, SeverityWarning,
					`[meta] has editable="true" with inline="true", which renders the bare value and no widget, so editable is ignored`)
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

		// MRQL check on every query the runtime executes, with its options.
		if opts.ValidateMRQL != nil {
			check := func(query string, checkOpts MRQLCheckOptions, start, end int, label string) {
				if strings.TrimSpace(query) == "" {
					return
				}
				if err := opts.ValidateMRQL(query, checkOpts); err != nil {
					add(start, end, SeverityError, "MRQL error in "+label+": "+err.Error())
				}
			}
			switch tk.name {
			case "mrql":
				// The runtime prefers a non-empty inline query over saved=, so a
				// saved query alone is the only one left unchecked.
				start, end := attrOffset(tk, "query")
				check(tk.attrs["query"], mrqlCheckOptions(tk.attrs, ""), start, end, "query")
			case "using":
				start, end := attrOffset(tk, "query")
				check(tk.attrs["query"], MRQLCheckOptions{Params: collectShortcodeParams(tk.attrs), Limit: 1}, start, end, "query")
			case "conditional":
				forEachConditionalQuery(tk.attrs, func(attr, suffix string) {
					start, end := attrOffset(tk, attr)
					check(tk.attrs[attr], mrqlCheckOptions(tk.attrs, suffix), start, end, attr)
				})
				// [elseif] is a divider, not a token, but its guard runs.
				if inner, ok := condInner[tk.start]; ok {
					dividers := SlotTagSpans("conditional", inner)
					for i, branch := range SplitBranches(inner) {
						if i == 0 || branch.IsElse {
							continue
						}
						span := dividers[i-1]
						guard := token{start: tk.end + span[0], end: tk.end + span[1], raw: inner[span[0]:span[1]]}
						forEachConditionalQuery(branch.Attrs, func(attr, suffix string) {
							start, end := attrOffset(guard, attr)
							check(branch.Attrs[attr], mrqlCheckOptions(branch.Attrs, suffix), start, end, "[elseif] "+attr)
						})
					}
				}
			default:
				// A plugin shortcode's query is the plugin's own: only parsed.
				// No other built-in runs a query.
				if strings.HasPrefix(tk.name, "plugin:") {
					for _, attr := range []string{"query", "mrql"} {
						start, end := attrOffset(tk, attr)
						check(tk.attrs[attr], MRQLCheckOptions{ParseOnly: true}, start, end, attr)
					}
				}
			}
		}
	}

	lintStrayCloseTags(input, tokens, add)

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

// lintUsing checks a [using] opener: exactly one target source, a type exactly
// where the source needs one, and values the handler would refuse. Each error
// here is a case RenderUsingShortcode renders as an error marker, and each
// warning one where it silently ignores what the author wrote.
func lintUsing(tk token, add func(start, end int, severity, msg string)) {
	var sources []string
	for _, name := range usingSources {
		if strings.TrimSpace(tk.attrs[name]) != "" {
			sources = append(sources, name)
		}
	}
	switch len(sources) {
	case 0:
		add(tk.start, tk.end, SeverityError,
			"[using] needs one of id, id-path, id-prop, rel or query to name its target")
		return
	case 1:
	default:
		add(tk.start, tk.end, SeverityError,
			"[using] takes exactly one of id, id-path, id-prop, rel or query; found "+strings.Join(sources, ", "))
		return
	}
	source := sources[0]

	typ := strings.TrimSpace(tk.attrs["type"])
	switch source {
	case "rel", "query":
		if typ != "" {
			add(tk.start, tk.end, SeverityWarning,
				`[using] ignores type= with `+source+`=: `+map[string]string{"rel": "rel always names a group", "query": "the query names its own type"}[source])
		}
	default:
		if typ == "" {
			add(tk.start, tk.end, SeverityError,
				`[using] with `+source+`= needs type="group", "resource" or "note"`)
		} else if !usingTypes[typ] {
			start, end := attrOffset(tk, "type")
			add(start, end, SeverityError,
				`[using] type must be "group", "resource" or "note", not "`+typ+`"`)
		}
	}

	if source != "query" {
		_, hasScope := tk.attrs["scope"]
		if hasScope || len(collectShortcodeParams(tk.attrs)) > 0 {
			add(tk.start, tk.end, SeverityWarning,
				"[using] reads scope= and param-… only with query=; a lookup by ID ignores them")
		}
	}

	switch source {
	case "id":
		if _, ok := parseEntityID(strings.TrimSpace(tk.attrs["id"])); !ok {
			start, end := attrOffset(tk, "id")
			add(start, end, SeverityError, "[using] id must be a positive whole number")
		}
	case "id-prop":
		prop := strings.TrimSpace(tk.attrs["id-prop"])
		if strings.HasSuffix(prop, "CategoryId") || strings.HasSuffix(prop, "TypeId") {
			start, end := attrOffset(tk, "id-prop")
			add(start, end, SeverityWarning,
				"[using] id-prop=\""+prop+"\" holds a category ID, and categories are not a [using] target; this renders the [else] branch or the wrong entity")
		}
	case "rel":
		if rel := strings.TrimSpace(tk.attrs["rel"]); rel != "owner" && rel != "root" {
			start, end := attrOffset(tk, "rel")
			add(start, end, SeverityError, `[using] rel must be "owner" or "root", not "`+rel+`"`)
		}
	case "query":
		// Shape checks only; whether the query is valid at all is the
		// ValidateMRQL pass's job, so a parse failure is left to it.
		parsed, err := mrql.Parse(tk.attrs["query"])
		if err != nil {
			return
		}
		start, end := attrOffset(tk, "query")
		if parsed.GroupBy != nil {
			add(start, end, SeverityError,
				"[using] query cannot use GROUP BY: it needs one entity, not buckets")
			return
		}
		if mrql.ExtractEntityType(parsed) == mrql.EntityUnspecified {
			add(start, end, SeverityWarning,
				"[using] query does not name an entity type (type = group, resource or note), so its first result may be any of the three")
		}
		if len(parsed.OrderBy) == 0 {
			add(start, end, SeverityWarning,
				"[using] query has no ORDER BY, so which match becomes the target is not stable")
		}
	}
}

// innermostSpanStart returns the start of the innermost span containing i, or
// -1 when none does. Spans of one block name nest, so the innermost is the one
// that starts last.
func innermostSpanStart(i int, spans [][2]int) int {
	best := -1
	for _, s := range spans {
		if i >= s[0] && i < s[1] && s[0] > best {
			best = s[0]
		}
	}
	return best
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

// forEachConditionalQuery visits the mrql sources a condition set runs, in
// the order evaluateConditionSet runs them: the base condition, then the
// numbered ones until the first number with no source of any kind.
func forEachConditionalQuery(attrs map[string]string, visit func(attr, suffix string)) {
	for i := 1; ; i++ {
		suffix := ""
		if i >= 2 {
			suffix = strconv.Itoa(i)
			if !hasConditionSource(attrs, suffix) {
				return
			}
		}
		if attrs["mrql"+suffix] != "" {
			visit("mrql"+suffix, suffix)
		}
	}
}

// lintStrayCloseTags reports closing tags the runtime leaves on the page as
// literal text when their body renders. [else] and [elseif] are dividers with no closing tag, so a
// written [/else] is never consumed. [header] and [footer] are real pairs, but
// only the first complete top-level pair inside an [mrql] block is read
// (SlotTagSpans), so any other [/header] or [/footer] is literal too.
func lintStrayCloseTags(input string, tokens []token, add func(start, end int, severity, msg string)) {
	consumed := map[int]bool{}
	for _, span := range namedBlockSpans(tokens, "mrql") {
		for _, s := range SlotTagSpans("mrql", input[span[0]:span[1]]) {
			consumed[span[0]+s[0]] = true
		}
	}
	for _, name := range []string{"else", "elseif", mrqlHeaderTag, mrqlFooterTag} {
		closer := "[/" + name + "]"
		for from := 0; ; {
			i := strings.Index(input[from:], closer)
			if i < 0 {
				break
			}
			i += from
			from = i + len(closer)
			switch name {
			case "else", "elseif":
				add(i, from, SeverityError,
					"["+name+"] has no closing tag; remove "+closer+", it may be ignored or left as literal text")
			default:
				if !consumed[i] {
					add(i, from, SeverityError,
						closer+" closes no slot; ["+name+"]…"+closer+" is read once, as the first top-level pair directly inside an [mrql] block, and anything else may be ignored or left as literal text")
				}
			}
		}
	}
}
