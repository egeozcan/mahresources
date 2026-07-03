package mrql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

// parseAndValidate is a helper returning the validation error for a query.
func parseAndValidate(t *testing.T, input string, entityType EntityType) error {
	t.Helper()
	q, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = entityType
	return Validate(q)
}

func TestCountFieldValidationHappyPaths(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		entityType EntityType
	}{
		{"resource tags.count gt", `tags.count > 5`, EntityResource},
		{"resource tags.count eq zero", `tags.count = 0`, EntityResource},
		{"resource notes.count gte", `notes.count >= 1`, EntityResource},
		{"resource groups.count lt", `groups.count < 3`, EntityResource},
		{"note resources.count neq", `resources.count != 0`, EntityNote},
		{"note tags.count lte", `tags.count <= 10`, EntityNote},
		{"group children.count gte", `children.count >= 2`, EntityGroup},
		{"group resources.count gt", `resources.count > 100`, EntityGroup},
		{"group notes.count eq", `notes.count = 0`, EntityGroup},
		{"order by tags.count", `tags.count > 0 ORDER BY tags.count DESC`, EntityResource},
		{"order by resources.count on group", `type = "group" ORDER BY resources.count DESC`, EntityGroup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := parseAndValidate(t, tc.query, tc.entityType); err != nil {
				t.Fatalf("expected valid query, got: %v", err)
			}
		})
	}
}

func TestCountFieldValidationErrors(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		entityType EntityType
		wantSubstr string
	}{
		{"owner.count", `owner.count > 1`, EntityResource, `owner is a single reference and cannot be counted; use owner IS NULL / IS NOT NULL`},
		{"parent.count", `parent.count > 1`, EntityGroup, `parent is a single reference and cannot be counted; use parent IS NULL / IS NOT NULL`},
		{"count IN", `tags.count IN (1, 2)`, EntityResource, `tags.count only supports comparison operators (=, !=, >, >=, <, <=)`},
		{"count IS EMPTY", `tags.count IS EMPTY`, EntityResource, `tags.count only supports comparison operators (=, !=, >, >=, <, <=)`},
		{"count IS NULL", `tags.count IS NULL`, EntityResource, `tags.count only supports comparison operators (=, !=, >, >=, <, <=)`},
		{"count like", `tags.count ~ "5"`, EntityResource, `tags.count only supports comparison operators (=, !=, >, >=, <, <=)`},
		{"count string value", `tags.count > "many"`, EntityResource, `tags.count must be compared to a non-negative integer`},
		{"count float value", `tags.count > 1.5`, EntityResource, `tags.count must be compared to a non-negative integer`},
		{"count unit value", `tags.count > 2kb`, EntityResource, `tags.count must be compared to a non-negative integer`},
		{"children.count IN on group", `children.count IN (1)`, EntityGroup, `children.count only supports comparison operators (=, !=, >, >=, <, <=)`},
		{"count on wrong entity", `children.count > 1`, EntityResource, ``}, // any error is fine — children is group-only
		{"count cross-entity", `tags.count > 1`, EntityUnspecified, `tags.count requires an explicit entity type`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := parseAndValidate(t, tc.query, tc.entityType)
			if err == nil {
				t.Fatalf("expected validation error for %q, got nil", tc.query)
			}
			if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error message mismatch for %q:\nwant substring: %s\ngot: %v", tc.query, tc.wantSubstr, err)
			}
		})
	}
}

func TestCountFieldSQLShapes(t *testing.T) {
	db := setupTestDB(t)

	cases := []struct {
		name       string
		query      string
		entityType EntityType
		want       string
	}{
		{
			name: "resource tags.count where", query: `tags.count > 5`, entityType: EntityResource,
			want: `(SELECT COUNT(*) FROM resource_tags jt WHERE jt.resource_id = resources.id) > ?`,
		},
		{
			name: "resource tags.count zero", query: `tags.count = 0`, entityType: EntityResource,
			want: `(SELECT COUNT(*) FROM resource_tags jt WHERE jt.resource_id = resources.id) = ?`,
		},
		{
			name: "resource notes.count", query: `notes.count >= 1`, entityType: EntityResource,
			want: `(SELECT COUNT(*) FROM resource_notes jt WHERE jt.resource_id = resources.id) >= ?`,
		},
		{
			name: "group resources.count", query: `resources.count >= 100`, entityType: EntityGroup,
			want: `(SELECT COUNT(*) FROM groups_related_resources jt WHERE jt.group_id = groups.id) >= ?`,
		},
		{
			name: "group children.count", query: `children.count >= 2`, entityType: EntityGroup,
			want: `(SELECT COUNT(*) FROM groups c WHERE c.owner_id = groups.id) >= ?`,
		},
		{
			name: "order by tags.count", query: `ORDER BY tags.count DESC`, entityType: EntityResource,
			want: `ORDER BY (SELECT COUNT(*) FROM resource_tags jt WHERE jt.resource_id = resources.id) DESC`,
		},
		{
			name: "note groups.count", query: `groups.count = 0`, entityType: EntityNote,
			want: `(SELECT COUNT(*) FROM groups_related_notes jt WHERE jt.note_id = notes.id) = ?`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sql := dryRunSQL(t, db, tc.query, tc.entityType)
			if !strings.Contains(sql, tc.want) {
				t.Errorf("generated SQL does not contain expected fragment.\nquery: %s\nwant fragment: %s\ngot SQL: %s", tc.query, tc.want, sql)
			}
		})
	}
}

