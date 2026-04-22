package application_context

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/fts"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/server/interfaces"
	"mahresources/storage"
)

type PopularTag struct {
	Name  string
	Id    uint
	Count int
}

type MahresourcesConfig struct {
	DbType           string
	DbDsn            string
	DbReadOnlyDsn    string
	AltFileSystems   map[string]string
	FfmpegPath       string
	LibreOfficePath  string
	BindAddress      string
	SharePort        string
	ShareBindAddress string
	// RemoteResourceConnectTimeout is the timeout for connecting to remote URLs (dial, TLS, response headers)
	RemoteResourceConnectTimeout time.Duration
	// RemoteResourceIdleTimeout is how long to wait before erroring if a remote server stops sending data
	RemoteResourceIdleTimeout time.Duration
	// RemoteResourceOverallTimeout is the maximum total time for a remote resource download (default: 30m)
	RemoteResourceOverallTimeout time.Duration
	// ICSCacheMaxEntries is the maximum number of ICS calendar files to cache (default: 100)
	ICSCacheMaxEntries int
	// ICSCacheTTL is how long cached ICS content is considered fresh (default: 30m)
	ICSCacheTTL time.Duration
	// VideoThumbnailTimeout is the max time for a single ffmpeg invocation (default: 30s)
	VideoThumbnailTimeout time.Duration
	// VideoThumbnailLockTimeout is the max time to wait for the video thumbnail lock (default: 60s)
	VideoThumbnailLockTimeout time.Duration
	// VideoThumbnailConcurrency is the max number of concurrent video thumbnail generations (default: 4)
	VideoThumbnailConcurrency uint
	// PluginPath is the directory where Lua plugins are loaded from (default: "./plugins")
	PluginPath string
	// PluginsDisabled disables all plugin loading when true
	PluginsDisabled bool
	// HashWorkerEnabled indicates whether the background hash worker is running
	HashWorkerEnabled bool
	// HashWorkerCount is the number of concurrent hash calculation workers
	HashWorkerCount int
	// HashBatchSize is the number of resources processed per batch cycle
	HashBatchSize int
	// HashPollInterval is the time between batch processing cycles
	HashPollInterval time.Duration
	// HashSimilarityThreshold is the maximum Hamming distance for similarity
	HashSimilarityThreshold int
	// HashCacheSize is the maximum entries in the hash similarity LRU cache
	HashCacheSize int
	// EphemeralMode indicates the server is running in fully ephemeral mode (memory DB + FS)
	EphemeralMode bool
	// MemoryDB indicates the server is using an in-memory SQLite database
	MemoryDB bool
	// MemoryFS indicates the server is using an in-memory filesystem
	MemoryFS bool
	// MaxDBConnections is the connection pool size limit (0 = unlimited)
	MaxDBConnections int
	// FileSavePath is the main file storage directory
	FileSavePath string
	// SkipFTS indicates whether Full-Text Search initialization was skipped
	SkipFTS bool
	// MaxJobConcurrency is the concurrency budget for the shared background job manager
	MaxJobConcurrency int
	// ExportRetention is how long completed group-export tars stay on disk
	ExportRetention time.Duration
	// MaxImportSize is the upper bound on import tar upload size in bytes
	MaxImportSize int64
}

