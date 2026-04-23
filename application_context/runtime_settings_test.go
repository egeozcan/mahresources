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

func TestRuntimeSettings_TypedGetters_Defaults(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	if rs.MaxUploadSize() != int64(2<<30) {
		t.Errorf("MaxUploadSize: got %d", rs.MaxUploadSize())
	}
	if rs.MaxImportSize() != int64(10<<30) {
		t.Errorf("MaxImportSize: got %d", rs.MaxImportSize())
	}
	if rs.MRQLDefaultLimit() != 500 {
		t.Errorf("MRQLDefaultLimit: got %d", rs.MRQLDefaultLimit())
	}
	if rs.MRQLQueryTimeout() != 10*time.Second {
		t.Errorf("MRQLQueryTimeout: got %v", rs.MRQLQueryTimeout())
	}
	if rs.ExportRetention() != 24*time.Hour {
		t.Errorf("ExportRetention: got %v", rs.ExportRetention())
	}
	if rs.RemoteConnectTimeout() != 30*time.Second {
		t.Errorf("RemoteConnectTimeout: got %v", rs.RemoteConnectTimeout())
	}
	if rs.RemoteIdleTimeout() != 60*time.Second {
		t.Errorf("RemoteIdleTimeout: got %v", rs.RemoteIdleTimeout())
	}
	if rs.RemoteOverallTimeout() != 30*time.Minute {
		t.Errorf("RemoteOverallTimeout: got %v", rs.RemoteOverallTimeout())
	}
	if rs.SharePublicURL() != "" {
		t.Errorf("SharePublicURL: got %q", rs.SharePublicURL())
	}
	if rs.HashSimilarityThreshold() != 10 {
		t.Errorf("HashSimilarityThreshold: got %d", rs.HashSimilarityThreshold())
	}
	if rs.HashAHashThreshold() != 5 {
		t.Errorf("HashAHashThreshold: got %d", rs.HashAHashThreshold())
	}
}

