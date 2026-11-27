package shortcodes

import (
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// excerptMaskMarkdownLabels hides bare Markdown labels from the shortcode
// parser without shifting byte offsets. The original spelling is copied from
// src when deleting shortcode spans. Mask before pairing, so e.g. a Markdown
// [details](url) cannot take the rest of the text with its stray [/details].
func excerptMaskMarkdownLabels(src string) string {
	inline := strings.Contains(src, "](")
	reference := strings.Contains(src, "]:")
	if !inline && !reference {
		return src
	}
	var masked []byte
	var labels map[int]bool
	for _, m := range shortcodePattern.FindAllStringSubmatchIndex(src, -1) {
		// A real shortcode carries key=value attributes. Prose after the name
		// ("[link to the guide](/guide)") is a Markdown label, as is a bare name.
		if m[4] >= 0 && attrPattern.MatchString(src[m[4]:m[5]]) {
			continue
		}
		start, end := m[0], m[1]
		if reference && labels == nil {
			labels = excerptReferenceLabels(src)
		}
		if labels[start] || (end < len(src) && src[end] == '(') {
			if masked == nil {
				masked = []byte(src)
			}
			masked[start] = ' '
		}
	}
	if masked != nil {
		return string(masked)
	}
	return src
}

// Reference-style links are ambiguous without their definitions. Ask Goldmark
// rather than guessing from adjacent brackets, which may be two shortcodes.
var excerptReferenceParser = func() parser.Parser {
	inlines := parser.DefaultInlineParsers()
	for i, p := range inlines {
		if p.Value == parser.NewLinkParser() {
			inlines[i].Value = excerptLinkParser{p.Value.(parser.InlineParser)}
			break
		}
	}
	p := parser.NewParser(
		parser.WithBlockParsers(parser.DefaultBlockParsers()...),
		parser.WithInlineParsers(inlines...),
		parser.WithParagraphTransformers(parser.DefaultParagraphTransformers()...),
	)
	return goldmark.New(goldmark.WithParser(p), goldmark.WithExtensions(extension.GFM)).Parser()
}()

var excerptReferenceLabelsKey = parser.NewContextKey()

func excerptReferenceLabels(src string) map[int]bool {
	labels := make(map[int]bool)
	pc := parser.NewContext()
	pc.Set(excerptReferenceLabelsKey, labels)
	doc := excerptReferenceParser.Parse(text.NewReader([]byte(src)), parser.WithContext(pc))
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Kind() == ast.KindLinkReferenceDefinition {
			labels[n.Pos()] = true
		}
		return ast.WalkContinue, nil
	})
	return labels
}

// excerptLinkParser delegates to Goldmark's link parser and records the source
// positions of accepted reference labels. For a full reference, only the parser
// knows which ']' closes the first label: nested brackets and code are legal.
type excerptLinkParser struct{ parser.InlineParser }

func (p excerptLinkParser) Parse(parent ast.Node, block text.Reader, pc parser.Context) ast.Node {
	_, close := block.Position()
	n := p.InlineParser.Parse(parent, block, pc)
	var start int
	var ref *ast.ReferenceLink
	switch link := n.(type) {
	case *ast.Link:
		start, ref = link.Pos(), link.Reference
	case *ast.Image:
		start, ref = link.Pos()+1, link.Reference // skip '!'
	}
	if ref != nil {
		labels := pc.Get(excerptReferenceLabelsKey).(map[int]bool)
		labels[start] = true
		if ref.Type == ast.ReferenceLinkFull {
			labels[close.Start+1] = true // [text][reference]
		}
	}
	return n
}

func (p excerptLinkParser) CloseBlock(parent ast.Node, block text.Reader, pc parser.Context) {
	p.InlineParser.(parser.CloseBlocker).CloseBlock(parent, block, pc)
}
