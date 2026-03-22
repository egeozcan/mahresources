# Admin Overview Page Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin overview page at `/admin/overview` with server health stats (auto-refreshing), data statistics, and expensive async-loaded analytics.

**Architecture:** Single template page with Alpine.js component that fetches from three JSON API endpoints. Server stats poll every 10s. Data stats load once. Expensive stats load async with spinners. CLI `mr admin` command hits the same endpoints.

**Tech Stack:** Go (GORM, Gorilla Mux, Pongo2), Alpine.js, Tailwind CSS, Cobra (CLI), Playwright (E2E tests)

**Spec:** `docs/superpowers/specs/2026-03-22-admin-overview-design.md`

---

### Task 1: Infrastructure — Config Plumbing

**Files:**
- Modify: `application_context/context.go` (add `StartedAt` field to `MahresourcesContext`, add hash/mode fields to `MahresourcesConfig`)
- Modify: `main.go` (plumb hash worker settings and mode flags into config)

- [ ] **Step 1: Add config fields and StartedAt to context.go**

Add new fields to `MahresourcesConfig` (after the existing `PluginsDisabled` field around line 63):

```go
// Admin overview fields
HashWorkerEnabled        bool
HashWorkerCount          int
HashBatchSize            int
HashPollInterval         time.Duration
HashSimilarityThreshold  int
HashCacheSize            int
EphemeralMode            bool
MemoryDB                 bool
MemoryFS                 bool
MaxDBConnections         int
FileSavePath             string
SkipFTS                  bool
```

Add `StartedAt` field to `MahresourcesContext` (after `ftsEnabled` around line 145):

```go
StartedAt time.Time
```

Set it in `NewMahresourcesContext` (at the start of the function, around line 150):

```go
ctx.StartedAt = time.Now()
```

- [ ] **Step 2: Plumb values from main.go into config**

In `main.go`, where `MahresourcesConfig` is constructed (find the `application_context.MahresourcesConfig{` block), add the new fields:

```go
HashWorkerEnabled:       !*hashWorkerDisabled,
HashWorkerCount:         *hashWorkerCount,
HashBatchSize:           *hashBatchSize,
HashPollInterval:        *hashPollInterval,
HashSimilarityThreshold: *hashSimilarityThreshold,
HashCacheSize:           *hashCacheSize,
EphemeralMode:           *ephemeral,
MemoryDB:                *memoryDB,
MemoryFS:                *memoryFS,
MaxDBConnections:        *maxDBConnections,
FileSavePath:            inputConfig.FileSavePath,
SkipFTS:                 *skipFTS,
```

Note: Find the exact variable names by reading `main.go`. The hash worker flags are around lines 105-110, ephemeral flags around lines 96-100. The `skipFTS` flag is also a local var. Match the dereferenced pointer names exactly.

- [ ] **Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles without errors

- [ ] **Step 4: Commit**

```bash
git add application_context/context.go main.go
git commit -m "feat(admin): plumb hash worker, mode, and storage config into MahresourcesConfig"
```

---

### Task 2: GetServerStats — Test & Implementation

**Files:**
- Create: `application_context/admin_context.go`
- Create: `application_context/admin_context_test.go`

- [ ] **Step 1: Write the test for GetServerStats**

Create `application_context/admin_context_test.go`:

```go
package application_context

import (
	"runtime"
	"testing"
	"time"
)

func TestGetServerStats(t *testing.T) {
	ctx := newTestContext(t)

	stats, err := ctx.GetServerStats()
	if err != nil {
		t.Fatalf("GetServerStats() error: %v", err)
	}

	// Uptime should be very small since context was just created
	if stats.UptimeSeconds < 0 {
		t.Errorf("expected non-negative uptime, got %d", stats.UptimeSeconds)
	}

	if stats.Uptime == "" {
		t.Error("expected non-empty uptime string")
	}

	if stats.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}

	// Memory
	if stats.Memory.HeapAlloc == 0 {
		t.Error("expected non-zero HeapAlloc")
	}
	if stats.Memory.Sys == 0 {
		t.Error("expected non-zero Sys")
	}
	if stats.Memory.HeapAllocFormatted == "" {
		t.Error("expected non-empty HeapAllocFormatted")
	}

	// Goroutines
	if stats.Goroutines < 1 {
		t.Errorf("expected at least 1 goroutine, got %d", stats.Goroutines)
	}

	// Go version
	if stats.GoVersion != runtime.Version() {
		t.Errorf("expected Go version %s, got %s", runtime.Version(), stats.GoVersion)
	}

	// Database
	if stats.Database.Type == "" {
		t.Error("expected non-empty database type")
	}
}
```

**Important:** Before writing this test, check how existing tests in `application_context/` create a test context. Look for a helper like `newTestContext` or see how tests set up an in-memory GORM DB. If no helper exists, create one at the top of the test file:

```go
func newTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	// Auto-migrate all models
	db.AutoMigrate(&models.Resource{}, &models.Note{}, &models.Group{}, &models.Tag{},
		&models.Category{}, &models.ResourceCategory{}, &models.NoteType{},
		&models.GroupRelation{}, &models.GroupRelationType{}, &models.Query{},
		&models.LogEntry{}, &models.ResourceVersion{}, &models.ImageHash{},
		&models.ResourceSimilarity{})

	fs := afero.NewMemMapFs()
	config := &MahresourcesConfig{
		DbType:      "SQLITE",
		DbDsn:       ":memory:",
		BindAddress: ":8181",
	}
	ctx := NewMahresourcesContext(fs, db, nil, config)
	return ctx
}
```

Adjust imports as needed. Check actual model names by looking at `models/` directory.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGetServerStats -v`
Expected: FAIL — `GetServerStats` method not found

- [ ] **Step 3: Implement GetServerStats**

Create `application_context/admin_context.go`:

```go
package application_context

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"mahresources/constants"
)

// ServerStats holds runtime server information.
type ServerStats struct {
	Uptime        string       `json:"uptime"`
	UptimeSeconds int64        `json:"uptimeSeconds"`
	StartedAt     time.Time    `json:"startedAt"`
	Memory        MemoryStats  `json:"memory"`
	Goroutines    int          `json:"goroutines"`
	GoVersion     string       `json:"goVersion"`
	Database      DBStats      `json:"database"`
	Workers       WorkerStats  `json:"workers"`
}

type MemoryStats struct {
	HeapAlloc          uint64 `json:"heapAlloc"`
	HeapInUse          uint64 `json:"heapInUse"`
	Sys                uint64 `json:"sys"`
	NumGC              uint32 `json:"numGC"`
	HeapAllocFormatted string `json:"heapAllocFormatted"`
	SysFormatted       string `json:"sysFormatted"`
}

type DBStats struct {
	Type                string `json:"type"`
	OpenConnections     int    `json:"openConnections"`
	InUse               int    `json:"inUse"`
	Idle                int    `json:"idle"`
	DBFileSize          int64  `json:"dbFileSize"`
	DBFileSizeFormatted string `json:"dbFileSizeFormatted"`
}

type WorkerStats struct {
	HashWorkerEnabled   bool `json:"hashWorkerEnabled"`
	HashWorkerCount     int  `json:"hashWorkerCount"`
	DownloadQueueLength int  `json:"downloadQueueLength"`
}

func (ctx *MahresourcesContext) GetServerStats() (*ServerStats, error) {
	// Memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	// Uptime
	uptime := time.Since(ctx.StartedAt)

	// DB stats
	sqlDB, err := ctx.db.DB()
	var dbStats DBStats
	if err == nil {
		stats := sqlDB.Stats()
		dbStats = DBStats{
			Type:            ctx.Config.DbType,
			OpenConnections: stats.OpenConnections,
			InUse:           stats.InUse,
			Idle:            stats.Idle,
		}
	} else {
		dbStats = DBStats{Type: ctx.Config.DbType}
	}

	// SQLite file size
	if ctx.Config.DbType == constants.DbTypeSqlite && ctx.Config.DbDsn != "" && ctx.Config.DbDsn != ":memory:" {
		if fi, err := os.Stat(ctx.Config.DbDsn); err == nil {
			dbStats.DBFileSize = fi.Size()
			dbStats.DBFileSizeFormatted = formatBytes(fi.Size())
		}
	}

	// Download queue
	downloadQueueLen := 0
	if ctx.downloadManager != nil {
		downloadQueueLen = ctx.downloadManager.ActiveCount()
	}

	return &ServerStats{
		Uptime:        formatDuration(uptime),
		UptimeSeconds: int64(uptime.Seconds()),
		StartedAt:     ctx.StartedAt,
		Memory: MemoryStats{
			HeapAlloc:          memStats.HeapAlloc,
			HeapInUse:          memStats.HeapInuse,
			Sys:                memStats.Sys,
			NumGC:              memStats.NumGC,
			HeapAllocFormatted: formatBytes(int64(memStats.HeapAlloc)),
			SysFormatted:       formatBytes(int64(memStats.Sys)),
		},
		Goroutines: runtime.NumGoroutine(),
		GoVersion:  runtime.Version(),
		Database:   dbStats,
		Workers: WorkerStats{
			HashWorkerEnabled:   ctx.Config.HashWorkerEnabled,
			HashWorkerCount:     ctx.Config.HashWorkerCount,
			DownloadQueueLength: downloadQueueLen,
		},
	}, nil
}

