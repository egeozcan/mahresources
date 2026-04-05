package shortcodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseEmpty(t *testing.T) {
	result := Parse("")
	assert.Empty(t, result)
}

func TestParseNoShortcodes(t *testing.T) {
	result := Parse("just some plain text with [brackets]")
	assert.Empty(t, result)
}

func TestParseMetaShortcode(t *testing.T) {
	result := Parse(`[meta path="cooking.time"]`)
	require.Len(t, result, 1)
	assert.Equal(t, "meta", result[0].Name)
	assert.Equal(t, "cooking.time", result[0].Attrs["path"])
	assert.Equal(t, `[meta path="cooking.time"]`, result[0].Raw)
	assert.Equal(t, 0, result[0].Start)
	assert.Equal(t, 26, result[0].End)
}

func TestParseMultipleAttributes(t *testing.T) {
	result := Parse(`[meta path="cooking.time" editable=true hide-empty=false]`)
	require.Len(t, result, 1)
	assert.Equal(t, "cooking.time", result[0].Attrs["path"])
	assert.Equal(t, "true", result[0].Attrs["editable"])
	assert.Equal(t, "false", result[0].Attrs["hide-empty"])
}

func TestParseUnquotedValues(t *testing.T) {
	result := Parse(`[meta path="a.b" editable=true]`)
	require.Len(t, result, 1)
	assert.Equal(t, "true", result[0].Attrs["editable"])
}

func TestParseMultipleShortcodes(t *testing.T) {
	result := Parse(`before [meta path="a"] middle [meta path="b"] after`)
	require.Len(t, result, 2)
	assert.Equal(t, "a", result[0].Attrs["path"])
	assert.Equal(t, "b", result[1].Attrs["path"])
}

func TestParsePluginShortcode(t *testing.T) {
	result := Parse(`[plugin:my-plugin:rating max="5"]`)
	require.Len(t, result, 1)
	assert.Equal(t, "plugin:my-plugin:rating", result[0].Name)
	assert.Equal(t, "5", result[0].Attrs["max"])
}

func TestParsePreservesHTMLAround(t *testing.T) {
	result := Parse(`<div class="flex">[meta path="a"]</div>`)
	require.Len(t, result, 1)
	assert.Equal(t, 18, result[0].Start)
}

func TestParseIgnoresUnrecognizedBrackets(t *testing.T) {
	result := Parse(`see [this page] for details`)
	assert.Empty(t, result)
}

func TestParseSingleQuotedValues(t *testing.T) {
	result := Parse(`[meta path='cooking.time']`)
	require.Len(t, result, 1)
	assert.Equal(t, "cooking.time", result[0].Attrs["path"])
}

func TestParsePluginNameWithUnderscore(t *testing.T) {
	result := Parse(`[plugin:my_plugin:star_rating max="5"]`)
	require.Len(t, result, 1)
	assert.Equal(t, "plugin:my_plugin:star_rating", result[0].Name)
	assert.Equal(t, "5", result[0].Attrs["max"])
}
