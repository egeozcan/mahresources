package mrql

import (
	"strings"
	"testing"
)

func fingerprintQuery(t *testing.T, source string, scope ScopeShape) string {
	t.Helper()
	q, err := Parse(source)
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate %q: %v", source, err)
	}
	return QueryShapeFingerprint(q, scope)
}

func TestQueryShapeFingerprintRedactsValues(t *testing.T) {
	first := fingerprintQuery(t, `type = "resource" AND name ~ "top-secret-a" AND fileSize > 100 LIMIT 10 OFFSET 20`, ScopeShapeNone)
	second := fingerprintQuery(t, `type = "resource" AND name ~ "top-secret-b" AND fileSize > 900 LIMIT 10 OFFSET 20`, ScopeShapeNone)

	if first != second {
		t.Fatalf("literal values must not change the shape fingerprint:\n%s\n%s", first, second)
	}
	if strings.Contains(first, "top-secret") {
		t.Fatalf("fingerprint leaked a literal: %s", first)
	}
	if !strings.HasPrefix(first, QueryShapeFingerprintVersion+":") {
		t.Fatalf("fingerprint %q lacks version prefix %q", first, QueryShapeFingerprintVersion)
	}
}

func TestQueryShapeFingerprintPreservesEffectiveStructure(t *testing.T) {
	base := fingerprintQuery(t, `type = "resource" AND name ~ "x" LIMIT 10 OFFSET 20`, ScopeShapeNone)
	cases := map[string]string{
		"operator": fingerprintQuery(t, `type = "resource" AND name = "x" LIMIT 10 OFFSET 20`, ScopeShapeNone),
		"entity":   fingerprintQuery(t, `type = "note" AND name ~ "x" LIMIT 10 OFFSET 20`, ScopeShapeNone),
		"limit":    fingerprintQuery(t, `type = "resource" AND name ~ "x" LIMIT 11 OFFSET 20`, ScopeShapeNone),
		"offset":   fingerprintQuery(t, `type = "resource" AND name ~ "x" LIMIT 10 OFFSET 21`, ScopeShapeNone),
		"scope":    fingerprintQuery(t, `type = "resource" AND name ~ "x" LIMIT 10 OFFSET 20`, ScopeShapeForced),
	}
	for name, got := range cases {
		if got == base {
			t.Errorf("%s change did not change fingerprint", name)
		}
	}
}

func TestQueryShapeFingerprintIncludesSQLShapingPolicy(t *testing.T) {
	q, err := Parse(`type = "resource" AND SIMILAR TO resource(1)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(q); err != nil {
		t.Fatal(err)
	}
	base := QueryShapeFingerprintWithPolicy(q, ScopeShapeNone, QueryShapePolicy{Dialect: "sqlite", SimilarityThreshold: 10, AHashThreshold: 5, FTSAvailable: true})
	cases := []QueryShapePolicy{
		{Dialect: "postgres", SimilarityThreshold: 10, AHashThreshold: 5, FTSAvailable: true},
		{Dialect: "sqlite", SimilarityThreshold: 11, AHashThreshold: 5, FTSAvailable: true},
		{Dialect: "sqlite", SimilarityThreshold: 10, AHashThreshold: 0, FTSAvailable: true},
		{Dialect: "sqlite", SimilarityThreshold: 10, AHashThreshold: 5, FTSAvailable: false},
	}
	for _, policy := range cases {
		if got := QueryShapeFingerprintWithPolicy(q, ScopeShapeNone, policy); got == base {
			t.Errorf("policy %#v did not change fingerprint", policy)
		}
	}
}

func TestQueryShapeFingerprintIgnoresSourceFormattingAndPositions(t *testing.T) {
	first := fingerprintQuery(t, `type="resource" AND name~"one"`, ScopeShapeExplicit)
	second := fingerprintQuery(t, "  type = \"resource\"\nAND name ~ \"two\"  ", ScopeShapeExplicit)
	if first != second {
		t.Fatalf("source formatting/token positions changed fingerprint:\n%s\n%s", first, second)
	}
}

func TestQueryShapeFingerprintRetainsShapeChangingBooleanAndTypeValues(t *testing.T) {
	shared := fingerprintQuery(t, `type = "note" AND shared = true`, ScopeShapeNone)
	notShared := fingerprintQuery(t, `type = "note" AND shared = false`, ScopeShapeNone)
	if shared == notShared {
		t.Fatal("boolean polarity changes generated SQL shape")
	}

	resources := fingerprintQuery(t, `type = "resource"`, ScopeShapeNone)
	notes := fingerprintQuery(t, `type = "note"`, ScopeShapeNone)
	if resources == notes {
		t.Fatal("entity type changes execution shape")
	}
}
