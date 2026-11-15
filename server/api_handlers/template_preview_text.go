package api_handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// previewTextLimit caps the text form of a preview.
const previewTextLimit = 4 << 10

// previewBlockElements start a new line in the text form.
var previewBlockElements = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "br": true, "dd": true, "div": true,
	"dl": true, "dt": true, "figcaption": true, "figure": true, "footer": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "header": true, "hr": true, "li": true, "main": true, "nav": true, "ol": true,
	"p": true, "pre": true, "section": true, "table": true, "tr": true, "ul": true,
}

// previewText renders a preview's HTML as the text a reader would see. The
// browser renders [meta] as a <meta-shortcode> element whose value sits in a
// data attribute; here the value is written in its place, so a model reading
// the preview sees "Status: active" rather than an empty element. Script and
// style bodies are dropped.
func previewText(src string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(src))
	skip := 0 // depth inside an element whose body is not text
	newline := func() {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
	}
	for b.Len() < previewTextLimit {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return finishPreviewText(b.String())
		case html.TextToken:
			if skip == 0 {
				b.WriteString(collapseSpace(string(z.Text())))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			switch {
			case tag == "script" || tag == "style" || tag == "template":
				if tt == html.StartTagToken {
					skip++
				}
			case tag == "meta-shortcode":
				var value string
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					if string(k) == "data-value" {
						value = metaDisplayValue(string(v))
					}
				}
				if skip == 0 {
					b.WriteString(value)
				}
				if tt == html.StartTagToken {
					skip++
				}
			case previewBlockElements[tag]:
				if skip == 0 {
					newline()
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			switch {
			case tag == "script" || tag == "style" || tag == "template" || tag == "meta-shortcode":
				if skip > 0 {
					skip--
				}
			case previewBlockElements[tag]:
				if skip == 0 {
					newline()
				}
			}
		}
	}
	return finishPreviewText(b.String()) + "…"
}

// collapseSpace folds each whitespace run into one space, keeping a leading
// or trailing one, so words in neighbouring text nodes stay apart.
func collapseSpace(s string) string {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		if s != "" {
			return " "
		}
		return ""
	}
	out := strings.Join(fields, " ")
	if strings.TrimLeft(s, " \t\r\n") != s {
		out = " " + out
	}
	if strings.TrimRight(s, " \t\r\n") != s {
		out += " "
	}
	return out
}

func finishPreviewText(s string) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// metaDisplayValue turns a [meta] data-value (JSON) into display text.
func metaDisplayValue(raw string) string {
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return raw
	}
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case float64, bool:
		return fmt.Sprint(t)
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}
