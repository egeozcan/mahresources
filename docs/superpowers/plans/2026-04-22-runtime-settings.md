# Runtime Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let operators override 11 in-scope settings (upload/import sizes, timeouts, MRQL limits, share URL, hash thresholds, export retention) at runtime via a web UI, HTTP API, or `mr` CLI — without restarting the server.

**Architecture:** New `RuntimeSettings` service holds a DB-backed cache of per-key overrides with typed getters. `MahresourcesConfig` stays immutable after boot and becomes the source of boot defaults. Hot-path reads switch from `appContext.Config.X` to `appContext.Settings.X()`. Eight of the eleven settings need small local refactors to re-read per use; three are already per-use reads.

**Tech Stack:** Go 1.x, GORM, Gorilla Mux, Pongo2 templates, Alpine.js, Cobra (CLI), Playwright (E2E), Docusaurus (docs).

**Spec:** `docs/superpowers/specs/2026-04-22-runtime-settings-design.md`

---

## File Structure

**New files:**

- `models/runtime_setting_model.go` — GORM model
- `application_context/runtime_settings.go` — the service (struct, constructor, Load, Set, Reset, List, typed getters)
- `application_context/runtime_setting_spec.go` — `SettingSpec`, spec registry, value envelope encode/decode
- `application_context/runtime_settings_test.go` — unit tests
- `application_context/runtime_settings_boot_test.go` — boot conflict test
- `server/api_handlers/admin_settings_handlers.go` — HTTP handlers
- `server/api_tests/admin_settings_test.go` — Go API tests
- `server/template_handlers/template_context_providers/admin_settings_template_context.go` — template context provider
- `templates/adminSettings.tpl` — Pongo2 template
- `e2e/tests/admin-settings.spec.ts` — browser E2E
- `e2e/tests/accessibility/admin-settings.a11y.spec.ts` — a11y
- `e2e/tests/cli/admin-settings-list.spec.ts`, `admin-settings-set-reset.spec.ts`, `admin-settings-bounds.spec.ts` — CLI E2E
- `cmd/mr/commands/admin_help/admin_settings.md`, `admin_settings_list.md`, `admin_settings_get.md`, `admin_settings_set.md`, `admin_settings_reset.md`, `admin_stats.md` — CLI help
- `docs-site/docs/configuration/runtime-settings.md` — new user doc
- `docs-site/docs/cli/admin/index.md`, `docs-site/docs/cli/admin/stats.md`, `docs-site/docs/cli/admin/settings.md` — restructured CLI reference

**Modified files:**

- `main.go` — instantiate `RuntimeSettings`, register in `AutoMigrate`, remove `application_context.MRQLQueryTimeout = *mrqlTimeout`
- `application_context/context.go` — add `settings` field and `Settings()` accessor on `MahresourcesContext`
- `application_context/mrql_context.go` — remove `MRQLQueryTimeout` package var; update 5 callsites + `GetMRQLDefaultLimit` to read from `Settings`
- `download_queue/manager.go` — introduce `DownloadSettings` interface; replace `timeoutConfig` + `exportRetention` storage with a provider
- `download_queue/manager_test.go` — switch the two `NewDownloadManagerWithConfig` callsites to `NewStaticDownloadSettings` adapter
- `server/api_handlers/import_api_handlers.go` — change `GetImportParseHandler` signature from `int64` to `func() int64`
- `server/api_tests/import_api_test.go` — update the one callsite (line 81)
- `server/routes.go` — swap `Config.X` references for `Settings.X()` at callsites for `MaxUploadSize`, `MaxImportSize`, `ExportRetention` (lines 122-123, 411, 439, 546); add admin-settings routes; add `/admin/settings` template route
- `server/routes_openapi.go` — OpenAPI metadata for the three new admin endpoints
- `server/template_handlers/template_handler.go` (or wherever `/admin/*` registrations live) — register `adminSettings.tpl`
- `hash_worker/config.go` + `hash_worker/worker.go` — replace `SimilarityThreshold int` / `AHashThreshold uint64` with `SimilarityThresholdFn func() int` / `AHashThresholdFn func() uint64`; update `DefaultConfig` and `worker.go:480`
- `hash_worker/worker_test.go`, `worker_solid_color_test.go` — wrap literal thresholds in `func()` closures
- `cmd/mr/commands/admin.go` — restructure: current behaviour moves to `mr admin stats`; bare `mr admin` stays as alias; add `settings` subcommand tree
- `cmd/mr/root.go` (or wherever commands are registered) — register any new top-level commands
- `cmd/mr/commands/admin_help/admin.md` — becomes group overview (existing stats help moves to `admin_stats.md`)

---

## Phase 1 — Core Service

### Task 1: Value envelope encode/decode + spec registry

**Files:**
- Create: `application_context/runtime_setting_spec.go`
- Create: `application_context/runtime_setting_spec_test.go`

- [ ] **Step 1: Write failing tests for envelope encode/decode**

`application_context/runtime_setting_spec_test.go`:

```go
package application_context

import (
	"testing"
	"time"
)

func TestEnvelopeRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		in   any
		typ  string
	}{
		{"int64", int64(1 << 31), "int64"},
		{"int", int(500), "int"},
		{"uint64", uint64(42), "uint64"},
		{"duration", 2 * time.Hour, "duration"},
		{"string_empty", "", "string"},
		{"string_url", "https://example.com", "string"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			enc, err := encodeSettingValue(tc.typ, tc.in)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			got, err := decodeSettingValue(tc.typ, enc)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if got != tc.in {
				t.Fatalf("round-trip: got %v want %v", got, tc.in)
			}
		})
	}
}

func TestEnvelopeTypeMismatch(t *testing.T) {
	enc, _ := encodeSettingValue("int64", int64(1))
	if _, err := decodeSettingValue("string", enc); err == nil {
		t.Fatal("expected mismatch error, got nil")
	}
}

func TestEnvelopeDurationEncodedAsNanos(t *testing.T) {
	enc, err := encodeSettingValue("duration", 500*time.Millisecond)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	// Envelope payload should be the nanosecond count, i.e. 500_000_000.
	wantSubstr := `"value":500000000`
	if !contains(string(enc), wantSubstr) {
		t.Fatalf("duration envelope %q should contain %q", string(enc), wantSubstr)
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestEnvelope -v`
Expected: FAIL — `encodeSettingValue` / `decodeSettingValue` undefined.

- [ ] **Step 3: Implement envelope + registry skeleton**

`application_context/runtime_setting_spec.go`:

```go
package application_context

import (
	"encoding/json"
	"fmt"
	"time"
)

// SettingType discriminates value encoding.
type SettingType string

const (
	SettingTypeInt64    SettingType = "int64"
	SettingTypeInt      SettingType = "int"
	SettingTypeUint64   SettingType = "uint64"
	SettingTypeDuration SettingType = "duration"
	SettingTypeString   SettingType = "string"
)

// SettingGroup is the UI grouping label. Stable machine identifier.
type SettingGroup string

const (
	GroupUploads         SettingGroup = "uploads"
	GroupQueries         SettingGroup = "queries"
	GroupRemoteDownloads SettingGroup = "remote_downloads"
	GroupSharing         SettingGroup = "sharing"
	GroupDeduplication   SettingGroup = "deduplication"
	GroupExports         SettingGroup = "exports"
)

// SettingSpec carries type, display metadata, and validation bounds for one key.
type SettingSpec struct {
	Key         string
	Label       string
	Description string
	Group       SettingGroup
	Type        SettingType
	// Bounds interpretation depends on Type:
	//   int64, int, uint64, duration (nanos)  → MinNumeric / MaxNumeric inclusive
	//   string  → validated via StringValidator (nil = no validation)
	// AllowZero for numeric types lets 0 bypass MinNumeric (e.g. "unlimited").
	MinNumeric      int64
	MaxNumeric      int64
	AllowZero       bool
	StringValidator func(string) error
}

// envelope is the on-disk JSON shape stored in runtime_settings.value_json.
type envelope struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

func encodeSettingValue(typ string, v any) ([]byte, error) {
	var raw []byte
	var err error
	switch typ {
	case string(SettingTypeInt64):
		if cast, ok := v.(int64); ok {
			raw, err = json.Marshal(cast)
		} else {
			return nil, fmt.Errorf("encode: want int64, got %T", v)
		}
	case string(SettingTypeInt):
		if cast, ok := v.(int); ok {
			raw, err = json.Marshal(cast)
		} else {
			return nil, fmt.Errorf("encode: want int, got %T", v)
		}
	case string(SettingTypeUint64):
		if cast, ok := v.(uint64); ok {
			raw, err = json.Marshal(cast)
		} else {
			return nil, fmt.Errorf("encode: want uint64, got %T", v)
		}
	case string(SettingTypeDuration):
		if cast, ok := v.(time.Duration); ok {
			raw, err = json.Marshal(int64(cast))
		} else {
			return nil, fmt.Errorf("encode: want duration, got %T", v)
		}
	case string(SettingTypeString):
		if cast, ok := v.(string); ok {
			raw, err = json.Marshal(cast)
		} else {
			return nil, fmt.Errorf("encode: want string, got %T", v)
		}
	default:
		return nil, fmt.Errorf("encode: unknown type %q", typ)
	}
	if err != nil {
		return nil, err
	}
	return json.Marshal(envelope{Type: typ, Value: raw})
}

func decodeSettingValue(typ string, data []byte) (any, error) {
	var env envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("decode envelope: %w", err)
	}
	if env.Type != typ {
		return nil, fmt.Errorf("decode: type mismatch: stored=%q expected=%q", env.Type, typ)
	}
	switch typ {
	case string(SettingTypeInt64):
		var v int64
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, err
		}
		return v, nil
	case string(SettingTypeInt):
		var v int
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, err
		}
		return v, nil
	case string(SettingTypeUint64):
		var v uint64
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, err
		}
		return v, nil
	case string(SettingTypeDuration):
		var nanos int64
		if err := json.Unmarshal(env.Value, &nanos); err != nil {
			return nil, err
		}
		return time.Duration(nanos), nil
	case string(SettingTypeString):
		var v string
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
	return nil, fmt.Errorf("decode: unknown type %q", typ)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestEnvelope -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add application_context/runtime_setting_spec.go application_context/runtime_setting_spec_test.go
git commit -m "feat(settings): value envelope encode/decode + spec types"
```

---

### Task 2: RuntimeSetting model + spec registry

**Files:**
- Create: `models/runtime_setting_model.go`
- Modify: `main.go` (add to AutoMigrate list)
- Modify: `application_context/runtime_setting_spec.go` (add `buildSpecs` registry)

- [ ] **Step 1: Create the model**

`models/runtime_setting_model.go`:

```go
package models

import "time"

// RuntimeSetting stores a runtime override for one configuration key.
// Absence of a row means "no override; use the boot-time default."
type RuntimeSetting struct {
	Key       string    `gorm:"primaryKey;size:100" json:"key"`
	ValueJSON string    `gorm:"type:text;not null" json:"valueJson"`
	Reason    string    `gorm:"type:text" json:"reason,omitempty"`
	UpdatedAt time.Time `json:"updatedAt"`
}
```

- [ ] **Step 2: Wire into AutoMigrate**

Modify `main.go`. Inside the `db.AutoMigrate(...)` list (around line 310-334), add `&models.RuntimeSetting{}` right after `&models.PluginKV{}` (it has no FK dependencies, so it fits in the first group).

```go
&models.PluginKV{},
&models.RuntimeSetting{},
&models.SavedMRQLQuery{},
```

- [ ] **Step 3: Add `buildSpecs` registry**

Append to `application_context/runtime_setting_spec.go`:

```go
import "net/url"

// Stable machine keys — also the primary keys in runtime_settings.
const (
	KeyMaxUploadSize           = "max_upload_size"
	KeyMaxImportSize           = "max_import_size"
	KeyMRQLDefaultLimit        = "mrql_default_limit"
	KeyMRQLQueryTimeout        = "mrql_query_timeout"
	KeyExportRetention         = "export_retention"
	KeyRemoteConnectTimeout    = "remote_connect_timeout"
	KeyRemoteIdleTimeout       = "remote_idle_timeout"
	KeyRemoteOverallTimeout    = "remote_overall_timeout"
	KeySharePublicURL          = "share_public_url"
	KeyHashSimilarityThreshold = "hash_similarity_threshold"
	KeyHashAHashThreshold      = "hash_ahash_threshold"
)

// buildSpecs returns the registry of runtime-editable settings.
// Keep the list in display order; the admin UI groups by SettingGroup.
func buildSpecs() map[string]SettingSpec {
	return map[string]SettingSpec{
		KeyMaxUploadSize: {
			Key: KeyMaxUploadSize, Label: "Max upload size",
			Description: "Upper bound on resource and version upload body size in bytes. 0 = unlimited.",
			Group:       GroupUploads, Type: SettingTypeInt64,
			MinNumeric: 1024, MaxNumeric: 1 << 40, AllowZero: true,
		},
		KeyMaxImportSize: {
			Key: KeyMaxImportSize, Label: "Max import size",
			Description: "Upper bound on group-import tar upload size in bytes.",
			Group:       GroupUploads, Type: SettingTypeInt64,
			MinNumeric: 1 << 20, MaxNumeric: 1 << 40,
		},
		KeyMRQLDefaultLimit: {
			Key: KeyMRQLDefaultLimit, Label: "MRQL default LIMIT",
			Description: "Default LIMIT applied to MRQL queries without an explicit LIMIT clause.",
			Group:       GroupQueries, Type: SettingTypeInt,
			MinNumeric: 1, MaxNumeric: 100000,
		},
		KeyMRQLQueryTimeout: {
			Key: KeyMRQLQueryTimeout, Label: "MRQL query timeout",
			Description: "Maximum execution time for a single MRQL query.",
			Group:       GroupQueries, Type: SettingTypeDuration,
			MinNumeric: int64(100 * time.Millisecond), MaxNumeric: int64(5 * time.Minute),
		},
		KeyExportRetention: {
			Key: KeyExportRetention, Label: "Export retention",
			Description: "How long completed group-export tars stay on disk before cleanup.",
			Group:       GroupExports, Type: SettingTypeDuration,
			MinNumeric: int64(time.Minute), MaxNumeric: int64(30 * 24 * time.Hour),
		},
		KeyRemoteConnectTimeout: {
			Key: KeyRemoteConnectTimeout, Label: "Remote connect timeout",
			Description: "Timeout for connecting to remote URLs (dial, TLS, response headers).",
			Group:       GroupRemoteDownloads, Type: SettingTypeDuration,
			MinNumeric: int64(time.Second), MaxNumeric: int64(10 * time.Minute),
		},
		KeyRemoteIdleTimeout: {
			Key: KeyRemoteIdleTimeout, Label: "Remote idle timeout",
			Description: "How long to wait before erroring if a remote server stops sending data.",
			Group:       GroupRemoteDownloads, Type: SettingTypeDuration,
			MinNumeric: int64(time.Second), MaxNumeric: int64(time.Hour),
		},
		KeyRemoteOverallTimeout: {
			Key: KeyRemoteOverallTimeout, Label: "Remote overall timeout",
			Description: "Maximum total time for a remote resource download.",
			Group:       GroupRemoteDownloads, Type: SettingTypeDuration,
			MinNumeric: int64(10 * time.Second), MaxNumeric: int64(24 * time.Hour),
		},
		KeySharePublicURL: {
			Key: KeySharePublicURL, Label: "Share public URL",
			Description: "Externally-routable base URL for shared notes (e.g. https://share.example.com). Empty = show /s/<token> path only.",
			Group:       GroupSharing, Type: SettingTypeString,
			StringValidator: validateSharePublicURL,
		},
		KeyHashSimilarityThreshold: {
			Key: KeyHashSimilarityThreshold, Label: "Hash similarity threshold",
			Description: "Maximum DHash Hamming distance to consider two resources similar.",
			Group:       GroupDeduplication, Type: SettingTypeInt,
			MinNumeric: 0, MaxNumeric: 64,
		},
		KeyHashAHashThreshold: {
			Key: KeyHashAHashThreshold, Label: "Hash aHash threshold",
			Description: "Max AHash Hamming distance for the secondary similarity check. 0 disables.",
			Group:       GroupDeduplication, Type: SettingTypeUint64,
			MinNumeric: 0, MaxNumeric: 64, AllowZero: true,
		},
	}
}

func validateSharePublicURL(s string) error {
	if s == "" {
		return nil
	}
	u, err := url.Parse(s)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme must be http or https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("host must not be empty")
	}
	return nil
}
```

