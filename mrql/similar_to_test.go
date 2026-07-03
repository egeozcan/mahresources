package mrql

import (
	"slices"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// Package 3: Similarity Search — SIMILAR TO resource(N) [WITHIN d] + ORDER BY distance.
//
// Pairs are seeded directly into resource_similarities (r1 < r2), mirroring
// what the hash worker writes. Effective distance = COALESCE(p_distance,
// hamming_distance).
//
// Seed pairs (on top of setupTestDB resources 1=sunset.jpg, 2=photo_album.png,
// 3=report.pdf, 4=untagged_file.txt):
//   (1,2) hamming=3  p=2   a=1    → effective 2
//   (1,3) hamming=8  p=nil a=nil  → effective 8 (legacy pair)
//   (2,4) hamming=0  p=11  a=9    → effective 11
//   (3,4) hamming=5  p=5   a=7    → effective 5

type testResourceSimilarity struct {
	ID              uint `gorm:"primarykey"`
	ResourceID1     uint
	ResourceID2     uint
	HammingDistance uint8
	PDistance       *uint8
	ADistance       *uint8
}

func (testResourceSimilarity) TableName() string { return "resource_similarities" }

func u8(v uint8) *uint8 { return &v }

func setupSimilarityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := setupTestDB(t)
	if err := db.AutoMigrate(&testResourceSimilarity{}); err != nil {
		t.Fatalf("auto-migrate resource_similarities failed: %v", err)
	}
	pairs := []testResourceSimilarity{
		{ResourceID1: 1, ResourceID2: 2, HammingDistance: 3, PDistance: u8(2), ADistance: u8(1)},
		{ResourceID1: 1, ResourceID2: 3, HammingDistance: 8},
		{ResourceID1: 2, ResourceID2: 4, HammingDistance: 0, PDistance: u8(11), ADistance: u8(9)},
		{ResourceID1: 3, ResourceID2: 4, HammingDistance: 5, PDistance: u8(5), ADistance: u8(7)},
	}
	if err := db.Create(&pairs).Error; err != nil {
		t.Fatalf("seed pairs failed: %v", err)
	}
	return db
}

// similarResourceIDs parses, validates, and executes a query for the resource
// entity with the given options, returning sorted result IDs.
func similarResourceIDs(t *testing.T, db *gorm.DB, input string, opts TranslateOptions) []uint {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error for %q: %v", input, err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error for %q: %v", input, err)
	}
	result, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate error for %q: %v", input, err)
	}
	var rows []testResource
	if err := result.Find(&rows).Error; err != nil {
		t.Fatalf("query error for %q: %v", input, err)
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	slices.Sort(ids)
	return ids
}

// orderedSimilarResourceIDs is similarResourceIDs without the sort (for ORDER BY tests).
func orderedSimilarResourceIDs(t *testing.T, db *gorm.DB, input string, opts TranslateOptions) []uint {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error for %q: %v", input, err)
	}
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validation error for %q: %v", input, err)
	}
	result, err := TranslateWithOptions(q, db, opts)
	if err != nil {
		t.Fatalf("translate error for %q: %v", input, err)
	}
	var rows []testResource
	if err := result.Find(&rows).Error; err != nil {
		t.Fatalf("query error for %q: %v", input, err)
	}
	ids := make([]uint, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	return ids
}

func intPtr(v int) *int { return &v }

// --- Lexer ---

func TestSimilarToLexing(t *testing.T) {
	// SIMILAR followed by TO merges into one token (ORDER BY precedent).
	lx := NewLexer(`SIMILAR TO resource(5)`)
	tok := lx.Next()
	if tok.Type != TokenSimilarTo || tok.Value != "SIMILAR TO" {
		t.Fatalf("expected merged SIMILAR TO token, got %v", tok)
	}
	// Case-insensitive merge.
	lx = NewLexer(`similar to resource(5)`)
	if tok := lx.Next(); tok.Type != TokenSimilarTo {
		t.Fatalf("expected merged similar to token, got %v", tok)
	}
	// SIMILAR alone stays a plain identifier (usable as a field name).
	lx = NewLexer(`similar = "x"`)
	if tok := lx.Next(); tok.Type != TokenIdentifier || tok.Value != "similar" {
		t.Fatalf("expected identifier 'similar', got %v", tok)
	}
	// WITHIN is not a keyword: usable as a field / meta key.
	lx = NewLexer(`within`)
	if tok := lx.Next(); tok.Type != TokenIdentifier {
		t.Fatalf("expected identifier 'within', got %v", tok)
	}
}

