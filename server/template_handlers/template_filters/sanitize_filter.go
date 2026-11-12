package template_filters

import (
	"regexp"
	"strings"

	"github.com/flosch/pongo2/v4"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Issue #92. With -auth on, a Description or a note text block is written by
// accounts less trusted than whoever opens it (a subtree-confined user's text
// is read by an admin), so the markdown2 output (raw HTML included, KAN-4) has
// to be cut down to markup that cannot run anything before it reaches a page.
// Both render paths run Alpine over the result, so Alpine directives count as
// script just as <script> and on* handlers do.
//
// The filter sits after render_mentions and before process_shortcodes:
// mentions are part of the user's markup and go through it, shortcode output is
// the application's own and must not. That ordering has one consequence the
// policy handles: a shortcode expanded inside an attribute would land after
// the check (a URL attribute set to `[property path=Name]`), so "[" and "]" in
// attribute values are written as character references, which the browser
// reads as the same characters but the shortcode parser does not match. In
// text, where shortcodes are meant to expand, they are left alone.
//
// It is an allowlist over golang.org/x/net/html's parse of the fragment: an
// element not listed is unwrapped (its children kept), except the ones whose
// content is code or is parsed differently (dropRaw), which go entirely; an
// attribute not listed for its element is dropped, and a URL attribute must
// be relative or use a listed scheme. The tree is written back by sanitizeRender
// rather than html.Render, to control the escaping above.

// sanitizeUnderAuthFilter is the `sanitize_under_auth` filter. Its parameter
// is whether auth is on; only an explicit false skips sanitizing, so a page
// whose context forgot to say renders sanitized rather than raw.
func sanitizeUnderAuthFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if enabled, ok := param.Interface().(bool); ok && !enabled {
		return in, nil
	}
	return pongo2.AsValue(SanitizeUserHTML(in.String())), nil
}

// SanitizeUserHTML returns fragment with everything that could run script
// removed. See the comment at the top of this file for the policy.
func SanitizeUserHTML(fragment string) string {
	context := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		// x/net/html reports only reader errors here; a strings.Reader has none.
		return html.EscapeString(fragment)
	}
	var b strings.Builder
	for _, n := range nodes {
		sanitizeRender(&b, n)
	}
	return b.String()
}

// dropRaw are removed together with their content: script and style bodies,
// raw-text and foreign (SVG/MathML) content whose re-parse can differ from its
// parse, embedded documents and plugins, and document-level elements.
var dropRaw = map[string]bool{
	"script": true, "style": true, "template": true, "noscript": true,
	"iframe": true, "frame": true, "frameset": true, "object": true, "embed": true, "applet": true,
	"textarea": true, "title": true, "xmp": true, "noembed": true, "noframes": true, "plaintext": true,
	"svg": true, "math": true, "base": true, "link": true, "meta": true, "head": true,
	"select": true, "option": true, "button": true, "form": true,
}

// voidElements have no end tag.
var voidElements = map[string]bool{"br": true, "hr": true, "img": true, "input": true, "col": true, "wbr": true}

// globalAttrs are allowed on every allowed element.
var globalAttrs = map[string]bool{"class": true, "title": true, "lang": true, "dir": true}

// allowedElements maps each allowed element to the attributes it may carry
// beyond globalAttrs.
var allowedElements = map[string]map[string]bool{
	"a": {"href": true}, "img": {"src": true, "alt": true, "width": true, "height": true, "loading": true},
	"p": nil, "div": nil, "span": nil, "br": nil, "hr": nil, "wbr": nil,
	"h1": {"id": true}, "h2": {"id": true}, "h3": {"id": true}, "h4": {"id": true}, "h5": {"id": true}, "h6": {"id": true},
	"strong": nil, "b": nil, "em": nil, "i": nil, "u": nil, "s": nil, "del": {"cite": true}, "ins": {"cite": true},
	"mark": nil, "small": nil, "sub": nil, "sup": nil, "abbr": nil, "cite": nil, "q": {"cite": true},
	"code": nil, "pre": nil, "kbd": nil, "samp": nil, "var": nil, "time": {"datetime": true},
	"blockquote": {"cite": true}, "ul": nil, "ol": {"start": true, "reversed": true}, "li": {"value": true},
	"dl": nil, "dt": nil, "dd": nil, "details": {"open": true}, "summary": nil, "figure": nil, "figcaption": nil,
	"table": nil, "caption": nil, "colgroup": {"span": true}, "col": {"span": true},
	"thead": nil, "tbody": nil, "tfoot": nil, "tr": nil,
	"th": {"colspan": true, "rowspan": true, "align": true, "style": true, "scope": true},
	"td": {"colspan": true, "rowspan": true, "align": true, "style": true},
	// GFM task lists render as disabled checkboxes; nothing else of <input>.
	"input": {"type": true, "checked": true, "disabled": true},
}

