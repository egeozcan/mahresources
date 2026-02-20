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

	// KAN-4: html.WithUnsafe() is by design. Mahresources is a personal information management
	// application designed to run on private/internal networks with no authentication layer.
	// All users are trusted, and allowing raw HTML in Markdown enables rich content authoring.
	converter := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(), // KAN-4: by design — internal network app, all users trusted
		),
	)

	var buffer bytes.Buffer
	if err := converter.Convert([]byte(md), &buffer); err != nil {
		return pongo2.AsValue(""), nil
	}

	return pongo2.AsValue(buffer.String()), nil
}
