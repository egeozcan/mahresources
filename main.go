package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
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

	// Alternative file systems: can be specified multiple times as -alt-fs=key:path
	var altFSFlags altFS
	flag.Var(&altFSFlags, "alt-fs", "Alternative file system in format key:path (can be specified multiple times)")

	// Remote resource timeout options
	remoteConnectTimeout := flag.Duration("remote-connect-timeout", parseDurationEnv("REMOTE_CONNECT_TIMEOUT", 30*time.Second), "Timeout for connecting to remote URLs (env: REMOTE_CONNECT_TIMEOUT)")
	remoteIdleTimeout := flag.Duration("remote-idle-timeout", parseDurationEnv("REMOTE_IDLE_TIMEOUT", 60*time.Second), "Timeout for idle remote transfers (env: REMOTE_IDLE_TIMEOUT)")
	remoteOverallTimeout := flag.Duration("remote-overall-timeout", parseDurationEnv("REMOTE_OVERALL_TIMEOUT", 30*time.Minute), "Maximum total time for remote downloads (env: REMOTE_OVERALL_TIMEOUT)")

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
	}

	context, db, mainFs := application_context.CreateContextWithConfig(cfg)

	if err := db.AutoMigrate(
		&models.Query{},
		&models.Resource{},
		&models.ResourceVersion{},
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
	); err != nil {
		log.Fatalf("failed to migrate: %v", err)
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
	}

	// Build alt filesystems map for hash worker
	altFsMap := make(map[string]afero.Fs)
	for name, path := range context.Config.AltFileSystems {
		altFsMap[name] = storage.CreateStorage(path)
	}

	hw := hash_worker.New(db, mainFs, altFsMap, hashWorkerConfig)
	hw.Start()
	defer hw.Stop()

	log.Fatal(server.CreateServer(context, mainFs, context.Config.AltFileSystems).ListenAndServe())
}
