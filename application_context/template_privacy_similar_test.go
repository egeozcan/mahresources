package application_context

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/shortcodes"
)

// [similar] in a Privacy-mode preview: the neighbour's and the target's names
// and the hash are masked, the distance and tier stay readable, and the guard
// does not trip on the rendered strip. (A distance is at most two digits, below
// the guard's four-character floor, so registering it as a kept scalar is for
// the corpus's completeness rather than a false positive this could show.)
func TestPrivacyModeMasksSimilarStrip(t *testing.T) {
	ctx := context.Background()
	p := NewTemplatePrivacy("sim")
	d1, d3 := 1, 3
	raw := &shortcodes.QueryResult{EntityType: "resource", Mode: "flat", Items: []shortcodes.QueryResultItem{
		{EntityType: "resource", EntityID: 17, Entity: &models.Resource{ID: 17, Name: "SECRET_TWIN_NAME", Hash: "SECRETHASH17"}, SimilarityDistance: &d1},
		{EntityType: "resource", EntityID: 18, Entity: &models.Resource{ID: 18, Name: "3", Hash: "SECRETHASH18"}, SimilarityDistance: &d3},
	}}
	executor := func(c context.Context, _ string, _ shortcodes.QueryOptions) (*shortcodes.QueryResult, error) {
		return p.QueryResult(c, raw)
	}
	target, err := p.Entity(ctx, &models.Resource{ID: 42, Name: "SECRET_TARGET_NAME"})
	if err != nil {
		t.Fatal(err)
	}
	out := shortcodes.Process(ctx, "[similar]", shortcodes.MetaShortcodeContext{EntityType: "resource", EntityID: 42, Entity: target}, nil, executor)

	for _, secret := range []string{"SECRET_TWIN_NAME", "SECRET_TARGET_NAME", "SECRETHASH17", "SECRETHASH18"} {
		if strings.Contains(out, secret) {
			t.Errorf("%q survived masking: %s", secret, out)
		}
	}
	for _, want := range []string{"Duplicate · 1", "Similar · 3", "near-certain duplicate, distance 1", "similar-strip"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q: %s", want, out)
		}
	}
	if raw.Items[0].SimilarityDistance != &d1 {
		t.Error("masking must not touch the input")
	}
	payload, _ := json.Marshal(map[string]string{"html": out})
	if err := p.CheckOutbound(ctx, payload); err != nil {
		t.Fatalf("the masked strip tripped the guard: %v", err)
	}
}