- [ ] **Step 4: Add registry tests**

Append to `application_context/runtime_setting_spec_test.go`:

```go
func TestBuildSpecs_ElevenKeys(t *testing.T) {
	specs := buildSpecs()
	if len(specs) != 11 {
		t.Fatalf("want 11 specs, got %d", len(specs))
	}
	expected := []string{
		KeyMaxUploadSize, KeyMaxImportSize, KeyMRQLDefaultLimit, KeyMRQLQueryTimeout,
		KeyExportRetention, KeyRemoteConnectTimeout, KeyRemoteIdleTimeout, KeyRemoteOverallTimeout,
		KeySharePublicURL, KeyHashSimilarityThreshold, KeyHashAHashThreshold,
	}
	for _, k := range expected {
		if _, ok := specs[k]; !ok {
			t.Errorf("missing spec for key %q", k)
		}
	}
}

func TestValidateSharePublicURL(t *testing.T) {
	ok := []string{"", "https://example.com", "http://example.com:8080/base"}
	bad := []string{"/relative", "no-scheme.example.com", "ftp://example.com", "http://", "https:///nohost"}
	for _, s := range ok {
		if err := validateSharePublicURL(s); err != nil {
			t.Errorf("want accept %q, got %v", s, err)
		}
	}
	for _, s := range bad {
		if err := validateSharePublicURL(s); err == nil {
			t.Errorf("want reject %q, got nil error", s)
		}
	}
}
```