// --- Parser ---

func TestSimilarToParsing(t *testing.T) {
	q, err := Parse(`type = "resource" AND SIMILAR TO resource(1234)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	and, ok := q.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr root, got %T", q.Where)
	}
	sim, ok := and.Right.(*SimilarToExpr)
	if !ok {
		t.Fatalf("expected SimilarToExpr, got %T", and.Right)
	}
	if sim.TargetID != 1234 {
		t.Errorf("TargetID = %d, want 1234", sim.TargetID)
	}
	if sim.Within != -1 {
		t.Errorf("Within = %d, want -1 (absent)", sim.Within)
	}

	q, err = Parse(`SIMILAR TO resource(7) WITHIN 5`)
	if err != nil {
		t.Fatalf("parse with WITHIN: %v", err)
	}
	sim, ok = q.Where.(*SimilarToExpr)
	if !ok {
		t.Fatalf("expected SimilarToExpr root, got %T", q.Where)
	}
	if sim.TargetID != 7 || sim.Within != 5 {
		t.Errorf("got TargetID=%d Within=%d, want 7/5", sim.TargetID, sim.Within)
	}

	// Composes under NOT and parentheses.
	q, err = Parse(`NOT (SIMILAR TO resource(1) OR SIMILAR TO resource(2))`)
	if err != nil {
		t.Fatalf("parse NOT composition: %v", err)
	}
	if _, ok := q.Where.(*NotExpr); !ok {
		t.Fatalf("expected NotExpr root, got %T", q.Where)
	}

	// "similar" and "within" remain usable as plain fields.
	if _, err := Parse(`similar = "x" AND meta.within = 3`); err != nil {
		t.Fatalf("similar/within as identifiers should parse: %v", err)
	}
}

func TestSimilarToParseErrors(t *testing.T) {
	cases := []struct {
		name  string
		query string
		want  string
	}{
		{"wrong entity kind", `SIMILAR TO note(5)`, "resource("},
		{"missing paren", `SIMILAR TO resource 5`, "("},
		{"missing id", `SIMILAR TO resource()`, "resource ID"},
		{"string id", `SIMILAR TO resource("x")`, "resource ID"},
		{"missing close", `SIMILAR TO resource(5`, ")"},
		{"WITHIN without value", `SIMILAR TO resource(5) WITHIN`, "WITHIN"},
		{"WITHIN string", `SIMILAR TO resource(5) WITHIN "far"`, "WITHIN"},
		// -1 is the internal "no WITHIN" sentinel — a negative literal must not
		// silently mean "use the default threshold".
		{"WITHIN negative", `SIMILAR TO resource(5) WITHIN -1`, "WITHIN"},
		{"bare SIMILAR TO", `SIMILAR TO`, "resource("},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.query)
			if err == nil {
				t.Fatalf("expected parse error for %q", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error for %q should mention %q, got: %v", tc.query, tc.want, err)
			}
		})
	}
}

// --- Validator ---

func TestSimilarToValidation(t *testing.T) {
	valid := []struct {
		query      string
		entityType EntityType
	}{
		{`type = "resource" AND SIMILAR TO resource(1)`, EntityResource},
		{`type = "resource" AND SIMILAR TO resource(1) WITHIN 0`, EntityResource},
		{`type = "resource" AND SIMILAR TO resource(1) WITHIN 11`, EntityResource},
		{`type = "resource" AND SIMILAR TO resource(1) ORDER BY distance ASC`, EntityResource},
		{`type = "resource" AND SIMILAR TO resource(1) ORDER BY distance DESC LIMIT 5`, EntityResource},
		{`type = "resource" AND NOT SIMILAR TO resource(1) ORDER BY distance`, EntityResource},
		// Type-guarded OR branch validates per-branch.
		{`(type = "resource" AND SIMILAR TO resource(1)) OR (type = "group" AND name = "x")`, EntityUnspecified},
	}
	for _, tc := range valid {
		q, err := Parse(tc.query)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.query, err)
		}
		q.EntityType = tc.entityType
		if err := Validate(q); err != nil {
			t.Errorf("query %q: unexpected validation error: %v", tc.query, err)
		}
	}

	invalid := []struct {
		name       string
		query      string
		entityType EntityType
		want       string
	}{
		{"note entity", `type = "note" AND SIMILAR TO resource(1)`, EntityNote, "type = \"resource\""},
		{"group entity", `type = "group" AND SIMILAR TO resource(1)`, EntityGroup, "type = \"resource\""},
		{"cross-entity unguarded", `SIMILAR TO resource(1)`, EntityUnspecified, "type = \"resource\""},
		{"zero id", `type = "resource" AND SIMILAR TO resource(0)`, EntityResource, "positive"},
		{"WITHIN too large", `type = "resource" AND SIMILAR TO resource(1) WITHIN 12`, EntityResource, "11"},
		{"ORDER BY distance without SIMILAR TO", `type = "resource" AND name = "x" ORDER BY distance`, EntityResource, "SIMILAR TO"},
		{"ORDER BY distance two targets", `type = "resource" AND (SIMILAR TO resource(1) OR SIMILAR TO resource(2)) ORDER BY distance`, EntityResource, "exactly one"},
		{"ORDER BY distance cross-entity", `(type = "resource" AND SIMILAR TO resource(1)) OR (type = "group" AND name = "x") ORDER BY distance`, EntityUnspecified, "distance"},
		{"ORDER BY distance with GROUP BY", `type = "resource" AND SIMILAR TO resource(1) GROUP BY contentType COUNT() ORDER BY distance`, EntityResource, "distance"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				return // parse-level rejection is acceptable
			}
			q.EntityType = tc.entityType
			err = Validate(q)
			if err == nil {
				t.Fatalf("query %q: expected validation error", tc.query)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error for %q should mention %q, got: %v", tc.query, tc.want, err)
			}
		})
	}
}

// --- Translator: SQL shapes ---

func TestSimilarToSQLShapes(t *testing.T) {
	db := setupSimilarityTestDB(t)

	sql := dryRunSQL(t, db, `SIMILAR TO resource(1)`, EntityResource)
	for _, want := range []string{
		"resources.id IN (",
		"SELECT rs.resource_id2 FROM resource_similarities rs WHERE rs.resource_id1 = 1",
		"UNION ALL",
		"SELECT rs.resource_id1 FROM resource_similarities rs WHERE rs.resource_id2 = 1",
		"COALESCE(rs.p_distance, rs.hamming_distance) <= 10",
	} {
		if !strings.Contains(sql, want) {
			t.Errorf("SQL missing %q:\n%s", want, sql)
		}
	}
	// No aHash clause when the threshold is 0/unset.
	if strings.Contains(sql, "a_distance") {
		t.Errorf("SQL should not filter a_distance when disabled:\n%s", sql)
	}
}

// --- Translator: execution ---

func TestSimilarToExecution(t *testing.T) {
	db := setupSimilarityTestDB(t)

	cases := []struct {
		name  string
		query string
		opts  TranslateOptions
		want  []uint
	}{
		{"default threshold 10", `SIMILAR TO resource(1)`, TranslateOptions{}, []uint{2, 3}},
		{"WITHIN 5", `SIMILAR TO resource(1) WITHIN 5`, TranslateOptions{}, []uint{2}},
		{"WITHIN 11 both directions", `SIMILAR TO resource(4) WITHIN 11`, TranslateOptions{}, []uint{2, 3}},
		{"default excludes distance 11", `SIMILAR TO resource(2)`, TranslateOptions{}, []uint{1}},
		{"direction coverage r2", `SIMILAR TO resource(2) WITHIN 11`, TranslateOptions{}, []uint{1, 4}},
		{"nonexistent target", `SIMILAR TO resource(999)`, TranslateOptions{}, []uint{}},
		{"option threshold", `SIMILAR TO resource(1)`, TranslateOptions{SimilarityThreshold: intPtr(2)}, []uint{2}},
		{"WITHIN overrides option", `SIMILAR TO resource(1) WITHIN 8`, TranslateOptions{SimilarityThreshold: intPtr(2)}, []uint{2, 3}},
		// aHash filter: legacy NULL a_distance always passes.
		{"aHash filter", `SIMILAR TO resource(4) WITHIN 11`, TranslateOptions{AHashThreshold: 8}, []uint{3}},
		{"aHash passes NULL", `SIMILAR TO resource(1)`, TranslateOptions{AHashThreshold: 1}, []uint{2, 3}},
		// Composition.
		{"AND composition", `SIMILAR TO resource(1) AND contentType ~ "image/*"`, TranslateOptions{}, []uint{2}},
		{"OR composition", `SIMILAR TO resource(1) WITHIN 5 OR name = "report.pdf"`, TranslateOptions{}, []uint{2, 3}},
		{"NOT similar", `NOT SIMILAR TO resource(1)`, TranslateOptions{}, []uint{1, 4}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := similarResourceIDs(t, db, tc.query, tc.opts)
			if !slices.Equal(got, tc.want) {
				t.Errorf("query %q: got %v want %v", tc.query, got, tc.want)
			}
		})
	}
}

func TestSimilarToOrderByDistance(t *testing.T) {
	db := setupSimilarityTestDB(t)

	// Distances from resource 1: r2 → 2, r3 → 8.
	got := orderedSimilarResourceIDs(t, db, `SIMILAR TO resource(1) ORDER BY distance ASC`, TranslateOptions{})
	if !slices.Equal(got, []uint{2, 3}) {
		t.Errorf("ASC: got %v want [2 3]", got)
	}
	got = orderedSimilarResourceIDs(t, db, `SIMILAR TO resource(1) ORDER BY distance DESC`, TranslateOptions{})
	if !slices.Equal(got, []uint{3, 2}) {
		t.Errorf("DESC: got %v want [3 2]", got)
	}
	// Pairless rows (matched via the OR branch) sort last in ASC order.
	got = orderedSimilarResourceIDs(t, db, `SIMILAR TO resource(1) WITHIN 5 OR name = "untagged_file.txt" ORDER BY distance ASC`, TranslateOptions{})
	if !slices.Equal(got, []uint{2, 4}) {
		t.Errorf("pairless-last: got %v want [2 4]", got)
	}
}

// --- Cross-entity: type-guarded OR clones ---

func TestSimilarToCrossEntityTypeGuardedOr(t *testing.T) {
	db := setupSimilarityTestDB(t)

	query := `(type = "resource" AND SIMILAR TO resource(1) WITHIN 5) OR (type = "group" AND name = "Vacation")`
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}

	// Resource clone: similarity branch live, group branch FALSE.
	cloneR := *q
	cloneR.EntityType = EntityResource
	result, err := TranslateWithOptions(&cloneR, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate resource clone: %v", err)
	}
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("resource query: %v", err)
	}
	if len(resources) != 1 || resources[0].ID != 2 {
		t.Errorf("resource clone: got %v, want just ID 2", resources)
	}

	// Group clone: SIMILAR TO must become FALSE, not a TranslateError.
	cloneG := *q
	cloneG.EntityType = EntityGroup
	result, err = TranslateWithOptions(&cloneG, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate group clone: %v", err)
	}
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("group query: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Vacation" {
		t.Errorf("group clone: got %v, want just Vacation", groups)
	}
}

// --- Completer ---

func TestSimilarToCompletion(t *testing.T) {
	// Offered at field position for resource-typed queries.
	prefix := `type = "resource" AND `
	suggestions := Complete(prefix, len(prefix))
	if !hasSuggestion(suggestions, "SIMILAR TO resource(") {
		t.Errorf("resource query: expected SIMILAR TO suggestion, got %v", suggestions)
	}
	// Not offered for note/group or untyped queries.
	for _, p := range []string{`type = "note" AND `, `type = "group" AND `, `name ~ "a" AND `} {
		if hasSuggestion(Complete(p, len(p)), "SIMILAR TO resource(") {
			t.Errorf("%q: SIMILAR TO should not be suggested", p)
		}
	}
	// After the closing paren, WITHIN is offered alongside the usual keywords.
	q := `type = "resource" AND SIMILAR TO resource(5) `
	suggestions = Complete(q, len(q))
	if !hasSuggestion(suggestions, "WITHIN") {
		t.Errorf("after resource(5): expected WITHIN suggestion, got %v", suggestions)
	}
	if !hasSuggestion(suggestions, "AND") {
		t.Errorf("after resource(5): expected AND suggestion, got %v", suggestions)
	}
	// ORDER BY offers distance when the query has a SIMILAR TO.
	q = `type = "resource" AND SIMILAR TO resource(5) ORDER BY `
	suggestions = Complete(q, len(q))
	if !hasSuggestion(suggestions, "distance") {
		t.Errorf("ORDER BY after SIMILAR TO: expected distance suggestion, got %v", suggestions)
	}
	q = `type = "resource" AND name = "x" ORDER BY `
	suggestions = Complete(q, len(q))
	if hasSuggestion(suggestions, "distance") {
		t.Errorf("ORDER BY without SIMILAR TO: distance should not be suggested, got %v", suggestions)
	}
}

// --- Generation lint ---

func TestSimilarToGenerationLint(t *testing.T) {
	// A well-formed generated query passes.
	q, err := Parse(`type = "resource" AND SIMILAR TO resource(42) WITHIN 3 LIMIT 50`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if errs := LintGeneratedQuery(q); len(errs) != 0 {
		t.Errorf("expected no lint errors, got %v", errs)
	}
}
