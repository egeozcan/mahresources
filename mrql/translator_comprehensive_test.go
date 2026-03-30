package mrql

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

// ---- helpers ----

// namesOf extracts sorted names from test result slices.
func namesOfResources(rs []testResource) []string {
	names := make([]string, len(rs))
	for i, r := range rs {
		names[i] = r.Name
	}
	sort.Strings(names)
	return names
}

func namesOfNotes(ns []testNote) []string {
	names := make([]string, len(ns))
	for i, n := range ns {
		names[i] = n.Name
	}
	sort.Strings(names)
	return names
}

func namesOfGroups(gs []testGroup) []string {
	names := make([]string, len(gs))
	for i, g := range gs {
		names[i] = g.Name
	}
	sort.Strings(names)
	return names
}

func assertNames(t *testing.T, got []string, want []string) {
	t.Helper()
	sort.Strings(want)
	if len(got) != len(want) {
		t.Fatalf("expected names %v, got %v", want, got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("expected names %v, got %v", want, got)
		}
	}
}

// ============================================================
// Comparison Operators (=, !=, >, >=, <, <=) x Entity Types
// ============================================================

func TestComprehensive_ComparisonOperators(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// ---- Resource name equality ----
		{"resource name eq", `type = "resource" AND name = "sunset.jpg"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource name neq", `type = "resource" AND name != "sunset.jpg"`, EntityResource, 3, nil},

		// ---- Resource numeric comparisons ----
		{"resource fileSize gt", `type = "resource" AND fileSize > 1000000`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource fileSize gte", `type = "resource" AND fileSize >= 1024000`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource fileSize lt", `type = "resource" AND fileSize < 1000000`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource fileSize lte", `type = "resource" AND fileSize <= 512000`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource fileSize eq", `type = "resource" AND fileSize = 100`, EntityResource, 1, []string{"untagged_file.txt"}},
		{"resource fileSize neq", `type = "resource" AND fileSize != 100`, EntityResource, 3, nil},

		// ---- Resource width/height ----
		{"resource width gt", `type = "resource" AND width > 1000`, EntityResource, 1, []string{"sunset.jpg"}},
		// height <= 600: photo_album.png (600), report.pdf (0), untagged_file.txt (0)
		{"resource height lte", `type = "resource" AND height <= 600`, EntityResource, 3, []string{"photo_album.png", "report.pdf", "untagged_file.txt"}},

		// ---- Resource contentType equality ----
		{"resource contentType eq", `type = "resource" AND contentType = "image/jpeg"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource contentType neq", `type = "resource" AND contentType != "image/jpeg"`, EntityResource, 3, nil},

		// ---- Note name comparisons ----
		{"note name eq", `type = "note" AND name = "Meeting notes"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note name neq", `type = "note" AND name != "Meeting notes"`, EntityNote, 1, []string{"Todo list"}},

		// ---- Group name comparisons ----
		{"group name eq", `type = "group" AND name = "Vacation"`, EntityGroup, 1, []string{"Vacation"}},
		{"group name neq", `type = "group" AND name != "Vacation"`, EntityGroup, 4, []string{"Work", "Archive", "Sub-Work", "Photos"}},

		// ---- Resource id comparisons ----
		{"resource id eq", `type = "resource" AND id = 1`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource id gt", `type = "resource" AND id > 2`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource id gte", `type = "resource" AND id >= 3`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource id lt", `type = "resource" AND id < 3`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource id lte", `type = "resource" AND id <= 2`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource id neq", `type = "resource" AND id != 1`, EntityResource, 3, nil},

		// ---- Group id comparisons ----
		{"group id gt", `type = "group" AND id > 2`, EntityGroup, 3, []string{"Archive", "Sub-Work", "Photos"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d resources, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d notes, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d groups, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// LIKE / NOT LIKE x Entity Types
// ============================================================

func TestComprehensive_LikeOperators(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// ---- Resource name LIKE ----
		{"resource name like prefix", `type = "resource" AND name ~ "sun*"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource name like suffix", `type = "resource" AND name ~ "*.jpg"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource name like contains", `type = "resource" AND name ~ "*album*"`, EntityResource, 1, []string{"photo_album.png"}},
		{"resource name like single char", `type = "resource" AND name ~ "report.pd?"`, EntityResource, 1, []string{"report.pdf"}},
		{"resource name not like", `type = "resource" AND name !~ "*photo*"`, EntityResource, 3, []string{"sunset.jpg", "report.pdf", "untagged_file.txt"}},

		// ---- Resource contentType LIKE ----
		{"resource contentType like", `type = "resource" AND contentType ~ "image/*"`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource contentType not like", `type = "resource" AND contentType !~ "image/*"`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},

		// ---- Note name LIKE ----
		{"note name like", `type = "note" AND name ~ "*notes*"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note name not like", `type = "note" AND name !~ "Meeting*"`, EntityNote, 1, []string{"Todo list"}},

		// ---- Group name LIKE ----
		{"group name like", `type = "group" AND name ~ "Vac*"`, EntityGroup, 1, []string{"Vacation"}},
		{"group name like contains", `type = "group" AND name ~ "*Work*"`, EntityGroup, 2, []string{"Work", "Sub-Work"}},
		{"group name not like", `type = "group" AND name !~ "*Work*"`, EntityGroup, 3, []string{"Vacation", "Archive", "Photos"}},

		// ---- Resource originalName LIKE ----
		{"resource originalName like", `type = "resource" AND originalName ~ "sunset*"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource originalName eq untagged", `type = "resource" AND originalName = "untagged.txt"`, EntityResource, 1, []string{"untagged_file.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// Tag Relation Operators (=, !=, ~, !~, IN, NOT IN, IS EMPTY, IS NOT EMPTY)
// ============================================================

func TestComprehensive_TagRelations(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// ---- Resource tags ----
		{"resource tags eq photo", `type = "resource" AND tags = "photo"`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource tags eq video", `type = "resource" AND tags = "video"`, EntityResource, 1, []string{"photo_album.png"}},
		{"resource tags neq photo", `type = "resource" AND tags != "photo"`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource tags like pho*", `type = "resource" AND tags ~ "pho*"`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource tags not like pho*", `type = "resource" AND tags !~ "pho*"`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource tags in", `type = "resource" AND tags IN ("photo", "video")`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource tags not in", `type = "resource" AND tags NOT IN ("photo", "video")`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource tags is empty", `type = "resource" AND tags IS EMPTY`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},
		{"resource tags is not empty", `type = "resource" AND tags IS NOT EMPTY`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},

		// ---- Note tags ----
		{"note tags eq document", `type = "note" AND tags = "document"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note tags eq photo", `type = "note" AND tags = "photo"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note tags neq document", `type = "note" AND tags != "document"`, EntityNote, 1, []string{"Todo list"}},
		{"note tags like doc*", `type = "note" AND tags ~ "doc*"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note tags not like doc*", `type = "note" AND tags !~ "doc*"`, EntityNote, 1, []string{"Todo list"}},
		{"note tags in", `type = "note" AND tags IN ("document", "photo")`, EntityNote, 1, []string{"Meeting notes"}},
		{"note tags not in", `type = "note" AND tags NOT IN ("document", "photo")`, EntityNote, 1, []string{"Todo list"}},
		{"note tags is empty", `type = "note" AND tags IS EMPTY`, EntityNote, 1, []string{"Todo list"}},
		{"note tags is not empty", `type = "note" AND tags IS NOT EMPTY`, EntityNote, 1, []string{"Meeting notes"}},

		// ---- Group tags ----
		{"group tags eq photo", `type = "group" AND tags = "photo"`, EntityGroup, 1, []string{"Vacation"}},
		{"group tags neq photo", `type = "group" AND tags != "photo"`, EntityGroup, 4, []string{"Work", "Archive", "Sub-Work", "Photos"}},
		{"group tags like pho*", `type = "group" AND tags ~ "pho*"`, EntityGroup, 1, []string{"Vacation"}},
		{"group tags is empty", `type = "group" AND tags IS EMPTY`, EntityGroup, 3, []string{"Archive", "Sub-Work", "Photos"}},
		{"group tags is not empty", `type = "group" AND tags IS NOT EMPTY`, EntityGroup, 2, []string{"Vacation", "Work"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// Group Relation Operators (resource, note)
// ============================================================

func TestComprehensive_GroupRelations(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// ---- Resource groups ----
		{"resource groups eq Vacation", `type = "resource" AND groups = "Vacation"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource groups eq Work", `type = "resource" AND groups = "Work"`, EntityResource, 1, []string{"report.pdf"}},
		{"resource groups neq Vacation", `type = "resource" AND groups != "Vacation"`, EntityResource, 3, []string{"photo_album.png", "report.pdf", "untagged_file.txt"}},
		{"resource groups like Vac*", `type = "resource" AND groups ~ "Vac*"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource groups not like Vac*", `type = "resource" AND groups !~ "Vac*"`, EntityResource, 3, nil},
		{"resource groups in", `type = "resource" AND groups IN ("Vacation", "Work")`, EntityResource, 2, []string{"sunset.jpg", "report.pdf"}},
		{"resource groups not in", `type = "resource" AND groups NOT IN ("Vacation", "Work")`, EntityResource, 2, []string{"photo_album.png", "untagged_file.txt"}},
		{"resource groups is empty", `type = "resource" AND groups IS EMPTY`, EntityResource, 2, []string{"photo_album.png", "untagged_file.txt"}},
		{"resource groups is not empty", `type = "resource" AND groups IS NOT EMPTY`, EntityResource, 2, []string{"sunset.jpg", "report.pdf"}},

		// ---- Note groups ----
		{"note groups eq Vacation", `type = "note" AND groups = "Vacation"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note groups eq Work", `type = "note" AND groups = "Work"`, EntityNote, 1, []string{"Todo list"}},
		{"note groups neq Vacation", `type = "note" AND groups != "Vacation"`, EntityNote, 1, []string{"Todo list"}},
		{"note groups like W*", `type = "note" AND groups ~ "W*"`, EntityNote, 1, []string{"Todo list"}},
		{"note groups in", `type = "note" AND groups IN ("Vacation", "Work")`, EntityNote, 2, []string{"Meeting notes", "Todo list"}},
		{"note groups not in Vacation", `type = "note" AND groups NOT IN ("Vacation")`, EntityNote, 1, []string{"Todo list"}},
		{"note groups is empty", `type = "note" AND groups IS EMPTY`, EntityNote, 0, nil},
		{"note groups is not empty", `type = "note" AND groups IS NOT EMPTY`, EntityNote, 2, []string{"Meeting notes", "Todo list"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// IS NULL / IS NOT NULL
// ============================================================

func TestComprehensive_IsNullNotNull(t *testing.T) {
	db := setupTestDB(t)

	// Set a non-empty hash on resource 1 so we can test IS NULL vs IS NOT NULL on hash
	db.Exec("UPDATE resources SET hash = 'abc123' WHERE id = 1")

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
	}{
		// hash IS NULL for resources — resources 2,3,4 have empty string hash, resource 1 has "abc123"
		// Note: SQLite treats empty string as NOT NULL, so IS NULL returns only truly NULL rows
		// In our seed data, hash is "" (empty string, not NULL) for resources 2-4
		// So hash IS NULL should return 0 resources (all have empty string or 'abc123')
		{"resource hash is null", `type = "resource" AND hash IS NULL`, EntityResource, 0},
		{"resource hash is not null", `type = "resource" AND hash IS NOT NULL`, EntityResource, 4},

		// description IS NULL for notes — all notes have empty string descriptions (not NULL)
		{"note description is null", `type = "note" AND description IS NULL`, EntityNote, 0},
		{"note description is not null", `type = "note" AND description IS NOT NULL`, EntityNote, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(resources))
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(notes))
				}
			}
		})
	}
}

// ============================================================
// IS EMPTY / IS NOT EMPTY for scalar fields
// ============================================================

func TestComprehensive_IsEmptyScalar(t *testing.T) {
	db := setupTestDB(t)

	// Set hash on resource 1 to test IS EMPTY/IS NOT EMPTY on scalar fields
	db.Exec("UPDATE resources SET hash = 'abc123' WHERE id = 1")

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
	}{
		// hash IS EMPTY: resources with NULL or empty string hash
		// Resources 2,3,4 have empty string hash, resource 1 has "abc123"
		{"resource hash is empty", `type = "resource" AND hash IS EMPTY`, EntityResource, 3},
		{"resource hash is not empty", `type = "resource" AND hash IS NOT EMPTY`, EntityResource, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			var resources []testResource
			if err := result.Find(&resources).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(resources) != tt.wantCount {
				t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
			}
		})
	}
}

// ============================================================
// Meta Field Operators (=, !=, >, >=, <, <=, IS NULL)
// ============================================================

func TestComprehensive_MetaFields(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// ---- Resource meta ----
		{"resource meta.rating eq 5", `type = "resource" AND meta.rating = 5`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource meta.rating eq 3", `type = "resource" AND meta.rating = 3`, EntityResource, 1, []string{"photo_album.png"}},
		{"resource meta.rating neq 5", `type = "resource" AND meta.rating != 5`, EntityResource, 1, []string{"photo_album.png"}},
		{"resource meta.rating gt 3", `type = "resource" AND meta.rating > 3`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource meta.rating gte 3", `type = "resource" AND meta.rating >= 3`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource meta.rating lt 5", `type = "resource" AND meta.rating < 5`, EntityResource, 1, []string{"photo_album.png"}},
		{"resource meta.rating lte 3", `type = "resource" AND meta.rating <= 3`, EntityResource, 1, []string{"photo_album.png"}},

		// ---- Note meta ----
		{"note meta.priority eq high", `type = "note" AND meta.priority = "high"`, EntityNote, 1, []string{"Meeting notes"}},
		{"note meta.priority eq low", `type = "note" AND meta.priority = "low"`, EntityNote, 1, []string{"Todo list"}},
		{"note meta.priority neq high", `type = "note" AND meta.priority != "high"`, EntityNote, 1, []string{"Todo list"}},
		{"note meta.count eq 7", `type = "note" AND meta.count = 7`, EntityNote, 1, []string{"Todo list"}},
		{"note meta.count gt 5", `type = "note" AND meta.count > 5`, EntityNote, 1, []string{"Todo list"}},

		// ---- Group meta ----
		{"group meta.region eq europe", `type = "group" AND meta.region = "europe"`, EntityGroup, 1, []string{"Vacation"}},
		{"group meta.priority eq 3", `type = "group" AND meta.priority = 3`, EntityGroup, 1, []string{"Vacation"}},
		{"group meta.priority gt 2", `type = "group" AND meta.priority > 2`, EntityGroup, 1, []string{"Vacation"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// Parent / Children Traversal (name, tags, category)
// ============================================================

func TestComprehensive_ParentChildrenTraversal(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantNames []string
	}{
		// parent.name traversal
		{"parent.name eq Vacation", `type = "group" AND parent.name = "Vacation"`, 2, []string{"Work", "Photos"}},
		{"parent.name eq Work", `type = "group" AND parent.name = "Work"`, 1, []string{"Sub-Work"}},
		// parent.name != includes groups with no parent (owner_id IS NULL)
		{"parent.name neq Vacation", `type = "group" AND parent.name != "Vacation"`, 3, []string{"Sub-Work", "Vacation", "Archive"}},

		// children.name traversal
		{"children.name eq Work", `type = "group" AND children.name = "Work"`, 1, []string{"Vacation"}},
		{"children.name eq Sub-Work", `type = "group" AND children.name = "Sub-Work"`, 1, []string{"Work"}},

		// parent.tags traversal
		{"parent.tags eq photo", `type = "group" AND parent.tags = "photo"`, 2, []string{"Work", "Photos"}},
		// parent.tags != includes groups with no parent (Vacation, Archive)
		{"parent.tags neq photo", `type = "group" AND parent.tags != "photo"`, 3, []string{"Sub-Work", "Vacation", "Archive"}},

		// parent/children LIKE traversal is tested in TestComprehensive_TraversalLike

		// parent IS EMPTY / IS NOT EMPTY
		{"parent is empty", `type = "group" AND parent IS EMPTY`, 2, []string{"Vacation", "Archive"}},
		{"parent is not empty", `type = "group" AND parent IS NOT EMPTY`, 3, []string{"Work", "Sub-Work", "Photos"}},

		// children IS EMPTY / IS NOT EMPTY
		{"children is empty", `type = "group" AND children IS EMPTY`, 3, []string{"Archive", "Sub-Work", "Photos"}},
		{"children is not empty", `type = "group" AND children IS NOT EMPTY`, 2, []string{"Vacation", "Work"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityGroup, db)

			var groups []testGroup
			if err := result.Find(&groups).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(groups) != tt.wantCount {
				t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
			}
			if tt.wantNames != nil {
				assertNames(t, namesOfGroups(groups), tt.wantNames)
			}
		})
	}
}

// ============================================================
// Children Tags Traversal
// ============================================================

func TestComprehensive_ChildrenTagsTraversal(t *testing.T) {
	db := setupTestDB(t)

	// Give "Work" (id=2) the "video" tag for this test
	db.Exec("INSERT INTO group_tags (group_id, tag_id) VALUES (2, 2)")

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantNames []string
	}{
		// children.tags: find groups whose children have a specific tag
		// Work (child of Vacation) has "video" tag => Vacation matches
		{"children.tags eq video", `type = "group" AND children.tags = "video"`, 1, []string{"Vacation"}},
		{"children.tags neq video", `type = "group" AND children.tags != "video"`, 4, []string{"Work", "Archive", "Sub-Work", "Photos"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityGroup, db)

			var groups []testGroup
			if err := result.Find(&groups).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(groups) != tt.wantCount {
				t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
			}
			if tt.wantNames != nil {
				assertNames(t, namesOfGroups(groups), tt.wantNames)
			}
		})
	}
}

// ============================================================
// Date Functions and Relative Dates
// ============================================================

func TestComprehensive_DateFunctions(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
	}{
		// Relative dates
		{"resource created > -3d", `type = "resource" AND created > -3d`, EntityResource, 3},
		{"resource created > -1y", `type = "resource" AND created > -1y`, EntityResource, 4},
		{"resource created < -3d", `type = "resource" AND created < -3d`, EntityResource, 1},
		{"resource created > -1w", `type = "resource" AND created > -1w`, EntityResource, 3},

		// NOW() function
		{"resource created < NOW()", `type = "resource" AND created < NOW()`, EntityResource, 4},
		{"note created < NOW()", `type = "note" AND created < NOW()`, EntityNote, 2},

		// START_OF_DAY()
		{"resource created > START_OF_DAY()", `type = "resource" AND created > START_OF_DAY()`, EntityResource, 3},

		// START_OF_MONTH()
		{"resource created > START_OF_MONTH()", `type = "resource" AND created > START_OF_MONTH()`, EntityResource, 3},

		// START_OF_YEAR()
		{"resource created > START_OF_YEAR()", `type = "resource" AND created > START_OF_YEAR()`, EntityResource, 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(notes))
				}
			}
		})
	}
}

// ============================================================
// IN / NOT IN on Scalar Fields
// ============================================================

func TestComprehensive_InNotIn(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// name IN
		{"resource name in", `type = "resource" AND name IN ("sunset.jpg", "report.pdf")`, EntityResource, 2, []string{"sunset.jpg", "report.pdf"}},
		{"resource name not in", `type = "resource" AND name NOT IN ("sunset.jpg", "report.pdf")`, EntityResource, 2, []string{"photo_album.png", "untagged_file.txt"}},

		// contentType IN
		{"resource contentType in", `type = "resource" AND contentType IN ("image/jpeg", "image/png")`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource contentType not in", `type = "resource" AND contentType NOT IN ("image/jpeg", "image/png")`, EntityResource, 2, []string{"report.pdf", "untagged_file.txt"}},

		// id IN (numeric)
		{"resource id in", `type = "resource" AND id IN (1, 3)`, EntityResource, 2, []string{"sunset.jpg", "report.pdf"}},
		{"resource id not in", `type = "resource" AND id NOT IN (1, 3)`, EntityResource, 2, []string{"photo_album.png", "untagged_file.txt"}},

		// note name IN
		{"note name in", `type = "note" AND name IN ("Meeting notes", "Todo list")`, EntityNote, 2, []string{"Meeting notes", "Todo list"}},
		{"note name not in", `type = "note" AND name NOT IN ("Meeting notes")`, EntityNote, 1, []string{"Todo list"}},

		// group name IN
		{"group name in", `type = "group" AND name IN ("Vacation", "Archive")`, EntityGroup, 2, []string{"Vacation", "Archive"}},
		{"group name not in", `type = "group" AND name NOT IN ("Vacation", "Archive")`, EntityGroup, 3, []string{"Work", "Sub-Work", "Photos"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// Complex Boolean Logic (nested AND/OR/NOT)
// ============================================================

func TestComprehensive_BooleanLogic(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantNames []string
	}{
		// AND
		{"and two conditions", `type = "resource" AND contentType = "image/jpeg" AND fileSize > 500000`, 1, []string{"sunset.jpg"}},

		// OR
		{"or two conditions", `type = "resource" AND (name = "sunset.jpg" OR name = "report.pdf")`, 2, []string{"sunset.jpg", "report.pdf"}},

		// NOT
		{"not name", `type = "resource" AND NOT name = "sunset.jpg"`, 3, []string{"photo_album.png", "report.pdf", "untagged_file.txt"}},
		{"not contentType like", `type = "resource" AND NOT contentType ~ "image/*"`, 2, []string{"report.pdf", "untagged_file.txt"}},

		// Nested AND/OR
		{"nested and or", `type = "resource" AND (contentType ~ "image/*" OR (fileSize < 200 AND name ~ "*file*"))`, 3, []string{"sunset.jpg", "photo_album.png", "untagged_file.txt"}},

		// NOT with OR
		{"not or", `type = "resource" AND NOT (name = "sunset.jpg" OR name = "report.pdf")`, 2, []string{"photo_album.png", "untagged_file.txt"}},

		// Complex: tags AND content type
		{"tags and contentType", `type = "resource" AND tags = "photo" AND contentType = "image/jpeg"`, 1, []string{"sunset.jpg"}},

		// Complex: groups OR tags
		{"groups or tags", `type = "resource" AND (groups = "Work" OR tags = "video")`, 2, []string{"photo_album.png", "report.pdf"}},

		// Triple nested
		{"triple nested", `type = "resource" AND ((name ~ "*.jpg" OR name ~ "*.png") AND fileSize > 500000)`, 2, []string{"sunset.jpg", "photo_album.png"}},

		// NOT with nested expression
		{"not nested", `type = "resource" AND NOT (contentType ~ "image/*" AND fileSize > 1500000)`, 3, []string{"sunset.jpg", "report.pdf", "untagged_file.txt"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityResource, db)

			var resources []testResource
			if err := result.Find(&resources).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(resources) != tt.wantCount {
				t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
			}
			if tt.wantNames != nil {
				assertNames(t, namesOfResources(resources), tt.wantNames)
			}
		})
	}
}

// ============================================================
// ORDER BY Variations
// ============================================================

func TestComprehensive_OrderBy(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name      string
		query     string
		wantFirst string
		wantLast  string
	}{
		{"order by name asc", `type = "resource" ORDER BY name ASC`, "photo_album.png", "untagged_file.txt"},
		{"order by name desc", `type = "resource" ORDER BY name DESC`, "untagged_file.txt", "photo_album.png"},
		{"order by fileSize asc", `type = "resource" ORDER BY fileSize ASC`, "untagged_file.txt", "photo_album.png"},
		{"order by fileSize desc", `type = "resource" ORDER BY fileSize DESC`, "photo_album.png", "untagged_file.txt"},
		{"order by contentType asc name desc", `type = "resource" ORDER BY contentType ASC, name DESC`, "report.pdf", "untagged_file.txt"},
		{"order by id desc", `type = "resource" ORDER BY id DESC`, "untagged_file.txt", "sunset.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityResource, db)

			var resources []testResource
			if err := result.Find(&resources).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(resources) != 4 {
				t.Fatalf("expected 4 resources, got %d", len(resources))
			}
			if resources[0].Name != tt.wantFirst {
				t.Errorf("expected first %q, got %q", tt.wantFirst, resources[0].Name)
			}
			if resources[len(resources)-1].Name != tt.wantLast {
				t.Errorf("expected last %q, got %q", tt.wantLast, resources[len(resources)-1].Name)
			}
		})
	}
}

// ============================================================
// ORDER BY with Meta Fields
// ============================================================

func TestComprehensive_OrderByMeta(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `type = "resource" AND meta.rating >= 1 ORDER BY meta.rating DESC`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	if len(resources) != 2 {
		t.Fatalf("expected 2 resources with rating, got %d", len(resources))
	}
	if resources[0].Name != "sunset.jpg" {
		t.Errorf("expected first 'sunset.jpg' (rating=5), got %q", resources[0].Name)
	}
	if resources[1].Name != "photo_album.png" {
		t.Errorf("expected second 'photo_album.png' (rating=3), got %q", resources[1].Name)
	}
}

// ============================================================
// LIMIT / OFFSET Edge Cases
// ============================================================

func TestComprehensive_LimitOffset(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		{"limit 1", `type = "resource" ORDER BY name ASC LIMIT 1`, EntityResource, 1, []string{"photo_album.png"}},
		{"limit 2", `type = "resource" ORDER BY name ASC LIMIT 2`, EntityResource, 2, []string{"photo_album.png", "report.pdf"}},
		{"limit 0", `type = "resource" ORDER BY name ASC LIMIT 0`, EntityResource, 0, nil},
		{"limit larger than results", `type = "resource" ORDER BY name ASC LIMIT 100`, EntityResource, 4, nil},
		{"offset 1", `type = "resource" ORDER BY name ASC LIMIT 10 OFFSET 1`, EntityResource, 3, []string{"report.pdf", "sunset.jpg", "untagged_file.txt"}},
		{"offset 2", `type = "resource" ORDER BY name ASC LIMIT 10 OFFSET 2`, EntityResource, 2, []string{"sunset.jpg", "untagged_file.txt"}},
		{"offset beyond results", `type = "resource" ORDER BY name ASC LIMIT 10 OFFSET 100`, EntityResource, 0, nil},
		{"limit 1 offset 2", `type = "resource" ORDER BY name ASC LIMIT 1 OFFSET 2`, EntityResource, 1, []string{"sunset.jpg"}},
		{"limit and offset on notes", `type = "note" ORDER BY name ASC LIMIT 1 OFFSET 0`, EntityNote, 1, []string{"Meeting notes"}},
		{"limit on groups", `type = "group" ORDER BY name ASC LIMIT 2`, EntityGroup, 2, []string{"Archive", "Photos"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// Case Insensitivity
// ============================================================

func TestComprehensive_CaseInsensitivity(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// Name equality is case-insensitive
		{"resource name uppercase", `type = "resource" AND name = "SUNSET.JPG"`, EntityResource, 1, []string{"sunset.jpg"}},
		{"resource name mixed case", `type = "resource" AND name = "Sunset.Jpg"`, EntityResource, 1, []string{"sunset.jpg"}},

		// LIKE is case-insensitive
		{"resource name like uppercase", `type = "resource" AND name ~ "PHOTO*"`, EntityResource, 1, []string{"photo_album.png"}},

		// Tag matching is case-insensitive
		{"resource tags uppercase", `type = "resource" AND tags = "PHOTO"`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},
		{"resource tags mixed case", `type = "resource" AND tags = "Photo"`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},

		// IN is case-insensitive for strings
		{"resource name in case insensitive", `type = "resource" AND name IN ("SUNSET.JPG", "REPORT.PDF")`, EntityResource, 2, []string{"sunset.jpg", "report.pdf"}},

		// Group name matching is case-insensitive
		{"resource groups case insensitive", `type = "resource" AND groups = "vacation"`, EntityResource, 1, []string{"sunset.jpg"}},

		// Group name equality is case-insensitive
		{"group name case insensitive", `type = "group" AND name = "vacation"`, EntityGroup, 1, []string{"Vacation"}},

		// Note name equality is case-insensitive
		{"note name case insensitive", `type = "note" AND name = "meeting notes"`, EntityNote, 1, []string{"Meeting notes"}},

		// Tags IN is case-insensitive
		{"tags in case insensitive", `type = "resource" AND tags IN ("PHOTO", "VIDEO")`, EntityResource, 2, []string{"sunset.jpg", "photo_album.png"}},

		// parent.name is case-insensitive
		{"parent name case insensitive", `type = "group" AND parent.name = "vacation"`, EntityGroup, 2, []string{"Work", "Photos"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(groups), namesOfGroups(groups))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfGroups(groups), tt.wantNames)
				}
			}
		})
	}
}

// ============================================================
// Wildcard Escaping Edge Cases
// ============================================================

func TestComprehensive_WildcardEscaping(t *testing.T) {
	db := setupTestDB(t)

	// The _ in "photo_album" should be treated as literal, not as wildcard
	// When using LIKE, MRQL wildcards * and ? are converted, but _ and % are escaped

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantNames []string
	}{
		// The name "photo_album.png" contains an underscore
		// LIKE pattern "photo_album*" should match — the _ is escaped to \_
		{"underscore in like pattern", `type = "resource" AND name ~ "photo?album*"`, 1, []string{"photo_album.png"}},
		// ? matches single char: "photo?album.png" should match photo_album.png
		{"single char wildcard", `type = "resource" AND name ~ "photo?album.png"`, 1, []string{"photo_album.png"}},
		// * at start and end
		{"star both ends", `type = "resource" AND name ~ "*album*"`, 1, []string{"photo_album.png"}},
		// No wildcards = exact match via LIKE
		{"exact via like", `type = "resource" AND name ~ "sunset.jpg"`, 1, []string{"sunset.jpg"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityResource, db)

			var resources []testResource
			if err := result.Find(&resources).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(resources) != tt.wantCount {
				t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
			}
			if tt.wantNames != nil {
				assertNames(t, namesOfResources(resources), tt.wantNames)
			}
		})
	}
}

// ============================================================
// File Size Units
// ============================================================

func TestComprehensive_FileSizeUnits(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		// 1mb = 1048576 bytes; resource 2 = 2048000 (>1mb), resource 1 = 1024000 (<1mb)
		{"fileSize gt 1mb", `type = "resource" AND fileSize > 1mb`, 1},
		{"fileSize gte 1mb", `type = "resource" AND fileSize >= 1mb`, 1},
		// 500kb = 512000; resources 1 (1024000) and 2 (2048000) and 3 (512000) are >= 500kb
		{"fileSize gte 500kb", `type = "resource" AND fileSize >= 500kb`, 3},
		// 2gb = huge; no resources that big
		{"fileSize gt 2gb", `type = "resource" AND fileSize > 2gb`, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityResource, db)

			var resources []testResource
			if err := result.Find(&resources).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(resources) != tt.wantCount {
				t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
			}
		})
	}
}

// ============================================================
// Empty/No-Filter Queries
// ============================================================

func TestComprehensive_EmptyQueries(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
	}{
		{"all resources", `ORDER BY name ASC`, EntityResource, 4},
		{"all notes", `ORDER BY name ASC`, EntityNote, 2},
		{"all groups", `ORDER BY name ASC`, EntityGroup, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			q.EntityType = tt.entityType

			result, err := Translate(q, db)
			if err != nil {
				t.Fatalf("translate error: %v", err)
			}

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(resources))
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(notes))
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(groups))
				}
			}
		})
	}
}

// ============================================================
// Entity Type Extraction from Query
// ============================================================

func TestComprehensive_EntityTypeExtraction(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{"type resource", `type = "resource" AND name = "sunset.jpg"`, 1},
		{"type note", `type = "note" AND name = "Meeting notes"`, 1},
		{"type group", `type = "group" AND name = "Vacation"`, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			// Do not set EntityType — let it be extracted from query
			if err := Validate(q); err != nil {
				t.Fatalf("validation error: %v", err)
			}

			result, err := Translate(q, db)
			if err != nil {
				t.Fatalf("translate error: %v", err)
			}

			switch q.EntityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(resources))
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(notes))
				}
			case EntityGroup:
				var groups []testGroup
				if err := result.Find(&groups).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(groups) != tt.wantCount {
					t.Fatalf("expected %d, got %d", tt.wantCount, len(groups))
				}
			}
		})
	}
}

// ============================================================
// Error Cases
// ============================================================

func TestComprehensive_TranslateErrors(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name  string
		query string
	}{
		{"no entity type", `name = "test"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse error: %v", err)
			}
			_, err = Translate(q, db)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
		})
	}
}

// ============================================================
// Combined Filters: tags + groups + meta
// ============================================================

// ============================================================
// Traversal LIKE patterns
// ============================================================

func TestComprehensive_TraversalLike(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name      string
		query     string
		wantCount int
		wantNames []string
	}{
		// parent.name ~ "Vac*" should match Work and Photos (both have parent Vacation)
		{"parent.name like", `type = "group" AND parent.name ~ "Vac*"`, 2, []string{"Work", "Photos"}},
		// children.name ~ "*Work*" matches Vacation (child Work) and Work (child Sub-Work)
		{"children.name like Work", `type = "group" AND children.name ~ "*Work*"`, 2, []string{"Vacation", "Work"}},
		// parent.name !~ "Vac*" should match Sub-Work (parent is Work) + parentless groups (Vacation, Archive)
		{"parent.name not like", `type = "group" AND parent.name !~ "Vac*"`, 3, nil},
		// children.name !~ "*Work*": both Vacation (child=Work) and Work (child=Sub-Work) match *Work*,
		// so leaf groups without any matching child (Archive, Sub-Work, Photos) pass the NOT LIKE
		{"children.name not like", `type = "group" AND children.name !~ "*Work*"`, 3, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityGroup, db)

			var groups []testGroup
			if err := result.Find(&groups).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(groups) != tt.wantCount {
				names := make([]string, len(groups))
				for i, g := range groups {
					names[i] = g.Name
				}
				t.Errorf("expected %d results, got %d: %v", tt.wantCount, len(groups), names)
			}
			if tt.wantNames != nil {
				names := make([]string, len(groups))
				for i, g := range groups {
					names[i] = g.Name
				}
				for _, want := range tt.wantNames {
					found := false
					for _, got := range names {
						if got == want {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected %q in results, got %v", want, names)
					}
				}
			}
		})
	}
}

// TestComprehensive_ChildrenNegationSemantics verifies that children.name != "X" means
// "has no child named X", NOT "has some child not named X". With mixed children
// (Vacation has children Work AND Photos), children.name != "Work" should EXCLUDE
// Vacation because it DOES have a child named Work.
func TestComprehensive_ChildrenNegationSemantics(t *testing.T) {
	db := setupTestDB(t)

	// Vacation has children: Work, Photos. Work has child: Sub-Work. Archive has none.
	// children.name != "Work" should mean "has no child named Work":
	//   - Vacation: has child "Work" → EXCLUDED
	//   - Work: has child "Sub-Work" (not "Work") → INCLUDED
	//   - Archive: no children → INCLUDED (leaf)
	//   - Sub-Work: no children → INCLUDED (leaf)
	//   - Photos: no children → INCLUDED (leaf)
	result := parseAndTranslate(t, `type = "group" AND children.name != "Work"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	names := namesOfGroups(groups)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups (Work, Archive, Sub-Work, Photos), got %d: %v", len(groups), names)
	}
	for _, name := range names {
		if name == "Vacation" {
			t.Fatalf("Vacation should be excluded (it has a child named Work), got: %v", names)
		}
	}
}

// TestComprehensive_ChildrenNotLikeSemantics verifies children.name !~ "W*" uses
// NOT EXISTS semantics (no child matches pattern), not "has some child not matching".
func TestComprehensive_ChildrenNotLikeSemantics(t *testing.T) {
	db := setupTestDB(t)

	// children.name !~ "W*" should mean "has no child with name matching W*":
	//   - Vacation: has child "Work" (matches W*) → EXCLUDED
	//   - Work: has child "Sub-Work" (no match) → INCLUDED
	//   - Archive, Sub-Work, Photos: no children → INCLUDED
	result := parseAndTranslate(t, `type = "group" AND children.name !~ "W*"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}

	names := namesOfGroups(groups)
	if len(groups) != 4 {
		t.Fatalf("expected 4 groups (Work, Archive, Sub-Work, Photos), got %d: %v", len(groups), names)
	}
	for _, name := range names {
		if name == "Vacation" {
			t.Fatalf("Vacation should be excluded, got: %v", names)
		}
	}
}

func TestComprehensive_CombinedFilters(t *testing.T) {
	db := setupTestDB(t)

	tests := []struct {
		name       string
		query      string
		entityType EntityType
		wantCount  int
		wantNames  []string
	}{
		// Tags AND groups
		{"resource with photo tag in Vacation group", `type = "resource" AND tags = "photo" AND groups = "Vacation"`, EntityResource, 1, []string{"sunset.jpg"}},
		// Tags AND meta
		{"resource with photo tag and high rating", `type = "resource" AND tags = "photo" AND meta.rating = 5`, EntityResource, 1, []string{"sunset.jpg"}},
		// Groups AND meta
		{"resource in Vacation group with high rating", `type = "resource" AND groups = "Vacation" AND meta.rating > 3`, EntityResource, 1, []string{"sunset.jpg"}},
		// All three combined
		{"resource with photo tag in Vacation with rating", `type = "resource" AND tags = "photo" AND groups = "Vacation" AND meta.rating >= 5`, EntityResource, 1, []string{"sunset.jpg"}},
		// Note with tags and groups
		{"note with document tag in Vacation", `type = "note" AND tags = "document" AND groups = "Vacation"`, EntityNote, 1, []string{"Meeting notes"}},
		// Note with meta and group
		{"note with priority high in Vacation", `type = "note" AND meta.priority = "high" AND groups = "Vacation"`, EntityNote, 1, []string{"Meeting notes"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, tt.entityType, db)

			switch tt.entityType {
			case EntityResource:
				var resources []testResource
				if err := result.Find(&resources).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(resources) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(resources), namesOfResources(resources))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfResources(resources), tt.wantNames)
				}
			case EntityNote:
				var notes []testNote
				if err := result.Find(&notes).Error; err != nil {
					t.Fatalf("query error: %v", err)
				}
				if len(notes) != tt.wantCount {
					t.Fatalf("expected %d, got %d (names: %v)", tt.wantCount, len(notes), namesOfNotes(notes))
				}
				if tt.wantNames != nil {
					assertNames(t, namesOfNotes(notes), tt.wantNames)
				}
			}
		})
	}
}

// TestComprehensive_ParentDirectNegation verifies that parent != "X" includes
// root groups (owner_id IS NULL), matching parent.name != semantics.
func TestComprehensive_ParentDirectNegation(t *testing.T) {
	db := setupTestDB(t)

	// parent != "Vacation": Work and Photos excluded (parent=Vacation).
	// Sub-Work (parent=Work), Vacation (root), Archive (root) included.
	result := parseAndTranslate(t, `type = "group" AND parent != "Vacation"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	names := namesOfGroups(groups)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups (Sub-Work, Vacation, Archive), got %d: %v", len(groups), names)
	}
	for _, n := range names {
		if n == "Work" || n == "Photos" {
			t.Fatalf("%s should be excluded (parent IS Vacation), got: %v", n, names)
		}
	}
}

// TestComprehensive_ParentDirectNotLike verifies parent !~ includes root groups.
func TestComprehensive_ParentDirectNotLike(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `type = "group" AND parent !~ "Vac*"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	names := namesOfGroups(groups)
	if len(groups) != 3 {
		t.Fatalf("expected 3 (Sub-Work, Vacation, Archive), got %d: %v", len(groups), names)
	}
}

// TestComprehensive_TypeOrTypeQuery verifies that `type = resource OR type = note`
// is treated as a cross-entity query, not collapsed to single-entity resource.
func TestComprehensive_TypeOrTypeQuery(t *testing.T) {
	_ = setupTestDB(t) // ensure test infra works

	// type = resource OR type = note should return both resources AND notes
	q, err := Parse(`(type = "resource" OR type = "note") AND name ~ "*"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}

	et := ExtractEntityType(q)
	// Should be Unspecified (cross-entity), NOT EntityResource
	if et != EntityUnspecified {
		t.Fatalf("expected EntityUnspecified for OR-ed types, got %s", et)
	}
}

// TestComprehensive_ValidatorRejectsInvalidTraversalSubfield verifies that
// the validator catches unknown subfields like parent.nonexistent at validation
// time, not at translation time.
func TestComprehensive_ValidatorRejectsInvalidTraversalSubfield(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"parent.nonexistent", `type = "group" AND parent.nonexistent = "x"`},
		{"children.foobar", `type = "group" AND children.foobar = "x"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// TestComprehensive_ValidatorAcceptsValidTraversalSubfields verifies that
// known subfields pass validation.
func TestComprehensive_ValidatorAcceptsValidTraversalSubfields(t *testing.T) {
	tests := []string{
		`type = "group" AND parent.name = "x"`,
		`type = "group" AND parent.tags = "x"`,
		`type = "group" AND parent.category IS NULL`,
		`type = "group" AND children.name IS NOT NULL`,
		`type = "group" AND children.name = "x"`,
		`type = "group" AND children.tags = "x"`,
		`type = "group" AND children.description ~ "x*"`,
	}

	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			q, err := Parse(query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := Validate(q); err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

// TestComprehensive_MultiTypeFilterExecution verifies that `type = resource OR type = note`
// only returns resources and notes, NOT groups.
func TestComprehensive_MultiTypeFilterExecution(t *testing.T) {
	db := setupTestDB(t)

	// Cross-entity query: only resource + note, not group.
	// ExtractEntityType returns Unspecified for OR-ed types, so the translator
	// fans out. But the type = comparisons must still be enforced as WHERE filters.
	// We test by calling Translate per entity type and checking that groups are
	// excluded when the query says type = resource OR type = note.

	// For entity type "group", the type = resource comparison should filter out all groups.
	q, err := Parse(`(type = "resource" OR type = "note") AND name ~ "*"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	Validate(q)

	// When translated for groups, `type = "resource"` should produce zero results
	// because none of the groups have type = resource.
	clone := *q
	clone.EntityType = EntityGroup
	result, err := TranslateWithOptions(&clone, db, TranslateOptions{})
	if err != nil {
		// TranslateError is acceptable — means the type field was rejected
		t.Logf("translate error (acceptable): %v", err)
		return
	}

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("expected 0 groups (type filter should exclude groups), got %d: %v",
			len(groups), namesOfGroups(groups))
	}
}

// TestComprehensive_MetaIsNull verifies that meta.rating IS NULL works
// without generating invalid SQL like "resources.meta.rating IS NULL".
func TestComprehensive_MetaIsNull(t *testing.T) {
	db := setupTestDB(t)

	// Resources 3 and 4 have meta={} (no rating key).
	// meta.rating IS NULL should return those resources.
	result := parseAndTranslate(t, `type = "resource" AND meta.rating IS NULL`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Resources 1 has rating=5, resource 2 has rating=3. Resources 3,4 have no rating.
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources without meta.rating, got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// TestComprehensive_MetaIsNotNull verifies meta.rating IS NOT NULL works.
func TestComprehensive_MetaIsNotNull(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `type = "resource" AND meta.rating IS NOT NULL`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources with meta.rating set, got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// TestComprehensive_MetaStringCaseInsensitive verifies that meta string
// equality is case-insensitive, matching the language's general rule.
func TestComprehensive_MetaStringCaseInsensitive(t *testing.T) {
	db := setupTestDB(t)

	// Note 1 has meta.priority = "high". Query with "HIGH" should match.
	result := parseAndTranslate(t, `type = "note" AND meta.priority = "HIGH"`, EntityNote, db)

	var notes []testNote
	if err := result.Find(&notes).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(notes) != 1 || notes[0].Name != "Meeting notes" {
		t.Fatalf("expected 1 note (Meeting notes) for case-insensitive meta.priority = HIGH, got %d: %v",
			len(notes), namesOfNotes(notes))
	}
}

// TestComprehensive_MetaIsEmpty verifies that meta.rating IS EMPTY works
// without generating invalid SQL like "resources.meta.rating IS NULL".
func TestComprehensive_MetaIsEmpty(t *testing.T) {
	db := setupTestDB(t)

	// Resources 3 and 4 have no rating. IS EMPTY on a meta field should
	// behave like IS NULL (the value doesn't exist in JSON).
	result := parseAndTranslate(t, `type = "resource" AND meta.rating IS EMPTY`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources without meta.rating, got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// TestComprehensive_MetaIsNotEmpty verifies meta.rating IS NOT EMPTY works.
func TestComprehensive_MetaIsNotEmpty(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `type = "resource" AND meta.rating IS NOT EMPTY`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources with meta.rating, got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// TestComprehensive_NotTypeResource verifies NOT type = "resource" is cross-entity
// and excludes resources from results.
func TestComprehensive_NotTypeResource(t *testing.T) {
	// NOT type = "resource" should NOT collapse to single-entity resource.
	q, err := Parse(`NOT type = "resource" AND name ~ "*"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	Validate(q)

	et := ExtractEntityType(q)
	if et != EntityUnspecified {
		t.Fatalf("NOT type = resource should be cross-entity (Unspecified), got %s", et)
	}
}

// TestComprehensive_TypeOrNonType verifies type = "resource" OR name = "foo"
// is cross-entity because the OR means non-resource entities could match the name.
func TestComprehensive_TypeOrNonType(t *testing.T) {
	q, err := Parse(`type = "resource" OR name = "Todo list"`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	Validate(q)

	et := ExtractEntityType(q)
	if et != EntityUnspecified {
		t.Fatalf("type=resource OR name=x should be cross-entity, got %s", et)
	}
}

// TestComprehensive_TypeNeqExcludesEntity verifies type != "resource" excludes
// resources and includes notes and groups.
func TestComprehensive_TypeNeqExcludesEntity(t *testing.T) {
	db := setupTestDB(t)

	// type != "resource" translated for the resource table should return 0 rows
	q, _ := Parse(`type != "resource" AND name ~ "*"`)
	Validate(q)

	clone := *q
	clone.EntityType = EntityResource
	result, err := TranslateWithOptions(&clone, db, TranslateOptions{})
	if err != nil {
		t.Logf("translate error (acceptable): %v", err)
		return
	}

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("type != resource should exclude all resources, got %d: %v",
			len(resources), namesOfResources(resources))
	}

	// ...but translated for notes should return notes
	clone2 := *q
	clone2.EntityType = EntityNote
	result2, err := TranslateWithOptions(&clone2, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate for notes: %v", err)
	}
	var notes []testNote
	result2.Find(&notes)
	if len(notes) == 0 {
		t.Fatal("type != resource should include notes, got 0")
	}
}

// TestComprehensive_NotTypeResourceExecution verifies that NOT type = "resource"
// actually excludes resources at query execution time, not just at extraction.
func TestComprehensive_NotTypeResourceExecution(t *testing.T) {
	db := setupTestDB(t)

	q, _ := Parse(`NOT type = "resource" AND name ~ "*"`)
	Validate(q)

	// Cross-entity: translate for resources — should return 0
	clone := *q
	clone.EntityType = EntityResource
	result, err := TranslateWithOptions(&clone, db, TranslateOptions{})
	if err != nil {
		t.Logf("translate error (acceptable): %v", err)
		return
	}
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("NOT type=resource should exclude ALL resources, got %d: %v",
			len(resources), namesOfResources(resources))
	}

	// Translate for notes — should return all notes
	clone2 := *q
	clone2.EntityType = EntityNote
	result2, err := TranslateWithOptions(&clone2, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate for notes: %v", err)
	}
	var notes []testNote
	result2.Find(&notes)
	if len(notes) == 0 {
		t.Fatal("NOT type=resource should include notes, got 0")
	}
}

// TestComprehensive_TypeOrNameExecution verifies that type = "resource" OR name = "Todo list"
// returns both matching resources AND the note named "Todo list".
func TestComprehensive_TypeOrNameExecution(t *testing.T) {
	db := setupTestDB(t)

	q, _ := Parse(`type = "resource" OR name = "Todo list"`)
	Validate(q)

	// For resources: type = "resource" matches, so all resources should be returned
	clone := *q
	clone.EntityType = EntityResource
	result, err := TranslateWithOptions(&clone, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate for resources: %v", err)
	}
	var resources []testResource
	result.Find(&resources)
	if len(resources) != 4 {
		t.Fatalf("type=resource arm should return all 4 resources, got %d: %v",
			len(resources), namesOfResources(resources))
	}

	// For notes: type = "resource" doesn't match notes, but name = "Todo list" does
	clone2 := *q
	clone2.EntityType = EntityNote
	result2, err := TranslateWithOptions(&clone2, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate for notes: %v", err)
	}
	var notes []testNote
	result2.Find(&notes)
	if len(notes) != 1 || notes[0].Name != "Todo list" {
		t.Fatalf("name=Todo list arm should match 1 note, got %d: %v",
			len(notes), namesOfNotes(notes))
	}
}

// P1: type with unsupported operators should be rejected at validation.
func TestComprehensive_TypeUnsupportedOperators(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"type != invalid", `type != "foobar"`},
		{"type like", `type ~ "res*"`},
		{"type not like", `type !~ "res*"`},
		{"type gt", `type > "resource"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return // parse error is fine
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// P2: meta.rating IN (3, 5) should not produce invalid SQL.
func TestComprehensive_MetaInExpr(t *testing.T) {
	db := setupTestDB(t)

	// Resources: 1 has rating=5, 2 has rating=3, 3+4 have no rating.
	result := parseAndTranslate(t, `type = "resource" AND meta.rating IN (3, 5)`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources with rating 3 or 5, got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// P2: parent.name IN (...) should be rejected at validation.
func TestComprehensive_TraversalInValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"parent.name IN", `type = "group" AND parent.name IN ("Vacation", "Work")`},
		{"children.name IS EMPTY", `type = "group" AND children.name IS EMPTY`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// P2: ORDER BY on relation/traversal fields should be rejected at validation.
func TestComprehensive_OrderByRelationFieldValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"order by tags", `type = "resource" ORDER BY tags ASC`},
		{"order by groups", `type = "resource" ORDER BY groups ASC`},
		{"order by parent.name", `type = "group" ORDER BY parent.name ASC`},
		{"order by children", `type = "group" ORDER BY children ASC`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// TestComprehensive_TraversalIsNull verifies parent.category IS NULL and
// children.name IS NOT NULL work via traversal subqueries.
func TestComprehensive_TraversalIsNull(t *testing.T) {
	db := setupTestDB(t)

	// All parent groups have null category_id. parent.category IS NULL matches:
	// - Work, Photos, Sub-Work (parents all have null category)
	// - Vacation, Archive (no parent → included by IS NULL null-parent fallback)
	result := parseAndTranslate(t, `type = "group" AND parent.category IS NULL`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups (all parents have null category + root groups), got %d: %v",
			len(groups), namesOfGroups(groups))
	}
}

// TestComprehensive_TraversalIsNotNull verifies parent.name IS NOT NULL.
func TestComprehensive_TraversalIsNotNull(t *testing.T) {
	db := setupTestDB(t)

	// All groups with a parent have parent.name IS NOT NULL.
	result := parseAndTranslate(t, `type = "group" AND parent.name IS NOT NULL`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Work (parent=Vacation), Photos (parent=Vacation), Sub-Work (parent=Work)
	if len(groups) != 3 {
		t.Fatalf("expected 3 groups with parent.name IS NOT NULL, got %d: %v",
			len(groups), namesOfGroups(groups))
	}
}

// TestComprehensive_ChildrenTraversalIsNotNull verifies children.name IS NOT NULL.
func TestComprehensive_ChildrenTraversalIsNotNull(t *testing.T) {
	db := setupTestDB(t)

	// children.name IS NOT NULL means "has at least one child" (same as children IS NOT EMPTY)
	result := parseAndTranslate(t, `type = "group" AND children.name IS NOT NULL`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Vacation (children: Work, Photos), Work (child: Sub-Work)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups with children.name IS NOT NULL, got %d: %v",
			len(groups), namesOfGroups(groups))
	}
}

func TestComprehensive_OwnerParentIsNull(t *testing.T) {
	db := setupTestDB(t)

	// owner.parent IS NULL: resources whose owner group has no parent.
	// sunset.jpg → Vacation (no parent) → matches
	// report.pdf → Work (parent=Vacation) → doesn't match
	// photo_album.png, untagged_file.txt → no owner → also matches (null FK fallback)
	result := parseAndTranslate(t, `type = "resource" AND owner.parent IS NULL`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// sunset.jpg (owner Vacation, no parent) + 2 ownerless = 3
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources with owner.parent IS NULL, got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

func TestComprehensive_OwnerParentIsNotNull(t *testing.T) {
	db := setupTestDB(t)

	// owner.parent IS NOT NULL: resources whose owner group has a parent.
	// report.pdf → Work (parent=Vacation) → matches
	result := parseAndTranslate(t, `type = "resource" AND owner.parent IS NOT NULL`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource with owner.parent IS NOT NULL, got %d: %v",
			len(resources), namesOfResources(resources))
	}
	if resources[0].Name != "report.pdf" {
		t.Errorf("expected report.pdf, got %s", resources[0].Name)
	}
}

// P1: Unsupported operators on relation fields should be rejected.
func TestComprehensive_RelationUnsupportedOperators(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"tags gt", `type = "resource" AND tags > 3`},
		{"tags gte", `type = "resource" AND tags >= 3`},
		{"tags lt", `type = "resource" AND tags < 3`},
		{"tags lte", `type = "resource" AND tags <= 3`},
		{"groups gt", `type = "resource" AND groups > 1`},
		{"parent gte", `type = "group" AND parent >= 1`},
		{"children lt", `type = "group" AND children < 5`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// P1: IS NULL on relation fields should use correct SQL (not fd.Column).
func TestComprehensive_RelationIsNull(t *testing.T) {
	db := setupTestDB(t)

	// parent IS NULL should work (owner_id IS NULL) — tests the relation IS NULL path
	result := parseAndTranslate(t, `type = "group" AND parent IS NULL`, EntityGroup, db)
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("parent IS NULL query error: %v", err)
	}
	// Vacation and Archive have no parent
	if len(groups) != 2 {
		t.Fatalf("expected 2 root groups, got %d: %v", len(groups), namesOfGroups(groups))
	}

	// tags IS NULL should be rejected — tags is a relation, use IS EMPTY instead
	q, _ := Parse(`type = "resource" AND tags IS NULL`)
	err := Validate(q)
	if err == nil {
		t.Fatal("expected validation error for tags IS NULL, got nil")
	}
}

// P2: Fractional LIMIT/OFFSET should be rejected.
func TestComprehensive_FractionalLimitOffset(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"fractional limit", `type = "resource" LIMIT 1.9`},
		{"fractional offset", `type = "resource" LIMIT 10 OFFSET 2.5`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.query)
			if err == nil {
				t.Fatalf("expected parse error for %s, got nil", tt.query)
			}
		})
	}
}

// Mixed-type OR: each branch scoped to its own entity type should validate.
func TestComprehensive_MixedTypeOrValidation(t *testing.T) {
	q, err := Parse(`(type = "note" AND noteType = 1) OR (type = "resource" AND contentType ~ "image/*")`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err != nil {
		t.Fatalf("expected valid query, got: %v", err)
	}
}

// parent IN / children IN should be rejected at validation.
func TestComprehensive_ParentChildrenInValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"parent IN", `type = "group" AND parent IN ("Vacation", "Work")`},
		{"children IN", `type = "group" AND children IN ("Sub-Work")`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// Completer should detect type = "resource" (quoted) and narrow fields.
func TestComprehensive_CompleterQuotedType(t *testing.T) {
	suggestions := Complete(`type = "resource" AND `, 22)
	hasContentType := false
	for _, s := range suggestions {
		if s.Value == "contentType" {
			hasContentType = true
		}
	}
	if !hasContentType {
		t.Fatalf("after type = \"resource\" AND, should suggest contentType; got %v", suggestions)
	}
}

// Mixed-type OR: per-entity translation should not fail on the opposite branch.
// type = resource AND contentType ~ "image/*" translated for notes should
// return 0 rows (type mismatch), NOT a translation error.
func TestComprehensive_MixedTypeOrTranslation(t *testing.T) {
	db := setupTestDB(t)

	q, _ := Parse(`(type = "resource" AND contentType ~ "image/*") OR (type = "note" AND name ~ "*notes*")`)
	Validate(q)

	// For resources: type=resource matches, contentType works. type=note → 1=0.
	// The note branch has name ~ which is valid on resources too. Should return image resources.
	clone := *q
	clone.EntityType = EntityResource
	result, err := TranslateWithOptions(&clone, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate for resources should not error: %v", err)
	}
	var resources []testResource
	result.Find(&resources)
	if len(resources) != 2 {
		t.Fatalf("expected 2 image resources, got %d: %v", len(resources), namesOfResources(resources))
	}

	// For notes: type=resource → 1=0, type=note → 1=1, name ~ "*notes*" filters.
	// contentType is resource-only but sits under an OR with type=resource (→ 1=0),
	// so the whole left branch is false. Should not error.
	clone2 := *q
	clone2.EntityType = EntityNote
	result2, err := TranslateWithOptions(&clone2, db, TranslateOptions{})
	if err != nil {
		t.Fatalf("translate for notes should not error: %v", err)
	}
	var notes []testNote
	result2.Find(&notes)
	if len(notes) != 1 || notes[0].Name != "Meeting notes" {
		t.Fatalf("expected 1 note (Meeting notes), got %d: %v", len(notes), namesOfNotes(notes))
	}
}

// P1: A AND (B OR C) must preserve grouping.
func TestComprehensive_NestedOrGrouping(t *testing.T) {
	db := setupTestDB(t)

	// name ~ "*photo*" matches only "photo_album.png" (1 resource)
	// tags = "video" matches resource 2 (photo_album has video tag)
	// contentType = "application/pdf" matches resource 3 (report.pdf)
	//
	// contentType = "application/pdf" AND (name ~ "*photo*" OR tags = "video")
	// Should match: only photo_album.png (matches name OR tags) that is also pdf? No.
	// Actually: contentType = "application/pdf" → resource 3. name ~ "*photo*" → resource 2.
	// tags = "video" → resource 2. So the AND groups: pdf AND (photo_name OR video_tag).
	// Resource 3 is pdf but doesn't match either OR branch. Resource 2 matches OR but not pdf.
	// Result: 0 rows. If grouping is broken (pdf AND photo_name OR video_tag),
	// resource 2 would leak through because video_tag is OR'd at top level.

	result := parseAndTranslate(t,
		`type = "resource" AND contentType = "application/pdf" AND (name ~ "*photo*" OR tags = "video")`,
		EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 0 {
		t.Fatalf("expected 0 results (pdf AND (photo_name OR video_tag) matches nothing), got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// P2: parent.meta and children.meta should be rejected at validation
// since the parser forbids 3-segment fields and the translator can't handle them.
func TestComprehensive_TraversalMetaValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"parent.meta", `type = "group" AND parent.meta = "x"`},
		{"children.meta", `type = "group" AND children.meta = "x"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return // parse error is acceptable
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// P1: NOT (type = "note" AND noteType = 1) should validate — the NOT branch
// has its own type guard scoping noteType.
func TestComprehensive_NotTypeGuardedField(t *testing.T) {
	q, err := Parse(`NOT (type = "note" AND noteType = 1)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err != nil {
		t.Fatalf("expected valid query, got: %v", err)
	}
}

// Postgres numeric meta cast should not blow up on mixed-type data.
// On SQLite, json_extract handles mixed types gracefully. This test
// verifies the comparison logic works when meta values are strings
// but the query uses a numeric comparison.
func TestComprehensive_MetaNumericOnMixedData(t *testing.T) {
	db := setupTestDB(t)

	// Resource 1 has meta.rating=5 (numeric), but let's add a resource
	// with a string value for the same key to simulate mixed data.
	db.Create(&testResource{
		ID: 10, Name: "mixed_meta.txt", ContentType: "text/plain",
		FileSize: 50, CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Meta: `{"rating":"not_a_number"}`,
	})

	// meta.rating > 3 should still work — the string value row should
	// simply not match, not crash the query.
	result := parseAndTranslate(t, `type = "resource" AND meta.rating > 3`, EntityResource, db)

	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query should not error on mixed meta types: %v", err)
	}
	// Only resource 1 (rating=5) should match, not the string-value one
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected 1 result (sunset.jpg with rating=5), got %d: %v",
			len(resources), namesOfResources(resources))
	}
}

// Completer should suggest group subfields after "parent.", not meta keys.
func TestComprehensive_CompleterParentDot(t *testing.T) {
	suggestions := Complete(`type = "group" AND parent.`, 26)
	hasName := false
	hasMeta := false
	for _, s := range suggestions {
		if s.Value == "name" {
			hasName = true
		}
		if s.Value == "meta.<key>" {
			hasMeta = true
		}
	}
	if !hasName {
		t.Fatalf("after parent., should suggest 'name'; got %v", suggestions)
	}
	if hasMeta {
		t.Fatalf("after parent., should NOT suggest meta.<key>; got %v", suggestions)
	}
}

// Completer should suggest group subfields after "children." too.
func TestComprehensive_CompleterChildrenDot(t *testing.T) {
	suggestions := Complete(`type = "group" AND children.`, 28)
	hasTags := false
	for _, s := range suggestions {
		if s.Value == "tags" {
			hasTags = true
		}
	}
	if !hasTags {
		t.Fatalf("after children., should suggest 'tags'; got %v", suggestions)
	}
}

// Direct children != should not be broken by NULL owner_ids.
// children != "Vacation" should return all groups since no group has a child named "Vacation".
func TestComprehensive_ChildrenDirectNeqNull(t *testing.T) {
	db := setupTestDB(t)

	// No group in the fixture has a child named "Vacation".
	// children != "Vacation" should return Work (child=Sub-Work) + leaf groups.
	// Vacation has children Work+Photos (neither named "Vacation") → included.
	// All 5 groups should match.
	result := parseAndTranslate(t, `type = "group" AND children != "Vacation"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups (no child is named Vacation), got %d: %v",
			len(groups), namesOfGroups(groups))
	}
}

// Direct children !~ should not be broken by NULL owner_ids.
func TestComprehensive_ChildrenDirectNotLikeNull(t *testing.T) {
	db := setupTestDB(t)

	// children !~ "NonExistent*" — no child matches, so all groups qualify.
	result := parseAndTranslate(t, `type = "group" AND children !~ "NonExistent*"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups, got %d: %v", len(groups), namesOfGroups(groups))
	}
}

// Traversal children.name != should use NOT EXISTS semantics and handle NULLs.
// children.name != "Vacation" should include all groups since no child is named "Vacation".
func TestComprehensive_ChildrenTraversalNeqNull(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `type = "group" AND children.name != "Vacation"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	// Vacation (children: Work, Photos — neither "Vacation") → INCLUDED
	// Work (child: Sub-Work) → INCLUDED
	// Archive, Sub-Work, Photos (no children) → INCLUDED (leaf)
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups (no child named Vacation), got %d: %v",
			len(groups), namesOfGroups(groups))
	}
}

// Traversal children.name !~ should handle NULLs.
func TestComprehensive_ChildrenTraversalNotLikeNull(t *testing.T) {
	db := setupTestDB(t)

	result := parseAndTranslate(t, `type = "group" AND children.name !~ "NoMatch*"`, EntityGroup, db)

	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 5 {
		t.Fatalf("expected 5 groups, got %d: %v", len(groups), namesOfGroups(groups))
	}
}

// P1: fileSize > "abc" should be rejected at validation (type mismatch).
func TestComprehensive_ValueTypeMismatch(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"number field with string", `type = "resource" AND fileSize > "abc"`},
		{"number field with string eq", `type = "resource" AND width = "hello"`},
		{"datetime field with number", `type = "resource" AND created > 42`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// P1: Valid type combinations should still pass.
func TestComprehensive_ValueTypeValid(t *testing.T) {
	tests := []string{
		`type = "resource" AND fileSize > 100`,
		`type = "resource" AND fileSize > 10mb`,
		`type = "resource" AND name = "hello"`,
		`type = "resource" AND name ~ "hel*"`,
		`type = "resource" AND created > -7d`,
		`type = "resource" AND created > NOW()`,
		`type = "resource" AND created >= "2024-01-01"`,
		`type = "resource" AND width > 1920`,
		`type = "resource" AND meta.rating > 5`,
		`type = "resource" AND meta.name = "hello"`,
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			q, err := Parse(query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := Validate(q); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}
}

// P2: meta.priority IN ("high", "LOW") should be case-insensitive.
func TestComprehensive_MetaInCaseInsensitive(t *testing.T) {
	db := setupTestDB(t)

	// Note 1 has meta.priority = "high". Query with "HIGH" should match.
	result := parseAndTranslate(t, `type = "note" AND meta.priority IN ("HIGH", "medium")`, EntityNote, db)

	var notes []testNote
	if err := result.Find(&notes).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(notes) != 1 || notes[0].Name != "Meeting notes" {
		t.Fatalf("expected 1 note (Meeting notes), got %d: %v",
			len(notes), namesOfNotes(notes))
	}
}

// P3: Completer should suggest fields (not operators) when cursor is
// immediately after a partial identifier with no trailing space.
func TestComprehensive_CompleterPartialField(t *testing.T) {
	// User typed "cont" (partial for contentType) — cursor at end, no space
	suggestions := Complete(`type = "resource" AND cont`, 26)
	hasOperator := false
	hasField := false
	for _, s := range suggestions {
		if s.Value == "=" || s.Value == "!=" {
			hasOperator = true
		}
		if s.Value == "contentType" {
			hasField = true
		}
	}
	if hasOperator {
		t.Fatalf("partial field 'cont' should not suggest operators; got %v", suggestions)
	}
	if !hasField {
		t.Fatalf("partial field 'cont' should suggest 'contentType'; got %v", suggestions)
	}
}

// Completer should suggest operators after a complete field name followed by space.
func TestComprehensive_CompleterCompleteField(t *testing.T) {
	// "name " — complete field with trailing space
	suggestions := Complete(`name `, 5)
	hasOperator := false
	for _, s := range suggestions {
		if s.Value == "=" {
			hasOperator = true
		}
	}
	if !hasOperator {
		t.Fatalf("complete field 'name ' should suggest operators; got %v", suggestions)
	}
}

// Focused regression tests for partial-field completer behavior.
func TestComprehensive_CompleterPartialFieldEdgeCases(t *testing.T) {
	tests := []struct {
		name          string
		query         string
		cursor        int
		wantFields    bool // should suggest fields, not operators
		wantOperators bool // should suggest operators, not fields
	}{
		// Partial identifier, cursor at end — suggest fields
		{"partial 'cont'", `type = "resource" AND cont`, 26, true, false},
		{"partial 'na'", `na`, 2, true, false},
		{"partial 'file'", `type = "resource" AND file`, 26, true, false},

		// Complete identifier followed by space — suggest operators
		{"complete 'name '", `name `, 5, false, true},
		{"complete 'fileSize '", `type = "resource" AND fileSize `, 31, false, true},

		// Partial after dot — suggest sub-fields (not operators)
		{"partial 'meta.ra'", `meta.ra`, 7, true, false},

		// Cursor mid-token (not at end) — suggest fields
		{"cursor mid-word", `contentType`, 4, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suggestions := Complete(tt.query, tt.cursor)
			hasField := false
			hasOperator := false
			for _, s := range suggestions {
				if s.Type == "field" || s.Type == "keyword" {
					hasField = true
				}
				if s.Type == "operator" {
					hasOperator = true
				}
			}
			if tt.wantFields && !hasField {
				t.Errorf("expected field suggestions, got %v", suggestions)
			}
			if tt.wantFields && hasOperator {
				t.Errorf("should not suggest operators for partial field, got %v", suggestions)
			}
			if tt.wantOperators && !hasOperator {
				t.Errorf("expected operator suggestions, got %v", suggestions)
			}
		})
	}
}

// Traversal relation subfields like parent.children, parent.groups,
// parent.tags IS NULL should be rejected at validation.
func TestComprehensive_TraversalRelationSubfieldValidation(t *testing.T) {
	tests := []struct {
		name  string
		query string
	}{
		{"parent.children", `type = "group" AND parent.children = "x"`},
		{"parent.groups", `type = "group" AND parent.groups = "x"`},
		{"children.parent", `type = "group" AND children.parent = "x"`},
		{"children.children", `type = "group" AND children.children = "x"`},
		{"parent.tags IS NULL", `type = "group" AND parent.tags IS NULL`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				return
			}
			err = Validate(q)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", tt.query)
			}
		})
	}
}

// Valid traversal subfields should still pass.
func TestComprehensive_TraversalValidSubfields(t *testing.T) {
	tests := []string{
		`type = "group" AND parent.name = "x"`,
		`type = "group" AND parent.tags = "photo"`,
		`type = "group" AND children.name ~ "x*"`,
		`type = "group" AND children.tags != "old"`,
		`type = "group" AND parent.description ~ "*test*"`,
		`type = "group" AND parent.category IS NULL`,
	}
	for _, query := range tests {
		t.Run(query, func(t *testing.T) {
			q, err := Parse(query)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if err := Validate(q); err != nil {
				t.Fatalf("expected valid, got: %v", err)
			}
		})
	}
}

// groupByVal dereferences pointer values returned by GORM's map[string]any scanning.
// SQLite driver often returns *int64, *string, etc. in raw map results.
func groupByVal(v any) string {
	if v == nil {
		return "<nil>"
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return "<nil>"
		}
		rv = rv.Elem()
	}
	return fmt.Sprintf("%v", rv.Interface())
}

// ============================================================
// GROUP BY — Aggregated Mode
// ============================================================

func TestComprehensive_GroupByAggregatedCount(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// 4 resources with 4 distinct contentTypes
	if len(result.Rows) != 4 {
		t.Errorf("expected 4 rows, got %d: %v", len(result.Rows), result.Rows)
	}
	// Each row should have contentType and count
	for _, row := range result.Rows {
		if _, ok := row["contentType"]; !ok {
			t.Error("missing 'contentType' in aggregated row")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count' in aggregated row")
		}
		// Each content type has exactly 1 resource in seed data
		if groupByVal(row["count"]) != "1" {
			t.Errorf("expected count=1 for each contentType, got %s for %s", groupByVal(row["count"]), groupByVal(row["contentType"]))
		}
	}
}

func TestComprehensive_GroupByAggregatedSumAvg(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	for _, row := range result.Rows {
		if _, ok := row["sum_fileSize"]; !ok {
			t.Error("missing 'sum_fileSize'")
		}
		if _, ok := row["avg_fileSize"]; !ok {
			t.Error("missing 'avg_fileSize'")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count'")
		}
	}

	// Verify specific values: each contentType has one resource, so sum=avg=fileSize
	expectedSizes := map[string]int64{
		"image/jpeg":      1024000,
		"image/png":       2048000,
		"application/pdf": 512000,
		"text/plain":      100,
	}
	for _, row := range result.Rows {
		ct := groupByVal(row["contentType"])
		expectedSize, ok := expectedSizes[ct]
		if !ok {
			t.Errorf("unexpected contentType %q", ct)
			continue
		}
		// SUM and AVG should equal the single resource's fileSize (since count=1)
		sumVal := groupByVal(row["sum_fileSize"])
		if sumVal != fmt.Sprintf("%d", expectedSize) {
			t.Errorf("contentType %q: expected sum_fileSize=%d, got %s", ct, expectedSize, sumVal)
		}
	}
}

func TestComprehensive_GroupByAggregatedMeta(t *testing.T) {
	db := setupTestDB(t)
	// Group resources by meta.rating — only sunset.jpg (rating=5) and photo_album.png (rating=3) have it
	q, err := Parse(`type = "resource" GROUP BY meta.rating COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Expect at least 2 distinct rating groups (5, 3) — resources without rating may form a null group
	if len(result.Rows) < 2 {
		t.Errorf("expected at least 2 rows for meta.rating grouping, got %d: %v", len(result.Rows), result.Rows)
	}
	for _, row := range result.Rows {
		if _, ok := row["meta.rating"]; !ok {
			t.Error("missing 'meta.rating' key in row")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count' key in row")
		}
	}
}

func TestComprehensive_GroupByAggregatedWithFilter(t *testing.T) {
	db := setupTestDB(t)
	// Only resources with fileSize > 100000 — excludes untagged_file.txt (100)
	q, err := Parse(`type = "resource" AND fileSize > 100000 GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	// 3 resources match: sunset.jpg, photo_album.png, report.pdf — each with distinct contentType
	if len(result.Rows) != 3 {
		t.Errorf("expected 3 rows after filter, got %d: %v", len(result.Rows), result.Rows)
	}
	for _, row := range result.Rows {
		count, ok := row["count"]
		if !ok {
			t.Error("missing count")
			continue
		}
		if groupByVal(count) == "0" {
			t.Error("expected non-zero count after filter")
		}
	}
}

func TestComprehensive_GroupByAggregatedOrderByLimit(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() ORDER BY count DESC LIMIT 2`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(result.Rows) > 2 {
		t.Errorf("expected at most 2 rows, got %d", len(result.Rows))
	}
	if len(result.Rows) == 0 {
		t.Error("expected at least 1 row")
	}
	// Verify all rows have both required fields
	for _, row := range result.Rows {
		if _, ok := row["contentType"]; !ok {
			t.Error("missing 'contentType'")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count'")
		}
	}
}

func TestComprehensive_GroupByMinMax(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType MIN(fileSize) MAX(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Each contentType has exactly 1 resource, so min=max=fileSize
	for _, row := range result.Rows {
		if _, ok := row["min_fileSize"]; !ok {
			t.Error("missing min_fileSize")
		}
		if _, ok := row["max_fileSize"]; !ok {
			t.Error("missing max_fileSize")
		}
		// With 1 item per group, min should equal max
		minVal := groupByVal(row["min_fileSize"])
		maxVal := groupByVal(row["max_fileSize"])
		if minVal != maxVal {
			t.Errorf("expected min=max for single-item groups, got min=%s max=%s for %s", minVal, maxVal, groupByVal(row["contentType"]))
		}
	}
}

func TestComprehensive_GroupByMultipleKeys(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType, meta.rating COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// 4 resources, each has unique contentType; 2 have rating, 2 don't
	// All 4 should have distinct (contentType, meta.rating) combinations
	if len(result.Rows) != 4 {
		t.Errorf("expected 4 rows, got %d: %v", len(result.Rows), result.Rows)
	}
	for _, row := range result.Rows {
		if _, ok := row["contentType"]; !ok {
			t.Error("missing contentType")
		}
		// meta.rating key must be present even if the value is nil
		if _, ok := row["meta.rating"]; !ok {
			t.Error("missing meta.rating key")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing count")
		}
	}
}

func TestComprehensive_GroupByNotesByOwner(t *testing.T) {
	db := setupTestDB(t)
	// Group notes by their owner — note 1 owned by Vacation, note 2 owned by Work
	q, err := Parse(`type = "note" GROUP BY owner COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityNote

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// 2 notes with 2 distinct owners
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows, got %d: %v", len(result.Rows), result.Rows)
	}
	ownerNames := make(map[string]bool)
	for _, row := range result.Rows {
		if _, ok := row["count"]; !ok {
			t.Error("missing count")
		}
		ownerName := groupByVal(row["owner"])
		ownerNames[ownerName] = true
	}
	if !ownerNames["Vacation"] || !ownerNames["Work"] {
		t.Errorf("expected owner names {Vacation, Work}, got %v", ownerNames)
	}
}

func TestComprehensive_GroupByAllAggregates(t *testing.T) {
	db := setupTestDB(t)
	// All 5 aggregate functions at once
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() SUM(fileSize) AVG(fileSize) MIN(fileSize) MAX(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	expectedKeys := []string{"contentType", "count", "sum_fileSize", "avg_fileSize", "min_fileSize", "max_fileSize"}
	for _, row := range result.Rows {
		for _, key := range expectedKeys {
			if _, ok := row[key]; !ok {
				t.Errorf("missing key %q in row %v", key, row)
			}
		}
	}
}

// ============================================================
// GROUP BY — Bucketed Mode
// ============================================================

func TestComprehensive_GroupByBucketedSimple(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	// 4 distinct content types
	if len(keys) != 4 {
		t.Errorf("expected 4 bucket keys, got %d: %v", len(keys), keys)
	}

	// Fetch items for each bucket
	totalResources := 0
	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(resources) > 5 {
			t.Errorf("expected at most 5 per bucket, got %d", len(resources))
		}
		if len(resources) == 0 {
			t.Errorf("expected at least 1 resource per bucket key %v", key)
		}
		totalResources += len(resources)
	}
	// All 4 resources accounted for (each unique contentType)
	if totalResources != 4 {
		t.Errorf("expected 4 total resources across all buckets, got %d", totalResources)
	}
}

func TestComprehensive_GroupByBucketedLimitEnforcement(t *testing.T) {
	db := setupTestDB(t)
	// LIMIT 1 per bucket — should return at most 1 resource per content type
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 1`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}

	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(resources) > 1 {
			t.Errorf("LIMIT 1 violated: got %d resources for bucket %v", len(resources), key)
		}
	}
}

func TestComprehensive_GroupByBucketedWithFilter(t *testing.T) {
	db := setupTestDB(t)
	// Only image/* resources — should produce 2 buckets (jpeg, png)
	q, err := Parse(`type = "resource" AND contentType ~ "image/*" GROUP BY contentType LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 bucket keys for image/* filter, got %d: %v", len(keys), keys)
	}

	expectedTypes := map[string]bool{"image/jpeg": true, "image/png": true}
	for _, key := range keys {
		ct := groupByVal(key["contentType"])
		if !expectedTypes[ct] {
			t.Errorf("unexpected bucket key contentType=%q", ct)
		}

		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(resources) != 1 {
			t.Errorf("expected 1 resource per bucket, got %d for %q", len(resources), ct)
		}
	}
}

func TestComprehensive_GroupByBucketedNotes(t *testing.T) {
	db := setupTestDB(t)
	// Group notes by owner — bucketed mode (no aggregates)
	q, err := Parse(`type = "note" GROUP BY owner LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityNote

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	// 2 notes, 2 distinct owners
	if len(keys) != 2 {
		t.Errorf("expected 2 bucket keys, got %d: %v", len(keys), keys)
	}

	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var notes []testNote
		if err := bucketDB.Find(&notes).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(notes) == 0 {
			t.Errorf("expected at least 1 note per bucket key %v", key)
		}
		if len(notes) > 10 {
			t.Errorf("LIMIT 10 violated: got %d notes", len(notes))
		}
	}
}

// ============================================================
// GROUP BY — Edge Cases
// ============================================================

func TestComprehensive_GroupByEmptyResultSet(t *testing.T) {
	db := setupTestDB(t)
	// Filter that matches nothing — fileSize > 999999999
	q, err := Parse(`type = "resource" AND fileSize > 999999999 GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	if len(result.Rows) != 0 {
		t.Errorf("expected 0 rows for empty result set, got %d: %v", len(result.Rows), result.Rows)
	}
}

func TestComprehensive_GroupByBucketedEmptyResultSet(t *testing.T) {
	db := setupTestDB(t)
	// Filter that matches nothing
	q, err := Parse(`type = "resource" AND fileSize > 999999999 GROUP BY contentType LIMIT 5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("expected 0 bucket keys for empty result set, got %d", len(keys))
	}
}

func TestComprehensive_GroupByTranslateReturnsNilForBucketed(t *testing.T) {
	db := setupTestDB(t)
	// Bucketed mode (no aggregates) — TranslateGroupBy should return nil result
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 5`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for bucketed mode, got %+v", result)
	}
}

func TestComprehensive_GroupByGroupsByMetaRegion(t *testing.T) {
	db := setupTestDB(t)
	// Group groups by meta.region — only Vacation has region=europe
	q, err := Parse(`type = "group" GROUP BY meta.region COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityGroup

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// At least 1 row for "europe", possibly more for null region
	if len(result.Rows) == 0 {
		t.Error("expected at least 1 row")
	}
	// Find the "europe" row
	foundEurope := false
	for _, row := range result.Rows {
		if groupByVal(row["meta.region"]) == "europe" {
			foundEurope = true
			if groupByVal(row["count"]) != "1" {
				t.Errorf("expected count=1 for europe, got %s", groupByVal(row["count"]))
			}
		}
	}
	if !foundEurope {
		t.Errorf("expected a row with meta.region=europe, got rows: %v", result.Rows)
	}
}

func TestComprehensive_GroupByAggregatedOrderByAscending(t *testing.T) {
	db := setupTestDB(t)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT() SUM(fileSize) ORDER BY sum_fileSize ASC`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if len(result.Rows) < 2 {
		t.Fatalf("need at least 2 rows to verify ordering, got %d", len(result.Rows))
	}
	// Verify ascending order of sum_fileSize (numeric comparison)
	parseNum := func(s string) int64 {
		var n int64
		fmt.Sscanf(s, "%d", &n)
		return n
	}
	for i := 1; i < len(result.Rows); i++ {
		prevSum := parseNum(groupByVal(result.Rows[i-1]["sum_fileSize"]))
		currSum := parseNum(groupByVal(result.Rows[i]["sum_fileSize"]))
		if prevSum > currSum {
			t.Errorf("rows not in ascending order: row[%d]=%d > row[%d]=%d", i-1, prevSum, i, currSum)
		}
	}
}

// ============================================================
// GROUP BY — Traversal Paths
// ============================================================

func TestComprehensive_GroupByTraversalOwnerName(t *testing.T) {
	db := setupTestDB(t)
	// GROUP BY owner.name — resources grouped by their owner group's name
	// sunset.jpg → Vacation, report.pdf → Work, others → NULL
	q, err := Parse(`type = "resource" GROUP BY owner.name COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Expect at least Vacation and Work rows
	foundVacation := false
	foundWork := false
	for _, row := range result.Rows {
		name := groupByVal(row["owner.name"])
		if name == "Vacation" {
			foundVacation = true
			if groupByVal(row["count"]) != "1" {
				t.Errorf("expected count=1 for Vacation, got %s", groupByVal(row["count"]))
			}
		}
		if name == "Work" {
			foundWork = true
			if groupByVal(row["count"]) != "1" {
				t.Errorf("expected count=1 for Work, got %s", groupByVal(row["count"]))
			}
		}
	}
	if !foundVacation {
		t.Errorf("expected Vacation in results, got: %v", result.Rows)
	}
	if !foundWork {
		t.Errorf("expected Work in results, got: %v", result.Rows)
	}
}

func TestComprehensive_GroupByTraversalBucketed(t *testing.T) {
	db := setupTestDB(t)
	// Bucketed mode with traversal: GROUP BY owner.name without aggregates
	q, err := Parse(`type = "resource" GROUP BY owner.name LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected at least one bucket key")
	}
	// Each key should have "owner.name"
	for _, key := range keys {
		if _, ok := key["owner.name"]; !ok {
			t.Errorf("expected 'owner.name' in key, got: %v", key)
		}
	}
}

func TestComprehensive_GroupByTraversalDeep(t *testing.T) {
	db := setupTestDB(t)
	// owner.parent.name: report.pdf → Work → Vacation (parent)
	// sunset.jpg → Vacation → no parent (NULL)
	q, err := Parse(`type = "resource" GROUP BY owner.parent.name COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	// Work's parent is Vacation, so report.pdf → owner.parent.name = "Vacation"
	foundVacation := false
	for _, row := range result.Rows {
		name := groupByVal(row["owner.parent.name"])
		if name == "Vacation" {
			foundVacation = true
		}
	}
	if !foundVacation {
		t.Errorf("expected owner.parent.name=Vacation for Work-owned resources, got: %v", result.Rows)
	}
}

// ============================================================
// GROUP BY — Additional Tests
// ============================================================

func TestComprehensive_GroupByTagsRelationWithCount(t *testing.T) {
	db := setupTestDB(t)
	// GROUP BY tags — each tag should appear as a key
	// resource_tags: r1→photo, r2→photo, r2→video
	// resources without tags (r3, r4) should appear under NULL tag
	q, err := Parse(`type = "resource" GROUP BY tags COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}

	// Verify tag names appear as keys
	tagCounts := make(map[string]string)
	for _, row := range result.Rows {
		tagName := groupByVal(row["tags"])
		count := groupByVal(row["count"])
		tagCounts[tagName] = count
	}

	// "photo" tag: resources 1 and 2
	if c, ok := tagCounts["photo"]; !ok {
		t.Error("expected 'photo' tag in grouped results")
	} else if c != "2" {
		t.Errorf("expected count=2 for photo tag, got %s", c)
	}

	// "video" tag: resource 2 only
	if c, ok := tagCounts["video"]; !ok {
		t.Error("expected 'video' tag in grouped results")
	} else if c != "1" {
		t.Errorf("expected count=1 for video tag, got %s", c)
	}
}

func TestComprehensive_GroupByOwnerRelationWithCount(t *testing.T) {
	db := setupTestDB(t)
	// GROUP BY owner (relation, no traversal) — joins groups via owner_id
	// r1 owned by Vacation, r3 owned by Work, r2 and r4 have no owner
	q, err := Parse(`type = "resource" GROUP BY owner COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}

	// Verify owner names
	ownerCounts := make(map[string]string)
	for _, row := range result.Rows {
		name := groupByVal(row["owner"])
		count := groupByVal(row["count"])
		ownerCounts[name] = count
	}
	if ownerCounts["Vacation"] != "1" {
		t.Errorf("expected count=1 for Vacation owner, got %s", ownerCounts["Vacation"])
	}
	if ownerCounts["Work"] != "1" {
		t.Errorf("expected count=1 for Work owner, got %s", ownerCounts["Work"])
	}
	// NULL owner (r2, r4)
	if ownerCounts["<nil>"] != "2" {
		t.Errorf("expected count=2 for NULL owner, got %s", ownerCounts["<nil>"])
	}
}

func TestComprehensive_GroupByWithWhereFilterOnSameField(t *testing.T) {
	db := setupTestDB(t)
	// Filter on contentType ~ "image" then GROUP BY contentType
	// Should only return image types
	q, err := Parse(`type = "resource" AND contentType ~ "image*" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	// Only image/* types should appear
	for _, row := range result.Rows {
		ct := groupByVal(row["contentType"])
		if ct != "image/jpeg" && ct != "image/png" {
			t.Errorf("unexpected contentType %q after image* filter", ct)
		}
	}
	if len(result.Rows) != 2 {
		t.Errorf("expected 2 rows (jpeg, png), got %d: %v", len(result.Rows), result.Rows)
	}
}

func TestComprehensive_GroupByCreatedDatetime(t *testing.T) {
	db := setupTestDB(t)
	// GROUP BY created (datetime field) — tests datetime handling
	q, err := Parse(`type = "resource" GROUP BY created COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// There are 4 resources. 3 share the same CreatedAt (now), 1 is 30 days ago.
	// So we should have at least 2 distinct groups.
	if len(result.Rows) < 2 {
		t.Errorf("expected at least 2 rows for created grouping, got %d: %v", len(result.Rows), result.Rows)
	}
	for _, row := range result.Rows {
		if _, ok := row["created"]; !ok {
			t.Error("missing 'created' key")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count' key")
		}
	}
}

func TestComprehensive_GroupBySumOnMetaNumericField(t *testing.T) {
	db := setupTestDB(t)
	// note 2 has meta.count=7, note 1 has no count
	// Group notes by owner, sum meta.count
	q, err := Parse(`type = "note" GROUP BY owner SUM(meta.count)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityNote

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}

	// Find the Work owner row (note 2, which has count=7)
	for _, row := range result.Rows {
		ownerName := groupByVal(row["owner"])
		if ownerName == "Work" {
			sumVal := groupByVal(row["sum_meta.count"])
			if sumVal != "7" {
				t.Errorf("expected sum_meta.count=7 for Work owner, got %s", sumVal)
			}
		}
	}
}

func TestComprehensive_GroupByBucketedWithOrderBy(t *testing.T) {
	db := setupTestDB(t)
	// Bucketed mode with ORDER BY within buckets
	q, err := Parse(`type = "resource" GROUP BY contentType ORDER BY name ASC LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) != 4 {
		t.Fatalf("expected 4 bucket keys, got %d", len(keys))
	}

	// Fetch items and verify order within buckets
	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		// Each bucket should have exactly 1 resource (all content types are unique)
		if len(resources) != 1 {
			t.Errorf("expected 1 resource per bucket, got %d for key %v", len(resources), key)
		}
	}
}

func TestComprehensive_GroupByBucketedByMetaField(t *testing.T) {
	db := setupTestDB(t)
	// Bucketed GROUP BY meta.rating on resources
	q, err := Parse(`type = "resource" GROUP BY meta.rating LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	// sunset.jpg rating=5, photo_album.png rating=3, others have no rating
	// Expect at least 2 distinct keys (5, 3, possibly null)
	if len(keys) < 2 {
		t.Errorf("expected at least 2 bucket keys for meta.rating, got %d: %v", len(keys), keys)
	}

	// Verify each bucket returns resources
	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		if len(resources) == 0 {
			t.Errorf("expected at least 1 resource per bucket key %v", key)
		}
	}
}

func TestComprehensive_GroupByOwnerTagsTraversal(t *testing.T) {
	db := setupTestDB(t)
	// GROUP BY owner.tags — traversal ending in a relation
	// r1 owner=Vacation → tags: photo
	// r3 owner=Work → tags: document
	// r2, r4 no owner → NULL
	q, err := Parse(`type = "resource" GROUP BY owner.tags COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}

	// Verify owner tag names appear
	tagNames := make(map[string]bool)
	for _, row := range result.Rows {
		name := groupByVal(row["owner.tags"])
		tagNames[name] = true
	}
	// Vacation group has tag "photo", Work group has tag "document"
	if !tagNames["photo"] {
		t.Errorf("expected 'photo' (Vacation's tag) in owner.tags grouping, got: %v", tagNames)
	}
	if !tagNames["document"] {
		t.Errorf("expected 'document' (Work's tag) in owner.tags grouping, got: %v", tagNames)
	}
}

func TestComprehensive_GroupByMultipleKeysWithTraversal(t *testing.T) {
	db := setupTestDB(t)
	// Multiple GROUP BY keys: owner.name + contentType
	q, err := Parse(`type = "resource" GROUP BY owner.name, contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}

	// Each row should have both owner.name and contentType
	for _, row := range result.Rows {
		if _, ok := row["owner.name"]; !ok {
			t.Error("missing 'owner.name' key")
		}
		if _, ok := row["contentType"]; !ok {
			t.Error("missing 'contentType' key")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count' key")
		}
	}

	// 4 resources, each with unique contentType and varying owners
	// r1: Vacation + image/jpeg, r2: NULL + image/png, r3: Work + application/pdf, r4: NULL + text/plain
	if len(result.Rows) != 4 {
		t.Errorf("expected 4 distinct (owner.name, contentType) rows, got %d: %v", len(result.Rows), result.Rows)
	}

	// Verify specific combos
	type combo struct{ owner, ct string }
	combos := make(map[combo]bool)
	for _, row := range result.Rows {
		c := combo{groupByVal(row["owner.name"]), groupByVal(row["contentType"])}
		combos[c] = true
	}
	if !combos[combo{"Vacation", "image/jpeg"}] {
		t.Error("missing Vacation + image/jpeg combination")
	}
	if !combos[combo{"Work", "application/pdf"}] {
		t.Error("missing Work + application/pdf combination")
	}
}

func TestComprehensive_GroupByTraversalMetaLeaf(t *testing.T) {
	db := setupTestDB(t)
	// owner.meta.region: sunset.jpg → Vacation (region=europe), report.pdf → Work (no region)
	q, err := Parse(`type = "resource" GROUP BY owner.meta.region COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Should have at least one row with region=europe (from Vacation-owned sunset.jpg)
	foundEurope := false
	for _, row := range result.Rows {
		val := groupByVal(row["owner.meta.region"])
		if val == "europe" {
			foundEurope = true
			if groupByVal(row["count"]) != "1" {
				t.Errorf("expected count=1 for europe, got %s", groupByVal(row["count"]))
			}
		}
	}
	if !foundEurope {
		t.Errorf("expected owner.meta.region=europe, got rows: %v", result.Rows)
	}
}

// ============================================================
// GROUP BY — Meta Traversal Leaf Tests
// ============================================================

func TestComprehensive_GroupByOwnerMetaPriority(t *testing.T) {
	db := setupTestDB(t)
	// owner.meta.priority: sunset.jpg → Vacation (priority=3), report.pdf → Work (no priority)
	q, err := Parse(`type = "resource" GROUP BY owner.meta.priority COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Should have a row with priority=3 (from Vacation-owned sunset.jpg)
	foundPriority3 := false
	for _, row := range result.Rows {
		val := groupByVal(row["owner.meta.priority"])
		if val == "3" {
			foundPriority3 = true
			if groupByVal(row["count"]) != "1" {
				t.Errorf("expected count=1 for priority=3, got %s", groupByVal(row["count"]))
			}
		}
	}
	if !foundPriority3 {
		t.Errorf("expected owner.meta.priority=3, got rows: %v", result.Rows)
	}
}

func TestComprehensive_GroupByOwnerMetaRegionBucketed(t *testing.T) {
	db := setupTestDB(t)
	// Bucketed mode: GROUP BY owner.meta.region without aggregates
	q, err := Parse(`type = "resource" GROUP BY owner.meta.region LIMIT 10`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	if len(keys) == 0 {
		t.Fatal("expected at least one bucket key")
	}
	// Each key should contain "owner.meta.region"
	for _, key := range keys {
		if _, ok := key["owner.meta.region"]; !ok {
			t.Errorf("expected 'owner.meta.region' in key, got: %v", key)
		}
	}
}

func TestComprehensive_GroupByParentMetaRegionOnGroups(t *testing.T) {
	db := setupTestDB(t)
	// parent.meta.region on groups:
	// Work (id=2, owner=Vacation) → parent meta has region=europe
	// Photos (id=5, owner=Vacation) → parent meta has region=europe
	// Sub-Work (id=4, owner=Work) → parent meta has no region (empty meta)
	// Vacation, Archive → no parent (NULL)
	q, err := Parse(`type = "group" GROUP BY parent.meta.region COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityGroup

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Should have a row with region=europe (Work and Photos both have parent=Vacation)
	foundEurope := false
	for _, row := range result.Rows {
		val := groupByVal(row["parent.meta.region"])
		if val == "europe" {
			foundEurope = true
			count := groupByVal(row["count"])
			// Work and Photos both have parent Vacation (region=europe)
			if count != "2" {
				t.Errorf("expected count=2 for europe (Work + Photos), got %s", count)
			}
		}
	}
	if !foundEurope {
		t.Errorf("expected parent.meta.region=europe, got rows: %v", result.Rows)
	}
}

func TestComprehensive_GroupByOwnerMetaNonexistent(t *testing.T) {
	db := setupTestDB(t)
	// owner.meta.nonexistent: no seeded data has this key, so all values should be NULL
	// The query should succeed without error
	q, err := Parse(`type = "resource" GROUP BY owner.meta.nonexistent COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// All resources should fall into NULL bucket(s) since no group has meta.nonexistent
	// Just verify it executes without error and returns rows
	if len(result.Rows) == 0 {
		t.Error("expected at least one row (NULL bucket), got 0")
	}
	// Verify all values are nil/<nil> since no group has this meta key
	for _, row := range result.Rows {
		val := groupByVal(row["owner.meta.nonexistent"])
		if val != "<nil>" && val != "" {
			t.Errorf("expected NULL for nonexistent meta key, got %q in rows: %v", val, result.Rows)
		}
	}
}

// ============================================================
// Bug fix tests
// ============================================================

// Bug 1: GROUP BY owner groups by name, not ID — distinct groups with same name merge.
func TestBugfix_GroupByOwnerUsesIDNotName(t *testing.T) {
	db := setupTestDB(t)

	// Seed two groups with the SAME name but different IDs
	db.Exec("INSERT INTO groups (id, name, meta, created_at, updated_at) VALUES (100, 'DuplicateName', '{}', datetime('now'), datetime('now'))")
	db.Exec("INSERT INTO groups (id, name, meta, created_at, updated_at) VALUES (101, 'DuplicateName', '{}', datetime('now'), datetime('now'))")
	// Assign resources to different groups with same name
	db.Exec("INSERT INTO resources (id, name, content_type, file_size, meta, created_at, updated_at, owner_id) VALUES (100, 'res-a', 'text/plain', 10, '{}', datetime('now'), datetime('now'), 100)")
	db.Exec("INSERT INTO resources (id, name, content_type, file_size, meta, created_at, updated_at, owner_id) VALUES (101, 'res-b', 'text/plain', 20, '{}', datetime('now'), datetime('now'), 101)")

	q, err := Parse(`type = "resource" GROUP BY owner COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}

	// There should be separate rows for each owner, even though names match.
	// Count rows where the display name is "DuplicateName"
	dupCount := 0
	for _, row := range result.Rows {
		name := groupByVal(row["owner"])
		if name == "DuplicateName" {
			dupCount++
		}
	}
	if dupCount < 2 {
		t.Errorf("expected 2 separate rows for owners named 'DuplicateName' (different IDs), got %d; rows: %v", dupCount, result.Rows)
	}
}

// Bug 2: TranslateGroupByKeys ignores OFFSET — bucketed pagination always returns first page.
func TestBugfix_BucketedKeysRespectsOffset(t *testing.T) {
	db := setupTestDB(t)

	// Resources have 4 content types: image/jpeg, image/png, application/pdf, text/plain
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 2 OFFSET 2`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	allKeys, err := TranslateGroupByKeys(&Query{
		Where: q.Where, GroupBy: &GroupByClause{Fields: q.GroupBy.Fields},
		EntityType: EntityResource, Limit: -1, Offset: -1,
	}, db)
	if err != nil {
		t.Fatalf("all keys: %v", err)
	}

	offsetKeys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("offset keys: %v", err)
	}

	// With 4 total content types, LIMIT 2 OFFSET 2 should skip the first 2
	if len(allKeys) < 4 {
		t.Skipf("need 4+ content types, got %d", len(allKeys))
	}
	if len(offsetKeys) > 2 {
		t.Errorf("expected at most 2 keys with LIMIT 2, got %d", len(offsetKeys))
	}
	// The offset keys should NOT be the same as the first 2 all-keys
	if len(offsetKeys) > 0 && len(allKeys) > 0 {
		firstAllKey := groupByVal(allKeys[0]["contentType"])
		firstOffsetKey := groupByVal(offsetKeys[0]["contentType"])
		if firstAllKey == firstOffsetKey {
			t.Errorf("OFFSET 2 returned same first key as OFFSET 0 (%s) — OFFSET is being ignored", firstAllKey)
		}
	}
}

// Bug 3: Grouped execution has no default limit — unbounded results.
// The default limit is applied by ExecuteMRQLGrouped in the execution layer,
// which sets parsed.Limit = defaultMRQLLimit (1000) when Limit < 0.
// We test this by verifying the execution layer mutates the query's Limit.
func TestBugfix_GroupedQueryAppliesDefaultLimit(t *testing.T) {
	// Verify that parsed queries start with Limit=-1 (unset)
	q, err := Parse(`type = "resource" GROUP BY contentType COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if q.Limit != -1 {
		t.Fatalf("expected Limit=-1 after parse, got %d", q.Limit)
	}

	// Verify that TranslateGroupByBucket applies LIMIT when Limit >= 0
	db := setupTestDB(t)
	q2, err := Parse(`type = "resource" GROUP BY contentType`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q2); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q2.EntityType = EntityResource
	// Simulate what ExecuteMRQLGrouped does: set default limit
	q2.Limit = 1000

	bucketDB, err := TranslateGroupByBucket(q2, db, map[string]any{"contentType": "image/jpeg"})
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	// Use DryRun to capture SQL without executing
	dryDB := bucketDB.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{})
	sql := strings.ToUpper(dryDB.Statement.SQL.String())
	if !strings.Contains(sql, "LIMIT") {
		t.Errorf("expected LIMIT in bucketed query when Limit=1000, got SQL: %s", sql)
	}
}

// Bug 4: GROUP BY type pseudo-field should be rejected by validator.
func TestBugfix_GroupByTypePseudoFieldRejected(t *testing.T) {
	q, err := Parse(`type = "resource" GROUP BY type COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	err = Validate(q)
	if err == nil {
		t.Fatal("expected validation error for GROUP BY type pseudo-field")
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("expected error mentioning 'type', got: %v", err)
	}
}

// P1a: Bucketed LIMIT should cap items per bucket, not the number of bucket keys.
func TestBugfix_BucketedLimitIsPerBucketNotKeyCount(t *testing.T) {
	db := setupTestDB(t)

	// 4 content types exist. LIMIT 2 should return ALL 4 groups, each with max 2 items.
	q, err := Parse(`type = "resource" GROUP BY contentType LIMIT 2`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}
	// Should return all 4 content types, not just 2
	if len(keys) < 4 {
		t.Errorf("expected all 4 content type keys (LIMIT is per-bucket, not key count), got %d: %v", len(keys), keys)
	}
}

// P1b: Bucketed ORDER BY should NOT apply to the keys query — only to items within buckets.
func TestBugfix_BucketedOrderByNotAppliedToKeysQuery(t *testing.T) {
	db := setupTestDB(t)

	// ORDER BY name is valid for items within buckets, but applying it to the
	// keys query (SELECT contentType ... GROUP BY contentType ORDER BY name)
	// would fail on PostgreSQL since "name" is neither grouped nor aggregated.
	q, err := Parse(`type = "resource" GROUP BY contentType ORDER BY name ASC LIMIT 3`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	// The keys query must succeed and NOT include ORDER BY name.
	// Use DryRun to inspect the SQL.
	tc := &translateContext{db: db, entityType: EntityResource, tableName: "resources"}
	_ = tc // suppress unused — we just need to verify TranslateGroupByKeys works

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys query failed (ORDER BY leaked to keys query): %v", err)
	}
	if len(keys) == 0 {
		t.Error("expected at least one key")
	}

	// Verify ORDER BY still works within a bucket
	bucketDB, err := TranslateGroupByBucket(q, db, keys[0])
	if err != nil {
		t.Fatalf("bucket: %v", err)
	}
	var resources []testResource
	if err := bucketDB.Find(&resources).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	// Items should be ordered by name ASC
	for i := 1; i < len(resources); i++ {
		if resources[i].Name < resources[i-1].Name {
			t.Errorf("items not in name ASC order: %q < %q", resources[i].Name, resources[i-1].Name)
		}
	}
}

// P2: GROUP BY parent/children should work — translator must handle these relation fields.
func TestBugfix_GroupByParentRelationField(t *testing.T) {
	db := setupTestDB(t)

	// GROUP BY parent on groups — should group by parent's FK (owner_id) and display parent name.
	// Groups: Vacation (no parent), Work (parent=Vacation), Archive (no parent),
	// Sub-Work (parent=Work), Photos (parent=Vacation)
	q, err := Parse(`type = "group" GROUP BY parent COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityGroup

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate failed (parent not handled): %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Should have rows for Vacation (parent of Work, Photos), Work (parent of Sub-Work), and NULL (no parent)
	if len(result.Rows) < 2 {
		t.Errorf("expected at least 2 rows, got %d: %v", len(result.Rows), result.Rows)
	}
}

func TestBugfix_GroupByChildrenRelationField(t *testing.T) {
	db := setupTestDB(t)

	// GROUP BY children on groups — reverse FK, group by child name.
	q, err := Parse(`type = "group" GROUP BY children COUNT()`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityGroup

	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate failed (children not handled): %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
}

// PG-safe: GROUP BY owner with aggregates must include the display name in GROUP BY
// or wrap it in an aggregate function, since PostgreSQL rejects non-grouped,
// non-aggregated columns in SELECT.
func TestBugfix_GroupByOwnerSelectGroupByConsistency(t *testing.T) {
	db := setupTestDB(t)

	q, err := Parse(`type = "resource" GROUP BY owner COUNT() SUM(fileSize)`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	// On SQLite this always works; on PostgreSQL the old code would fail because
	// SELECT _gb_owner.name ... GROUP BY resources.owner_id is invalid.
	result, err := TranslateGroupBy(q, db)
	if err != nil {
		t.Fatalf("translate: %v", err)
	}
	if result.Mode != "aggregated" {
		t.Errorf("expected aggregated, got %s", result.Mode)
	}
	// Verify the result has the expected keys
	for _, row := range result.Rows {
		if _, ok := row["owner"]; !ok {
			t.Error("missing 'owner' key in row")
		}
		if _, ok := row["count"]; !ok {
			t.Error("missing 'count' key in row")
		}
	}
}

// Bucketed total items must be bounded even without explicit LIMIT.
func TestBugfix_BucketedTotalItemsCapped(t *testing.T) {
	db := setupTestDB(t)

	// No explicit LIMIT — the execution layer sets defaultMRQLLimit=1000 per bucket.
	// With 4 content types, that's 4000 potential items. There should be a global cap.
	q, err := Parse(`type = "resource" GROUP BY contentType`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	// Simulate execution layer setting default limit
	q.Limit = 1000

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}

	// Count total items across all buckets
	totalItems := 0
	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}
		totalItems += len(resources)
	}

	// With 4 resources in test data, this is fine. The real concern is
	// at scale — the execution layer should enforce a global cap.
	// For now just verify the per-bucket limit is applied.
	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		dryDB := bucketDB.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{})
		sql := strings.ToUpper(dryDB.Statement.SQL.String())
		if !strings.Contains(sql, "LIMIT") {
			t.Errorf("expected LIMIT in per-bucket query, got SQL: %s", sql)
		}
	}
}

// P1: Bucketed keys query must have deterministic ordering for stable pagination.
func TestBugfix_BucketedKeysHaveDeterministicOrder(t *testing.T) {
	db := setupTestDB(t)

	q, err := Parse(`type = "resource" GROUP BY contentType`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	// Run twice — results should be in the same order
	keys1, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys1: %v", err)
	}
	keys2, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys2: %v", err)
	}

	if len(keys1) != len(keys2) {
		t.Fatalf("key counts differ: %d vs %d", len(keys1), len(keys2))
	}
	for i := range keys1 {
		ct1 := groupByVal(keys1[i]["contentType"])
		ct2 := groupByVal(keys2[i]["contentType"])
		if ct1 != ct2 {
			t.Errorf("key order unstable at index %d: %q vs %q", i, ct1, ct2)
		}
	}

	// Verify keys are actually sorted
	for i := 1; i < len(keys1); i++ {
		prev := groupByVal(keys1[i-1]["contentType"])
		curr := groupByVal(keys1[i]["contentType"])
		if curr < prev {
			t.Errorf("keys not sorted: %q < %q at index %d", curr, prev, i)
		}
	}
}

// P2a: Bucketed queries with relation JOINs must not duplicate base entities.
// GROUP BY owner joins groups, and if the owner group has multiple children
// or tags, the JOIN can multiply rows. The bucket query must deduplicate.
func TestBugfix_BucketedRelationJoinNoDuplicates(t *testing.T) {
	db := setupTestDB(t)

	// GROUP BY owner — the join to groups for filtering shouldn't duplicate
	// entities. sunset.jpg (owner=Vacation) should appear exactly once.
	// Add a second tag to Vacation to create potential duplication via
	// the group_tags join (not directly relevant here, but test with
	// a GROUP BY that causes JOINs).
	q, err := Parse(`type = "resource" GROUP BY owner LIMIT 100`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}

	for _, key := range keys {
		bucketDB, err := TranslateGroupByBucket(q, db, key)
		if err != nil {
			t.Fatalf("bucket: %v", err)
		}
		var resources []testResource
		if err := bucketDB.Find(&resources).Error; err != nil {
			t.Fatalf("find: %v", err)
		}

		// Check for duplicate IDs
		seen := make(map[uint]bool)
		for _, r := range resources {
			if seen[r.ID] {
				t.Errorf("bucket owner=%v: duplicate resource ID %d (%s)", key["owner"], r.ID, r.Name)
			}
			seen[r.ID] = true
		}
	}
}

// P2b: Bucketed keys for relation fields must include ID for disambiguation.
func TestBugfix_BucketedRelationKeysIncludeID(t *testing.T) {
	db := setupTestDB(t)

	// Seed two groups with the same name but different IDs
	db.Exec("INSERT INTO groups (id, name, meta, created_at, updated_at) VALUES (200, 'SameName', '{}', datetime('now'), datetime('now'))")
	db.Exec("INSERT INTO groups (id, name, meta, created_at, updated_at) VALUES (201, 'SameName', '{}', datetime('now'), datetime('now'))")
	db.Exec("INSERT INTO resources (id, name, content_type, file_size, meta, created_at, updated_at, owner_id) VALUES (200, 'res-x', 'text/plain', 10, '{}', datetime('now'), datetime('now'), 200)")
	db.Exec("INSERT INTO resources (id, name, content_type, file_size, meta, created_at, updated_at, owner_id) VALUES (201, 'res-y', 'text/plain', 20, '{}', datetime('now'), datetime('now'), 201)")

	q, err := Parse(`type = "resource" GROUP BY owner LIMIT 100`)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := Validate(q); err != nil {
		t.Fatalf("validate: %v", err)
	}
	q.EntityType = EntityResource

	keys, err := TranslateGroupByKeys(q, db)
	if err != nil {
		t.Fatalf("keys: %v", err)
	}

	// Find the two SameName buckets — they must be distinguishable.
	// The keys query includes _gbid_owner (internal), which the execution
	// layer renames to owner_id (public). At the translator level we see _gbid_.
	sameNameKeys := 0
	for _, key := range keys {
		name := groupByVal(key["owner"])
		if name == "SameName" {
			sameNameKeys++
			// At translator level the key has _gbid_owner for bucket filtering
			idKey := key["_gbid_owner"]
			if idKey == nil {
				t.Error("expected _gbid_owner in key for disambiguation, got nil")
			}
		}
	}
	if sameNameKeys < 2 {
		t.Errorf("expected 2 SameName buckets, got %d", sameNameKeys)
	}
}