// MahresourcesInputConfig holds all configuration options that can be passed
// via command-line flags or environment variables
type MahresourcesInputConfig struct {
	FileSavePath     string
	DbType           string
	DbDsn            string
	DbReadOnlyDsn    string
	DbLogFile        string
	BindAddress      string
	FfmpegPath       string
	LibreOfficePath  string
	SharePort        string
	ShareBindAddress string
	AltFileSystems   map[string]string
	// MemoryDB uses an in-memory SQLite database (ephemeral, no persistence)
	MemoryDB bool
	// MemoryFS uses an in-memory filesystem (ephemeral, no persistence)
	MemoryFS bool
	// SeedDB is a path to an existing SQLite file to use as the basis for memory-db
	SeedDB string
	// SeedFS is a path to a directory to use as the read-only base for memory-fs (copy-on-write)
	SeedFS string
	// RemoteResourceConnectTimeout is the timeout for connecting to remote URLs (dial, TLS, response headers)
	RemoteResourceConnectTimeout time.Duration
	// RemoteResourceIdleTimeout is how long to wait before erroring if a remote server stops sending data
	RemoteResourceIdleTimeout time.Duration
	// RemoteResourceOverallTimeout is the maximum total time for a remote resource download (default: 30m)
	RemoteResourceOverallTimeout time.Duration
	// MaxDBConnections limits the database connection pool size (useful for SQLite in test environments)
	// When set to 0 (default), no limit is applied
	MaxDBConnections int
	// VideoThumbnailTimeout is the max time for a single ffmpeg invocation (default: 30s)
	VideoThumbnailTimeout time.Duration
	// VideoThumbnailLockTimeout is the max time to wait for the video thumbnail lock (default: 60s)
	VideoThumbnailLockTimeout time.Duration
	// VideoThumbnailConcurrency is the max number of concurrent video thumbnail generations (default: 4)
	VideoThumbnailConcurrency uint
	// PluginPath is the directory where Lua plugins are loaded from (default: "./plugins")
	PluginPath string
	// PluginsDisabled disables all plugin loading when true
	PluginsDisabled bool
	// HashWorkerEnabled indicates whether the background hash worker is running
	HashWorkerEnabled bool
	// HashWorkerCount is the number of concurrent hash calculation workers
	HashWorkerCount int
	// HashBatchSize is the number of resources processed per batch cycle
	HashBatchSize int
	// HashPollInterval is the time between batch processing cycles
	HashPollInterval time.Duration
	// HashSimilarityThreshold is the maximum Hamming distance for similarity
	HashSimilarityThreshold int
	// HashCacheSize is the maximum entries in the hash similarity LRU cache
	HashCacheSize int
	// EphemeralMode indicates the server is running in fully ephemeral mode (memory DB + FS)
	EphemeralMode bool
	// SkipFTS indicates whether Full-Text Search initialization was skipped
	SkipFTS bool
	// MaxJobConcurrency is the concurrency budget for the shared background job manager
	MaxJobConcurrency int
	// ExportRetention is how long completed group-export tars stay on disk
	ExportRetention time.Duration
	// MaxImportSize is the upper bound on import tar upload size in bytes
	MaxImportSize int64
}

type MahresourcesLocks struct {
	ThumbnailGenerationLock      *lib.IDLock[uint]
	VideoThumbnailGenerationLock *lib.IDLock[uint]
	OfficeDocumentGenerationLock *lib.IDLock[uint]
	ResourceHashLock             *lib.IDLock[string]
	VersionUploadLock            *lib.IDLock[uint]
}