- [ ] **Step 5: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run 'TestBuildSpecs|TestValidateSharePublicURL' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add models/runtime_setting_model.go main.go application_context/runtime_setting_spec.go application_context/runtime_setting_spec_test.go
git commit -m "feat(settings): runtime_setting model + spec registry"
```

---

### Task 3: RuntimeSettings service — Load / Set / Reset / List

**Files:**
- Create: `application_context/runtime_settings.go`
- Create: `application_context/runtime_settings_test.go`

- [ ] **Step 1: Write failing tests for service behavior**

`application_context/runtime_settings_test.go`:

```go
package application_context

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
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
	if len(views) != 11 {
		t.Fatalf("want 11 views, got %d", len(views))
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestRuntimeSettings -v`
Expected: FAIL — `NewRuntimeSettings` / `getRaw` / `Set` / etc. undefined.

- [ ] **Step 3: Implement the service**

`application_context/runtime_settings.go`:

```go
package application_context

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// SettingsLogger is the minimal logger surface RuntimeSettings needs.
// Production passes a thin adapter over log.Printf; tests pass a stub.
type SettingsLogger interface {
	Warn(format string, args ...any)
	Error(format string, args ...any)
}

// SettingView is the API/UI-facing representation of a single setting.
type SettingView struct {
	Key         string       `json:"key"`
	Label       string       `json:"label"`
	Description string       `json:"description"`
	Group       SettingGroup `json:"group"`
	Type        SettingType  `json:"type"`
	Current     any          `json:"current"`
	BootDefault any          `json:"bootDefault"`
	Overridden  bool         `json:"overridden"`
	UpdatedAt   *time.Time   `json:"updatedAt,omitempty"`
	Reason      string       `json:"reason,omitempty"`
	MinNumeric  *int64       `json:"minNumeric,omitempty"`
	MaxNumeric  *int64       `json:"maxNumeric,omitempty"`
	AllowZero   bool         `json:"allowZero,omitempty"`
}

type persistedEntry struct {
	value     any
	updatedAt time.Time
	reason    string
}

// RuntimeSettings holds a DB-backed cache of runtime-editable overrides.
// Boot-time defaults live in `defaults`; overrides in `overrides`.
// Reads take the RWMutex read-lock; writes take the write-lock.
type RuntimeSettings struct {
	db        *gorm.DB
	log       SettingsLogger
	specs     map[string]SettingSpec
	defaults  map[string]any
	mu        sync.RWMutex
	overrides map[string]persistedEntry
}

func NewRuntimeSettings(db *gorm.DB, log SettingsLogger, specs map[string]SettingSpec, defaults map[string]any) *RuntimeSettings {
	return &RuntimeSettings{
		db:        db,
		log:       log,
		specs:     specs,
		defaults:  defaults,
		overrides: make(map[string]persistedEntry),
	}
}

// Load reads persisted rows into the cache. Called once at startup.
// Emits one WARN per key whose override differs from the boot default, and
// one ERROR per row whose stored value fails the spec's bounds check (in
// which case the key is dropped from cache and the getter falls back to the
// boot default).
func (s *RuntimeSettings) Load() error {
	var rows []models.RuntimeSetting
	if err := s.db.Find(&rows).Error; err != nil {
		return fmt.Errorf("load: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range rows {
		spec, ok := s.specs[r.Key]
		if !ok {
			// Unknown key in DB (perhaps from a rolled-back deploy). Log and skip.
			s.log.Warn("runtime_settings: unknown key %q in DB, ignoring", r.Key)
			continue
		}
		v, err := decodeSettingValue(string(spec.Type), []byte(r.ValueJSON))
		if err != nil {
			s.log.Error("runtime_settings: decode %q: %v; falling back to boot default", r.Key, err)
			continue
		}
		if err := validateBounds(spec, v); err != nil {
			s.log.Error("runtime_settings: persisted %q fails bounds (%v); falling back to boot default", r.Key, err)
			continue
		}
		s.overrides[r.Key] = persistedEntry{value: v, updatedAt: r.UpdatedAt, reason: r.Reason}
		if bootDefault, ok := s.defaults[r.Key]; ok && bootDefault != v {
			s.log.Warn(`runtime_setting %q override (%v) differs from boot flag (%v)`, r.Key, v, bootDefault)
		}
	}
	return nil
}

// Set parses rawValue per the key's spec, validates bounds, writes the DB row,
// updates the cache, and returns the updated view. Errors leave cache + DB unchanged.
// Note: audit log_entries write happens in a later task (Task 7); this task
// focuses on the core Set/Reset/Load round-trip.
func (s *RuntimeSettings) Set(key, rawValue, reason, actor string) error {
	spec, ok := s.specs[key]
	if !ok {
		return fmt.Errorf("unknown setting %q", key)
	}
	v, err := parseSettingValue(spec, rawValue)
	if err != nil {
		return fmt.Errorf("parse %q: %w", key, err)
	}
	if err := validateBounds(spec, v); err != nil {
		return fmt.Errorf("validate %q: %w", key, err)
	}
	enc, err := encodeSettingValue(string(spec.Type), v)
	if err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	now := time.Now()
	row := models.RuntimeSetting{Key: key, ValueJSON: string(enc), Reason: reason, UpdatedAt: now}
	// GORM Save upserts by primary key.
	if err := s.db.Save(&row).Error; err != nil {
		return fmt.Errorf("db save: %w", err)
	}
	s.mu.Lock()
	s.overrides[key] = persistedEntry{value: v, updatedAt: now, reason: reason}
	s.mu.Unlock()
	_ = actor // used in Task 7 for log_entries audit
	return nil
}

// Reset removes the DB row and cache entry for the key.
func (s *RuntimeSettings) Reset(key, reason, actor string) error {
	if _, ok := s.specs[key]; !ok {
		return fmt.Errorf("unknown setting %q", key)
	}
	if err := s.db.Delete(&models.RuntimeSetting{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("db delete: %w", err)
	}
	s.mu.Lock()
	delete(s.overrides, key)
	s.mu.Unlock()
	_ = actor
	_ = reason
	return nil
}

// List returns a stable-order snapshot of all registered settings for the UI/API.
func (s *RuntimeSettings) List() []SettingView {
	s.mu.RLock()
	defer s.mu.RUnlock()
	views := make([]SettingView, 0, len(s.specs))
	// Stable display order: sort by (Group, Key).
	order := sortedSpecKeys(s.specs)
	for _, k := range order {
		spec := s.specs[k]
		view := SettingView{
			Key: spec.Key, Label: spec.Label, Description: spec.Description,
			Group: spec.Group, Type: spec.Type,
			BootDefault: s.defaults[k],
			Current:     s.defaults[k],
		}
		if spec.Type != SettingTypeString {
			view.MinNumeric = ptrInt64(spec.MinNumeric)
			view.MaxNumeric = ptrInt64(spec.MaxNumeric)
			view.AllowZero = spec.AllowZero
		}
		if entry, ok := s.overrides[k]; ok {
			view.Current = entry.value
			view.Overridden = true
			t := entry.updatedAt
			view.UpdatedAt = &t
			view.Reason = entry.reason
		}
		views = append(views, view)
	}
	return views
}

// getRaw is the internal accessor that typed getters in Task 4 use.
func (s *RuntimeSettings) getRaw(key string) (any, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if entry, ok := s.overrides[key]; ok {
		return entry.value, true
	}
	if d, ok := s.defaults[key]; ok {
		return d, true
	}
	return nil, false
}

// parseSettingValue turns the CLI/HTTP string form into the typed value.
// Accepts:
//   int64/int: decimal integer; byte suffixes (K, M, G, T; base 2) for size keys.
//   uint64:    decimal integer.
//   duration:  time.ParseDuration format (e.g. "30s", "5m", "2h").
//   string:    raw.
func parseSettingValue(spec SettingSpec, raw string) (any, error) {
	switch spec.Type {
	case SettingTypeInt64:
		n, err := parseIntWithByteSuffix(raw)
		if err != nil {
			return nil, err
		}
		return n, nil
	case SettingTypeInt:
		n, err := parseIntWithByteSuffix(raw)
		if err != nil {
			return nil, err
		}
		if n > int64(^uint(0)>>1) || n < -(int64(^uint(0)>>1)-1) {
			return nil, fmt.Errorf("value out of int range")
		}
		return int(n), nil
	case SettingTypeUint64:
		u, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse uint64: %w", err)
		}
		return u, nil
	case SettingTypeDuration:
		d, err := time.ParseDuration(raw)
		if err != nil {
			return nil, fmt.Errorf("parse duration: %w", err)
		}
		return d, nil
	case SettingTypeString:
		return raw, nil
	}
	return nil, fmt.Errorf("unknown type %q", spec.Type)
}

// parseIntWithByteSuffix accepts a decimal number with an optional K/M/G/T
// suffix (base 2: 1K = 1024). Plain integers also work. Negative values allowed.
func parseIntWithByteSuffix(raw string) (int64, error) {
	if len(raw) == 0 {
		return 0, fmt.Errorf("empty")
	}
	mult := int64(1)
	body := raw
	switch raw[len(raw)-1] {
	case 'K', 'k':
		mult, body = 1 << 10, raw[:len(raw)-1]
	case 'M', 'm':
		mult, body = 1 << 20, raw[:len(raw)-1]
	case 'G', 'g':
		mult, body = 1 << 30, raw[:len(raw)-1]
	case 'T', 't':
		mult, body = 1 << 40, raw[:len(raw)-1]
	}
	n, err := strconv.ParseInt(body, 10, 64)
	if err != nil {
		return 0, err
	}
	return n * mult, nil
}

func validateBounds(spec SettingSpec, v any) error {
	switch spec.Type {
	case SettingTypeInt64:
		n := v.(int64)
		if n == 0 && spec.AllowZero {
			return nil
		}
		if n < spec.MinNumeric || n > spec.MaxNumeric {
			return fmt.Errorf("value %d out of bounds [%d, %d]", n, spec.MinNumeric, spec.MaxNumeric)
		}
	case SettingTypeInt:
		n := int64(v.(int))
		if n == 0 && spec.AllowZero {
			return nil
		}
		if n < spec.MinNumeric || n > spec.MaxNumeric {
			return fmt.Errorf("value %d out of bounds [%d, %d]", n, spec.MinNumeric, spec.MaxNumeric)
		}
	case SettingTypeUint64:
		u := v.(uint64)
		if u == 0 && spec.AllowZero {
			return nil
		}
		if int64(u) < spec.MinNumeric || int64(u) > spec.MaxNumeric {
			return fmt.Errorf("value %d out of bounds [%d, %d]", u, spec.MinNumeric, spec.MaxNumeric)
		}
	case SettingTypeDuration:
		d := int64(v.(time.Duration))
		if d < spec.MinNumeric || d > spec.MaxNumeric {
			return fmt.Errorf("duration %v out of bounds [%v, %v]", time.Duration(d), time.Duration(spec.MinNumeric), time.Duration(spec.MaxNumeric))
		}
	case SettingTypeString:
		s := v.(string)
		if spec.StringValidator != nil {
			return spec.StringValidator(s)
		}
	}
	return nil
}

func sortedSpecKeys(m map[string]SettingSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Sort by (Group, Key) for stable UI ordering.
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			a, b := m[keys[i]], m[keys[j]]
			if a.Group > b.Group || (a.Group == b.Group && a.Key > b.Key) {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}

func ptrInt64(v int64) *int64 { return &v }
```

- [ ] **Step 4: Run tests**

Run: `go test --tags 'json1 fts5' -race ./application_context/ -run TestRuntimeSettings -v`
Expected: PASS (including `-race`).

- [ ] **Step 5: Commit**

```bash
git add application_context/runtime_settings.go application_context/runtime_settings_test.go
git commit -m "feat(settings): RuntimeSettings service with Load/Set/Reset/List"
```

---

### Task 4: Typed getters

**Files:**
- Modify: `application_context/runtime_settings.go` (append getters)
- Modify: `application_context/runtime_settings_test.go` (append typed-getter tests)

- [ ] **Step 1: Write failing tests for each typed getter**

Append to `application_context/runtime_settings_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestRuntimeSettings_TypedGetters -v`
Expected: FAIL — methods undefined.

- [ ] **Step 3: Implement getters**

Append to `application_context/runtime_settings.go`:

```go
func (s *RuntimeSettings) MaxUploadSize() int64 { v, _ := s.getRaw(KeyMaxUploadSize); return v.(int64) }
func (s *RuntimeSettings) MaxImportSize() int64 { v, _ := s.getRaw(KeyMaxImportSize); return v.(int64) }
func (s *RuntimeSettings) MRQLDefaultLimit() int { v, _ := s.getRaw(KeyMRQLDefaultLimit); return v.(int) }
func (s *RuntimeSettings) MRQLQueryTimeout() time.Duration {
	v, _ := s.getRaw(KeyMRQLQueryTimeout)
	return v.(time.Duration)
}
func (s *RuntimeSettings) ExportRetention() time.Duration {
	v, _ := s.getRaw(KeyExportRetention)
	return v.(time.Duration)
}
func (s *RuntimeSettings) RemoteConnectTimeout() time.Duration {
	v, _ := s.getRaw(KeyRemoteConnectTimeout)
	return v.(time.Duration)
}
func (s *RuntimeSettings) RemoteIdleTimeout() time.Duration {
	v, _ := s.getRaw(KeyRemoteIdleTimeout)
	return v.(time.Duration)
}
func (s *RuntimeSettings) RemoteOverallTimeout() time.Duration {
	v, _ := s.getRaw(KeyRemoteOverallTimeout)
	return v.(time.Duration)
}
func (s *RuntimeSettings) SharePublicURL() string {
	v, _ := s.getRaw(KeySharePublicURL)
	return v.(string)
}
func (s *RuntimeSettings) HashSimilarityThreshold() int {
	v, _ := s.getRaw(KeyHashSimilarityThreshold)
	return v.(int)
}
func (s *RuntimeSettings) HashAHashThreshold() uint64 {
	v, _ := s.getRaw(KeyHashAHashThreshold)
	return v.(uint64)
}
```

- [ ] **Step 4: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestRuntimeSettings_TypedGetters -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add application_context/runtime_settings.go application_context/runtime_settings_test.go
git commit -m "feat(settings): typed getters for all 11 keys"
```

---

### Task 5: Boot-conflict and bounds-failure logging

**Files:**
- Create: `application_context/runtime_settings_boot_test.go`

- [ ] **Step 1: Write the boot-conflict tests**

`application_context/runtime_settings_boot_test.go`:

```go
package application_context

import (
	"testing"
	"time"

	"mahresources/models"
)

func TestBoot_DivergenceEmitsWarning(t *testing.T) {
	db := newTestDB(t)
	enc, _ := encodeSettingValue("int64", int64(4<<30))
	db.Create(&models.RuntimeSetting{
		Key: KeyMaxUploadSize, ValueJSON: string(enc), UpdatedAt: time.Now(),
	})
	log := &stubLogger{}
	rs := NewRuntimeSettings(db, log, buildSpecs(), defaults())
	if err := rs.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if !log.contains(`runtime_setting "max_upload_size" override`) {
		t.Fatalf("want divergence WARN, got entries: %#v", log.entries)
	}
	if !log.contains(`boot flag`) {
		t.Fatalf("WARN should mention the boot flag value")
	}
}

func TestBoot_NoDivergenceWhenValuesMatch(t *testing.T) {
	db := newTestDB(t)
	// Persisted value equals the boot default from defaults().
	enc, _ := encodeSettingValue("int64", int64(2<<30))
	db.Create(&models.RuntimeSetting{
		Key: KeyMaxUploadSize, ValueJSON: string(enc), UpdatedAt: time.Now(),
	})
	log := &stubLogger{}
	rs := NewRuntimeSettings(db, log, buildSpecs(), defaults())
	_ = rs.Load()
	if log.contains(`override`) {
		t.Fatalf("no WARN expected when override equals default; got %#v", log.entries)
	}
}

func TestBoot_OutOfBoundsDroppedFromCache(t *testing.T) {
	db := newTestDB(t)
	enc, _ := encodeSettingValue("int64", int64(-1)) // below bounds
	db.Create(&models.RuntimeSetting{
		Key: KeyMaxImportSize, ValueJSON: string(enc), UpdatedAt: time.Now(),
	})
	log := &stubLogger{}
	rs := NewRuntimeSettings(db, log, buildSpecs(), defaults())
	_ = rs.Load()
	if rs.MaxImportSize() != int64(10<<30) {
		t.Fatalf("want fallback to default, got %v", rs.MaxImportSize())
	}
	if !log.contains("fails bounds") {
		t.Fatalf("want bounds-fail ERROR, got %#v", log.entries)
	}
}
```

- [ ] **Step 2: Run tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestBoot -v`
Expected: PASS — the warning/error paths already exist in `Load()` from Task 3.

- [ ] **Step 3: Commit**

```bash
git add application_context/runtime_settings_boot_test.go
git commit -m "test(settings): boot-conflict and bounds-failure logging"
```

---

### Task 6: Audit — write log_entries rows on Set/Reset

**Files:**
- Modify: `application_context/runtime_settings.go` (wire Set/Reset to existing Logger)
- Modify: `application_context/runtime_settings_test.go` (audit tests)

- [ ] **Step 1: Write failing audit tests**

Append to `application_context/runtime_settings_test.go`:

```go
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
```

- [ ] **Step 2: Add `LogActionReset` constant**

Modify `models/log_entry_model.go`, append to the action constants block:

```go
const (
	LogActionCreate   = "create"
	LogActionUpdate   = "update"
	LogActionDelete   = "delete"
	LogActionSystem   = "system"
	LogActionProgress = "progress"
	LogActionPlugin   = "plugin"
	LogActionReset    = "reset"
)
```

- [ ] **Step 3: Add auditor plumbing + implementations**

Append to `application_context/runtime_settings.go`:

```go
// Auditor writes an audit log entry. Production implementations delegate to the
// existing Logger (LogFromRequest). The interface keeps RuntimeSettings
// testable in isolation.
type Auditor interface {
	Audit(action, entityType, entityName, message string, details map[string]any, ipAddress string)
}

// SetAuditor configures the auditor used by Set/Reset. If nil (the default),
// no audit row is written — useful in unit tests that don't care.
func (s *RuntimeSettings) SetAuditor(a Auditor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditor = a
}
```

Add `auditor Auditor` to the `RuntimeSettings` struct (under the mutex — accessed through locks in Set/Reset).

Update `Set` to finish with (replace the `_ = actor` line):

```go
	s.mu.Lock()
	oldValue, _ := s.overrideOrDefaultLocked(key)
	s.overrides[key] = persistedEntry{value: v, updatedAt: now, reason: reason}
	a := s.auditor
	s.mu.Unlock()
	if a != nil {
		msg := fmt.Sprintf("%s: %v → %v (reason: %s)", key, oldValue, v, reason)
		a.Audit(models.LogActionUpdate, "runtime_setting", key, msg, map[string]any{
			"oldValue": oldValue, "newValue": v, "reason": reason, "type": string(spec.Type),
		}, actor)
	}
	return nil
```

Update `Reset` similarly:

```go
	s.mu.Lock()
	oldValue, _ := s.overrideOrDefaultLocked(key)
	delete(s.overrides, key)
	bootDefault := s.defaults[key]
	a := s.auditor
	s.mu.Unlock()
	if a != nil {
		msg := fmt.Sprintf("%s: %v → %v (reset; reason: %s)", key, oldValue, bootDefault, reason)
		a.Audit(models.LogActionReset, "runtime_setting", key, msg, map[string]any{
			"oldValue": oldValue, "newValue": bootDefault, "reason": reason,
		}, actor)
	}
	return nil
```

Add a locked helper:

```go
// overrideOrDefaultLocked returns the current effective value under the lock.
// Caller must hold s.mu (read or write).
func (s *RuntimeSettings) overrideOrDefaultLocked(key string) (any, bool) {
	if e, ok := s.overrides[key]; ok {
		return e.value, true
	}
	if d, ok := s.defaults[key]; ok {
		return d, false
	}
	return nil, false
}
```

Add the models import: `"mahresources/models"` (may already be present).

Now the test-only auditor — append to `application_context/runtime_settings_test.go`:

```go
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
```

- [ ] **Step 4: Run audit tests**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestAudit -v`
Expected: PASS.

- [ ] **Step 5: Run full RuntimeSettings test suite**

Run: `go test --tags 'json1 fts5' -race ./application_context/ -run 'TestRuntimeSettings|TestBoot|TestAudit|TestEnvelope|TestBuildSpecs|TestValidateShare' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add models/log_entry_model.go application_context/runtime_settings.go application_context/runtime_settings_test.go
git commit -m "feat(settings): audit Set/Reset to log_entries via Auditor"
```

---

## Phase 2 — Context Wiring

### Task 7: Wire RuntimeSettings into MahresourcesContext and main.go

**Files:**
- Modify: `application_context/context.go` (add Settings field, accessor, Logger-backed Auditor)
- Modify: `main.go` (instantiate after AutoMigrate; remove `MRQLQueryTimeout` assignment)

- [ ] **Step 1: Extend MahresourcesContext**

In `application_context/context.go`, add the field to the `MahresourcesContext` struct (near the existing service fields):

```go
// settings holds runtime-editable configuration overrides. Nil until
// NewRuntimeSettings is wired in main.go after AutoMigrate.
settings *RuntimeSettings
```

Add the accessor and setter, near other `ctx.X()` methods at the bottom of the file (keep them simple):

```go
// Settings returns the runtime-settings service. Panics if called before wiring.
func (ctx *MahresourcesContext) Settings() *RuntimeSettings {
	if ctx.settings == nil {
		panic("MahresourcesContext.Settings() called before wiring")
	}
	return ctx.settings
}

// SetSettings installs the runtime-settings service. Called once from main.go.
func (ctx *MahresourcesContext) SetSettings(rs *RuntimeSettings) {
	ctx.settings = rs
}
```

- [ ] **Step 2: Add the Logger-backed Auditor**

Append to `application_context/runtime_settings.go`:

```go
// NewContextAuditor returns an Auditor that writes through the context's
// existing Logger infrastructure. IP/path/UA come from request auditing; this
// wrapper supplies the IP address directly because RuntimeSettings is called
// from handler scope and already has it.
func NewContextAuditor(ctx *MahresourcesContext) Auditor {
	return &contextAuditor{ctx: ctx}
}

type contextAuditor struct{ ctx *MahresourcesContext }

func (a *contextAuditor) Audit(action, entityType, entityName, message string, details map[string]any, _ string) {
	// LogFromRequest populates IPAddress/RequestPath/UserAgent from the
	// current request, which is set per-request on the context via WithRequest().
	a.ctx.Logger().Info(action, entityType, nil, entityName, message, details)
}
```

- [ ] **Step 3: Wire in main.go**

Modify `main.go`. After `db.AutoMigrate(...)` succeeds (around line 336) and before `util.AddInitialData(db)`:

```go
// Initialize runtime settings (bucket-A overrides: sizes, timeouts, etc.)
settings := application_context.NewRuntimeSettings(
	db,
	application_context.NewStdlibSettingsLogger(), // or similar small helper
	application_context.BuildSpecsExported(),
	application_context.BuildDefaultsFromConfig(context.Config),
)
if err := settings.Load(); err != nil {
	log.Fatalf("failed to load runtime settings: %v", err)
}
settings.SetAuditor(application_context.NewContextAuditor(context))
context.SetSettings(settings)
```

Remove the existing line:

```go
// Configure MRQL query timeout
application_context.MRQLQueryTimeout = *mrqlTimeout
```

(It moves into the settings system in Task 9.)

- [ ] **Step 4: Add the exported helpers**

Append to `application_context/runtime_setting_spec.go`:

```go
// BuildSpecsExported is the main.go-visible accessor for the spec registry.
func BuildSpecsExported() map[string]SettingSpec { return buildSpecs() }

// BuildDefaultsFromConfig snapshots every in-scope setting from the boot-time
// MahresourcesConfig into a map keyed by spec key.
func BuildDefaultsFromConfig(cfg *MahresourcesConfig) map[string]any {
	return map[string]any{
		KeyMaxUploadSize:           cfg.MaxUploadSize,
		KeyMaxImportSize:           cfg.MaxImportSize,
		KeyMRQLDefaultLimit:        cfg.MRQLDefaultLimit,
		KeyMRQLQueryTimeout:        mrqlQueryTimeoutDefault(cfg),
		KeyExportRetention:         cfg.ExportRetention,
		KeyRemoteConnectTimeout:    cfg.RemoteResourceConnectTimeout,
		KeyRemoteIdleTimeout:       cfg.RemoteResourceIdleTimeout,
		KeyRemoteOverallTimeout:    cfg.RemoteResourceOverallTimeout,
		KeySharePublicURL:          cfg.SharePublicURL,
		KeyHashSimilarityThreshold: cfg.HashSimilarityThreshold,
		KeyHashAHashThreshold:      uint64(5), // current main.go default; cfg doesn't carry this
	}
}

// mrqlQueryTimeoutDefault sources the boot MRQL timeout. Until Task 9 the
// package-level var is still canonical; afterwards, main.go passes the value
// explicitly via cfg.
func mrqlQueryTimeoutDefault(cfg *MahresourcesConfig) time.Duration {
	if MRQLQueryTimeout > 0 {
		return MRQLQueryTimeout
	}
	return 10 * time.Second
}
```

Note: `cfg.HashSimilarityThreshold` and `cfg.HashAHashThreshold` — verify these fields exist on `MahresourcesConfig`; if not, add them from the `hash_worker.Config` that `main.go` builds (check Task 8's current state). If the struct field is missing, add it as part of this task and populate from the flag.

Add a minimal stdlib logger helper:

```go
// NewStdlibSettingsLogger returns a SettingsLogger backed by the stdlib log package.
func NewStdlibSettingsLogger() SettingsLogger { return stdlibSettingsLogger{} }

type stdlibSettingsLogger struct{}

func (stdlibSettingsLogger) Warn(format string, args ...any)  { log.Printf("WARN: "+format, args...) }
func (stdlibSettingsLogger) Error(format string, args ...any) { log.Printf("ERROR: "+format, args...) }
```

And import `"log"` at the top of `runtime_setting_spec.go`.

- [ ] **Step 5: Verify build**

Run: `go build --tags 'json1 fts5' ./...`
Expected: build passes. If `cfg.HashSimilarityThreshold` / `cfg.HashAHashThreshold` are not on the struct, add them to `MahresourcesConfig` (both copies, line 108-ish and 182-ish), populate from the flag values in the `cfg := &MahresourcesInputConfig{...}` and context-build blocks in main.go, then re-run.

- [ ] **Step 6: Smoke test — server starts**

Run: `./mahresources -ephemeral -bind-address=:19191 &`
Then: `curl -s http://localhost:19191/v1/resources?max=1 | head -c 200; echo`
Kill the server. Expected: server boots, responds to API.

- [ ] **Step 7: Commit**

```bash
git add application_context/context.go application_context/runtime_settings.go application_context/runtime_setting_spec.go main.go
git commit -m "feat(settings): wire RuntimeSettings into MahresourcesContext"
```

---

## Phase 3 — Refactor Hot Paths

### Task 8: Switch trivial per-use callsites to Settings

These are the three "already per-use" settings. One-line changes at each callsite.

**Files:**
- Modify: `server/routes.go` (lines 411, 439 — MaxUploadSize closures)
- Modify: `application_context/mrql_context.go` (line 137 — MRQLDefaultLimit)
- Find all `appContext.Config.SharePublicURL` readers and switch

- [ ] **Step 1: Switch MaxUploadSize in routes**

In `server/routes.go:411` and `:439`, replace:

```go
func() int64 { return appContext.Config.MaxUploadSize }
```

with:

```go
func() int64 { return appContext.Settings().MaxUploadSize() }
```

- [ ] **Step 2: Switch MRQLDefaultLimit**

In `application_context/mrql_context.go:136-137`, the current code reads `ctx.Config.MRQLDefaultLimit`. Change the method to prefer `Settings`:

```go
func (ctx *MahresourcesContext) GetMRQLDefaultLimit() int {
	if ctx.settings != nil {
		return ctx.settings.MRQLDefaultLimit()
	}
	if ctx.Config != nil && ctx.Config.MRQLDefaultLimit > 0 {
		return ctx.Config.MRQLDefaultLimit
	}
	return DefaultMRQLLimitFallback
}
```

(The `ctx.settings != nil` guard keeps tests that don't wire settings working.)

- [ ] **Step 3: Switch SharePublicURL**

Run: `grep -rn 'Config\.SharePublicURL' --include='*.go'` — expect hits in the share-template or admin-shares context files. For each production read, switch to `appContext.Settings().SharePublicURL()`.

Do not change tests that inject a manual `Config` without a `settings` service — those still exercise the legacy path (and will be covered by integration E2E later).

- [ ] **Step 4: Build & run unit tests**

Run: `go build --tags 'json1 fts5' ./... && go test --tags 'json1 fts5' ./...`
Expected: build + existing tests still pass.

- [ ] **Step 5: Commit**

```bash
git add server/routes.go application_context/mrql_context.go # + any Share URL files touched
git commit -m "refactor(settings): route trivial per-use reads through Settings()"
```

---

### Task 9: Refactor MRQL query timeout

**Files:**
- Modify: `application_context/mrql_context.go` (remove package var; update 5 callsites)
- Modify: `main.go` (remove `application_context.MRQLQueryTimeout = *mrqlTimeout` — already done in Task 7)
- Update tests as needed

- [ ] **Step 1: Write a failing integration test for runtime override**

Create `application_context/mrql_timeout_runtime_test.go` — skip if there's an existing MRQL timeout test that can be extended; otherwise:

```go
package application_context

import (
	"context"
	"testing"
	"time"
)

// TestMRQLQueryTimeout_RuntimeOverride confirms the query timeout is read
// through appContext.Settings() per call, not captured at startup.
func TestMRQLQueryTimeout_RuntimeOverride(t *testing.T) {
	// This test is a contract check: it doesn't execute real MRQL, it just
	// verifies the helper used by the 5 callsites reads through Settings.
	ctx := &MahresourcesContext{Config: &MahresourcesConfig{}}
	rs := NewRuntimeSettings(newTestDB(t), &stubLogger{}, buildSpecs(), defaults())
	_ = rs.Load()
	ctx.SetSettings(rs)
	if got := ctx.mrqlQueryTimeout(); got != 10*time.Second {
		t.Fatalf("default: want 10s, got %v", got)
	}
	if err := rs.Set(KeyMRQLQueryTimeout, "2s", "", ""); err != nil {
		t.Fatal(err)
	}
	if got := ctx.mrqlQueryTimeout(); got != 2*time.Second {
		t.Fatalf("override: want 2s, got %v", got)
	}
	_ = context.Background()
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestMRQLQueryTimeout_RuntimeOverride -v`
Expected: FAIL — `ctx.mrqlQueryTimeout()` undefined.

- [ ] **Step 3: Replace the package var**

In `application_context/mrql_context.go`:

1. Delete the `var MRQLQueryTimeout = 10 * time.Second` declaration and its doc comment.
2. Add a method:

```go
// mrqlQueryTimeout returns the current MRQL query timeout from runtime settings,
// falling back to 10s if settings haven't been wired (test contexts).
func (ctx *MahresourcesContext) mrqlQueryTimeout() time.Duration {
	if ctx.settings != nil {
		return ctx.settings.MRQLQueryTimeout()
	}
	return 10 * time.Second
}
```

3. Replace each occurrence of `MRQLQueryTimeout` (lines 152, 194, 426, 769, 820) with `ctx.mrqlQueryTimeout()`. Example:

```go
queryCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
```

4. Remove the `mrqlQueryTimeoutDefault(cfg)` helper from Task 7 in `runtime_setting_spec.go`; replace with a plain `10 * time.Second` default:

```go
KeyMRQLQueryTimeout: 10 * time.Second,
```

Wait — `BuildDefaultsFromConfig` still needs the boot value. Replace with:

```go
KeyMRQLQueryTimeout: cfg.MRQLQueryTimeoutBoot,
```

and add a `MRQLQueryTimeoutBoot time.Duration` field to `MahresourcesConfig` (both declarations at lines 104-108 and 180-183). Wire it from `*mrqlTimeout` in `main.go` in the `cfg := &MahresourcesInputConfig{...}` block.

- [ ] **Step 4: Run the new test + full mrql suite**

Run: `go test --tags 'json1 fts5' ./application_context/ -run TestMRQLQueryTimeout_RuntimeOverride -v && go test --tags 'json1 fts5 postgres' ./mrql/... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add application_context/mrql_context.go application_context/mrql_timeout_runtime_test.go application_context/context.go application_context/runtime_setting_spec.go main.go
git commit -m "refactor(mrql): route query timeout through Settings"
```

---

### Task 10: Refactor MaxImportSize handler signature

**Files:**
- Modify: `server/api_handlers/import_api_handlers.go`
- Modify: `server/api_tests/import_api_test.go` (line 81)
- Modify: `server/routes.go:546`

- [ ] **Step 1: Change handler signature**

In `server/api_handlers/import_api_handlers.go`, change:

```go
func GetImportParseHandler(ctx GroupImporter, maxSize int64) func(http.ResponseWriter, *http.Request) {
```

to:

```go
func GetImportParseHandler(ctx GroupImporter, maxSize func() int64) func(http.ResponseWriter, *http.Request) {
```

Inside the handler, change any `maxSize` reference to `maxSize()`.

- [ ] **Step 2: Update the test callsite**

In `server/api_tests/import_api_test.go:81`:

```go
api_handlers.GetImportParseHandler(mock, func() int64 { return 0 })(rec, req)
```

- [ ] **Step 3: Update the production callsite**

In `server/routes.go:546`:

```go
router.Methods(http.MethodPost).Path("/v1/groups/import/parse").HandlerFunc(
	api_handlers.GetImportParseHandler(appContext, func() int64 { return appContext.Settings().MaxImportSize() }),
)
```

- [ ] **Step 4: Write a hot-path integration test**

Append to `server/api_tests/import_api_test.go`:

```go
func TestImportParse_RuntimeOverrideRejectsLargeBody(t *testing.T) {
	tc := setupTestContext(t) // assumes existing helper; adapt to the one in place
	if err := tc.AppCtx.Settings().Set(application_context.KeyMaxImportSize, "1048576", "test", ""); err != nil {
		t.Fatalf("set: %v", err)
	}
	body := bytes.Repeat([]byte{0}, 2<<20) // 2 MiB
	req := httptest.NewRequest(http.MethodPost, "/v1/groups/import/parse", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	api_handlers.GetImportParseHandler(tc.AppCtx, func() int64 { return tc.AppCtx.Settings().MaxImportSize() })(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d", rec.Code)
	}
}
```

If `setupTestContext` in the existing test file wires `Settings()`, great; if not, adapt the helper (or create a dedicated mini-helper that constructs `MahresourcesContext` with a `RuntimeSettings` plus in-memory DB). Reference: the pattern used in `upload_size_limit_test.go` — it may already handle this.

- [ ] **Step 5: Run tests**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run 'Import|Upload' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/api_handlers/import_api_handlers.go server/api_tests/import_api_test.go server/routes.go
git commit -m "refactor(import): MaxImportSize handler reads through Settings per request"
```

---

### Task 11: Refactor DownloadManager — DownloadSettings interface

**Files:**
- Modify: `download_queue/manager.go`
- Modify: `download_queue/manager_test.go` (2 callsites at lines 1009, 1063)
- Modify: `application_context/context.go` (pass `ctx.settings` — or adapter — instead of `TimeoutConfig{...}`)

- [ ] **Step 1: Add DownloadSettings interface and static adapter**

Append to `download_queue/manager.go`, near the `TimeoutConfig` declaration:

```go
// DownloadSettings is the runtime configuration surface for the download
// manager. Reads are called per download start so runtime changes take effect
// without a restart. See application_context.RuntimeSettings.
type DownloadSettings interface {
	ConnectTimeout() time.Duration
	IdleTimeout() time.Duration
	OverallTimeout() time.Duration
	ExportRetention() time.Duration
}

// NewStaticDownloadSettings returns a DownloadSettings whose values never
// change. Used by tests and by the legacy NewDownloadManager constructor.
func NewStaticDownloadSettings(tc TimeoutConfig, exportRetention time.Duration) DownloadSettings {
	return staticDownloadSettings{tc: tc, er: exportRetention}
}

type staticDownloadSettings struct {
	tc TimeoutConfig
	er time.Duration
}

func (s staticDownloadSettings) ConnectTimeout() time.Duration  { return s.tc.ConnectTimeout }
func (s staticDownloadSettings) IdleTimeout() time.Duration     { return s.tc.IdleTimeout }
func (s staticDownloadSettings) OverallTimeout() time.Duration  { return s.tc.OverallTimeout }
func (s staticDownloadSettings) ExportRetention() time.Duration { return s.er }
```

- [ ] **Step 2: Swap the manager's storage**

In `download_queue/manager.go`, replace the `timeoutConfig TimeoutConfig` and `exportRetention time.Duration` fields on `DownloadManager` with a single provider:

```go
type DownloadManager struct {
	mu            sync.RWMutex
	jobs          map[string]*DownloadJob
	jobOrder      []string
	resourceCtx   ResourceCreator
	settings      DownloadSettings // was timeoutConfig + exportRetention
	semaphore     chan struct{}
	// ... rest unchanged
}
```

In `NewDownloadManagerWithConfig`, accept the provider directly and drop the `exportRetention: cfg.ExportRetention` copy:

```go
func NewDownloadManagerWithConfig(resourceCtx ResourceCreator, settings DownloadSettings, cfg ManagerConfig) *DownloadManager {
	// ... validate concurrency/jobRetention as before
	dm := &DownloadManager{
		jobs:         make(map[string]*DownloadJob),
		jobOrder:     make([]string, 0),
		resourceCtx:  resourceCtx,
		settings:     settings,
		semaphore:    make(chan struct{}, cfg.Concurrency),
		subscribers:  make(map[chan JobEvent]struct{}),
		done:         make(chan struct{}),
		concurrency:  cfg.Concurrency,
		jobRetention: cfg.JobRetention,
	}
	dm.cleanupTicker = time.NewTicker(5 * time.Minute)
	go dm.cleanupLoop()
	return dm
}
```

`ManagerConfig.ExportRetention` is no longer used — remove the field from the struct, or leave it and ignore it for now. Clean removal is preferred.

Replace `NewDownloadManager` to delegate through the adapter:

```go
func NewDownloadManager(resourceCtx ResourceCreator, tc TimeoutConfig) *DownloadManager {
	return NewDownloadManagerWithConfig(resourceCtx, NewStaticDownloadSettings(tc, 0), ManagerConfig{})
}
```

Change the accessor:

```go
func (m *DownloadManager) ExportRetention() time.Duration { return m.settings.ExportRetention() }
```

Change every `dm.timeoutConfig.X` reference (lines 294, 296-298, 329) to `dm.settings.X()`:

```go
Timeout: dm.settings.OverallTimeout(),
DialContext:           (&net.Dialer{Timeout: dm.settings.ConnectTimeout()}).DialContext,
TLSHandshakeTimeout:   dm.settings.ConnectTimeout() / 2,
ResponseHeaderTimeout: dm.settings.ConnectTimeout(),
// ...
timeoutBody := NewTimeoutReaderWithContext(resp.Body, dm.settings.IdleTimeout(), job.GetContext())
```

- [ ] **Step 3: Update download_queue tests**

In `download_queue/manager_test.go`, replace both `NewDownloadManagerWithConfig(nil, TimeoutConfig{}, ManagerConfig{...})` callsites (lines 1009, 1063):

```go
dm := NewDownloadManagerWithConfig(nil, NewStaticDownloadSettings(TimeoutConfig{}, 24*time.Hour), ManagerConfig{
	// ... rest unchanged; ExportRetention removed from ManagerConfig literal
})
```

(Pass the retention value the test previously set via `ExportRetention: 24 * time.Hour`.)

- [ ] **Step 4: Update context.go**

In `application_context/context.go` around line 282 where the download manager is constructed, switch to pass `ctx.settings` directly (once it's set — see step 5 on ordering):

```go
ctx.downloadManager = download_queue.NewDownloadManagerWithConfig(
	ctx,
	ctx.settings, // implements DownloadSettings via ConnectTimeout/IdleTimeout/OverallTimeout/ExportRetention
	download_queue.ManagerConfig{
		Concurrency:  config.MaxJobConcurrency,
		JobRetention: 30 * time.Minute, // or whatever the existing value is
	},
)
```

But there's an ordering problem: `NewMahresourcesContext` creates the download manager *before* settings is wired. We need to either:

(a) Add a `SetDownloadSettings` method on the manager (post-hoc provider injection), or
(b) Restructure `NewMahresourcesContext` so it accepts the settings, or
(c) Delay download-manager creation in main.go.

Simplest: option (a). Add:

```go
func (m *DownloadManager) SetSettings(settings DownloadSettings) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.settings = settings
}
```

In `NewMahresourcesContext`, keep passing `NewStaticDownloadSettings(TimeoutConfig{ConnectTimeout: ..., ...}, config.ExportRetention)` as a boot-time seed. In `main.go`, after `context.SetSettings(settings)`, also call `context.DownloadManager().SetSettings(context.Settings())` so the manager re-reads through the live provider.

Document the two-phase init at the SetSettings callsite with a one-line comment.

- [ ] **Step 5: Make RuntimeSettings satisfy DownloadSettings**

The existing getter names (`ConnectTimeout`, `IdleTimeout`, `OverallTimeout`, `ExportRetention`) on `RuntimeSettings` are `RemoteConnectTimeout` / `RemoteIdleTimeout` / `RemoteOverallTimeout` / `ExportRetention`. Add thin alias methods:

```go
// DownloadSettings adapter methods — satisfy download_queue.DownloadSettings
// without leaking the download_queue package into the service signatures.
func (s *RuntimeSettings) ConnectTimeout() time.Duration  { return s.RemoteConnectTimeout() }
func (s *RuntimeSettings) IdleTimeout() time.Duration     { return s.RemoteIdleTimeout() }
func (s *RuntimeSettings) OverallTimeout() time.Duration  { return s.RemoteOverallTimeout() }
// ExportRetention already exists as a getter.
```

- [ ] **Step 6: Write a runtime-override test for the manager**

Append to `download_queue/manager_test.go`:

```go
type mutableSettings struct {
	mu sync.RWMutex
	v  time.Duration
}

func (m *mutableSettings) ConnectTimeout() time.Duration  { m.mu.RLock(); defer m.mu.RUnlock(); return m.v }
func (m *mutableSettings) IdleTimeout() time.Duration     { m.mu.RLock(); defer m.mu.RUnlock(); return m.v }
func (m *mutableSettings) OverallTimeout() time.Duration  { m.mu.RLock(); defer m.mu.RUnlock(); return m.v }
func (m *mutableSettings) ExportRetention() time.Duration { m.mu.RLock(); defer m.mu.RUnlock(); return m.v }

func TestExportRetention_RuntimeOverride(t *testing.T) {
	ms := &mutableSettings{v: 1 * time.Hour}
	dm := NewDownloadManagerWithConfig(nil, ms, ManagerConfig{})
	defer dm.Stop() // if Stop exists; otherwise omit
	if dm.ExportRetention() != 1*time.Hour {
		t.Fatalf("initial: got %v", dm.ExportRetention())
	}
	ms.mu.Lock()
	ms.v = 2 * time.Hour
	ms.mu.Unlock()
	if dm.ExportRetention() != 2*time.Hour {
		t.Fatalf("after override: got %v", dm.ExportRetention())
	}
}
```

- [ ] **Step 7: Run download_queue tests**

Run: `go test --tags 'json1 fts5' ./download_queue/... -v`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add download_queue/manager.go download_queue/manager_test.go application_context/context.go application_context/runtime_settings.go main.go
git commit -m "refactor(downloads): DownloadSettings interface; timeouts/retention read per-use"
```

---

### Task 12: ExportRetention template sites

**Files:**
- Modify: `server/routes.go:122-123`

- [ ] **Step 1: Switch the two template-context reads**

In `server/routes.go:122-123`, replace:

```go
ctx["exportRetention"] = appContext.Config.ExportRetention.String()
ctx["exportRetentionMs"] = appContext.Config.ExportRetention.Milliseconds()
```

with:

```go
ctx["exportRetention"] = appContext.Settings().ExportRetention().String()
ctx["exportRetentionMs"] = appContext.Settings().ExportRetention().Milliseconds()
```

- [ ] **Step 2: Extend the existing disclosure E2E**

In `e2e/tests/c10-bh036-export-retention-disclosure.spec.ts`, add one test that overrides the setting via `PUT /v1/admin/settings/export_retention` (once Task 15 lands; this test can be written now but skipped with `test.skip(true, 'pending admin settings API')` until then — or add this test to Task 15 if simpler). Add a `TODO(BH-TASK-12)` reference and move on.

Alternative (cleaner): defer the test to Task 15 when the API exists. Choose this path to avoid skip-chains. The code change stands on its own — existing export tests still pass with boot-defaults.

- [ ] **Step 3: Run existing export E2E tests**

Run: `cd e2e && npm run test:with-server -- --grep 'export|retention'`
Expected: PASS (values equal boot defaults, same behavior as before).

- [ ] **Step 4: Commit**

```bash
git add server/routes.go
git commit -m "refactor(exports): template context reads ExportRetention through Settings"
```

---

### Task 13: HashWorker threshold callbacks

**Files:**
- Modify: `hash_worker/config.go`
- Modify: `hash_worker/worker.go` (line 480)
- Modify: `hash_worker/worker_test.go`, `worker_solid_color_test.go`
- Modify: `main.go` (wire `appContext.Settings()` getters into the Config)

- [ ] **Step 1: Replace fields with callbacks**

`hash_worker/config.go`:

```go
package hash_worker

import "time"

type Config struct {
	WorkerCount int
	BatchSize   int
	PollInterval time.Duration
	// SimilarityThresholdFn returns the max DHash Hamming distance. Called per
	// pair comparison so runtime settings changes take effect without restart.
	SimilarityThresholdFn func() int
	// AHashThresholdFn returns the max AHash Hamming distance for the secondary
	// check. Return 0 to disable. Called per pair comparison.
	AHashThresholdFn func() uint64
	Disabled         bool
	CacheSize        int
}

func DefaultConfig() Config {
	return Config{
		WorkerCount:           4,
		BatchSize:             500,
		PollInterval:          time.Minute,
		SimilarityThresholdFn: func() int { return 10 },
		AHashThresholdFn:      func() uint64 { return 5 },
		Disabled:              false,
		CacheSize:             100000,
	}
}
```

- [ ] **Step 2: Update worker.go:480**

Replace:

```go
if !AreSimilar(dHash, aHash, otherEntry.DHash, otherEntry.AHash,
	uint64(w.config.SimilarityThreshold), w.config.AHashThreshold) {
```

with:

```go
if !AreSimilar(dHash, aHash, otherEntry.DHash, otherEntry.AHash,
	uint64(w.config.SimilarityThresholdFn()), w.config.AHashThresholdFn()) {
```

Also check the log-init line at worker.go:100:

```go
w.config.WorkerCount, w.config.BatchSize, w.config.PollInterval, w.config.SimilarityThresholdFn())
```

- [ ] **Step 3: Update hash_worker tests**

In `hash_worker/worker_test.go:59, 92` and `worker_solid_color_test.go:94`, replace literal threshold initializations with closures:

```go
// was: SimilarityThreshold: 10,
SimilarityThresholdFn: func() int { return 10 },
AHashThresholdFn:      func() uint64 { return 5 },
```

- [ ] **Step 4: Wire Settings into main.go**

In `main.go`, find the `hashWorkerConfig := hash_worker.Config{...}` block (around line 449-458) and change to:

```go
hashWorkerConfig := hash_worker.Config{
	WorkerCount:           *hashWorkerCount,
	BatchSize:             *hashBatchSize,
	PollInterval:          *hashPollInterval,
	SimilarityThresholdFn: context.Settings().HashSimilarityThreshold,
	AHashThresholdFn:      context.Settings().HashAHashThreshold,
	Disabled:              *hashWorkerDisabled,
	CacheSize:             *hashCacheSize,
}
```

Ensure this block sits *after* `context.SetSettings(settings)` (from Task 7) — move the settings-wiring block up if needed.

- [ ] **Step 5: Write a runtime-override test**

Append to `hash_worker/worker_test.go`:

```go
func TestHashWorker_SimilarityThresholdLive(t *testing.T) {
	threshold := 10
	cfg := Config{
		WorkerCount:           1,
		BatchSize:             1,
		PollInterval:          time.Millisecond,
		SimilarityThresholdFn: func() int { return threshold },
		AHashThresholdFn:      func() uint64 { return 5 },
		CacheSize:             100,
	}
	// Construct a pair with DHash distance 12 → above threshold=10, below threshold=15.
	// Specific hash values depend on existing test helpers; reuse pairs from
	// TestAreSimilar or similar. Skeleton:
	got1 := AreSimilar(dhashA, ahashA, dhashB, ahashB, uint64(cfg.SimilarityThresholdFn()), cfg.AHashThresholdFn())
	if got1 {
		t.Fatal("want not similar at threshold=10")
	}
	threshold = 15
	got2 := AreSimilar(dhashA, ahashA, dhashB, ahashB, uint64(cfg.SimilarityThresholdFn()), cfg.AHashThresholdFn())
	if !got2 {
		t.Fatal("want similar at threshold=15")
	}
}
```

Adapt `dhashA`, etc. to whatever values are available in the existing test file. If no suitable helper exists, this step can be folded into the broader integration test in Task 28.

- [ ] **Step 6: Run hash_worker tests**

Run: `go test --tags 'json1 fts5' ./hash_worker/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add hash_worker/config.go hash_worker/worker.go hash_worker/worker_test.go hash_worker/worker_solid_color_test.go main.go
git commit -m "refactor(hash): threshold callbacks route through Settings"
```

---

## Phase 4 — HTTP API

### Task 14: GET /v1/admin/settings

**Files:**
- Create: `server/api_handlers/admin_settings_handlers.go`
- Create: `server/api_tests/admin_settings_test.go`
- Modify: `server/routes.go` (register route)

- [ ] **Step 1: Write failing API test**

`server/api_tests/admin_settings_test.go`:

```go
package api_tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mahresources/application_context"
	"mahresources/server/api_handlers"
)

func TestListSettings_EmptyDB(t *testing.T) {
	tc := setupTestContext(t) // existing helper; wires RuntimeSettings
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/settings", nil)
	rec := httptest.NewRecorder()
	api_handlers.GetListSettingsHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	var views []application_context.SettingView
	if err := json.Unmarshal(rec.Body.Bytes(), &views); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(views) != 11 {
		t.Fatalf("want 11, got %d", len(views))
	}
	for _, v := range views {
		if v.Overridden {
			t.Errorf("expected no overrides: %s overridden", v.Key)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestListSettings -v`
Expected: FAIL — handler undefined.

- [ ] **Step 3: Implement the handler**

`server/api_handlers/admin_settings_handlers.go`:

```go
package api_handlers

import (
	"encoding/json"
	"net/http"

	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/server/http_utils"
)

func GetListSettingsHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		views := ctx.Settings().List()
		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(views)
	}
}
```

- [ ] **Step 4: Register the route**

In `server/routes.go`, near the existing `/v1/admin/*` registrations (around line 563-565):

```go
router.Methods(http.MethodGet).Path("/v1/admin/settings").HandlerFunc(api_handlers.GetListSettingsHandler(appContext))
```

- [ ] **Step 5: Run test**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run TestListSettings -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add server/api_handlers/admin_settings_handlers.go server/api_tests/admin_settings_test.go server/routes.go
git commit -m "feat(api): GET /v1/admin/settings"
```

---

### Task 15: PUT /v1/admin/settings/{key} + DELETE

**Files:**
- Modify: `server/api_handlers/admin_settings_handlers.go`
- Modify: `server/api_tests/admin_settings_test.go`
- Modify: `server/routes.go`

- [ ] **Step 1: Write failing PUT tests**

Append to `server/api_tests/admin_settings_test.go`:

```go
func TestSetSetting_Valid(t *testing.T) {
	tc := setupTestContext(t)
	body := strings.NewReader(`{"value":"1048576","reason":"bump"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/max_upload_size", body)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d; body=%s", rec.Code, rec.Body.String())
	}
	if got := tc.AppCtx.Settings().MaxUploadSize(); got != 1<<20 {
		t.Fatalf("effective value: got %d want 1MiB", got)
	}
}

func TestSetSetting_OutOfBounds(t *testing.T) {
	tc := setupTestContext(t)
	body := strings.NewReader(`{"value":"1"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/max_upload_size", body)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rec.Code)
	}
}

func TestSetSetting_UnknownKey(t *testing.T) {
	tc := setupTestContext(t)
	body := strings.NewReader(`{"value":"1"}`)
	req := httptest.NewRequest(http.MethodPut, "/v1/admin/settings/not_a_key", body)
	req = mux.SetURLVars(req, map[string]string{"key": "not_a_key"})
	rec := httptest.NewRecorder()
	api_handlers.GetSetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rec.Code)
	}
}

func TestResetSetting(t *testing.T) {
	tc := setupTestContext(t)
	_ = tc.AppCtx.Settings().Set("max_upload_size", "1048576", "", "")
	req := httptest.NewRequest(http.MethodDelete, "/v1/admin/settings/max_upload_size", nil)
	req = mux.SetURLVars(req, map[string]string{"key": "max_upload_size"})
	rec := httptest.NewRecorder()
	api_handlers.GetResetSettingHandler(tc.AppCtx)(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rec.Code)
	}
	if got := tc.AppCtx.Settings().MaxUploadSize(); got != 2<<30 {
		t.Fatalf("after reset: want default, got %d", got)
	}
}
```

Required imports on this test file: `strings`, `github.com/gorilla/mux`.

- [ ] **Step 2: Implement PUT and DELETE handlers**

Append to `server/api_handlers/admin_settings_handlers.go`:

```go
import (
	"errors"
	"github.com/gorilla/mux"
)

type setSettingRequest struct {
	Value  string `json:"value"`
	Reason string `json:"reason,omitempty"`
}

type resetSettingRequest struct {
	Reason string `json:"reason,omitempty"`
}

func GetSetSettingHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		var req setSettingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}
		actor := clientIP(r)
		if err := ctx.Settings().Set(key, req.Value, req.Reason, actor); err != nil {
			status := classifySettingError(err)
			http_utils.HandleError(err, w, r, status)
			return
		}
		writeSettingView(w, ctx, key)
	}
}

