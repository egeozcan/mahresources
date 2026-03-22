package application_context

import (
	"runtime"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/models"
)

// createAdminTestContext creates a self-contained test context with an in-memory
// SQLite database and a unique cache name to avoid sharing with other tests.
func createAdminTestContext(t *testing.T, cacheName string) *MahresourcesContext {
	t.Helper()

	dsn := "file:" + cacheName + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
		&models.ResourceCategory{},
		&models.Series{},
		&models.NoteBlock{},
		&models.PluginKV{},
		&models.ResourceVersion{},
	)
	if err != nil {
		t.Fatalf("Failed to migrate database: %v", err)
	}

	config := &MahresourcesConfig{
		DbType:            constants.DbTypeSqlite,
		HashWorkerEnabled: true,
		HashWorkerCount:   4,
	}
	fs := afero.NewMemMapFs()
	sqlDB, _ := db.DB()
	readOnlyDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(fs, db, readOnlyDB, config)
}

// ---- Task 2 tests ----

func TestGetServerStats_UptimeNonNegative(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_server_stats_test")

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error = %v", err)
	}

	if stats.UptimeSeconds < 0 {
		t.Errorf("expected non-negative uptime, got %f", stats.UptimeSeconds)
	}

	if stats.StartedAt.IsZero() {
		t.Error("StartedAt should not be zero")
	}

	if stats.Uptime == "" {
		t.Error("Uptime formatted string should not be empty")
	}
}

func TestGetServerStats_MemoryStatsNonZero(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_server_stats_mem_test")

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error = %v", err)
	}

	if stats.Sys == 0 {
		t.Error("Sys memory should be non-zero")
	}

	if stats.HeapAllocFmt == "" {
		t.Error("HeapAllocFmt should not be empty")
	}

	if stats.HeapInUseFmt == "" {
		t.Error("HeapInUseFmt should not be empty")
	}

	if stats.SysFmt == "" {
		t.Error("SysFmt should not be empty")
	}
}

func TestGetServerStats_GoVersionMatches(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_server_stats_go_test")

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error = %v", err)
	}

	if stats.GoVersion != runtime.Version() {
		t.Errorf("expected GoVersion %q, got %q", runtime.Version(), stats.GoVersion)
	}
}

func TestGetServerStats_GoroutinesPositive(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_server_stats_goroutines_test")

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error = %v", err)
	}

	if stats.Goroutines <= 0 {
		t.Errorf("expected positive goroutine count, got %d", stats.Goroutines)
	}
}

func TestGetServerStats_DbType(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_server_stats_db_test")

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error = %v", err)
	}

	if stats.DBType != constants.DbTypeSqlite {
		t.Errorf("expected DBType %q, got %q", constants.DbTypeSqlite, stats.DBType)
	}
}

func TestGetServerStats_WorkerFields(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_server_stats_workers_test")

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error = %v", err)
	}

	if !stats.HashWorkerEnabled {
		t.Error("expected HashWorkerEnabled to be true")
	}

	if stats.HashWorkerCount != 4 {
		t.Errorf("expected HashWorkerCount 4, got %d", stats.HashWorkerCount)
	}
}

// ---- helper tests ----

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.00 KB"},
		{1024 * 1024, "1.00 MB"},
		{1024 * 1024 * 1024, "1.00 GB"},
	}

	for _, c := range cases {
		got := formatBytes(c.input)
		if got != c.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", c.input, got, c.expected)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		input    time.Duration
		expected string
	}{
		{5 * time.Second, "5s"},
		{65 * time.Second, "1m 5s"},
		{3661 * time.Second, "1h 1m 1s"},
		{25 * time.Hour, "1d 1h 0m 0s"},
	}

	for _, c := range cases {
		got := formatDuration(c.input)
		if got != c.expected {
			t.Errorf("formatDuration(%v) = %q, want %q", c.input, got, c.expected)
		}
	}
}

// ---- Task 3 tests ----

func TestGetDataStats_EntityCounts(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_data_stats_counts_test")

	// Seed test data
	tag := &models.Tag{Name: "test-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	owner := &models.Group{Name: "test-group"}
	if err := ctx.db.Create(owner).Error; err != nil {
		t.Fatalf("create group: %v", err)
	}

	res1 := &models.Resource{Name: "resource1.txt", FileSize: 1024}
	res2 := &models.Resource{Name: "resource2.txt", FileSize: 2048}
	if err := ctx.db.Create(res1).Error; err != nil {
		t.Fatalf("create resource1: %v", err)
	}
	if err := ctx.db.Create(res2).Error; err != nil {
		t.Fatalf("create resource2: %v", err)
	}

	note := &models.Note{Name: "test-note"}
	if err := ctx.db.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error = %v", err)
	}

	if stats.Entities.Resources < 2 {
		t.Errorf("expected at least 2 resources, got %d", stats.Entities.Resources)
	}

	if stats.Entities.Notes < 1 {
		t.Errorf("expected at least 1 note, got %d", stats.Entities.Notes)
	}

	if stats.Entities.Tags < 1 {
		t.Errorf("expected at least 1 tag, got %d", stats.Entities.Tags)
	}

	if stats.Entities.Groups < 1 {
		t.Errorf("expected at least 1 group, got %d", stats.Entities.Groups)
	}
}