type MahresourcesContext struct {
	// StartedAt records when NewMahresourcesContext was called, for uptime calculation
	StartedAt time.Time
	// the main file system
	fs afero.Fs
	// the db connection to the main db with read and write rights
	db *gorm.DB
	// the db readonly connection to the main db
	readOnlyDB *sqlx.DB
	Config     *MahresourcesConfig
	// these are the alternative locations to look at files or import them from
	altFileSystems map[string]afero.Fs
	locks          MahresourcesLocks
	// downloadManager handles background remote URL downloads
	downloadManager *download_queue.DownloadManager
	// searchCache provides caching for global search results
	searchCache *SearchCache
	// currentRequest holds the current HTTP request for logging purposes.
	// This is set per-request via WithRequest() to capture request metadata in logs.
	currentRequest *http.Request
	// hashQueue is a channel to queue resources for async hash processing
	hashQueue chan<- uint
	// thumbnailQueue is a channel to queue video resources for async thumbnail generation
	thumbnailQueue chan<- uint
	// icsCache provides LRU caching for ICS calendar data
	icsCache *ICSCache
	// ftsProvider is the active FTS provider (nil if FTS is not initialized)
	ftsProvider fts.FTSProvider
	// ftsEnabled indicates whether FTS is available
	ftsEnabled bool
	// pluginManager manages Lua plugin loading and hook execution
	pluginManager *plugin_system.PluginManager
	// DefaultResourceCategoryID is the resolved ID of the default resource category.
	// Set at startup; used as the fallback when no category is specified and as the
	// reassignment target when a category is deleted.
	DefaultResourceCategoryID uint
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, readOnlyDB *sqlx.DB, config *MahresourcesConfig) *MahresourcesContext {
	altFileSystems := make(map[string]afero.Fs, len(config.AltFileSystems))

	for key, path := range config.AltFileSystems {
		altFileSystems[key] = storage.CreateStorage(path)
	}

	thumbnailGenerationLock := lib.NewIDLock[uint](uint(0), nil)
	videoThumbConcurrency := config.VideoThumbnailConcurrency
	if videoThumbConcurrency == 0 {
		videoThumbConcurrency = 4
	}
	videoThumbnailGenerationLock := lib.NewIDLock[uint](videoThumbConcurrency, nil)
	officeDocumentGenerationLock := lib.NewIDLock[uint](uint(2), nil)
	resourceHashLock := lib.NewIDLock[string](uint(0), nil)
	versionUploadLock := lib.NewIDLock[uint](uint(0), nil)

	// Initialize search cache with 60 second TTL and 1000 max entries
	searchCache := NewSearchCache(60*time.Second, 1000)

	// Initialize ICS cache with configurable or default values
	icsCacheMaxEntries := config.ICSCacheMaxEntries
	if icsCacheMaxEntries == 0 {
		icsCacheMaxEntries = 100
	}
	icsCacheTTL := config.ICSCacheTTL
	if icsCacheTTL == 0 {
		icsCacheTTL = 30 * time.Minute
	}
	icsCache := NewICSCache(icsCacheMaxEntries, icsCacheTTL)

	ctx := &MahresourcesContext{
		StartedAt:      time.Now(),
		fs:             filesystem,
		db:             db,
		readOnlyDB:     readOnlyDB,
		Config:         config,
		altFileSystems: altFileSystems,
		locks: MahresourcesLocks{
			ThumbnailGenerationLock:      thumbnailGenerationLock,
			VideoThumbnailGenerationLock: videoThumbnailGenerationLock,
			OfficeDocumentGenerationLock: officeDocumentGenerationLock,
			ResourceHashLock:             resourceHashLock,
			VersionUploadLock:            versionUploadLock,
		},
		searchCache:               searchCache,
		icsCache:                  icsCache,
		DefaultResourceCategoryID: 1,
	}

	// Initialize download manager with timeout config and concurrency/retention config
	ctx.downloadManager = download_queue.NewDownloadManagerWithConfig(ctx, download_queue.TimeoutConfig{
		ConnectTimeout: config.RemoteResourceConnectTimeout,
		IdleTimeout:    config.RemoteResourceIdleTimeout,
		OverallTimeout: config.RemoteResourceOverallTimeout,
	}, download_queue.ManagerConfig{
		Concurrency:     config.MaxJobConcurrency,
		ExportRetention: config.ExportRetention,
	})

	// Sweep orphaned export tars at startup. `filesystem` is already rooted at
	// FileSavePath via BasePathFs in disk mode, so the path stays root-relative —
	// matching how resource_upload_context writes into "/resources/...".
	removed, sweepErr := download_queue.SweepOrphanedExports(filesystem, "_exports", ctx.downloadManager.ExportRetention())
	if sweepErr != nil {
		log.Printf("warning: SweepOrphanedExports failed: %v", sweepErr)
	} else if removed > 0 {
		log.Printf("startup: removed %d orphaned export tars", removed)
	}

	removed, sweepErr = download_queue.SweepOrphanedExports(filesystem, "_imports", ctx.downloadManager.ExportRetention())
	if sweepErr != nil {
		log.Printf("warning: sweep _imports failed: %v", sweepErr)
	} else if removed > 0 {
		log.Printf("startup: removed %d orphaned import files", removed)
	}

	// Wire periodic export-tar sweep into the manager's cleanup loop so tars
	// are purged every 5 minutes, not only at startup.
	exportFs := filesystem
	exportRetention := ctx.downloadManager.ExportRetention()
	ctx.downloadManager.SetExportSweepFn(func() {
		n, err := download_queue.SweepOrphanedExports(exportFs, "_exports", exportRetention)
		if err != nil {
			log.Printf("warning: periodic SweepOrphanedExports failed: %v", err)
		} else if n > 0 {
			log.Printf("periodic sweep: removed %d expired export tars", n)
		}
	})

	// Initialize plugin manager unless disabled
	if !config.PluginsDisabled {
		pluginPath := config.PluginPath
		if pluginPath == "" {
			pluginPath = "./plugins"
		}
		pm, pmErr := plugin_system.NewPluginManager(pluginPath)
		if pmErr != nil {
			log.Printf("[plugin] WARNING: failed to initialize plugin system: %v", pmErr)
		} else {
			ctx.pluginManager = pm
			if discovered := pm.DiscoveredPlugins(); len(discovered) > 0 {
				log.Printf("[plugin] Discovered %d plugin(s)", len(discovered))
				for _, p := range discovered {
					log.Printf("[plugin]   - %s v%s", p.Name, p.Version)
				}
			}
			adapter := &pluginDBAdapter{ctx: ctx}
			pm.SetEntityQuerier(adapter)
			pm.SetEntityWriter(adapter)
			pm.SetPluginLogger(adapter)
			pm.SetKVStore(adapter)
			mrqlAdapter := &pluginMRQLAdapter{ctx: ctx}
			pm.SetMRQLExecutor(mrqlAdapter)
		}
	}

	return ctx
}

