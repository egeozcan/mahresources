package models

import (
	"log"
	"strconv"
)

// Hash status values stored in ImageHash.Status.
const (
	// HashStatusOK marks a successfully hashed row. Stored as "" for legacy rows
	// and "ok" for v2 rows so both read as "hashable".
	HashStatusOK = "ok"
	// HashStatusFailed marks a row whose file could not be decoded (corrupt,
	// missing, unsupported). Prevents infinite retry.
	HashStatusFailed = "failed"
	// HashStatusFlat marks an image with near-zero pixel variance (solid colour,
	// blank scans). Excluded from similarity matching to avoid false positives.
	HashStatusFlat = "flat"
)

type ImageHash struct {
	ID       uint   `gorm:"primarykey"`
	AHash    string `gorm:"index"` // old, kept during migration
	DHash    string `gorm:"index"` // old, kept during migration
	AHashInt *int64 `gorm:"index"` // stored as int64 (bit-reinterpreted from uint64 for PostgreSQL compatibility)
	DHashInt *int64 `gorm:"index"` // stored as int64 (bit-reinterpreted from uint64 for PostgreSQL compatibility)

	// v2 fields (image similarity v2). NULL HashVersion means a legacy v1 row.
	HashVersion *int   `gorm:"index"`                // NULL = v1 (legacy), 2 = v2 (goimagehash pHash)
	PHashInt    *int64 `gorm:"index"`                // v2 pHash, stored as int64 (bit-reinterpreted from uint64)
	PChunk0     *int32 `gorm:"index:idx_ih_pchunk0"` // pHash bits 0-15
	PChunk1     *int32 `gorm:"index:idx_ih_pchunk1"` // pHash bits 16-31
	PChunk2     *int32 `gorm:"index:idx_ih_pchunk2"` // pHash bits 32-47
	PChunk3     *int32 `gorm:"index:idx_ih_pchunk3"` // pHash bits 48-63
	Status      string `gorm:"index"`                // "" / "ok" / "failed" / "flat"

	Resource   *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ResourceId *uint     `gorm:"uniqueIndex"`
}

// GetDHash returns the DHash as uint64, preferring the new column
// and falling back to parsing the old string column.
// The int64 storage is bit-reinterpreted back to uint64.
func (h *ImageHash) GetDHash() uint64 {
	if h.DHashInt != nil {
		return uint64(*h.DHashInt)
	}
	if h.DHash == "" {
		return 0
	}
	val, err := parseHashString(h.DHash)
	if err != nil {
		log.Printf("Warning: failed to parse DHash %q for hash ID %d: %v", h.DHash, h.ID, err)
		return 0
	}
	return val
}

// GetAHash returns the AHash as uint64, preferring the new column
// and falling back to parsing the old string column.
// The int64 storage is bit-reinterpreted back to uint64.
func (h *ImageHash) GetAHash() uint64 {
	if h.AHashInt != nil {
		return uint64(*h.AHashInt)
	}
	if h.AHash == "" {
		return 0
	}
	val, err := parseHashString(h.AHash)
	if err != nil {
		log.Printf("Warning: failed to parse AHash %q for hash ID %d: %v", h.AHash, h.ID, err)
		return 0
	}
	return val
}

// parseHashString parses a hash string that may be in binary (64 chars of 0/1)
// or hexadecimal format (16 chars).
func parseHashString(s string) (uint64, error) {
	if len(s) == 64 && isBinaryString(s) {
		return strconv.ParseUint(s, 2, 64)
	}
	return strconv.ParseUint(s, 16, 64)
}

// isBinaryString returns true if s contains only '0' and '1' characters.
func isBinaryString(s string) bool {
	for _, c := range s {
		if c != '0' && c != '1' {
			return false
		}
	}
	return true
}

// IsMigrated returns true if this hash has been migrated to uint64 format.
func (h *ImageHash) IsMigrated() bool {
	return h.DHashInt != nil
}

// GetPHash returns the v2 pHash as uint64. The int64 storage is
// bit-reinterpreted back to uint64. Returns 0 when unset.
func (h *ImageHash) GetPHash() uint64 {
	if h.PHashInt != nil {
		return uint64(*h.PHashInt)
	}
	return 0
}

// IsV2 returns true when this row was computed by the v2 hash engine.
func (h *ImageHash) IsV2() bool {
	return h.HashVersion != nil && *h.HashVersion >= 2
}