func GetResetSettingHandler(ctx *application_context.MahresourcesContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := mux.Vars(r)["key"]
		var req resetSettingRequest
		_ = json.NewDecoder(r.Body).Decode(&req) // empty body is fine
		actor := clientIP(r)
		if err := ctx.Settings().Reset(key, req.Reason, actor); err != nil {
			status := classifySettingError(err)
			http_utils.HandleError(err, w, r, status)
			return
		}
		writeSettingView(w, ctx, key)
	}
}

func writeSettingView(w http.ResponseWriter, ctx *application_context.MahresourcesContext, key string) {
	for _, v := range ctx.Settings().List() {
		if v.Key == key {
			w.Header().Set("Content-Type", constants.JSON)
			_ = json.NewEncoder(w).Encode(v)
			return
		}
	}
	http.Error(w, `{"error":"setting not found"}`, http.StatusNotFound)
}

func classifySettingError(err error) int {
	if err == nil {
		return http.StatusOK
	}
	if errors.Is(err, application_context.ErrUnknownSetting) ||
		// string-match fallback: Set returns `unknown setting %q`
		containsMsg(err.Error(), "unknown setting") {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

func containsMsg(s, substr string) bool {
	return len(substr) == 0 || (len(s) >= len(substr) && indexOf(s, substr) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// clientIP extracts the request IP, honoring X-Forwarded-For if present.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	return r.RemoteAddr
}
```

Add an `ErrUnknownSetting` sentinel to `runtime_settings.go` and use it in `Set`/`Reset`:

```go
// ErrUnknownSetting is returned by Set/Reset when the key is not registered.
var ErrUnknownSetting = errors.New("unknown setting")
```

And update the two `return fmt.Errorf("unknown setting %q", key)` lines to:

```go
return fmt.Errorf("%w: %q", ErrUnknownSetting, key)
```

- [ ] **Step 3: Register routes**

In `server/routes.go`, near the GET registration from Task 14:

```go
router.Methods(http.MethodPut).Path("/v1/admin/settings/{key}").HandlerFunc(api_handlers.GetSetSettingHandler(appContext))
router.Methods(http.MethodDelete).Path("/v1/admin/settings/{key}").HandlerFunc(api_handlers.GetResetSettingHandler(appContext))
```

- [ ] **Step 4: Run tests**

Run: `go test --tags 'json1 fts5' ./server/api_tests/ -run 'SetSetting|ResetSetting|ListSettings' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/api_handlers/admin_settings_handlers.go server/api_tests/admin_settings_test.go application_context/runtime_settings.go server/routes.go
git commit -m "feat(api): PUT and DELETE /v1/admin/settings/{key}"
```

---

### Task 16: OpenAPI metadata

**Files:**
- Modify: `server/routes_openapi.go`

- [ ] **Step 1: Add metadata entries for the three endpoints**

In `server/routes_openapi.go`, follow the pattern used for other admin endpoints (e.g. `/v1/admin/server-stats`). Add three entries:

```go
{
	Method: http.MethodGet, Path: "/v1/admin/settings",
	Summary: "List runtime settings",
	Description: "Returns the 11 runtime-editable settings with current value, boot default, and override metadata.",
	Tags: []string{"admin"},
	Responses: map[int]openapi.Response{http.StatusOK: {Description: "List of SettingView objects"}},
},
{
	Method: http.MethodPut, Path: "/v1/admin/settings/{key}",
	Summary: "Set runtime setting override",
	Tags: []string{"admin"},
	PathParams: []openapi.Param{{Name: "key", Description: "Setting key, e.g. max_upload_size"}},
	RequestBody: openapi.RequestBody{Description: `{"value":"<string>","reason":"<optional>"}`},
	Responses: map[int]openapi.Response{
		http.StatusOK:        {Description: "Updated SettingView"},
		http.StatusBadRequest: {Description: "Invalid value / out of bounds"},
		http.StatusNotFound:   {Description: "Unknown setting key"},
	},
},
{
	Method: http.MethodDelete, Path: "/v1/admin/settings/{key}",
	Summary: "Reset runtime setting to boot default",
	Tags: []string{"admin"},
	PathParams: []openapi.Param{{Name: "key"}},
	RequestBody: openapi.RequestBody{Description: `{"reason":"<optional>"}`},
	Responses: map[int]openapi.Response{http.StatusOK: {Description: "SettingView with boot default"}},
},
```

Adapt field names to whatever shape `routes_openapi.go` actually uses (this file's types are local to the repo — match existing entries).

- [ ] **Step 2: Regenerate and diff**

Run: `go run ./cmd/openapi-gen -output openapi.yaml && git diff openapi.yaml | head -40`
Expected: three new path entries appear. No removals.

- [ ] **Step 3: Run OpenAPI validator**

Run: `go run ./cmd/openapi-gen/validate.go openapi.yaml`
Expected: no validation errors.

- [ ] **Step 4: Commit**

```bash
git add server/routes_openapi.go openapi.yaml
git commit -m "feat(api): OpenAPI metadata for admin settings endpoints"
```

---

## Phase 5 — UI

### Task 17: Admin settings template + context provider + nav

**Files:**
- Create: `server/template_handlers/template_context_providers/admin_settings_template_context.go`
- Create: `templates/adminSettings.tpl`
- Modify: `server/routes.go` (add `/admin/settings` to the template routes table — line 99-102)
- Modify: an existing admin nav include (find via grep)

- [ ] **Step 1: Write the context provider**

`server/template_handlers/template_context_providers/admin_settings_template_context.go`:

```go
package template_context_providers

import (
	"net/http"

	"mahresources/application_context"
)

func AdminSettingsContextProvider(ctx *application_context.MahresourcesContext, r *http.Request) (map[string]interface{}, error) {
	views := ctx.Settings().List()
	groups := groupByGroup(views)
	return map[string]interface{}{
		"title":       "Settings",
		"settingsByGroup": groups,
		"bootOnly":    bootOnlyFields(ctx.Config),
	}, nil
}

type settingsGroupView struct {
	Group string
	Items []application_context.SettingView
}

func groupByGroup(views []application_context.SettingView) []settingsGroupView {
	var out []settingsGroupView
	cur := settingsGroupView{}
	for _, v := range views {
		if string(v.Group) != cur.Group {
			if cur.Group != "" {
				out = append(out, cur)
			}
			cur = settingsGroupView{Group: string(v.Group)}
		}
		cur.Items = append(cur.Items, v)
	}
	if cur.Group != "" {
		out = append(out, cur)
	}
	return out
}

// bootOnlyFields returns a read-only snapshot of restart-only settings, shown
// in the collapsible "Requires restart" section at the bottom.
func bootOnlyFields(cfg *application_context.MahresourcesConfig) []map[string]string {
	return []map[string]string{
		{"label": "DB type", "value": cfg.DbType},
		{"label": "Bind address", "value": cfg.BindAddress},
		{"label": "File save path", "value": cfg.FileSavePath},
		{"label": "Ephemeral mode", "value": boolStr(cfg.MemoryDB || cfg.MemoryFS)},
		{"label": "Share port", "value": cfg.SharePort},
		{"label": "FTS enabled", "value": boolStr(!cfg.SkipFTS)},
		{"label": "Plugin path", "value": cfg.PluginPath},
	}
}

func boolStr(b bool) string { if b { return "yes" }; return "no" }
```

- [ ] **Step 2: Register the template route**

In `server/routes.go`, extend the `templateRoutes` map (around line 99-102):

```go
"/admin/settings": {template_context_providers.AdminSettingsContextProvider, "adminSettings.tpl", http.MethodGet},
```

- [ ] **Step 3: Write the template**

`templates/adminSettings.tpl`:

```html
{% extends "base.tpl" %}

{% block title %}Settings — {{ block.super }}{% endblock %}

{% block content %}
<div class="container mx-auto p-4">
  <h1 class="text-2xl font-bold mb-4">Settings</h1>
  <p class="text-sm text-gray-600 mb-6">Runtime overrides take effect immediately. Boot defaults shown for reference.</p>

  {% for group in settingsByGroup %}
    <section class="mb-8" aria-labelledby="grp-{{ group.Group }}">
      <h2 id="grp-{{ group.Group }}" class="text-xl font-semibold mb-3 capitalize">{{ group.Group|replace:"_":" " }}</h2>
      <div class="space-y-4">
        {% for s in group.Items %}
          <div class="border rounded p-4" x-data="settingRow({{ s|tojson }})">
            <label for="setting-{{ s.Key }}" class="block font-medium">{{ s.Label }}
              {% if s.Overridden %}<span class="ml-2 text-xs bg-yellow-100 text-yellow-800 px-2 py-0.5 rounded">Override</span>{% endif %}
            </label>
            <p class="text-sm text-gray-600 mt-1">{{ s.Description }}</p>
            <div class="mt-2 flex gap-2 items-start">
              <input id="setting-{{ s.Key }}" type="text" x-model="value"
                class="border rounded px-2 py-1 flex-1" />
              <input type="text" placeholder="Reason (optional)" x-model="reason"
                class="border rounded px-2 py-1 w-48" aria-label="Reason for {{ s.Label }}" />
              <button type="button" @click="save()" class="bg-blue-600 text-white px-3 py-1 rounded">Save</button>
              <template x-if="overridden">
                <button type="button" @click="reset()" class="bg-gray-200 px-3 py-1 rounded">Reset</button>
              </template>
            </div>
            <div class="text-xs text-gray-500 mt-1">
              Boot default: <code>{{ s.BootDefault }}</code>
              {% if s.MinNumeric %}<span class="ml-4">Min: <code>{{ s.MinNumeric }}</code></span>{% endif %}
              {% if s.MaxNumeric %}<span class="ml-4">Max: <code>{{ s.MaxNumeric }}</code></span>{% endif %}
            </div>
            <div class="text-sm mt-1 min-h-[1.25rem]" role="status" aria-live="polite">
              <span x-show="flash" x-text="flash" class="text-green-700"></span>
              <span x-show="error" x-text="error" class="text-red-700"></span>
            </div>
          </div>
        {% endfor %}
      </div>
    </section>
  {% endfor %}

  <details class="mt-10">
    <summary class="cursor-pointer font-medium">Boot-only settings (require restart)</summary>
    <table class="mt-3 text-sm w-full">
      <thead><tr><th class="text-left p-2">Setting</th><th class="text-left p-2">Value</th></tr></thead>
      <tbody>
        {% for f in bootOnly %}
          <tr class="border-t"><td class="p-2">{{ f.label }}</td><td class="p-2"><code>{{ f.value }}</code></td></tr>
        {% endfor %}
      </tbody>
    </table>
  </details>
</div>

<script>
function settingRow(initial) {
  return {
    key: initial.key,
    value: formatInitial(initial),
    reason: '',
    overridden: initial.overridden,
    flash: '',
    error: '',
    async save() {
      this.error = ''; this.flash = '';
      const res = await fetch(`/v1/admin/settings/${this.key}`, {
        method: 'PUT', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({value: this.value, reason: this.reason})
      });
      const body = await res.json();
      if (!res.ok) { this.error = body.error || `HTTP ${res.status}`; return; }
      this.overridden = body.overridden; this.flash = `Saved — took effect at ${new Date().toLocaleTimeString()}`;
    },
    async reset() {
      this.error = ''; this.flash = '';
      const res = await fetch(`/v1/admin/settings/${this.key}`, {
        method: 'DELETE', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({reason: this.reason})
      });
      const body = await res.json();
      if (!res.ok) { this.error = body.error || `HTTP ${res.status}`; return; }
      this.overridden = false; this.value = formatInitial(body); this.flash = 'Reset to boot default';
    }
  };
}
function formatInitial(s) {
  if (s.type === 'duration') return nanosToShort(s.current);
  return String(s.current);
}
function nanosToShort(n) {
  // Pretty-print nanoseconds as a time.Duration-compatible string.
  const ms = Math.floor(n / 1e6);
  if (ms < 1000) return `${ms}ms`;
  const s = Math.floor(ms / 1000);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  return `${h}h${m % 60 ? `${m%60}m` : ''}`;
}
</script>
{% endblock %}
```

The `tojson` Pongo2 filter may need adding; if not present, replace `x-data="settingRow({{ s|tojson }})"` with `x-data="settingRow({key:'{{ s.Key }}', type:'{{ s.Type }}', current: {{ s.Current|tojson|default:'null' }}, overridden: {{ s.Overridden|lower }}})"`. Check what filters existing templates use by `grep tojson templates/` first and adapt.

- [ ] **Step 4: Add nav link**

Find the existing admin-nav include (grep for `/admin/overview` in templates). Append:

```html
<a href="/admin/settings" class="nav-link">Settings</a>
```

Match the existing link styling.

- [ ] **Step 5: Build the app and smoke-test the page**

Run:

```bash
npm run build && ./mahresources -ephemeral -bind-address=:19192 &
sleep 2
curl -s http://localhost:19192/admin/settings | head -100
kill %1
```

Expected: HTML returned with `Settings` heading and at least one row per group.

- [ ] **Step 6: Commit**

```bash
git add server/template_handlers/template_context_providers/admin_settings_template_context.go templates/adminSettings.tpl server/routes.go # + nav file
git commit -m "feat(ui): /admin/settings page"
```

---

### Task 18: Browser E2E + accessibility

**Files:**
- Create: `e2e/tests/admin-settings.spec.ts`
- Create: `e2e/tests/accessibility/admin-settings.a11y.spec.ts`

- [ ] **Step 1: Write the E2E spec**

`e2e/tests/admin-settings.spec.ts`:

```typescript
import { test, expect } from '../fixtures/base.fixture';

test.describe('/admin/settings', () => {
  test('renders 11 settings grouped', async ({ page }) => {
    await page.goto('/admin/settings');
    await expect(page.getByRole('heading', { name: 'Settings' })).toBeVisible();
    const groups = await page.locator('section[aria-labelledby^="grp-"]').count();
    expect(groups).toBeGreaterThanOrEqual(6); // Uploads, Queries, Remote, Sharing, Dedup, Exports
    const rows = await page.locator('[x-data^="settingRow"]').count();
    expect(rows).toBe(11);
  });

  test('save + reset max_upload_size roundtrip', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const row = page.locator('[x-data^="settingRow"]').filter({ hasText: 'Max upload size' });
    await row.locator('input[type="text"]').first().fill('1048576');
    await row.getByPlaceholder('Reason (optional)').fill('e2e-save');
    await row.getByRole('button', { name: 'Save' }).click();
    await expect(row.getByText(/Saved — took effect/)).toBeVisible();
    await expect(row.getByText('Override')).toBeVisible();

    // Verify via API
    const listResp = await request.get('/v1/admin/settings');
    const list = await listResp.json();
    const mus = list.find((s: any) => s.key === 'max_upload_size');
    expect(mus.overridden).toBe(true);

    // Reset
    await row.getByRole('button', { name: 'Reset' }).click();
    await expect(row.getByText(/Reset to boot default/)).toBeVisible();
    await expect(row.getByText('Override')).not.toBeVisible();
  });

  test('out-of-bounds shows inline error, nothing persisted', async ({ page, request }) => {
    await page.goto('/admin/settings');
    const row = page.locator('[x-data^="settingRow"]').filter({ hasText: 'Max upload size' });
    await row.locator('input[type="text"]').first().fill('1');
    await row.getByRole('button', { name: 'Save' }).click();
    await expect(row.getByRole('status').getByText(/out of bounds|invalid/i)).toBeVisible();

    const listResp = await request.get('/v1/admin/settings');
    const list = await listResp.json();
    const mus = list.find((s: any) => s.key === 'max_upload_size');
    expect(mus.overridden).toBe(false);
  });

  test('boot-only section lists restart-required settings', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.getByText('Boot-only settings (require restart)').click();
    await expect(page.getByText('Bind address')).toBeVisible();
    await expect(page.getByText('File save path')).toBeVisible();
  });
});
```

- [ ] **Step 2: Write the a11y spec**

`e2e/tests/accessibility/admin-settings.a11y.spec.ts`:

```typescript
import { test, expect } from '../../fixtures/a11y.fixture';

test.describe('a11y: /admin/settings', () => {
  test('passes axe checks', async ({ page, makeAxeBuilder }) => {
    await page.goto('/admin/settings');
    const results = await makeAxeBuilder().analyze();
    expect(results.violations).toEqual([]);
  });
});
```

- [ ] **Step 3: Run E2E tests**

Run:

```bash
cd e2e && npm run test:with-server -- --grep 'admin/settings'
cd e2e && npm run test:with-server:a11y -- --grep 'admin/settings'
```

Expected: PASS. If failures, inspect `e2e/playwright-report/` and adjust selectors.

- [ ] **Step 4: Commit**

```bash
git add e2e/tests/admin-settings.spec.ts e2e/tests/accessibility/admin-settings.a11y.spec.ts
git commit -m "test(e2e): /admin/settings browser + a11y"
```

---

## Phase 6 — CLI

### Task 19: Restructure `mr admin` into command group

**Files:**
- Modify: `cmd/mr/commands/admin.go`
- Modify: `cmd/mr/commands/admin_help/admin.md` (now group help)
- Create: `cmd/mr/commands/admin_help/admin_stats.md` (former admin.md content)

- [ ] **Step 1: Move current admin.md content to admin_stats.md**

Keep a copy of the existing `cmd/mr/commands/admin_help/admin.md`. Rename the file to `admin_stats.md` and replace `admin.md` with group help:

`cmd/mr/commands/admin_help/admin.md`:

```markdown
# admin

Commands for server administration: view runtime stats, manage settings.

## Subcommands

- `stats` — Show server and data statistics (default).
- `settings` — View and manage runtime configuration overrides.

## Examples

List subcommands:

    mr admin --help

Show server stats (default behavior, same as `mr admin stats`):

    mr admin
```

(Former content now lives in `admin_stats.md`.)

- [ ] **Step 2: Refactor admin.go into a group**

Replace `NewAdminCmd` in `cmd/mr/commands/admin.go` with a group that delegates to sub-commands:

```go
func NewAdminCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(adminHelpFS, "admin_help/admin.md")
	cmd := &cobra.Command{
		Use:         "admin",
		Short:       "Server administration commands",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
	}
	// Bare `mr admin` delegates to `mr admin stats` for backward compatibility.
	statsCmd := NewAdminStatsCmd(c, opts)
	cmd.RunE = statsCmd.RunE
	cmd.Flags().AddFlagSet(statsCmd.Flags())

	cmd.AddCommand(statsCmd)
	cmd.AddCommand(NewAdminSettingsCmd(c, opts))
	return cmd
}

