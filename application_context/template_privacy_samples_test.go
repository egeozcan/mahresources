package application_context

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
)

func privacyTestGroup(tagOrder []string) *models.Group {
	g := &models.Group{
		ID: 1, Name: "Moonbase Alpha", Description: strings.Repeat("long protected description ", 20),
		Owner: &models.Group{Name: "Parent Secret"},
		Meta:  types.JSON(`{"status":"open","note":"protected note","n":4}`),
		Relationships: []*models.GroupRelation{
			{ID: 1, ToGroup: &models.Group{Name: "Target Secret"}, RelationType: &models.GroupRelationType{Name: "depends-on"}},
		},
	}
	for _, name := range tagOrder {
		g.Tags = append(g.Tags, &models.Tag{Name: name})
	}
	return g
}

func TestProjectTemplateSampleForMasksValuesAndKeepsVocabulary(t *testing.T) {
	p := NewTemplatePrivacy("sp")
	schema := `{"properties":{"status":{"enum":["open","closed"]}}}`
	s := ProjectTemplateSampleFor(privacyTestGroup([]string{"red", "blue"}), true, p, schema)
	blob, _ := json.Marshal(s)
	for _, secret := range []string{"Moonbase", "protected", "Parent Secret", "Target Secret", "red", "blue"} {
		if strings.Contains(string(blob), secret) {
			t.Errorf("%q leaked: %s", secret, blob)
		}
	}
	if !strings.Contains(string(blob), "depends-on") || !strings.Contains(string(blob), `"status":"open"`) {
		t.Errorf("relation type names and enum values stay readable: %s", blob)
	}
	if desc := s["Description"].(string); strings.Contains(desc, "…(") {
		t.Errorf("a Placeholder is not clipped: %s", desc)
	}
	var legendValues []string
	for _, e := range p.Legend() {
		legendValues = append(legendValues, e.Value)
	}
	if !strings.Contains(strings.Join(legendValues, "|"), strings.Repeat("long protected description ", 20)) {
		t.Error("the legend holds the whole unclipped value")
	}
}

func TestProjectTemplateSampleForNilPrivacyMatchesTheOriginal(t *testing.T) {
	g := privacyTestGroup([]string{"red", "blue"})
	a, _ := json.Marshal(ProjectTemplateSample(g, true))
	b, _ := json.Marshal(ProjectTemplateSampleFor(g, true, nil, `{"x":1}`))
	if string(a) != string(b) {
		t.Fatalf("privacy off must not change a byte:\n%s\n%s", a, b)
	}
}

func TestProjectTemplateSampleForIsDeterministicAcrossAssociationOrder(t *testing.T) {
	render := func(order []string) string {
		p := NewTemplatePrivacy("dt")
		s := ProjectTemplateSampleFor(privacyTestGroup(order), true, p, "")
		return strings.Join(DescribeTemplateSamples([]TemplateSample{s}, TemplateTargetSlot, false), "\n")
	}
	if a, b := render([]string{"red", "green", "blue"}), render([]string{"blue", "red", "green"}); a != b {
		t.Fatalf("shuffled association order changed the Seed:\n%s\n%s", a, b)
	}
}

func TestProjectTemplateSampleForMasksNoteDates(t *testing.T) {
	when := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	p := NewTemplatePrivacy("nd")
	s := ProjectTemplateSampleFor(&models.Note{Name: "n", StartDate: &when}, false, p, "")
	if got := s["StartDate"].(string); !strings.HasPrefix(got, "⟦nd·date ") {
		t.Fatalf("dates are Placeholders: %q", got)
	}
}
