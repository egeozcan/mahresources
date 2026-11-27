package shortcodes

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
)

// looksLikeShortcode reports whether a bracketed word that is not a shortcode
// stands where a shortcode would: on its own, not as an index or a selector
// (styles[class], array[0], a.b[c]), lowercase like every shortcode name, and
// directly closed or followed by an attribute. Such text renders verbatim,
// which is rarely what a template author meant.
func looksLikeShortcode(input string, start, nameEnd int, name string) bool {
	if strings.HasPrefix(input[start:], "[/") || strings.Contains(name, ":") || slotDividers[name] {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	if start > 0 {
		prev := input[start-1]
		if prev == '_' || prev == ']' || prev == ')' || prev == '.' ||
			prev >= 'a' && prev <= 'z' || prev >= 'A' && prev <= 'Z' || prev >= '0' && prev <= '9' {
			return false
		}
	}
	if nameEnd >= len(input) {
		return false
	}
	next := input[nameEnd]
	return next == ']' || next == ' '
}

// slotDividers are bracket words a parent shortcode reads as its own slots
// and dividers ([mrql] header and footer, [else], [item]); they are not
// tokens, but they are not literal text either.
var slotDividers = map[string]bool{mrqlHeaderTag: true, mrqlFooterTag: true, "else": true, "elseif": true, "item": true}

// metaSchemaProperties returns a metadata schema's declared top-level
// property names, or nil when there is nothing reliable to check against: no
// properties, or a schema composed with allOf/anyOf/oneOf/$ref or pattern
// properties, whose effective property set this does not compute.
func metaSchemaProperties(schema string) []string {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var doc map[string]json.RawMessage
	if json.Unmarshal([]byte(schema), &doc) != nil {
		return nil
	}
	for _, k := range []string{"allOf", "anyOf", "oneOf", "$ref", "patternProperties", "if"} {
		if _, ok := doc[k]; ok {
			return nil
		}
	}
	var props map[string]json.RawMessage
	if json.Unmarshal(doc["properties"], &props) != nil || len(props) == 0 {
		return nil
	}
	names := make([]string, 0, len(props))
	for name := range props {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// metaPathIssue flags a [meta] path whose first segment the schema does not
// declare. Metadata may hold undeclared keys, which [meta] still shows, so
// this is a likely typo rather than a certain one.
func metaPathIssue(path string, props []string) string {
	return metaPathIssueFor("[meta]", "it renders empty", path, props)
}

// metaPathIssueFor is metaPathIssue for the shortcode named by label, which
// leads the message; typo says what a mistyped path does to that shortcode.
func metaPathIssueFor(label, typo, path string, props []string) string {
	path = strings.TrimSpace(path)
	if len(props) == 0 || path == "" {
		return ""
	}
	first := path
	if i := strings.IndexAny(first, ".["); i >= 0 {
		first = first[:i]
	}
	for _, p := range props {
		if p == first {
			return ""
		}
	}
	shown := props
	if len(shown) > 12 {
		shown = append(append([]string{}, shown[:12]...), "…")
	}
	return label + ` path "` + path + `": "` + first + `" is not declared in the metadata schema (declared: ` +
		strings.Join(shown, ", ") + `); if it is a typo, ` + typo
}

func withinSpans(pos int, spans [][2]int) bool {
	for _, s := range spans {
		if pos >= s[0] && pos < s[1] {
			return true
		}
	}
	return false
}

var elseifPattern = regexp.MustCompile(`\[elseif\b(?:"[^"]*"|'[^']*'|[^\]"'])*\]`)

var styleStartPattern = regexp.MustCompile(`(?i)<style[\t\n\f\r />]`)

// styleElementSpans marks only the CSS bodies, not style-like text in a
// script, comment or attribute. Skip the markup scan when no opener can occur.
func styleElementSpans(input string) [][2]int {
	if !styleStartPattern.MatchString(input) {
		return nil
	}
	var spans [][2]int
	z := html.NewTokenizer(strings.NewReader(input))
	inside := -1
	for pos := 0; ; {
		tt := z.Next()
		end := pos + len(z.Raw())
		switch tt {
		case html.ErrorToken:
			if inside >= 0 {
				spans = append(spans, [2]int{inside, len(input)})
			}
			return spans
		case html.StartTagToken, html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == "style" {
				if tt == html.StartTagToken {
					inside = end
				} else if inside >= 0 {
					spans = append(spans, [2]int{inside, pos})
					inside = -1
				}
			}
		}
		pos = end
	}
}

// htmlVoid elements have no closing tag; htmlOptionalClose ones may omit it.
var (
	htmlVoid = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
		"img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}
	htmlOptionalClose = map[string]bool{"p": true, "li": true, "dt": true, "dd": true, "tr": true, "td": true,
		"th": true, "thead": true, "tbody": true, "tfoot": true, "option": true, "optgroup": true, "rp": true,
		"rt": true, "colgroup": true, "caption": true, "html": true, "head": true, "body": true}
)

// lintUnbalancedHTML warns about elements left open and closing tags that
// close nothing. A browser repairs both, but an element left open in one slot
// swallows whatever the page renders after it. Elements whose closing tag is
// optional are not tracked. Shortcode branches can open and close an element
// on different sides, so these are warnings.
func lintUnbalancedHTML(w *schemaWork, input string, tokens []token, add func(start, end int, severity, msg string)) {
	if !w.charge(1 + len(input)/16) {
		return
	}
	// Shortcode openers are blanked out, length for length, so an attribute
	// value such as eq="<div>" is not read as markup and offsets still match.
	masked := []byte(input)
	blank := func(start, end int) {
		for i := start; i < end && i < len(masked); i++ {
			if masked[i] != '\n' {
				masked[i] = ' '
			}
		}
	}
	for _, tk := range tokens {
		blank(tk.start, tk.end)
	}
	// [elseif] guards are dividers, not tokens, and carry attributes too.
	for _, m := range elseifPattern.FindAllStringIndex(input, -1) {
		blank(m[0], m[1])
	}
	type open struct {
		name       string
		start, end int
	}
	var stack []open
	foreign := 0 // depth inside <svg> or <math>, where <x/> is a complete element
	z := html.NewTokenizer(strings.NewReader(string(masked)))
	pos := 0
	for {
		tt := z.Next()
		raw := len(z.Raw())
		start, end := pos, pos+raw
		pos = end
		switch tt {
		case html.ErrorToken:
			for _, o := range stack {
				add(o.start, o.end, SeverityWarning, "<"+o.name+"> is never closed; it will contain everything rendered after this slot")
			}
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			nameBytes, _ := z.TagName()
			name := string(nameBytes)
			if tt == html.SelfClosingTagToken && (foreign > 0 || name == "svg" || name == "math") {
				continue
			}
			if name == "svg" || name == "math" {
				foreign++
			}
			// Outside SVG and MathML, <div/> opens a <div>: the slash is ignored.
			if !htmlVoid[name] && !htmlOptionalClose[name] {
				stack = append(stack, open{name, start, end})
			}
		case html.EndTagToken:
			nameBytes, _ := z.TagName()
			name := string(nameBytes)
			if htmlVoid[name] || htmlOptionalClose[name] {
				continue
			}
			i := len(stack) - 1
			for i >= 0 && stack[i].name != name {
				i--
			}
			if i < 0 {
				add(start, end, SeverityWarning, "</"+name+"> closes no open <"+name+">")
				continue
			}
			for _, o := range stack[i+1:] {
				add(o.start, o.end, SeverityWarning, "<"+o.name+"> is not closed before </"+name+">")
			}
			for _, o := range stack[i:] {
				if (o.name == "svg" || o.name == "math") && foreign > 0 {
					foreign--
				}
			}
			stack = stack[:i]
		}
	}
}
