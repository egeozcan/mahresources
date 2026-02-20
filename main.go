package main

import (
	gocontext "context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/hash_worker"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
	"mahresources/storage"
	"mahresources/thumbnail_worker"
)

// altFS is a custom flag type that collects multiple -alt-fs flags
type altFS []string

func (a *altFS) String() string {
	return strings.Join(*a, ", ")
}

func (a *altFS) Set(value string) error {
	*a = append(*a, value)
	return nil
}

// parseDurationEnv parses a duration from an environment variable, returning the default if not set or invalid
func parseDurationEnv(envVar string, defaultVal time.Duration) time.Duration {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(val)
	if err != nil {
		log.Printf("Warning: invalid duration for %s=%q, using default %v", envVar, val, defaultVal)
		return defaultVal
	}
	return d
}

// parseIntEnv parses an int from an environment variable, returning the default if not set or invalid
func parseIntEnv(envVar string, defaultVal int) int {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}
	var i int
	if _, err := fmt.Sscanf(val, "%d", &i); err != nil {
		log.Printf("Warning: invalid integer for %s=%q, using default %d", envVar, val, defaultVal)
		return defaultVal
	}
	return i
}

// getEnvOrDefault returns the value of the environment variable or a default if not set
func getEnvOrDefault(envVar string, defaultVal string) string {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}
	return val
}

