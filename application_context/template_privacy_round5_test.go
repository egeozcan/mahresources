package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"mahresources/models"
)

// Privacy off: a long bracketed title that two samples share keeps the pre-change
// summary, compared against a literal (round 5, #2).
func TestPrivacyOffBracketedTitleKeepsThePreChangeSummary(t *testing.T) {
	title := "⟦this is an ordinary long bracketed title in the database⟧"
	if len(title) <= 40 {
		t.Fatal("the title must exceed the 40-byte threshold")
	}
	samples := []TemplateSample{{"Name": title}, {"Name": title}}
	for name, lines := range map[string][]string{
		"wrapper": DescribeTemplateSamples(samples, TemplateTargetBundle, false),
		"nil":     DescribeTemplateSamplesFor(samples, TemplateTargetBundle, false, nil),
	} {
		text := strings.Join(lines, "\n")
		if !strings.Contains(text, "- Name (2 of 2): string\n") && !strings.HasSuffix(strings.Split(text, "Raw samples:")[0], "- Name (2 of 2): string\n") {
			if !strings.Contains(text, "- Name (2 of 2): string") {
				t.Fatalf("%s: the summary line is the pre-change one, got:\n%s", name, text)
			}
		}
		if strings.Contains(text, "values seen") {
			t.Fatalf("%s: with Privacy off a long bracketed string gets no 'values seen' summary:\n%s", name, text)
		}
	}
	// With the masker, an exact issued Placeholder does count as short.
	p := NewTemplatePrivacy("pb")
	ph := p.Text("a long repeated protected value that is over the threshold of forty bytes")
	text := strings.Join(DescribeTemplateSamplesFor([]TemplateSample{{"Name": ph}, {"Name": ph}}, TemplateTargetBundle, false, p), "\n")
	if !strings.Contains(text, "values seen") {
		t.Fatalf("an issued Placeholder keeps its summary line:\n%s", text)
	}
}

// The counts the projection generates are preserved numbers (round 5, #3).
func TestSampleCountsAreReadableToTheGuard(t *testing.T) {
	p := NewTemplatePrivacy("sc")
	p.Text("4371")
	g := &models.Group{Name: "G"}
	for i := 0; i < 4371; i++ {
		g.Relationships = append(g.Relationships, &models.GroupRelation{ID: uint(i + 1), ToGroup: &models.Group{Name: fmt.Sprintf("target %d", i)}})
	}
	for i := 0; i < 20; i++ {
		g.Tags = append(g.Tags, &models.Tag{Name: fmt.Sprintf("tag %d", i)})
	}
	sample := ProjectTemplateSampleFor(g, true, p, "")
	text := strings.Join(DescribeTemplateSamplesFor([]TemplateSample{sample}, TemplateTargetSlot, false, p), "\n")
	if !strings.Contains(text, `"RelationCount":4371`) {
		t.Fatalf("the relation count is in the Seed text:\n%.400s", text)
	}
	body, _ := json.Marshal(map[string]string{"m": text})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("the generated count must not look like the protected string \"4371\": %v", err)
	}
	if strings.Contains(text, "target 7") {
		t.Fatal("the relation names are masked")
	}
}

// An original enrolled first and issued later is transferred, not charged twice
// (round 5, #4).
func TestProtectedThenIssuedIsTransferredNotRecharged(t *testing.T) {
	p := NewTemplatePrivacy("tr")
	p.protect("2024-05-06T07:08:09Z")
	bytesBefore, valuesBefore := p.bytes, len(p.guard.values)
	ph := p.Text("2024-05-06T07:08:09Z")
	if ph == "" || len(p.legendValues()) != 1 {
		t.Fatalf("issued: %q", ph)
	}
	if p.bytes != bytesBefore || len(p.guard.values) != valuesBefore || len(p.protected) != 0 {
		t.Fatalf("transfer must not recharge: bytes %d->%d values %d->%d protected %d", bytesBefore, p.bytes, valuesBefore, len(p.guard.values), len(p.protected))
	}
	// Date calls take the same path for every distinct whole-second instant.
	q := NewTemplatePrivacy("dq")
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 15000; i++ {
		q.Date(base.Add(time.Duration(i) * time.Second))
	}
	if q.Err() != nil {
		t.Fatalf("15,000 distinct instants are 15,000 values, not 30,000: %v", q.Err())
	}
	if want := 15000 * len("2024-01-01T00:00:00Z"); q.bytes != want {
		t.Fatalf("byte accounting: got %d want %d", q.bytes, want)
	}
}

func (p *TemplatePrivacy) legendValues() []string {
	var out []string
	for _, e := range p.Legend() {
		out = append(out, e.Value)
	}
	return out
}
