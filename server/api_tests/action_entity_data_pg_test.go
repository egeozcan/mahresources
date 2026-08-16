//go:build postgres

package api_tests

import (
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// actionMatchesFilters asserts the entity data's types STRICTLY —
// entityData["category_id"].(uint), not a numeric conversion. A value that
// arrives as int64 fails the assertion, the entity is treated as not matching,
// and the whole filter feature becomes a no-op that looks like it works: every
// filtered action refuses everything, fail-closed and silent.
//
// The SQLite coverage lives in application_context/action_entity_data_reader_test.go.
// That is not enough on its own. SetupTestEnv is SQLite even under the postgres
// build tag — only SetupPostgresTestEnv reaches a real server — so every
// existing test of this reader, including the API-level ones in this package,
// has only ever measured one driver. Postgres returns int8/int4 where SQLite
// returns INTEGER, and the conversion happens in GORM's scan into the reader's
// typed struct rather than anywhere this package controls.
//
// So this test exists to measure the driver, not the logic.
func TestEntityFilterDataYieldsUintOnPostgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	category := &models.ResourceCategory{Name: "pg-filter-category"}
	if err := tc.DB.Create(category).Error; err != nil {
		t.Fatalf("create resource category: %v", err)
	}
	resource := &models.Resource{
		Name:               "pg-filter-resource",
		ContentType:        "image/png",
		Hash:               "pg-filter-hash",
		Location:           "pg-filter-location",
		ResourceCategoryId: category.ID,
	}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	noteType := &models.NoteType{Name: "pg-filter-notetype"}
	if err := tc.DB.Create(noteType).Error; err != nil {
		t.Fatalf("create note type: %v", err)
	}
	note := &models.Note{Name: "pg-filter-note", NoteTypeId: &noteType.ID}
	if err := tc.DB.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	groupCategory := &models.Category{Name: "pg-filter-group-category"}
	if err := tc.DB.Create(groupCategory).Error; err != nil {
		t.Fatalf("create category: %v", err)
	}
	group := &models.Group{Name: "pg-filter-group", CategoryId: &groupCategory.ID}
	if err := tc.DB.Create(group).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	reader := application_context.NewActionEntityDataReader(tc.AppCtx)

	cases := []struct {
		entity string
		id     uint
		key    string
		want   uint
	}{
		{"resource", resource.ID, "category_id", category.ID},
		{"note", note.ID, "note_type_id", noteType.ID},
		{"group", group.ID, "category_id", groupCategory.ID},
	}

	for _, c := range cases {
		data, err := reader.EntityFilterData(c.entity, []uint{c.id})
		if err != nil {
			t.Fatalf("EntityFilterData(%s): %v", c.entity, err)
		}
		fields, ok := data[c.id]
		if !ok {
			t.Fatalf("%s %d came back absent, which reads as a filter mismatch", c.entity, c.id)
		}
		raw, present := fields[c.key]
		if !present {
			t.Fatalf("%s %d has no %s; the filter can never match it", c.entity, c.id, c.key)
		}
		// The assertion actionMatchesFilters makes, made here directly.
		got, isUint := raw.(uint)
		if !isUint {
			t.Fatalf("%s.%s is %T on Postgres, not uint — actionMatchesFilters asserts .(uint), "+
				"so every filtered action would silently refuse every entity", c.entity, c.key, raw)
		}
		if got != c.want {
			t.Errorf("%s.%s = %d, want %d", c.entity, c.key, got, c.want)
		}
	}

	// content_type is asserted as a string on the same path.
	data, err := reader.EntityFilterData("resource", []uint{resource.ID})
	if err != nil {
		t.Fatalf("EntityFilterData(resource): %v", err)
	}
	ct, isString := data[resource.ID]["content_type"].(string)
	if !isString {
		t.Fatalf("resource.content_type is %T on Postgres, not string", data[resource.ID]["content_type"])
	}
	if ct != "image/png" {
		t.Errorf("content_type = %q, want image/png", ct)
	}
}
