package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
)

// JSON's own words are readable: a meta string "false" is masked, and the
// sample's HasURL:false does not make the request look like a leak (round 6, #1).
func TestJSONLiteralsAreReadable(t *testing.T) {
	p := NewTemplatePrivacy("jl")
	g := &models.Group{Name: "No Url Group", Meta: types.JSON(`{"flag":"false","other":"null","yes":"true"}`)}
	sample := ProjectTemplateSampleFor(g, true, p, "")
	text := strings.Join(DescribeTemplateSamplesFor([]TemplateSample{sample}, TemplateTargetMetaSchema, false, p), "\n")
	if !strings.Contains(text, `"HasURL":false`) || strings.Contains(text, `"flag":"false"`) {
		t.Fatalf("HasURL is shown and the string value is masked:\n%s", text)
	}
	body, _ := json.Marshal(map[string]string{"m": text})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("the JSON literal false must not look like the masked string \"false\": %v", err)
	}
}

// Re-reading an issued date, or the same instant on another entity, costs nothing
// (round 6, #2).
func TestRereadingAnIssuedDateDoesNotRecharge(t *testing.T) {
	p := NewTemplatePrivacy("rd")
	when := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	p.Date(when)
	bytesAfter, valuesAfter, enrolled := p.bytes, len(p.guard.values), len(p.enrolled)
	p.Date(when)
	if _, err := p.Entity(context.Background(), &models.Group{CreatedAt: when}); err != nil {
		t.Fatal(err)
	}
	if p.bytes != bytesAfter || len(p.guard.values) != valuesAfter || len(p.enrolled) != enrolled {
		t.Fatalf("a re-read charged again: bytes %d->%d values %d->%d enrolled %d->%d", bytesAfter, p.bytes, valuesAfter, len(p.guard.values), enrolled, len(p.enrolled))
	}

	q := NewTemplatePrivacy("rq")
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < templatePrivacyMaxValues; i++ {
		q.Date(base.Add(time.Duration(i) * time.Second))
	}
	if q.Err() != nil {
		t.Fatalf("20,000 distinct instants fit exactly: %v", q.Err())
	}
	q.Date(base) // a revisit introduces no new value
	if q.Err() != nil {
		t.Fatalf("revisiting an issued date must not trip the limit: %v", q.Err())
	}
}

// The generated "[cut: N of M bytes shown]" notice is readable: a protected Name
// equal to N does not stop the request (round 6, #3).
func TestCutNoticeNumbersAreReadable(t *testing.T) {
	p := NewTemplatePrivacy("ct")
	p.Text("8192")
	var rows []string
	for i := 0; i < 2000; i++ {
		rows = append(rows, fmt.Sprintf(`{"name":%q}`, p.Text(fmt.Sprintf("entity number %d", i))))
	}
	out := ShapeToolResultFor(p, "tool get_entity", "["+strings.Join(rows, ",")+"]", 8192)
	if !out.Truncated || !strings.Contains(out.Text, "[cut: 8192 of ") {
		t.Fatalf("the result must be cut at 8192 bytes:\n%.200s", out.Text)
	}
	body, _ := json.Marshal(map[string]string{"m": out.Text})
	if err := p.CheckOutbound(context.Background(), body); err != nil {
		t.Fatalf("the generated cut notice must not look like the protected Name \"8192\": %v", err)
	}
}
