package application_context

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/contracts"
	"mahresources/download_queue"
	"mahresources/groupio"
	"mahresources/idlock"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/search"
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
	// SharePublicURL is the externally-routable base URL for shared notes
	// (e.g., "https://share.example.com"). When set, the note sidebar and
	// the /admin/shares dashboard use it to build {SharePublicURL}/s/<token>.
	// When unset, the UI renders a warning + the relative /s/<token> path
	// rather than synthesizing a bind-address URL (BH-033).
	SharePublicURL string
	// DocsSiteBaseURL is the published documentation site's base URL. Runtime
	// overrides live in RuntimeSettings.
	DocsSiteBaseURL string
	// DocsLinksDisabled hides contextual links to the external documentation
	// site when true. Runtime overrides live in RuntimeSettings.
	DocsLinksDisabled bool
	// RemoteResourceConnectTimeout is the timeout for connecting to remote URLs (dial, TLS, response headers)
	RemoteResourceConnectTimeout time.Duration
	// RemoteResourceIdleTimeout is how long to wait before erroring if a remote server stops sending data
	RemoteResourceIdleTimeout time.Duration
	// RemoteResourceOverallTimeout is the maximum total time for a remote resource download (default: 30m)
	RemoteResourceOverallTimeout time.Duration
	// AllowPrivateFetch lists the private addresses and CIDR blocks the
	// application's own fetches may reach — /v1/resource/remote, the download
	// queue, and the calendar block's ICS fetch. Empty (the default) means none
	// of them, which is what closes the SSRF; entries are validated at startup
	// by plugin_system.HostFetchPolicy. See -allow-private-fetch.
	AllowPrivateFetch []string
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
	// HLSMaxSegments, HLSMaxTotalBytes and HLSConcurrency bound one HLS
	// download. They refuse rather than truncate: half a video delivered as a
	// success is worse than a refusal that says why. Zero means the hls
	// package's own default.
	HLSMaxSegments   int
	HLSMaxTotalBytes int64
	HLSConcurrency   int
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
	// HashAHashThreshold is the max AHash Hamming distance for the secondary
	// similarity check. 0 disables.
	HashAHashThreshold uint64
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
	// DownloadFailedRetention is how long a failed or cancelled download stays in
	// the persisted download history (default: one week).
	DownloadFailedRetention time.Duration
	// DownloadHistoryRetention is how long a completed download stays in the
	// persisted download history.
	DownloadHistoryRetention time.Duration
	// DownloadCockpitLimit is how many of the newest jobs the jobs panel renders.
	DownloadCockpitLimit int
	// PluginScheduleTick is how often the plugin scheduler looks for due work.
	// It bounds the resolution of every schedule.
	PluginScheduleTick time.Duration
	// MaxImportSize is the upper bound on import tar upload size in bytes
	MaxImportSize int64
	// MaxUploadSize is the upper bound on resource + version upload body size
	// in bytes. BH-034. 0 = unlimited (legacy behaviour).
	MaxUploadSize int64
	// MaxJSONBodySize bounds the size of application/json request bodies (the
	// shared decode path). 0 = unlimited (the historical default). Multipart
	// uploads are governed by MaxUploadSize instead and are unaffected.
	MaxJSONBodySize int64
	// MaxActionEntities bounds how many entities one plugin-action run may
	// name. 0 selects the default rather than "unlimited": an unbounded
	// fan-out is the defect this exists to stop, and every programmatic config
	// (api_tests, embeds) carries a zero it never meant as a policy.
	MaxActionEntities int
	// MaxUserTokens caps how many API tokens a single user may hold. 0 =
	// unlimited. Zero-value (test/programmatic configs) keeps the historical
	// uncapped behaviour; main.go sets a non-zero default for real deployments.
	MaxUserTokens int
	// MRQLDefaultLimit is the default LIMIT applied to MRQL queries without an
	// explicit LIMIT clause. When 0, callers should fall back to the historical
	// DefaultMRQLLimitFallback constant (keeps test fixtures that instantiate
	// MahresourcesConfig{} directly working without updates).
	MRQLDefaultLimit int
	// MRQLPageQueryBudget caps how many distinct MRQL queries a single page
	// render may execute via inline [mrql] shortcodes (per-card scoping means
	// list pages can accumulate many). 0 disables the budget.
	MRQLPageQueryBudget int
	// MRQLQueryTimeoutBoot is the boot-time default for the MRQL query timeout.
	// Runtime overrides live in RuntimeSettings.
	MRQLQueryTimeoutBoot time.Duration
	// DeepSeekAPIKey enables MRQL natural-language generation when configured.
	// It is intentionally env-only and must not be exposed through runtime settings.
	DeepSeekAPIKey string
	// DeepSeekModel is the chat model used for MRQL generation.
	DeepSeekModel string
	// DeepSeekTimeout bounds a single MRQL generation provider call.
	DeepSeekTimeout time.Duration
	// TemplateSigningKey is an optional operator-supplied secret (env
	// TEMPLATE_SIGNING_KEY) used to derive the HMAC key that authenticates the
	// [lazy]/[details] deferred-render tokens. Set it to a shared value across all
	// processes in a multi-process / behind-LB deployment so a lazy-reveal request
	// that lands on a different process than the page render still verifies. When
	// empty, each process generates its own per-boot random key. Env-only.
	TemplateSigningKey string

	// AuthEnabled turns on user accounts + RBAC. When false (default) the server
	// behaves exactly as the historical no-auth deployment: every request runs as
	// an implicit administrator.
	AuthEnabled bool
	// SessionTTL is how long a browser login session stays valid.
	SessionTTL time.Duration
	// SessionCookieName is the name of the session cookie.
	SessionCookieName string
	// SessionCookieSecure marks the session cookie Secure (HTTPS-only).
	SessionCookieSecure bool
	// LoginRateLimit is the number of failed login attempts permitted from a
	// single client IP within LoginRateWindow before further attempts are
	// throttled (HTTP 429). 0 disables login rate-limiting.
	LoginRateLimit int
	// LoginRateWindow is the sliding window over which failed login attempts are
	// counted (and the lockout duration once the limit is hit).
	LoginRateWindow time.Duration
	// TrustProxyHeaders controls whether X-Forwarded-For is trusted when deriving
	// the client IP (for login rate-limiting). Off by default: when the server is
	// exposed directly, a client controls XFF and could otherwise spoof it to
	// defeat the per-IP limiter. Enable only when behind a trusted reverse proxy.
	TrustProxyHeaders bool
}

