package models

import "strconv"

type ImageHash struct {
	ID         uint      `gorm:"primarykey"`
	AHash      string    `gorm:"index"`  // old, kept during migration
	DHash      string    `gorm:"index"`  // old, kept during migration
	AHashInt   *uint64   `gorm:"index"`  // new uint64 column
	DHashInt   *uint64   `gorm:"index"`  // new uint64 column
	Resource   *Resource `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ResourceId *uint     `gorm:"uniqueIndex"`
}

// GetDHash returns the DHash as uint64, preferring the new column
// and falling back to parsing the old string column.
func (h *ImageHash) GetDHash() uint64 {
	if h.DHashInt != nil {
		return *h.DHashInt
	}
	if h.DHash == "" {
		return 0
	}
	val, _ := strconv.ParseUint(h.DHash, 16, 64)
	return val
}

// GetAHash returns the AHash as uint64, preferring the new column
// and falling back to parsing the old string column.
func (h *ImageHash) GetAHash() uint64 {
	if h.AHashInt != nil {
		return *h.AHashInt
	}
	if h.AHash == "" {
		return 0
	}
	val, _ := strconv.ParseUint(h.AHash, 16, 64)
	return val
}

// IsMigrated returns true if this hash has been migrated to uint64 format.
func (h *ImageHash) IsMigrated() bool {
	return h.DHashInt != nil
}