// formatBytes converts bytes to a human-readable string.
func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatDuration formats a duration as "Xd Xh Xm".
func formatDuration(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm", hours, minutes)
	}
	return fmt.Sprintf("%dm", minutes)
}
```

**Important:** Check the exact constant name for SQLite. Look in `constants/` for `DbTypeSQLite` or similar — the spec says `SQLITE` but verify the constant name. Also verify `ctx.downloadManager` field name and `ActiveCount()` method availability.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGetServerStats -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/admin_context.go application_context/admin_context_test.go
git commit -m "feat(admin): add GetServerStats with runtime, memory, DB, and worker stats"
```

---

### Task 3: GetDataStats — Test & Implementation

**Files:**
- Modify: `application_context/admin_context.go`
- Modify: `application_context/admin_context_test.go`

- [ ] **Step 1: Write the test for GetDataStats**

Add to `admin_context_test.go`:

```go
func TestGetDataStats(t *testing.T) {
	ctx := newTestContext(t)

	// Seed some test data
	ctx.db.Create(&models.Resource{Name: "test.jpg", ContentType: "image/jpeg", FileSize: 1024})
	ctx.db.Create(&models.Resource{Name: "test2.jpg", ContentType: "image/jpeg", FileSize: 2048})
	ctx.db.Create(&models.Note{Name: "Test Note"})
	ctx.db.Create(&models.Tag{Name: "test-tag"})
	group := models.Group{Name: "Test Group"}
	ctx.db.Create(&group)

	stats, err := ctx.GetDataStats()
	if err != nil {
		t.Fatalf("GetDataStats() error: %v", err)
	}

	// Entity counts
	if stats.Counts.Resources != 2 {
		t.Errorf("expected 2 resources, got %d", stats.Counts.Resources)
	}
	if stats.Counts.Notes != 1 {
		t.Errorf("expected 1 note, got %d", stats.Counts.Notes)
	}
	if stats.Counts.Tags != 1 {
		t.Errorf("expected 1 tag, got %d", stats.Counts.Tags)
	}
	if stats.Counts.Groups != 1 {
		t.Errorf("expected 1 group, got %d", stats.Counts.Groups)
	}

	// Storage
	if stats.TotalStorageBytes != 3072 {
		t.Errorf("expected 3072 bytes total storage, got %d", stats.TotalStorageBytes)
	}
	if stats.TotalStorageFormatted == "" {
		t.Error("expected non-empty TotalStorageFormatted")
	}

	// Config should be populated
	if stats.Config.DbType == "" {
		t.Error("expected non-empty config DbType")
	}

	// Growth — all resources were just created, so last7Days should match total
	if stats.Growth.Resources.Last7Days != 2 {
		t.Errorf("expected 2 resources in last 7 days, got %d", stats.Growth.Resources.Last7Days)
	}
}
```

**Important:** Adjust model constructors to match actual struct fields. Check `models/resource_model.go` for required fields (some models may need `Model: gorm.Model{}` or other required fields). Use `ctx.db.Create()` directly for test data.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGetDataStats -v`
Expected: FAIL — `GetDataStats` method not found

- [ ] **Step 3: Implement GetDataStats**

Add to `admin_context.go`:

```go
// DataStats holds entity counts, storage totals, growth, and config.
type DataStats struct {
	Counts                    EntityCounts   `json:"counts"`
	TotalStorageBytes         int64          `json:"totalStorageBytes"`
	TotalStorageFormatted     string         `json:"totalStorageFormatted"`
	TotalVersionStorageBytes  int64          `json:"totalVersionStorageBytes"`
	TotalVersionStorageFormatted string      `json:"totalVersionStorageFormatted"`
	Growth                    GrowthStats    `json:"growth"`
	Config                    ConfigSummary  `json:"config"`
}

type EntityCounts struct {
	Resources          int64 `json:"resources"`
	Notes              int64 `json:"notes"`
	Groups             int64 `json:"groups"`
	Tags               int64 `json:"tags"`
	Categories         int64 `json:"categories"`
	ResourceCategories int64 `json:"resourceCategories"`
	NoteTypes          int64 `json:"noteTypes"`
	RelationTypes      int64 `json:"relationTypes"`
	Relations          int64 `json:"relations"`
	Queries            int64 `json:"queries"`
	LogEntries         int64 `json:"logEntries"`
	ResourceVersions   int64 `json:"resourceVersions"`
}

type GrowthStats struct {
	Resources GrowthPeriods `json:"resources"`
	Notes     GrowthPeriods `json:"notes"`
	Groups    GrowthPeriods `json:"groups"`
}

type GrowthPeriods struct {
	Last7Days  int64 `json:"last7Days"`
	Last30Days int64 `json:"last30Days"`
	Last90Days int64 `json:"last90Days"`
}

type ConfigSummary struct {
	BindAddress              string   `json:"bindAddress"`
	FileSavePath             string   `json:"fileSavePath"`
	DbType                   string   `json:"dbType"`
	DbDsn                    string   `json:"dbDsn"`
	HasReadOnlyDB            bool     `json:"hasReadOnlyDB"`
	FfmpegAvailable          bool     `json:"ffmpegAvailable"`
	LibreOfficeAvailable     bool     `json:"libreOfficeAvailable"`
	FtsEnabled               bool     `json:"ftsEnabled"`
	HashWorkerEnabled        bool     `json:"hashWorkerEnabled"`
	HashWorkerCount          int      `json:"hashWorkerCount"`
	HashBatchSize            int      `json:"hashBatchSize"`
	HashPollInterval         string   `json:"hashPollInterval"`
	HashSimilarityThreshold  int      `json:"hashSimilarityThreshold"`
	HashCacheSize            int      `json:"hashCacheSize"`
	AltFileSystems           []string `json:"altFileSystems"`
	EphemeralMode            bool     `json:"ephemeralMode"`
	MemoryDB                 bool     `json:"memoryDB"`
	MemoryFS                 bool     `json:"memoryFS"`
	MaxDBConnections         int      `json:"maxDBConnections"`
	RemoteConnectTimeout     string   `json:"remoteConnectTimeout"`
	RemoteIdleTimeout        string   `json:"remoteIdleTimeout"`
	RemoteOverallTimeout     string   `json:"remoteOverallTimeout"`
}

