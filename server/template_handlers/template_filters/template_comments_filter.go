package template_filters

import (
	"github.com/flosch/pongo2/v4"

	"mahresources/shortcodes"
)

// stripTemplateCommentsFilter removes [comment]...[/comment] regions from text
// a page shows without running shortcodes over it, like a Description preview
// on a card, so a comment hides there as it does on the detail page.
func stripTemplateCommentsFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(shortcodes.StripTemplateComments(in.String())), nil
}
