package mcpserver

import (
	"strings"
	"testing"

	"mahresources/skills"
)

// The section filter must not treat a "## " line inside a code fence as a
// heading, and must keep every section it does not name.
func TestMRQLLanguageForMCPDropsOnlyNamedSections(t *testing.T) {
	got := mrqlLanguageForMCP()
	for _, line := range strings.Split(skills.MRQLLanguage, "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		if mrqlReferenceDroppedSections[title] == strings.Contains(got, "\n"+line+"\n") {
			t.Errorf("section %q: dropped=%v but present=%v", title, mrqlReferenceDroppedSections[title], !mrqlReferenceDroppedSections[title])
		}
	}
	// Every dropped name must still exist, or the filter silently keeps a
	// renamed CLI section.
	for title := range mrqlReferenceDroppedSections {
		if !strings.Contains(skills.MRQLLanguage, "\n## "+title+"\n") {
			t.Errorf("dropped section %q is not in the reference any more", title)
		}
	}
	if strings.Contains(got, "GENERATED FILE") {
		t.Error("generated-file header kept")
	}
}
