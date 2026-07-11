package mrql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

// dryRunSQL parses, validates, and translates the query, then captures the SQL
// that GORM would execute without hitting the database.
func dryRunSQL(t *testing.T, db *gorm.DB, input string, entityType EntityType) string {
	t.Helper()

	q, err := Parse(input)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	q.EntityType = entityType
	if err := Validate(q); err != nil {
		t.Fatalf("validation error: %v", err)
	}

	tx := db.Session(&gorm.Session{DryRun: true})
	result, err := Translate(q, tx)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}

	var rows []map[string]any
	stmt := result.Find(&rows).Statement
	return stmt.SQL.String()
}

// TestJunctionSQLShapes is the refactor safety net for the junction-relation
// translator: it pins the exact SQL fragments generated for tags/groups
// comparisons, IN, and IS EMPTY on all entities. The junction descriptor
// refactor must keep these byte-identical.
func TestJunctionSQLShapes(t *testing.T) {
	db := setupTestDB(t)

	cases := []struct {
		name       string
		query      string
		entityType EntityType
		want       string
	}{
		{
			name: "resource tags equality", query: `tags = "photo"`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) = LOWER(?))`,
		},
		{
			name: "resource tags numeric ID", query: `tags = 42`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE t.id = ?)`,
		},
		{
			name: "resource tags negation", query: `tags != "photo"`, entityType: EntityResource,
			want: `resources.id NOT IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) = LOWER(?))`,
		},
		{
			name: "resource tags like", query: `tags ~ "pho*"`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) LIKE LOWER(?) ESCAPE '\')`,
		},
		{
			name: "resource tags not like", query: `tags !~ "pho*"`, entityType: EntityResource,
			want: `resources.id NOT IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) LIKE LOWER(?) ESCAPE '\')`,
		},
		{
			name: "note tags equality", query: `tags = "photo"`, entityType: EntityNote,
			want: `notes.id IN (SELECT jt.note_id FROM note_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) = LOWER(?))`,
		},
		{
			name: "group tags equality", query: `tags = "photo"`, entityType: EntityGroup,
			want: `groups.id IN (SELECT jt.group_id FROM group_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) = LOWER(?))`,
		},
		{
			name: "resource groups equality", query: `groups = "Vacation"`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM groups_related_resources jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) = LOWER(?))`,
		},
		{
			name: "resource groups negation", query: `groups != "Vacation"`, entityType: EntityResource,
			want: `resources.id NOT IN (SELECT jt.resource_id FROM groups_related_resources jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) = LOWER(?))`,
		},
		{
			name: "resource groups like", query: `groups ~ "Vaca*"`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM groups_related_resources jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) LIKE LOWER(?) ESCAPE '\')`,
		},
		{
			name: "note groups equality", query: `groups = "Work"`, entityType: EntityNote,
			want: `notes.id IN (SELECT jt.note_id FROM groups_related_notes jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) = LOWER(?))`,
		},
		{
			name: "resource tags IN", query: `tags IN ("photo", "video")`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) IN (?,?))`,
		},
		{
			name: "resource tags NOT IN", query: `tags NOT IN ("photo", "video")`, entityType: EntityResource,
			want: `resources.id NOT IN (SELECT jt.resource_id FROM resource_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) IN (?,?))`,
		},
		{
			name: "note tags IN", query: `tags IN ("photo")`, entityType: EntityNote,
			want: `notes.id IN (SELECT jt.note_id FROM note_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) IN (?))`,
		},
		{
			name: "group tags IN", query: `tags IN ("photo")`, entityType: EntityGroup,
			want: `groups.id IN (SELECT jt.group_id FROM group_tags jt JOIN tags t ON t.id = jt.tag_id WHERE LOWER(t.name) IN (?))`,
		},
		{
			name: "resource groups IN", query: `groups IN ("Vacation", "Work")`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM groups_related_resources jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) IN (?,?))`,
		},
		{
			name: "note groups NOT IN", query: `groups NOT IN ("Work")`, entityType: EntityNote,
			want: `notes.id NOT IN (SELECT jt.note_id FROM groups_related_notes jt JOIN groups g ON g.id = jt.group_id WHERE LOWER(g.name) IN (?))`,
		},
		{
			name: "resource tags IS EMPTY", query: `tags IS EMPTY`, entityType: EntityResource,
			want: `NOT EXISTS (SELECT 1 FROM resource_tags jt WHERE jt.resource_id = resources.id)`,
		},
		{
			name: "resource tags IS NOT EMPTY", query: `tags IS NOT EMPTY`, entityType: EntityResource,
			want: `EXISTS (SELECT 1 FROM resource_tags jt WHERE jt.resource_id = resources.id)`,
		},
		{
			name: "note tags IS EMPTY", query: `tags IS EMPTY`, entityType: EntityNote,
			want: `NOT EXISTS (SELECT 1 FROM note_tags jt WHERE jt.note_id = notes.id)`,
		},
		{
			name: "group tags IS EMPTY", query: `tags IS EMPTY`, entityType: EntityGroup,
			want: `NOT EXISTS (SELECT 1 FROM group_tags jt WHERE jt.group_id = groups.id)`,
		},
		{
			name: "resource groups IS EMPTY", query: `groups IS EMPTY`, entityType: EntityResource,
			want: `NOT EXISTS (SELECT 1 FROM groups_related_resources jt WHERE jt.resource_id = resources.id)`,
		},
		{
			name: "resource similar images IS NOT EMPTY", query: `similarImages IS NOT EMPTY`, entityType: EntityResource,
			want: `EXISTS (SELECT 1 FROM image_hashes ih WHERE ih.resource_id = resources.id AND EXISTS (SELECT 1 FROM image_hashes i WHERE ih.d_hash = i.d_hash AND ih.id <> i.id))`,
		},
		{
			name: "note groups IS NOT EMPTY", query: `groups IS NOT EMPTY`, entityType: EntityNote,
			want: `EXISTS (SELECT 1 FROM groups_related_notes jt WHERE jt.note_id = notes.id)`,
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

// TestNewRelationSQLShapes pins the SQL generated for the notes/resources
// relation fields introduced by the junction descriptor refactor.
func TestNewRelationSQLShapes(t *testing.T) {
	db := setupTestDB(t)

	cases := []struct {
		name       string
		query      string
		entityType EntityType
		want       string
	}{
		{
			name: "resource notes equality", query: `notes = "Meeting notes"`, entityType: EntityResource,
			want: `resources.id IN (SELECT jt.resource_id FROM resource_notes jt JOIN notes n ON n.id = jt.note_id WHERE LOWER(n.name) = LOWER(?))`,
		},
		{
			name: "resource notes IS EMPTY", query: `notes IS EMPTY`, entityType: EntityResource,
			want: `NOT EXISTS (SELECT 1 FROM resource_notes jt WHERE jt.resource_id = resources.id)`,
		},
		{
			name: "note resources equality", query: `resources = "sunset.jpg"`, entityType: EntityNote,
			want: `notes.id IN (SELECT jt.note_id FROM resource_notes jt JOIN resources r ON r.id = jt.resource_id WHERE LOWER(r.name) = LOWER(?))`,
		},
		{
			name: "note resources IN", query: `resources IN ("sunset.jpg", "report.pdf")`, entityType: EntityNote,
			want: `notes.id IN (SELECT jt.note_id FROM resource_notes jt JOIN resources r ON r.id = jt.resource_id WHERE LOWER(r.name) IN (?,?))`,
		},
		{
			name: "group resources equality", query: `resources = "sunset.jpg"`, entityType: EntityGroup,
			want: `groups.id IN (SELECT jt.group_id FROM groups_related_resources jt JOIN resources r ON r.id = jt.resource_id WHERE LOWER(r.name) = LOWER(?))`,
		},
		{
			name: "group notes like", query: `notes ~ "Meeting*"`, entityType: EntityGroup,
			want: `groups.id IN (SELECT jt.group_id FROM groups_related_notes jt JOIN notes n ON n.id = jt.note_id WHERE LOWER(n.name) LIKE LOWER(?) ESCAPE '\')`,
		},
		{
			name: "group notes IS NOT EMPTY", query: `notes IS NOT EMPTY`, entityType: EntityGroup,
			want: `EXISTS (SELECT 1 FROM groups_related_notes jt WHERE jt.group_id = groups.id)`,
		},
		{
			name: "resource notes NOT IN", query: `notes NOT IN ("Meeting notes")`, entityType: EntityResource,
			want: `resources.id NOT IN (SELECT jt.resource_id FROM resource_notes jt JOIN notes n ON n.id = jt.note_id WHERE LOWER(n.name) IN (?))`,
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

// TestNewRelationExecution exercises the new notes/resources relation fields
// against seeded data.
func TestNewRelationExecution(t *testing.T) {
	db := setupTestDB(t)

	t.Run("resource notes equality", func(t *testing.T) {
		result := parseAndTranslate(t, `notes = "Meeting notes"`, EntityResource, db)
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
			t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
		}
	})

	t.Run("resource notes IS EMPTY", func(t *testing.T) {
		result := parseAndTranslate(t, `notes IS EMPTY`, EntityResource, db)
		var resources []testResource
		if err := result.Find(&resources).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		// resources 2 and 4 have no linked notes
		if len(resources) != 2 {
			t.Fatalf("expected 2 resources without notes, got %d: %v", len(resources), namesOfResources(resources))
		}
	})

	t.Run("note resources equality", func(t *testing.T) {
		result := parseAndTranslate(t, `resources = "report.pdf"`, EntityNote, db)
		var notes []testNote
		if err := result.Find(&notes).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(notes) != 1 || notes[0].Name != "Todo list" {
			t.Fatalf("expected [Todo list], got %v", namesOfNotes(notes))
		}
	})

	t.Run("group resources equality", func(t *testing.T) {
		result := parseAndTranslate(t, `resources = "sunset.jpg"`, EntityGroup, db)
		var groups []testGroup
		if err := result.Find(&groups).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		if len(groups) != 1 || groups[0].Name != "Vacation" {
			t.Fatalf("expected [Vacation], got %v", namesOfGroups(groups))
		}
	})

	t.Run("group notes IS NOT EMPTY", func(t *testing.T) {
		result := parseAndTranslate(t, `notes IS NOT EMPTY`, EntityGroup, db)
		var groups []testGroup
		if err := result.Find(&groups).Error; err != nil {
			t.Fatalf("query error: %v", err)
		}
		// groups 1 (Vacation) and 2 (Work) have related notes
		if len(groups) != 2 {
			t.Fatalf("expected 2 groups with notes, got %d: %v", len(groups), namesOfGroups(groups))
		}
	})

	t.Run("notes relation rejects unsupported operator", func(t *testing.T) {
		q, err := Parse(`notes > 3`)
		if err != nil {
			t.Fatalf("parse error: %v", err)
		}
		q.EntityType = EntityResource
		if err := Validate(q); err == nil {
			t.Fatal("expected validation error for notes > 3, got nil")
		}
	})
}
