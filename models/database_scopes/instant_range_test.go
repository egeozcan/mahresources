package database_scopes

import (
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openInstantRangeDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "instants.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec("CREATE TABLE stamps (id INTEGER PRIMARY KEY, at DATETIME)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec("CREATE INDEX idx_stamps_at ON stamps(at)").Error; err != nil {
		t.Fatalf("create index: %v", err)
	}
	return db
}

// Each stored text names an instant; the range holds exactly the instants in
// it, whichever offset, separator or precision the text was written with.
func TestInstantRangeComparesInstantsOnSQLite(t *testing.T) {
	db := openInstantRangeDB(t)
	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 19, 0, 0, 0, 0, time.UTC)

	rows := []struct {
		text string
		in   bool
	}{
		{"2026-01-11 23:59:59.998+00:00", false},
		{"2026-01-12 00:00:00+00:00", true},
		{"2026-01-12 13:59:00+14:00", false}, // 23:59 UTC the day before
		{"2026-01-12 14:00:00+14:00", true},
		{"2026-01-11 12:00:00-12:00", true}, // 00:00 UTC on the first day
		{"2026-01-11T11:59:00-12:00", false},
		{"2026-01-15 10:00:00.123456789+02:00", true},
		{"2026-01-18T23:30:00Z", true},
		{"2026-01-18 23:59:59", true}, // CURRENT_TIMESTAMP's shape, UTC
		{"2026-01-19 00:00:00", false},
		{"2026-01-19 13:30:00+14:00", true},  // 23:30 UTC on the last day
		{"2026-01-18 12:30:00-12:00", false}, // 00:30 UTC after the range
	}
	var want []int
	for i, row := range rows {
		if err := db.Exec("INSERT INTO stamps (id, at) VALUES (?, ?)", i+1, row.text).Error; err != nil {
			t.Fatalf("insert %q: %v", row.text, err)
		}
		if row.in {
			want = append(want, i+1)
		}
	}

	var got []int
	if err := db.Table("stamps").Scopes(InstantRange("at", start, end)).Pluck("id", &got).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	sort.Ints(got)
	if len(got) != len(want) {
		t.Fatalf("ids in range = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ids in range = %v, want %v", got, want)
		}
	}
}

// The bare column bounds the scan, so an index on it still answers the range.
func TestInstantRangeKeepsTheColumnIndexOnSQLite(t *testing.T) {
	db := openInstantRangeDB(t)
	start := time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 1, 19, 0, 0, 0, 0, time.UTC)

	stmt := db.Session(&gorm.Session{DryRun: true}).Table("stamps").
		Scopes(InstantRange("at", start, end)).Select("count(*)").Find(&[]map[string]any{}).Statement
	rows, err := db.Raw("EXPLAIN QUERY PLAN "+stmt.SQL.String(), stmt.Vars...).Rows()
	if err != nil {
		t.Fatalf("explain: %v", err)
	}
	defer rows.Close()
	var plan []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan = append(plan, detail)
	}
	joined := strings.Join(plan, "; ")
	if !strings.Contains(joined, "idx_stamps_at") {
		t.Fatalf("query plan does not use the column index: %s", joined)
	}
}

// A row is "after" only when its later column holds a later instant, however
// the two columns were written.
func TestInstantAfterComparesInstantsOnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "after.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec("CREATE TABLE pairs (id INTEGER PRIMARY KEY, created DATETIME, updated DATETIME)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	rows := []struct {
		created, updated string
		after            bool
	}{
		{"2026-01-13 01:05:00+02:00", "2026-01-12 23:10:00+00:00", true},  // five minutes later, sorts earlier as text
		{"2026-01-12 12:00:00+00:00", "2026-01-12 14:00:00+02:00", false}, // the same instant
		{"2026-01-12 12:00:00+00:00", "2026-01-12 13:00:00+02:00", false}, // an hour earlier, sorts later as text
		{"2026-01-12 12:00:00+00:00", "2026-01-12 12:00:00.5+00:00", true},
	}
	var want []int
	for i, row := range rows {
		if err := db.Exec("INSERT INTO pairs (id, created, updated) VALUES (?, ?, ?)", i+1, row.created, row.updated).Error; err != nil {
			t.Fatalf("insert: %v", err)
		}
		if row.after {
			want = append(want, i+1)
		}
	}
	var got []int
	if err := db.Table("pairs").Scopes(InstantAfter("updated", "created")).Order("id").Pluck("id", &got).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("rows updated after they were created = %v, want %v", got, want)
	}
}