// MahresourcesInputConfig holds all configuration options that can be passed
// via command-line flags or environment variables
type MahresourcesInputConfig struct {
	FileSavePath  string
	DbType        string
	DbDsn         string
	DbReadOnlyDsn string
	DbLogFile     string
	// DbSlowQueryThreshold logs SQL queries slower than this duration to the
	// DB log and the application log; 0 disables (default)
	DbSlowQueryThreshold time.Duration
	BindAddress          string
	FfmpegPath           string
	LibreOfficePath      string
	SharePort            string
	ShareBindAddress     string
	// SharePublicURL is the externally-routable base URL for shared notes.
	// Empty by default; see MahresourcesConfig.SharePublicURL. BH-033.
	SharePublicURL string
	// DocsSiteBaseURL is the published documentation site's base URL.
	DocsSiteBaseURL string
	// DocsLinksDisabled hides contextual external documentation links when true.
	DocsLinksDisabled bool
	AltFileSystems    map[string]string
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
	// AllowPrivateFetch lists private addresses/CIDR blocks the application's
	// own fetches may reach. See MahresourcesConfig.AllowPrivateFetch.
	AllowPrivateFetch []string
	// MaxDBConnections limits the database connection pool size (useful for SQLite in test environments)
	// When set to 0 (default), no limit is applied
	MaxDBConnections int
	// VideoThumbnailTimeout is the max time for a single ffmpeg invocation (default: 30s)
	VideoThumbnailTimeout time.Duration
	// VideoThumbnailLockTimeout is the max time to wait for the video thumbnail lock (default: 60s)
	VideoThumbnailLockTimeout time.Duration
	// VideoThumbnailConcurrency is the max number of concurrent video thumbnail generations (default: 4)
	VideoThumbnailConcurrency uint
	// HLSMaxSegments, HLSMaxTotalBytes and HLSConcurrency bound one HLS
	// download. They refuse rather than truncate: half a video delivered as a
	// success is worse than a refusal that says why. Zero means the hls
	// package's own default.
	HLSMaxSegments   int
	HLSMaxTotalBytes int64
	HLSConcurrency   int
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
	// HashAHashThreshold is the max AHash Hamming distance for the secondary
	// similarity check. 0 disables.
	HashAHashThreshold uint64
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
	// DownloadFailedRetention is how long a failed or cancelled download stays in
	// the persisted download history (default: one week).
	DownloadFailedRetention time.Duration
	// DownloadHistoryRetention is how long a completed download stays in the
	// persisted download history.
	DownloadHistoryRetention time.Duration
	// DownloadCockpitLimit is how many of the newest jobs the jobs panel renders.
	DownloadCockpitLimit int
	// PluginScheduleTick is how often the plugin scheduler looks for due work.
	// It bounds the resolution of every schedule.
	PluginScheduleTick time.Duration
	// MaxImportSize is the upper bound on import tar upload size in bytes
	MaxImportSize int64
	// MaxUploadSize is the upper bound on resource + version upload body size
	// in bytes. BH-034. 0 = unlimited.
	MaxUploadSize int64
	// MaxJSONBodySize bounds application/json request bodies. 0 = unlimited.
	MaxJSONBodySize int64
	// MaxActionEntities bounds the entities one plugin-action run may name.
	// 0 selects the default.
	MaxActionEntities int
	// MaxUserTokens caps how many API tokens a single user may hold. 0 = unlimited.
	MaxUserTokens int
	// MRQLDefaultLimit is the default LIMIT applied to MRQL queries without an
	// explicit LIMIT clause (default: 500).
	MRQLDefaultLimit int
	// MRQLPageQueryBudget caps how many distinct MRQL queries a single page
	// render may execute via inline [mrql] shortcodes (default: 200, 0 disables).
	MRQLPageQueryBudget int
	// MRQLQueryTimeoutBoot is the boot-time default for the MRQL query timeout.
	// Runtime overrides live in RuntimeSettings.
	MRQLQueryTimeoutBoot time.Duration
	// DeepSeekAPIKey enables MRQL natural-language generation when configured.
	// It is intentionally env-only and must not be exposed through runtime settings.
	DeepSeekAPIKey string
	// DeepSeekModel is the chat model used for MRQL generation.
	DeepSeekModel string
	// DeepSeekTimeout bounds a single MRQL generation provider call.
	DeepSeekTimeout time.Duration
	// TemplateSigningKey is an optional operator-supplied secret (env
	// TEMPLATE_SIGNING_KEY) for the [lazy]/[details] deferred-render token HMAC
	// key. Empty → per-boot random key. Env-only. See MahresourcesConfig.
	TemplateSigningKey string

	// AuthEnabled turns on user accounts + RBAC (env: AUTH_ENABLED=1).
	AuthEnabled bool
	// SessionTTL is how long a browser login session stays valid.
	SessionTTL time.Duration
	// SessionCookieSecure marks the session cookie Secure (HTTPS-only).
	SessionCookieSecure bool
	// LoginRateLimit caps failed login attempts per client IP within
	// LoginRateWindow before throttling (0 disables it).
	LoginRateLimit int
	// LoginRateWindow is the sliding window for LoginRateLimit.
	LoginRateWindow time.Duration
	// TrustProxyHeaders trusts X-Forwarded-For for the client IP (login
	// rate-limiting). Off by default; enable only behind a trusted proxy.
	TrustProxyHeaders bool
	// CreateAdminUser/CreateAdminPassword bootstrap an admin account at startup
	// when set. Idempotent: an existing account with that username is reset to an
	// enabled admin with the given password.
	CreateAdminUser     string
	CreateAdminPassword string
}