// PluginManager returns the plugin manager, or nil if plugins are disabled.
func (ctx *MahresourcesContext) PluginManager() *plugin_system.PluginManager {
	return ctx.pluginManager
}

// RegisterAltFs adds an alternative filesystem under the given key. This is
// used at startup (via NewMahresourcesContext from config) and in tests that
// need to inject an in-memory alt-fs without going through disk paths.
func (ctx *MahresourcesContext) RegisterAltFs(key string, fs afero.Fs) {
	ctx.altFileSystems[key] = fs
}

// RunBeforePluginHooks executes before-hooks for the given event.
// If no plugin manager is active, data is returned unmodified.
func (ctx *MahresourcesContext) RunBeforePluginHooks(event string, data map[string]any) (map[string]any, error) {
	if ctx.pluginManager == nil {
		return data, nil
	}
	return ctx.pluginManager.RunBeforeHooks(event, data)
}

// RunAfterPluginHooks executes after-hooks for the given event.
// Errors are logged and ignored; execution is synchronous.
// If no plugin manager is active, this is a no-op.
func (ctx *MahresourcesContext) RunAfterPluginHooks(event string, data map[string]any) {
	if ctx.pluginManager == nil {
		return
	}
	ctx.pluginManager.RunAfterHooks(event, data)
}

// DownloadManager returns the download queue manager for background remote downloads
func (ctx *MahresourcesContext) DownloadManager() *download_queue.DownloadManager {
	return ctx.downloadManager
}

// GetDefaultFs returns the main filesystem (rooted at FileSavePath via
// BasePathFs in disk mode, or an in-memory fs in memory mode). Used by
// handlers that need to read/write files alongside the main resource store.
func (ctx *MahresourcesContext) GetDefaultFs() afero.Fs {
	return ctx.fs
}