func TestCountFieldExecution(t *testing.T) {
	db := setupTestDB(t)

	t.Run("tags.count = 0 matches untagged", func(t *testing.T) {
		result := parseAndTranslate(t, `tags.count = 0`, EntityResource, db)
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		// resources 3 and 4 have no tags
		if len(resources) != 2 {
			t.Fatalf("expected 2 untagged resources, got %d: %v", len(resources), namesOfResources(resources))
		}
	})

	t.Run("tags.count >= 2 matches multi-tagged", func(t *testing.T) {
		result := parseAndTranslate(t, `tags.count >= 2`, EntityResource, db)
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(resources) != 1 || resources[0].Name != "photo_album.png" {
			t.Fatalf("expected [photo_album.png], got %v", namesOfResources(resources))
		}
	})

	t.Run("order by tags.count desc", func(t *testing.T) {
		result := parseAndTranslate(t, `type = "resource" ORDER BY tags.count DESC, id ASC`, EntityResource, db)
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(resources) != 4 {
			t.Fatalf("expected 4 resources, got %d", len(resources))
		}
		if resources[0].Name != "photo_album.png" {
			t.Errorf("expected photo_album.png first (2 tags), got %q", resources[0].Name)
		}
	})

	t.Run("children.count on groups", func(t *testing.T) {
		result := parseAndTranslate(t, `children.count >= 2`, EntityGroup, db)
		var groups []testGroup
		if err := result.Find(&groups).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		// Vacation has children Work and Photos
		if len(groups) != 1 || groups[0].Name != "Vacation" {
			t.Fatalf("expected [Vacation], got %v", namesOfGroups(groups))
		}
	})

	t.Run("notes.count = 0 on resources", func(t *testing.T) {
		result := parseAndTranslate(t, `notes.count = 0`, EntityResource, db)
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		// resources 2 and 4 have no notes
		if len(resources) != 2 {
			t.Fatalf("expected 2 resources without notes, got %d: %v", len(resources), namesOfResources(resources))
		}
	})
}

// TestCountFieldCrossEntityTypeGuardedOr covers count pseudo-fields inside
// type-guarded OR branches of a cross-entity query. executeCrossEntity
// validates once (cross-entity, per-branch types) and then translates the
// full AST once per entity type — so a count field that is only valid on
// another entity must translate to FALSE for the current entity, exactly
// like entity-specific scalar fields, instead of failing the whole entity
// query with a TranslateError (which would silently drop that entity's
// results).
func TestCountFieldCrossEntityTypeGuardedOr(t *testing.T) {
	db := setupTestDB(t)

	// Mimics executeCrossEntity: validate cross-entity, translate a per-entity clone.
	translateForEntity := func(t *testing.T, input string, et EntityType) *gorm.DB {
		t.Helper()
		q, err := Parse(input)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		if err := Validate(q); err != nil {
			t.Fatalf("validation error: %v", err)
		}
		clone := *q
		clone.EntityType = et
		result, err := Translate(&clone, db)
		if err != nil {
			t.Fatalf("translate error for %s: %v", et, err)
		}
		return result
	}

	query := `(type = "group" AND children.count > 0) OR (type = "resource" AND contentType ~ "image/*")`

	t.Run("resource branch survives group-only count field", func(t *testing.T) {
		// children.count is group-only; on the resource entity it must become
		// FALSE, leaving the image branch to match resources 1 and 2.
		result := translateForEntity(t, query, EntityResource)
		var resources []testResource
		if err := result.Order("id").Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(resources) != 2 || resources[0].ID != 1 || resources[1].ID != 2 {
			t.Fatalf("expected resources [1 2], got %v", namesOfResources(resources))
		}
	})

	t.Run("group branch matches groups with children", func(t *testing.T) {
		result := translateForEntity(t, query, EntityGroup)
		var groups []testGroup
		if err := result.Order("id").Find(&groups).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		// Vacation(1) and Work(2) have children.
		if len(groups) != 2 || groups[0].ID != 1 || groups[1].ID != 2 {
			t.Fatalf("expected groups [1 2], got %v", namesOfGroups(groups))
		}
	})

	t.Run("note entity matches nothing but does not error", func(t *testing.T) {
		result := translateForEntity(t, query, EntityNote)
		var notes []testNote
		if err := result.Find(&notes).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(notes) != 0 {
			t.Fatalf("expected no notes, got %d", len(notes))
		}
	})

	t.Run("junction-backed count field on wrong entity", func(t *testing.T) {
		// notes.count is valid on resources and groups but not notes: the note
		// entity clone must translate (FALSE branch), not error.
		q := `(type = "resource" AND notes.count > 0) OR (type = "note" AND name ~ "*meeting*")`
		result := translateForEntity(t, q, EntityNote)
		var notes []testNote
		if err := result.Find(&notes).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
	})
}