func main() {
	// Load .env first so environment variables are available as defaults
	// you may have no .env, it's okay
	_ = godotenv.Load(".env")

	// Define flags with environment variables as defaults
	fileSavePath := flag.String("file-save-path", os.Getenv("FILE_SAVE_PATH"), "Main file storage directory (env: FILE_SAVE_PATH)")
	dbType := flag.String("db-type", os.Getenv("DB_TYPE"), "Database type: SQLITE or POSTGRES (env: DB_TYPE)")
	dbDsn := flag.String("db-dsn", os.Getenv("DB_DSN"), "Database connection string (env: DB_DSN)")
	dbReadOnlyDsn := flag.String("db-readonly-dsn", os.Getenv("DB_READONLY_DSN"), "Read-only database connection string (env: DB_READONLY_DSN)")
	dbLogFile := flag.String("db-log-file", os.Getenv("DB_LOG_FILE"), "DB log destination: STDOUT, empty, or file path (env: DB_LOG_FILE)")
	bindAddress := flag.String("bind-address", os.Getenv("BIND_ADDRESS"), "Server bind address:port (env: BIND_ADDRESS)")
	ffmpegPath := flag.String("ffmpeg-path", os.Getenv("FFMPEG_PATH"), "Path to ffmpeg binary for video thumbnails (env: FFMPEG_PATH)")
	libreOfficePath := flag.String("libreoffice-path", os.Getenv("LIBREOFFICE_PATH"), "Path to LibreOffice binary for office document thumbnails (env: LIBREOFFICE_PATH)")
	skipFTS := flag.Bool("skip-fts", os.Getenv("SKIP_FTS") == "1", "Skip Full-Text Search initialization (env: SKIP_FTS=1)")
	skipVersionMigration := flag.Bool("skip-version-migration", os.Getenv("SKIP_VERSION_MIGRATION") == "1", "Skip resource version migration at startup (env: SKIP_VERSION_MIGRATION=1)")

	// Ephemeral/in-memory options
	memoryDB := flag.Bool("memory-db", os.Getenv("MEMORY_DB") == "1", "Use in-memory SQLite database (env: MEMORY_DB=1)")
	memoryFS := flag.Bool("memory-fs", os.Getenv("MEMORY_FS") == "1", "Use in-memory filesystem (env: MEMORY_FS=1)")
	ephemeral := flag.Bool("ephemeral", os.Getenv("EPHEMERAL") == "1", "Run in fully ephemeral mode (memory DB + memory FS) (env: EPHEMERAL=1)")
	seedDB := flag.String("seed-db", os.Getenv("SEED_DB"), "Path to SQLite file to use as basis for memory-db (env: SEED_DB)")
	seedFS := flag.String("seed-fs", os.Getenv("SEED_FS"), "Path to directory to use as read-only base for memory-fs (env: SEED_FS)")
	maxDBConnections := flag.Int("max-db-connections", parseIntEnv("MAX_DB_CONNECTIONS", 0), "Limit database connection pool size, useful for SQLite under test load (env: MAX_DB_CONNECTIONS)")
	cleanupLogsDays := flag.Int("cleanup-logs-days", parseIntEnv("CLEANUP_LOGS_DAYS", 0), "Delete log entries older than N days on startup (0=disabled) (env: CLEANUP_LOGS_DAYS)")

	// Hash worker options
	hashWorkerCount := flag.Int("hash-worker-count", parseIntEnv("HASH_WORKER_COUNT", 4), "Number of concurrent hash calculation workers (env: HASH_WORKER_COUNT)")
	hashBatchSize := flag.Int("hash-batch-size", parseIntEnv("HASH_BATCH_SIZE", 500), "Resources to process per batch cycle (env: HASH_BATCH_SIZE)")
	hashPollInterval := flag.Duration("hash-poll-interval", parseDurationEnv("HASH_POLL_INTERVAL", time.Minute), "Time between batch processing cycles (env: HASH_POLL_INTERVAL)")
	hashSimilarityThreshold := flag.Int("hash-similarity-threshold", parseIntEnv("HASH_SIMILARITY_THRESHOLD", 10), "Maximum Hamming distance for similarity (env: HASH_SIMILARITY_THRESHOLD)")
	hashWorkerDisabled := flag.Bool("hash-worker-disabled", os.Getenv("HASH_WORKER_DISABLED") == "1", "Disable hash worker (env: HASH_WORKER_DISABLED=1)")
	hashCacheSize := flag.Int("hash-cache-size", parseIntEnv("HASH_CACHE_SIZE", 100000), "Maximum entries in the hash similarity cache (env: HASH_CACHE_SIZE)")

	// Video thumbnail options
	videoThumbTimeout := flag.Duration("video-thumb-timeout", parseDurationEnv("VIDEO_THUMB_TIMEOUT", 30*time.Second), "Timeout for video thumbnail ffmpeg invocation (env: VIDEO_THUMB_TIMEOUT)")
	videoThumbLockTimeout := flag.Duration("video-thumb-lock-timeout", parseDurationEnv("VIDEO_THUMB_LOCK_TIMEOUT", 60*time.Second), "Timeout waiting for video thumbnail lock (env: VIDEO_THUMB_LOCK_TIMEOUT)")
	videoThumbConcurrency := flag.Int("video-thumb-concurrency", parseIntEnv("VIDEO_THUMB_CONCURRENCY", 4), "Max concurrent video thumbnail generations (env: VIDEO_THUMB_CONCURRENCY)")

	// Thumbnail worker options
	thumbWorkerCount := flag.Int("thumb-worker-count", parseIntEnv("THUMB_WORKER_COUNT", 2), "Number of concurrent thumbnail generation workers (env: THUMB_WORKER_COUNT)")
	thumbWorkerDisabled := flag.Bool("thumb-worker-disabled", os.Getenv("THUMB_WORKER_DISABLED") == "1", "Disable thumbnail worker (env: THUMB_WORKER_DISABLED=1)")
	thumbBatchSize := flag.Int("thumb-batch-size", parseIntEnv("THUMB_BATCH_SIZE", 10), "Videos to process per backfill cycle (env: THUMB_BATCH_SIZE)")
	thumbPollInterval := flag.Duration("thumb-poll-interval", parseDurationEnv("THUMB_POLL_INTERVAL", time.Minute), "Time between backfill processing cycles (env: THUMB_POLL_INTERVAL)")
	thumbBackfill := flag.Bool("thumb-backfill", os.Getenv("THUMB_BACKFILL") == "1", "Enable backfilling thumbnails for existing videos (env: THUMB_BACKFILL=1)")

	// Alternative file systems: can be specified multiple times as -alt-fs=key:path
	var altFSFlags altFS
	flag.Var(&altFSFlags, "alt-fs", "Alternative file system in format key:path (can be specified multiple times)")

	// Remote resource timeout options
	remoteConnectTimeout := flag.Duration("remote-connect-timeout", parseDurationEnv("REMOTE_CONNECT_TIMEOUT", 30*time.Second), "Timeout for connecting to remote URLs (env: REMOTE_CONNECT_TIMEOUT)")
	remoteIdleTimeout := flag.Duration("remote-idle-timeout", parseDurationEnv("REMOTE_IDLE_TIMEOUT", 60*time.Second), "Timeout for idle remote transfers (env: REMOTE_IDLE_TIMEOUT)")
	remoteOverallTimeout := flag.Duration("remote-overall-timeout", parseDurationEnv("REMOTE_OVERALL_TIMEOUT", 30*time.Minute), "Maximum total time for remote downloads (env: REMOTE_OVERALL_TIMEOUT)")

	// Share server options
	sharePort := flag.String("share-port", os.Getenv("SHARE_PORT"), "Port for public share server (env: SHARE_PORT)")
	shareBindAddress := flag.String("share-bind-address", getEnvOrDefault("SHARE_BIND_ADDRESS", "0.0.0.0"), "Bind address for share server (env: SHARE_BIND_ADDRESS)")

	flag.Parse()

	// Build alt file systems map from flags or fall back to env vars
	altFileSystems := make(map[string]string)
	if len(altFSFlags) > 0 {
		// Use command-line flags
		for _, fs := range altFSFlags {
			parts := strings.SplitN(fs, ":", 2)
			if len(parts) == 2 {
				altFileSystems[parts[0]] = parts[1]
			} else {
				log.Fatalf("Invalid -alt-fs format: %s (expected key:path)", fs)
			}
		}
	} else {
		// Fall back to environment variables
		fileAltCountStr := os.Getenv("FILE_ALT_COUNT")
		if fileAltCountStr != "" {
			var numAlt int
			if _, err := fmt.Sscanf(fileAltCountStr, "%d", &numAlt); err == nil {
				for i := 1; i <= numAlt; i++ {
					name := os.Getenv(fmt.Sprintf("FILE_ALT_NAME_%d", i))
					path := os.Getenv(fmt.Sprintf("FILE_ALT_PATH_%d", i))
					if name != "" && path != "" {
						altFileSystems[name] = path
					}
				}
			}
		}
	}

	// Handle ephemeral flag (sets both memory-db and memory-fs)
	useMemoryDB := *memoryDB || *ephemeral
	useMemoryFS := *memoryFS || *ephemeral

	// Create configuration
	cfg := &application_context.MahresourcesInputConfig{
		FileSavePath:                 *fileSavePath,
		DbType:                       *dbType,
		DbDsn:                        *dbDsn,
		DbReadOnlyDsn:                *dbReadOnlyDsn,
		DbLogFile:                    *dbLogFile,
		BindAddress:                  *bindAddress,
		SharePort:                    *sharePort,
		ShareBindAddress:             *shareBindAddress,
		FfmpegPath:                   *ffmpegPath,
		LibreOfficePath:              *libreOfficePath,
		AltFileSystems:               altFileSystems,
		MemoryDB:                     useMemoryDB,
		MemoryFS:                     useMemoryFS,
		SeedDB:                       *seedDB,
		SeedFS:                       *seedFS,
		RemoteResourceConnectTimeout: *remoteConnectTimeout,
		RemoteResourceIdleTimeout:    *remoteIdleTimeout,
		RemoteResourceOverallTimeout: *remoteOverallTimeout,
		MaxDBConnections:             *maxDBConnections,
		VideoThumbnailTimeout:        *videoThumbTimeout,
		VideoThumbnailLockTimeout:    *videoThumbLockTimeout,
		VideoThumbnailConcurrency:    uint(*videoThumbConcurrency),
	}

	context, db, mainFs := application_context.CreateContextWithConfig(cfg)

	// Validate or auto-detect ffmpeg
	if context.Config.FfmpegPath != "" {
		if _, err := exec.LookPath(context.Config.FfmpegPath); err != nil {
			log.Printf("Warning: configured ffmpeg path %q not found, video thumbnails will be unavailable", context.Config.FfmpegPath)
		}
	} else {
		if path, err := exec.LookPath("ffmpeg"); err == nil {
			context.Config.FfmpegPath = path
			log.Printf("Auto-detected ffmpeg at %s", path)
		} else {
			log.Println("Warning: ffmpeg not found in PATH, video thumbnails will be unavailable")
		}
	}

	// Disable foreign keys during AutoMigrate for SQLite.
	// SQLite can't ALTER TABLE to add constraints, so GORM recreates the table
	// (create temp, copy, drop original, rename). The DROP fails if other tables
	// reference it with FKs enabled.
	if context.Config.DbType == constants.DbTypeSqlite {
		db.Exec("PRAGMA foreign_keys = OFF")
	}

	if err := db.AutoMigrate(
		&models.Query{},
		&models.Series{},
		&models.Resource{},
		&models.ResourceVersion{},
		&models.Note{},
		&models.NoteBlock{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
		&models.ImageHash{},
		&models.ResourceSimilarity{},
		&models.LogEntry{},
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	if context.Config.DbType == constants.DbTypeSqlite {
		db.Exec("PRAGMA foreign_keys = ON")

		// Log any FK violations. These are typically pre-existing (SQLite doesn't
		// enforce FKs by default), not caused by AutoMigrate.
		var fkViolations []struct {
			Table  string
			Rowid  int64
			Parent string
			Fkid   int64
		}
		if result := db.Raw("PRAGMA foreign_key_check").Scan(&fkViolations); result.Error == nil && len(fkViolations) > 0 {
			log.Printf("Warning: %d foreign key violation(s) found in database:", len(fkViolations))
			for _, v := range fkViolations {
				log.Printf("  table=%q rowid=%d parent=%q fkid=%d", v.Table, v.Rowid, v.Parent, v.Fkid)
			}
		}
	}

	util.AddInitialData(db)

	indexQueries := [...]string{
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__note_id ON resource_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__resource_id ON resource_notes(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id ON groups_related_resources(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources USING HASH (resource_id);",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__group_id ON groups_related_resources(group_id)",
		"CREATE INDEX IF NOT EXISTS idx__log_entries__entity_type_entity_id ON log_entries(entity_type, entity_id)",
	}

	indexQueriesSqlite := [...]string{
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__note_id ON resource_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__resource_id ON resource_notes(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id ON groups_related_resources(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources(resource_id);",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__group_id ON groups_related_resources(group_id)",
		"CREATE INDEX IF NOT EXISTS idx__log_entries__entity_type_entity_id ON log_entries(entity_type, entity_id)",
	}

	if context.Config.DbType == constants.DbTypePosgres {
		for _, query := range indexQueries {
			if err := db.Exec(query).Error; err != nil {
				log.Fatalf("Error when creating index: %v", err)
			}
		}
	} else {
		for _, query := range indexQueriesSqlite {
			if err := db.Exec(query).Error; err != nil {
				log.Fatalf("Error when creating index: %v", err)
			}
		}
	}

	// Migrate existing resources to versioning system in background (skip with -skip-version-migration flag)
	if !*skipVersionMigration {
		go func() {
			if err := context.MigrateResourceVersions(); err != nil {
				log.Printf("Warning: failed to migrate resource versions: %v", err)
			}

			// Sync resource fields from their current versions (fixes resources
			// where versions were uploaded before the sync fix was deployed)
			if err := context.SyncResourcesFromCurrentVersion(); err != nil {
				log.Printf("Warning: failed to sync resources from versions: %v", err)
			}
		}()
	} else {
		log.Println("Version migration skipped (-skip-version-migration flag or SKIP_VERSION_MIGRATION=1)")
	}

	// Initialize Full-Text Search (skip with -skip-fts flag or SKIP_FTS=1 env var)
	if !*skipFTS {
		if err := context.InitFTS(); err != nil {
			log.Printf("Warning: FTS setup failed, falling back to LIKE-based search: %v", err)
		}
	} else {
		log.Println("FTS setup skipped (-skip-fts flag or SKIP_FTS=1)")
	}

	// Cleanup old logs if configured
	if *cleanupLogsDays > 0 {
		deleted, err := context.CleanupOldLogs(*cleanupLogsDays)
		if err != nil {
			log.Printf("Warning: failed to cleanup old logs: %v", err)
		} else if deleted > 0 {
			log.Printf("Cleaned up %d log entries older than %d days", deleted, *cleanupLogsDays)
		}
	}

	// Start hash worker for background perceptual hash calculation
	hashWorkerConfig := hash_worker.Config{
		WorkerCount:         *hashWorkerCount,
		BatchSize:           *hashBatchSize,
		PollInterval:        *hashPollInterval,
		SimilarityThreshold: *hashSimilarityThreshold,
		Disabled:            *hashWorkerDisabled,
		CacheSize:           *hashCacheSize,
	}

	// Build alt filesystems map for hash worker
	altFsMap := make(map[string]afero.Fs)
	for name, path := range context.Config.AltFileSystems {
		altFsMap[name] = storage.CreateStorage(path)
	}

	hw := hash_worker.New(db, mainFs, altFsMap, hashWorkerConfig, context.Logger())
	hw.Start()
	context.SetHashQueue(hw.GetQueue())
	defer hw.Stop()

	// Start thumbnail worker for background video thumbnail pre-generation
	thumbWorkerConfig := thumbnail_worker.Config{
		WorkerCount:  *thumbWorkerCount,
		BatchSize:    *thumbBatchSize,
		PollInterval: *thumbPollInterval,
		Disabled:     *thumbWorkerDisabled,
		Backfill:     *thumbBackfill,
	}

	tw := thumbnail_worker.New(db, context, thumbWorkerConfig)
	tw.Start()
	context.SetThumbnailQueue(tw.GetQueue())
	defer tw.Stop()

	// Start share server if configured
	if cfg.SharePort != "" {
		shareServer := server.NewShareServer(context)
		if err := shareServer.Start(cfg.ShareBindAddress, cfg.SharePort); err != nil {
			log.Fatalf("Failed to start share server: %v", err)
		}
		defer shareServer.Stop()
		log.Printf("Share server available at http://%s:%s", cfg.ShareBindAddress, cfg.SharePort)
	}

	srv := server.CreateServer(context, mainFs, context.Config.AltFileSystems)

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	shutdownCtx, shutdownCancel := gocontext.WithTimeout(gocontext.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited cleanly")
}