// WithRequest returns a shallow copy of the context with the HTTP request set.
// This enables log entries to capture request metadata (path, IP, user agent).
// Use this in HTTP handlers to enable request-aware logging:
//
//	ctx.WithRequest(r).CreateTag(&creator)
//
// The returned value implements all the same interfaces as the original context.
// Implements interfaces.RequestContextSetter.
func (ctx *MahresourcesContext) WithRequest(r *http.Request) any {
	// Create a shallow copy to avoid modifying the original
	ctxCopy := *ctx
	ctxCopy.currentRequest = r
	return &ctxCopy
}

// SetHashQueue sets the channel for queueing resources for hash processing.
func (ctx *MahresourcesContext) SetHashQueue(queue chan<- uint) {
	ctx.hashQueue = queue
}

// QueueForHashing queues a resource ID for async hash processing.
// Returns true if queued, false if queue is nil or full.
func (ctx *MahresourcesContext) QueueForHashing(resourceID uint) bool {
	if ctx.hashQueue == nil {
		return false
	}
	select {
	case ctx.hashQueue <- resourceID:
		return true
	default:
		return false
	}
}

// SetThumbnailQueue sets the channel for queueing resources for thumbnail generation.
func (ctx *MahresourcesContext) SetThumbnailQueue(queue chan<- uint) {
	ctx.thumbnailQueue = queue
}

// QueueForThumbnailing queues a resource ID for async thumbnail generation.
// Returns true if queued, false if queue is nil or full.
func (ctx *MahresourcesContext) QueueForThumbnailing(resourceID uint) bool {
	if ctx.thumbnailQueue == nil {
		return false
	}
	select {
	case ctx.thumbnailQueue <- resourceID:
		return true
	default:
		return false
	}
}

// OnResourceFileChanged handles cleanup when a resource's file content changes.
// This deletes the old hash (cascade removes similarity pairs) and re-queues for hashing.
func (ctx *MahresourcesContext) OnResourceFileChanged(resourceID uint) {
	// Delete old hash - cascade will remove associated similarity pairs
	ctx.db.Where("resource_id = ?", resourceID).Delete(&models.ImageHash{})
	// Re-queue for hashing
	ctx.QueueForHashing(resourceID)
}

// EnsureForeignKeysActive ensures that sqlite connection somehow didn't manage to deactivate foreign keys
// I really don't know why this happens, so @todo please remove this if you can fix the root issue
func (ctx *MahresourcesContext) EnsureForeignKeysActive(db *gorm.DB) {
	if ctx.Config.DbType != constants.DbTypeSqlite {
		return
	}

	query := "PRAGMA foreign_keys = ON;"

	if db == nil {
		if err := ctx.db.Exec(query).Error; err != nil {
			log.Printf("warning: failed to enable foreign keys: %v", err)
		}
		return
	}

	if err := db.Exec(query).Error; err != nil {
		log.Printf("warning: failed to enable foreign keys: %v", err)
	}
}

// IsReadOnlyDBEnforced returns true if the read-only database connection
// has database-level read-only enforcement (e.g., SQLite mode=ro or separate DSN).
func (ctx *MahresourcesContext) IsReadOnlyDBEnforced() bool {
	if ctx.readOnlyDB == nil {
		return false
	}
	dsn := ctx.Config.DbReadOnlyDsn
	if strings.Contains(dsn, "mode=ro") {
		return true
	}
	if ctx.Config.DbType == constants.DbTypePosgres && dsn != "" && dsn != ctx.Config.DbDsn {
		return true
	}
	return false
}

func (ctx *MahresourcesContext) WithTransaction(txFn func(transactionCtx *MahresourcesContext) error) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		// Create a shallow copy that shares the parent's locks, caches, and alt filesystems
		// but uses the transactional *gorm.DB
		txCtx := *ctx
		txCtx.db = tx
		return txFn(&txCtx)
	})
}

func parseHTMLTime(timeStr string) *time.Time {
	return timeOrNil(time.Parse(constants.TimeFormat, timeStr))
}

func timeOrNil(time time.Time, err error) *time.Time {
	if err != nil {
		log.Printf("couldn't parse date: %v", err)

		return nil
	}

	return &time
}

