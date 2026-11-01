package shortcodes

import (
	"strings"

	"golang.org/x/net/html"

	"mahresources/templatecomment"
)

// Template comments ([comment]...[/comment]) are removed before any shortcode
// is parsed; the rules live in templatecomment.
func stripTemplateComments(input string) string {
	return templatecomment.Strip(input)
}

// StripTemplateComments removes every template comment from text that is shown
// without going through Process, such as a Description's plain-text preview.
func StripTemplateComments(text string) string {
	return templatecomment.Strip(text)
}

// JoinCSS joins a carrier's CSS fields into one stylesheet. Each field's
// template comments are removed on their own first, so an unclosed [comment]
// hides the rest of its own field and not the fields after it.
func JoinCSS(parts ...string) string {
	stripped := make([]string, len(parts))
	for i, p := range parts {
		stripped[i] = templatecomment.Strip(p)
	}
	return strings.Join(stripped, "\n")
}

// htmlCommentSpans returns the byte range of every HTML comment in input, in
// order, as a browser reads them: the tokenizer knows that "--!>" closes one,
// that "<!--" in an attribute value or in <script> or <style> text is not one,
// and that an unclosed "<!--" runs to the end.
func htmlCommentSpans(input string) []templatecomment.Span {
	if !strings.Contains(input, "<!--") {
		return nil
	}
	var spans []templatecomment.Span
	z := html.NewTokenizer(strings.NewReader(input))
	for pos := 0; ; {
		tt := z.Next()
		if tt == html.ErrorToken {
			return spans
		}
		end := pos + len(z.Raw())
		if tt == html.CommentToken && strings.HasPrefix(input[pos:], "<!--") {
			spans = append(spans, templatecomment.Span{Start: pos, End: end})
		}
		pos = end
	}
}
