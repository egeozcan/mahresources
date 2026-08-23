package shortcodes

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// commentTerminators are the ways an HTML comment can end, plus the pieces they
// are built from. The rule the emitters keep is stronger than "no --> in the
// payload" and does not depend on enumerating these correctly: every ">" is
// escaped, so the only one left in the output is the one that closes the
// comment. Entities are not decoded inside a comment, so nothing puts it back.
var commentTerminators = []string{"-->", "--!>", "--", "<!--", "<!-->", ">", "--\n>"}

// Both comment emitters share one rule, so neither can be closed by the text it
// carries.
func TestShortcodeCommentCannotBeClosedFromInside(t *testing.T) {
	assert.Equal(t, "<!-- mr:a--&gt;b -->", shortcodeComment("a-->b"))
	assert.Equal(t, "<!-- mr:plain message -->", shortcodeComment("plain message"))

	for _, term := range commentTerminators {
		got := shortcodeComment("a" + term + "b")
		assert.True(t, strings.HasSuffix(got, " -->"), "%q: lost its own terminator", term)
		assert.Equal(t, 1, strings.Count(got, ">"),
			"%q: the payload contributed a %q that could close the comment", term, ">")
	}
}
