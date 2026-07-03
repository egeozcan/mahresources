package application_context

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// TestSlowQueryLogSink_WritesWarningEntries exercises the full slow-query
// pipeline: a traced connection with a tiny threshold, the sink wired into the
// application log, and a query that must surface as a warning LogEntry.
func TestSlowQueryLogSink_WritesWarningEntries(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, slowLogger, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", time.Nanosecond)
	if err != nil {
		t.Fatalf("CreateDatabaseConnection: %v", err)
	}
	if slowLogger == nil {
		t.Fatal("expected a SlowQueryLogger when threshold > 0")
	}
	if err := db.AutoMigrate(&models.LogEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, readOnlyDB, &MahresourcesConfig{
		DbType: constants.DbTypeSqlite,
	})
	ctx.StartSlowQueryLogSink(slowLogger)

	if err := db.Exec("SELECT 1").Error; err != nil {
		t.Fatalf("exec: %v", err)
	}

	// The sink writes asynchronously; poll on a silenced session (which
	// bypasses the slow-query logger) until the entry appears.
	silent := db.Session(&gorm.Session{Logger: gormlogger.Discard, NewDB: true})
	deadline := time.Now().Add(5 * time.Second)
	var count int64
	for time.Now().Before(deadline) {
		if err := silent.Model(&models.LogEntry{}).
			Where("entity_type = ? AND level = ?", "sql", models.LogLevelWarning).
			Count(&count).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		if count > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if count == 0 {
		t.Fatal("expected a slow-query LogEntry to be written to the application log")
	}

	var entry models.LogEntry
	if err := silent.Where("entity_type = ?", "sql").First(&entry).Error; err != nil {
		t.Fatalf("load entry: %v", err)
	}
	if entry.Action != models.LogActionSystem {
		t.Errorf("entry Action = %q, want %q", entry.Action, models.LogActionSystem)
	}
	if !strings.Contains(entry.Message, "SLOW SQL") {
		t.Errorf("entry Message = %q, want it to contain %q", entry.Message, "SLOW SQL")
	}
	if len(entry.Details) == 0 {
		t.Error("entry Details is empty, want JSON with sql/duration/rows")
	}

	// Recursion guard: the log-entry inserts themselves must never be logged.
	var selfLogged int64
	if err := silent.Model(&models.LogEntry{}).
		Where("entity_type = ? AND message LIKE ?", "sql", "%log_entries%").
		Count(&selfLogged).Error; err != nil {
		t.Fatalf("count self-logged: %v", err)
	}
	if selfLogged > 0 {
		t.Errorf("found %d slow-query entries about log_entries statements (recursion guard failed)", selfLogged)
	}
}
