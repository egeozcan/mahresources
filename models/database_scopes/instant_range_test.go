package database_scopes

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
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
		// Within one millisecond, which julianday() cannot tell apart; the text
		// still can when both carry the same offset.
		{"2026-01-12 12:00:00.0001+00:00", "2026-01-12 12:00:00.0004+00:00", true},
		{"2026-01-12 12:00:00.0004+00:00", "2026-01-12 12:00:00.0001+00:00", false},
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

func TestInstantAfterComparesSupportedSQLiteTimestampFormsPrecisely(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "after-forms.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec("CREATE TABLE pairs (id INTEGER PRIMARY KEY, created DATETIME, updated DATETIME)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	type timestampForm struct {
		name   string
		format func(time.Time) string
	}
	forms := make([]timestampForm, 0, 21)
	for i, layout := range sqlite3.SQLiteTimestampFormats {
		layout := layout
		forms = append(forms, timestampForm{
			name: fmt.Sprintf("go-sqlite3 format %d (%s)", i, layout),
			format: func(value time.Time) string {
				return value.UTC().Format(layout)
			},
		})
	}
	for _, zone := range []struct {
		name          string
		offsetSeconds int
	}{{"west", -12 * 60 * 60}, {"east", 2 * 60 * 60}, {"far-east", 14 * 60 * 60}, {"half-hour-east", 5*60*60 + 30*60}, {"quarter-hour-west", -3*60*60 - 45*60}} {
		location := time.FixedZone(zone.name, zone.offsetSeconds)
		for _, separator := range []string{" ", "T"} {
			separator := separator
			layout := "2006-01-02" + separator + "15:04:05.999999999-07:00"
			forms = append(forms, timestampForm{
				name: fmt.Sprintf("%s offset %s", zone.name, separator),
				format: func(value time.Time) string {
					return value.In(location).Format(layout)
				},
			})
		}
	}
	forms = append(forms,
		timestampForm{
			name: "RFC3339Nano Z",
			format: func(value time.Time) string {
				return value.UTC().Format(time.RFC3339Nano)
			},
		},
		timestampForm{
			name: "RFC3339 Z without fraction",
			format: func(value time.Time) string {
				return value.UTC().Format(time.RFC3339)
			},
		},
		timestampForm{
			name: "negative zero offset",
			format: func(value time.Time) string {
				return strings.ReplaceAll(value.UTC().Format(sqlite3.SQLiteTimestampFormats[0]), "+00:00", "-00:00")
			},
		},
		timestampForm{
			name: "nine digit fractional padding",
			format: func(value time.Time) string {
				return value.UTC().Format("2006-01-02 15:04:05.000000000-07:00")
			},
		},
		timestampForm{
			name: "Go parser comma fraction",
			format: func(value time.Time) string {
				return strings.Replace(value.UTC().Format(sqlite3.SQLiteTimestampFormats[0]), ".", ",", 1)
			},
		},
		timestampForm{
			name: "historical CURRENT_TIMESTAMP",
			format: func(value time.Time) string {
				return value.UTC().Format("2006-01-02 15:04:05")
			},
		},
	)

	parse := func(text string) time.Time {
		t.Helper()
		for _, layout := range append(slices.Clone(sqlite3.SQLiteTimestampFormats), time.RFC3339Nano, time.RFC3339) {
			if value, err := time.ParseInLocation(layout, text, time.UTC); err == nil {
				return value
			}
		}
		t.Fatalf("parse SQLite timestamp %q using the go-sqlite3 layouts", text)
		return time.Time{}
	}

	origin := time.Date(2026, time.January, 14, 12, 34, 56, 100_000, time.UTC)
	wholeSecond := time.Date(2026, time.January, 14, 12, 34, 56, 0, time.UTC)
	relations := []struct {
		name             string
		created, updated time.Time
	}{
		{"before", origin.Add(300 * time.Microsecond), origin},
		{"equal", origin, origin},
		{"equal at a whole second across absent and zero fractions", wholeSecond, wholeSecond},
		{"after", origin, origin.Add(300 * time.Microsecond)},
	}
	type pair struct {
		id                       int
		createdForm, updatedForm string
		created, updated         string
		want                     bool
	}
	var cases []pair
	var want []int
	for _, createdForm := range forms {
		for _, updatedForm := range forms {
			for _, relation := range relations {
				created := createdForm.format(relation.created)
				updated := updatedForm.format(relation.updated)
				id := len(cases) + 1
				expected := parse(updated).After(parse(created))
				if err := db.Exec("INSERT INTO pairs (id, created, updated) VALUES (?, ?, ?)", id, created, updated).Error; err != nil {
					t.Fatalf("insert %s -> %s (%s): %v", createdForm.name, updatedForm.name, relation.name, err)
				}
				cases = append(cases, pair{
					id: id, createdForm: createdForm.name, updatedForm: updatedForm.name,
					created: created, updated: updated, want: expected,
				})
				if expected {
					want = append(want, id)
				}
			}
		}
	}

	var got []int
	if err := db.Table("pairs").Scopes(InstantAfter("updated", "created")).Order("id").Pluck("id", &got).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if !slices.Equal(got, want) {
		gotSet := make(map[int]bool, len(got))
		for _, id := range got {
			gotSet[id] = true
		}
		wantSet := make(map[int]bool, len(want))
		for _, id := range want {
			wantSet[id] = true
		}
		for _, row := range cases {
			if gotSet[row.id] != wantSet[row.id] {
				t.Errorf("InstantAfter(%s -> %s) for %q then %q = %t, want %t",
					row.createdForm, row.updatedForm, row.created, row.updated, gotSet[row.id], row.want)
			}
		}
		t.Fatalf("rows updated after they were created = %d rows, want %d", len(got), len(want))
	}
}

