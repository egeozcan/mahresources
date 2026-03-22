package application_context

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
)

// ---- helper functions ----

// formatBytes converts a byte count into a human-readable string (e.g. "1.23 MB").
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
	return fmt.Sprintf("%.2f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// formatDuration converts a duration into a human-readable string (e.g. "2d 3h 15m 4s").
func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	days := int(d.Hours()) / 24
	d -= time.Duration(days*24) * time.Hour
	hours := int(d.Hours())
	d -= time.Duration(hours) * time.Hour
	minutes := int(d.Minutes())
	d -= time.Duration(minutes) * time.Minute
	seconds := int(d.Seconds())

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// ---- Task 2: GetServerStats ----

// ServerStats holds runtime information about the server.
type ServerStats struct {
	// Uptime
	Uptime          string    `json:"uptime"`
	UptimeSeconds   float64   `json:"uptimeSeconds"`
	StartedAt       time.Time `json:"startedAt"`
	// Memory
	HeapAlloc       uint64 `json:"heapAlloc"`
	HeapInUse       uint64 `json:"heapInUse"`
	Sys             uint64 `json:"sys"`
	NumGC           uint32 `json:"numGC"`
	HeapAllocFmt    string `json:"heapAllocFmt"`
	HeapInUseFmt    string `json:"heapInUseFmt"`
	SysFmt          string `json:"sysFmt"`
	// Runtime
	Goroutines      int    `json:"goroutines"`
	GoVersion       string `json:"goVersion"`
	// Database
	DBType          string `json:"dbType"`
	DBOpenConns     int    `json:"dbOpenConns"`
	DBIdleConns     int    `json:"dbIdleConns"`
	DBInUse         int    `json:"dbInUse"`
	DBFileSizeBytes int64  `json:"dbFileSizeBytes"`
	DBFileSizeFmt   string `json:"dbFileSizeFmt"`
	// Workers
	HashWorkerEnabled    bool `json:"hashWorkerEnabled"`
	HashWorkerCount      int  `json:"hashWorkerCount"`
	DownloadQueueLength  int  `json:"downloadQueueLength"`
}

// GetServerStats returns runtime information about the server.
func (ctx *MahresourcesContext) GetServerStats() (*ServerStats, error) {
	now := time.Now()
	uptime := now.Sub(ctx.StartedAt)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	stats := &ServerStats{
		Uptime:        formatDuration(uptime),
		UptimeSeconds: uptime.Seconds(),
		StartedAt:     ctx.StartedAt,

		HeapAlloc:    mem.HeapAlloc,
		HeapInUse:    mem.HeapInuse,
		Sys:          mem.Sys,
		NumGC:        mem.NumGC,
		HeapAllocFmt: formatBytes(int64(mem.HeapAlloc)),
		HeapInUseFmt: formatBytes(int64(mem.HeapInuse)),
		SysFmt:       formatBytes(int64(mem.Sys)),

		Goroutines: runtime.NumGoroutine(),
		GoVersion:  runtime.Version(),

		DBType: ctx.Config.DbType,

		HashWorkerEnabled:   ctx.Config.HashWorkerEnabled,
		HashWorkerCount:     ctx.Config.HashWorkerCount,
		DownloadQueueLength: ctx.downloadManager.ActiveCount(),
	}

	// Database connection pool stats
	if sqlDB, err := ctx.db.DB(); err == nil {
		dbStats := sqlDB.Stats()
		stats.DBOpenConns = dbStats.OpenConnections
		stats.DBIdleConns = dbStats.Idle
		stats.DBInUse = dbStats.InUse
	}

	// SQLite file size
	if ctx.Config.DbType == constants.DbTypeSqlite && ctx.Config.DbDsn != "" {
		// Extract the file path from the DSN (strip query params and "file:" prefix)
		dsn := ctx.Config.DbDsn
		// Handle "file:path?..." format
		if len(dsn) > 5 && dsn[:5] == "file:" {
			dsn = dsn[5:]
		}
		// Strip query string
		if idx := len(dsn); idx > 0 {
			for i, c := range dsn {
				if c == '?' {
					dsn = dsn[:i]
					break
				}
			}
		}
		if info, err := os.Stat(dsn); err == nil {
			stats.DBFileSizeBytes = info.Size()
			stats.DBFileSizeFmt = formatBytes(info.Size())
		}
	}

	return stats, nil
}

// ---- Task 3: GetDataStats ----

// EntityCounts holds the count of each entity type.
type EntityCounts struct {
	Resources         int64 `json:"resources"`
	Notes             int64 `json:"notes"`
	Groups            int64 `json:"groups"`
	Tags              int64 `json:"tags"`
	Categories        int64 `json:"categories"`
	ResourceCategories int64 `json:"resourceCategories"`
	NoteTypes         int64 `json:"noteTypes"`
	Queries           int64 `json:"queries"`
	GroupRelations    int64 `json:"groupRelations"`
	GroupRelationTypes int64 `json:"groupRelationTypes"`
	LogEntries        int64 `json:"logEntries"`
	ResourceVersions  int64 `json:"resourceVersions"`
}

// GrowthPeriods holds creation counts for a single period (e.g. 7 days).
type GrowthPeriods struct {
	Resources int64 `json:"resources"`
	Notes     int64 `json:"notes"`
	Groups    int64 `json:"groups"`
}

// GrowthStats holds entity creation counts for the last 7, 30, and 90 days.
type GrowthStats struct {
	Last7Days  GrowthPeriods `json:"last7Days"`
	Last30Days GrowthPeriods `json:"last30Days"`
	Last90Days GrowthPeriods `json:"last90Days"`
}

// ConfigSummary holds a subset of server configuration values.
type ConfigSummary struct {
	DbType          string `json:"dbType"`
	EphemeralMode   bool   `json:"ephemeralMode"`
	MemoryDB        bool   `json:"memoryDb"`
	MemoryFS        bool   `json:"memoryFs"`
	FTSEnabled      bool   `json:"ftsEnabled"`
	HashWorkerEnabled bool `json:"hashWorkerEnabled"`
}

// DataStats holds entity counts, storage totals, growth stats, and config summary.
type DataStats struct {
	Entities        EntityCounts `json:"entities"`
	StorageTotalBytes int64      `json:"storageTotalBytes"`
	StorageTotalFmt   string     `json:"storageTotalFmt"`
	Growth          GrowthStats  `json:"growth"`
	Config          ConfigSummary `json:"config"`
}

// GetResourceVersionsCount returns the total count of resource versions.
func (ctx *MahresourcesContext) GetResourceVersionsCount() (int64, error) {
	var count int64
	return count, ctx.db.Model(&models.ResourceVersion{}).Count(&count).Error
}

// GetDataStats returns entity counts, storage totals, and growth stats.
func (ctx *MahresourcesContext) GetDataStats() (*DataStats, error) {
	stats := &DataStats{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	// Entity counts (all in parallel)
	type countResult struct {
		field *int64
		fn    func() (int64, error)
	}

	counts := []countResult{
		{&stats.Entities.Resources, func() (int64, error) {
			return ctx.GetResourceCount(&query_models.ResourceSearchQuery{})
		}},
		{&stats.Entities.Notes, func() (int64, error) {
			return ctx.GetNoteCount(&query_models.NoteQuery{})
		}},
		{&stats.Entities.Groups, func() (int64, error) {
			return ctx.GetGroupsCount(&query_models.GroupQuery{})
		}},
		{&stats.Entities.Tags, func() (int64, error) {
			return ctx.GetTagsCount(&query_models.TagQuery{})
		}},
		{&stats.Entities.Categories, func() (int64, error) {
			return ctx.GetCategoriesCount(&query_models.CategoryQuery{})
		}},
		{&stats.Entities.ResourceCategories, func() (int64, error) {
			return ctx.GetResourceCategoriesCount(&query_models.ResourceCategoryQuery{})
		}},
		{&stats.Entities.NoteTypes, func() (int64, error) {
			return ctx.GetNoteTypesCount(&query_models.NoteTypeQuery{})
		}},
		{&stats.Entities.Queries, func() (int64, error) {
			return ctx.GetQueriesCount(&query_models.QueryQuery{})
		}},
		{&stats.Entities.GroupRelations, func() (int64, error) {
			return ctx.GetRelationsCount(&query_models.GroupRelationshipQuery{})
		}},
		{&stats.Entities.GroupRelationTypes, func() (int64, error) {
			return ctx.GetRelationTypesCount(&query_models.RelationshipTypeQuery{})
		}},
		{&stats.Entities.LogEntries, func() (int64, error) {
			return ctx.GetLogEntriesCount(&query_models.LogEntryQuery{})
		}},
		{&stats.Entities.ResourceVersions, func() (int64, error) {
			return ctx.GetResourceVersionsCount()
		}},
	}

	for _, cr := range counts {
		cr := cr
		wg.Add(1)
		go func() {
			defer wg.Done()
			n, err := cr.fn()
			if err != nil {
				setErr(err)
				return
			}
			mu.Lock()
			*cr.field = n
			mu.Unlock()
		}()
	}

	// Storage total (resources + resource_versions)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var resourceStorage, versionStorage int64
		if err := ctx.db.Model(&models.Resource{}).Select("COALESCE(SUM(file_size), 0)").Scan(&resourceStorage).Error; err != nil {
			setErr(err)
			return
		}
		if err := ctx.db.Model(&models.ResourceVersion{}).Select("COALESCE(SUM(file_size), 0)").Scan(&versionStorage).Error; err != nil {
			setErr(err)
			return
		}
		total := resourceStorage + versionStorage
		mu.Lock()
		stats.StorageTotalBytes = total
		stats.StorageTotalFmt = formatBytes(total)
		mu.Unlock()
	}()

	// Growth stats
	now := time.Now()
	for _, period := range []struct {
		days int
		dest *GrowthPeriods
	}{
		{7, &stats.Growth.Last7Days},
		{30, &stats.Growth.Last30Days},
		{90, &stats.Growth.Last90Days},
	} {
		period := period
		since := now.AddDate(0, 0, -period.days)

		wg.Add(1)
		go func() {
			defer wg.Done()
			var rCount, nCount, gCount int64
			if err := ctx.db.Model(&models.Resource{}).Where("created_at >= ?", since).Count(&rCount).Error; err != nil {
				setErr(err)
				return
			}
			if err := ctx.db.Model(&models.Note{}).Where("created_at >= ?", since).Count(&nCount).Error; err != nil {
				setErr(err)
				return
			}
			if err := ctx.db.Model(&models.Group{}).Where("created_at >= ?", since).Count(&gCount).Error; err != nil {
				setErr(err)
				return
			}
			mu.Lock()
			period.dest.Resources = rCount
			period.dest.Notes = nCount
			period.dest.Groups = gCount
			mu.Unlock()
		}()
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	stats.Config = ConfigSummary{
		DbType:            ctx.Config.DbType,
		EphemeralMode:     ctx.Config.EphemeralMode,
		MemoryDB:          ctx.Config.MemoryDB,
		MemoryFS:          ctx.Config.MemoryFS,
		FTSEnabled:        ctx.ftsEnabled,
		HashWorkerEnabled: ctx.Config.HashWorkerEnabled,
	}

	return stats, nil
}

// ---- Task 4: GetExpensiveStats ----

// ContentTypeStorage holds storage usage for a single content type.
type ContentTypeStorage struct {
	ContentType string `json:"contentType" gorm:"column:content_type"`
	TotalBytes  int64  `json:"totalBytes" gorm:"column:total_bytes"`
	TotalFmt    string `json:"totalFmt"`
	Count       int64  `json:"count" gorm:"column:count"`
}

// TagCount holds a tag name and its resource count.
type TagCount struct {
	ID    uint   `json:"id" gorm:"column:id"`
	Name  string `json:"name" gorm:"column:name"`
	Count int64  `json:"count" gorm:"column:count"`
}

// CategoryCount holds a category name and its group count.
type CategoryCount struct {
	ID    uint   `json:"id" gorm:"column:id"`
	Name  string `json:"name" gorm:"column:name"`
	Count int64  `json:"count" gorm:"column:count"`
}

// OrphanStats holds counts of resources without tags or groups.
type OrphanStats struct {
	WithoutTags   int64 `json:"withoutTags"`
	WithoutGroups int64 `json:"withoutGroups"`
}

// SimilarityInfo holds counts related to image hashing and similarity.
type SimilarityInfo struct {
	HashedResources    int64 `json:"hashedResources"`
	SimilarityPairs    int64 `json:"similarityPairs"`
}

// LogStatsInfo holds log entry counts by level and recent error count.
type LogStatsInfo struct {
	TotalInfo     int64 `json:"totalInfo"`
	TotalWarning  int64 `json:"totalWarning"`
	TotalError    int64 `json:"totalError"`
	RecentErrors  int64 `json:"recentErrors"` // last 24h
}

// ExpensiveStats holds more resource-intensive statistics.
type ExpensiveStats struct {
	StorageByContentType []ContentTypeStorage `json:"storageByContentType"`
	TopTags              []TagCount           `json:"topTags"`
	TopCategories        []CategoryCount      `json:"topCategories"`
	Orphans              OrphanStats          `json:"orphans"`
	Similarity           SimilarityInfo       `json:"similarity"`
	LogStats             LogStatsInfo         `json:"logStats"`
}

// GetExpensiveStats returns statistics that involve heavier queries.
func (ctx *MahresourcesContext) GetExpensiveStats() (*ExpensiveStats, error) {
	stats := &ExpensiveStats{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	setErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}

	// Storage by content type
	wg.Add(1)
	go func() {
		defer wg.Done()
		var rows []ContentTypeStorage
		err := ctx.db.Model(&models.Resource{}).
			Select("content_type, COALESCE(SUM(file_size), 0) AS total_bytes, COUNT(*) AS count").
			Group("content_type").
			Order("total_bytes DESC").
			Scan(&rows).Error
		if err != nil {
			setErr(err)
			return
		}
		for i := range rows {
			rows[i].TotalFmt = formatBytes(rows[i].TotalBytes)
		}
		mu.Lock()
		stats.StorageByContentType = rows
		mu.Unlock()
	}()

	// Top tags (by resource count)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var rows []TagCount
		err := ctx.db.Table("tags").
			Select("tags.id AS id, tags.name AS name, COUNT(resource_tags.resource_id) AS count").
			Joins("LEFT JOIN resource_tags ON resource_tags.tag_id = tags.id").
			Group("tags.id, tags.name").
			Order("count DESC").
			Limit(10).
			Scan(&rows).Error
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		stats.TopTags = rows
		mu.Unlock()
	}()

	// Top categories (by group count)
	wg.Add(1)
	go func() {
		defer wg.Done()
		var rows []CategoryCount
		err := ctx.db.Table("categories").
			Select("categories.id AS id, categories.name AS name, COUNT(groups.id) AS count").
			Joins("LEFT JOIN \"groups\" ON \"groups\".category_id = categories.id").
			Group("categories.id, categories.name").
			Order("count DESC").
			Limit(10).
			Scan(&rows).Error
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		stats.TopCategories = rows
		mu.Unlock()
	}()

	// Orphaned resources — without tags
	wg.Add(1)
	go func() {
		defer wg.Done()
		var count int64
		err := ctx.db.Table("resources").
			Joins("LEFT JOIN resource_tags ON resource_tags.resource_id = resources.id").
			Where("resource_tags.resource_id IS NULL").
			Count(&count).Error
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		stats.Orphans.WithoutTags = count
		mu.Unlock()
	}()

	// Orphaned resources — without groups
	wg.Add(1)
	go func() {
		defer wg.Done()
		var count int64
		err := ctx.db.Table("resources").
			Joins("LEFT JOIN groups_related_resources ON groups_related_resources.resource_id = resources.id").
			Where("groups_related_resources.resource_id IS NULL").
			Count(&count).Error
		if err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		stats.Orphans.WithoutGroups = count
		mu.Unlock()
	}()

	// Similarity stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		var hashed, pairs int64
		if err := ctx.db.Model(&models.ImageHash{}).Count(&hashed).Error; err != nil {
			setErr(err)
			return
		}
		if err := ctx.db.Model(&models.ResourceSimilarity{}).Count(&pairs).Error; err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		stats.Similarity.HashedResources = hashed
		stats.Similarity.SimilarityPairs = pairs
		mu.Unlock()
	}()

	// Log stats
	wg.Add(1)
	go func() {
		defer wg.Done()
		var infoCount, warnCount, errCount, recentErrCount int64
		if err := ctx.db.Model(&models.LogEntry{}).Where("level = ?", models.LogLevelInfo).Count(&infoCount).Error; err != nil {
			setErr(err)
			return
		}
		if err := ctx.db.Model(&models.LogEntry{}).Where("level = ?", models.LogLevelWarning).Count(&warnCount).Error; err != nil {
			setErr(err)
			return
		}
		if err := ctx.db.Model(&models.LogEntry{}).Where("level = ?", models.LogLevelError).Count(&errCount).Error; err != nil {
			setErr(err)
			return
		}
		since24h := time.Now().Add(-24 * time.Hour)
		if err := ctx.db.Model(&models.LogEntry{}).
			Where("level = ? AND created_at >= ?", models.LogLevelError, since24h).
			Count(&recentErrCount).Error; err != nil {
			setErr(err)
			return
		}
		mu.Lock()
		stats.LogStats.TotalInfo = infoCount
		stats.LogStats.TotalWarning = warnCount
		stats.LogStats.TotalError = errCount
		stats.LogStats.RecentErrors = recentErrCount
		mu.Unlock()
	}()

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return stats, nil
}
