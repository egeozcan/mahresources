package lib

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMentions_EmptyText(t *testing.T) {
	assert.Empty(t, ParseMentions(""))
}

func TestParseMentions_NoMentions(t *testing.T) {
	assert.Empty(t, ParseMentions("Hello world, no mentions here."))
}

func TestParseMentions_SingleMention(t *testing.T) {
	result := ParseMentions("Hello @[group:42:My Group] world")
	assert.Equal(t, []Mention{
		{Type: "group", ID: 42, Name: "My Group", OriginalMatch: "@[group:42:My Group]"},
	}, result)
}

func TestParseMentions_MultipleMentions(t *testing.T) {
	result := ParseMentions("See @[note:1:First Note] and @[resource:2:Some File]")
	assert.Equal(t, []Mention{
		{Type: "note", ID: 1, Name: "First Note", OriginalMatch: "@[note:1:First Note]"},
		{Type: "resource", ID: 2, Name: "Some File", OriginalMatch: "@[resource:2:Some File]"},
	}, result)
}

func TestParseMentions_ColonsInDisplayName(t *testing.T) {
	result := ParseMentions("Check @[group:5:Meeting: Monday: 9AM]")
	assert.Equal(t, []Mention{
		{Type: "group", ID: 5, Name: "Meeting: Monday: 9AM", OriginalMatch: "@[group:5:Meeting: Monday: 9AM]"},
	}, result)
}

func TestParseMentions_InvalidID_NonNumeric(t *testing.T) {
	// The regex requires \d+ so non-numeric IDs won't match at all
	assert.Empty(t, ParseMentions("@[group:abc:Bad ID]"))
}

func TestParseMentions_InvalidID_Zero(t *testing.T) {
	assert.Empty(t, ParseMentions("@[group:0:Zero ID]"))
}

func TestParseMentions_Deduplication(t *testing.T) {
	result := ParseMentions("@[group:1:First] and again @[group:1:First]")
	assert.Len(t, result, 1)
	assert.Equal(t, Mention{Type: "group", ID: 1, Name: "First", OriginalMatch: "@[group:1:First]"}, result[0])
}

func TestParseMentions_DeduplicationKeepsFirst(t *testing.T) {
	// Same type+id but different display name: keeps the first occurrence
	result := ParseMentions("@[group:1:Original Name] and @[group:1:Different Name]")
	assert.Len(t, result, 1)
	assert.Equal(t, "Original Name", result[0].Name)
}

func TestParseMentions_DeduplicationDifferentTypes(t *testing.T) {
	// Same ID but different types should NOT be deduplicated
	result := ParseMentions("@[group:1:A Group] and @[note:1:A Note]")
	assert.Len(t, result, 2)
}

func TestParseMentions_AllEntityTypes(t *testing.T) {
	text := "@[group:1:G] @[note:2:N] @[resource:3:R] @[tag:4:T] @[category:5:C] @[query:6:Q]"
	result := ParseMentions(text)
	assert.Len(t, result, 6)

	types := make(map[string]bool)
	for _, m := range result {
		types[m.Type] = true
	}
	assert.True(t, types["group"])
	assert.True(t, types["note"])
	assert.True(t, types["resource"])
	assert.True(t, types["tag"])
	assert.True(t, types["category"])
	assert.True(t, types["query"])
}

func TestParseMentions_TypeIsLowercased(t *testing.T) {
	result := ParseMentions("@[Group:1:Mixed Case Type]")
	assert.Len(t, result, 1)
	assert.Equal(t, "group", result[0].Type)
}

func TestParseMentions_LargeID(t *testing.T) {
	result := ParseMentions("@[resource:999999999:Big ID]")
	assert.Len(t, result, 1)
	assert.Equal(t, uint(999999999), result[0].ID)
}

// --- IsMentionOnlyOnLine tests ---

func TestIsMentionOnlyOnLine_Standalone(t *testing.T) {
	assert.True(t, IsMentionOnlyOnLine("@[group:1:Test]", "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_StandaloneWithWhitespace(t *testing.T) {
	assert.True(t, IsMentionOnlyOnLine("  @[group:1:Test]  ", "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_StandaloneWithTabs(t *testing.T) {
	assert.True(t, IsMentionOnlyOnLine("\t@[group:1:Test]\t", "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_Inline(t *testing.T) {
	assert.False(t, IsMentionOnlyOnLine("Hello @[group:1:Test] world", "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_InlinePrefix(t *testing.T) {
	assert.False(t, IsMentionOnlyOnLine("See @[group:1:Test]", "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_InlineSuffix(t *testing.T) {
	assert.False(t, IsMentionOnlyOnLine("@[group:1:Test] here", "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_MultilineStandalone(t *testing.T) {
	text := "Some text here\n@[group:1:Test]\nMore text"
	assert.True(t, IsMentionOnlyOnLine(text, "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_MultilineInline(t *testing.T) {
	text := "Some text here\nSee @[group:1:Test] for details\nMore text"
	assert.False(t, IsMentionOnlyOnLine(text, "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_MultilineStandaloneWithWhitespace(t *testing.T) {
	text := "Some text\n   @[group:1:Test]   \nMore text"
	assert.True(t, IsMentionOnlyOnLine(text, "@[group:1:Test]"))
}

func TestIsMentionOnlyOnLine_MarkerNotPresent(t *testing.T) {
	assert.False(t, IsMentionOnlyOnLine("Hello world", "@[group:1:Test]"))
}

// --- GroupMentionsByType tests ---

func TestGroupMentionsByType_Empty(t *testing.T) {
	result := GroupMentionsByType(nil)
	assert.Empty(t, result)
}

func TestGroupMentionsByType_SingleType(t *testing.T) {
	mentions := []Mention{
		{Type: "group", ID: 1, Name: "A", OriginalMatch: "@[group:1:A]"},
		{Type: "group", ID: 2, Name: "B", OriginalMatch: "@[group:2:B]"},
	}
	result := GroupMentionsByType(mentions)
	assert.Equal(t, map[string][]uint{
		"group": {1, 2},
	}, result)
}

func TestGroupMentionsByType_MultipleTypes(t *testing.T) {
	mentions := []Mention{
		{Type: "group", ID: 1, Name: "G1", OriginalMatch: "@[group:1:G1]"},
		{Type: "note", ID: 2, Name: "N1", OriginalMatch: "@[note:2:N1]"},
		{Type: "group", ID: 3, Name: "G2", OriginalMatch: "@[group:3:G2]"},
		{Type: "resource", ID: 4, Name: "R1", OriginalMatch: "@[resource:4:R1]"},
		{Type: "note", ID: 5, Name: "N2", OriginalMatch: "@[note:5:N2]"},
	}
	result := GroupMentionsByType(mentions)
	assert.Equal(t, map[string][]uint{
		"group":    {1, 3},
		"note":     {2, 5},
		"resource": {4},
	}, result)
}

func TestGroupMentionsByType_Integration(t *testing.T) {
	text := "@[group:1:G1] @[note:2:N1] @[group:3:G2] @[resource:4:R1]"
	mentions := ParseMentions(text)
	result := GroupMentionsByType(mentions)

	assert.Len(t, result, 3)
	assert.Equal(t, []uint{1, 3}, result["group"])
	assert.Equal(t, []uint{2}, result["note"])
	assert.Equal(t, []uint{4}, result["resource"])
}