// ValidateMeta checks that the given string is valid JSON and that
// the top-level value is a JSON object (i.e. starts with '{').
// GORM's JSONB scanner and SQLite's json_each both expect objects;
// storing scalars or arrays causes 500 errors on list pages.
func ValidateMeta(meta string) error {
	meta = strings.TrimSpace(meta)
	if meta == "" {
		return nil
	}
	if !json.Valid([]byte(meta)) {
		return fmt.Errorf("invalid JSON in meta field")
	}
	if meta[0] != '{' {
		return fmt.Errorf("meta must be a JSON object, got %c", meta[0])
	}
	// Reject empty or whitespace-only keys
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(meta), &parsed); err != nil {
		return fmt.Errorf("invalid JSON in meta field: %w", err)
	}
	for key := range parsed {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("meta object keys must not be empty or whitespace-only")
		}
	}
	return nil
}

func pageLimit(db *gorm.DB) *gorm.DB {
	return db.Limit(constants.MaxResultsPerPage)
}

func pageLimitCustom(maxResults int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(maxResults)
	}
}

func metaKeys(ctx *MahresourcesContext, table string) ([]interfaces.MetaKey, error) {
	var results []interfaces.MetaKey

	if ctx.Config.DbType == constants.DbTypePosgres {
		if err := ctx.db.
			Table(table).
			Select("DISTINCT jsonb_object_keys(Meta) as Key").
			Where("Meta IS NOT NULL").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else if ctx.Config.DbType == constants.DbTypeSqlite {
		if err := ctx.db.
			Table(fmt.Sprintf("%v, json_each(%v.meta)", table, table)).
			Select("DISTINCT json_each.key as Key").
			Where("Meta IS NOT NULL").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else {
		results = make([]interfaces.MetaKey, 0)
	}

	return results, nil
}

// copySeedDatabase copies a SQLite database file to the destination path
func copySeedDatabase(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open seed database %s: %w", src, err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination database %s: %w", dst, err)
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy database: %w", err)
	}

	return dstFile.Sync()
}