type MahresourcesLocks struct {
	ThumbnailGenerationLock      *idlock.Lock[uint]
	VideoThumbnailGenerationLock *idlock.Lock[uint]
	OfficeDocumentGenerationLock *idlock.Lock[uint]
	ResourceHashLock             *idlock.Lock[string]
	VersionUploadLock            *idlock.Lock[uint]
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
	// groupio owns group import/export. Safe as a field because it holds only
	// filesystems, never a db handle — see groupioDeps() in groupio_facade.go.
	// It shares the altFileSystems map object, so RegisterAltFs (which mutates
	// in place and never reassigns) stays visible to it.
	groupio *groupio.Service
	locks   MahresourcesLocks
	// downloadManager handles background remote URL downloads
	downloadManager *download_queue.DownloadManager
	// search owns global search: the cross-entity query, the LIKE/FTS backends,
	// and the process-wide result cache. Safe as a field because it holds no db
	// handle -- see searchDeps() in search_facade.go. It is shared by every
	// derived context, which is what keeps InitFTS state and cache
	// invalidation visible across them.
	search *search.Service
	// currentRequest holds the HTTP request this context is serving, set by
	// WithRequest(). It is the request metadata the logger stamps, and it is
	// also this context's answer to "who is the caller and are they still
	// there": callerContext() reads r.Context() off it so a hook wait can be
	// abandoned when the client goes away.
	currentRequest *http.Request
	// principal is the authenticated identity for this (request-scoped) context.
	// Set by WithRequest/WithPrincipal. nil on the singleton context, which is
	// treated as an unrestricted "system" caller (background workers, migrations).
	principal *auth.Principal
	// pluginInvocation is set only on a context produced by the plugin DB
	// adapter's BindInvocation: it means "this work originates inside a plugin
	// VM call". It is opaque here — application_context stores it and hands it
	// back to plugin_system when dispatching hooks, so that a hook dispatch can
	// refuse to re-enter a VM already executing on this call chain. nil
	// everywhere else, including ordinary requests.
	pluginInvocation *plugin_system.Invocation

	// pluginEgress is the network policy of the plugin whose call this context
	// was bound for, when that call can reach the network. It is nil on the
	// process-wide context and on every operator-initiated path, which is what
	// keeps /v1/resource and /v1/resource/remote on their existing unrestricted
	// downloader while a plugin's fetch is policed.
	pluginEgress *plugin_system.NetworkPolicy

	// hostFetchPolicy polices the application's OWN outbound fetches: the
	// operator-initiated ones that carry no plugin policy at all. Unlike
	// pluginEgress it is set on every context, because its whole purpose is to
	// be the answer when no plugin is involved. Public hosts are unrestricted;
	// a private address is refused unless -allow-private-fetch names it.
	hostFetchPolicy plugin_system.NetworkPolicy

	// pluginFetch marks a context bound for a plugin invocation that may fetch.
	// Without it, AddRemoteResource cannot tell "no plugin is involved, use the
	// unrestricted operator path" from "a plugin is fetching but its policy did
	// not survive the trip" — and those must not both mean "skip the layers".
	pluginFetch bool

	// deferredHooks is set only on a context inside a plugin transaction
	// (RunInTransaction). While it is set, RunAfterPluginHooks queues rather
	// than dispatches: an after-hook announces a write, and inside an open
	// transaction the write has not happened yet and may never. A pointer, so
	// every clone this context makes — including the one BindInvocation builds
	// for another plugin's hook — appends to the same queue.
	deferredHooks *deferredPluginHooks

	// hashQueue is a channel to queue resources for async hash processing
	hashQueue chan<- uint
	// thumbnailQueue is a channel to queue video resources for async thumbnail generation
	thumbnailQueue chan<- uint
	// icsCache provides LRU caching for ICS calendar data
	icsCache *ICSCache
	// pluginManager manages Lua plugin loading and hook execution
	pluginManager *plugin_system.PluginManager
	// pluginScheduler owns the clock that fires plugin schedules, and is the only
	// thing that can run one on demand. It is installed by main after the
	// scheduler is constructed, the way the two worker queues above are, because
	// the scheduler is built *from* this context and cannot exist before it.
	//
	// Process-lifetime state, so a derived context (WithPrincipal, WithTransaction)
	// carries the same pointer. That is deliberate and is the whole reason a run
	// started from a request does not inherit that request: the scheduler holds
	// the singleton handle, so every run — ticked or manual — executes as the
	// schedule's own owner against an unscoped handle, and never as the caller.
	pluginScheduler *PluginScheduler
	// Narrow internal seams used to coordinate scope-integrity concurrency tests
	// and to record post-commit group-delete effects. Derived contexts share them.
	scopeLockBarrier      func(operation string, groupID uint)
	identityReuseBarrier  func(operation string, expected *models.User)
	groupDeleteEffectSink groupDeleteEffectSink
	// DefaultResourceCategoryID is the resolved ID of the default resource category.
	// Set at startup; used as the fallback when no category is specified and as the
	// reassignment target when a category is deleted.
	DefaultResourceCategoryID uint
	// settings is the runtime-settings service. Installed after AutoMigrate via SetSettings.
	// scopedAccess caches which plugins group-limited principals may reach. A
	// pointer, so every shallow clone of this context shares the one cache.
	scopedAccess *scopedPluginAccess

	settings *RuntimeSettings
	// exportSweepFs is the filesystem rooted at FileSavePath used by the
	// startup export/import sweep. Captured at NewMahresourcesContext so
	// main.go can trigger the sweep after the DownloadManager has been
	// swapped over to the live RuntimeSettings provider.
	exportSweepFs afero.Fs
	// mrqlGenerator converts natural-language prompts into locally validated MRQL drafts.
	mrqlGenerator MRQLGenerator
	// mrqlGenerationLimiter rate-limits provider calls per caller key.
	mrqlGenerationLimiter *MRQLGenerationRateLimiter
	// templateGenerator converts natural-language prompts into category template sections.
	templateGenerator TemplateGenerator
	// templateGenerationLimiter rate-limits template generation calls per caller key
	// (a separate bucket from MRQL generation).
	templateGenerationLimiter *MRQLGenerationRateLimiter
	// rootAdmin caches a snapshot of the oldest enabled admin (the "root" user).
	// It is a pointer so the shallow copies made by WithRequest/WithPrincipal/
	// WithTransaction all share the same live cache. Read on the hot create path
	// via defaultActorID() (a pure atomic load); refreshed synchronously after
	// every user mutation. See root_admin.go.
	rootAdmin *rootAdminCache
	// deferredSigningKey is the HMAC key that authenticates the tokens backing the
	// [lazy]/[details] deferred-render shortcodes (lib/deferredtoken). Derived from
	// Config.TemplateSigningKey when set, otherwise a per-boot random 32 bytes.
	deferredSigningKey []byte
	// shareServerListening records that the public share server bound its port and
	// has not stopped serving. Finding 51: a bind failure was logged and swallowed,
	// so /admin/settings went on advertising the share port and the note sidebar
	// went on minting tokens for URLs nothing would ever answer. A pointer for the
	// same reason as rootAdmin — WithRequest/WithPrincipal/WithTransaction
	// shallow-copy the struct, and every copy has to see the same flag.
	//
	// It is a positive fact, not the absence of a negative one. Batch 11 spelled
	// this as `shareServerFailed`, which starts false — so a context whose share
	// server was never started at all reported "no failure observed" and
	// ShareEnabled() answered true for a port nothing was listening on. That is
	// finding 7 again, through a different door: main.go's boot path is not the
	// only caller of CreateServer, and the endpoint would happily mint tokens for
	// a `/s/` route no process serves.
	shareServerListening *atomic.Bool
}