func TestRuntimeSettings_TypedGetters_Overrides(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	if err := rs.Set(KeyMRQLQueryTimeout, "2s", "", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	if rs.MRQLQueryTimeout() != 2*time.Second {
		t.Fatalf("want 2s, got %v", rs.MRQLQueryTimeout())
	}
}

// gormAuditor is a test-only Auditor that persists entries to the test DB.
type gormAuditor struct{ db *gorm.DB }

func (g *gormAuditor) Audit(action, entityType, entityName, message string, _ map[string]any, ipAddress string) {
	g.db.Create(&models.LogEntry{
		CreatedAt:  time.Now(),
		Level:      models.LogLevelInfo,
		Action:     action,
		EntityType: entityType,
		EntityName: entityName,
		Message:    message,
		IPAddress:  ipAddress,
	})
}

func TestAudit_SetWritesLogEntry(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	rs.SetAuditor(&gormAuditor{db: db})
	if err := rs.Set(KeyMaxUploadSize, "1048576", "bump for video", "192.0.2.1"); err != nil {
		t.Fatalf("set: %v", err)
	}
	var e models.LogEntry
	if err := db.First(&e, "entity_type = ? AND entity_name = ?", "runtime_setting", KeyMaxUploadSize).Error; err != nil {
		t.Fatalf("log entry not found: %v", err)
	}
	if e.Action != "update" {
		t.Errorf("action: got %q want update", e.Action)
	}
	if e.EntityID != nil {
		t.Errorf("EntityID: want nil, got %v", *e.EntityID)
	}
	if e.IPAddress != "192.0.2.1" {
		t.Errorf("IPAddress: got %q", e.IPAddress)
	}
	if !strings.Contains(e.Message, "1048576") || !strings.Contains(e.Message, "bump for video") {
		t.Errorf("Message missing old/new/reason: %q", e.Message)
	}
}

func TestAudit_ResetWritesLogEntry(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	rs.SetAuditor(&gormAuditor{db: db})
	_ = rs.Set(KeyMaxUploadSize, "1048576", "", "192.0.2.1")
	if err := rs.Reset(KeyMaxUploadSize, "revert", "192.0.2.1"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	var rows []models.LogEntry
	db.Where("entity_type = ? AND entity_name = ?", "runtime_setting", KeyMaxUploadSize).Order("created_at asc").Find(&rows)
	if len(rows) != 2 {
		t.Fatalf("want 2 log entries, got %d", len(rows))
	}
	if rows[1].Action != "reset" {
		t.Errorf("second entry: want reset, got %q", rows[1].Action)
	}
}

func TestAudit_FailedSetNoLogEntry(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	rs.SetAuditor(&gormAuditor{db: db})
	// Below min — should reject and not log.
	_ = rs.Set(KeyMaxUploadSize, "1", "", "192.0.2.1")
	var count int64
	db.Model(&models.LogEntry{}).Where("entity_type = ?", "runtime_setting").Count(&count)
	if count != 0 {
		t.Fatalf("want 0 log entries on rejection, got %d", count)
	}
}

// TestContextAuditor_PreservesIPAddress guards against a regression where the
// production auditor (NewContextAuditor) used to ignore the actor parameter and
// log through LogFromRequest without a request, leaving IPAddress empty on
// rows created by HTTP admin-settings calls.
func TestContextAuditor_PreservesIPAddress(t *testing.T) {
	db := newTestDB(t)
	ctx := &MahresourcesContext{db: db}
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	rs.SetAuditor(NewContextAuditor(ctx))
	if err := rs.Set(KeyMaxUploadSize, "1048576", "bump", "203.0.113.42"); err != nil {
		t.Fatalf("set: %v", err)
	}
	var e models.LogEntry
	if err := db.First(&e, "entity_type = ? AND entity_name = ?", "runtime_setting", KeyMaxUploadSize).Error; err != nil {
		t.Fatalf("log entry not found: %v", err)
	}
	if e.IPAddress != "203.0.113.42" {
		t.Fatalf("IPAddress: want 203.0.113.42, got %q", e.IPAddress)
	}
	if e.Action != "update" {
		t.Errorf("Action: got %q", e.Action)
	}
	if !strings.Contains(e.Message, "1048576") || !strings.Contains(e.Message, "bump") {
		t.Errorf("Message missing expected content: %q", e.Message)
	}
}

// TestRuntimeSettings_ConcurrentSetKeepsDBAndCacheConsistent exercises the
// lock discipline that serializes DB mutation + cache update. Without it,
// concurrent Set calls on the same key could commit DB in one order and
// update the cache in the opposite order, leaving the runtime value
// divergent from what survives a restart.
func TestRuntimeSettings_ConcurrentSetKeepsDBAndCacheConsistent(t *testing.T) {
	db := newTestDB(t)
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = rs.Set(KeyMaxUploadSize, fmt.Sprintf("%d", int64(1<<20)+int64(n)), "", "")
		}(i)
	}
	wg.Wait()

	// Re-load from DB and compare with the current in-memory value — they
	// must match.
	rs2 := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	if err := rs2.Load(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if rs.MaxUploadSize() != rs2.MaxUploadSize() {
		t.Fatalf("cache/DB skew: cache=%d db=%d", rs.MaxUploadSize(), rs2.MaxUploadSize())
	}
}

// TestContextAuditor_TruncatesOversizeIP guards against Postgres silently
// rejecting the audit write when the caller passes an oversize string (e.g.
// an IPv6-with-brackets-and-port value) into the 45-character IPAddress
// column.
func TestContextAuditor_TruncatesOversizeIP(t *testing.T) {
	db := newTestDB(t)
	ctx := &MahresourcesContext{db: db}
	rs := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	rs.SetAuditor(NewContextAuditor(ctx))
	// 60 chars — over the 45-char IPAddress column limit.
	oversize := "[2001:0db8:85a3:0000:0000:8a2e:0370:7334]:65535"
	if err := rs.Set(KeyMaxUploadSize, "1048576", "", oversize); err != nil {
		t.Fatalf("set: %v", err)
	}
	var e models.LogEntry
	if err := db.First(&e, "entity_type = ?", "runtime_setting").Error; err != nil {
		t.Fatalf("log entry missing: %v", err)
	}
	if len(e.IPAddress) > 45 {
		t.Fatalf("IPAddress not truncated: len=%d value=%q", len(e.IPAddress), e.IPAddress)
	}
}