// CreateContextWithConfig creates a context using the provided configuration.
// This is the preferred way to create a context when using command-line flags.
func CreateContextWithConfig(cfg *MahresourcesInputConfig) (*MahresourcesContext, *gorm.DB, afero.Fs) {
	var db *gorm.DB
	var mainFs afero.Fs

	// Determine effective database settings
	dbType := cfg.DbType
	dbDsn := cfg.DbDsn
	readOnlyDsn := cfg.DbReadOnlyDsn

	// Validate seed-db usage
	if cfg.SeedDB != "" {
		if !cfg.MemoryDB {
			log.Fatal("-seed-db requires -memory-db or -ephemeral flag")
		}
		if strings.ToUpper(cfg.DbType) == "POSTGRES" {
			log.Fatal("-seed-db is only supported with SQLite, not Postgres")
		}
		// Check seed-db file exists
		if info, err := os.Stat(cfg.SeedDB); err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("-seed-db file does not exist: %s", cfg.SeedDB)
			}
			log.Fatalf("-seed-db file error: %v", err)
		} else if info.IsDir() {
			log.Fatalf("-seed-db path is a directory, not a file: %s", cfg.SeedDB)
		}
	}

	if cfg.MemoryDB {
		dbType = "SQLITE"
		// Use a per-process temp file with WAL mode for better concurrent write handling.
		// Including the PID ensures multiple ephemeral instances don't share the same file.
		ephemeralPath := fmt.Sprintf("/tmp/mahresources_ephemeral_%d.db", os.Getpid())
		dbDsn = fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL", ephemeralPath)
		readOnlyDsn = fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=10000&mode=ro", ephemeralPath)

		// Remove any existing temp database files for this PID
		os.Remove(ephemeralPath)
		os.Remove(ephemeralPath + "-wal")
		os.Remove(ephemeralPath + "-shm")

		if cfg.SeedDB != "" {
			// Copy seed database to temp location
			if err := copySeedDatabase(cfg.SeedDB, ephemeralPath); err != nil {
				log.Fatalf("Failed to copy seed database: %v", err)
			}
			log.Printf("Using ephemeral SQLite database seeded from %s", cfg.SeedDB)
		} else {
			log.Println("Using ephemeral SQLite database with WAL mode")
		}
	}

	// Validate seed-fs usage: needs either memory-fs or file-save-path for the overlay
	if cfg.SeedFS != "" && !cfg.MemoryFS && cfg.FileSavePath == "" {
		log.Fatal("-seed-fs requires either -memory-fs or -file-save-path for the writable overlay")
	}

	// Validate seed-fs directory exists
	if cfg.SeedFS != "" {
		if info, err := os.Stat(cfg.SeedFS); err != nil {
			if os.IsNotExist(err) {
				log.Fatalf("-seed-fs directory does not exist: %s", cfg.SeedFS)
			}
			log.Fatalf("-seed-fs directory error: %v", err)
		} else if !info.IsDir() {
			log.Fatalf("-seed-fs path is not a directory: %s", cfg.SeedFS)
		}
	}

	// Determine effective filesystem
	if cfg.SeedFS != "" {
		// Copy-on-write mode: seed directory is read-only base, overlay handles writes
		var overlay afero.Fs
		if cfg.MemoryFS {
			overlay = storage.CreateMemoryStorage()
			log.Printf("Using copy-on-write filesystem seeded from %s (memory overlay)", cfg.SeedFS)
		} else {
			overlay = storage.CreateStorage(cfg.FileSavePath)
			log.Printf("Using copy-on-write filesystem seeded from %s (disk overlay: %s)", cfg.SeedFS, cfg.FileSavePath)
		}
		mainFs = storage.CreateCopyOnWriteStorage(cfg.SeedFS, overlay)
	} else if cfg.MemoryFS {
		mainFs = storage.CreateMemoryStorage()
		log.Println("Using in-memory filesystem (ephemeral mode)")
	} else {
		if cfg.FileSavePath == "" {
			log.Fatal("File save path is empty (use -memory-fs for ephemeral mode)")
		}
		mainFs = storage.CreateStorage(cfg.FileSavePath)
	}

	fmt.Printf("DB_TYPE %v FILE_SAVE_PATH %v\n", dbType, cfg.FileSavePath)

	if connectedDB, err := models.CreateDatabaseConnection(dbType, dbDsn, cfg.DbLogFile); err != nil {
		log.Fatal(err)
	} else {
		db = connectedDB
	}

	// Apply connection pool limits if configured (useful for SQLite under test load)
	if cfg.MaxDBConnections > 0 {
		sqlDB, err := db.DB()
		if err != nil {
			log.Printf("Warning: failed to get underlying DB for connection pool config: %v", err)
		} else {
			sqlDB.SetMaxOpenConns(cfg.MaxDBConnections)
			sqlDB.SetMaxIdleConns(cfg.MaxDBConnections)
			log.Printf("Database connection pool limited to %d connections", cfg.MaxDBConnections)
		}
	}

	readOnlyDb, err := models.CreateReadOnlyDatabaseConnection(strings.ToLower(dbType), readOnlyDsn)

	if err != nil {
		log.Fatal(err.Error())
	}

	// Apply connection pool limits to read-only connection as well
	if cfg.MaxDBConnections > 0 {
		readOnlyDb.SetMaxOpenConns(cfg.MaxDBConnections)
		readOnlyDb.SetMaxIdleConns(cfg.MaxDBConnections)
	}

	// Apply default timeouts if not specified
	connectTimeout := cfg.RemoteResourceConnectTimeout
	if connectTimeout == 0 {
		connectTimeout = 30 * time.Second
	}
	idleTimeout := cfg.RemoteResourceIdleTimeout
	if idleTimeout == 0 {
		idleTimeout = 60 * time.Second
	}
	overallTimeout := cfg.RemoteResourceOverallTimeout
	if overallTimeout == 0 {
		overallTimeout = 30 * time.Minute
	}

	// Apply default video thumbnail timeouts if not specified
	videoThumbTimeout := cfg.VideoThumbnailTimeout
	if videoThumbTimeout == 0 {
		videoThumbTimeout = 30 * time.Second
	}
	videoThumbLockTimeout := cfg.VideoThumbnailLockTimeout
	if videoThumbLockTimeout == 0 {
		videoThumbLockTimeout = 60 * time.Second
	}

	return NewMahresourcesContext(mainFs, db, readOnlyDb, &MahresourcesConfig{
		DbType:                       dbType,
		DbDsn:                        dbDsn,
		DbReadOnlyDsn:                readOnlyDsn,
		AltFileSystems:               cfg.AltFileSystems,
		FfmpegPath:                   cfg.FfmpegPath,
		LibreOfficePath:              cfg.LibreOfficePath,
		BindAddress:                  cfg.BindAddress,
		SharePort:                    cfg.SharePort,
		ShareBindAddress:             cfg.ShareBindAddress,
		RemoteResourceConnectTimeout: connectTimeout,
		RemoteResourceIdleTimeout:    idleTimeout,
		RemoteResourceOverallTimeout: overallTimeout,
		VideoThumbnailTimeout:        videoThumbTimeout,
		VideoThumbnailLockTimeout:    videoThumbLockTimeout,
		VideoThumbnailConcurrency:    cfg.VideoThumbnailConcurrency,
		PluginPath:                   cfg.PluginPath,
		PluginsDisabled:              cfg.PluginsDisabled,
		HashWorkerEnabled:            cfg.HashWorkerEnabled,
		HashWorkerCount:              cfg.HashWorkerCount,
		HashBatchSize:                cfg.HashBatchSize,
		HashPollInterval:             cfg.HashPollInterval,
		HashSimilarityThreshold:      cfg.HashSimilarityThreshold,
		HashCacheSize:                cfg.HashCacheSize,
		EphemeralMode:                cfg.EphemeralMode,
		MemoryDB:                     cfg.MemoryDB,
		MemoryFS:                     cfg.MemoryFS,
		MaxDBConnections:             cfg.MaxDBConnections,
		FileSavePath:                 cfg.FileSavePath,
		SkipFTS:                      cfg.SkipFTS,
		MaxJobConcurrency:            cfg.MaxJobConcurrency,
		ExportRetention:              cfg.ExportRetention,
		MaxImportSize:                cfg.MaxImportSize,
	}), db, mainFs
}