// MarkShareServerListening records that the share server bound its port and is
// serving. Called by server.ShareServer.Start once net.Listen has succeeded, and
// by test setups that stand in for it.
func (ctx *MahresourcesContext) MarkShareServerListening() {
	if ctx != nil && ctx.shareServerListening != nil {
		ctx.shareServerListening.Store(true)
	}
}

// MarkShareServerFailed records that the share server is not serving. Called by
// server.ShareServer on a bind failure and if Serve ever returns unexpectedly.
func (ctx *MahresourcesContext) MarkShareServerFailed() {
	if ctx != nil && ctx.shareServerListening != nil {
		ctx.shareServerListening.Store(false)
	}
}

// MarkShareServerStopped records that the share server was shut down deliberately.
// It writes the same fact as MarkShareServerFailed — nothing is listening — and
// exists so that server.ShareServer.Stop does not have to say "failed" about an
// orderly shutdown. Round 3, finding 3: Stop used to write nothing at all, so a
// stopped share server went on being advertised.
func (ctx *MahresourcesContext) MarkShareServerStopped() {
	if ctx != nil && ctx.shareServerListening != nil {
		ctx.shareServerListening.Store(false)
	}
}

// ShareServerListening reports whether a share server is known to be serving.
func (ctx *MahresourcesContext) ShareServerListening() bool {
	return ctx != nil && ctx.shareServerListening != nil && ctx.shareServerListening.Load()
}

// DeferredSigningKey returns the HMAC key used to sign and verify deferred-render
// tokens for the [lazy]/[details] shortcodes. It is never nil on a context built
// by NewMahresourcesContext.
func (ctx *MahresourcesContext) DeferredSigningKey() []byte {
	return ctx.deferredSigningKey
}

// RunStartupExportSweep cleans up orphaned export/import tars left over from a
// previous run. Separated from NewMahresourcesContext so main.go can call it
// AFTER SetSettings + DownloadManager.SetSettings, ensuring the first-pass
// retention value reflects any persisted runtime override rather than the boot
// flag. Safe to call on a nil or unconfigured context.
func (ctx *MahresourcesContext) RunStartupExportSweep() {
	if ctx == nil || ctx.exportSweepFs == nil || ctx.downloadManager == nil {
		return
	}
	retention := ctx.downloadManager.ExportRetention()
	if removed, err := download_queue.SweepOrphanedExports(ctx.exportSweepFs, "_exports", retention); err != nil {
		log.Printf("warning: SweepOrphanedExports failed: %v", err)
	} else if removed > 0 {
		log.Printf("startup: removed %d orphaned export tars", removed)
	}
	if removed, err := download_queue.SweepOrphanedExports(ctx.exportSweepFs, "_imports", retention); err != nil {
		log.Printf("warning: sweep _imports failed: %v", err)
	} else if removed > 0 {
		log.Printf("startup: removed %d orphaned import files", removed)
	}
}

