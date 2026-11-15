package shortcodes

import (
	"encoding/json"
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

// metaSchemaProperties returns a metadata schema's top-level property names,
// or nil when it has none to check against.
func metaSchemaProperties(schema string) []string {
	if strings.TrimSpace(schema) == "" {
		return nil
	}
	var doc struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if json.Unmarshal([]byte(schema), &doc) != nil || len(doc.Properties) == 0 {
		return nil
	}
	names := make([]string, 0, len(doc.Properties))
	for name := range doc.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// metaPathIssue flags a [meta] path whose first segment is not a schema
// property; such a path renders empty.
func metaPathIssue(path string, props []string) string {
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
	return `[meta] path "` + path + `": "` + first + `" is not a property of the metadata schema, so it renders empty (properties: ` +
		strings.Join(shown, ", ") + ")"
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
func lintUnbalancedHTML(w *schemaWork, input string, add func(start, end int, severity, msg string)) {
	if !w.charge(1 + len(input)/16) {
		return
	}
	type open struct {
		name       string
		start, end int
	}
	var stack []open
	z := html.NewTokenizer(strings.NewReader(input))
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
		case html.StartTagToken:
			nameBytes, _ := z.TagName()
			name := string(nameBytes)
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
			stack = stack[:i]
		}
	}
}
