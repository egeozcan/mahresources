package application_context

import (
	"mahresources/models"
	"mahresources/models/types"
)

// legacyReduction is a Resource Reduction row as the previous release wrote it: the
// ordinary row plus the `plan` and `extent` JSON documents that release kept in it.
// models.ResourceReduction no longer names those two columns, so no ordinary load
// can read them. This model exists for the migration and its tests, which create
// legacy rows with it; nothing else may select the columns (reductionColumns is the
// projection every other load uses).
type legacyReduction struct {
	models.ResourceReduction
	Plan   types.JSON `gorm:"column:plan;type:json"`
	Extent types.JSON `gorm:"column:extent;type:json"`
}

func (legacyReduction) TableName() string { return "resource_reductions" }
