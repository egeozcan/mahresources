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
