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
		"<!-- c -->", "<!-- c --!>", `<div weird">`, "</div x=y>", "<!doctype html>",
		"<select>", "</select>", "<option>", "</option>", "<button>",
		"</button>", "<h2>", "</h2>", "<h3>", "</h3>", "<dd>", "</dd>",
		"<image>", "<keygen>", "<frameset>", "</frameset>", "<frame>",
		"<head>", "</head>", "<template>", "</template>", "<ul>", "</ul>",
		"<noframes>", "</noframes>", "<body>", "<html>", "<annotation-xml>",
		"<input>", `<input type="hidden">`, "<hr>", "<pre>", "</pre>",
		"<td>", "</td>", "<tr>", "</tr>", "<marquee>", "</marquee>",
		"<object>", "</object>", "<optgroup>", "<dt>", "</dt>",
	}
	fails := 0
	for _, seed := range []int64{11, 42} {
		rng := rand.New(rand.NewSource(seed))
		for c := 0; c < 5000 && fails < 12; c++ {
			n := 2 + rng.Intn(10)
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
				// Only the program-landing messages excuse a missing probe
				// warning; a broad predicate here once let an unrelated CSS
				// warning mask a real dual-mode miss.
				excused := lintSaysAny(lintSrc, "which is foreign content, where the parser decodes entities") ||
					lintSaysAny(lintSrc, "sits inside a <script>") || lintSaysAny(lintSrc, "sits inside a <style>") ||
					lintSaysAny(lintSrc, "interpolated into a tag or attribute NAME")
				// When the probe is live, its occurrence is a real QUOTED
				// attribute value (srcdoc, href), so a warning that it is an
				// unquoted value or an interpolated NAME is a false positive —
				// the class the lexical over-read bugs produced.
				extraFalse := ""
				if live && p.attr != "" {
					if lintSaysAny(lintSrc, "sits in an unquoted attribute value") {
						extraFalse = "false unquoted"
					} else if lintSaysAny(lintSrc, "interpolated into a tag or attribute NAME") {
						extraFalse = "false NAME"
					}
				}
				switch {
				case warned && !live:
					fails++
					t.Errorf("FALSE POSITIVE (%s): no browser makes this live, yet %q warns %q",
						p.name, lintSrc, lintWarnings(lintSrc))
				case live && !warned && !excused:
					fails++
					t.Errorf("MISSED (%s): live in a browser, and %q said %q",
						p.name, lintSrc, lintWarnings(lintSrc))
				case extraFalse != "":
					fails++
					t.Errorf("EXTRA FALSE (%s, %s): the value is a live quoted attribute, yet %q warns %q",
						p.name, extraFalse, lintSrc, lintWarnings(lintSrc))
				}
			}
		}
	}
}

