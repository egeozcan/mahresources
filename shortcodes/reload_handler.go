package shortcodes

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"
)

// reloadElementTag opens the custom element a [reload] button renders into.
// ContainsReloadButton looks for it in already-rendered slot HTML, so the two
// must stay in sync.
const reloadElementTag = "<reload-shortcode"

// defaultReloadLabel names an icon-only reload button for assistive technology.
// A button whose whole face is a decorative glyph has no text to derive an
// accessible name from, so one is always supplied.
const defaultReloadLabel = "Reload"

// reloadIconSVG is the default button face: a circular arrow. aria-hidden and
// focusable="false" keep it out of the accessibility tree (and out of the tab
// order in legacy Edge/IE), so the button's name comes from its aria-label.
const reloadIconSVG = `<svg class="reload-shortcode-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><path d="M20 11.5a8 8 0 1 1-2.34-5.66"/><polyline points="20.5 3.5 20.5 9.5 14.5 9.5"/></svg>`

// htmlCommentPattern and htmlTagPattern back the two checks below. They are
// heuristics, not a parser: they only have to tell "this button face is a
// graphic" from "this button face reads as words" from "this button face is not
// there at all", and they err toward treating content as textless, which adds an
// accessible name rather than withholding one.
var (
	htmlCommentPattern = regexp.MustCompile(`(?s)<!--.*?-->`)
	htmlTagPattern     = regexp.MustCompile(`<[^>]*>`)
)

// rendersAnything reports whether fragment puts anything on the page. A body that
// expands to only comments does not: an [mrql] with no executor, or a plugin
// shortcode with no renderer, leaves an HTML comment behind and nothing else.
func rendersAnything(fragment string) bool {
	return strings.TrimSpace(htmlCommentPattern.ReplaceAllString(fragment, "")) != ""
}

// hasVisibleText reports whether fragment renders any text of its own, as
// opposed to being nothing but markup (an <svg> or an <img>, say).
func hasVisibleText(fragment string) bool {
	stripped := htmlTagPattern.ReplaceAllString(htmlCommentPattern.ReplaceAllString(fragment, ""), "")
	return strings.TrimSpace(stripped) != ""
}

// reloadFaceKey marks the render of a [reload] button's face.
type reloadFaceKey struct{}

// markReloadFace returns a context that tells nested handlers they are rendering
// inside a <button>.
func markReloadFace(reqCtx context.Context) context.Context {
	return context.WithValue(reqCtx, reloadFaceKey{}, true)
}

// withinReloadFace reports whether this render is producing the face of a reload
// button. A <button> takes phrasing content only, so [lazy] and [details] — both
// of which emit a block — refuse to expand here. That also closes the one way a
// second button could still reach the inside of this one: a deferred body is
// sealed rather than rendered, so a [reload] inside it is invisible to the checks
// above and would only appear when the deferred fetch lands.
func withinReloadFace(reqCtx context.Context) bool {
	inFace, _ := reqCtx.Value(reloadFaceKey{}).(bool)
	return inFace
}

// containsReloadShortcode reports whether source holds a [reload] opener at any
// depth, including inside a block whose body this render will not expand. It
// reads the same token stream the parser and the linter do, so "[reload" written
// as prose is not mistaken for a shortcode.
func containsReloadShortcode(source string) bool {
	if !strings.Contains(source, "[reload") {
		return false
	}
	for _, tk := range matchTokens(source) {
		if tk.name == "reload" && !tk.closing {
			return true
		}
	}
	return false
}

// ContainsReloadButton reports whether rendered slot HTML holds a [reload]
// button that will actually be shown. The process_shortcodes tag uses it to
// decide whether the slot it just rendered needs a reloadable region wrapper.
//
// During a page render a [reload] inside a deferred [lazy]/[details] body is not
// expanded at all (that body is captured into a token instead), so a button in
// this output is one with no deferred ancestor on the page. Markup an author
// commented out still expands, though, so a button that only exists inside an
// HTML comment must not mint a region nothing can use. The comment strip runs
// only after the cheap scan has already matched.
func ContainsReloadButton(rendered string) bool {
	if !strings.Contains(rendered, reloadElementTag) {
		return false
	}
	return strings.Contains(htmlCommentPattern.ReplaceAllString(rendered, ""), reloadElementTag)
}

