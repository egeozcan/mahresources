package shortcodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Both comment emitters share one rule, so neither can be closed by the text it
// carries. Entities are not decoded inside an HTML comment, so escaping ">" is
// what makes "-->" inert — nothing else can terminate a comment.
func TestShortcodeCommentCannotBeClosedFromInside(t *testing.T) {
	assert.Equal(t, "<!-- mr:a--&gt;b -->", shortcodeComment("a-->b"))
	assert.Equal(t, "<!-- mr:plain message -->", shortcodeComment("plain message"))
}
