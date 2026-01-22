package application_context

import (
	"fmt"
	"io"
	"log"
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
	"mahresources/lib"
	"mahresources/models"
	"mahresources/server/interfaces"
	"mahresources/storage"
)

type MahresourcesConfig struct {
	DbType         string
	AltFileSystems map[string]string
	FfmpegPath     string
	BindAddress    string
	// RemoteResourceConnectTimeout is the timeout for connecting to remote URLs (dial, TLS, response headers)
	RemoteResourceConnectTimeout time.Duration
	// RemoteResourceIdleTimeout is how long to wait before erroring if a remote server stops sending data
	RemoteResourceIdleTimeout time.Duration
	// RemoteResourceOverallTimeout is the maximum total time for a remote resource download (default: 30m)
	RemoteResourceOverallTimeout time.Duration
}

// MahresourcesInputConfig holds all configuration options that can be passed
// via command-line flags or environment variables
type MahresourcesInputConfig struct {
	FileSavePath   string
	DbType         string
	DbDsn          string
	DbReadOnlyDsn  string
	DbLogFile      string
	BindAddress    string
	FfmpegPath     string
	AltFileSystems map[string]string
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
}

type MahresourcesLocks struct {
	ThumbnailGenerationLock      *lib.IDLock[uint]
	VideoThumbnailGenerationLock *lib.IDLock[uint]
	ResourceHashLock             *lib.IDLock[string]
}

type MahresourcesContext struct {
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
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, readOnlyDB *sqlx.DB, config *MahresourcesConfig) *MahresourcesContext {
	altFileSystems := make(map[string]afero.Fs, len(config.AltFileSystems))

	for key, path := range config.AltFileSystems {
		altFileSystems[key] = storage.CreateStorage(path)
	}

	thumbnailGenerationLock := lib.NewIDLock[uint](uint(0), nil)
	videoThumbnailGenerationLock := lib.NewIDLock[uint](uint(1), nil)
	resourceHashLock := lib.NewIDLock[string](uint(0), nil)

	// Initialize search cache with 60 second TTL and 1000 max entries
	searchCache := NewSearchCache(60*time.Second, 1000)

	ctx := &MahresourcesContext{
		fs:             filesystem,
		db:             db,
		readOnlyDB:     readOnlyDB,
		Config:         config,
		altFileSystems: altFileSystems,
		locks: MahresourcesLocks{
			ThumbnailGenerationLock:      thumbnailGenerationLock,
			VideoThumbnailGenerationLock: videoThumbnailGenerationLock,
			ResourceHashLock:             resourceHashLock,
		},
		searchCache: searchCache,
	}

	// Initialize download manager with timeout config
	ctx.downloadManager = download_queue.NewDownloadManager(ctx, download_queue.TimeoutConfig{
		ConnectTimeout: config.RemoteResourceConnectTimeout,
		IdleTimeout:    config.RemoteResourceIdleTimeout,
		OverallTimeout: config.RemoteResourceOverallTimeout,
	})

	return ctx
}

// DownloadManager returns the download queue manager for background remote downloads
func (ctx *MahresourcesContext) DownloadManager() *download_queue.DownloadManager {
	return ctx.downloadManager
}

// EnsureForeignKeysActive ensures that sqlite connection somehow didn't manage to deactivate foreign keys
// I really don't know why this happens, so @todo please remove this if you can fix the root issue
func (ctx *MahresourcesContext) EnsureForeignKeysActive(db *gorm.DB) {
	if ctx.Config.DbType != constants.DbTypeSqlite {
		return
	}

	query := "PRAGMA foreign_keys = ON;"

	if db == nil {
		ctx.db.Exec(query)
	}

	db.Exec(query)
}

func (ctx *MahresourcesContext) WithTransaction(txFn func(transactionCtx *MahresourcesContext) error) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		altContext := NewMahresourcesContext(ctx.fs, tx, ctx.readOnlyDB, ctx.Config)
		return txFn(altContext)
	})
}

func parseHTMLTime(timeStr string) *time.Time {
	return timeOrNil(time.Parse(constants.TimeFormat, timeStr))
}

func timeOrNil(time time.Time, err error) *time.Time {
	if err != nil {
		fmt.Println("couldn't parse date", err.Error())

		return nil
	}

	return &time
}

func pageLimit(db *gorm.DB) *gorm.DB {
	return db.Limit(constants.MaxResultsPerPage)
}

func pageLimitCustom(maxResults int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(maxResults)
	}
}

func metaKeys(ctx *MahresourcesContext, table string) (*[]interfaces.MetaKey, error) {
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

	return &results, nil
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
		// Use a temp file with WAL mode for better concurrent write handling
		// The file is auto-deleted when all connections close
		dbDsn = "file:/tmp/mahresources_ephemeral.db?_journal_mode=WAL&_busy_timeout=10000&_synchronous=NORMAL"
		readOnlyDsn = "file:/tmp/mahresources_ephemeral.db?_journal_mode=WAL&_busy_timeout=10000&mode=ro"

		// Remove any existing temp database files
		os.Remove("/tmp/mahresources_ephemeral.db")
		os.Remove("/tmp/mahresources_ephemeral.db-wal")
		os.Remove("/tmp/mahresources_ephemeral.db-shm")

		if cfg.SeedDB != "" {
			// Copy seed database to temp location
			if err := copySeedDatabase(cfg.SeedDB, "/tmp/mahresources_ephemeral.db"); err != nil {
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

	fmt.Printf("DB_TYPE %v DB_DSN %v FILE_SAVE_PATH %v\n", dbType, dbDsn, cfg.FileSavePath)

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

	return NewMahresourcesContext(mainFs, db, readOnlyDb, &MahresourcesConfig{
		DbType:                       dbType,
		AltFileSystems:               cfg.AltFileSystems,
		FfmpegPath:                   cfg.FfmpegPath,
		BindAddress:                  cfg.BindAddress,
		RemoteResourceConnectTimeout: connectTimeout,
		RemoteResourceIdleTimeout:    idleTimeout,
		RemoteResourceOverallTimeout: overallTimeout,
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