// NewAdminStatsCmd is the old NewAdminCmd body, verbatim, but with
// Use: "stats" and the help file path changed to admin_help/admin_stats.md.
func NewAdminStatsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var serverOnly bool
	var dataOnly bool
	help := helptext.Load(adminHelpFS, "admin_help/admin_stats.md")
	cmd := &cobra.Command{
		Use:         "stats",
		Short:       "Show server and data statistics",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		// RunE: ... (paste the existing RunE body unchanged)
	}
	cmd.Flags().BoolVar(&serverOnly, "server", false, "Show only server statistics")
	cmd.Flags().BoolVar(&dataOnly, "data", false, "Show only data statistics")
	return cmd
}
```

`NewAdminSettingsCmd` is a placeholder for Task 20. For now:

```go
func NewAdminSettingsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{Use: "settings", Short: "Manage runtime settings (filled in next task)"}
}
```

- [ ] **Step 3: Build and run a smoke test**

Run:

```bash
go build --tags 'json1 fts5' -o mr ./cmd/mr
./mr admin --help | head
./mr admin stats --help | head
./mr admin settings --help | head
```

Expected: each prints its own help.

- [ ] **Step 4: Run existing CLI E2E suite**

Run: `cd e2e && npm run test:with-server:cli -- --grep admin`
Expected: existing `mr admin` tests still pass — bare command still emits stats.

- [ ] **Step 5: Commit**

```bash
git add cmd/mr/commands/admin.go cmd/mr/commands/admin_help/admin.md cmd/mr/commands/admin_help/admin_stats.md
git commit -m "refactor(cli): admin becomes a command group with stats subcommand"
```

---

### Task 20: `mr admin settings` subcommands

**Files:**
- Modify: `cmd/mr/commands/admin.go` (replace placeholder `NewAdminSettingsCmd`)
- Create: `cmd/mr/commands/admin_help/admin_settings.md`, `admin_settings_list.md`, `admin_settings_get.md`, `admin_settings_set.md`, `admin_settings_reset.md`

- [ ] **Step 1: Implement settings subcommands**

Append to `cmd/mr/commands/admin.go`:

```go
type adminSettingView struct {
	Key         string      `json:"key"`
	Label       string      `json:"label"`
	Description string      `json:"description"`
	Group       string      `json:"group"`
	Type        string      `json:"type"`
	Current     interface{} `json:"current"`
	BootDefault interface{} `json:"bootDefault"`
	Overridden  bool        `json:"overridden"`
	UpdatedAt   *time.Time  `json:"updatedAt,omitempty"`
	Reason      string      `json:"reason,omitempty"`
}

func NewAdminSettingsCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(adminHelpFS, "admin_help/admin_settings.md")
	cmd := &cobra.Command{
		Use:         "settings",
		Short:       "View and manage runtime settings",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
	}
	cmd.AddCommand(NewAdminSettingsListCmd(c, opts))
	cmd.AddCommand(NewAdminSettingsGetCmd(c, opts))
	cmd.AddCommand(NewAdminSettingsSetCmd(c, opts))
	cmd.AddCommand(NewAdminSettingsResetCmd(c, opts))
	return cmd
}

func NewAdminSettingsListCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(adminHelpFS, "admin_help/admin_settings_list.md")
	return &cobra.Command{
		Use: "list", Short: "List runtime settings",
		Long: help.Long, Example: help.Example, Annotations: help.Annotations,
		RunE: func(_ *cobra.Command, _ []string) error {
			var views []adminSettingView
			if err := c.Get("/v1/admin/settings", nil, &views); err != nil {
				return err
			}
			if opts.JSON {
				return output.PrintJSON(views)
			}
			return printSettingsTable(views)
		},
	}
}

func NewAdminSettingsGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(adminHelpFS, "admin_help/admin_settings_get.md")
	return &cobra.Command{
		Use: "get <key>", Short: "Show a single runtime setting",
		Args: cobra.ExactArgs(1),
		Long: help.Long, Example: help.Example, Annotations: help.Annotations,
		RunE: func(_ *cobra.Command, args []string) error {
			var views []adminSettingView
			if err := c.Get("/v1/admin/settings", nil, &views); err != nil {
				return err
			}
			for _, v := range views {
				if v.Key == args[0] {
					if opts.JSON {
						return output.PrintJSON(v)
					}
					return printSettingsTable([]adminSettingView{v})
				}
			}
			return fmt.Errorf("unknown setting %q", args[0])
		},
	}
}

func NewAdminSettingsSetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(adminHelpFS, "admin_help/admin_settings_set.md")
	var reason string
	cmd := &cobra.Command{
		Use: "set <key> <value>", Short: "Override a runtime setting",
		Args: cobra.ExactArgs(2),
		Long: help.Long, Example: help.Example, Annotations: help.Annotations,
		RunE: func(_ *cobra.Command, args []string) error {
			body := map[string]string{"value": args[1], "reason": reason}
			var view adminSettingView
			if err := c.Put(fmt.Sprintf("/v1/admin/settings/%s", args[0]), body, &view); err != nil {
				return err
			}
			if opts.JSON {
				return output.PrintJSON(view)
			}
			return printSettingsTable([]adminSettingView{view})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for the change (goes into audit log)")
	return cmd
}

func NewAdminSettingsResetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(adminHelpFS, "admin_help/admin_settings_reset.md")
	var reason string
	cmd := &cobra.Command{
		Use: "reset <key>", Short: "Revert a runtime setting to boot default",
		Args: cobra.ExactArgs(1),
		Long: help.Long, Example: help.Example, Annotations: help.Annotations,
		RunE: func(_ *cobra.Command, args []string) error {
			body := map[string]string{"reason": reason}
			var view adminSettingView
			if err := c.Delete(fmt.Sprintf("/v1/admin/settings/%s", args[0]), body, &view); err != nil {
				return err
			}
			if opts.JSON {
				return output.PrintJSON(view)
			}
			return printSettingsTable([]adminSettingView{view})
		},
	}
	cmd.Flags().StringVar(&reason, "reason", "", "Reason for the reset (goes into audit log)")
	return cmd
}

func printSettingsTable(views []adminSettingView) error {
	t := output.NewTable("KEY", "GROUP", "CURRENT", "BOOT DEFAULT", "OVERRIDDEN", "UPDATED")
	for _, v := range views {
		updated := ""
		if v.UpdatedAt != nil {
			updated = v.UpdatedAt.Format("2006-01-02 15:04:05")
		}
		t.AddRow(v.Key, v.Group, fmt.Sprintf("%v", v.Current), fmt.Sprintf("%v", v.BootDefault), fmt.Sprintf("%v", v.Overridden), updated)
	}
	return t.Render()
}
```

Verify the `client.Client` exposes `Put` and `Delete` methods. If not, add them by following the existing `Get`/`Post` pattern. Likewise, `output.NewTable` and `output.PrintJSON` — adapt to existing helpers; match what `admin.go` already uses for stats output.

- [ ] **Step 2: Write help markdowns**

`cmd/mr/commands/admin_help/admin_settings.md`:

```markdown
# admin settings

View and manage runtime configuration overrides. Overrides persist to the
database and take effect immediately without restarting the server.

## Subcommands

- `list` — Show all 11 runtime settings.
- `get <key>` — Show a single setting.
- `set <key> <value>` — Override a setting.
- `reset <key>` — Remove the override and revert to boot default.

## Examples

    mr admin settings list
    mr admin settings get max_upload_size
    mr admin settings set max_upload_size 2G --reason "increase for video"
    mr admin settings reset max_upload_size --reason "revert"
```

`cmd/mr/commands/admin_help/admin_settings_list.md`:

```markdown
# admin settings list

List all runtime-editable settings with current value, boot default, override
status, and update timestamp.

## Flags

- `--json` — Emit raw JSON instead of a table.

## Examples

    mr admin settings list
    mr admin settings list --json
```

`cmd/mr/commands/admin_help/admin_settings_get.md`:

```markdown
# admin settings get

Show a single runtime setting by key.

## Arguments

- `<key>` — Setting key, e.g. `max_upload_size`.

## Examples

    mr admin settings get max_upload_size
    mr admin settings get mrql_query_timeout --json
```

`cmd/mr/commands/admin_help/admin_settings_set.md`:

```markdown
# admin settings set

