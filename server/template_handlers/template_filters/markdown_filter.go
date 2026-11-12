package template_filters

import (
	"bytes"
	"github.com/flosch/pongo2/v4"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

func markDownFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	interfaceVal := in.Interface()
	var md string

	switch v := interfaceVal.(type) {
	case string:
		md = v
	case *string:
		md = *v
	}

	// KAN-4: html.WithUnsafe() is by design, and with auth off (the default) the output is used
	// as is: every user of a server without accounts is trusted, and raw HTML in Markdown enables
	// rich content authoring. With -auth on, accounts less trusted than an admin write the same
	// text, so every page that renders a Description or a note text block passes this output
	// through sanitize_under_auth (sanitize_filter.go) before shortcodes expand (#92).
	converter := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(), // KAN-4; sanitized afterwards under -auth (#92)
		),
	)

	var buffer bytes.Buffer
	if err := converter.Convert([]byte(md), &buffer); err != nil {
		return pongo2.AsValue(""), nil
	}

	return pongo2.AsValue(buffer.String()), nil
}

// RenderDescriptionMarkup is the markup half of how a Description is shown on
// its detail page (templates/partials/description.tpl): markdown2, then
// render_mentions, then sanitize_under_auth. The shortcode pass, which needs
// the entity, comes after.
func RenderDescriptionMarkup(text string, authEnabled bool) string {
	md, _ := markDownFilter(pongo2.AsValue(text), nil)
	withMentions, _ := renderMentionsFilter(md, nil)
	sanitized, _ := sanitizeUnderAuthFilter(withMentions, pongo2.AsValue(authEnabled))
	return sanitized.String()
}