// TestLintUnquotedSiblingDifferential is the independent oracle for the
// unquoted-sibling rule (round 19). It substitutes a space-bearing value — the
// worst case of an escaped [property] — into the SAME interpolation point with
// a marker attribute the test alone controls, parses in BOTH scripting modes,
// and asserts the linter says "unquoted attribute value" exactly when a browser
// keeps the marker as a live sibling. It is independent of the linter's own
// probe: a bug in unquotedSiblingInjectors shows up as a mismatch here.
func TestLintUnquotedSiblingDifferential(t *testing.T) {
	markerLive := func(src string, scripting bool) bool {
		opts := []html.ParseOption{}
		if !scripting {
			opts = append(opts, html.ParseOptionEnableScripting(false))
		}
		doc, err := html.ParseWithOptions(strings.NewReader(src), opts...)
		if err != nil {
			return false
		}
		found := false
		var walk func(*html.Node)
		walk = func(n *html.Node) {
			if n.Type == html.ElementNode {
				for _, a := range n.Attr {
					if a.Namespace == "" && a.Key == "zqmarkerx" {
						found = true
					}
				}
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
		}
		walk(doc)
		return found
	}

	tokens := []string{
		"<svg>", "</svg>", "<math>", "</math>", "<div>", "</div>", "<b>", "</b>",
		"<a>", "</a>", "<iframe>", "</iframe>", "<textarea>", "</textarea>",
		"<title>", "</title>", "<foreignObject>", "</foreignObject>", "<p>", "</p>",
		"<form>", "</form>", "<table>", "</table>", "<noscript>", "</noscript>",
		"<style>", "</style>", "<select>", "</select>", "<option>", "<button>",
		"<frameset>", "</frameset>", "<head>", "</head>", "<template>", "</template>",
		"<noframes>", "</noframes>", "<body>", "<html>", "<td>", "<tr>", "<optgroup>",
		"<script>", "</script>", "<span>", "</span>", "<font>", "</font>", "<h1>",
	}
	shapes := []struct{ lint, oracle string }{
		{`<div class=[property path='Name']>`, `<div class=zqx zqmarkerx>`},
		// Occurrences with LITERAL text glued after them (round 21): the probe
		// must delimit its own name so the trailing text does not corrupt it.
		{`<div class=[property path='Name']SAFE>`, `<div class=zqx zqmarkerx SAFE>`},
		{`<div title=fixed title=[property path='Name']7>`, `<div title=fixed title=zqx zqmarkerx 7>`},
		{`<div class=[property path='Name']=v>`, `<div class=zqx zqmarkerx =v>`},
		{`<body class=[property path='Name']>`, `<body class=zqx zqmarkerx>`},
		{`<html class=[property path='Name']>`, `<html class=zqx zqmarkerx>`},
		{`<div title="fixed" title=[property path='Name']>`, `<div title="fixed" title=zqx zqmarkerx>`},
		{`<body id=a><body id=[property path='Name']>`, `<body id=a><body id=zqx zqmarkerx>`},
		{`<input value=[property path='Name']>`, `<input value=zqx zqmarkerx>`},
		{`<tr foo=[property path='Name']>`, `<tr foo=zqx zqmarkerx>`},
		{`<td foo=[property path='Name']>`, `<td foo=zqx zqmarkerx>`},
	}
	warns := func(src, sub string) bool { return lintSaysAny(src, sub) }
	mism := 0
	for _, seed := range []int64{1, 7, 19, 55} {
		rng := rand.New(rand.NewSource(seed))
		for c := 0; c < 3000 && mism < 15; c++ {
			n := rng.Intn(9)
			var sb strings.Builder
			for i := 0; i < n; i++ {
				sb.WriteString(tokens[rng.Intn(len(tokens))])
			}
			before := sb.String()
			for _, sh := range shapes {
				lintSrc := before + sh.lint
				oracleSrc := before + sh.oracle
				live := markerLive(oracleSrc, true) || markerLive(oracleSrc, false)
				warnedUnquoted := warns(lintSrc, "sits in an unquoted attribute value")
				warnedName := warns(lintSrc, "interpolated into a tag or attribute NAME")
				switch {
				case warnedUnquoted && !live:
					mism++
					t.Errorf("FALSE POSITIVE: marker not live yet unquoted-warned: %q -> %q", lintSrc, lintWarnings(lintSrc))
				case live && !warnedUnquoted && !warnedName:
					mism++
					t.Errorf("MISSED: marker live yet no unquoted/NAME warning: %q -> %q", lintSrc, lintWarnings(lintSrc))
				}
			}
		}
	}
}

// TestLintRawBreakoutDifferential is the independent oracle for the raw= rules
// (round 22). For each context it substitutes a CONTEXT-SPECIFIC real breakout
// (a single-quote closer for a single-quoted value, a <frame> for a frameset,
// …) — deliberately NOT the linter's own combined probe string — parses both
// scripting modes, and asserts the linter warns raw= exactly when that breakout
// reaches a live element. A probe that is incomplete for some context (as the
// round-21 probe was for single quotes) shows up here as a mismatch.
func TestLintRawBreakoutDifferential(t *testing.T) {
	marker := "zqrawimg"
	live := func(src string) bool {
		for _, scripting := range []bool{true, false} {
			opts := []html.ParseOption{}
			if !scripting {
				opts = append(opts, html.ParseOptionEnableScripting(false))
			}
			doc, err := html.ParseWithOptions(strings.NewReader(src), opts...)
			if err != nil {
				continue
			}
			found := false
			var walk func(*html.Node)
			walk = func(n *html.Node) {
				if n.Type == html.ElementNode {
					if n.Data == marker {
						found = true
					}
					for _, a := range n.Attr {
						if a.Key == marker {
							found = true
						}
					}
				}
				for c := n.FirstChild; c != nil; c = c.NextSibling {
					walk(c)
				}
			}
			walk(doc)
			if found {
				return true
			}
		}
		return false
	}
	warnsRaw := func(src string) bool {
		// The raw= hazard is reported by whichever message fits where the value
		// landed: "becomes real elements" (text/dropped), "close the attribute"
		// (an attribute value), or "can close the <script>/<style>" (a raw-text
		// program body). Any of them means the raw= danger was caught.
		return lintSaysAny(src, "becomes real elements") ||
			lintSaysAny(src, "close the attribute") ||
			lintSaysAny(src, "can close the <")
	}

	tokens := []string{
		"<div>", "</div>", "<svg>", "</svg>", "<desc>", "<template>", "</template>",
		"<frameset>", "</frameset>", "<textarea>", "</textarea>", "<title>", "</title>",
		"<noscript>", "</noscript>", "<p>", "</p>", "<b>", "<style>", "</style>",
		"<math>", "<foreignObject>", "<span>", "</span>", "<table>", "<a>", "</a>",
	}
	// A real raw= attacker writes an unescaped value that closes whatever
	// contains it. This is that value — raw-text closers, both quote closers, a
	// tag closer, then a marker element and a frameset frame — substituted into
	// each context. It is the worst case a value could actually take, so the
	// linter (which must warn iff SOME value is live) should agree with it.
	// The oracle's OWN copy of the worst-case raw value: it must close whatever
	// the occurrence sits in and then try every way bytes become a live node.
	// A single value cannot both close its tag (for an element) and keep it open
	// (to add an attribute to a <plaintext> start tag), so there are two, tried
	// in turn — the same shape the probe uses, kept as a SEPARATE copy so a
	// vector dropped from the probe still shows here as a mismatch.
	attacks := []string{
		`'"></title></textarea></style></script></xmp></iframe></noembed></noframes></noscript><` +
			marker + `></` + marker + `><frame ` + marker + ` src=x><html ` + marker + `><body ` + marker + `>`,
		`'" ` + marker + `=x `,
	}
	shapes := []struct{ lint, oracle string }{
		{`<x>[property path='Name' raw='true']</x>`, `<x>%s</x>`},
		{`<x title="[property path='Name' raw='true']">`, `<x title="%s">`},
		{`<x title='[property path="Name" raw="true"]'>`, `<x title='%s'>`},
		{`<x title=fixed title='[property path="Name" raw="true"]'>`, `<x title=fixed title='%s'>`},
		{`<x>y</x data-z="[property path='Name' raw='true']">`, `<x>y</x data-z="%s">`},
		{`[property path='Name' raw='true']`, `%s`},
	}
	fails := 0
	for _, seed := range []int64{3, 14, 15, 92} {
		rng := rand.New(rand.NewSource(seed))
		for c := 0; c < 1500 && fails < 12; c++ {
			n := rng.Intn(7)
			var sb strings.Builder
			for i := 0; i < n; i++ {
				sb.WriteString(tokens[rng.Intn(len(tokens))])
			}
			before := sb.String()
			for _, sh := range shapes {
				lintSrc := before + sh.lint
				isLive := false
				for _, a := range attacks {
					if live(before + strings.Replace(sh.oracle, "%s", a, 1)) {
						isLive = true
						break
					}
				}
				warned := warnsRaw(lintSrc)
				switch {
				case warned && !isLive:
					fails++
					t.Errorf("FALSE POSITIVE: raw= warned but no attack is live: %q -> %q", lintSrc, lintWarnings(lintSrc))
				case isLive && !warned:
					fails++
					t.Errorf("MISSED: a raw= attack is live yet no warning: %q -> %q", lintSrc, lintWarnings(lintSrc))
				}
			}
		}
	}
}
