package main

import (
	gocontext "context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/hash_worker"
	"mahresources/hostfetch"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/seed"
	"mahresources/plugin_system"
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

// splitCommaList splits a comma-separated flag value into trimmed, non-empty
// entries. An unset flag yields nil, which every consumer reads as "none".
func splitCommaList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if entry := strings.TrimSpace(part); entry != "" {
			out = append(out, entry)
		}
	}
	return out
}

func pluginCommandPathValue(flagValue string, flagSet bool, envValue string, envSet bool, inherited string) (string, bool) {
	if flagSet {
		return flagValue, true
	}
	if envSet {
		return envValue, true
	}
	return inherited, false
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

// parseInt64Env parses an int64 from an environment variable, returning the default if not set or invalid
func parseInt64Env(envVar string, defaultVal int64) int64 {
	s := os.Getenv(envVar)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

// parseUint64Env parses a uint64 from an environment variable, returning the default if not set or invalid
func parseUint64Env(envVar string, defaultVal uint64) uint64 {
	s := os.Getenv(envVar)
	if s == "" {
		return defaultVal
	}
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return defaultVal
	}
	return v
}

// getEnvOrDefault returns the value of the environment variable or a default if not set
func getEnvOrDefault(envVar string, defaultVal string) string {
	val := os.Getenv(envVar)
	if val == "" {
		return defaultVal
	}
	return val
}

// jobReplayKeyConfig turns the boot configuration into the replay keyring
// configuration the Job control plane is loaded with.
//
// Every input is a boot fact, and the key is env-only (never a flag, never a
// runtime setting): a key is not a tuning knob, and a key that could be changed
// from /admin/settings is a key an administrator could use to render every
// stored envelope unreadable.
//
// Ephemeral is the in-memory database, which is the one deployment whose Jobs
// die with the process and may therefore generate a key it throws away.
// KeyFilePath is only consulted for a persistent SQLite deployment: PostgreSQL
// may have several processes and hosts writing one database, so a file beside
// one of them is not shared state.
func jobReplayKeyConfig(fileSavePath, dbType string, ephemeral bool) jobs.ReplayKeyConfig {
	keyPath := ""
	if strings.TrimSpace(fileSavePath) != "" {
		keyPath = filepath.Join(fileSavePath, jobs.JobReplayKeyFileName)
	}
	return jobs.ReplayKeyConfig{
		Keys:        os.Getenv("JOB_REPLAY_KEY"),
		Dialect:     dbType,
		Ephemeral:   ephemeral,
		KeyFilePath: keyPath,
	}
}

func main() {
	// A non-zero exit is set and returned rather than os.Exit'd from inside, so the
	// deferred shutdowns below — the download manager's in particular, which is
	// what writes the record of the downloads a restart interrupts — still run.
	exitCode := 0
	defer func() {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
	}()
	fail := func(format string, args ...any) {
		log.Printf("ERROR: "+format, args...)
		exitCode = 1
	}

	// Load .env first so environment variables are available as defaults
	// you may have no .env, it's okay
	_ = godotenv.Load(".env")

	// Define flags with environment variables as defaults
	fileSavePath := flag.String("file-save-path", os.Getenv("FILE_SAVE_PATH"), "Main file storage directory (env: FILE_SAVE_PATH)")
	dbType := flag.String("db-type", os.Getenv("DB_TYPE"), "Database type: SQLITE or POSTGRES (env: DB_TYPE)")
	dbDsn := flag.String("db-dsn", os.Getenv("DB_DSN"), "Database connection string (env: DB_DSN)")
	dbReadOnlyDsn := flag.String("db-readonly-dsn", os.Getenv("DB_READONLY_DSN"), "Read-only database connection string (env: DB_READONLY_DSN)")
	dbLogFile := flag.String("db-log-file", os.Getenv("DB_LOG_FILE"), "DB log destination: STDOUT, empty, or file path (env: DB_LOG_FILE)")
	dbSlowQueryThreshold := flag.Duration("db-slow-query-threshold", parseDurationEnv("DB_SLOW_QUERY_THRESHOLD", 0), "Log SQL queries slower than this duration (e.g. 200ms) to the DB log and the application log; 0 disables (env: DB_SLOW_QUERY_THRESHOLD)")
	bindAddress := flag.String("bind-address", os.Getenv("BIND_ADDRESS"), "Server bind address:port (env: BIND_ADDRESS)")
	ffmpegPath := flag.String("ffmpeg-path", os.Getenv("FFMPEG_PATH"), "Path to ffmpeg binary for video thumbnails (env: FFMPEG_PATH)")
	libreOfficePath := flag.String("libreoffice-path", os.Getenv("LIBREOFFICE_PATH"), "Path to LibreOffice binary for office document thumbnails (env: LIBREOFFICE_PATH)")
	skipFTS := flag.Bool("skip-fts", os.Getenv("SKIP_FTS") == "1", "Skip Full-Text Search initialization (env: SKIP_FTS=1)")
	skipVersionMigration := flag.Bool("skip-version-migration", os.Getenv("SKIP_VERSION_MIGRATION") == "1", "Skip resource version migration at startup (env: SKIP_VERSION_MIGRATION=1)")
	skipBlockRefCleanup := flag.Bool("skip-block-ref-cleanup", os.Getenv("SKIP_BLOCK_REF_CLEANUP") == "1", "Skip one-shot cleanup of dangling references in note_blocks (env: SKIP_BLOCK_REF_CLEANUP=1)")

	// Ephemeral/in-memory options
	memoryDB := flag.Bool("memory-db", os.Getenv("MEMORY_DB") == "1", "Use in-memory SQLite database (env: MEMORY_DB=1)")
	memoryFS := flag.Bool("memory-fs", os.Getenv("MEMORY_FS") == "1", "Use in-memory filesystem (env: MEMORY_FS=1)")
	ephemeral := flag.Bool("ephemeral", os.Getenv("EPHEMERAL") == "1", "Run in fully ephemeral mode (memory DB + memory FS) (env: EPHEMERAL=1)")
	seedDB := flag.String("seed-db", os.Getenv("SEED_DB"), "Path to SQLite file to use as basis for memory-db (env: SEED_DB)")
	seedFS := flag.String("seed-fs", os.Getenv("SEED_FS"), "Path to directory to use as read-only base for memory-fs (env: SEED_FS)")
	maxDBConnections := flag.Int("max-db-connections", parseIntEnv("MAX_DB_CONNECTIONS", 0), "Limit database connection pool size, useful for SQLite under test load (env: MAX_DB_CONNECTIONS)")
	maxJobConcurrency := flag.Int("max-job-concurrency", parseIntEnv("MAX_JOB_CONCURRENCY", 6), "Concurrency budget for the shared background job manager (env: MAX_JOB_CONCURRENCY)")
	exportRetention := flag.Duration("export-retention", parseDurationEnv("EXPORT_RETENTION", 24*time.Hour), "How long completed group-export tars stay on disk before cleanup (env: EXPORT_RETENTION)")
	downloadFailedRetention := flag.Duration("download-failed-retention", parseDurationEnv("DOWNLOAD_FAILED_RETENTION", 7*24*time.Hour), "How long a failed or cancelled download stays in the download history (env: DOWNLOAD_FAILED_RETENTION)")
	downloadHistoryRetention := flag.Duration("download-history-retention", parseDurationEnv("DOWNLOAD_HISTORY_RETENTION", 24*time.Hour), "How long a completed download stays in the download history (env: DOWNLOAD_HISTORY_RETENTION)")
	downloadCockpitLimit := flag.Int("download-cockpit-limit", parseIntEnv("DOWNLOAD_COCKPIT_LIMIT", 10), "How many finished downloads the jobs panel renders; active work and non-download jobs are never capped (env: DOWNLOAD_COCKPIT_LIMIT)")
	jobHistoryRetention := flag.Duration("job-history-retention", parseDurationEnv("JOB_HISTORY_RETENTION", jobs.DefaultHistoryRetention), "How long a succeeded or cancelled job's history stays after it finishes (env: JOB_HISTORY_RETENTION)")
	jobAttentionRetention := flag.Duration("job-attention-retention", parseDurationEnv("JOB_ATTENTION_RETENTION", jobs.DefaultAttentionRetention), "How long a failed or interrupted job's history stays after it finishes (env: JOB_ATTENTION_RETENTION)")
	jobPinLimit := flag.Int("job-pin-limit", parseIntEnv("JOB_PIN_LIMIT", jobs.DefaultPinLimit), "How many jobs one user may pin; pinning exempts a job's history from ordinary retention (env: JOB_PIN_LIMIT)")
	jobMigrationWritersDrained := flag.Bool("job-migration-writers-drained", os.Getenv("JOB_MIGRATION_WRITERS_DRAINED") == "1", "Attest that every pre-fence server process has stopped before the Job migration scrubs legacy input and advances writer epoch (env: JOB_MIGRATION_WRITERS_DRAINED=1)")
	jobMigrationBatchSize := flag.Int("job-migration-batch-size", parseIntEnv("JOB_MIGRATION_BATCH_SIZE", 100), "Maximum legacy source rows copied, verified or scrubbed per Job migration batch (env: JOB_MIGRATION_BATCH_SIZE)")
	pluginScheduleTick := flag.Duration("plugin-schedule-tick", parseDurationEnv("PLUGIN_SCHEDULE_TICK", application_context.DefaultScheduleTick), "How often the plugin scheduler looks for due work; bounds the resolution of every plugin schedule (env: PLUGIN_SCHEDULE_TICK)")
	maxImportSize := flag.Int64("max-import-size", parseInt64Env("MAX_IMPORT_SIZE", 10737418240), "Maximum import tar upload size in bytes (env: MAX_IMPORT_SIZE)")
	maxUploadSize := flag.Int64("max-upload-size", parseInt64Env("MAX_UPLOAD_SIZE", 2<<30), "Maximum per-upload body size in bytes for resource and version uploads (default: 2 GB, env: MAX_UPLOAD_SIZE)")
	maxJSONBody := flag.Int64("max-json-body", parseInt64Env("MAX_JSON_BODY", 0), "Maximum application/json request body size in bytes; 0 disables the limit (default: 0/unlimited, env: MAX_JSON_BODY)")
	maxActionEntities := flag.Int("max-action-entities", int(parseInt64Env("MAX_ACTION_ENTITIES", 0)), "Maximum entities one plugin-action run may name; 0 selects the default of 1000 (env: MAX_ACTION_ENTITIES)")
	maxMassEditEntities := flag.Int("max-mass-edit-entities", int(parseInt64Env("MAX_MASS_EDIT_ENTITIES", 0)), "Maximum entities one mass edit may change; 0 selects the default of 10000 (env: MAX_MASS_EDIT_ENTITIES)")
	maxUserTokens := flag.Int("max-user-tokens", parseIntEnv("MAX_USER_TOKENS", 100), "Maximum API tokens a single user may hold; 0 disables the cap (default: 100, env: MAX_USER_TOKENS)")
	cleanupLogsDays := flag.Int("cleanup-logs-days", parseIntEnv("CLEANUP_LOGS_DAYS", 0), "Delete log entries older than N days on startup (0=disabled) (env: CLEANUP_LOGS_DAYS)")

	// Hash worker options
	hashWorkerCount := flag.Int("hash-worker-count", parseIntEnv("HASH_WORKER_COUNT", 4), "Number of concurrent hash calculation workers (env: HASH_WORKER_COUNT)")
	hashBatchSize := flag.Int("hash-batch-size", parseIntEnv("HASH_BATCH_SIZE", 500), "Resources to process per batch cycle (env: HASH_BATCH_SIZE)")
	hashPollInterval := flag.Duration("hash-poll-interval", parseDurationEnv("HASH_POLL_INTERVAL", time.Minute), "Time between batch processing cycles (env: HASH_POLL_INTERVAL)")
	hashSimilarityThreshold := flag.Int("hash-similarity-threshold", parseIntEnv("HASH_SIMILARITY_THRESHOLD", 10), "Maximum Hamming distance for similarity (env: HASH_SIMILARITY_THRESHOLD)")
	hashAHashThreshold := flag.Uint64("hash-ahash-threshold", parseUint64Env("HASH_AHASH_THRESHOLD", 5), "Max AHash Hamming distance for secondary check to suppress solid-color false positives (BH-018); 0 disables the check (env: HASH_AHASH_THRESHOLD)")
	hashWorkerDisabled := flag.Bool("hash-worker-disabled", os.Getenv("HASH_WORKER_DISABLED") == "1", "Disable hash worker (env: HASH_WORKER_DISABLED=1)")
	hashCacheSize := flag.Int("hash-cache-size", parseIntEnv("HASH_CACHE_SIZE", 100000), "Maximum entries in the hash similarity cache (env: HASH_CACHE_SIZE)")

	// Video thumbnail options
	videoThumbTimeout := flag.Duration("video-thumb-timeout", parseDurationEnv("VIDEO_THUMB_TIMEOUT", 30*time.Second), "Timeout for video thumbnail ffmpeg invocation (env: VIDEO_THUMB_TIMEOUT)")
	videoThumbLockTimeout := flag.Duration("video-thumb-lock-timeout", parseDurationEnv("VIDEO_THUMB_LOCK_TIMEOUT", 60*time.Second), "Timeout waiting for video thumbnail lock (env: VIDEO_THUMB_LOCK_TIMEOUT)")
	videoThumbConcurrency := flag.Int("video-thumb-concurrency", parseIntEnv("VIDEO_THUMB_CONCURRENCY", 4), "Max concurrent video thumbnail generations (env: VIDEO_THUMB_CONCURRENCY)")

	// HLS ingest options. An HLS URL is one request that becomes hundreds, so
	// these bound what one submitted playlist can spend. Both limits refuse
	// rather than truncate: half a video stored as a success is worse than a
	// refusal that says why.
	hlsMaxSegments := flag.Int("hls-max-segments", parseIntEnv("HLS_MAX_SEGMENTS", 5000), "Maximum segments in one HLS download (env: HLS_MAX_SEGMENTS)")
	hlsMaxBytes := flag.Int64("hls-max-bytes", parseInt64Env("HLS_MAX_BYTES", 16<<30), "Maximum total bytes for one HLS download (env: HLS_MAX_BYTES)")
	hlsConcurrency := flag.Int("hls-concurrency", parseIntEnv("HLS_CONCURRENCY", 4), "Segments fetched at once during an HLS download (env: HLS_CONCURRENCY)")
	hlsTempDir := flag.String("hls-temp-dir", os.Getenv("HLS_TEMP_DIR"), "Working directory for HLS assembly and for the temporary copy every resource upload is staged through; empty uses the system temp directory (env: HLS_TEMP_DIR)")

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
	remoteUserAgent := flag.String("remote-user-agent", os.Getenv("REMOTE_USER_AGENT"), "User-Agent the server's own fetches send (/v1/resource/remote, the download queue, HLS segments). Empty uses a browser-like default, because some media endpoints answer 403 to Go's (env: REMOTE_USER_AGENT)")
	allowPrivateFetch := flag.String("allow-private-fetch", os.Getenv("ALLOW_PRIVATE_FETCH"), "Comma-separated private addresses or CIDR blocks the server's own fetches may reach (/v1/resource/remote, the download queue, calendar blocks). Empty (default) denies every private, loopback and link-local address, plus Azure's host-internal 168.63.129.16, which is what stops a user-supplied URL from reaching the cloud metadata endpoint or an internal service. Name addresses, not hostnames. IPv4 blocks must be /8 or longer, IPv6 blocks /32 or longer (env: ALLOW_PRIVATE_FETCH)")

	// Share server options
	sharePort := flag.String("share-port", os.Getenv("SHARE_PORT"), "Port for public share server (env: SHARE_PORT)")
	shareBindAddress := flag.String("share-bind-address", getEnvOrDefault("SHARE_BIND_ADDRESS", "0.0.0.0"), "Bind address for share server (env: SHARE_BIND_ADDRESS)")
	sharePublicURL := flag.String("share-public-url", os.Getenv("SHARE_PUBLIC_URL"), "Externally-routable base URL for shared notes (e.g. https://share.example.com). If unset, the share sidebar shows a warning and the relative /s/<token> path instead of synthesizing a bind-address URL (env: SHARE_PUBLIC_URL)")

	// Documentation link options
	docsSiteBaseURL := flag.String("docs-site-base-url", getEnvOrDefault("DOCS_SITE_BASE_URL", application_context.DefaultDocsSiteBaseURL), "Base URL for contextual links to the published documentation site (env: DOCS_SITE_BASE_URL)")
	docsLinksDisabled := flag.Bool("docs-links-disabled", os.Getenv("DOCS_LINKS_DISABLED") == "1", "Disable contextual external documentation links in the app (env: DOCS_LINKS_DISABLED=1)")

	// MRQL options
	mrqlTimeout := flag.Duration("mrql-query-timeout", parseDurationEnv("MRQL_QUERY_TIMEOUT", 10*time.Second), "Maximum execution time for MRQL queries (env: MRQL_QUERY_TIMEOUT)")
	mrqlDefaultLimit := flag.Int("mrql-default-limit", parseIntEnv("MRQL_DEFAULT_LIMIT", 500), "Default LIMIT applied to MRQL queries without an explicit LIMIT clause (env: MRQL_DEFAULT_LIMIT)")
	mrqlPageQueryBudget := flag.Int("mrql-page-query-budget", parseIntEnv("MRQL_PAGE_QUERY_BUDGET", 200), "Maximum distinct MRQL queries a single page render may execute via inline [mrql] shortcodes; 0 disables (env: MRQL_PAGE_QUERY_BUDGET)")

	// Plugin options
	pluginPath := flag.String("plugin-path", getEnvOrDefault("PLUGIN_PATH", "./plugins"), "Path to plugin directory (env: PLUGIN_PATH)")
	pluginsDisabled := flag.Bool("plugins-disabled", os.Getenv("PLUGINS_DISABLED") == "1", "Disable all plugins (env: PLUGINS_DISABLED=1)")
	pluginCommandPathEnv, pluginCommandPathEnvSet := os.LookupEnv("PLUGIN_COMMAND_PATH")
	pluginCommandPathDefault := os.Getenv("PATH")
	if pluginCommandPathEnvSet {
		pluginCommandPathDefault = pluginCommandPathEnv
	}
	pluginCommandPath := flag.String("plugin-command-path", pluginCommandPathDefault, "Trusted absolute directories searched for plugin command executables (env: PLUGIN_COMMAND_PATH)")
	pluginCommandStagingEnv, pluginCommandStagingEnvSet := os.LookupEnv("PLUGIN_COMMAND_STAGING_PATH")
	pluginCommandStagingPath := flag.String("plugin-command-staging-path", pluginCommandStagingEnv, "Private OS staging root for plugin command exchange files (env: PLUGIN_COMMAND_STAGING_PATH)")
	pluginCommandRunQuota := flag.Int64("plugin-command-run-quota", parseInt64Env("PLUGIN_COMMAND_RUN_QUOTA", application_context.DefaultPluginCommandRunQuota), "Maximum sampled bytes for one plugin command run (env: PLUGIN_COMMAND_RUN_QUOTA)")
	pluginCommandStagingQuota := flag.Int64("plugin-command-staging-quota", parseInt64Env("PLUGIN_COMMAND_STAGING_QUOTA", application_context.DefaultPluginCommandStagingQuota), "Maximum sampled bytes for all plugin command staging (env: PLUGIN_COMMAND_STAGING_QUOTA)")
	pluginCommandExchangeRetention := flag.Duration("plugin-command-exchange-retention", parseDurationEnv("PLUGIN_COMMAND_EXCHANGE_RETENTION", application_context.DefaultPluginCommandExchangeRetention), "Retention for terminal plugin command exchange folders (env: PLUGIN_COMMAND_EXCHANGE_RETENTION)")
	pluginCommandOutputRetention := flag.Duration("plugin-command-output-retention", parseDurationEnv("PLUGIN_COMMAND_OUTPUT_RETENTION", application_context.DefaultPluginCommandOutputRetention), "Retention for plugin command output tails (env: PLUGIN_COMMAND_OUTPUT_RETENTION)")

	// Authentication options (opt-in). When disabled (default) every request runs
	// as an implicit administrator, matching the historical no-auth deployment.
	authEnabled := flag.Bool("auth", os.Getenv("AUTH_ENABLED") == "1", "Enable user accounts + RBAC (env: AUTH_ENABLED=1)")
	sessionTTL := flag.Duration("session-ttl", parseDurationEnv("SESSION_TTL", 30*24*time.Hour), "How long a browser login session stays valid (env: SESSION_TTL)")
	sessionCookieSecure := flag.Bool("session-cookie-secure", os.Getenv("SESSION_COOKIE_SECURE") == "1", "Mark the session cookie Secure (HTTPS-only) (env: SESSION_COOKIE_SECURE=1)")
	createAdminUser := flag.String("create-admin-user", os.Getenv("CREATE_ADMIN_USER"), "Bootstrap: create or reset this admin username at startup (env: CREATE_ADMIN_USER)")
	createAdminPassword := flag.String("create-admin-password", os.Getenv("CREATE_ADMIN_PASSWORD"), "Bootstrap: password for -create-admin-user (env: CREATE_ADMIN_PASSWORD)")
	loginMaxAttempts := flag.Int("login-max-attempts", parseIntEnv("LOGIN_MAX_ATTEMPTS", 0), "Max failed login attempts per client IP within -login-attempt-window before throttling with HTTP 429; 0 disables (env: LOGIN_MAX_ATTEMPTS)")
	loginAttemptWindow := flag.Duration("login-attempt-window", parseDurationEnv("LOGIN_ATTEMPT_WINDOW", 15*time.Minute), "Sliding window for -login-max-attempts and the lockout duration once it is hit (env: LOGIN_ATTEMPT_WINDOW)")
	trustProxyHeaders := flag.Bool("trust-proxy-headers", os.Getenv("TRUST_PROXY_HEADERS") == "1", "Trust X-Forwarded-For for the client IP in login rate-limiting; enable only behind a trusted reverse proxy (env: TRUST_PROXY_HEADERS=1)")

	flag.Parse()
	setFlags := make(map[string]bool)
	flag.CommandLine.Visit(func(f *flag.Flag) { setFlags[f.Name] = true })
	resolvedPluginCommandPath, pluginCommandPathExplicit := pluginCommandPathValue(
		*pluginCommandPath, setFlags["plugin-command-path"], pluginCommandPathEnv, pluginCommandPathEnvSet, os.Getenv("PATH"),
	)
	pluginCommandStagingExplicit := pluginCommandStagingEnvSet || setFlags["plugin-command-staging-path"]

	deepSeekAPIKey := os.Getenv("DEEPSEEK_API_KEY")
	deepSeekModel := getEnvOrDefault("DEEPSEEK_MODEL", application_context.DefaultDeepSeekMRQLGenerationModel)
	deepSeekTimeoutRaw := getEnvOrDefault("DEEPSEEK_TIMEOUT", application_context.DefaultDeepSeekMRQLGenerationTimeout.String())
	deepSeekTimeout, err := time.ParseDuration(deepSeekTimeoutRaw)
	if err != nil {
		fail("invalid DEEPSEEK_TIMEOUT=%q: %v", deepSeekTimeoutRaw, err)
		return
	}

	// Build alt file systems map from flags or fall back to env vars
	altFileSystems := make(map[string]string)
	if len(altFSFlags) > 0 {
		// Use command-line flags
		for _, fs := range altFSFlags {
			parts := strings.SplitN(fs, ":", 2)
			if len(parts) == 2 {
				// The empty key is not a name, it is how every create path
				// spells "the main filesystem" (BH-023). Accepting it built a
				// map entry nothing could address: uploads never consult it,
				// export coalesces it away, and group import treats it as the
				// default. The env-var branch below already requires a name;
				// this one silently did not.
				if parts[0] == "" {
					fail("Invalid -alt-fs entry %q: the key may not be empty (an empty storage key means the main filesystem)", fs)
					return
				}
				altFileSystems[parts[0]] = parts[1]
			} else {
				fail("Invalid -alt-fs format: %s (expected key:path)", fs)
				return
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

	// Parsed here to fail startup on a bad entry, rather than at first fetch on
	// a background worker where the operator would learn about it from a failed
	// download. The context builds the policy again from these strings — this
	// call is the validation, and it is the only one that can refuse to boot.
	allowPrivateFetchEntries := splitCommaList(*allowPrivateFetch)
	if _, err := plugin_system.HostFetchPolicy(allowPrivateFetchEntries); err != nil {
		fail("invalid %v", err)
		return
	}

	// Checked at startup for the same reason: a User-Agent net/http refuses
	// breaks *every* host fetch, and the runtime setting's own validator does
	// not see a value that arrived by flag or environment.
	if err := hostfetch.ValidateUserAgent(*remoteUserAgent); err != nil {
		fail("invalid -remote-user-agent: %v", err)
		return
	}

	pluginCommandConfig, err := application_context.ResolvePluginCommandConfig(application_context.PluginCommandConfigInput{
		CommandPath: resolvedPluginCommandPath, CommandPathExplicit: pluginCommandPathExplicit,
		InheritedPath: os.Getenv("PATH"),
		StagingPath:   *pluginCommandStagingPath, StagingPathExplicit: pluginCommandStagingExplicit,
		FileSavePath: *fileSavePath, MemoryFS: useMemoryFS,
		RunQuota: *pluginCommandRunQuota, RunQuotaSet: true,
		StagingQuota: *pluginCommandStagingQuota, StagingQuotaSet: true,
		ExchangeRetention: *pluginCommandExchangeRetention, ExchangeRetentionSet: true,
		OutputRetention: *pluginCommandOutputRetention, OutputRetentionSet: true,
	})
	if err != nil {
		fail("invalid plugin command configuration: %v", err)
		return
	}
	if pluginCommandConfig.TemporaryStaging {
		defer os.RemoveAll(pluginCommandConfig.StagingPath)
	}

	// Create configuration
	cfg := &application_context.MahresourcesInputConfig{
		FileSavePath:                 *fileSavePath,
		DbType:                       *dbType,
		DbDsn:                        *dbDsn,
		DbReadOnlyDsn:                *dbReadOnlyDsn,
		DbLogFile:                    *dbLogFile,
		DbSlowQueryThreshold:         *dbSlowQueryThreshold,
		BindAddress:                  *bindAddress,
		SharePort:                    *sharePort,
		ShareBindAddress:             *shareBindAddress,
		SharePublicURL:               *sharePublicURL,
		DocsSiteBaseURL:              *docsSiteBaseURL,
		DocsLinksDisabled:            *docsLinksDisabled,
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
		RemoteUserAgent:              *remoteUserAgent,
		AllowPrivateFetch:            allowPrivateFetchEntries,
		MaxDBConnections:             *maxDBConnections,
		VideoThumbnailTimeout:        *videoThumbTimeout,
		VideoThumbnailLockTimeout:    *videoThumbLockTimeout,
		VideoThumbnailConcurrency:    uint(*videoThumbConcurrency),
		HLSMaxSegments:               *hlsMaxSegments,
		HLSMaxTotalBytes:             *hlsMaxBytes,
		HLSConcurrency:               *hlsConcurrency,
		HLSTempDir:                   *hlsTempDir,
		PluginPath:                   *pluginPath,
		PluginsDisabled:              *pluginsDisabled,
		HashWorkerEnabled:            !*hashWorkerDisabled,
		HashWorkerCount:              *hashWorkerCount,
		HashBatchSize:                *hashBatchSize,
		HashPollInterval:             *hashPollInterval,
		HashSimilarityThreshold:      *hashSimilarityThreshold,
		HashAHashThreshold:           *hashAHashThreshold,
		HashCacheSize:                *hashCacheSize,
		EphemeralMode:                *ephemeral,
		SkipFTS:                      *skipFTS,
		MaxJobConcurrency:            *maxJobConcurrency,
		ExportRetention:              *exportRetention,
		DownloadFailedRetention:      *downloadFailedRetention,
		DownloadHistoryRetention:     *downloadHistoryRetention,
		DownloadCockpitLimit:         *downloadCockpitLimit,
		JobHistoryRetention:          *jobHistoryRetention,
		JobAttentionRetention:        *jobAttentionRetention,
		JobPinLimit:                  *jobPinLimit,
		PluginScheduleTick:           *pluginScheduleTick,
		MaxImportSize:                *maxImportSize,
		MaxUploadSize:                *maxUploadSize,
		MaxJSONBodySize:              *maxJSONBody,
		MaxActionEntities:            *maxActionEntities,
		MaxMassEditEntities:          *maxMassEditEntities,
		MaxUserTokens:                *maxUserTokens,
		MRQLDefaultLimit:             *mrqlDefaultLimit,
		MRQLPageQueryBudget:          *mrqlPageQueryBudget,
		MRQLQueryTimeoutBoot:         *mrqlTimeout,
		DeepSeekAPIKey:               deepSeekAPIKey,
		DeepSeekModel:                deepSeekModel,
		DeepSeekTimeout:              deepSeekTimeout,
		TemplateSigningKey:           os.Getenv("TEMPLATE_SIGNING_KEY"),
		AuthEnabled:                  *authEnabled,
		SessionTTL:                   *sessionTTL,
		SessionCookieSecure:          *sessionCookieSecure,
		LoginRateLimit:               *loginMaxAttempts,
		LoginRateWindow:              *loginAttemptWindow,
		TrustProxyHeaders:            *trustProxyHeaders,
		CreateAdminUser:              *createAdminUser,
		CreateAdminPassword:          *createAdminPassword,
	}
	cfg.PluginCommandPath = pluginCommandConfig.CommandPath
	cfg.PluginCommandStagingPath = pluginCommandConfig.StagingPath
	cfg.PluginCommandRunQuota = pluginCommandConfig.RunQuota
	cfg.PluginCommandStagingQuota = pluginCommandConfig.StagingQuota
	cfg.PluginCommandExchangeRetention = pluginCommandConfig.ExchangeRetention
	cfg.PluginCommandOutputRetention = pluginCommandConfig.OutputRetention
	cfg.PluginCommandStagingTemporary = pluginCommandConfig.TemporaryStaging

	// Loaded before the context exists, so a deployment that could accept
	// durable secret work without a replay key it will still hold after a
	// restart refuses to start rather than discovering it when a Retry cannot
	// work. Nothing below this line is reached on that failure.
	jobReplayKeyring, err := jobs.LoadReplayKeyring(jobReplayKeyConfig(*fileSavePath, *dbType, useMemoryDB))
	if err != nil {
		fail("failed to load the job replay key: %v", err)
		return
	}

	context, db, mainFs, err := application_context.OpenContextWithConfig(cfg)
	if err != nil {
		fail("failed to create application context: %v", err)
		return
	}
	context.SetJobReplayKeyring(jobReplayKeyring)
	if context.Config.DeepSeekAPIKey != "" {
		provider := application_context.NewDeepSeekMRQLDraftProvider(
			application_context.DefaultDeepSeekChatCompletionsURL,
			context.Config.DeepSeekAPIKey,
			context.Config.DeepSeekModel,
			nil,
		)
		context.SetMRQLGenerator(application_context.NewMRQLGenerator(provider, application_context.MRQLGenerationConfig{
			APIKey:   context.Config.DeepSeekAPIKey,
			Model:    context.Config.DeepSeekModel,
			Timeout:  context.Config.DeepSeekTimeout,
			Postgres: context.Config.DbType == constants.DbTypePosgres,
		}))

		templateProvider := application_context.NewDeepSeekTemplateDraftProvider(
			application_context.DefaultDeepSeekChatCompletionsURL,
			context.Config.DeepSeekAPIKey,
			context.Config.DeepSeekModel,
			nil,
		)
		context.SetTemplateGenerator(application_context.NewTemplateGenerator(templateProvider, application_context.TemplateGenerationConfig{
			APIKey:  context.Config.DeepSeekAPIKey,
			Model:   context.Config.DeepSeekModel,
			Timeout: context.Config.DeepSeekTimeout,
		}))
	}

	// Register process-lifetime teardown before any later startup failure can
	// return. Commands are registered after recovery below, so LIFO remains:
	// command dispatcher, download manager, plugin manager.
	if context.PluginManager() != nil {
		defer context.PluginManager().Close()
	}
	defer context.DownloadManager().Shutdown()

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

	// Pre-migration: resolve/create default resource category and backfill NULLs.
	// This must happen before AutoMigrate adds the NOT NULL constraint on resource_category_id.
	context.DefaultResourceCategoryID = resolveDefaultResourceCategory(db, context.Config.DbType)
	var nullRCCount int64
	db.Raw("SELECT count(*) FROM resources WHERE resource_category_id IS NULL").Scan(&nullRCCount)
	if nullRCCount > 0 {
		log.Printf("Pre-migration: backfilling %d resources with NULL resource_category_id → %d", nullRCCount, context.DefaultResourceCategoryID)
		for {
			result := db.Exec(
				"UPDATE resources SET resource_category_id = ? WHERE id IN (SELECT id FROM resources WHERE resource_category_id IS NULL LIMIT 10000)",
				context.DefaultResourceCategoryID,
			)
			if result.Error != nil || result.RowsAffected == 0 {
				break
			}
		}
	}

	// Disable foreign keys during AutoMigrate for SQLite.
	// SQLite can't ALTER TABLE to add constraints, so GORM recreates the table
	// (create temp, copy, drop original, rename). The DROP fails if other tables
	// reference it with FKs enabled.
	if context.Config.DbType == constants.DbTypeSqlite {
		db.Exec("PRAGMA foreign_keys = OFF")
	}

	// Migration order matters for Postgres: tables with FK constraints must be
	// created after the tables they reference. Independent tables first, then
	// tables with foreign keys in dependency order.
	if err := db.AutoMigrate(
		// Independent tables (no FK dependencies)
		&models.Query{},
		&models.Series{},
		&models.Tag{},
		&models.Category{},
		&models.ResourceCategory{},
		&models.NoteType{},
		&models.LogEntry{},
		&models.PluginState{},
		&models.PluginKV{},
		&models.RuntimeSetting{},
		&models.SavedMRQLQuery{},
		&models.TemplatePartial{},
		&models.DownloadHistoryEntry{},
		&models.ScheduledDownload{}, // no FK association; created_by_user_id is a scalar
		// No FK association either; plugin_name/schedule_id is its own key and
		// created_by_user_id is a scalar.
		&models.PluginSchedule{}, // no FK association; created_by_user_id is a scalar
		// Durable command rows are associationless too. The actor columns are
		// scalar provenance; run/output and import/map lifetimes are coordinated
		// transactionally by the command store rather than by foreign keys.
		&models.PluginCommandRun{},
		&models.PluginCommandRunOutput{},
		&models.PluginCommandImport{},
		&models.PluginCommandImportMap{},
		// Likewise associationless: the Extent and the plan are JSON documents of
		// ids, deliberately not foreign keys — a Cluster naming a Resource that has
		// since been deleted is a staleness the apply revalidation must detect and
		// report, not a row the database quietly rewrites.
		&models.ResourceReduction{},
		// Tables with FK to independent tables
		&models.Group{},             // FK to Category (self-referencing Owner is handled by GORM)
		&models.GroupRelationType{}, // FK to Category
		&models.Resource{},          // FK to ResourceCategory, Series, Group
		&models.User{},              // FK to Group (ScopeGroupId)
		&models.SavedSearch{},       // personal saved list searches
		&models.UserSetting{},       // per-user KV prefs; no FK association (like PluginKV)
		// Tables with FK to Resource/Group/Note
		&models.Note{},               // FK to Group, NoteType; many2many with Resource
		&models.ResourceVersion{},    // FK to Resource
		&models.NoteBlock{},          // FK to Note
		&models.Preview{},            // FK to Resource
		&models.GroupRelation{},      // FK to Group, GroupRelationType
		&models.ImageHash{},          // FK to Resource
		&models.ResourceSimilarity{}, // FK to Resource
		&models.Session{},            // FK to User
		&models.ApiToken{},           // FK to User
	); err != nil {
		fail("failed to migrate: %v", err)
		return
	}

	// Refresh planner statistics for the tables that just gained the v2 hash
	// columns. On Postgres, columns added by AutoMigrate have no statistics
	// until autoanalyze (~10% row churn), and without them the chunk-index
	// candidate queries fall back to full sequential scans. Non-fatal: worst
	// case is the old slow-plan behaviour until autoanalyze catches up.
	if err := models.AnalyzePerceptualHashTables(db); err != nil {
		log.Printf("Warning: post-migrate ANALYZE failed: %v", err)
	}

	// Repair deployments whose image_hashes.resource_id has a non-unique index
	// instead of the unique index the model declares. Without it the hash
	// worker's ON CONFLICT (resource_id) upsert fails on every save (SQLSTATE
	// 42P10), re-hashing the same resources every cycle and pinning the CPU.
	// Idempotent, self-healing, and a no-op once the unique index exists.
	if err := models.EnsureImageHashResourceIdUnique(db); err != nil {
		log.Printf("Warning: ensuring image_hashes.resource_id unique index failed: %v", err)
	}

	// The durable Job core. Separated from the migration list above because the
	// step is not only a schema: it also seeds the writer epoch that every
	// subsequent start preflights against, and keeping the two together is what
	// makes "the tables exist" and "the epoch is recorded" one fact rather than
	// two that can drift.
	if err := migrateJobCore(db); err != nil {
		fail("failed to migrate the job core: %v", err)
		return
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

	// Initialize runtime settings (bucket-A overrides: sizes, timeouts, etc.)
	settings := application_context.NewRuntimeSettings(
		db,
		application_context.NewStdlibSettingsLogger(),
		application_context.BuildSpecsExported(),
		application_context.BuildDefaultsFromConfig(context.Config),
	)
	if err := settings.Load(); err != nil {
		fail("failed to load runtime settings: %v", err)
		return
	}
	settings.SetAuditor(application_context.NewContextAuditor(context))
	context.SetSettings(settings)
	// Wire the live runtime-settings provider into the download manager so that
	// timeout and export-retention overrides take effect per download start.
	context.DownloadManager().SetSettings(context.Settings())
	// Now that the manager reads retention through RuntimeSettings, run the
	// startup sweep so the first-pass cleanup honors any persisted override.
	context.RunStartupExportSweep()
	// Install the shared control plane before command recovery so recovery can
	// reconcile authoritative command rows and canonical Jobs in one transaction.
	jobService := installJobControlPlane(context)
	// Import Retry selectors use durable file-availability facts. Repair old
	// missing facts and stale files after the service exists, before migration,
	// plugin activation, or dispatch can expose import commands.
	if err := context.ReconcileImportCommandAvailability(); err != nil {
		fail("failed to reconcile import command availability: %v", err)
		return
	}
	migration, err := context.RunJobMigrationToGate(application_context.JobMigrationOptions{
		BatchSize: *jobMigrationBatchSize, MaxBatches: 20, WritersDrained: *jobMigrationWritersDrained,
	})
	if err != nil {
		fail("failed to run the bounded Job source migration: %v", err)
		return
	}
	if !migration.Complete {
		log.Printf("[jobs] source migration remains in phase %s after %d bounded batches; plaintext retirement is not active (quarantined sources: %d)",
			migration.Phase, migration.Batches, migration.BlockedSources)
	}

	// Recovery must settle every durable command/import writer before a plugin
	// VM can load and observe mah.commands or mah.fs. The context-owned gate keeps
	// disabled deployments free of dispatcher goroutines and host publication.
	if err := context.StartPluginCommandsIfEnabled(gocontext.Background(), context.PluginCommandSettings()); err != nil {
		fail("failed to start plugin commands: %v", err)
		return
	}
	defer func() {
		if err := context.StopPluginCommands(); err != nil {
			log.Printf("ERROR: plugin command shutdown failed: %v", err)
			exitCode = 1
		}
	}()

	seed.AddInitialData(db)

	// Bootstrap an admin account from flags/env when requested. Idempotent and
	// independent of whether auth is currently enabled, so an operator can seed
	// credentials before flipping -auth on.
	if cfg.CreateAdminUser != "" {
		if cfg.CreateAdminPassword == "" {
			fail("-create-admin-user requires -create-admin-password")
			return
		}
		if u, err := context.EnsureAdminUser(cfg.CreateAdminUser, cfg.CreateAdminPassword); err != nil {
			fail("failed to bootstrap admin user: %v", err)
			return
		} else {
			log.Printf("bootstrapped admin user %q (id=%d)", u.Username, u.ID)
		}
	}
	// Guarantee a root admin exists in every auth mode. Placed after the
	// -create-admin-user bootstrap so an operator-provided admin becomes the root
	// (the oldest enabled admin). Auto-create lives only here (not in
	// NewMahresourcesContext) so a fresh context stays at zero users for tests.
	// EnsureRootAdmin also warms the no-auth default-actor cache.
	if rootUser, err := context.EnsureRootAdmin(); err != nil {
		fail("failed to ensure root admin: %v", err)
		return
	} else if rootUser != nil && rootUser.PasswordAutoGenerated {
		log.Printf("auto-created root administrator %q (id=%d) with a random password", rootUser.Username, rootUser.ID)
	}

	// Lockout guard: under -auth, if every enabled admin has an auto-generated
	// (operator-unknown) password, warn on every boot with remediation. This
	// persists across boots until an operator sets a real password.
	if context.Config.AuthEnabled {
		if n, err := context.CountEnabledAdminsWithRealPassword(); err == nil && n == 0 {
			log.Println("WARNING: -auth is enabled but every enabled administrator has an " +
				"auto-generated password that no operator knows. Restart with " +
				"-create-admin-user=<name> -create-admin-password=<password> to set known " +
				"credentials (this resets the account and clears the auto-generated marker).")
		}
	}

	// The durable Job control plane, installed, and then the enabled plugins.
	//
	// One step rather than two because their order is a correctness property: an
	// enabled plugin's init() runs during activation, and the host half of the
	// plugin-work seam is installed with the control plane. Whatever the order, the
	// process has to end up with the plane installed before any plugin can accept
	// work — see installJobControlPlaneBeforePluginActivation.
	activatePluginsWithJobControlPlane(context)
	if pm := context.PluginManager(); pm != nil {
		if plugins := pm.Plugins(); len(plugins) > 0 {
			log.Printf("[plugin] Activated %d plugin(s)", len(plugins))
		}
	}

	if err := models.EnsureSupplementalIndexes(db); err != nil {
		fail("Error when creating supplemental indexes: %v", err)
		return
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

	// One-shot cleanup of dangling block references (BH-020 — skip with -skip-block-ref-cleanup flag)
	if !*skipBlockRefCleanup {
		go func() {
			if err := application_context.MigrateBlockReferencesOnce(db); err != nil {
				log.Printf("Warning: block-ref cleanup migration failed: %v", err)
			}
		}()
	} else {
		log.Println("Block-ref cleanup migration skipped (-skip-block-ref-cleanup flag or SKIP_BLOCK_REF_CLEANUP=1)")
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
		WorkerCount:           *hashWorkerCount,
		BatchSize:             *hashBatchSize,
		PollInterval:          *hashPollInterval,
		SimilarityThresholdFn: context.Settings().HashSimilarityThreshold,
		AHashThresholdFn:      context.Settings().HashAHashThreshold,
		Disabled:              *hashWorkerDisabled,
		CacheSize:             *hashCacheSize,
		BackfillPausedFn:      context.Settings().HashBackfillPaused,
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

	// The plugin scheduler. It owns its own ticker rather than borrowing the
	// download manager's, whose five-minute interval is hardcoded and whose
	// registered callbacks take no context and cannot be drained.
	//
	// Started here, after the startup plugin enable has had its chance to record
	// what each plugin declares. It re-reads the due set on every tick, so the
	// ordering is a courtesy rather than a requirement — but a first tick against
	// rows that do not exist yet would simply do nothing, which is harder to
	// read in a log than not ticking at all.
	//
	// Stop is deferred like the workers above, and it waits: an ActionJob lives
	// in this process's memory only, so a run abandoned at exit holds its claim
	// until that claim expires, and its schedule is unavailable until then.
	scheduler := application_context.NewPluginScheduler(context, cfg.PluginScheduleTick)
	scheduler.Start()
	defer scheduler.Stop()
	// Installed on the context so an operator can ask for a run outside the
	// cadence. The scheduler is built from the context, so this cannot be done at
	// construction; it is the same shape as the two worker queues above.
	context.SetPluginScheduler(scheduler)

	// Terminal job events for mah.on. Started here rather than in the context
	// for the same reason the scheduler is: it owns a goroutine, so the place
	// that can defer its Stop is the place that starts it. Stop is bounded, so a
	// plugin handler that ignores its context cannot hold the exit open.
	jobEvents := application_context.NewJobEventDispatcher(context)
	defer jobEvents.Stop()
	context.SetJobEventSink(jobEvents)

	// The durable dispatch loop: the application-owned half of the Job control
	// plane. Started here for the same reason the scheduler and the event
	// dispatcher are — it owns goroutines, so the place that can defer its Stop
	// is the place that starts it.
	//
	// It is inert until a Kind registers an adapter with it, and it runs
	// regardless: reconciliation is not optional, because a claim left by a
	// previous process still has to be resolved even when this one cannot run the
	// work at all. The deployment-wide concurrency budget is read from the
	// deployment's own configuration inside the runtime, because the host-side
	// claim paths (a plugin action's submitting process) take that same budget:
	// one number, one place it is read.
	//
	// The service itself was installed before plugin activation; this is only the
	// loop.
	jobRuntime := application_context.NewJobRuntime(context, jobService, application_context.JobRuntimeConfig{})
	jobRuntime.Start()
	defer jobRuntime.Stop()

	// Start share server if configured.
	//
	// Start binds synchronously, so the "available at" line is printed only
	// after something is listening. Returning on failure preserves every command,
	// download and plugin teardown registered above.
	if cfg.SharePort != "" {
		shareServer := server.NewShareServer(context)
		if err := shareServer.Start(cfg.ShareBindAddress, cfg.SharePort); err != nil {
			fail("Failed to start share server: %v\n"+
				"Sharing was requested (-share-port=%s) and the port is not available. "+
				"Free the port, choose another, or drop -share-port to run without sharing.",
				err, cfg.SharePort)
			return
		}
		defer shareServer.Stop()
		log.Printf("Share server available at http://%s:%s", cfg.ShareBindAddress, cfg.SharePort)
	}

	srv := server.CreateServer(context, mainFs, context.Config.AltFileSystems)

	// Set BaseContext so all request contexts derive from a cancellable parent.
	// This allows long-lived handlers (e.g. SSE) to detect shutdown and return.
	serverCtx, serverCancel := gocontext.WithCancel(gocontext.Background())
	context.StartMetadataIndexer(serverCtx)
	srv.BaseContext = func(_ net.Listener) gocontext.Context {
		return serverCtx
	}

	serveErr := make(chan error, 1)
	go func() {
		serveErr <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-quit:
		log.Println("Shutting down server...")
	case err := <-serveErr:
		if err != nil && err != http.ErrServerClosed {
			fail("Server error: %v", err)
		}
	}

	serverCancel()

	shutdownCtx, shutdownCancel := gocontext.WithTimeout(gocontext.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		// Return through the registered teardown stack so every in-flight download
		// and plugin command gets its durable terminal classification.
		log.Printf("ERROR: server forced to shutdown: %v", err)
		exitCode = 1
		return
	}

	log.Println("Server exited cleanly")
}

// installJobControlPlaneBeforePluginActivation installs the process's Job control plane
// and then activates the plugins an operator has enabled.
//
// The two are one step because their order is a correctness property rather than a
// matter of taste. An enabled plugin's init() runs during activation, `mah.start_job`
// reaches the durable control plane only once the host half of that seam is installed,
// and the plugin manager silently keeps its in-memory registry without it — so activating
// first made every job a plugin started at boot a memory-only record, on every boot,
// while a request starting the same work was durable. The step returns the plane the
// dispatch loop later runs on; starting that loop is a separate question, asked further
// down once startup is ready for it.
//
// It lives here rather than inline so the ordering is testable without starting a server,
// exactly as migrateJobCore does.
func installJobControlPlaneBeforePluginActivation(context *application_context.MahresourcesContext) (*jobs.Service, error) {
	jobService := installJobControlPlane(context)
	if err := context.ReconcileImportCommandAvailability(); err != nil {
		return nil, err
	}
	activatePluginsWithJobControlPlane(context)
	return jobService, nil
}

func installJobControlPlane(context *application_context.MahresourcesContext) *jobs.Service {
	jobService := jobs.NewService()
	// Installed on the context as well as handed back for the runtime: the runtime
	// registers the Kind adapters, and a facade holding a second control plane would
	// read one with no adapters registered. One process, one control plane.
	context.SetJobService(jobService)
	return jobService
}

func activatePluginsWithJobControlPlane(context *application_context.MahresourcesContext) {
	if context.PluginManager() != nil {
		if _, err := context.EnsurePluginStates(); err != nil {
			log.Printf("[plugin] WARNING: failed to initialize plugin states: %v", err)
		}
		context.ActivateEnabledPlugins()
	}
}

// migrateJobCore creates the durable job tables and seeds the writer epoch.
//
// Most job tables carry no foreign keys — a Job's owner and actor are scalar
// columns, and lineage links address Jobs by their UUID. The resource receipt is
// the exception: its Job and Resource references are acyclic, so their cascade
// constraints are installed explicitly after table migration.
//
// It lives here rather than inline so the startup step is testable without
// starting a server, and it is idempotent: AutoMigrate is, and
// EnsureJobWriterEpoch only ever writes a missing row.
func migrateJobCore(db *gorm.DB) error {
	if err := db.AutoMigrate(
		&models.Job{},
		&models.JobResourceReceipt{},
		&models.JobEvent{},
		&models.JobEventSequence{},
		&models.JobLink{},
		&models.JobOutput{},
		&models.JobReplayEnvelope{},
		&models.JobClaim{},
		&models.JobCapacityLease{},
		&models.JobPreference{},
		&models.JobPinGuard{},
		&models.JobCommandRequest{},
		&models.JobLegacyHandle{},
		&models.JobImportCommandFact{},
		&models.JobWriterEpoch{},
		&models.JobSourceMapping{},
		&models.JobMigrationCheckpoint{},
		&models.JobRuntimeFence{},
	); err != nil {
		return err
	}
	if err := models.EnsureJobResourceReceiptConstraints(db); err != nil {
		return err
	}
	return models.EnsureJobWriterEpoch(db)
}

// resolveDefaultResourceCategory finds or creates the default resource category
// and returns its ID. It checks: ID 1, then name "Default", then creates one.
// This runs before AutoMigrate so it uses raw SQL (the table may not have the
// NOT NULL constraint yet).
func resolveDefaultResourceCategory(db *gorm.DB, dbType string) uint {
	// Check if the resource_categories table exists at all (fresh database)
	var tableExists int64
	if dbType == constants.DbTypePosgres {
		db.Raw("SELECT count(*) FROM information_schema.tables WHERE table_name = 'resource_categories'").Scan(&tableExists)
	} else {
		db.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name='resource_categories'").Scan(&tableExists)
	}
	if tableExists == 0 {
		// Fresh database — table doesn't exist yet. AutoMigrate will create it,
		// then AddInitialData will create the default category. Return 1 as the
		// expected ID; the actual row creation happens after AutoMigrate.
		return 1
	}

	// 1. Prefer a category explicitly named "Default" — this is the canonical default
	//    regardless of what ID it was assigned.
	var defaultId uint
	db.Raw("SELECT id FROM resource_categories WHERE name = 'Default' LIMIT 1").Scan(&defaultId)
	if defaultId != 0 {
		return defaultId
	}

	// 2. No "Default" category exists. Create one with ID 1 if possible.
	if dbType == constants.DbTypePosgres {
		db.Exec("INSERT INTO resource_categories (id, name, description, created_at, updated_at) VALUES (1, 'Default', 'Default resource category.', NOW(), NOW()) ON CONFLICT (id) DO NOTHING")
		// Advance the sequence past 1 so the next auto-ID insert doesn't collide.
		db.Exec("SELECT setval(pg_get_serial_sequence('resource_categories', 'id'), GREATEST(nextval(pg_get_serial_sequence('resource_categories', 'id')), (SELECT COALESCE(MAX(id), 0) + 1 FROM resource_categories)))")
	} else {
		db.Exec("INSERT OR IGNORE INTO resource_categories (id, name, description, created_at, updated_at) VALUES (1, 'Default', 'Default resource category.', datetime('now'), datetime('now'))")
	}

	// Check if the insert succeeded (it may conflict if ID 1 is occupied by another category)
	db.Raw("SELECT id FROM resource_categories WHERE name = 'Default' LIMIT 1").Scan(&defaultId)
	if defaultId != 0 {
		return defaultId
	}

	// 3. ID 1 was occupied by a non-Default category. Create without explicit ID.
	if dbType == constants.DbTypePosgres {
		db.Raw("INSERT INTO resource_categories (name, description, created_at, updated_at) VALUES ('Default', 'Default resource category.', NOW(), NOW()) RETURNING id").Scan(&defaultId)
	} else {
		db.Exec("INSERT INTO resource_categories (name, description, created_at, updated_at) VALUES ('Default', 'Default resource category.', datetime('now'), datetime('now'))")
		db.Raw("SELECT id FROM resource_categories WHERE name = 'Default' LIMIT 1").Scan(&defaultId)
	}
	if defaultId != 0 {
		return defaultId
	}

	// Should not reach here, but return 1 as last resort
	log.Println("Warning: could not resolve default resource category, using ID 1")
	return 1
}
