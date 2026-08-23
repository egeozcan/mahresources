package shortcodes

import (
	"math/rand"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The fixed nesting sweeps above enumerate the shapes somebody thought of.
// This one asks about the shapes nobody did: random tag soup — start tags,
// stray end tags, self-closers, CDATA — in front of each probe, judged by the
// same parser oracle. Deterministic seeds, so a failure is a reproduction and
// not a flake. It found two real families the fixed corpus missed (an
// unclosed <textarea> inside a <noscript> body, and "</li>"/"</table>"
// popping past a <div> the any-other walk stops at), which is the argument
// for keeping it.
func TestLintDifferentialTagSoup(t *testing.T) {
	type probe struct {
		name                      string
		lintSrc, oracleSrc        string
		element, attr, attrPrefix string
		namespaces                []string
		childText                 bool
		scriptingOff              bool
		msg                       string
	}
	probes := []probe{
		{name: "javascript URL", msg: `continues a "javascript:" URL`,
			lintSrc:   `<a href="javascript:[property path='Name']">go</a>`,
			oracleSrc: `<a href="javascript:VALUE">go</a>`,
			element:   "a", attr: "href", attrPrefix: "javascript:",
			namespaces: []string{"", "svg", "math"}},
		{name: "srcdoc", msg: `sits in a "srcdoc" attribute`,
			lintSrc:   `<iframe srcdoc="[property path='Name']"></iframe>`,
			oracleSrc: `<iframe srcdoc="VALUE"></iframe>`,
			element:   "iframe", attr: "srcdoc",
			namespaces: []string{""}, scriptingOff: true},
		{name: "script program", msg: "JavaScript",
			lintSrc:   `<script>var s = "[property path='Name']"</script>`,
			oracleSrc: `<script>var s = "VALUE"</script>`,
			element:   "script", namespaces: []string{"", "svg"}, childText: true},
	}
	exists := func(src string, p probe, scripting bool) bool {
		opts := []html.ParseOption{}
		if !scripting {
			opts = append(opts, html.ParseOptionEnableScripting(false))
		}
		doc, err := html.ParseWithOptions(strings.NewReader(src), opts...)
		if err != nil {
			return false
		}
		var walk func(*html.Node) bool
		walk = func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == p.element {
				for _, ns := range p.namespaces {
					if n.Namespace != ns {
						continue
					}
					switch {
					case p.childText:
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							if c.Type == html.TextNode && strings.Contains(c.Data, "VALUE") {
								return true
							}
						}
					case p.attr == "":
						return true
					default:
						for _, a := range n.Attr {
							if a.Key == p.attr && strings.HasPrefix(strings.ToLower(a.Val), p.attrPrefix) {
								return true
							}
						}
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if walk(c) {
					return true
				}
			}
			return false
		}
		return walk(doc)
	}

	tokens := []string{
		"<svg>", "</svg>", "<math>", "</math>", "<div>", "</div>", "<b>", "</b>",
		"<a>", "</a>", "<nobr>", "</nobr>", "<g>", "</g>", "<iframe>", "</iframe>",
		"<textarea>", "</textarea>", "<title>", "</title>", "<foreignObject>",
		"</foreignObject>", "<mtext>", "</mtext>", "<annotation-xml>", "</annotation-xml>",
		"<p>", "</p>", "<br>", "<img>", "<div/>", "<path/>", "<span>", "</span>",
		"<form>", "</form>", "<table>", "</table>", "<mglyph>", "<font>", "</font>",
		`<font color="red">`, "<noscript>", "</noscript>", "<li>", "</li>",
		"<![CDATA[x]]>", "<![CDATA[ <div> ]]>", "<style>", "</style>",
		"<select>", "</select>", "<option>", "</option>", "<button>",
		"</button>", "<h2>", "</h2>", "<h3>", "</h3>", "<dd>", "</dd>",
		"<image>", "<keygen>", "<frameset>", "</frameset>", "<frame>",
		"<head>", "</head>", "<template>", "</template>", "<ul>", "</ul>",
	}
	fails := 0
	for _, seed := range []int64{11, 42} {
		rng := rand.New(rand.NewSource(seed))
		for c := 0; c < 5000 && fails < 12; c++ {
			n := 2 + rng.Intn(8)
			var sb strings.Builder
			for i := 0; i < n; i++ {
				sb.WriteString(tokens[rng.Intn(len(tokens))])
			}
			before := sb.String()
			for _, p := range probes {
				lintSrc := before + p.lintSrc
				oracleSrc := before + p.oracleSrc
				// For an execution probe, only the scripting-on reading can
				// make the placement worth reporting: a javascript: URL that
				// exists only with scripting disabled never runs, and the
				// linter deliberately says nothing about it.
				live := exists(oracleSrc, p, true)
				if !live && p.scriptingOff {
					live = exists(oracleSrc, p, false)
				}
				warned := lintSaysAny(lintSrc, p.msg)
				// A value that lands in a program body draws the language
				// message instead of the probe's own; same excusal as the
				// fixed sweeps.
				excused := lintSaysAny(lintSrc, "which is foreign content, where the parser decodes entities") ||
					lintSaysAny(lintSrc, "sits inside a <script>") || lintSaysAny(lintSrc, "sits inside a <style>") ||
					lintSaysAny(lintSrc, "JavaScript") || lintSaysAny(lintSrc, "CSS") ||
					lintSaysAny(lintSrc, "whose body is markup only when scripting is disabled")
				switch {
				case warned && !live:
					fails++
					t.Errorf("FALSE POSITIVE (%s): no browser makes this live, yet %q warns %q",
						p.name, lintSrc, lintWarnings(lintSrc))
				case live && !warned && !excused:
					fails++
					t.Errorf("MISSED (%s): live in a browser, and %q said %q",
						p.name, lintSrc, lintWarnings(lintSrc))
				}
			}
		}
	}
}
