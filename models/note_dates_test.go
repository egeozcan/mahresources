package models

import (
	"path/filepath"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A note's start and end dates are wall-clock values the host parses without a
// zone, as UTC. PostgreSQL hands a timestamptz back in the server's zone, so a
// due date written as 12:00 read back as 02:00 the next day in a UTC+14 server,
// the edit form offered that, and every save moved the date again. The row
// below is stored the way such a read labels it; loading it must give back the
// wall clock that was written, directly and through a preload.
func TestNoteDatesReadBackAsTheWallClockTheyWereWrittenAs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "notes.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&Note{}, &Group{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	owner := &Group{Name: "owner"}
	if err := db.Create(owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	plusFourteen := time.FixedZone("UTC+14", 14*3600)
	start := time.Date(2026, 9, 26, 23, 30, 0, 0, time.UTC).In(plusFourteen)
	end := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC).In(plusFourteen)
	note := &Note{Name: "due", OwnerId: &owner.ID, StartDate: &start, EndDate: &end}
	if err := db.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	check := func(how string, got *Note) {
		t.Helper()
		if got.StartDate == nil || got.StartDate.Format("2006-01-02T15:04") != "2026-09-26T23:30" {
			t.Errorf("%s: start date reads %v, want 2026-09-26T23:30", how, got.StartDate)
		}
		if got.EndDate == nil || got.EndDate.Format("2006-01-02T15:04") != "2026-09-27T12:00" {
			t.Errorf("%s: end date reads %v, want 2026-09-27T12:00", how, got.EndDate)
		}
	}

	var loaded Note
	if err := db.First(&loaded, note.ID).Error; err != nil {
		t.Fatalf("load note: %v", err)
	}
	check("First", &loaded)

	var group Group
	if err := db.Preload("OwnNotes").First(&group, owner.ID).Error; err != nil {
		t.Fatalf("load owner: %v", err)
	}
	if len(group.OwnNotes) != 1 {
		t.Fatalf("preloaded %d notes, want 1", len(group.OwnNotes))
	}
	check("Preload", group.OwnNotes[0])
}