func (ctx *MahresourcesContext) GetDataStats() (*DataStats, error) {
	var counts EntityCounts
	var totalStorage, totalVersionStorage int64
	var growth GrowthStats

	var wg sync.WaitGroup

	// Entity counts — run all in parallel
	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Resources, _ = ctx.GetResourceCount(&query_models.ResourceSearchQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Notes, _ = ctx.GetNoteCount(&query_models.NoteQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Groups, _ = ctx.GetGroupsCount(&query_models.GroupQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Tags, _ = ctx.GetTagsCount(&query_models.TagQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Categories, _ = ctx.GetCategoriesCount(&query_models.CategoryQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.ResourceCategories, _ = ctx.GetResourceCategoriesCount(&query_models.ResourceCategoryQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.NoteTypes, _ = ctx.GetNoteTypesCount(&query_models.NoteTypeQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.RelationTypes, _ = ctx.GetRelationTypesCount(&query_models.RelationshipTypeQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Relations, _ = ctx.GetRelationsCount(&query_models.GroupRelationshipQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.Queries, _ = ctx.GetQueriesCount(&query_models.QueryQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.LogEntries, _ = ctx.GetLogEntriesCount(&query_models.LogEntryQuery{})
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		counts.ResourceVersions, _ = ctx.GetResourceVersionsCount()
	}()

	// Total storage
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.Resource{}).Select("COALESCE(SUM(file_size), 0)").Scan(&totalStorage)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.ResourceVersion{}).Select("COALESCE(SUM(file_size), 0)").Scan(&totalVersionStorage)
	}()

	// Growth stats
	now := time.Now()
	d7 := now.AddDate(0, 0, -7)
	d30 := now.AddDate(0, 0, -30)
	d90 := now.AddDate(0, 0, -90)

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.Resource{}).Where("created_at > ?", d7).Count(&growth.Resources.Last7Days)
		ctx.db.Model(&models.Resource{}).Where("created_at > ?", d30).Count(&growth.Resources.Last30Days)
		ctx.db.Model(&models.Resource{}).Where("created_at > ?", d90).Count(&growth.Resources.Last90Days)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.Note{}).Where("created_at > ?", d7).Count(&growth.Notes.Last7Days)
		ctx.db.Model(&models.Note{}).Where("created_at > ?", d30).Count(&growth.Notes.Last30Days)
		ctx.db.Model(&models.Note{}).Where("created_at > ?", d90).Count(&growth.Notes.Last90Days)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.Group{}).Where("created_at > ?", d7).Count(&growth.Groups.Last7Days)
		ctx.db.Model(&models.Group{}).Where("created_at > ?", d30).Count(&growth.Groups.Last30Days)
		ctx.db.Model(&models.Group{}).Where("created_at > ?", d90).Count(&growth.Groups.Last90Days)
	}()

	wg.Wait()

	// Config summary
	altFSNames := make([]string, 0, len(ctx.Config.AltFileSystems))
	for name := range ctx.Config.AltFileSystems {
		altFSNames = append(altFSNames, name)
	}

	config := ConfigSummary{
		BindAddress:              ctx.Config.BindAddress,
		FileSavePath:             ctx.Config.FileSavePath,
		DbType:                   ctx.Config.DbType,
		DbDsn:                    ctx.Config.DbDsn,
		HasReadOnlyDB:            ctx.Config.DbReadOnlyDsn != "",
		FfmpegAvailable:          ctx.Config.FfmpegPath != "",
		LibreOfficeAvailable:     ctx.Config.LibreOfficePath != "",
		FtsEnabled:               ctx.ftsEnabled,
		HashWorkerEnabled:        ctx.Config.HashWorkerEnabled,
		HashWorkerCount:          ctx.Config.HashWorkerCount,
		HashBatchSize:            ctx.Config.HashBatchSize,
		HashPollInterval:         ctx.Config.HashPollInterval.String(),
		HashSimilarityThreshold:  ctx.Config.HashSimilarityThreshold,
		HashCacheSize:            ctx.Config.HashCacheSize,
		AltFileSystems:           altFSNames,
		EphemeralMode:            ctx.Config.EphemeralMode,
		MemoryDB:                 ctx.Config.MemoryDB,
		MemoryFS:                 ctx.Config.MemoryFS,
		MaxDBConnections:         ctx.Config.MaxDBConnections,
		RemoteConnectTimeout:     ctx.Config.RemoteResourceConnectTimeout.String(),
		RemoteIdleTimeout:        ctx.Config.RemoteResourceIdleTimeout.String(),
		RemoteOverallTimeout:     ctx.Config.RemoteResourceOverallTimeout.String(),
	}

	return &DataStats{
		Counts:                       counts,
		TotalStorageBytes:            totalStorage,
		TotalStorageFormatted:        formatBytes(totalStorage),
		TotalVersionStorageBytes:     totalVersionStorage,
		TotalVersionStorageFormatted: formatBytes(totalVersionStorage),
		Growth:                       growth,
		Config:                       config,
	}, nil
}

// GetResourceVersionsCount returns the total number of resource versions.
func (ctx *MahresourcesContext) GetResourceVersionsCount() (int64, error) {
	var count int64
	return count, ctx.db.Model(&models.ResourceVersion{}).Count(&count).Error
}
```

Add needed imports: `"sync"`, `"mahresources/models"`, `"mahresources/models/query_models"`.

**Important:** Verify that `nil` is accepted by the existing count methods. Some may require non-nil query objects — check the GORM scope functions. If they panic on nil, pass empty structs instead (e.g., `&query_models.ResourceSearchQuery{}`). Also verify the model names: `models.ResourceVersion` — check `models/` for exact name.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGetDataStats -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add application_context/admin_context.go application_context/admin_context_test.go
git commit -m "feat(admin): add GetDataStats with entity counts, storage, growth, and config"
```

---

### Task 4: GetExpensiveStats — Test & Implementation

**Files:**
- Modify: `application_context/admin_context.go`
- Modify: `application_context/admin_context_test.go`

- [ ] **Step 1: Write the test for GetExpensiveStats**

Add to `admin_context_test.go`:

```go
func TestGetExpensiveStats(t *testing.T) {
	ctx := newTestContext(t)

	// Seed resources with different content types
	ctx.db.Create(&models.Resource{Name: "a.jpg", ContentType: "image/jpeg", FileSize: 1000})
	ctx.db.Create(&models.Resource{Name: "b.jpg", ContentType: "image/jpeg", FileSize: 2000})
	ctx.db.Create(&models.Resource{Name: "c.pdf", ContentType: "application/pdf", FileSize: 500})

	// Seed tags and associate with resources
	tag1 := models.Tag{Name: "landscape"}
	ctx.db.Create(&tag1)
	tag2 := models.Tag{Name: "portrait"}
	ctx.db.Create(&tag2)

	// Associate tag1 with first two resources (check exact association method)
	// This depends on the GORM many-to-many setup — check resource model for join table name

	// Seed a log entry
	ctx.db.Create(&models.LogEntry{Level: "error", Action: "system", Message: "test error"})
	ctx.db.Create(&models.LogEntry{Level: "info", Action: "create", Message: "test info"})

	stats, err := ctx.GetExpensiveStats()
	if err != nil {
		t.Fatalf("GetExpensiveStats() error: %v", err)
	}

	// Storage by content type
	if len(stats.StorageByContentType) < 2 {
		t.Errorf("expected at least 2 content types, got %d", len(stats.StorageByContentType))
	}

	// Log stats
	if stats.LogStats.TotalEntries != 2 {
		t.Errorf("expected 2 log entries, got %d", stats.LogStats.TotalEntries)
	}
	if stats.LogStats.ByLevel["error"] != 1 {
		t.Errorf("expected 1 error log, got %d", stats.LogStats.ByLevel["error"])
	}

	// Orphaned resources (none have tags via association, so all 3 should be orphaned)
	if stats.OrphanedResources.WithoutTags != 3 {
		t.Errorf("expected 3 resources without tags, got %d", stats.OrphanedResources.WithoutTags)
	}
}
```

**Important:** Adjust model creation to match actual struct fields. Check how many-to-many associations work in this codebase — the tag association on resources may use `resource_tags` join table. For the test, creating resources without tag associations is sufficient to test the orphan count.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGetExpensiveStats -v`
Expected: FAIL — `GetExpensiveStats` method not found

- [ ] **Step 3: Implement GetExpensiveStats**

Add to `admin_context.go`:

```go
// ExpensiveStats holds computed analytics that may be slow on large databases.
type ExpensiveStats struct {
	StorageByContentType []ContentTypeStorage `json:"storageByContentType"`
	TopTags              []TagCount           `json:"topTags"`
	TopCategories        []CategoryCount      `json:"topCategories"`
	OrphanedResources    OrphanStats          `json:"orphanedResources"`
	SimilarityStats      SimilarityInfo       `json:"similarityStats"`
	LogStats             LogStatsInfo         `json:"logStats"`
}

type ContentTypeStorage struct {
	ContentType string `json:"contentType"`
	Count       int64  `json:"count"`
	TotalBytes  int64  `json:"totalBytes"`
	Formatted   string `json:"formatted"`
}

type TagCount struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type CategoryCount struct {
	ID    uint   `json:"id"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type OrphanStats struct {
	WithoutTags   int64 `json:"withoutTags"`
	WithoutGroups int64 `json:"withoutGroups"`
}

type SimilarityInfo struct {
	TotalHashes       int64 `json:"totalHashes"`
	SimilarPairsFound int64 `json:"similarPairsFound"`
}

type LogStatsInfo struct {
	TotalEntries int64            `json:"totalEntries"`
	ByLevel      map[string]int64 `json:"byLevel"`
	RecentErrors int64            `json:"recentErrors"`
}

func (ctx *MahresourcesContext) GetExpensiveStats() (*ExpensiveStats, error) {
	var stats ExpensiveStats
	var wg sync.WaitGroup

	// Storage by content type
	wg.Add(1)
	go func() {
		defer wg.Done()
		var results []ContentTypeStorage
		ctx.db.Model(&models.Resource{}).
			Select("content_type, COUNT(*) as count, SUM(file_size) as total_bytes").
			Group("content_type").
			Order("total_bytes DESC").
			Scan(&results)
		for i := range results {
			results[i].Formatted = formatBytes(results[i].TotalBytes)
		}
		stats.StorageByContentType = results
	}()

	// Top tags (by resource count)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var results []TagCount
		ctx.db.Raw(`
			SELECT t.id, t.name, COUNT(*) as count
			FROM tags t
			JOIN resource_tags rt ON rt.tag_id = t.id
			GROUP BY t.id, t.name
			ORDER BY count DESC
			LIMIT 10
		`).Scan(&results)
		stats.TopTags = results
	}()

	// Top categories (categories are used by groups via FK category_id)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var results []CategoryCount
		ctx.db.Raw(`
			SELECT c.id, c.name, COUNT(*) as count
			FROM categories c
			JOIN "groups" g ON g.category_id = c.id
			GROUP BY c.id, c.name
			ORDER BY count DESC
			LIMIT 10
		`).Scan(&results)
		stats.TopCategories = results
	}()

	// Orphaned resources
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Raw(`
			SELECT COUNT(*) FROM resources r
			LEFT JOIN resource_tags rt ON rt.resource_id = r.id
			WHERE rt.resource_id IS NULL
		`).Scan(&stats.OrphanedResources.WithoutTags)

		ctx.db.Raw(`
			SELECT COUNT(*) FROM resources r
			LEFT JOIN groups_related_resources gr ON gr.resource_id = r.id
			WHERE gr.resource_id IS NULL
		`).Scan(&stats.OrphanedResources.WithoutGroups)
	}()

	// Similarity stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.ImageHash{}).Count(&stats.SimilarityStats.TotalHashes)
		ctx.db.Model(&models.ResourceSimilarity{}).Count(&stats.SimilarityStats.SimilarPairsFound)
	}()

	// Log stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		ctx.db.Model(&models.LogEntry{}).Count(&stats.LogStats.TotalEntries)

		type levelCount struct {
			Level string
			Count int64
		}
		var levels []levelCount
		ctx.db.Model(&models.LogEntry{}).
			Select("level, COUNT(*) as count").
			Group("level").
			Scan(&levels)

		stats.LogStats.ByLevel = make(map[string]int64)
		for _, lc := range levels {
			stats.LogStats.ByLevel[lc.Level] = lc.Count
		}

		// Recent errors (last 24h)
		ctx.db.Model(&models.LogEntry{}).
			Where("level = ? AND created_at > ?", models.LogLevelError, time.Now().AddDate(0, 0, -1)).
			Count(&stats.LogStats.RecentErrors)
	}()

	wg.Wait()

	// Default empty slices instead of nil
	if stats.StorageByContentType == nil {
		stats.StorageByContentType = []ContentTypeStorage{}
	}
	if stats.TopTags == nil {
		stats.TopTags = []TagCount{}
	}
	if stats.TopCategories == nil {
		stats.TopCategories = []CategoryCount{}
	}

	return &stats, nil
}
```

**Important:** Join tables verified from model tags: `resource_tags` (tags↔resources), `groups_related_resources` (groups↔resources). Categories use FK `category_id` on groups. Verify `models.ImageHash` and `models.ResourceSimilarity` model names exist.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGetExpensiveStats -v`
Expected: PASS

- [ ] **Step 5: Run all admin tests together**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestGet -v`
Expected: All admin tests PASS

- [ ] **Step 6: Commit**

```bash
git add application_context/admin_context.go application_context/admin_context_test.go
git commit -m "feat(admin): add GetExpensiveStats with storage breakdown, top tags, orphans, log stats"
```

---

### Task 5: API Handlers

**Files:**
- Create: `server/api_handlers/admin_handlers.go`

- [ ] **Step 1: Create the three API handlers**

Create `server/api_handlers/admin_handlers.go`:

```go
package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/server/http_utils"
)

func GetServerStatsHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		stats, err := ctx.GetServerStats()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(stats)
	}
}

func GetDataStatsHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		stats, err := ctx.GetDataStats()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(stats)
	}
}

func GetExpensiveStatsHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		stats, err := ctx.GetExpensiveStats()
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(stats)
	}
}
```

**Note:** These handlers take `*application_context.MahresourcesContext` directly rather than interfaces since the admin stats methods are specific to the admin feature and don't need to be abstracted.

- [ ] **Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles without errors

- [ ] **Step 3: Commit**

```bash
git add server/api_handlers/admin_handlers.go
git commit -m "feat(admin): add API handlers for server-stats, data-stats, and expensive-stats"
```

---

### Task 6: Route Registration & OpenAPI

**Files:**
- Modify: `server/routes.go`
- Modify: `server/routes_openapi.go`

- [ ] **Step 1: Add template route and API routes to routes.go**

In `server/routes.go`, add to the `templates` map (around line 22):

```go
"/admin/overview": {template_context_providers.AdminOverviewContextProvider, "adminOverview.tpl", http.MethodGet},
```

Add API routes after the existing API route registrations (find the block of `router.Methods(...)` calls):

```go
// Admin stats
router.Methods(http.MethodGet).Path("/v1/admin/server-stats").HandlerFunc(api_handlers.GetServerStatsHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/admin/data-stats").HandlerFunc(api_handlers.GetDataStatsHandler(appContext))
router.Methods(http.MethodGet).Path("/v1/admin/data-stats/expensive").HandlerFunc(api_handlers.GetExpensiveStatsHandler(appContext))
```

- [ ] **Step 2: Register OpenAPI endpoints**

In `server/routes_openapi.go`, add a new function:

```go
func registerAdminRoutes(r *openapi.Registry) {
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/server-stats",
		OperationID:          "getServerStats",
		Summary:              "Get server runtime statistics",
		Description:          "Returns runtime information including uptime, memory usage, goroutines, database stats, and worker status. Designed for periodic polling.",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/data-stats",
		OperationID:          "getDataStats",
		Summary:              "Get entity counts and data statistics",
		Description:          "Returns counts for all entity types, total storage usage, growth trends (7/30/90 days), and current configuration summary.",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/data-stats/expensive",
		OperationID:          "getExpensiveStats",
		Summary:              "Get expensive computed statistics",
		Description:          "Returns storage breakdown by content type, top tags and categories, orphaned resources, similarity stats, and log statistics. These queries may be slow on large databases.",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}
```

Then find where other `register*Routes` functions are called (likely in an `init()` or registration function) and add:

```go
registerAdminRoutes(r)
```

- [ ] **Step 3: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles (template file doesn't exist yet, but template compilation is at runtime)

- [ ] **Step 4: Commit**

```bash
git add server/routes.go server/routes_openapi.go
git commit -m "feat(admin): register admin overview routes and OpenAPI endpoints"
```

---

### Task 7: Navigation — Add to Admin Menu

**Files:**
- Modify: `server/template_handlers/template_context_providers/static_template_context.go`

- [ ] **Step 1: Add "Overview" as first item in adminMenu**

In `static_template_context.go`, find the `"adminMenu"` slice (around line 65) and add as the first entry:

```go
"adminMenu": []template_entities.Entry{
	{
		Name: "Overview",
		Url:  "/admin/overview",
	},
	{
		Name: "Categories",
		// ... existing entries unchanged
```

- [ ] **Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles

- [ ] **Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/static_template_context.go
git commit -m "feat(admin): add Overview link to admin dropdown menu"
```

---

### Task 8: Template Context Provider

**Files:**
- Create: `server/template_handlers/template_context_providers/admin_overview_template_context.go`

- [ ] **Step 1: Create the context provider**

Create `server/template_handlers/template_context_providers/admin_overview_template_context.go`:

```go
package template_context_providers

import (
	"net/http"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
)

func AdminOverviewContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		baseContext := staticTemplateCtx(request)

		return pongo2.Context{
			"pageTitle":         "Admin Overview",
			"adminOverviewPage": true,
		}.Update(baseContext)
	}
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build --tags 'json1 fts5'`
Expected: Compiles

- [ ] **Step 3: Commit**

```bash
git add server/template_handlers/template_context_providers/admin_overview_template_context.go
git commit -m "feat(admin): add AdminOverviewContextProvider"
```

---

### Task 9: Alpine.js Component

**Files:**
- Create: `src/components/adminOverview.js`
- Modify: `src/main.js`

- [ ] **Step 1: Create the Alpine.js component**

Create `src/components/adminOverview.js`:

```javascript
import { abortableFetch } from '../index.js';

export function adminOverview() {
    return {
        serverStats: null,
        dataStats: null,
        expensiveStats: null,
        serverLoading: true,
        dataLoading: true,
        expensiveLoading: true,
        serverError: null,
        dataError: null,
        expensiveError: null,
        _pollInterval: null,
        _serverAbort: null,

        init() {
            this.fetchServerStats();
            this.fetchDataStats();
            this.fetchExpensiveStats();

            this._pollInterval = setInterval(() => {
                this.fetchServerStats();
            }, 10000);
        },

        destroy() {
            if (this._pollInterval) {
                clearInterval(this._pollInterval);
            }
            if (this._serverAbort) {
                this._serverAbort();
            }
        },

        fetchServerStats() {
            if (this._serverAbort) {
                this._serverAbort();
            }

            const { abort, ready } = abortableFetch('/v1/admin/server-stats');
            this._serverAbort = abort;

            ready
                .then(r => r.json())
                .then(data => {
                    this.serverStats = data;
                    this.serverError = null;
                })
                .catch(err => {
                    if (err.name !== 'AbortError') {
                        this.serverError = err.message;
                    }
                })
                .finally(() => {
                    this.serverLoading = false;
                });
        },

        fetchDataStats() {
            const { ready } = abortableFetch('/v1/admin/data-stats');

            ready
                .then(r => r.json())
                .then(data => {
                    this.dataStats = data;
                    this.dataError = null;
                })
                .catch(err => {
                    if (err.name !== 'AbortError') {
                        this.dataError = err.message;
                    }
                })
                .finally(() => {
                    this.dataLoading = false;
                });
        },

        fetchExpensiveStats() {
            const { ready } = abortableFetch('/v1/admin/data-stats/expensive');

            ready
                .then(r => r.json())
                .then(data => {
                    this.expensiveStats = data;
                    this.expensiveError = null;
                })
                .catch(err => {
                    if (err.name !== 'AbortError') {
                        this.expensiveError = err.message;
                    }
                })
                .finally(() => {
                    this.expensiveLoading = false;
                });
        },

        formatNumber(n) {
            if (n == null) return '—';
            if (n >= 1_000_000) return (n / 1_000_000).toFixed(2) + 'M';
            if (n >= 1_000) return (n / 1_000).toFixed(n >= 10_000 ? 0 : 1) + 'K';
            return n.toLocaleString();
        },
    };
}
```

- [ ] **Step 2: Register in main.js**

In `src/main.js`, add the import at the top with other component imports:

```javascript
import { adminOverview } from './components/adminOverview.js';
```

And register with Alpine (find the `Alpine.data(...)` block):

```javascript
Alpine.data('adminOverview', adminOverview);
```

- [ ] **Step 3: Build JS**

Run: `npm run build-js`
Expected: Builds without errors

- [ ] **Step 4: Commit**

```bash
git add src/components/adminOverview.js src/main.js
git commit -m "feat(admin): add adminOverview Alpine.js component with polling"
```

---

### Task 10: Page Template

**Files:**
- Create: `templates/adminOverview.tpl`

- [ ] **Step 1: Create the template**

Create `templates/adminOverview.tpl`:

```django
{% extends "/layouts/base.tpl" %}

{% block body %}
<div x-data="adminOverview()" class="max-w-6xl mx-auto px-4 py-6 space-y-6">

    {# Server Health Section #}
    <section aria-label="Server health" aria-live="polite" aria-atomic="true">
        <h2 class="text-lg font-semibold font-mono mb-3">Server Health</h2>
        <template x-if="serverLoading && !serverStats">
            <div class="bg-amber-50 border border-amber-200 rounded p-4 text-sm text-stone-500">Loading server stats...</div>
        </template>
        <template x-if="serverError && !serverStats">
            <div class="bg-red-50 border border-red-200 rounded p-4 text-sm text-red-700" role="alert" x-text="'Error: ' + serverError"></div>
        </template>
        <template x-if="serverStats">
            <div class="bg-amber-50 border border-amber-200 rounded p-4">
                <div class="flex flex-wrap gap-x-6 gap-y-2 text-sm font-mono">
                    <div><span class="text-stone-500">Uptime:</span> <span x-text="serverStats.uptime"></span></div>
                    <div><span class="text-stone-500">Memory:</span> <span x-text="serverStats.memory.heapAllocFormatted"></span> heap, <span x-text="serverStats.memory.sysFormatted"></span> sys</div>
                    <div><span class="text-stone-500">GC runs:</span> <span x-text="serverStats.memory.numGC"></span></div>
                    <div><span class="text-stone-500">Goroutines:</span> <span x-text="serverStats.goroutines"></span></div>
                    <div><span class="text-stone-500">Go:</span> <span x-text="serverStats.goVersion"></span></div>
                    <div><span class="text-stone-500">DB:</span> <span x-text="serverStats.database.type"></span>
                        <template x-if="serverStats.database.dbFileSizeFormatted">
                            <span>(<span x-text="serverStats.database.dbFileSizeFormatted"></span>)</span>
                        </template>
                    </div>
                    <div><span class="text-stone-500">Connections:</span> <span x-text="serverStats.database.openConnections"></span> (<span x-text="serverStats.database.inUse"></span> active)</div>
                    <div><span class="text-stone-500">Hash workers:</span> <span x-text="serverStats.workers.hashWorkerEnabled ? serverStats.workers.hashWorkerCount + ' active' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">Downloads:</span> <span x-text="serverStats.workers.downloadQueueLength"></span> queued</div>
                </div>
            </div>
        </template>
    </section>

    {# Configuration Section #}
    <section aria-label="Configuration">
        <h2 class="text-lg font-semibold font-mono mb-3">Configuration</h2>
        <template x-if="dataLoading && !dataStats">
            <div class="bg-stone-50 border border-stone-200 rounded p-4 text-sm text-stone-500">Loading configuration...</div>
        </template>
        <template x-if="dataStats">
            <div class="bg-stone-50 border border-stone-200 rounded p-4">
                <div class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-x-6 gap-y-1 text-sm font-mono">
                    <div><span class="text-stone-500">Bind:</span> <span x-text="dataStats.config.bindAddress"></span></div>
                    <div><span class="text-stone-500">Storage:</span> <span x-text="dataStats.config.fileSavePath || 'Memory'"></span></div>
                    <div><span class="text-stone-500">Database:</span> <span x-text="dataStats.config.dbType"></span></div>
                    <div><span class="text-stone-500">Read-only DB:</span> <span x-text="dataStats.config.hasReadOnlyDB ? 'Enabled' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">FFmpeg:</span> <span x-text="dataStats.config.ffmpegAvailable ? 'Enabled' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">LibreOffice:</span> <span x-text="dataStats.config.libreOfficeAvailable ? 'Enabled' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">Full-text search:</span> <span x-text="dataStats.config.ftsEnabled ? 'Enabled' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">Hash workers:</span> <span x-text="dataStats.config.hashWorkerEnabled ? dataStats.config.hashWorkerCount + ' workers' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">Alt filesystems:</span> <span x-text="dataStats.config.altFileSystems.length > 0 ? dataStats.config.altFileSystems.join(', ') : 'None'"></span></div>
                    <div><span class="text-stone-500">Ephemeral:</span> <span x-text="dataStats.config.ephemeralMode ? 'Enabled' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">Memory DB:</span> <span x-text="dataStats.config.memoryDB ? 'Enabled' : 'Disabled'"></span></div>
                    <div><span class="text-stone-500">Memory FS:</span> <span x-text="dataStats.config.memoryFS ? 'Enabled' : 'Disabled'"></span></div>
                </div>
            </div>
        </template>
    </section>

    {# Data Overview Section #}
    <section aria-label="Data overview">
        <h2 class="text-lg font-semibold font-mono mb-3">Data Overview</h2>
        <template x-if="dataLoading && !dataStats">
            <div class="bg-white border border-stone-200 rounded p-4 text-sm text-stone-500">Loading data stats...</div>
        </template>
        <template x-if="dataStats">
            <div>
                {# Storage summary #}
                <div class="flex flex-wrap gap-4 mb-4">
                    <div class="bg-white border border-stone-200 rounded p-3 text-center min-w-[120px]">
                        <div class="text-2xl font-semibold font-mono" x-text="dataStats.totalStorageFormatted"></div>
                        <div class="text-xs text-stone-500 font-mono">Total Storage</div>
                    </div>
                    <div class="bg-white border border-stone-200 rounded p-3 text-center min-w-[120px]">
                        <div class="text-2xl font-semibold font-mono" x-text="dataStats.totalVersionStorageFormatted"></div>
                        <div class="text-xs text-stone-500 font-mono">Version Storage</div>
                    </div>
                </div>

                {# Entity count cards #}
                <div class="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-4 xl:grid-cols-6 gap-3">
                    <a href="/resources" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.resources)"></div>
                        <div class="text-xs text-stone-500 font-mono">Resources</div>
                        <template x-if="dataStats.growth.resources.last7Days > 0">
                            <div class="text-xs text-green-600 font-mono" x-text="'+' + dataStats.growth.resources.last7Days + ' this week'"></div>
                        </template>
                    </a>
                    <a href="/notes" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.notes)"></div>
                        <div class="text-xs text-stone-500 font-mono">Notes</div>
                        <template x-if="dataStats.growth.notes.last7Days > 0">
                            <div class="text-xs text-green-600 font-mono" x-text="'+' + dataStats.growth.notes.last7Days + ' this week'"></div>
                        </template>
                    </a>
                    <a href="/groups" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.groups)"></div>
                        <div class="text-xs text-stone-500 font-mono">Groups</div>
                        <template x-if="dataStats.growth.groups.last7Days > 0">
                            <div class="text-xs text-green-600 font-mono" x-text="'+' + dataStats.growth.groups.last7Days + ' this week'"></div>
                        </template>
                    </a>
                    <a href="/tags" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.tags)"></div>
                        <div class="text-xs text-stone-500 font-mono">Tags</div>
                    </a>
                    <a href="/categories" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.categories)"></div>
                        <div class="text-xs text-stone-500 font-mono">Categories</div>
                    </a>
                    <a href="/resourceCategories" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.resourceCategories)"></div>
                        <div class="text-xs text-stone-500 font-mono">Res. Categories</div>
                    </a>
                    <a href="/noteTypes" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.noteTypes)"></div>
                        <div class="text-xs text-stone-500 font-mono">Note Types</div>
                    </a>
                    <a href="/relationTypes" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.relationTypes)"></div>
                        <div class="text-xs text-stone-500 font-mono">Relation Types</div>
                    </a>
                    <a href="/relations" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.relations)"></div>
                        <div class="text-xs text-stone-500 font-mono">Relations</div>
                    </a>
                    <a href="/queries" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.queries)"></div>
                        <div class="text-xs text-stone-500 font-mono">Queries</div>
                    </a>
                    <a href="/logs" class="bg-white border border-stone-200 rounded p-3 text-center hover:border-amber-400 transition-colors">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.logEntries)"></div>
                        <div class="text-xs text-stone-500 font-mono">Log Entries</div>
                    </a>
                    <div class="bg-white border border-stone-200 rounded p-3 text-center">
                        <div class="text-xl font-semibold font-mono" x-text="formatNumber(dataStats.counts.resourceVersions)"></div>
                        <div class="text-xs text-stone-500 font-mono">Versions</div>
                    </div>
                </div>
            </div>
        </template>
    </section>

    {# Expensive Stats Section #}
    <section aria-label="Detailed statistics">
        <h2 class="text-lg font-semibold font-mono mb-3">Detailed Statistics</h2>
        <template x-if="expensiveLoading">
            <div class="bg-white border border-stone-200 rounded p-4 text-sm text-stone-500" aria-live="polite">
                <span class="inline-block animate-spin mr-2">&#8635;</span>Computing detailed statistics...
            </div>
        </template>
        <template x-if="expensiveError">
            <div class="bg-red-50 border border-red-200 rounded p-4 text-sm text-red-700" role="alert" x-text="'Error: ' + expensiveError"></div>
        </template>
        <template x-if="!expensiveLoading && expensiveStats">
            <div class="grid grid-cols-1 md:grid-cols-2 gap-4" aria-live="polite">
                {# Storage by Content Type #}
                <div class="bg-white border border-stone-200 rounded p-4">
                    <h3 class="text-sm font-semibold font-mono text-stone-600 mb-2">Storage by Content Type</h3>
                    <template x-if="expensiveStats.storageByContentType.length === 0">
                        <p class="text-sm text-stone-400">No resources</p>
                    </template>
                    <div class="space-y-1 text-sm font-mono">
                        <template x-for="ct in expensiveStats.storageByContentType" :key="ct.contentType">
                            <div class="flex justify-between">
                                <span class="text-stone-600 truncate mr-2" x-text="ct.contentType"></span>
                                <span class="whitespace-nowrap"><span x-text="ct.formatted"></span> <span class="text-stone-400">(<span x-text="formatNumber(ct.count)"></span>)</span></span>
                            </div>
                        </template>
                    </div>
                </div>

                {# Top Tags #}
                <div class="bg-white border border-stone-200 rounded p-4">
                    <h3 class="text-sm font-semibold font-mono text-stone-600 mb-2">Top Tags</h3>
                    <template x-if="expensiveStats.topTags.length === 0">
                        <p class="text-sm text-stone-400">No tagged resources</p>
                    </template>
                    <div class="space-y-1 text-sm font-mono">
                        <template x-for="tag in expensiveStats.topTags" :key="tag.id">
                            <div class="flex justify-between">
                                <a :href="'/tag?id=' + tag.id" class="text-amber-700 hover:underline truncate mr-2" x-text="tag.name"></a>
                                <span class="text-stone-500 whitespace-nowrap" x-text="formatNumber(tag.count)"></span>
                            </div>
                        </template>
                    </div>
                </div>

                {# Top Categories #}
                <div class="bg-white border border-stone-200 rounded p-4">
                    <h3 class="text-sm font-semibold font-mono text-stone-600 mb-2">Top Categories</h3>
                    <template x-if="expensiveStats.topCategories.length === 0">
                        <p class="text-sm text-stone-400">No categorized resources</p>
                    </template>
                    <div class="space-y-1 text-sm font-mono">
                        <template x-for="cat in expensiveStats.topCategories" :key="cat.id">
                            <div class="flex justify-between">
                                <a :href="'/category?id=' + cat.id" class="text-amber-700 hover:underline truncate mr-2" x-text="cat.name"></a>
                                <span class="text-stone-500 whitespace-nowrap" x-text="formatNumber(cat.count)"></span>
                            </div>
                        </template>
                    </div>
                </div>

                {# Orphaned Resources + Similarity + Logs #}
                <div class="bg-white border border-stone-200 rounded p-4 space-y-4">
                    <div>
                        <h3 class="text-sm font-semibold font-mono text-stone-600 mb-2">Orphaned Resources</h3>
                        <div class="space-y-1 text-sm font-mono">
                            <div class="flex justify-between">
                                <span class="text-stone-600">Without tags</span>
                                <span role="status" x-text="formatNumber(expensiveStats.orphanedResources.withoutTags)"
                                      :class="expensiveStats.orphanedResources.withoutTags > 0 ? 'text-amber-600' : 'text-stone-500'"></span>
                            </div>
                            <div class="flex justify-between">
                                <span class="text-stone-600">Without groups</span>
                                <span role="status" x-text="formatNumber(expensiveStats.orphanedResources.withoutGroups)"
                                      :class="expensiveStats.orphanedResources.withoutGroups > 0 ? 'text-amber-600' : 'text-stone-500'"></span>
                            </div>
                        </div>
                    </div>

                    <div>
                        <h3 class="text-sm font-semibold font-mono text-stone-600 mb-2">Similarity Detection</h3>
                        <div class="space-y-1 text-sm font-mono">
                            <div class="flex justify-between">
                                <span class="text-stone-600">Hashed resources</span>
                                <span class="text-stone-500" x-text="formatNumber(expensiveStats.similarityStats.totalHashes)"></span>
                            </div>
                            <div class="flex justify-between">
                                <span class="text-stone-600">Similar pairs</span>
                                <span class="text-stone-500" x-text="formatNumber(expensiveStats.similarityStats.similarPairsFound)"></span>
                            </div>
                        </div>
                    </div>

                    <div>
                        <h3 class="text-sm font-semibold font-mono text-stone-600 mb-2">Log Statistics</h3>
                        <div class="space-y-1 text-sm font-mono">
                            <div class="flex justify-between">
                                <span class="text-stone-600">Total entries</span>
                                <span class="text-stone-500" x-text="formatNumber(expensiveStats.logStats.totalEntries)"></span>
                            </div>
                            <template x-for="(count, level) in expensiveStats.logStats.byLevel" :key="level">
                                <div class="flex justify-between">
                                    <span class="text-stone-600" x-text="level"></span>
                                    <span x-text="formatNumber(count)" :class="level === 'error' ? 'text-red-600' : 'text-stone-500'"></span>
                                </div>
                            </template>
                            <div class="flex justify-between border-t border-stone-100 pt-1 mt-1">
                                <span class="text-stone-600">Errors (24h)</span>
                                <span role="status" x-text="formatNumber(expensiveStats.logStats.recentErrors)"
                                      :class="expensiveStats.logStats.recentErrors > 0 ? 'text-red-600 font-semibold' : 'text-stone-500'"></span>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </template>
    </section>
</div>
{% endblock %}
```

- [ ] **Step 2: Build everything**

Run: `npm run build`
Expected: CSS, JS, and Go binary all build successfully

- [ ] **Step 3: Manual smoke test**

Start the server in ephemeral mode and verify the page renders:

Run: `./mahresources -ephemeral -bind-address=:8181`

Navigate to `http://localhost:8181/admin/overview`. Verify:
- Server health section loads with uptime, memory, etc.
- Configuration section shows current config
- Data overview shows entity count cards (all zeros in ephemeral mode)
- Detailed statistics section loads (empty/zero state)
- Admin dropdown in nav has "Overview" as first item

Stop the server after verification.

- [ ] **Step 4: Commit**

```bash
git add templates/adminOverview.tpl
git commit -m "feat(admin): add admin overview page template with all stat sections"
```

---

### Task 11: CLI Command

**Files:**
- Create: `cmd/mr/commands/admin.go`
- Modify: `cmd/mr/main.go`

- [ ] **Step 1: Create the admin CLI command**

Create `cmd/mr/commands/admin.go`:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"net/url"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

func NewAdminCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		serverOnly bool
		dataOnly   bool
	)

	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Show server and data statistics",
		Long:  "Display server health, configuration, entity counts, and detailed analytics.",
		RunE: func(cmd *cobra.Command, args []string) error {
			showAll := !serverOnly && !dataOnly

			if showAll || serverOnly {
				var raw json.RawMessage
				if err := c.Get("/v1/admin/server-stats", nil, &raw); err != nil {
					return fmt.Errorf("fetching server stats: %w", err)
				}

				if opts.JSON {
					output.PrintRawJSON(raw)
					if showAll {
						fmt.Println() // separator between JSON blocks
					}
				} else {
					var stats serverStatsResponse
					if err := json.Unmarshal(raw, &stats); err != nil {
						return fmt.Errorf("parsing server stats: %w", err)
					}
					printServerStats(stats)
				}
			}

			if showAll || dataOnly {
				var raw json.RawMessage
				if err := c.Get("/v1/admin/data-stats", nil, &raw); err != nil {
					return fmt.Errorf("fetching data stats: %w", err)
				}

				if opts.JSON {
					output.PrintRawJSON(raw)
					if showAll {
						fmt.Println()
					}
				} else {
					var stats dataStatsResponse
					if err := json.Unmarshal(raw, &stats); err != nil {
						return fmt.Errorf("parsing data stats: %w", err)
					}
					printDataStats(stats)
				}
			}

			if showAll {
				var raw json.RawMessage
				q := url.Values{}
				if err := c.Get("/v1/admin/data-stats/expensive", q, &raw); err != nil {
					return fmt.Errorf("fetching expensive stats: %w", err)
				}

				if opts.JSON {
					output.PrintRawJSON(raw)
				} else {
					var stats expensiveStatsResponse
					if err := json.Unmarshal(raw, &stats); err != nil {
						return fmt.Errorf("parsing expensive stats: %w", err)
					}
					printExpensiveStats(stats)
				}
			}

			return nil
		},
	}

	cmd.Flags().BoolVar(&serverOnly, "server", false, "Show only server stats")
	cmd.Flags().BoolVar(&dataOnly, "data", false, "Show only data stats")

	return cmd
}

// Response types for JSON parsing

type serverStatsResponse struct {
	Uptime    string `json:"uptime"`
	GoVersion string `json:"goVersion"`
	Memory    struct {
		HeapAllocFormatted string `json:"heapAllocFormatted"`
		SysFormatted       string `json:"sysFormatted"`
		NumGC              uint32 `json:"numGC"`
	} `json:"memory"`
	Goroutines int `json:"goroutines"`
	Database   struct {
		Type                string `json:"type"`
		OpenConnections     int    `json:"openConnections"`
		InUse               int    `json:"inUse"`
		DBFileSizeFormatted string `json:"dbFileSizeFormatted"`
	} `json:"database"`
	Workers struct {
		HashWorkerEnabled   bool `json:"hashWorkerEnabled"`
		HashWorkerCount     int  `json:"hashWorkerCount"`
		DownloadQueueLength int  `json:"downloadQueueLength"`
	} `json:"workers"`
}

type dataStatsResponse struct {
	Counts struct {
		Resources          int64 `json:"resources"`
		Notes              int64 `json:"notes"`
		Groups             int64 `json:"groups"`
		Tags               int64 `json:"tags"`
		Categories         int64 `json:"categories"`
		ResourceCategories int64 `json:"resourceCategories"`
		NoteTypes          int64 `json:"noteTypes"`
		RelationTypes      int64 `json:"relationTypes"`
		Relations          int64 `json:"relations"`
		Queries            int64 `json:"queries"`
		LogEntries         int64 `json:"logEntries"`
		ResourceVersions   int64 `json:"resourceVersions"`
	} `json:"counts"`
	TotalStorageFormatted        string `json:"totalStorageFormatted"`
	TotalVersionStorageFormatted string `json:"totalVersionStorageFormatted"`
	Growth                       struct {
		Resources struct{ Last7Days int64 `json:"last7Days"` } `json:"resources"`
		Notes     struct{ Last7Days int64 `json:"last7Days"` } `json:"notes"`
		Groups    struct{ Last7Days int64 `json:"last7Days"` } `json:"groups"`
	} `json:"growth"`
}

type expensiveStatsResponse struct {
	StorageByContentType []struct {
		ContentType string `json:"contentType"`
		Count       int64  `json:"count"`
		Formatted   string `json:"formatted"`
	} `json:"storageByContentType"`
	OrphanedResources struct {
		WithoutTags   int64 `json:"withoutTags"`
		WithoutGroups int64 `json:"withoutGroups"`
	} `json:"orphanedResources"`
	LogStats struct {
		TotalEntries int64            `json:"totalEntries"`
		ByLevel      map[string]int64 `json:"byLevel"`
		RecentErrors int64            `json:"recentErrors"`
	} `json:"logStats"`
}

func printServerStats(s serverStatsResponse) {
	fmt.Println("=== Server Health ===")
	output.PrintSingle(output.Options{}, []output.KeyValue{
		{Key: "Uptime", Value: s.Uptime},
		{Key: "Go Version", Value: s.GoVersion},
		{Key: "Heap Memory", Value: s.Memory.HeapAllocFormatted},
		{Key: "System Memory", Value: s.Memory.SysFormatted},
		{Key: "GC Runs", Value: fmt.Sprintf("%d", s.Memory.NumGC)},
		{Key: "Goroutines", Value: fmt.Sprintf("%d", s.Goroutines)},
		{Key: "Database", Value: fmt.Sprintf("%s (%s)", s.Database.Type, s.Database.DBFileSizeFormatted)},
		{Key: "DB Connections", Value: fmt.Sprintf("%d open, %d active", s.Database.OpenConnections, s.Database.InUse)},
		{Key: "Hash Workers", Value: fmt.Sprintf("%v (%d)", s.Workers.HashWorkerEnabled, s.Workers.HashWorkerCount)},
		{Key: "Download Queue", Value: fmt.Sprintf("%d", s.Workers.DownloadQueueLength)},
	}, nil)
	fmt.Println()
}

func printDataStats(s dataStatsResponse) {
	fmt.Println("=== Data Overview ===")
	output.PrintSingle(output.Options{}, []output.KeyValue{
		{Key: "Resources", Value: fmt.Sprintf("%d (+%d this week)", s.Counts.Resources, s.Growth.Resources.Last7Days)},
		{Key: "Notes", Value: fmt.Sprintf("%d (+%d this week)", s.Counts.Notes, s.Growth.Notes.Last7Days)},
		{Key: "Groups", Value: fmt.Sprintf("%d (+%d this week)", s.Counts.Groups, s.Growth.Groups.Last7Days)},
		{Key: "Tags", Value: fmt.Sprintf("%d", s.Counts.Tags)},
		{Key: "Categories", Value: fmt.Sprintf("%d", s.Counts.Categories)},
		{Key: "Relations", Value: fmt.Sprintf("%d", s.Counts.Relations)},
		{Key: "Queries", Value: fmt.Sprintf("%d", s.Counts.Queries)},
		{Key: "Log Entries", Value: fmt.Sprintf("%d", s.Counts.LogEntries)},
		{Key: "Versions", Value: fmt.Sprintf("%d", s.Counts.ResourceVersions)},
		{Key: "Total Storage", Value: s.TotalStorageFormatted},
		{Key: "Version Storage", Value: s.TotalVersionStorageFormatted},
	}, nil)
	fmt.Println()
}

func printExpensiveStats(s expensiveStatsResponse) {
	fmt.Println("=== Detailed Statistics ===")

	if len(s.StorageByContentType) > 0 {
		fmt.Println("\nStorage by Content Type:")
		columns := []string{"TYPE", "COUNT", "SIZE"}
		var rows [][]string
		for _, ct := range s.StorageByContentType {
			rows = append(rows, []string{ct.ContentType, fmt.Sprintf("%d", ct.Count), ct.Formatted})
		}
		output.Print(output.Options{}, columns, rows, nil)
	}

	fmt.Printf("\nOrphaned: %d without tags, %d without groups\n",
		s.OrphanedResources.WithoutTags, s.OrphanedResources.WithoutGroups)

	fmt.Printf("Logs: %d total, %d errors (24h)\n",
		s.LogStats.TotalEntries, s.LogStats.RecentErrors)
}
```

- [ ] **Step 2: Register in cmd/mr/main.go**

Add to the `rootCmd.AddCommand(...)` block:

```go
rootCmd.AddCommand(commands.NewAdminCmd(c, opts))
```

- [ ] **Step 3: Build and verify**

Run: `go build --tags 'json1 fts5' ./cmd/mr/`
Expected: Compiles

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/admin.go cmd/mr/main.go
git commit -m "feat(admin): add mr admin CLI command with --server, --data, --json flags"
```

---

### Task 12: E2E Browser Tests

**Files:**
- Create: `e2e/tests/admin-overview.spec.ts`

- [ ] **Step 1: Write the E2E browser tests**

Create `e2e/tests/admin-overview.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('Admin Overview', () => {
  test('should be accessible from admin dropdown', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/dashboard`);
    // Open admin dropdown and click Overview
    await page.locator('.navbar-link--dropdown:has-text("Admin")').click();
    await page.locator('.navbar-dropdown-item:has-text("Overview")').click();
    await page.waitForURL(/\/admin\/overview/);
    expect(page.url()).toContain('/admin/overview');
  });

  test('should load admin overview page', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);
    await expect(page.locator('h2:has-text("Server Health")')).toBeVisible();
    await expect(page.locator('h2:has-text("Configuration")')).toBeVisible();
    await expect(page.locator('h2:has-text("Data Overview")')).toBeVisible();
    await expect(page.locator('h2:has-text("Detailed Statistics")')).toBeVisible();
  });

  test('should show server stats after loading', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);
    // Wait for server stats to load (uptime should appear)
    await expect(page.locator('text=Uptime:')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('text=Memory:')).toBeVisible();
    await expect(page.locator('text=Goroutines:')).toBeVisible();
  });

  test('should show entity counts after loading', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);
    // Wait for data stats to load
    await expect(page.locator('text=Total Storage')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('text=Resources')).toBeVisible();
    await expect(page.locator('text=Notes')).toBeVisible();
    await expect(page.locator('text=Tags')).toBeVisible();
  });

  test('should load expensive stats asynchronously', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);
    // Wait for expensive stats section to finish loading
    await expect(page.locator('text=Storage by Content Type')).toBeVisible({ timeout: 30000 });
    await expect(page.locator('text=Top Tags')).toBeVisible();
    await expect(page.locator('text=Orphaned Resources')).toBeVisible();
    await expect(page.locator('text=Log Statistics')).toBeVisible();
  });

  test('entity count cards should link to list pages', async ({ page, baseURL }) => {
    await page.goto(`${baseURL}/admin/overview`);
    await expect(page.locator('text=Resources')).toBeVisible({ timeout: 10000 });

    // Verify resource link goes to /resources
    const resourceLink = page.locator('a[href="/resources"]:has-text("Resources")');
    await expect(resourceLink).toBeVisible();
  });
});