// IsDeferrableEntity reports whether ctx describes an entity the deferred-render
// endpoint can reload by (type, id). Exported for the process_shortcodes tag,
// which must not mint a region token for a carrier context the endpoint would
// refuse to load.
func IsDeferrableEntity(ctx MetaShortcodeContext) bool {
	return isDeferrableEntity(ctx)
}

// RenderReloadShortcode expands [reload] into a button that re-renders the
// nearest enclosing deferred area.
//
// The button is inert on the server: it carries no token of its own and never
// names its target. The frontend resolves that at click time by walking up the
// DOM, which is what makes "the most immediate area" well defined even when the
// button arrives inside content that was itself fetched on demand. It reloads
// the closest ancestor [lazy]/[details] block; failing that, the custom-content
// region the button sits in; failing that, the page.
//
// The inline form renders the default circular-arrow glyph. The block form puts
// its (shortcode-expanded) inner content inside the button instead, so authors
// can label it. Keep that content to phrasing content: a <button> may not
// contain interactive or block-level descendants.
func RenderReloadShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	label := strings.TrimSpace(sc.Attrs["label"])

	var inner string
	if sc.IsBlock {
		// A <button> may not contain another button. Emitting one anyway would
		// leave the browser to repair the markup however it likes, and whatever it
		// produced would be two nested controls in the accessibility tree. Refuse
		// the whole button instead: an author-facing marker is diagnosable, invalid
		// interactive nesting is not.
		//
		// The source is checked before it is expanded, because expanding it can
		// hide the problem rather than reveal it: a [lazy]/[details] inside this
		// body is sealed into a token instead of being rendered, so a [reload]
		// within it would be invisible now and arrive inside the button later,
		// when the deferred fetch lands. This mirrors the linter, which reads the
		// same tokens from the same source.
		if containsReloadShortcode(sc.InnerContent) {
			return shortcodeErrorMarker("reload", "[reload] cannot contain another [reload]")
		}
		inner = strings.TrimSpace(processWithDepth(markReloadFace(reqCtx), sc.InnerContent, ctx, renderer, executor, depth+1))
		// Expansion can also introduce one that the source never mentioned, by way
		// of a [partial] or an [each] body.
		if strings.Contains(inner, reloadElementTag) {
			return shortcodeErrorMarker("reload", "[reload] cannot contain another [reload]")
		}
		// A body that renders nothing would leave a zero-size button: present in
		// the tab order, impossible to see or point at. Fall back to the glyph.
		if !rendersAnything(inner) {
			inner = ""
		}
	}

	classes := "reload-shortcode-button"

	iconOnly := inner == ""
	if iconOnly {
		inner = reloadIconSVG
		classes += " reload-shortcode-button--icon"
	}

	// A button whose face carries no text cannot derive an accessible name from
	// its content. That covers the default glyph and an author's own <svg>/<img>
	// body alike, so both get the default name and a pointer tooltip rather than
	// shipping an unnamed control.
	textless := iconOnly || !hasVisibleText(inner)
	if textless && label == "" {
		label = defaultReloadLabel
	}

	var nameAttrs string
	if label != "" {
		// With visible text present, label= overrides the name the text would
		// give. Authors are told to keep that text inside the label (WCAG 2.5.3
		// Label in Name); a tooltip is only added where there is no visible text
		// to duplicate.
		nameAttrs = fmt.Sprintf(` aria-label="%s"`, html.EscapeString(label))
		if textless {
			nameAttrs += fmt.Sprintf(` title="%s"`, html.EscapeString(label))
		}
	}

	return fmt.Sprintf(
		`<reload-shortcode><button type="button" class="%s"%s>%s</button></reload-shortcode>`,
		classes, nameAttrs, inner,
	)
}
