package application_context

import (
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"time"
)

// SettingType discriminates value encoding on disk.
//
// Both SettingTypeInt and SettingTypeInt64 exist deliberately — the typed
// getters return the matching Go type (int vs int64) so callers don't have
// to cast. The on-disk envelope carries the discriminator, so a blob encoded
// as "int" cannot be silently decoded as "int64" (decodeSettingValue returns
// a type-mismatch error before returning any value).
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
			return nil, fmt.Errorf("decode %s value: %w", typ, err)
		}
		return v, nil
	case string(SettingTypeInt):
		var v int
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, fmt.Errorf("decode %s value: %w", typ, err)
		}
		return v, nil
	case string(SettingTypeUint64):
		var v uint64
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, fmt.Errorf("decode %s value: %w", typ, err)
		}
		return v, nil
	case string(SettingTypeDuration):
		var nanos int64
		if err := json.Unmarshal(env.Value, &nanos); err != nil {
			return nil, fmt.Errorf("decode %s value: %w", typ, err)
		}
		return time.Duration(nanos), nil
	case string(SettingTypeString):
		var v string
		if err := json.Unmarshal(env.Value, &v); err != nil {
			return nil, fmt.Errorf("decode %s value: %w", typ, err)
		}
		return v, nil
	}
	return nil, fmt.Errorf("decode: unknown type %q", typ)
}

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

// BuildSpecsExported is the main.go-visible accessor for the spec registry.
func BuildSpecsExported() map[string]SettingSpec { return buildSpecs() }

// BuildDefaultsFromConfig snapshots every in-scope setting from the boot-time
// MahresourcesConfig into a map keyed by spec key.
func BuildDefaultsFromConfig(cfg *MahresourcesConfig) map[string]any {
	return map[string]any{
		KeyMaxUploadSize:           cfg.MaxUploadSize,
		KeyMaxImportSize:           cfg.MaxImportSize,
		KeyMRQLDefaultLimit:        cfg.MRQLDefaultLimit,
		KeyMRQLQueryTimeout:        cfg.MRQLQueryTimeoutBoot,
		KeyExportRetention:         cfg.ExportRetention,
		KeyRemoteConnectTimeout:    cfg.RemoteResourceConnectTimeout,
		KeyRemoteIdleTimeout:       cfg.RemoteResourceIdleTimeout,
		KeyRemoteOverallTimeout:    cfg.RemoteResourceOverallTimeout,
		KeySharePublicURL:          cfg.SharePublicURL,
		KeyHashSimilarityThreshold: cfg.HashSimilarityThreshold,
		KeyHashAHashThreshold:      cfg.HashAHashThreshold,
	}
}

// NewStdlibSettingsLogger returns a SettingsLogger backed by the stdlib log package.
func NewStdlibSettingsLogger() SettingsLogger { return stdlibSettingsLogger{} }

type stdlibSettingsLogger struct{}

func (stdlibSettingsLogger) Warn(format string, args ...any)  { log.Printf("WARN: "+format, args...) }
func (stdlibSettingsLogger) Error(format string, args ...any) { log.Printf("ERROR: "+format, args...) }