test.describe('Admin Overview with data', () => {
  test('should reflect created entities in counts', async ({ page, baseURL, apiClient }) => {
    const tagName = `AdminTest Tag ${Date.now()}`;
    const tag = await apiClient.createTag(tagName, 'test');

    try {
      await page.goto(`${baseURL}/admin/overview`);
      // Wait for data stats to load, then verify tag count > 0
      await expect(page.locator('text=Total Storage')).toBeVisible({ timeout: 10000 });
      const tagCard = page.locator('a[href="/tags"]');
      await expect(tagCard).toBeVisible();
    } finally {
      try { await apiClient.deleteTag(tag.ID); } catch { /* cleanup */ }
    }
  });
});
```

- [ ] **Step 2: Run the E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep "Admin Overview"`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/admin-overview.spec.ts
git commit -m "test(admin): add E2E browser tests for admin overview page"
```

---

### Task 13: Accessibility Tests

**Files:**
- Modify: `e2e/helpers/accessibility/a11y-config.ts`

- [ ] **Step 1: Add admin overview to static pages**

In `e2e/helpers/accessibility/a11y-config.ts`, add to the `STATIC_PAGES` array (after the dashboard entry):

```typescript
{ path: '/admin/overview', name: 'Admin overview' },
```

- [ ] **Step 2: Run accessibility tests**

Run: `cd e2e && npm run test:with-server:a11y`
Expected: Admin overview passes axe-core scan

- [ ] **Step 3: Commit**

```bash
git add e2e/helpers/accessibility/a11y-config.ts
git commit -m "test(admin): add admin overview to accessibility test suite"
```

---

### Task 14: CLI E2E Tests

**Files:**
- Create: `e2e/tests/cli/cli-admin.spec.ts`

- [ ] **Step 1: Write CLI E2E tests**

Create `e2e/tests/cli/cli-admin.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/cli.fixture';

