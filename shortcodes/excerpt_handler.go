package shortcodes

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"html"
	"reflect"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"

	"mahresources/mentions"
)

const (
	// excerptSourceCap is the most bytes of the source text an excerpt reads.
	// The longest excerpt (excerptMaxWords × excerptRunesPerWord runes) is far
	// shorter, so the cap never shortens a normal excerpt; it bounds the work
	// a pathological Description can cause.
	excerptSourceCap      = 16 << 10
	excerptMaxWords       = 200
	excerptDefaultWords   = 30
	excerptRunesPerWord   = 12
	excerptScratchPerByte = 12
)

// A mention sentinel wraps a per-conversion nonce and the mention's decimal
// index in private-use runes. Markdown and HTML leave that spelling alone.
const (
	excerptMentionOpen  = '\uE000'
	excerptMentionClose = '\uE001'
)

// excerptMarkdown parses the source the way the markdown2 filter does
// (server/template_handlers/template_filters/markdown_filter.go): GFM, with raw
// HTML passed through. The filter's other two options, WithAutoHeadingID and
// WithHardWraps, change only attributes and <br>, which become nothing and a
// space here either way. Raw HTML is safe to pass through because none of this
// HTML is emitted: only its text is. Keep the two in step;
// markdown_excerpt_parity_test.go in template_filters fails when they disagree
// on visible text.
var excerptMarkdown = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(gmhtml.WithUnsafe()),
)

type publicSharePageKey struct{}

// WithPublicSharePage marks a render for the public share server, which serves
// only /s/<token> pages and none of the entity detail pages. [excerpt] leaves
// out its more= link there, since the link would point at a page that does not
// exist on that server.
func WithPublicSharePage(ctx context.Context) context.Context {
	return context.WithValue(ctx, publicSharePageKey{}, true)
}

func onPublicSharePage(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	on, _ := ctx.Value(publicSharePageKey{}).(bool)
	return on
}

// RenderExcerptShortcode expands [excerpt] into a short plain-text preview of
// the entity's Description, another string field (field=) or a Meta string
// (path=): Markdown, HTML, mentions, template comments and shortcodes are
// stripped, the text is cut to words= words, and the result is HTML-escaped.
// It reads only the current entity and its Meta and never loads data, so it
// shows nothing the surface that rendered the entity has not already loaded in
// the viewer's scope.
//
// Without more= the output is a bare value, usable in inert quoted attributes.
// With more= and a truncated text it appends a link to the entity's detail
// page, except inside [reload] button faces, hover cards and public share pages.
func RenderExcerptShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string {
	path := sc.Attrs["path"]
	field := sc.Attrs["field"]
	if path != "" && field != "" {
		return shortcodeErrorMarker("excerpt", "set path or field, not both")
	}

	source, ok := excerptSource(path, field, ctx)
	text, truncated := "", false
	if ok {
		text, truncated, ok = excerptText(meterFrom(reqCtx), source, parseExcerptWords(sc.Attrs["words"]))
		if !ok {
			return ""
		}
	}
	if text == "" && !truncated {
		return html.EscapeString(sc.Attrs["default"])
	}

	out := html.EscapeString(text)
	if !truncated {
		return out
	}
	out += "…"

	more := strings.TrimSpace(sc.Attrs["more"])
	if more == "" || withinReloadFace(reqCtx) || withinTooltip(reqCtx) || onPublicSharePage(reqCtx) {
		return out
	}
	url, ok := resolveLinkURL("self", ctx)
	if !ok || ctx.EntityID == 0 {
		return out
	}
	return fmt.Sprintf(`%s <a href="%s" class="underline" aria-label="%s">%s</a>`,
		out, html.EscapeString(url), html.EscapeString(more+": "+excerptLinkSubject(ctx)), html.EscapeString(more))
}

// excerptSource resolves the text an [excerpt] previews. ok is false when there
// is none: a missing or non-string Meta value, or a field that is dotted,
// unknown, unexported, a withheld credential or not a string. field= names one
// field of this entity, never a preloaded association: a related entity is
// previewed through [using], which loads it in the viewer's scope.
func excerptSource(path, field string, ctx MetaShortcodeContext) (string, bool) {
	if path != "" {
		s, ok := ctx.rawValueAtPath(path).(string)
		return s, ok
	}
	if field == "" {
		field = "Description"
	}
	if strings.Contains(field, ".") || ctx.Entity == nil {
		return "", false
	}
	v, ok := traversePropertyPath(ctx.Entity, field, ctx.credentialsVisible)
	if !ok {
		return "", false
	}
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return "", false
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.String {
		return "", false
	}
	return v.String(), true
}

// excerptLinkSubject names the entity in the more= link's accessible name, so
// the "Read more" links of a list are told apart (WCAG 2.4.4).
func excerptLinkSubject(ctx MetaShortcodeContext) string {
	if v, ok := traversePropertyPath(ctx.Entity, "Name", ctx.credentialsVisible); ok && v.Kind() == reflect.String {
		if name := strings.TrimSpace(v.String()); name != "" {
			return name
		}
	}
	return ctx.EntityType + " " + strconv.FormatUint(uint64(ctx.EntityID), 10)
}

