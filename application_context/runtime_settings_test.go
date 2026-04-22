package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Use a unique per-test named in-memory database to avoid cross-test pollution.
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&models.RuntimeSetting{}, &models.LogEntry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func defaults() map[string]any {
	return map[string]any{
		KeyMaxUploadSize:           int64(2 << 30),
		KeyMaxImportSize:           int64(10 << 30),
		KeyMRQLDefaultLimit:        int(500),
		KeyMRQLQueryTimeout:        10 * time.Second,
		KeyExportRetention:         24 * time.Hour,
		KeyRemoteConnectTimeout:    30 * time.Second,
		KeyRemoteIdleTimeout:       60 * time.Second,
		KeyRemoteOverallTimeout:    30 * time.Minute,
		KeySharePublicURL:          "",
		KeyHashSimilarityThreshold: int(10),
		KeyHashAHashThreshold:      uint64(5),
	}
}

type stubLogger struct {
	mu      sync.Mutex
	entries []string
}

func (l *stubLogger) Warn(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, "WARN: "+fmt.Sprintf(format, args...))
}

func (l *stubLogger) Error(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, "ERROR: "+fmt.Sprintf(format, args...))
}

func (l *stubLogger) contains(substr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, e := range l.entries {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

func TestRuntimeSettings_LoadEmpty(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	if err := rs.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	v, ok := rs.getRaw(KeyMaxUploadSize)
	if !ok {
		t.Fatal("want default present")
	}
	if v.(int64) != int64(2<<30) {
		t.Fatalf("want 2<<30, got %v", v)
	}
}

func TestRuntimeSettings_LoadSeeded(t *testing.T) {
	db := newTestDB(t)
	enc, _ := encodeSettingValue("int64", int64(1<<20))
	db.Create(&models.RuntimeSetting{Key: KeyMaxUploadSize, ValueJSON: string(enc), Reason: "test", UpdatedAt: time.Now()})
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	if err := rs.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	v, _ := rs.getRaw(KeyMaxUploadSize)
	if v.(int64) != int64(1<<20) {
		t.Fatalf("want override 1MiB, got %v", v)
	}
}

func TestRuntimeSettings_SetGetRoundTrip(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	if err := rs.Set(KeyMaxUploadSize, "1048576", "bump", "127.0.0.1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	v, _ := rs.getRaw(KeyMaxUploadSize)
	if v.(int64) != int64(1<<20) {
		t.Fatalf("want 1MiB, got %v", v)
	}
	// DB row exists
	var row models.RuntimeSetting
	if err := db.First(&row, "key = ?", KeyMaxUploadSize).Error; err != nil {
		t.Fatalf("db row missing: %v", err)
	}
}

func TestRuntimeSettings_Reset(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	_ = rs.Set(KeyMaxUploadSize, "1048576", "", "127.0.0.1")
	if err := rs.Reset(KeyMaxUploadSize, "revert", "127.0.0.1"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	v, _ := rs.getRaw(KeyMaxUploadSize)
	if v.(int64) != int64(2<<30) {
		t.Fatalf("want default after reset, got %v", v)
	}
	var count int64
	db.Model(&models.RuntimeSetting{}).Where("key = ?", KeyMaxUploadSize).Count(&count)
	if count != 0 {
		t.Fatalf("want 0 rows after reset, got %d", count)
	}
}

func TestRuntimeSettings_UnknownKey(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	if err := rs.Set("not_a_key", "1", "", "127.0.0.1"); err == nil {
		t.Fatal("want error for unknown key")
	}
}

func TestRuntimeSettings_List(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	_ = rs.Set(KeyMaxUploadSize, "1048576", "bump", "127.0.0.1")
	views := rs.List()
	if len(views) != len(buildSpecs()) {
		t.Fatalf("want %d views, got %d", len(buildSpecs()), len(views))
	}
	var found bool
	for _, v := range views {
		if v.Key == KeyMaxUploadSize {
			found = true
			if !v.Overridden {
				t.Error("overridden flag not set")
			}
			if v.Reason != "bump" {
				t.Errorf("reason: got %q want %q", v.Reason, "bump")
			}
		}
	}
	if !found {
		t.Fatal("max_upload_size view missing")
	}
}

func TestRuntimeSettings_List_GroupOrdering(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	views := rs.List()
	// The first view must be in GroupUploads; the last must be in GroupExports.
	if views[0].Group != GroupUploads {
		t.Fatalf("first group: want %q, got %q", GroupUploads, views[0].Group)
	}
	if views[len(views)-1].Group != GroupExports {
		t.Fatalf("last group: want %q, got %q", GroupExports, views[len(views)-1].Group)
	}
	// No group should appear before an earlier group in groupDisplayOrder.
	lastIdx := -1
	for _, v := range views {
		idx := groupOrderIndex(v.Group)
		if idx < lastIdx {
			t.Fatalf("group %q (idx %d) appeared after group with idx %d", v.Group, idx, lastIdx)
		}
		lastIdx = idx
	}
}

func TestRuntimeSettings_ConcurrentSetGet(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(2)
		go func(i int) { defer wg.Done(); _ = rs.Set(KeyMRQLDefaultLimit, fmt.Sprintf("%d", 100+i), "", "a") }(i)
		go func() { defer wg.Done(); _, _ = rs.getRaw(KeyMRQLDefaultLimit) }()
	}
	wg.Wait()
}
