package template_filters

import (
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
)

// Issue #92: what sanitize_under_auth removes, and what it must keep.
func TestSanitizeUserHTML_RemovesScript(t *testing.T) {
	cases := map[string]string{
		"event handler":         `<img src=x onerror="alert(1)">`,
		"script element":        `<script>alert(1)</script>`,
		"alpine x-init":         `<div x-data x-init="alert(1)"></div>`,
		"alpine @ shorthand":    `<button @click="alert(1)">b</button><span @click="alert(1)">s</span>`,
		"alpine : shorthand":    `<span :class="alert(1)">s</span>`,
		"alpine x-on":           `<span x-on:click="alert(1)">s</span>`,
		"javascript href":       `<a href="javascript:alert(1)">x</a>`,
		"mixed-case scheme":     `<a href="JaVaScRiPt:alert(1)">x</a>`,
		"tab inside scheme":     "<a href=\"java\tscript:alert(1)\">x</a>",
		"entity-encoded scheme": `<a href="java&#x73;cript:alert(1)">x</a>`,
		"leading space scheme":  `<a href=" javascript:alert(1)">x</a>`,
		"data html link":        `<a href="data:text/html,<script>alert(1)</script>">x</a>`,
		"vbscript":              `<a href="vbscript:alert(1)">x</a>`,
		"iframe":                `<iframe src="javascript:alert(1)"></iframe>`,
		"svg script":            `<svg><script>alert(1)</script></svg>`,
		"svg onload":            `<svg onload="alert(1)"></svg>`,
		"math mxss":             `<math><mtext><table><mglyph><style><img src=x onerror=alert(1)>`,
		"noscript mxss":         `<noscript><p title="</noscript><img src=x onerror=alert(1)>">`,
		"style element":         `<style>*{background:url(javascript:alert(1))}</style>`,
		"style attribute":       `<p style="background:url(javascript:alert(1))">p</p>`,
		"object":                `<object data="javascript:alert(1)"></object>`,
		"form action":           `<form action="javascript:alert(1)"><input type=submit></form>`,
		"meta refresh":          `<meta http-equiv="refresh" content="0;url=javascript:alert(1)">`,
		"template":              `<template><img src=x onerror=alert(1)></template>`,
		"srcdoc":                `<iframe srcdoc="<script>alert(1)</script>"></iframe>`,
		"unknown element attrs": `<custom-el onclick="alert(1)" x-init="alert(1)">text</custom-el>`,
		"formaction on unknown": `<x formaction="javascript:alert(1)">t</x>`,
		"img data svg":          `<img src="data:image/svg+xml,<svg onload=alert(1)>">`,
		"comment":               `<!-- <img src=x onerror=alert(1)> -->`,
		"unquoted handler":      `<img src=x onerror=alert(1)//>`,
		"broken out of attr":    `<img alt="a" onerror="alert(1)" src=x>`,
		"details ontoggle":      `<details open ontoggle="alert(1)">d</details>`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			out := SanitizeUserHTML(in)
			lower := strings.ToLower(out)
			for _, bad := range []string{"<script", "onerror", "onload", "onclick", "ontoggle", "x-init", "x-on", "@click", ":class",
				"javascript:", "vbscript:", "data:text", "<iframe", "<svg", "<math", "<style", "<object", "<form", "<meta", "<template", "srcdoc", "formaction", "style=", "<!--"} {
				if strings.Contains(lower, bad) {
					t.Errorf("%q kept %q: %s", in, bad, out)
				}
			}
		})
	}
}

