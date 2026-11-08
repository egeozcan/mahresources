package application_context

import (
	"context"
	"errors"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"
)

// The masked copy never aliases the original's JSON bytes, whatever the early
// return (round 7, #3).
func TestMaskedJSONNeverAliasesTheOriginal(t *testing.T) {
	p := NewTemplatePrivacy("al")
	whitespace := &models.Group{Meta: types.JSON("    ")}
	out, err := p.Entity(context.Background(), whitespace)
	if err != nil {
		t.Fatal(err)
	}
	out.(*models.Group).Meta[0] = 'x'
	if string(whitespace.Meta) != "    " {
		t.Fatalf("whitespace-only Meta aliases the original: %q", whitespace.Meta)
	}

	spare := make(types.JSON, 0, 16)
	empty := &models.Group{Meta: spare}
	out, _ = p.Entity(context.Background(), empty)
	masked := out.(*models.Group).Meta
	masked = append(masked, '{', '}')
	if string(spare[:2]) == "{}" {
		t.Fatalf("an empty slice with spare capacity shares its backing array with the copy")
	}
	_ = masked

	item := &shortcodes.QueryResult{Items: []shortcodes.QueryResultItem{{Meta: []byte("  \n ")}}}
	res, err := p.QueryResult(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	res.Items[0].Meta[0] = 'x'
	if string(item.Items[0].Meta) != "  \n " {
		t.Fatalf("a query item's whitespace Meta aliases the cached one: %q", item.Items[0].Meta)
	}
}

// A pointer or interface cell, as GORM's map scan hands back for an SQL
// expression column, is unwrapped into a detached masked value (round 7, #2).
func TestQueryCellsThroughPointersAreMaskedAndDetached(t *testing.T) {
	p := NewTemplatePrivacy("pc")
	var cell any = "PROT_REGION_EAST"
	ptr := &cell
	inner := map[string]any{"deep": &cell}
	in := &shortcodes.QueryResult{Mode: "bucketed", Groups: []shortcodes.QueryResultGroup{{Key: map[string]any{"region": ptr, "nested": inner, "n": 7}}}}
	out, err := p.QueryResult(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	got := out.Groups[0].Key["region"]
	if s, ok := got.(string); !ok || !strings.HasPrefix(s, "⟦") {
		t.Fatalf("a pointer cell must be masked, got %#v", got)
	}
	if deep := out.Groups[0].Key["nested"].(map[string]any)["deep"]; !strings.HasPrefix(deep.(string), "⟦") {
		t.Fatalf("a nested pointer cell must be masked, got %#v", deep)
	}
	if in.Groups[0].Key["region"] != ptr || cell != "PROT_REGION_EAST" {
		t.Fatal("the cached input must be untouched")
	}
	odd := &shortcodes.QueryResult{Rows: []map[string]any{{"x": struct{ Secret string }{"PROT_ODD_REPRESENTATION"}}}}
	out, _ = p.QueryResult(context.Background(), odd)
	if s, ok := out.Rows[0]["x"].(string); !ok || strings.Contains(s, "PROT_ODD") {
		t.Fatalf("an unsupported representation fails closed (masked text): %#v", out.Rows[0]["x"])
	}
}

// The Seed overflow keeps both identities (round 7, #4).
func TestSeedOverflowKeepsBothIdentities(t *testing.T) {
	p := NewTemplatePrivacy("so")
	p.Text(strings.Repeat("x", templatePrivacyMaxValueBytes+1))
	err := privacySeedError(TemplateGenerationInput{Privacy: p})
	if !errors.Is(err, ErrTemplateGenerationBadRequest) || !errors.Is(err, ErrTemplatePrivacyLimit) {
		t.Fatalf("both identities must survive: %v", err)
	}
}