// deriveDeferredSigningKey returns the HMAC key for deferred-render tokens. When
// an operator configures TemplateSigningKey (env TEMPLATE_SIGNING_KEY) it is
// hashed to a fixed 32 bytes so any-length passphrase works and the same value
// yields the same key across processes (needed for multi-process / behind-LB
// deployments where a lazy-reveal may hit a different process than the page
// render). Otherwise a per-boot cryptographically random key is generated, which
// is correct for the common single-process deployment.
func deriveDeferredSigningKey(configured string) []byte {
	if configured != "" {
		sum := sha256.Sum256([]byte(configured))
		return sum[:]
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		panic(err) // crypto/rand should never fail
	}
	return key
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, readOnlyDB *sqlx.DB, config *MahresourcesConfig) *MahresourcesContext {
	altFileSystems := make(map[string]afero.Fs, len(config.AltFileSystems))

	for key, path := range config.AltFileSystems {
		altFileSystems[key] = storage.CreateStorage(path)
	}

	// Built here rather than taken pre-built from the config so that every
	// construction path gets one — tests and the e2e harness included — and so
	// the zero value of MahresourcesConfig means "deny every private address"
	// rather than "deny everything" or "allow everything".
	//
	// main.go validates the same entries at flag-parse time and refuses to
	// start on a bad one, so an error here is unreachable in a real deployment.
	// It still fails closed rather than propagating: a policy that could not be
	// built is not a reason to fetch anywhere.
	hostFetchPolicy, hostFetchErr := plugin_system.HostFetchPolicy(config.AllowPrivateFetch)
	if hostFetchErr != nil {
		log.Printf("[egress] WARNING: -allow-private-fetch could not be parsed (%v); no private address will be reachable", hostFetchErr)
		hostFetchPolicy, _ = plugin_system.HostFetchPolicy(nil)
	}

	thumbnailGenerationLock := idlock.New[uint](uint(0), nil)
	videoThumbConcurrency := config.VideoThumbnailConcurrency
	if videoThumbConcurrency == 0 {
		videoThumbConcurrency = 4
	}
	videoThumbnailGenerationLock := idlock.New[uint](videoThumbConcurrency, nil)
	officeDocumentGenerationLock := idlock.New[uint](uint(2), nil)
	resourceHashLock := idlock.New[string](uint(0), nil)
	versionUploadLock := idlock.New[uint](uint(0), nil)

	// Initialize search cache with 60 second TTL and 1000 max entries
	searchCache := search.NewSearchCache(60*time.Second, 1000)

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
		StartedAt:       time.Now(),
		scopedAccess:    &scopedPluginAccess{},
		fs:              filesystem,
		db:              db,
		readOnlyDB:      readOnlyDB,
		Config:          config,
		hostFetchPolicy: hostFetchPolicy,
		altFileSystems:  altFileSystems,
		groupio:         groupio.NewService(filesystem, altFileSystems),
		locks: MahresourcesLocks{
			ThumbnailGenerationLock:      thumbnailGenerationLock,
			VideoThumbnailGenerationLock: videoThumbnailGenerationLock,
			OfficeDocumentGenerationLock: officeDocumentGenerationLock,
			ResourceHashLock:             resourceHashLock,
			VersionUploadLock:            versionUploadLock,
		},
		search:                    search.NewService(searchCache, config.DbType),
		icsCache:                  icsCache,
		DefaultResourceCategoryID: 1,
		rootAdmin:                 newRootAdminCache(),
		deferredSigningKey:        deriveDeferredSigningKey(config.TemplateSigningKey),
		shareServerListening:      &atomic.Bool{},
	}

	// Install RBAC group-subtree scoping + CreatedByUserId stamping callbacks.
	// Registered here (after the ctx struct — including its rootAdmin cache — is
	// built) so the stamp callback's closure over ctx can call defaultActorID().
	// The scope callbacks are no-ops unless a query runs on a db whose context
	// carries a scope filter/actor (see scoping.go).
	registerScopeCallbacks(ctx)

	// Initialize download manager. A static settings provider seeded from the
	// boot config is used here; main.go swaps it for the live RuntimeSettings
	// provider via SetSettings after wiring is complete so that runtime
	// overrides take effect per download.
	ctx.downloadManager = download_queue.NewDownloadManagerWithConfig(
		ctx,
		download_queue.NewStaticDownloadSettings(download_queue.TimeoutConfig{
			ConnectTimeout: config.RemoteResourceConnectTimeout,
			IdleTimeout:    config.RemoteResourceIdleTimeout,
			OverallTimeout: config.RemoteResourceOverallTimeout,
		}, config.ExportRetention),
		download_queue.ManagerConfig{
			Concurrency: config.MaxJobConcurrency,
			// The queue assembles an HLS playlist the way the synchronous path
			// does, so it needs the same two things: where ffmpeg is, and the
			// deployment's limits.
			// Read per download rather than captured: startup auto-detects
			// ffmpeg after this context is built, so a value taken here is the
			// empty one from before the detection.
			FfmpegPath: func() string { return ctx.Config.FfmpegPath },
			HLSOptions: hlsOptionsFromConfig(config),
			// The host allowlist, applied to every URL a playlist names as well
			// as to the one submitted.
			HostCheckURL: func(u string) error {
				return plugin_system.CheckEgressURL(hostFetchPolicy, u)
			},
			// The queue fetches a user-supplied URL on a background worker with
			// no request and no principal, so it is the operator fetch path
			// with the least context of its own. It gets the same host policy
			// as the synchronous one.
			//
			// The connect timeout arrives per call: the decoration replaces the
			// dialler, and the queue reads that timeout from live settings on
			// every download. Baking in the boot value here would have left the
			// runtime setting applying to two of a transport's three timeouts.
			ClientPolicy: func(client *http.Client, connectTimeout time.Duration) *http.Client {
				return plugin_system.ApplyEgressPolicy(client, hostFetchPolicy, connectTimeout)
			},
			// Sanitize for the submitter, and log the unsanitized error here —
			// download_queue has no logger, and a refusal nobody can diagnose is
			// the cost of hiding the resolved address from the person who asked.
			RefusalMessage: func(url string, err error) (string, bool) {
				msg, blocked := plugin_system.HostFetchRefusal(err)
				if blocked {
					ctx.Logger().Warning(models.LogActionCreate, "resource", nil, "Download refused",
						fmt.Sprintf("%s: %s", url, err.Error()), nil)
				}
				return msg, blocked
			},
		},
	)

	// Wire periodic export-tar sweep into the manager's cleanup loop so tars
	// are purged every 5 minutes, not only at startup. Retention is read
	// through ctx.downloadManager.ExportRetention() on every sweep so runtime
	// overrides of export_retention take effect without a process restart.
	exportFs := filesystem
	ctx.exportSweepFs = exportFs
	ctx.downloadManager.SetExportSweepFn(func() {
		n, err := download_queue.SweepOrphanedExports(exportFs, "_exports", ctx.downloadManager.ExportRetention())
		if err != nil {
			log.Printf("warning: periodic SweepOrphanedExports failed: %v", err)
		} else if n > 0 {
			log.Printf("periodic sweep: removed %d expired export tars", n)
		}
	})

	// Terminal downloads are persisted so a failure outlives the in-memory queue's
	// eviction cap, its one-hour sweep, and the process itself. The recorder is the
	// context; the sweep runs on the same cleanup ticker as the export sweep and
	// reads its retention windows from the live settings on every call.
	ctx.downloadManager.SetHistoryRecorder(ctx)
	ctx.downloadManager.SetHistoryLogger(historyLogger{})
	ctx.downloadManager.SetHistorySweepFn(func() {
		if n, err := ctx.SweepDownloadHistory(); err != nil {
			log.Printf("warning: download history sweep failed: %v", err)
		} else if n > 0 {
			log.Printf("periodic sweep: removed %d expired download history rows", n)
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
			pm.SetPrincipalBinder(adapter)
			pm.SetPluginLogger(adapter)
			pm.SetKVStore(adapter)
			// mah.download.submit. The context implements the seam directly
			// rather than through the adapter: what it needs is the queue and
			// the calling plugin's name, neither of which is db state.
			pm.SetDownloadSubmitter(ctx)
			// Without this the manager falls back to its in-memory consent
			// store, which forgets every decision on restart.
			pm.SetConsentStore(&pluginConsentStore{ctx: ctx})
			mrqlAdapter := &pluginMRQLAdapter{ctx: ctx}
			pm.SetMRQLExecutor(mrqlAdapter)

			// A download a plugin submitted runs under that plugin's own
			// network list, not the host policy. Wired here rather than through
			// ManagerConfig because the manager is built while this context is
			// still assembling itself, before pm exists — a closure captured
			// there would capture nil. Unset, the manager refuses plugin
			// downloads rather than fetching them unpoliced, which is why this
			// is the only thing that may set it.
			if ctx.downloadManager != nil {
				ctx.downloadManager.SetPolicyResolver(func(pluginName string) (download_queue.EgressPolicy, bool) {
					policy, ok := pm.NetworkPolicyForPlugin(pluginName)
					if !ok {
						return download_queue.EgressPolicy{}, false
					}
					return download_queue.EgressPolicy{
						Decorate: func(client *http.Client, connectTimeout time.Duration) *http.Client {
							return plugin_system.ApplyEgressPolicy(client, policy, connectTimeout)
						},
						// The allowlist half. Without it a playlist a plugin
						// fetched could name segments on any public host, since
						// the decoration polices addresses rather than names.
						CheckURL: func(u string) error {
							return plugin_system.CheckEgressURL(policy, u)
						},
					}, true
				})
			}
		}
	}

	return ctx
}

