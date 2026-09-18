//go:build postgres

package mrql

import (
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestPG_ResourceNameContains(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `name ~ "sunset"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

func TestPG_ContentTypeContains(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `contentType ~ "image"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 image resources, got %d: %v", len(resources), namesOfResources(resources))
	}
}

func TestPG_TagsEqual(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 photo-tagged resources, got %d", len(resources))
	}
}

func TestPG_MetaJsonExtract(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `meta.rating > 3`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg] with rating > 3, got %v", namesOfResources(resources))
	}
}

func TestPG_MetaJsonBooleanComparison(t *testing.T) {
	db := setupPostgresTestDB(t)
	metadata := map[uint]string{
		1: `{"trending":true,"info":{"el":{"node":{"music_metadata":{"original_sound_info":{"consumption_info":{"is_trending_in_clips":true}}}}}}}`,
		2: `{"trending":"true","info":{"el":{"node":{"music_metadata":{"original_sound_info":{"consumption_info":{"is_trending_in_clips":"true"}}}}}}}`,
		3: `{"trending":false,"info":{"el":{"node":{"music_metadata":{"original_sound_info":{"consumption_info":{"is_trending_in_clips":false}}}}}}}`,
	}
	for id, meta := range metadata {
		if err := db.Model(&testResource{}).Where("id = ?", id).Update("meta", meta).Error; err != nil {
			t.Fatalf("seed resource %d metadata: %v", id, err)
		}
	}
	if err := db.Model(&testResource{}).Where("id IN ?", []uint{1, 2}).Update("resource_category_id", 2).Error; err != nil {
		t.Fatalf("seed resource category 2: %v", err)
	}
	if err := db.Model(&testResource{}).Where("id = ?", 3).Update("resource_category_id", 3).Error; err != nil {
		t.Fatalf("seed resource category 3: %v", err)
	}

	tests := []struct {
		name     string
		query    string
		wantName string
	}{
		{name: "true", query: `meta.trending = true`, wantName: "sunset.jpg"},
		{name: "false", query: `meta.trending = false`, wantName: "report.pdf"},
		{
			name:     "production nested path",
			query:    `category = 2 AND meta.info.el.node.music_metadata.original_sound_info.consumption_info.is_trending_in_clips = true`,
			wantName: "sunset.jpg",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAndTranslate(t, tt.query, EntityResource, db)
			var resources []testResource
			if err := result.Find(&resources).Error; err != nil {
				t.Fatalf("query error: %v", err)
			}
			if len(resources) != 1 || resources[0].Name != tt.wantName {
				t.Fatalf("expected [%s], got %v", tt.wantName, namesOfResources(resources))
			}
		})
	}
}

func TestPG_OwnerDirect(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner = "Vacation"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

func TestPG_OwnerTags(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "sunset.jpg" {
		t.Fatalf("expected [sunset.jpg], got %v", namesOfResources(resources))
	}
}

func TestPG_OwnerParentChain(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.parent.name = "Vacation"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

func TestPG_OwnerParentTags(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.parent.tags = "photo"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 1 || resources[0].Name != "report.pdf" {
		t.Fatalf("expected [report.pdf], got %v", namesOfResources(resources))
	}
}

func TestPG_ParentParentChain(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `parent.parent.name = "Vacation"`, EntityGroup, db)
	var groups []testGroup
	if err := result.Find(&groups).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(groups) != 1 || groups[0].Name != "Sub-Work" {
		t.Fatalf("expected [Sub-Work], got %v", namesOfGroups(groups))
	}
}

func TestPG_OwnerNegationNull(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner != "Work"`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d: %v", len(resources), namesOfResources(resources))
	}
}

func TestPG_NoteOwner(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner.tags = "document"`, EntityNote, db)
	var notes []testNote
	if err := result.Find(&notes).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(notes) != 1 || notes[0].Name != "Todo list" {
		t.Fatalf("expected [Todo list], got %v", namesOfNotes(notes))
	}
}

func TestPG_OwnerIsEmpty(t *testing.T) {
	db := setupPostgresTestDB(t)
	result := parseAndTranslate(t, `owner IS EMPTY`, EntityResource, db)
	var resources []testResource
	if err := result.Find(&resources).Error; err != nil {
		t.Fatalf("query error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources with no owner, got %d: %v", len(resources), namesOfResources(resources))
	}
}

func TestPG_OrderByRandomSQL(t *testing.T) {
	db := setupPostgresTestDB(t)
	q := mustParse(t, `type = "resource" ORDER BY RANDOM()`)
	q.EntityType = EntityResource
	if err := Validate(q); err != nil {
		t.Fatalf("validate error: %v", err)
	}
	result, err := Translate(q, db)
	if err != nil {
		t.Fatalf("translate error: %v", err)
	}
	sql := result.Session(&gorm.Session{DryRun: true}).Find(&[]testResource{}).Statement.SQL.String()
	if !strings.Contains(strings.ToUpper(sql), "ORDER BY RANDOM()") {
		t.Fatalf("expected ORDER BY RANDOM() in SQL, got: %s", sql)
	}
}