func TestGetDataStats_StorageSum(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_data_stats_storage_test")

	// Seed two resources with known file sizes
	res1 := &models.Resource{Name: "r1.txt", FileSize: 1000}
	res2 := &models.Resource{Name: "r2.txt", FileSize: 2000}
	if err := ctx.db.Create(res1).Error; err != nil {
		t.Fatalf("create resource1: %v", err)
	}
	if err := ctx.db.Create(res2).Error; err != nil {
		t.Fatalf("create resource2: %v", err)
	}

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error = %v", err)
	}

	if stats.StorageTotalBytes < 3000 {
		t.Errorf("expected StorageTotalBytes >= 3000, got %d", stats.StorageTotalBytes)
	}

	if stats.StorageTotalFmt == "" {
		t.Error("StorageTotalFmt should not be empty")
	}
}

func TestGetDataStats_GrowthStats(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_data_stats_growth_test")

	// Create resources "now" so they appear in all periods
	res1 := &models.Resource{Name: "recent1.txt"}
	res2 := &models.Resource{Name: "recent2.txt"}
	if err := ctx.db.Create(res1).Error; err != nil {
		t.Fatalf("create resource1: %v", err)
	}
	if err := ctx.db.Create(res2).Error; err != nil {
		t.Fatalf("create resource2: %v", err)
	}

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error = %v", err)
	}

	// Resources created just now should appear in all windows
	if stats.Growth.Last7Days.Resources < 2 {
		t.Errorf("expected at least 2 resources in last 7 days, got %d", stats.Growth.Last7Days.Resources)
	}
	if stats.Growth.Last30Days.Resources < 2 {
		t.Errorf("expected at least 2 resources in last 30 days, got %d", stats.Growth.Last30Days.Resources)
	}
	if stats.Growth.Last90Days.Resources < 2 {
		t.Errorf("expected at least 2 resources in last 90 days, got %d", stats.Growth.Last90Days.Resources)
	}
}

func TestGetDataStats_ConfigSummary(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_data_stats_config_test")

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error = %v", err)
	}

	if stats.Config.DbType != constants.DbTypeSqlite {
		t.Errorf("expected DbType %q, got %q", constants.DbTypeSqlite, stats.Config.DbType)
	}
}

