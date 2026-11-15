package api_handlers

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

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

// previewTextBuilder writes at most previewTextLimit bytes, cut on a rune
// boundary, and remembers whether anything was cut.
type previewTextBuilder struct {
	b   strings.Builder
	cut bool
}

func (p *previewTextBuilder) write(s string) {
	room := previewTextLimit - p.b.Len()
	if room <= 0 {
		p.cut = p.cut || s != ""
		return
	}
	if len(s) > room {
		s = s[:room]
		for !utf8.ValidString(s) && len(s) > 0 {
			s = s[:len(s)-1]
		}
		p.cut = true
	}
	p.b.WriteString(s)
}

func (p *previewTextBuilder) newline() {
	if p.b.Len() > 0 && !strings.HasSuffix(p.b.String(), "\n") {
		p.write("\n")
	}
}

// previewText renders a preview's HTML as the text a reader would see. The
// browser renders [meta] as a <meta-shortcode> element whose value sits in a
// data attribute; here the element is replaced by what it displays (see
// metaElementText), so a model reading the preview sees "Status: active".
// Script, style and template bodies are dropped.
func previewText(src string) string {
	var p previewTextBuilder
	z := html.NewTokenizer(strings.NewReader(src))
	skip := 0 // depth inside an element whose body is not text
	for !p.cut {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return finishPreviewText(p.b.String(), false)
		case html.TextToken:
			if skip == 0 {
				p.write(collapseSpace(string(z.Text())))
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
				attrs := map[string]string{}
				present := map[string]bool{}
				for hasAttr {
					var k, v []byte
					k, v, hasAttr = z.TagAttr()
					attrs[string(k)] = string(v)
					present[string(k)] = true
				}
				if skip == 0 {
					p.write(metaElementText(attrs, present["data-value"]))
				}
				if tt == html.StartTagToken {
					skip++
				}
			case previewBlockElements[tag]:
				if skip == 0 {
					p.newline()
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
					p.newline()
				}
			}
		}
	}
	return finishPreviewText(p.b.String(), true)
}

// metaElementText is what a read-only <meta-shortcode> shows, following
// src/webcomponents/meta-shortcode.ts: an empty value (absent, null or blank
// text) shows nothing under hide-empty, else the default, else "Label: —"
// with the schema's title or the path's last segment title-cased.
func metaElementText(attrs map[string]string, hasValue bool) string {
	var value any
	if hasValue && json.Unmarshal([]byte(attrs["data-value"]), &value) != nil {
		value = attrs["data-value"]
	}
	empty := !hasValue || value == nil
	if s, ok := value.(string); ok && strings.TrimSpace(s) == "" {
		empty = true
	}
	if !empty {
		if label, ok := schemaDisplay(attrs["data-schema"], value); ok {
			return label
		}
		switch t := value.(type) {
		case string:
			return t
		case float64, bool:
			return fmt.Sprint(t)
		default:
			b, _ := json.Marshal(t)
			return string(b)
		}
	}
	if attrs["data-hide-empty"] == "true" {
		return ""
	}
	if d := attrs["data-default"]; d != "" {
		return d
	}
	return metaLabel(attrs["data-schema"], attrs["data-path"]) + ": —"
}

var (
	titleCaseCamel = regexp.MustCompile(`([a-z])([A-Z])`)
	titleCaseWord  = regexp.MustCompile(`\b\w`)
)

// metaLabel is the element's label: the schema title, else the last path
// segment title-cased (titleCase in src/schema-editor/schema-core.ts).
func metaLabel(schema, path string) string {
	var s struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(schema), &s) == nil && s.Title != "" {
		return s.Title
	}
	parts := strings.Split(path, ".")
	key := parts[len(parts)-1]
	key = titleCaseCamel.ReplaceAllString(key, "$1 $2")
	key = strings.NewReplacer("_", " ", "-", " ").Replace(key)
	return titleCaseWord.ReplaceAllStringFunc(key, strings.ToUpper)
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

func finishPreviewText(s string, cut bool) string {
	lines := strings.Split(s, "\n")
	out := lines[:0]
	for _, l := range lines {
		if l = strings.Join(strings.Fields(l), " "); l != "" {
			out = append(out, l)
		}
	}
	text := strings.Join(out, "\n")
	if cut {
		text += "…"
	}
	return text
}

// schemaDisplay applies the element's schema-driven display for the common
// cases: a labeled enum (oneOf entries with const) shows the entry's title,
// and a boolean property shows Yes or No. Custom displays (x-display,
// date-time layouts, plugin renderers) are not reproduced; the stored value
// is shown for them.
func schemaDisplay(schemaText string, value any) (string, bool) {
	var schema struct {
		Type  json.RawMessage `json:"type"`
		OneOf []struct {
			Const any    `json:"const"`
			Title string `json:"title"`
		} `json:"oneOf"`
	}
	if schemaText == "" || json.Unmarshal([]byte(schemaText), &schema) != nil {
		return "", false
	}
	want, _ := json.Marshal(value)
	for _, entry := range schema.OneOf {
		// Strict, as the element compares (===): 3 is not "3".
		if got, _ := json.Marshal(entry.Const); entry.Const != nil && string(got) == string(want) {
			if entry.Title != "" {
				return entry.Title, true
			}
			return fmt.Sprint(value), true
		}
	}
	if b, ok := value.(bool); ok && strings.Contains(string(schema.Type), `"boolean"`) {
		if b {
			return "Yes", true
		}
		return "No", true
	}
	return "", false
}