// CreateContext creates a context using environment variables.
// Deprecated: Use CreateContextWithConfig for new code.
func CreateContext() (*MahresourcesContext, *gorm.DB, afero.Fs) {
	var numAlt int64 = 0

	if fileAltCount, err := strconv.ParseInt(os.Getenv("FILE_ALT_COUNT"), 10, 8); err == nil {
		numAlt = fileAltCount
	}

	altFSystems := make(map[string]string, numAlt)
	for i := int64(0); i < numAlt; i++ {
		altFSystems[os.Getenv(fmt.Sprintf("FILE_ALT_NAME_%v", i+1))] = os.Getenv(fmt.Sprintf("FILE_ALT_PATH_%v", i+1))
	}

	return CreateContextWithConfig(&MahresourcesInputConfig{
		FileSavePath:   os.Getenv("FILE_SAVE_PATH"),
		DbType:         os.Getenv("DB_TYPE"),
		DbDsn:          os.Getenv("DB_DSN"),
		DbReadOnlyDsn:  os.Getenv("DB_READONLY_DSN"),
		DbLogFile:      os.Getenv("DB_LOG_FILE"),
		BindAddress:    os.Getenv("BIND_ADDRESS"),
		FfmpegPath:     os.Getenv("FFMPEG_PATH"),
		AltFileSystems: altFSystems,
	})
}