var urlAttrs = map[string]bool{"href": true, "src": true, "cite": true}

// tableAlignStyle is the only style allowed: what goldmark writes for an
// aligned GFM table column.
var tableAlignStyle = regexp.MustCompile(`^text-align:\s*(left|right|center);?$`)

// dataImageURL is the only data: URL allowed, and only in <img src>.
var dataImageURL = regexp.MustCompile(`^data:image/(png|gif|jpeg|webp);base64,`)

func allowedAttr(element string, a html.Attribute) bool {
	if a.Namespace != "" {
		return false
	}
	if !globalAttrs[a.Key] && !allowedElements[element][a.Key] {
		return false
	}
	switch {
	case urlAttrs[a.Key]:
		return safeURL(a.Val, element == "img" && a.Key == "src")
	case a.Key == "style":
		return tableAlignStyle.MatchString(a.Val)
	case element == "input" && a.Key == "type":
		return a.Val == "checkbox"
	}
	return true
}

// allowedClass matches the class names the rendered markup itself uses: a
// fenced code block's language and the mention links render_mentions writes.
// Any other class is dropped, so stored text cannot borrow the page's utility
// classes to lay itself over the page (a full-screen "fixed inset-0" link).
var allowedClass = regexp.MustCompile(`^(language-[A-Za-z0-9_+#.-]+|mention-[a-z-]+)$`)

func filterClasses(v string) string {
	var kept []string
	for _, c := range strings.Fields(v) {
		if allowedClass.MatchString(c) {
			kept = append(kept, c)
		}
	}
	return strings.Join(kept, " ")
}

// safeURL reports whether u is relative or uses http, https or mailto. The
// scheme is read the way a browser reads it after dropping the whitespace and
// control characters it ignores, so "java\tscript:" is javascript.
func safeURL(u string, image bool) bool {
	compact := strings.Map(func(r rune) rune {
		if r <= ' ' || r == 0x7f {
			return -1
		}
		return r
	}, u)
	end := strings.IndexAny(compact, ":/?#")
	if end == -1 || compact[end] != ':' {
		return true // no scheme: a relative URL
	}
	switch strings.ToLower(compact[:end]) {
	case "http", "https", "mailto":
		return true
	case "data":
		return image && dataImageURL.MatchString(strings.ToLower(compact))
	}
	return false
}

// attrEscaper escapes an attribute value for a double-quoted attribute, and
// writes the shortcode brackets as character references (see the top of the
// file).
var attrEscaper = strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `'`, "&#39;", `<`, "&lt;", `>`, "&gt;", `[`, "&#91;", `]`, "&#93;")

// textEscaper escapes text the way goldmark does, leaving "'" and the
// brackets as they are, so shortcodes in text parse exactly as before.
var textEscaper = strings.NewReplacer(`&`, "&amp;", `"`, "&quot;", `<`, "&lt;", `>`, "&gt;")

func sanitizeRender(b *strings.Builder, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		b.WriteString(textEscaper.Replace(n.Data))
		return
	case html.ElementNode:
	default:
		return // comments, doctypes
	}
	if n.Namespace != "" || dropRaw[n.Data] {
		return
	}
	if _, ok := allowedElements[n.Data]; !ok {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			sanitizeRender(b, c)
		}
		return
	}
	b.WriteString("<" + n.Data)
	for _, a := range n.Attr {
		if a.Key == "class" {
			if a.Val = filterClasses(a.Val); a.Val == "" {
				continue
			}
		}
		if allowedAttr(n.Data, a) {
			b.WriteString(" " + a.Key + `="` + attrEscaper.Replace(a.Val) + `"`)
		}
	}
	b.WriteString(">")
	if voidElements[n.Data] {
		return
	}
	// The parser drops a newline right after <pre>; write one back so a
	// leading newline in the content survives the next parse.
	if n.Data == "pre" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode && strings.HasPrefix(n.FirstChild.Data, "\n") {
		b.WriteString("\n")
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sanitizeRender(b, c)
	}
	b.WriteString("</" + n.Data + ">")
}