func TestInstantAfterOrdersNanosecondsAtYearZeroAnd9999(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "after-year-boundaries.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec("CREATE TABLE pairs (id INTEGER PRIMARY KEY, created DATETIME, updated DATETIME)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	rows := []struct {
		created, updated string
	}{
		{"0000-01-01 00:00:00.000000000+00:00", "0000-01-01 00:00:00.000000001+00:00"},
		{"9999-12-31 23:59:59.999999998+00:00", "9999-12-31 23:59:59.999999999+00:00"},
	}
	for i, row := range rows {
		if err := db.Exec("INSERT INTO pairs (id, created, updated) VALUES (?, ?, ?)", i+1, row.created, row.updated).Error; err != nil {
			t.Fatalf("insert year boundary pair %q -> %q: %v", row.created, row.updated, err)
		}
	}
	var ids []int
	if err := db.Table("pairs").Scopes(InstantAfter("updated", "created")).Order("id").Pluck("id", &ids).Error; err != nil {
		t.Fatalf("query: %v", err)
	}
	if !slices.Equal(ids, []int{1, 2}) {
		t.Fatalf("InstantAfter at the Go year boundaries returned %v, want [1 2]", ids)
	}
}

func TestInstantAfterComparesGoSQLiteBindingsAtNanosecondBoundaries(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "after-bindings.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec("CREATE TABLE pairs (id INTEGER PRIMARY KEY, created DATETIME, updated DATETIME)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}

	utc := time.UTC
	east := time.FixedZone("east", 2*60*60)
	halfHourEast := time.FixedZone("half-hour-east", 5*60*60+30*60)
	quarterHourWest := time.FixedZone("quarter-hour-west", -3*60*60-45*60)
	second := time.Date(2026, time.January, 14, 12, 34, 56, 0, utc)
	rows := []struct {
		name             string
		created, updated time.Time
		after            bool
	}{
		{"one nanosecond after zero", second, second.Add(time.Nanosecond), true},
		{"one nanosecond before zero", second.Add(time.Nanosecond), second, false},
		{"one nanosecond across a second", second.Add(time.Second - time.Nanosecond), second.Add(time.Second), true},
		{"one nanosecond before a second", second.Add(time.Second), second.Add(time.Second - time.Nanosecond), false},
		{"300 microseconds later in another offset", second.Add(100 * time.Microsecond), second.Add(400 * time.Microsecond).In(east), true},
		{"300 microseconds earlier in another offset", second.Add(400 * time.Microsecond).In(east), second.Add(100 * time.Microsecond), false},
		{"300 microseconds later with a half-hour offset", second.Add(100 * time.Microsecond), second.Add(400 * time.Microsecond).In(halfHourEast), true},
		{"300 microseconds earlier with a quarter-hour offset", second.Add(400 * time.Microsecond).In(quarterHourWest), second.Add(100 * time.Microsecond), false},
		{"equal across offsets", second, second.In(east), false},
	}
	var want []int
	for i, row := range rows {
		if err := db.Exec("INSERT INTO pairs (id, created, updated) VALUES (?, ?, ?)", i+1, row.created, row.updated).Error; err != nil {
			t.Fatalf("insert %s: %v", row.name, err)
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

	if err := db.Exec("INSERT INTO pairs (id, created, updated) VALUES (100, ?, ?)",
		"2026-01-14 12:34:56", second.Add(100*time.Nanosecond)).Error; err != nil {
		t.Fatalf("insert CURRENT_TIMESTAMP followed by a Go time: %v", err)
	}
	var gotCurrentTimestamp int64
	if err := db.Table("pairs").Where("id = 100").Scopes(InstantAfter("updated", "created")).Count(&gotCurrentTimestamp).Error; err != nil {
		t.Fatalf("query CURRENT_TIMESTAMP followed by a Go time: %v", err)
	}
	if gotCurrentTimestamp != 1 {
		t.Fatalf("CURRENT_TIMESTAMP followed by a 100ns Go update matched %d rows, want 1", gotCurrentTimestamp)
	}
}

func TestInstantAfterGroupsSQLiteBranchesBeforeCombiningScopes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "after-scope-grouping.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Exec("CREATE TABLE pairs (id INTEGER PRIMARY KEY, created DATETIME, updated DATETIME)").Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := db.Exec("INSERT INTO pairs (id, created, updated) VALUES (1, ?, ?), (2, ?, ?)",
		"2026-01-14 12:34:56", "2026-01-14 12:34:57", 1, 2).Error; err != nil {
		t.Fatalf("insert mixed representations: %v", err)
	}
	var numericIDs []int
	if err := db.Table("pairs").Where("id = ?", 2).Scopes(InstantAfter("updated", "created")).Order("id").Pluck("id", &numericIDs).Error; err != nil {
		t.Fatalf("query legacy numeric timestamps: %v", err)
	}
	if !slices.Equal(numericIDs, []int{2}) {
		t.Fatalf("InstantAfter on legacy numeric timestamps returned %v, want [2]", numericIDs)
	}

	var ids []int
	if err := db.Table("pairs").Scopes(InstantAfter("updated", "created")).Where("id = ?", 1).Order("id").Pluck("id", &ids).Error; err != nil {
		t.Fatalf("query combined scope: %v", err)
	}
	if !slices.Equal(ids, []int{1}) {
		t.Fatalf("InstantAfter combined with an id filter returned %v, want [1]", ids)
	}
}