test.describe('CLI: admin', () => {
  test('shows all stats by default', async ({ cli }) => {
    const result = cli.runOrFail('admin');
    expect(result.stdout).toContain('Server Health');
    expect(result.stdout).toContain('Uptime');
    expect(result.stdout).toContain('Data Overview');
    expect(result.stdout).toContain('Resources');
  });

  test('--server shows only server stats', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--server');
    expect(result.stdout).toContain('Server Health');
    expect(result.stdout).toContain('Uptime');
    expect(result.stdout).not.toContain('Data Overview');
  });

  test('--data shows only data stats', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--data');
    expect(result.stdout).toContain('Data Overview');
    expect(result.stdout).toContain('Resources');
    expect(result.stdout).not.toContain('Server Health');
  });

  test('--json outputs valid JSON', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--server', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toHaveProperty('uptime');
    expect(parsed).toHaveProperty('memory');
    expect(parsed).toHaveProperty('goroutines');
  });

  test('--data --json outputs valid JSON', async ({ cli }) => {
    const result = cli.runOrFail('admin', '--data', '--json');
    const parsed = JSON.parse(result.stdout);
    expect(parsed).toHaveProperty('counts');
    expect(parsed).toHaveProperty('totalStorageFormatted');
    expect(parsed).toHaveProperty('growth');
  });
});
```

**Important:** Check the `cli.fixture.ts` to verify the exact fixture API. The `runOrFail` method may return the stdout string directly, or it may return an object with `.stdout`. Adjust accordingly.

- [ ] **Step 2: Run CLI E2E tests**

Run: `cd e2e && npm run test:with-server:cli -- --grep "admin"`
Expected: All tests PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/cli/cli-admin.spec.ts
git commit -m "test(admin): add CLI E2E tests for mr admin command"
```

