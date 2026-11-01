package template_filters

import (
	"testing"

	"github.com/flosch/pongo2/v4"
	"github.com/stretchr/testify/assert"
)

func TestStripTemplateCommentsFilter(t *testing.T) {
	out, err := stripTemplateCommentsFilter(pongo2.AsValue("a [comment]b[/comment]c"), nil)
	assert.Nil(t, err)
	assert.Equal(t, "a c", out.String())
}
