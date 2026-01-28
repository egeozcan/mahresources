package models

import (
	"log"
	"strconv"
)

type ImageHash struct {
	ID         uint      `gorm:"primarykey"`
	AHash      string    `gorm:"index"`  // old, kept during migration
	DHash      string    `gorm:"index"`  // old, kept during migration
	AHashInt   *int64    `gorm:"index"`  // stored as int64 (bit-reinterpreted from uint64 for PostgreSQL compatibility)
	DHashInt   *int64    `gorm:"index"`  // stored as int64 (bit-reinterpreted from uint64 for PostgreSQL compatibility)
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
	val, err := strconv.ParseUint(h.DHash, 16, 64)
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
	val, err := strconv.ParseUint(h.AHash, 16, 64)
	if err != nil {
		log.Printf("Warning: failed to parse AHash %q for hash ID %d: %v", h.AHash, h.ID, err)
		return 0
	}
	return val
}

// IsMigrated returns true if this hash has been migrated to uint64 format.
func (h *ImageHash) IsMigrated() bool {
	return h.DHashInt != nil
}
