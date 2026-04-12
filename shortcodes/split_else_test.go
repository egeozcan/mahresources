package shortcodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitElseNoBranch(t *testing.T) {
	ifBranch, elseBranch := SplitElse("just content")
	assert.Equal(t, "just content", ifBranch)
	assert.Equal(t, "", elseBranch)
}

func TestSplitElseSimple(t *testing.T) {
	ifBranch, elseBranch := SplitElse("yes[else]no")
	assert.Equal(t, "yes", ifBranch)
	assert.Equal(t, "no", elseBranch)
}

func TestSplitElseWithWhitespace(t *testing.T) {
	ifBranch, elseBranch := SplitElse("  yes  [else]  no  ")
	assert.Equal(t, "  yes  ", ifBranch)
	assert.Equal(t, "  no  ", elseBranch)
}

func TestSplitElseNestedBlockIgnored(t *testing.T) {
	input := `[conditional path="x" eq="1"]inner[else]fallback[/conditional][else]outer-else`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[conditional path="x" eq="1"]inner[else]fallback[/conditional]`, ifBranch)
	assert.Equal(t, "outer-else", elseBranch)
}

func TestSplitElseMultipleTopLevel(t *testing.T) {
	ifBranch, elseBranch := SplitElse("a[else]b[else]c")
	assert.Equal(t, "a", ifBranch)
	assert.Equal(t, "b[else]c", elseBranch)
}

func TestSplitElseEmptyBranches(t *testing.T) {
	ifBranch, elseBranch := SplitElse("[else]")
	assert.Equal(t, "", ifBranch)
	assert.Equal(t, "", elseBranch)
}

func TestSplitElseSelfClosingBeforeElse(t *testing.T) {
	input := `[meta path="x"][else]fallback`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[meta path="x"]`, ifBranch)
	assert.Equal(t, "fallback", elseBranch)
}

func TestSplitElseMultipleSelfClosingBeforeElse(t *testing.T) {
	input := `[meta path="a"][property path="Name"][else]no`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[meta path="a"][property path="Name"]`, ifBranch)
	assert.Equal(t, "no", elseBranch)
}

func TestSplitElseSelfClosingSameNameAsLaterBlock(t *testing.T) {
	input := `[conditional path="x" eq="1"][conditional path="y" eq="2"]inner[/conditional][else]fallback`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[conditional path="x" eq="1"][conditional path="y" eq="2"]inner[/conditional]`, ifBranch)
	assert.Equal(t, "fallback", elseBranch)
}

func TestSplitElseNestedBlockStartsWithElse(t *testing.T) {
	input := `[conditional path="a" eq="1"][else]nested-else[/conditional][else]outer-else`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[conditional path="a" eq="1"][else]nested-else[/conditional]`, ifBranch)
	assert.Equal(t, "outer-else", elseBranch)
}

func TestSplitElseNestedMultipleLevels(t *testing.T) {
	input := `before[conditional path="a" eq="1"][conditional path="b" eq="2"]deep[else]deep-else[/conditional][else]mid-else[/conditional][else]top-else`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `before[conditional path="a" eq="1"][conditional path="b" eq="2"]deep[else]deep-else[/conditional][else]mid-else[/conditional]`, ifBranch)
	assert.Equal(t, "top-else", elseBranch)
}