// PluginManager returns the plugin manager, or nil if plugins are disabled.
func (ctx *MahresourcesContext) PluginManager() *plugin_system.PluginManager {
	return ctx.pluginManager
}

// ActionEntityRefReader returns an EntityRefReader bound to this context.
// Used by GetActionRunHandler to validate entity_ref param IDs.
func (ctx *MahresourcesContext) ActionEntityRefReader() plugin_system.EntityRefReader {
	return NewActionEntityRefReader(ctx)
}

// ActionEntityDataReader returns an ActionEntityDataReader bound to this
// context. Used by GetActionRunHandler to re-check an action's Filters against
// the entities it was asked to run on.
func (ctx *MahresourcesContext) ActionEntityDataReader() plugin_system.ActionEntityDataReader {
	return NewActionEntityDataReader(ctx)
}

// RegisterAltFs adds an alternative filesystem under the given key. This is
// used at startup (via NewMahresourcesContext from config) and in tests that
// need to inject an in-memory alt-fs without going through disk paths.
func (ctx *MahresourcesContext) RegisterAltFs(key string, fs afero.Fs) {
	ctx.altFileSystems[key] = fs
}

// hookInvocation describes, for a hook dispatch, who triggered the write and
// which plugin VMs are already running on this call chain.
//
// It reads both off the receiver, so it is uniform across the two origins and
// neither caller has to know which one it is in. An ordinary request carries the
// requesting principal and no invocation. A plugin-originated write runs on the
// clone that pluginDBAdapter.BindInvocation produced, whose principal *is* the
// actor and whose pluginInvocation is the chain so far.
func (ctx *MahresourcesContext) hookInvocation() *plugin_system.Invocation {
	if ctx.pluginInvocation != nil {
		return ctx.pluginInvocation
	}
	// SuperUser yields no actor, matching plugin_system.actorFromContext and
	// principalOwnerID in the HTTP layer: with auth off every request is the
	// same implicit administrator, so a hook that starts a job must leave it
	// ownerless rather than claiming root submitted it.
	var actor uint
	if ctx.principal != nil && !ctx.principal.SuperUser {
		actor = ctx.principal.UserID
	}
	return plugin_system.NewInvocation(actor)
}

// callerContext is the context of whoever made the call that raised a hook.
//
// Read off the receiver rather than handed in, which is what keeps the ~35 sites
// that raise a hook out of this change: none of them takes a context.
//
// The request first, because a write made over HTTP is the case the whole rule
// is about and the request is what ends when the client hangs up. The db handle
// second, for a caller that bound its context there instead (WithMRQLPrincipal
// does): the same place visibleGroupIDs reads the subtree filter back from.
// Background when there is neither: a worker, a startup seed, the singleton.
//
// The order is not a preference between two equivalent sources, it is the only
// one that works. applyPrincipalScope parents every request-scoped handle on
// context.Background() deliberately, so that a client hanging up does not tear
// the write's own SQL out mid-statement, and that decision stands. The handle
// therefore never carries the request's cancellation and asking it alone was
// asking the one place guaranteed not to know. Reading the request directly
// gives the hook wait the caller's lifetime while the write's SQL keeps the
// detached one it has always had.
func (ctx *MahresourcesContext) callerContext() context.Context {
	// (*http.Request).Context never returns nil; it defaults to Background.
	if ctx.currentRequest != nil {
		return ctx.currentRequest.Context()
	}
	if ctx.db == nil || ctx.db.Statement == nil || ctx.db.Statement.Context == nil {
		return context.Background()
	}
	return ctx.db.Statement.Context
}

// RunBeforePluginHooks executes before-hooks for the given event.
// If no plugin manager is active, data is returned unmodified.
//
// The caller's context bounds the wait for a busy plugin's VM. A before-hook
// runs before the write it can veto, so giving up on the wait fails the write,
// and failing a write whose client has already gone is safe: it is the same
// answer ErrHookVMBusy gives when a nested dispatch's bound expires, and nobody
// is left believing the write happened. RunAfterPluginHooks passes no context
// and must not; see plugin_system.RunAfterHooks.
func (ctx *MahresourcesContext) RunBeforePluginHooks(event string, data map[string]any) (map[string]any, error) {
	if ctx.pluginManager == nil {
		return data, nil
	}
	return ctx.pluginManager.RunBeforeHooks(ctx.callerContext(), ctx.hookInvocation(), event, data)
}

// RunAfterPluginHooks executes after-hooks for the given event.
// Errors are logged and ignored; execution is synchronous.
// If no plugin manager is active, this is a no-op.
func (ctx *MahresourcesContext) RunAfterPluginHooks(event string, data map[string]any) {
	if ctx.pluginManager == nil {
		return
	}
	// Inside a plugin transaction, queue instead. See deferredPluginHooks: an
	// after-hook says a write happened, and here it has not committed yet.
	// Queued before the manager check would be wrong the other way — with no
	// manager there is nothing to dispatch to, ever.
	if ctx.deferredHooks != nil {
		ctx.deferredHooks.add(ctx.hookInvocation(), event, data)
		return
	}
	ctx.pluginManager.RunAfterHooks(ctx.hookInvocation(), event, data)
}

// DownloadManager returns the download queue manager for background remote downloads
func (ctx *MahresourcesContext) DownloadManager() *download_queue.DownloadManager {
	return ctx.downloadManager
}