// parseExcerptWords reads words=: a whole number from 1 to excerptMaxWords. Not
// a whole number, or below 1, falls back to excerptDefaultWords; above the
// maximum is clamped to it.
func parseExcerptWords(v string) int {
	n, err := strconv.Atoi(strings.TrimSpace(v))
	if err != nil || n < 1 {
		return excerptDefaultWords
	}
	return min(n, excerptMaxWords)
}

// excerptText turns src into at most words words of single-line plain text.
// truncated reports that text is shorter than the source's visible text (a
// source past excerptSourceCap always counts as truncated). ok is false when
// the render's scratch budget refused the work.
func excerptText(m *renderMeter, src string, words int) (text string, truncated, ok bool) {
	capped := false
	if len(src) > excerptSourceCap {
		cut := excerptSourceCap
		for cut > 0 && !utf8.RuneStart(src[cut]) {
			cut--
		}
		src, capped = src[:cut], true
	}
	// Estimate the scratch used by the intermediate strings, rendered HTML
	// and Goldmark AST. This is a heuristic, not a byte ceiling: quote-heavy
	// HTML, mention markers and node-heavy Markdown have different ratios.
	// The source cap and single-use mention restoration bound the work.
	if !m.chargeScratch(excerptScratchPerByte * int64(len(src))) {
		return "", false, false
	}

	src = stripTemplateComments(src)
	src = excerptDeleteShortcodes(src)
	src, nonce, names := excerptMentionSentinels(src)

	var rendered bytes.Buffer
	if err := excerptMarkdown.Convert([]byte(src), &rendered); err != nil {
		return "", false, true
	}
	plain := excerptRestoreMentions(excerptVisibleText(rendered.Bytes()), nonce, names)

	kept := strings.Fields(plain)
	if len(kept) > words {
		kept, truncated = kept[:words], true
	}
	text = strings.Join(kept, " ")
	if maxRunes := words * excerptRunesPerWord; utf8.RuneCountInString(text) > maxRunes {
		text, truncated = excerptCutRunes(text, maxRunes), true
	}
	if truncated || (capped && text != "") {
		// Punctuation before the ellipsis reads as a stutter ("a, b,…"), but
		// a text that is nothing else keeps it rather than becoming empty.
		if trimmed := strings.TrimRight(text, " ,;:.-–—"); trimmed != "" {
			text = trimmed
		}
		truncated = true
	}
	return text, truncated, true
}

// excerptCutRunes cuts to at most n runes without splitting a base from its
// combining marks, variation selectors, emoji modifiers or ZWJ sequence.
func excerptCutRunes(s string, n int) string {
	i := 0
	for pos, next := range s {
		if i == n {
			for pos > 0 {
				prev, size := utf8.DecodeLastRuneInString(s[:pos])
				if prev != '\u200d' && !excerptJoinsPreviousRune(next) {
					break
				}
				pos, next = pos-size, prev
			}
			return strings.TrimRight(s[:pos], " ")
		}
		i++
	}
	return s
}

func excerptJoinsPreviousRune(r rune) bool {
	return r == '\u200d' || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) ||
		(r >= '\U0001f3fb' && r <= '\U0001f3ff') // emoji skin-tone modifiers
}

// excerptDeleteShortcodes removes every shortcode from src without running it:
// a block goes with its body, and a closer with no opener goes too. It runs on
// the source before Markdown, so a shortcode inside a code span is removed as
// well. The render's meter is not used: its token and attribute caps would let
// a Description's text end a budgeted render, and excerptSourceCap already
// bounds the parse.
func excerptDeleteShortcodes(src string) string {
	spans := parseWithBlocksM(nil, excerptMaskMarkdownLabels(src))
	if len(spans) > 0 {
		var b strings.Builder
		b.Grow(len(src))
		last := 0
		for _, sc := range spans {
			b.WriteString(src[last:sc.Start])
			last = sc.End
		}
		b.WriteString(src[last:])
		src = b.String()
	}
	return closingTagPattern.ReplaceAllString(src, "")
}

// excerptMentionSentinels replaces each mention with its sentinel and returns
// the nonce and names needed to restore it. A nonce is needed even after
// removing literal sentinel runes: Markdown/HTML can decode numeric entities.
func excerptMentionSentinels(src string) (string, string, []string) {
	src = strings.Map(func(r rune) rune {
		if r == excerptMentionOpen || r == excerptMentionClose {
			return -1
		}
		return r
	}, src)
	var nonce string
	var names []string
	src = mentions.ReplaceAll(src, func(m mentions.Mention) string {
		if nonce == "" {
			nonce = rand.Text()
		}
		s := string(excerptMentionOpen) + nonce + strconv.Itoa(len(names)) + string(excerptMentionClose)
		names = append(names, m.Name)
		return s
	})
	return src, nonce, names
}