---

### Task 15: Documentation

**Files:**
- Create: `docs-site/docs/features/admin-overview.md`
- Modify: `docs-site/sidebars.ts`

- [ ] **Step 1: Create the documentation page**

Create `docs-site/docs/features/admin-overview.md`:

```markdown
---
sidebar_position: 18
title: Admin Overview
---

# Admin Overview

The admin overview page provides a single view of server health, configuration, and data statistics. Access it from the **Admin** dropdown in the navigation bar.

![Admin overview page showing server stats and entity counts](/img/admin-overview.png)

## Server Health

The top section displays real-time server metrics that auto-refresh every 10 seconds:

| Metric | Description |
|--------|-------------|
| Uptime | How long the server has been running |
| Memory | Go heap and system memory usage |
| GC runs | Number of garbage collection cycles |
| Goroutines | Active Go goroutines |
| Go version | Runtime version |
| Database | Type, file size (SQLite), and connection pool stats |
| Hash workers | Background perceptual hash worker status |
| Downloads | Active download queue length |

## Configuration

Shows current runtime configuration including storage paths, enabled features (FFmpeg, LibreOffice, full-text search), hash worker settings, and mode flags (ephemeral, memory DB/FS).

## Data Overview

Displays counts for all entity types with links to their respective list pages. Resources, notes, and groups include weekly growth indicators showing how many were added in the last 7 days. Total storage usage (resources and versions) is shown prominently.

## Detailed Statistics

These analytics load asynchronously since they involve heavier database queries:

- **Storage by content type** -- breakdown of file storage by MIME type
- **Top tags and categories** -- most-used tags and categories by resource count
- **Orphaned resources** -- resources without any tags or group membership
- **Similarity detection** -- number of perceptual hashes computed and similar pairs found
- **Log statistics** -- total log entries by level and recent errors (last 24 hours)

## CLI

The `mr admin` command provides the same information in terminal format:

```bash
# Show all stats
mr admin