// Settings returns the runtime-settings service. Panics if called before wiring.
func (ctx *MahresourcesContext) Settings() *RuntimeSettings {
	if ctx.settings == nil {
		panic("MahresourcesContext.Settings() called before wiring")
	}
	return ctx.settings
}

// SetSettings installs the runtime-settings service. Called once from main.go
// after AutoMigrate. Must happen before workers that read through Settings start.
func (ctx *MahresourcesContext) SetSettings(rs *RuntimeSettings) {
	ctx.settings = rs
}

// MRQLGenerator returns the configured natural-language MRQL generator.
func (ctx *MahresourcesContext) MRQLGenerator() MRQLGenerator {
	return ctx.mrqlGenerator
}

// SetMRQLGenerator installs the natural-language MRQL generator.
func (ctx *MahresourcesContext) SetMRQLGenerator(generator MRQLGenerator) {
	ctx.mrqlGenerator = generator
}

// MRQLGenerationRateLimiter returns the configured per-caller generation limiter.
func (ctx *MahresourcesContext) MRQLGenerationRateLimiter() *MRQLGenerationRateLimiter {
	if ctx.mrqlGenerationLimiter == nil {
		ctx.mrqlGenerationLimiter = NewMRQLGenerationRateLimiter(10, time.Minute)
	}
	return ctx.mrqlGenerationLimiter
}

// SetMRQLGenerationRateLimiter installs the per-caller generation limiter.
func (ctx *MahresourcesContext) SetMRQLGenerationRateLimiter(l *MRQLGenerationRateLimiter) {
	ctx.mrqlGenerationLimiter = l
}

// TemplateGenerator returns the configured natural-language template generator.
func (ctx *MahresourcesContext) TemplateGenerator() TemplateGenerator {
	return ctx.templateGenerator
}

// SetTemplateGenerator installs the natural-language template generator.
func (ctx *MahresourcesContext) SetTemplateGenerator(generator TemplateGenerator) {
	ctx.templateGenerator = generator
}

// TemplateGenerationRateLimiter returns the per-caller template generation limiter.
func (ctx *MahresourcesContext) TemplateGenerationRateLimiter() *MRQLGenerationRateLimiter {
	if ctx.templateGenerationLimiter == nil {
		ctx.templateGenerationLimiter = NewMRQLGenerationRateLimiter(10, time.Minute)
	}
	return ctx.templateGenerationLimiter
}