func TestGetResourceVersionsCount(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_rv_count_test")

	count, err := ctx.GetResourceVersionsCount()
	if err != nil {
		t.Fatalf("GetResourceVersionsCount() error = %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 versions in empty db, got %d", count)
	}

	// Create a resource and then a version
	res := &models.Resource{Name: "test.txt"}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	version := &models.ResourceVersion{
		ResourceID:    res.ID,
		VersionNumber: 1,
		Hash:          "abc123",
		HashType:      "SHA1",
		FileSize:      100,
		Location:      "test/path",
	}
	if err := ctx.db.Create(version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	count, err = ctx.GetResourceVersionsCount()
	if err != nil {
		t.Fatalf("GetResourceVersionsCount() error = %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 version, got %d", count)
	}
}

// ---- Task 4 tests ----

func TestGetExpensiveStats_StorageByContentType(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_expensive_content_type_test")

	// Seed resources with different content types
	res1 := &models.Resource{Name: "img.jpg", ContentType: "image/jpeg", FileSize: 5000}
	res2 := &models.Resource{Name: "doc.pdf", ContentType: "application/pdf", FileSize: 3000}
	res3 := &models.Resource{Name: "img2.jpg", ContentType: "image/jpeg", FileSize: 2000}
	for _, r := range []*models.Resource{res1, res2, res3} {
		if err := ctx.db.Create(r).Error; err != nil {
			t.Fatalf("create resource: %v", err)
		}
	}

	stats, err := ctx.GetExpensiveStats()
	if err != nil {
		t.Fatalf("GetExpensiveStats() error = %v", err)
	}

	if len(stats.StorageByContentType) == 0 {
		t.Fatal("expected at least one content type entry")
	}

	// Find image/jpeg in results
	found := false
	for _, row := range stats.StorageByContentType {
		if row.ContentType == "image/jpeg" {
			found = true
			if row.TotalBytes < 7000 {
				t.Errorf("expected image/jpeg total >= 7000, got %d", row.TotalBytes)
			}
			if row.Count < 2 {
				t.Errorf("expected image/jpeg count >= 2, got %d", row.Count)
			}
			if row.TotalFmt == "" {
				t.Error("TotalFmt should not be empty")
			}
		}
	}
	if !found {
		t.Error("expected to find image/jpeg in StorageByContentType")
	}
}

func TestGetExpensiveStats_LogStats(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_expensive_log_stats_test")

	// Seed log entries at different levels
	logs := []*models.LogEntry{
		{Level: models.LogLevelInfo, Action: models.LogActionSystem, Message: "info1"},
		{Level: models.LogLevelInfo, Action: models.LogActionSystem, Message: "info2"},
		{Level: models.LogLevelWarning, Action: models.LogActionSystem, Message: "warn1"},
		{Level: models.LogLevelError, Action: models.LogActionSystem, Message: "error1"},
		{Level: models.LogLevelError, Action: models.LogActionSystem, Message: "error2"},
	}
	for _, l := range logs {
		if err := ctx.db.Create(l).Error; err != nil {
			t.Fatalf("create log entry: %v", err)
		}
	}

	stats, err := ctx.GetExpensiveStats()
	if err != nil {
		t.Fatalf("GetExpensiveStats() error = %v", err)
	}

	if stats.LogStats.TotalEntries < 5 {
		t.Errorf("expected at least 5 total log entries, got %d", stats.LogStats.TotalEntries)
	}
	if stats.LogStats.ByLevel[models.LogLevelInfo] < 2 {
		t.Errorf("expected at least 2 info logs, got %d", stats.LogStats.ByLevel[models.LogLevelInfo])
	}
	if stats.LogStats.ByLevel[models.LogLevelWarning] < 1 {
		t.Errorf("expected at least 1 warning log, got %d", stats.LogStats.ByLevel[models.LogLevelWarning])
	}
	if stats.LogStats.ByLevel[models.LogLevelError] < 2 {
		t.Errorf("expected at least 2 error logs, got %d", stats.LogStats.ByLevel[models.LogLevelError])
	}
	// All errors were created just now, so they should appear in recent 24h
	if stats.LogStats.RecentErrors < 2 {
		t.Errorf("expected at least 2 recent errors, got %d", stats.LogStats.RecentErrors)
	}
}

func TestGetExpensiveStats_OrphanStats(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_expensive_orphan_test")

	// Create a resource with no tags and no groups
	res := &models.Resource{Name: "orphan.txt"}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	stats, err := ctx.GetExpensiveStats()
	if err != nil {
		t.Fatalf("GetExpensiveStats() error = %v", err)
	}

	if stats.Orphans.WithoutTags < 1 {
		t.Errorf("expected at least 1 resource without tags, got %d", stats.Orphans.WithoutTags)
	}
	if stats.Orphans.WithoutGroups < 1 {
		t.Errorf("expected at least 1 resource without groups, got %d", stats.Orphans.WithoutGroups)
	}
}

func TestGetExpensiveStats_SimilarityInfo(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_expensive_similarity_test")

	// Empty DB should have 0 hashes and 0 pairs
	stats, err := ctx.GetExpensiveStats()
	if err != nil {
		t.Fatalf("GetExpensiveStats() error = %v", err)
	}

	if stats.Similarity.TotalHashes != 0 {
		t.Errorf("expected 0 hashed resources on empty db, got %d", stats.Similarity.TotalHashes)
	}
	if stats.Similarity.SimilarPairsFound != 0 {
		t.Errorf("expected 0 similarity pairs on empty db, got %d", stats.Similarity.SimilarPairsFound)
	}
}

func TestGetExpensiveStats_TopTags(t *testing.T) {
	ctx := createAdminTestContext(t, "admin_expensive_top_tags_test")

	// Create a tag
	tag := &models.Tag{Name: "popular-tag"}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}

	// Create a resource and attach the tag
	res := &models.Resource{Name: "tagged.txt"}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if err := ctx.db.Model(res).Association("Tags").Append(tag); err != nil {
		t.Fatalf("attach tag: %v", err)
	}

	stats, err := ctx.GetExpensiveStats()
	if err != nil {
		t.Fatalf("GetExpensiveStats() error = %v", err)
	}

	if len(stats.TopTags) == 0 {
		t.Fatal("expected at least one top tag")
	}
}