# Server stats only
mr admin --server

# Data stats only
mr admin --data

# Raw JSON output
mr admin --json
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /v1/admin/server-stats` | Runtime server statistics (designed for polling) |
| `GET /v1/admin/data-stats` | Entity counts, storage, growth, and configuration |
| `GET /v1/admin/data-stats/expensive` | Computed analytics (may be slow on large databases) |
```

- [ ] **Step 2: Add to sidebar**

In `docs-site/sidebars.ts`, add `'features/admin-overview'` to the "Advanced Features" items array:

```typescript
{
  type: 'category',
  label: 'Advanced Features',
  items: [
    'features/admin-overview',
    'features/versioning',
    // ... rest unchanged
  ],
},
```

- [ ] **Step 3: Commit**

```bash
git add docs-site/docs/features/admin-overview.md docs-site/sidebars.ts
git commit -m "docs: add admin overview documentation page"
```

---

### Task 16: Final Verification

- [ ] **Step 1: Run all Go tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: All tests PASS (including new admin tests)

- [ ] **Step 2: Run all E2E tests**

Run: `cd e2e && npm run test:with-server:all`
Expected: All browser + CLI tests PASS

- [ ] **Step 3: Generate updated OpenAPI spec**

Run: `go run ./cmd/openapi-gen`
Expected: Spec generates successfully and includes the three new admin endpoints

- [ ] **Step 4: Retake screenshots for docs**

Run the screenshot pipeline to capture the admin overview page for documentation:
Follow the `retake-screenshots` skill/process.

- [ ] **Step 5: Final commit if any generated files changed**

```bash
git add -A
git status
# If there are changes (e.g., updated OpenAPI spec):
git commit -m "chore: update generated files for admin overview"
```
