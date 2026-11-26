package template_filters

import (
	"context"
	"html"
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	xhtml "golang.org/x/net/html"

	"mahresources/models"
	"mahresources/shortcodes"
)

// [excerpt] parses Markdown with its own goldmark instance. This pins that its
// text is the visible text of what markdown2 renders, so a change to either
// configuration that changes what a reader sees fails here. The corpus holds no
// mentions and no shortcodes: on a page render_mentions and the shortcode pass
// run after markdown2, so neither belongs to the comparison.
func TestExcerptMatchesTheVisibleTextOfMarkdown2(t *testing.T) {
	corpus := map[string]string{
		"gfm table":     "| Fruit | Qty |\n|-------|----:|\n| apple | 3 |\n| pear | 10 |",
		"strikethrough": "keep ~~drop~~ this",
		"autolink":      "see https://example.com/a?b=c and www.example.org",
		"task list":     "- [x] done\n- [ ] open",
		"nested list":   "1. one\n   - inner a\n   - inner b\n2. two",
		"inline html":   "a <span class=\"x\">span</span> <b>bold</b> <img src=\"x.png\" alt=\"alt\"> z",
		"html block":    "<div>\n<p>inside div</p>\n</div>\n\nafter",
		"hard wraps":    "line one\nline two\nline three",
		"entities":      "caf&eacute; &amp; &lt;tag&gt; &copy; 5 < 6 & 7",
		"headings":      "# Title\n\nbody\n\n## Sub {#custom}\n\ntext",
		"code":          "`inline` and\n\n```go\nfunc main() {}\n```",
		"emphasis":      "*em* **strong** ***both*** _under_",
		"links":         "[text](http://x \"title\") ![image](y.png) <http://z>",
		"blockquote":    "> quoted\n> more\n\nplain",
		"script":        "before <script>var x = 1;</script> after",
	}
	for name, src := range corpus {
		md, _ := markDownFilter(pongo2.AsValue(src), nil)
		want := visibleTextForParity(md.String())
		ctx := shortcodes.MetaShortcodeContext{EntityType: "note", EntityID: 1, Entity: &models.Note{ID: 1, Description: src}}
		got := shortcodes.Process(context.Background(), `[excerpt words="200"]`, ctx, nil, nil)
		if got != want {
			t.Errorf("%s:\n excerpt:   %q\n markdown2: %q", name, got, want)
		}
	}
}

// visibleTextForParity is the text of rendered HTML as [excerpt] reads it: raw
// text elements and tags dropped, block boundaries a space, whitespace
// collapsed, then escaped.
func visibleTextForParity(rendered string) string {
	rawText := map[string]bool{"script": true, "style": true, "textarea": true, "title": true, "xmp": true,
		"iframe": true, "noembed": true, "noframes": true, "noscript": true, "plaintext": true}
	block := map[string]bool{"p": true, "div": true, "br": true, "li": true, "tr": true, "td": true, "th": true,
		"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "blockquote": true, "pre": true,
		"hr": true, "ul": true, "ol": true, "table": true, "dd": true, "dt": true, "section": true, "article": true}
	var b strings.Builder
	z := xhtml.NewTokenizer(strings.NewReader(rendered))
	skipping := ""
	for {
		tt := z.Next()
		if tt == xhtml.ErrorToken {
			break
		}
		name, _ := z.TagName()
		switch {
		case tt == xhtml.TextToken && skipping == "":
			b.Write(z.Text())
		case (tt == xhtml.StartTagToken || tt == xhtml.SelfClosingTagToken) && rawText[string(name)]:
			skipping = string(name)
		case tt == xhtml.EndTagToken && skipping != "":
			if string(name) == skipping {
				skipping = ""
			}
		case (tt == xhtml.StartTagToken || tt == xhtml.SelfClosingTagToken || tt == xhtml.EndTagToken) && skipping == "" && block[string(name)]:
			b.WriteByte(' ')
		}
	}
	return html.EscapeString(strings.Join(strings.Fields(b.String()), " "))
}
