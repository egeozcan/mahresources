package models

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm/logger"
)

func TestDbLoggerConfig(t *testing.T) {
	tests := []struct {
		name          string
		logType       string
		threshold     time.Duration
		wantLevel     logger.LogLevel
		wantThreshold time.Duration
	}{
		{"disabled default", "", 0, 0, 0},
		{"threshold only logs slow queries", "", 200 * time.Millisecond, logger.Warn, 200 * time.Millisecond},
		{"stdout without threshold", "STDOUT", 0, logger.Info, 0},
		{"stdout with threshold", "STDOUT", 200 * time.Millisecond, logger.Info, 200 * time.Millisecond},
		{"file with threshold", "/tmp/db.log", 150 * time.Millisecond, logger.Info, 150 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dbLoggerConfig(tt.logType, tt.threshold)
			if got.LogLevel != tt.wantLevel {
				t.Errorf("dbLoggerConfig(%q, %v).LogLevel = %v, want %v", tt.logType, tt.threshold, got.LogLevel, tt.wantLevel)
			}
			if got.SlowThreshold != tt.wantThreshold {
				t.Errorf("dbLoggerConfig(%q, %v).SlowThreshold = %v, want %v", tt.logType, tt.threshold, got.SlowThreshold, tt.wantThreshold)
			}
		})
	}
}

// recordingLogger is a fake logger.Interface that counts Trace delegations.
type recordingLogger struct {
	traced int
}

func (r *recordingLogger) LogMode(logger.LogLevel) logger.Interface      { return r }
func (r *recordingLogger) Info(context.Context, string, ...interface{})  {}
func (r *recordingLogger) Warn(context.Context, string, ...interface{})  {}
func (r *recordingLogger) Error(context.Context, string, ...interface{}) {}
func (r *recordingLogger) Trace(context.Context, time.Time, func() (string, int64), error) {
	r.traced++
}

func TestSlowQueryLoggerTrace(t *testing.T) {
	fc := func(sql string) func() (string, int64) {
		return func() (string, int64) { return sql, 3 }
	}
	slowBegin := time.Now().Add(-time.Second)

	t.Run("fires at or above threshold and delegates", func(t *testing.T) {
		wrapped := &recordingLogger{}
		sl := newSlowQueryLogger(wrapped, time.Millisecond)
		var got []SlowQueryEntry
		sl.SetSink(func(e SlowQueryEntry) { got = append(got, e) })

		sl.Trace(context.Background(), slowBegin, fc("SELECT * FROM resources"), nil)

		if wrapped.traced != 1 {
			t.Errorf("wrapped logger traced %d times, want 1", wrapped.traced)
		}
		if len(got) != 1 {
			t.Fatalf("sink received %d entries, want 1", len(got))
		}
		if got[0].SQL != "SELECT * FROM resources" {
			t.Errorf("entry SQL = %q", got[0].SQL)
		}
		if got[0].Rows != 3 {
			t.Errorf("entry Rows = %d, want 3", got[0].Rows)
		}
		if got[0].Elapsed < time.Second {
			t.Errorf("entry Elapsed = %v, want >= 1s", got[0].Elapsed)
		}
	})

	t.Run("does not fire below threshold", func(t *testing.T) {
		sl := newSlowQueryLogger(&recordingLogger{}, time.Hour)
		fired := false
		sl.SetSink(func(SlowQueryEntry) { fired = true })

		sl.Trace(context.Background(), time.Now(), fc("SELECT 1"), nil)

		if fired {
			t.Error("sink fired for a fast query")
		}
	})

	t.Run("skips statements touching log_entries", func(t *testing.T) {
		sl := newSlowQueryLogger(&recordingLogger{}, time.Millisecond)
		fired := false
		sl.SetSink(func(SlowQueryEntry) { fired = true })

		sl.Trace(context.Background(), slowBegin, fc(`INSERT INTO "log_entries" (level) VALUES ('warning')`), nil)

		if fired {
			t.Error("sink fired for a log_entries statement (recursion guard failed)")
		}
	})

	t.Run("safe with no sink set", func(t *testing.T) {
		wrapped := &recordingLogger{}
		sl := newSlowQueryLogger(wrapped, time.Millisecond)

		sl.Trace(context.Background(), slowBegin, fc("SELECT 1"), nil)

		if wrapped.traced != 1 {
			t.Errorf("wrapped logger traced %d times, want 1", wrapped.traced)
		}
	})

	t.Run("LogMode copies share the sink", func(t *testing.T) {
		sl := newSlowQueryLogger(&recordingLogger{}, time.Millisecond)
		copied := sl.LogMode(logger.Info)
		fired := false
		sl.SetSink(func(SlowQueryEntry) { fired = true })

		copied.Trace(context.Background(), slowBegin, fc("SELECT 1"), nil)

		if !fired {
			t.Error("sink set on original did not fire via LogMode copy")
		}
	})
}

func TestCreateDatabaseConnectionSlowLogger(t *testing.T) {
	t.Run("nil slow logger when threshold is zero", func(t *testing.T) {
		db, slowLogger, err := CreateDatabaseConnection("SQLITE", "file:slowlog_off?mode=memory&cache=private", "", 0)
		if err != nil {
			t.Fatalf("CreateDatabaseConnection: %v", err)
		}
		if slowLogger != nil {
			t.Error("expected nil SlowQueryLogger when threshold is 0")
		}
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})

	t.Run("creates log file and reports slow queries", func(t *testing.T) {
		logFile := filepath.Join(t.TempDir(), "db.log")
		db, slowLogger, err := CreateDatabaseConnection("SQLITE", "file:slowlog_on?mode=memory&cache=private", logFile, time.Nanosecond)
		if err != nil {
			t.Fatalf("CreateDatabaseConnection: %v", err)
		}
		if slowLogger == nil {
			t.Fatal("expected a SlowQueryLogger when threshold > 0")
		}

		var got []SlowQueryEntry
		slowLogger.SetSink(func(e SlowQueryEntry) { got = append(got, e) })

		if err := db.Exec("SELECT 1").Error; err != nil {
			t.Fatalf("exec: %v", err)
		}

		if len(got) == 0 {
			t.Error("expected the sink to receive the slow query")
		} else if got[0].SQL != "SELECT 1" {
			t.Errorf("entry SQL = %q, want %q", got[0].SQL, "SELECT 1")
		}

		if _, err := os.Stat(logFile); err != nil {
			t.Errorf("log file was not created: %v", err)
		}
		sqlDB, _ := db.DB()
		_ = sqlDB.Close()
	})
}
