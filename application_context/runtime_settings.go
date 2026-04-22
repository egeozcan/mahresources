package application_context

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

// ErrUnknownSetting is returned by Set/Reset when the key is not registered.
// HTTP handlers use errors.Is to map it to 404.
var ErrUnknownSetting = errors.New("unknown setting")

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

// Auditor writes an audit log entry. Production implementations delegate to the
// existing Logger (LogFromRequest). The interface keeps RuntimeSettings
// testable in isolation.
type Auditor interface {
	Audit(action, entityType, entityName, message string, details map[string]any, ipAddress string)
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
	auditor   Auditor
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
// Note: audit log_entries write happens in a later task (Task 6); this task
// focuses on the core Set/Reset/Load round-trip.
func (s *RuntimeSettings) Set(key, rawValue, reason, actor string) error {
	spec, ok := s.specs[key]
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownSetting, key)
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
}

// Reset removes the DB row and cache entry for the key.
func (s *RuntimeSettings) Reset(key, reason, actor string) error {
	if _, ok := s.specs[key]; !ok {
		return fmt.Errorf("%w: %q", ErrUnknownSetting, key)
	}
	if err := s.db.Delete(&models.RuntimeSetting{}, "key = ?", key).Error; err != nil {
		return fmt.Errorf("db delete: %w", err)
	}
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

// SetAuditor configures the auditor used by Set/Reset. If nil (the default),
// no audit row is written — useful in unit tests that don't care.
func (s *RuntimeSettings) SetAuditor(a Auditor) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auditor = a
}

// overrideOrDefaultLocked returns the current effective value under the lock.
// Caller must hold s.mu (read or write).
// The second return value is true when the value comes from an override,
// false when it falls back to the boot default.
func (s *RuntimeSettings) overrideOrDefaultLocked(key string) (any, bool) {
	if e, ok := s.overrides[key]; ok {
		return e.value, true
	}
	if d, ok := s.defaults[key]; ok {
		return d, false
	}
	return nil, false
}

// parseSettingValue turns the CLI/HTTP string form into the typed value.
// Accepts:
//
//	int64/int: decimal integer; byte suffixes (K, M, G, T; base 2) for size keys.
//	uint64:    decimal integer.
//	duration:  time.ParseDuration format (e.g. "30s", "5m", "2h").
//	string:    raw.
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
		mult, body = 1<<10, raw[:len(raw)-1]
	case 'M', 'm':
		mult, body = 1<<20, raw[:len(raw)-1]
	case 'G', 'g':
		mult, body = 1<<30, raw[:len(raw)-1]
	case 'T', 't':
		mult, body = 1<<40, raw[:len(raw)-1]
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
		n, ok := v.(int64)
		if !ok {
			return fmt.Errorf("validateBounds %q: want int64, got %T", spec.Key, v)
		}
		if n == 0 && spec.AllowZero {
			return nil
		}
		if n < spec.MinNumeric || n > spec.MaxNumeric {
			return fmt.Errorf("value %d out of bounds [%d, %d]", n, spec.MinNumeric, spec.MaxNumeric)
		}
	case SettingTypeInt:
		ni, ok := v.(int)
		if !ok {
			return fmt.Errorf("validateBounds %q: want int, got %T", spec.Key, v)
		}
		n := int64(ni)
		if n == 0 && spec.AllowZero {
			return nil
		}
		if n < spec.MinNumeric || n > spec.MaxNumeric {
			return fmt.Errorf("value %d out of bounds [%d, %d]", n, spec.MinNumeric, spec.MaxNumeric)
		}
	case SettingTypeUint64:
		u, ok := v.(uint64)
		if !ok {
			return fmt.Errorf("validateBounds %q: want uint64, got %T", spec.Key, v)
		}
		if u == 0 && spec.AllowZero {
			return nil
		}
		if int64(u) < spec.MinNumeric || int64(u) > spec.MaxNumeric {
			return fmt.Errorf("value %d out of bounds [%d, %d]", u, spec.MinNumeric, spec.MaxNumeric)
		}
	case SettingTypeDuration:
		dv, ok := v.(time.Duration)
		if !ok {
			return fmt.Errorf("validateBounds %q: want time.Duration, got %T", spec.Key, v)
		}
		d := int64(dv)
		if d < spec.MinNumeric || d > spec.MaxNumeric {
			return fmt.Errorf("duration %v out of bounds [%v, %v]", time.Duration(d), time.Duration(spec.MinNumeric), time.Duration(spec.MaxNumeric))
		}
	case SettingTypeString:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("validateBounds %q: want string, got %T", spec.Key, v)
		}
		if spec.StringValidator != nil {
			return spec.StringValidator(s)
		}
	}
	return nil
}

// groupDisplayOrder sets the UI ordering. Groups not listed fall to the end in
// alphabetical order (safety net for any future group that forgets to register
// here).
var groupDisplayOrder = []SettingGroup{
	GroupUploads,
	GroupQueries,
	GroupRemoteDownloads,
	GroupSharing,
	GroupDeduplication,
	GroupExports,
}

func groupOrderIndex(g SettingGroup) int {
	for i, known := range groupDisplayOrder {
		if known == g {
			return i
		}
	}
	return len(groupDisplayOrder) // unknown → sorts after all known groups
}

func sortedSpecKeys(m map[string]SettingSpec) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := m[keys[i]], m[keys[j]]
		ai, bi := groupOrderIndex(a.Group), groupOrderIndex(b.Group)
		if ai != bi {
			return ai < bi
		}
		return a.Key < b.Key
	})
	return keys
}

func ptrInt64(v int64) *int64 { return &v }

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

// DownloadSettings adapter methods — satisfy download_queue.DownloadSettings
// without leaking the download_queue package into the service layer.
func (s *RuntimeSettings) ConnectTimeout() time.Duration  { return s.RemoteConnectTimeout() }
func (s *RuntimeSettings) IdleTimeout() time.Duration     { return s.RemoteIdleTimeout() }
func (s *RuntimeSettings) OverallTimeout() time.Duration  { return s.RemoteOverallTimeout() }

// ExportRetention already exists on RuntimeSettings (see above).
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

// NewContextAuditor returns an Auditor that writes through the context's
// existing Logger infrastructure. IPAddress/RequestPath/UserAgent are captured
// automatically from the request via LogFromRequest — we don't pass them
// explicitly because the existing Logger pipeline already handles that.
func NewContextAuditor(ctx *MahresourcesContext) Auditor {
	return &contextAuditor{ctx: ctx}
}

type contextAuditor struct{ ctx *MahresourcesContext }

func (a *contextAuditor) Audit(action, entityType, entityName, message string, details map[string]any, _ string) {
	a.ctx.Logger().Info(action, entityType, nil, entityName, message, details)
}
