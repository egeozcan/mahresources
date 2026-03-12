package template_filters

import (
	"fmt"
	"html"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/lib"
)

func renderMentionsFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	preview := param != nil && param.String() == "preview"
	interfaceVal := in.Interface()
	var text string

	switch v := interfaceVal.(type) {
	case string:
		text = v
	case *string:
		if v == nil {
			return in, nil
		}
		text = *v
	default:
		return in, nil
	}

	mentions := lib.ParseMentions(text)
	if len(mentions) == 0 {
		return in, nil
	}

	for _, m := range mentions {
		marker := m.OriginalMatch
		escapedName := html.EscapeString(m.Name)

		path, ok := entityPaths[m.Type]
		if !ok {
			path = "/" + m.Type
		}

		var replacement string

		if m.Type == "resource" {
			if !preview && lib.IsMentionOnlyOnLine(text, marker) {
				replacement = fmt.Sprintf(
					`<a href="%s?id=%d" class="mention-card">`+
						`<img src="/v1/resource/preview?id=%d" alt="%s" class="mention-card-thumb">`+
						`<span class="mention-card-name">%s</span></a>`,
					path, m.ID, m.ID, escapedName, escapedName,
				)
			} else {
				replacement = fmt.Sprintf(
					`<a href="%s?id=%d" class="mention-inline">`+
						`<img src="/v1/resource/preview?id=%d" alt="" class="mention-inline-thumb">`+
						`%s</a>`,
					path, m.ID, m.ID, escapedName,
				)
			}
		} else {
			replacement = fmt.Sprintf(
				`<a href="%s?id=%d" class="mention-badge mention-%s">%s</a>`,
				path, m.ID, m.Type, escapedName,
			)
		}

		text = strings.Replace(text, marker, replacement, 1)
	}

	return pongo2.AsValue(text), nil
}
