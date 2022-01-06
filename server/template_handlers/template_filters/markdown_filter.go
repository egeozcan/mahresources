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

	switch interfaceVal.(type) {
	case string:
		md = interfaceVal.(string)
	case *string:
		md = *interfaceVal.(*string)
	}

	converter := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithUnsafe(),
		),
	)

	var buffer bytes.Buffer
	if err := converter.Convert([]byte(md), &buffer); err != nil {
		return pongo2.AsValue(""), nil
	}

	return pongo2.AsValue(buffer.String()), nil
}