Override a runtime setting. The override persists to the database and takes
effect on the next use of the setting — no restart required.

Size values accept K/M/G/T suffixes (base 2: 1K = 1024). Duration values use
Go's time.ParseDuration format ("30s", "5m", "2h").

## Arguments

- `<key>` — Setting key (see `mr admin settings list`).
- `<value>` — New value; format depends on the setting's type.

## Flags

- `--reason` — Free-text note recorded in the audit log.

## Examples

    mr admin settings set max_upload_size 2G --reason "increase for video workflow"
    mr admin settings set mrql_query_timeout 30s
    mr admin settings set share_public_url https://share.example.com
```

`cmd/mr/commands/admin_help/admin_settings_reset.md`:

```markdown
# admin settings reset

Remove a runtime override and revert the setting to its boot-time default.

## Arguments

- `<key>` — Setting key.

## Flags

- `--reason` — Free-text note recorded in the audit log.

## Examples

    mr admin settings reset max_upload_size
    mr admin settings reset mrql_query_timeout --reason "back to default"
```

- [ ] **Step 3: Run docs lint and check-examples**

Run:

```bash
./mr docs lint
./mr docs check-examples --grep settings
```

Expected: both pass.

- [ ] **Step 4: Commit**

```bash
git add cmd/mr/commands/admin.go cmd/mr/commands/admin_help/admin_settings*.md
git commit -m "feat(cli): mr admin settings list/get/set/reset"
```

---

### Task 21: CLI E2E tests

**Files:**
- Create: `e2e/tests/cli/admin-settings-list.spec.ts`
- Create: `e2e/tests/cli/admin-settings-set-reset.spec.ts`
- Create: `e2e/tests/cli/admin-settings-bounds.spec.ts`

- [ ] **Step 1: Write `admin-settings-list.spec.ts`**

```typescript
import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings list', () => {
  test('lists 11 settings', async ({ cli }) => {
    const { stdout, exitCode } = await cli.run(['admin', 'settings', 'list']);
    expect(exitCode).toBe(0);
    expect(stdout).toContain('max_upload_size');
    expect(stdout).toContain('mrql_query_timeout');
    const rows = stdout.split('\n').filter(l => l.match(/^[a-z_]+\s/));
    expect(rows.length).toBeGreaterThanOrEqual(11);
  });

  test('--json emits parseable JSON', async ({ cli }) => {
    const { stdout, exitCode } = await cli.run(['admin', 'settings', 'list', '--json']);
    expect(exitCode).toBe(0);
    const parsed = JSON.parse(stdout);
    expect(Array.isArray(parsed)).toBe(true);
    expect(parsed.length).toBe(11);
  });
});
```

- [ ] **Step 2: Write `admin-settings-set-reset.spec.ts`**

```typescript
import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings set/reset', () => {
  test('round-trip on max_upload_size', async ({ cli }) => {
    const set = await cli.run(['admin', 'settings', 'set', 'max_upload_size', '1M', '--reason', 'cli-e2e']);
    expect(set.exitCode).toBe(0);

    const list1 = await cli.run(['admin', 'settings', 'list', '--json']);
    const views1 = JSON.parse(list1.stdout);
    const mus1 = views1.find((v: any) => v.key === 'max_upload_size');
    expect(mus1.overridden).toBe(true);
    expect(mus1.current).toBe(1 << 20);

    const reset = await cli.run(['admin', 'settings', 'reset', 'max_upload_size', '--reason', 'cli-e2e-revert']);
    expect(reset.exitCode).toBe(0);

    const list2 = await cli.run(['admin', 'settings', 'list', '--json']);
    const mus2 = JSON.parse(list2.stdout).find((v: any) => v.key === 'max_upload_size');
    expect(mus2.overridden).toBe(false);
  });
});
```

- [ ] **Step 3: Write `admin-settings-bounds.spec.ts`**

```typescript
import { test, expect } from '../../fixtures/cli.fixture';

test.describe('mr admin settings bounds', () => {
  test('rejects below-min with nonzero exit and stderr message', async ({ cli }) => {
    const { exitCode, stderr } = await cli.run(['admin', 'settings', 'set', 'max_upload_size', '1']);
    expect(exitCode).not.toBe(0);
    expect(stderr.toLowerCase()).toMatch(/out of bounds|invalid/);
  });

  test('rejects unknown key', async ({ cli }) => {
    const { exitCode } = await cli.run(['admin', 'settings', 'set', 'not_a_key', '1']);
    expect(exitCode).not.toBe(0);
  });
});
```

- [ ] **Step 4: Run CLI E2E**

Run: `cd e2e && npm run test:with-server:cli -- --grep admin-settings`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add e2e/tests/cli/admin-settings-list.spec.ts e2e/tests/cli/admin-settings-set-reset.spec.ts e2e/tests/cli/admin-settings-bounds.spec.ts
git commit -m "test(cli-e2e): mr admin settings coverage"
```

---

## Phase 7 — Docs Site

### Task 22: Docs-site pages

**Files:**
- Create: `docs-site/docs/configuration/runtime-settings.md`
- Modify: `docs-site/docs/configuration/overview.md` (add link section)
- Restructure: `docs-site/docs/cli/admin.md` → `docs-site/docs/cli/admin/index.md`, `stats.md`, `settings.md`

- [ ] **Step 1: Create the configuration page**

`docs-site/docs/configuration/runtime-settings.md`:

```markdown
---
sidebar_position: 5
---

# Runtime Settings

Most configuration flags bind once at startup. A curated subset can be
overridden at runtime via the `/admin/settings` page, the `mr admin settings`
CLI, or the `/v1/admin/settings` HTTP API — no restart required.

## How precedence works

1. Boot flag / env var supplies the initial value.
2. If the `runtime_settings` table has a row for the key, that override wins.
3. When a flag is set *and* an override differs from it, one WARN line is
   logged at startup so operators are not silently ignored.

Reset via the UI (Reset button), CLI (`mr admin settings reset <key>`), or API
(`DELETE /v1/admin/settings/<key>`) removes the override and returns to the
boot value.

## Runtime-editable settings

| Key | Type | Bounds | Boot flag | Takes effect |
| --- | --- | --- | --- | --- |
| `max_upload_size` | int64 (bytes) | 1 KiB–1 TiB; 0 = unlimited | `-max-upload-size` | next upload request |
| `max_import_size` | int64 (bytes) | 1 MiB–1 TiB | `-max-import-size` | next import parse |
| `mrql_default_limit` | int | 1–100000 | `-mrql-default-limit` | next MRQL query |
| `mrql_query_timeout` | duration | 100ms–5m | `-mrql-query-timeout` | next MRQL query |
| `export_retention` | duration | 1m–30d | `-export-retention` | next sweep + UI disclosure |
| `remote_connect_timeout` | duration | 1s–10m | `-remote-connect-timeout` | next remote download |
| `remote_idle_timeout` | duration | 1s–1h | `-remote-idle-timeout` | next remote download |
| `remote_overall_timeout` | duration | 10s–24h | `-remote-overall-timeout` | next remote download |
| `share_public_url` | string (http/https URL) | absolute; non-empty host | `-share-public-url` | next share link render |
| `hash_similarity_threshold` | int | 0–64 | `-hash-similarity-threshold` | next hash comparison |
| `hash_ahash_threshold` | uint64 | 0–64; 0 disables | `-hash-ahash-threshold` | next hash comparison |

## Audit trail

Every change writes a row to `log_entries` with `entity_type=runtime_setting`,
the key as `entity_name`, old→new values in `message`, and the request IP in
`ip_address`. Visible in the admin log view at `/admin/overview`.

## CLI reference

See `mr admin settings` — [`list`](../cli/admin/settings.md#list),
[`get`](../cli/admin/settings.md#get),
[`set`](../cli/admin/settings.md#set),
[`reset`](../cli/admin/settings.md#reset).
```

- [ ] **Step 2: Update configuration overview**

In `docs-site/docs/configuration/overview.md`, add a section near the top:

```markdown
## Runtime vs. boot-only settings

Most flags apply only at startup. A [curated subset](./runtime-settings.md) can
be changed at runtime via the admin UI, CLI, or API — no restart needed.

Boot-only settings include: database DSN, bind addresses, file save path,
ephemeral mode, alt filesystems, share port, FTS initialization, worker pool
sizes, and max DB connections.
```

- [ ] **Step 3: Restructure the CLI admin docs**

Create `docs-site/docs/cli/admin/` directory. Move the existing `cli/admin.md` content into three files:

`docs-site/docs/cli/admin/index.md`:

```markdown
---
sidebar_position: 1
---

# admin

The `mr admin` command group covers server administration — runtime stats and
runtime configuration management.

## Subcommands

- [`stats`](./stats.md) — Show server and data statistics (the default).
- [`settings`](./settings.md) — View and manage runtime configuration overrides.

## Examples

    mr admin                  # shorthand for `mr admin stats`
    mr admin stats --server   # server stats only
    mr admin settings list
```

`docs-site/docs/cli/admin/stats.md` — paste the content of the current `cli/admin.md`, change the H1 to `# admin stats`.

`docs-site/docs/cli/admin/settings.md`:

```markdown
---
sidebar_position: 3
---

# admin settings

Runtime configuration management. See [Runtime Settings](../../configuration/runtime-settings.md) for the conceptual overview.

## list {#list}

    mr admin settings list [--json]

Show all 11 runtime-editable settings.

## get {#get}

    mr admin settings get <key> [--json]

Show a single setting by key.

## set {#set}

    mr admin settings set <key> <value> [--reason <text>]

Override a setting. Size values accept K/M/G/T suffixes (base 2). Duration
values use Go's `time.ParseDuration` format.

Out-of-bounds values exit nonzero with a stderr message describing the valid range.

## reset {#reset}

    mr admin settings reset <key> [--reason <text>]

Remove the override and revert to boot default.

## Examples

    mr admin settings list
    mr admin settings set max_upload_size 2G --reason "increase for video workflow"
    mr admin settings set mrql_query_timeout 30s
    mr admin settings reset max_upload_size
```

Delete the now-redundant `docs-site/docs/cli/admin.md`.

- [ ] **Step 4: Capture the screenshot (manual step, flagged in review checklist)**

After Task 17 lands and the server is seeded, run the `retake-screenshots` skill to refresh the manifest. Add `/admin/settings` to the screenshot list if not already there. Commit the new screenshot under `docs-site/static/img/screenshots/`. Reference it in `runtime-settings.md`:

```markdown
![Admin settings page](/img/screenshots/admin-settings.png)
```

If the skill runs can't be done in this session, leave a TODO comment in the markdown: `<!-- SCREENSHOT: /admin/settings — regenerate via retake-screenshots skill -->`.

- [ ] **Step 5: Build docs-site**

Run:

```bash
cd docs-site && npm run build
```

Expected: no broken links, all markdown compiles.

- [ ] **Step 6: Commit**

```bash
git add docs-site/docs/configuration/runtime-settings.md docs-site/docs/configuration/overview.md docs-site/docs/cli/admin
git rm docs-site/docs/cli/admin.md
git commit -m "docs: runtime settings page + cli/admin restructure"
```

---

## Phase 8 — Full Verification

### Task 23: End-to-end verification pass

**Files:** none (verification only)

- [ ] **Step 1: Full Go test suite (SQLite)**

Run: `go test --tags 'json1 fts5' -race ./...`
Expected: PASS.

- [ ] **Step 2: Full Go test suite (Postgres)**

Requires Docker running.

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`
Expected: PASS.

- [ ] **Step 3: Browser E2E**

Run: `cd e2e && npm run test:with-server`
Expected: PASS.

- [ ] **Step 4: CLI E2E**

Run: `cd e2e && npm run test:with-server:cli`
Expected: PASS.

- [ ] **Step 5: Postgres E2E**

Run: `cd e2e && npm run test:with-server:postgres`
Expected: PASS.

- [ ] **Step 6: Docs lint & examples**

Run:

```bash
./mr docs lint
./mr docs check-examples
```

Expected: both pass.

- [ ] **Step 7: Smoke test boot conflict log**

```bash
./mahresources -ephemeral -bind-address=:19193 &
sleep 2
# Set an override via API
curl -sX PUT -H 'Content-Type: application/json' -d '{"value":"1M","reason":"smoke"}' http://localhost:19193/v1/admin/settings/max_upload_size
kill %1
# Boot again with a different flag and point at the same seed DB — verify WARN
./mahresources -ephemeral -max-upload-size=1G -bind-address=:19194 2>&1 | grep -i 'runtime_setting.*override' | head -1
```

Expected: a WARN line mentioning `max_upload_size` divergence appears in the output. (The second run with `-ephemeral` re-creates the DB so the override is gone; adapt this to use persistent `-db-dsn` if you want the override to carry across — or simply assert the logic in a Go test.)

- [ ] **Step 8: Commit any in-flight fixes from the verification pass**

If any step uncovered a regression, fix it with a minimal commit, then re-run the full suite until all green.

- [ ] **Step 9: Tag the plan complete**

```bash
git log --oneline -30
```

Expected: ~20 feature commits, clean history.

---

## Spec Coverage Check (Plan Self-Review)

| Spec section | Plan tasks |
| --- | --- |
| Data Model (table + envelope) | Tasks 1, 2 |
| Service API (struct + getters + Set/Reset/List) | Tasks 3, 4 |
| Boot Sequence (Load + conflict log + bounds fallback) | Tasks 3, 5 |
| Live-Reread Refactors (5 locations) | Tasks 9, 10, 11, 12, 13 |
| HTTP API (GET/PUT/DELETE) | Tasks 14, 15 |
| UI (template, context provider, nav) | Task 17 |
| Validation & Bounds | Task 3 (validateBounds) + tests throughout |
| Precedence (DB wins, conflict log) | Task 3 (Load) |
| Audit (log_entries integration) | Task 6 |
| CLI (admin group + settings subgroup + help files) | Tasks 19, 20 |
| Docs Site (runtime-settings page + overview + cli/admin restructure) | Task 22 |
| Concurrency & Safety (RWMutex discipline) | Tasks 3, 6 (tests include `-race`) |
| Testing Plan (unit + API + boot + refactor + browser + CLI E2E + docs + postgres) | Tasks 3, 5, 6, 9, 10, 11, 13, 14, 15, 18, 21, 23 |

All 11 spec sections have implementation tasks. No gaps identified.
