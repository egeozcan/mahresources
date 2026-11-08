package application_context

import (
	"fmt"
	"testing"
)

// The 20,000 bound counts issued and guard-only values together, in either
// order (round 4, #3).
func TestCombinedValueBoundCountsIssuedAndProtected(t *testing.T) {
	p := NewTemplatePrivacy("cb")
	for i := 0; i < templatePrivacyMaxValues; i++ {
		p.protect(fmt.Sprintf("original-%06d", i))
	}
	if p.Err() != nil {
		t.Fatalf("20,000 protected values fit: %v", p.Err())
	}
	if got := p.Text("one more value"); got != "⟦cb·withheld⟧" || p.Err() == nil {
		t.Fatalf("the 20,001st value overall must overflow even when the first 20,000 were guard-only: %q %v", got, p.Err())
	}

	q := NewTemplatePrivacy("cq")
	for i := 0; i < templatePrivacyMaxValues-1; i++ {
		q.Text(fmt.Sprintf("issued-%06d", i))
	}
	q.protect("the last original that fits")
	if q.Err() != nil {
		t.Fatalf("exactly 20,000: %v", q.Err())
	}
	q.protect("one original too many")
	if q.Err() == nil {
		t.Fatal("protect must overflow past the combined bound")
	}
}
