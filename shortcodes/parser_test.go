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

func TestParsePropertyShortcode(t *testing.T) {
	result := Parse(`before [property path="Name"] after`)
	assert.Len(t, result, 1)
	assert.Equal(t, "property", result[0].Name)
	assert.Equal(t, "Name", result[0].Attrs["path"])
	assert.Equal(t, `[property path="Name"]`, result[0].Raw)
}

func TestParsePropertyRawAttr(t *testing.T) {
	result := Parse(`[property path="Description" raw="true"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "property", result[0].Name)
	assert.Equal(t, "Description", result[0].Attrs["path"])
	assert.Equal(t, "true", result[0].Attrs["raw"])
}

func TestParseMRQLQueryShortcode(t *testing.T) {
	result := Parse(`[mrql query="type = 'resource'" limit="10" format="table"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "mrql", result[0].Name)
	assert.Equal(t, "type = 'resource'", result[0].Attrs["query"])
	assert.Equal(t, "10", result[0].Attrs["limit"])
	assert.Equal(t, "table", result[0].Attrs["format"])
}

func TestParseMRQLSavedShortcode(t *testing.T) {
	result := Parse(`[mrql saved="my-query" format="custom"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "mrql", result[0].Name)
	assert.Equal(t, "my-query", result[0].Attrs["saved"])
	assert.Equal(t, "custom", result[0].Attrs["format"])
}

func TestParseMRQLBucketsAttr(t *testing.T) {
	result := Parse(`[mrql query="type = 'resource' GROUP BY category" buckets="3" limit="5"]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "3", result[0].Attrs["buckets"])
	assert.Equal(t, "5", result[0].Attrs["limit"])
}

func TestParseHTMLEntityEncodedAttrs(t *testing.T) {
	// After markdown processing, " becomes &quot; — parser must handle this
	result := Parse(`[property path=&quot;Name&quot;]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "property", result[0].Name)
	assert.Equal(t, "Name", result[0].Attrs["path"])
}

func TestParseMRQLWithHTMLEntityQuotes(t *testing.T) {
	// MRQL query with &quot; from markdown processing
	result := Parse(`[mrql query=&quot;type = 'resource'&quot; limit=&quot;10&quot;]`)
	assert.Len(t, result, 1)
	assert.Equal(t, "type = 'resource'", result[0].Attrs["query"])
	assert.Equal(t, "10", result[0].Attrs["limit"])
}
