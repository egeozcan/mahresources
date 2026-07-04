//go:build postgres

package mrql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

func regexResources(t *testing.T, db *gorm.DB, query string) []testResource {
	t.Helper()
	result := parseAndTranslate(t, query, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query %q error: %v", query, err)
	}
	return resources
}

// TestRegexMatchPG: ~* matches case-insensitively against a POSIX regex.
func TestRegexMatchPG(t *testing.T) {
	db := setupPostgresTestDB(t)

	// names: sunset.jpg, photo_album.png, report.pdf, untagged_file.txt
	got := regexResources(t, db, `type = "resource" AND name ~* "^photo_.*\.png$"`)
	if len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("expected only photo_album.png, got %+v", got)
	}
}

// TestRegexCaseInsensitivePG: ~* ignores case (unlike PG's own ~).
func TestRegexCaseInsensitivePG(t *testing.T) {
	db := setupPostgresTestDB(t)
	got := regexResources(t, db, `type = "resource" AND name ~* "SUNSET"`)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected sunset.jpg via case-insensitive match, got %+v", got)
	}
}

// TestRegexNegationPG: !~* excludes matching rows.
func TestRegexNegationPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	got := regexResources(t, db, `type = "resource" AND name !~* "\.(png|jpg)$"`)
	// Excludes sunset.jpg and photo_album.png → report.pdf + untagged_file.txt.
	if len(got) != 2 {
		t.Fatalf("expected 2 non-image names, got %d (%+v)", len(got), got)
	}
	for _, r := range got {
		if r.ID == 1 || r.ID == 2 {
			t.Fatalf("!~* should exclude image files, got %+v", got)
		}
	}
}

// TestRegexMetaFieldPG: ~* applies to a text-extracted meta value.
func TestRegexMetaFieldPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	// note 1 meta.priority = "high", note 2 = "low".
	result := parseAndTranslate(t, `type = "note" AND meta.priority ~* "^hi"`, EntityNote, db)
	var notes []testNote
	if err := result.Find(&notes).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(notes) != 1 || notes[0].ID != 1 {
		t.Fatalf("expected note 1 (priority high), got %+v", notes)
	}
}

// TestRegexTraversalLeafPG: ~* on an owner.name traversal leaf.
func TestRegexTraversalLeafPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	// resource 1 owner = Vacation, resource 3 owner = Work.
	got := regexResources(t, db, `type = "resource" AND owner.name ~* "^vac"`)
	if len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("expected resource 1 (owner Vacation), got %+v", got)
	}
}

// TestRegexParamBoundPG: a $param pattern binds through to ~*.
func TestRegexParamBoundPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	q, err := Parse(`type = "resource" AND name ~* $pat`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if err := BindParams(q, map[string]any{"pat": "report"}); err != nil {
		t.Fatalf("bind error: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate error: %v", err)
	}
	q.EntityType = EntityResource
	result, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].ID != 3 {
		t.Fatalf("expected report.pdf via param pattern, got %+v", resources)
	}
}

// TestRegexInvalidPatternPG surfaces a PG "invalid regular expression" error
// (SQLSTATE 2201B) as a query-execution error rather than succeeding.
func TestRegexInvalidPatternPG(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `type = "resource" AND name ~* "["`, EntityResource, db)
	var resources []testResource
	err := result.Find(&resources).Error
	if err == nil {
		t.Fatalf("expected an invalid-regex execution error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "invalid regular expression") {
		t.Fatalf("expected 'invalid regular expression' error, got %v", err)
	}
}