// SetTemplateGenerationRateLimiter installs the per-caller template generation limiter.
func (ctx *MahresourcesContext) SetTemplateGenerationRateLimiter(l *MRQLGenerationRateLimiter) {
	ctx.templateGenerationLimiter = l
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
// Implements contracts.RequestContextSetter.
func (ctx *MahresourcesContext) WithRequest(r *http.Request) any {
	// Create a shallow copy to avoid modifying the original
	ctxCopy := *ctx
	ctxCopy.currentRequest = r
	// Carry the authenticated principal and apply group-subtree scoping so that
	// write handlers (which already route through WithRequest) are confined to a
	// group-limited principal's subtree.
	p := auth.PrincipalFromContext(r.Context())
	// If this context is already scoped to the same principal (e.g. it came from
	// scopedAPI's WithPrincipal for this request), its db is already confined to
	// the subtree — reuse it instead of resolving the subtree (and walking the
	// recursive group-tree CTE) a second time for the same request.
	if ctx.principal != nil && ctx.principal == p {
		return &ctxCopy
	}
	ctxCopy.principal = p
	applyPrincipalScope(&ctxCopy, ctx, p)
	return &ctxCopy
}

// WithActorUserID returns a ResourceCreator that stamps CreatedByUserId with the
// given user id (a background download's submitter) while running UNSCOPED. The
// download submit handlers validate scope targets at enqueue time, so the worker
// intentionally creates on the unscoped context; this only restores the creator
// attribution the singleton context would otherwise drop under auth-on. A
// principal carrying just the UserID applies no scope filter (it is neither
// scoped nor a scope-requiring role), so behaviour is unchanged apart from the
// stamp. Returns the receiver for id 0. Consumed via the download_queue
// actorResourceCreator capability.
func (ctx *MahresourcesContext) WithActorUserID(userID uint) download_queue.ResourceCreator {
	if userID == 0 {
		return ctx
	}
	return ctx.WithPrincipal(&auth.Principal{UserID: userID})
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

// SetPluginScheduler installs the running scheduler, so an operator can ask for
// a run outside the schedule's own cadence.
func (ctx *MahresourcesContext) SetPluginScheduler(scheduler *PluginScheduler) {
	ctx.pluginScheduler = scheduler
}

// SetJobEventSink installs the terminal-job observer on the download manager.
//
// Built in main and installed here for the same reason the scheduler is: it owns
// a goroutine, so the place that can defer its Stop is the place that starts it.
// A deployment that never calls this has no sink, and every emit in the queue is
// a nil check — which is what the CLI's and the tests' bare managers get.
func (ctx *MahresourcesContext) SetJobEventSink(sink download_queue.JobEventSink) {
	if ctx == nil || ctx.downloadManager == nil {
		return
	}
	ctx.downloadManager.SetJobEventSink(sink)
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

func metaKeys(ctx *MahresourcesContext, table string) ([]contracts.MetaKey, error) {
	var results []contracts.MetaKey

	// Group-subtree scoping is applied explicitly here rather than via the GORM
	// scope callbacks: the SQLite query's FROM clause is a multi-table
	// expression ("notes, json_each(notes.meta)") that the callback's table
	// matcher does not recognize, so a scoped principal would otherwise see the
	// distinct meta-key names of every entity. The Postgres branch's single-table
	// FROM is callback-eligible, but we filter both branches the same way so the
	// behaviour does not depend on GORM's statement-table parsing.
	ids, scoped, deny := ctx.subtreeScopeIDs()
	scopeCol, _ := scopeColumn(table)
	applyScope := func(q *gorm.DB) *gorm.DB {
		if deny {
			return q.Where("1 = 0")
		}
		if scoped {
			return q.Where(fmt.Sprintf("%v.%v IN ?", table, scopeCol), ids)
		}
		return q
	}

	if ctx.Config.DbType == constants.DbTypePosgres {
		q := applyScope(ctx.db.
			Table(table).
			Select("DISTINCT jsonb_object_keys(Meta) as Key").
			Where("Meta IS NOT NULL"))
		if err := q.Scan(&results).Error; err != nil {
			return nil, err
		}
	} else if ctx.Config.DbType == constants.DbTypeSqlite {
		q := applyScope(ctx.db.
			Table(fmt.Sprintf("%v, json_each(%v.meta)", table, table)).
			Select("DISTINCT json_each.key as Key").
			Where("Meta IS NOT NULL"))
		if err := q.Scan(&results).Error; err != nil {
			return nil, err
		}
	} else {
		results = make([]contracts.MetaKey, 0)
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

	var slowQueryLogger *models.SlowQueryLogger
	if connectedDB, slowLogger, err := models.CreateDatabaseConnection(dbType, dbDsn, cfg.DbLogFile, cfg.DbSlowQueryThreshold); err != nil {
		log.Fatal(err)
	} else {
		db = connectedDB
		slowQueryLogger = slowLogger
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

	// Apply default session TTL (30 days) when auth is enabled without one set.
	sessionTTL := cfg.SessionTTL
	if sessionTTL == 0 {
		sessionTTL = 30 * 24 * time.Hour
	}

	mahContext := NewMahresourcesContext(mainFs, db, readOnlyDb, &MahresourcesConfig{
		DbType:                       dbType,
		DbDsn:                        dbDsn,
		DbReadOnlyDsn:                readOnlyDsn,
		AltFileSystems:               cfg.AltFileSystems,
		FfmpegPath:                   cfg.FfmpegPath,
		LibreOfficePath:              cfg.LibreOfficePath,
		BindAddress:                  cfg.BindAddress,
		SharePort:                    cfg.SharePort,
		ShareBindAddress:             cfg.ShareBindAddress,
		SharePublicURL:               cfg.SharePublicURL,
		DocsSiteBaseURL:              cfg.DocsSiteBaseURL,
		DocsLinksDisabled:            cfg.DocsLinksDisabled,
		RemoteResourceConnectTimeout: connectTimeout,
		RemoteResourceIdleTimeout:    idleTimeout,
		RemoteResourceOverallTimeout: overallTimeout,
		AllowPrivateFetch:            cfg.AllowPrivateFetch,
		HLSMaxSegments:               cfg.HLSMaxSegments,
		HLSMaxTotalBytes:             cfg.HLSMaxTotalBytes,
		HLSConcurrency:               cfg.HLSConcurrency,
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
		HashAHashThreshold:           cfg.HashAHashThreshold,
		HashCacheSize:                cfg.HashCacheSize,
		EphemeralMode:                cfg.EphemeralMode,
		MemoryDB:                     cfg.MemoryDB,
		MemoryFS:                     cfg.MemoryFS,
		MaxDBConnections:             cfg.MaxDBConnections,
		FileSavePath:                 cfg.FileSavePath,
		SkipFTS:                      cfg.SkipFTS,
		MaxJobConcurrency:            cfg.MaxJobConcurrency,
		ExportRetention:              cfg.ExportRetention,
		DownloadFailedRetention:      cfg.DownloadFailedRetention,
		DownloadHistoryRetention:     cfg.DownloadHistoryRetention,
		DownloadCockpitLimit:         cfg.DownloadCockpitLimit,
		PluginScheduleTick:           cfg.PluginScheduleTick,
		MaxImportSize:                cfg.MaxImportSize,
		MaxUploadSize:                cfg.MaxUploadSize,
		MaxJSONBodySize:              cfg.MaxJSONBodySize,
		MaxActionEntities:            cfg.MaxActionEntities,
		MaxUserTokens:                cfg.MaxUserTokens,
		MRQLDefaultLimit:             cfg.MRQLDefaultLimit,
		MRQLPageQueryBudget:          cfg.MRQLPageQueryBudget,
		MRQLQueryTimeoutBoot:         cfg.MRQLQueryTimeoutBoot,
		DeepSeekAPIKey:               cfg.DeepSeekAPIKey,
		DeepSeekModel:                cfg.DeepSeekModel,
		DeepSeekTimeout:              cfg.DeepSeekTimeout,
		TemplateSigningKey:           cfg.TemplateSigningKey,
		AuthEnabled:                  cfg.AuthEnabled,
		SessionTTL:                   sessionTTL,
		SessionCookieName:            "mr_session",
		SessionCookieSecure:          cfg.SessionCookieSecure,
		LoginRateLimit:               cfg.LoginRateLimit,
		LoginRateWindow:              cfg.LoginRateWindow,
		TrustProxyHeaders:            cfg.TrustProxyHeaders,
	})

	// The slow-query logger exists before the context does, so its
	// application-log sink can only be attached now.
	if slowQueryLogger != nil {
		mahContext.StartSlowQueryLogSink(slowQueryLogger)
	}

	return mahContext, db, mainFs
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

	// time.ParseDuration errors on an empty/invalid value and returns 0,
	// which matches the disabled default.
	slowQueryThreshold, _ := time.ParseDuration(os.Getenv("DB_SLOW_QUERY_THRESHOLD"))

	return CreateContextWithConfig(&MahresourcesInputConfig{
		FileSavePath:         os.Getenv("FILE_SAVE_PATH"),
		DbType:               os.Getenv("DB_TYPE"),
		DbDsn:                os.Getenv("DB_DSN"),
		DbReadOnlyDsn:        os.Getenv("DB_READONLY_DSN"),
		DbLogFile:            os.Getenv("DB_LOG_FILE"),
		DbSlowQueryThreshold: slowQueryThreshold,
		BindAddress:          os.Getenv("BIND_ADDRESS"),
		FfmpegPath:           os.Getenv("FFMPEG_PATH"),
		AltFileSystems:       altFSystems,
		// Read here too, so this constructor's answer to "how many entities may
		// one action run name" is the operator's rather than the default. A
		// bad value reads as 0, which selects the default — the same shape
		// every other limit here uses.
		MaxActionEntities: maxActionEntitiesFromEnv(),
	})
}

// maxActionEntitiesFromEnv reads MAX_ACTION_ENTITIES for the deprecated
// environment-only constructor. main.go parses the same variable beside its
// flag; this exists because CreateContext never reaches that code.
func maxActionEntitiesFromEnv() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("MAX_ACTION_ENTITIES")))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
