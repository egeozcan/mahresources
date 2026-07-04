package shortcodes

import (
	"fmt"
	"html"
)

// Failure markers make a template problem diagnosable from the rendered page
// instead of leaking the literal `[shortcode …]` source. Two tiers:
//
//   - shortcodeErrorMarker — an author-facing inline span for actionable
//     failures (a plugin renderer error, a malformed shortcode). It reuses the
//     mrql-error visual language but stays inline so it can sit mid-sentence.
//     The trust model is a private tool, so the full error is exposed in the
//     title attribute.
//   - shortcodeComment — an HTML comment for structural stops where visible
//     output would be noise (recursion depth cap, an executor/renderer that a
//     context deliberately does not wire). Authors inspecting the page source
//     still see why expansion stopped.

// shortcodeErrorMarker renders a small inline warning span. label is the short
// visible text (e.g. the plugin shortcode name); detail is the longer
// explanation surfaced via the title attribute. Both are HTML-escaped.
func shortcodeErrorMarker(label, detail string) string {
	return fmt.Sprintf(
		`<span class="shortcode-error text-red-700 font-mono" title="%s">⚠ %s</span>`,
		html.EscapeString(detail),
		html.EscapeString(label),
	)
}

// shortcodeComment renders an HTML comment marker. msg must not contain "--"
// (callers pass fixed strings), so no escaping of comment terminators is needed.
func shortcodeComment(msg string) string {
	return "<!-- mr:" + msg + " -->"
}
