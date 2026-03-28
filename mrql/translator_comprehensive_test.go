package mrql

import (
	"sort"
	"testing"
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
		{"group tags is empty", `type = "group" AND tags IS EMPTY`, EntityGroup, 4, []string{"Work", "Archive", "Sub-Work", "Photos"}},
		{"group tags is not empty", `type = "group" AND tags IS NOT EMPTY`, EntityGroup, 1, []string{"Vacation"}},
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