func TestSanitizeUserHTML_KeepsOrdinaryMarkup(t *testing.T) {
	cases := map[string]string{
		`<p><strong>b</strong> <em>i</em> <a href="https://e.com/?a=1&amp;b=2">l</a></p>`: `<p><strong>b</strong> <em>i</em> <a href="https://e.com/?a=1&amp;b=2">l</a></p>`,
		// x/net/html hands an <a>'s attributes back in reverse order.
		`<a href="/group?id=3" class="mention-badge mention-group">G</a>`: `<a class="mention-badge mention-group" href="/group?id=3">G</a>`,
		`<a href="mailto:a@b.c">m</a>`:                                    `<a href="mailto:a@b.c">m</a>`,
		`<img src="data:image/png;base64,AAAA" alt="x">`:                  `<img src="data:image/png;base64,AAAA" alt="x">`,
		`<h2 id="intro">Intro</h2>`:                                       `<h2 id="intro">Intro</h2>`,
		`<th style="text-align:center">c</th>`:                            `c`, // a lone cell outside a table is dropped by the parser
		`<table><tr><td style="text-align:right">1</td></tr></table>`:     `<table><tbody><tr><td style="text-align:right">1</td></tr></tbody></table>`,
		`<pre><code class="language-go">x := "1"
</code></pre>`: `<pre><code class="language-go">x := &quot;1&quot;
</code></pre>`,
		"<pre>\n\nlead</pre>": "<pre>\n\nlead</pre>",
		`<ul><li><input checked="" disabled="" type="checkbox"> t</li></ul>`: `<ul><li><input checked="" disabled="" type="checkbox"> t</li></ul>`,
		`<details><summary>s</summary>body</details>`:                        `<details><summary>s</summary>body</details>`,
		// Classes the page styles itself with are dropped: no overlaying the page.
		`<a href="/x" class="fixed inset-0 z-50 mention-badge">x</a>`: `<a class="mention-badge" href="/x">x</a>`,
		`<div class="fixed inset-0">x</div>`:                          `<div>x</div>`,
		// An unknown element is unwrapped, its text kept.
		`<font color="red">t</font>`: `t`,
	}
	for in, want := range cases {
		if got := SanitizeUserHTML(in); got != want {
			t.Errorf("SanitizeUserHTML(%q)\n got: %q\nwant: %q", in, got, want)
		}
	}
}

// Shortcodes expand after the sanitizer. In text they must parse as before
// (single-quoted attributes may hold brackets), and in an attribute they must
// not parse at all, or their output would land in an href after the check.
func TestSanitizeUserHTML_Shortcodes(t *testing.T) {
	// As goldmark writes it: '"' as &quot;, "'" literal.
	text := `<p>[pills options='[&quot;a&quot;,&quot;b&quot;]'] [meta path=&quot;x&quot;]</p>`
	if got := SanitizeUserHTML(text); got != text {
		t.Errorf("shortcodes in text changed:\n got: %q\nwant: %q", got, text)
	}
	got := SanitizeUserHTML(`<a href="[property path=Name]" title="[meta path=x]">l</a>`)
	if strings.Contains(got, "[") || strings.Contains(got, "]") {
		t.Errorf("shortcode brackets left in an attribute: %s", got)
	}
	if !strings.Contains(got, `href="&#91;property path=Name&#93;"`) {
		t.Errorf("bracketed attribute value not kept as references: %s", got)
	}
}

// The filter sanitizes unless its parameter is an explicit false, so a page
// context that does not set authEnabled fails closed.
func TestSanitizeUnderAuthFilter_Parameter(t *testing.T) {
	const raw = `<img src=x onerror="alert(1)">`
	for name, param := range map[string]*pongo2.Value{"true": pongo2.AsValue(true), "missing": pongo2.AsValue(nil)} {
		out, _ := sanitizeUnderAuthFilter(pongo2.AsValue(raw), param)
		if strings.Contains(out.String(), "onerror") {
			t.Errorf("param %s: not sanitized: %s", name, out.String())
		}
	}
	out, _ := sanitizeUnderAuthFilter(pongo2.AsValue(raw), pongo2.AsValue(false))
	if out.String() != raw {
		t.Errorf("auth off must leave the markup alone, got %s", out.String())
	}
}
