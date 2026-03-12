package template_filters

import (
	"fmt"
	"html"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/lib"
)

func renderMentionsFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
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

	mentions := lib.ParseAllMentions(text)
	if len(mentions) == 0 {
		return in, nil
	}

	// Process each mention occurrence individually so that the card-vs-inline
	// decision is based on the specific position, not just any line in the text.
	// Build the result by scanning left-to-right and replacing one marker at a time.
	for _, m := range mentions {
		marker := m.OriginalMatch
		pos := strings.Index(text, marker)
		if pos == -1 {
			continue
		}

		escapedName := html.EscapeString(m.Name)

		path, ok := entityPaths[m.Type]
		if !ok {
			path = "/" + m.Type
		}

		var replacement string

		if m.Type == "resource" {
			if lib.IsMentionStandaloneAt(text, pos, marker) {
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

		text = text[:pos] + replacement + text[pos+len(marker):]
	}

	return pongo2.AsValue(text), nil
}