// excerptRestoreMentions puts names back only for exact markers emitted by
// this conversion. Unrecognised opener and closer runes are stripped.
func excerptRestoreMentions(text, nonce string, names []string) string {
	if !strings.ContainsRune(text, excerptMentionOpen) && !strings.ContainsRune(text, excerptMentionClose) {
		return text
	}
	// Each name is part of the capped source and can be used at most once.
	// Reserve that bounded expansion before writing any restored text.
	capacity := len(text)
	for _, name := range names {
		capacity += len(name)
	}
	var b strings.Builder
	b.Grow(capacity)
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		switch r {
		case excerptMentionOpen:
			if nonce == "" || !strings.HasPrefix(text[i+size:], nonce) {
				break
			}
			start := i + size + len(nonce)
			j := start
			for j < len(text) && text[j] >= '0' && text[j] <= '9' {
				j++
			}
			closer, closerSize := utf8.DecodeRuneInString(text[j:])
			if idx, err := strconv.Atoi(text[start:j]); err == nil && closer == excerptMentionClose &&
				idx < len(names) && (j-start == 1 || text[start] != '0') {
				b.WriteString(names[idx])
				names[idx] = ""
				i = j + closerSize
				continue
			}
		case excerptMentionClose:
		default:
			b.WriteString(text[i : i+size])
		}
		i += size
	}
	return b.String()
}

// excerptRawTextElements hold text that is not shown as text: scripts, styles,
// form values, the document title and fallback content.
var excerptRawTextElements = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true, "xmp": true,
	"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true,
}

// excerptBlockElements end a run of text: their start and end tags read as a
// space, so "<p>a</p><p>b</p>" is "a b" and not "ab".
var excerptBlockElements = map[string]bool{
	"p": true, "div": true, "br": true, "li": true, "tr": true, "td": true, "th": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"blockquote": true, "pre": true, "hr": true, "ul": true, "ol": true, "table": true,
	"dd": true, "dt": true, "section": true, "article": true,
}

// excerptVisibleText is the text of an HTML fragment as a reader sees it, with
// entities decoded: comments, tags, attributes (alt text included) and the
// contents of raw-text elements and inert templates are dropped.
func excerptVisibleText(fragment []byte) string {
	var b strings.Builder
	b.Grow(len(fragment))
	z := xhtml.NewTokenizer(bytes.NewReader(fragment))
	skipping := ""
	templateDepth := 0
	for {
		switch z.Next() {
		case xhtml.ErrorToken:
			return b.String()
		case xhtml.TextToken:
			if skipping == "" && templateDepth == 0 {
				b.Write(z.Text())
			}
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			name, _ := z.TagName()
			if skipping != "" {
				continue
			}
			if string(name) == "template" {
				templateDepth++
			} else if excerptRawTextElements[string(name)] {
				skipping = string(name)
			} else if templateDepth == 0 && excerptBlockElements[string(name)] {
				b.WriteByte(' ')
			}
		case xhtml.EndTagToken:
			name, _ := z.TagName()
			if skipping != "" {
				if string(name) == skipping {
					skipping = ""
				}
			} else if string(name) == "template" && templateDepth > 0 {
				templateDepth--
			} else if templateDepth == 0 && excerptBlockElements[string(name)] {
				b.WriteByte(' ')
			}
		}
	}
}

// lintExcerpt reports what the renderer would do differently from what an
// [excerpt] seems to ask for. otherContext are the bodies whose [excerpt] reads
// another entity's Meta, so the schema check skips them.
func lintExcerpt(tk token, opts LintOptions, otherContext, reloadSpans [][2]int, add func(start, end int, severity, msg string)) {
	path, field := tk.attrs["path"], tk.attrs["field"]
	if path != "" && field != "" {
		add(tk.start, tk.end, SeverityError, "[excerpt] takes path or field, not both")
	}
	if strings.Contains(field, ".") {
		add(tk.start, tk.end, SeverityWarning,
			"[excerpt] field= names one field of this entity; use [using] to preview a related entity")
	}
	if v, set := tk.attrs["words"]; set {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err != nil || n < 1 || n > excerptMaxWords {
			add(tk.start, tk.end, SeverityWarning,
				"[excerpt] words= must be a whole number from 1 to 200; it falls back to 30 (or 200 above it)")
		}
	}
	if path != "" && !withinSpans(tk.start, otherContext) {
		if msg := metaPathIssueFor("[excerpt]", "it renders the default", path, opts.metaProps); msg != "" {
			add(tk.start, tk.end, SeverityWarning, msg)
		}
	}
	if strings.TrimSpace(tk.attrs["more"]) != "" && insideSpans(tk.start, reloadSpans) {
		add(tk.start, tk.end, SeverityWarning,
			"[excerpt] more= inside a [reload] button face renders no link: a button cannot contain one")
	}
}
