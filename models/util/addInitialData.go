package util

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/types"
)

func AddInitialData(db *gorm.DB) {
	var categoryCount int64
	db.Model(&models.Category{}).Count(&categoryCount)

	if categoryCount == 0 {
		personCategory := &models.Category{Name: "Person", Description: "A person. Likely a human."}
		db.Create(personCategory)
		locationCategory := &models.Category{Name: "Location", Description: "Some place you know about."}
		db.Create(locationCategory)
		businessCategory := &models.Category{Name: "Business", Description: "Some business you know about."}
		db.Create(businessCategory)

		db.Create(&models.GroupRelationType{
			Name:           "Address",
			FromCategoryId: &personCategory.ID,
			ToCategoryId:   &locationCategory.ID,
		})

		db.Create(&models.GroupRelationType{
			Name:           "Employer",
			FromCategoryId: &personCategory.ID,
			ToCategoryId:   &businessCategory.ID,
		})
	}

	var noteTypeCount int64
	db.Model(&models.NoteType{}).Count(&noteTypeCount)

	if noteTypeCount == 0 {
		var noteCount int64
		db.Model(&models.Note{}).Count(&noteCount)

		if noteCount > 0 {
			defaultNoteType := &models.NoteType{Name: "Default", Description: "Default note type for existing notes."}
			db.Create(defaultNoteType)
			db.Model(&models.Note{}).Where("note_type_id IS NULL").Update("note_type_id", defaultNoteType.ID)
		}
	}

	var resourceCategoryCount int64
	db.Model(&models.ResourceCategory{}).Count(&resourceCategoryCount)

	if resourceCategoryCount == 0 {
		var resourceCount int64
		db.Model(&models.Resource{}).Count(&resourceCount)

		if resourceCount > 0 {
			defaultResourceCategory := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
			db.Create(defaultResourceCategory)

			var totalRemaining int64
			db.Model(&models.Resource{}).Where("resource_category_id IS NULL").Count(&totalRemaining)

			logMigrationProgress(db, fmt.Sprintf("Resource category migration: starting (%d resources to update)", totalRemaining),
				map[string]interface{}{"total": totalRemaining})

			// Batch update to avoid a single massive transaction on large databases
			var totalUpdated int64
			for {
				result := db.Exec(
					"UPDATE resources SET resource_category_id = ? WHERE id IN (SELECT id FROM resources WHERE resource_category_id IS NULL LIMIT 10000)",
					defaultResourceCategory.ID,
				)
				if result.Error != nil {
					logMigrationProgress(db, fmt.Sprintf("Resource category migration: error: %v", result.Error), nil)
					break
				}
				if result.RowsAffected == 0 {
					break
				}
				totalUpdated += result.RowsAffected
				logMigrationProgress(db,
					fmt.Sprintf("Resource category migration: updated %d/%d resources", totalUpdated, totalRemaining),
					map[string]interface{}{"updated": totalUpdated, "remaining": totalRemaining - totalUpdated})
			}

			logMigrationProgress(db, fmt.Sprintf("Resource category migration: complete (%d resources updated)", totalUpdated),
				map[string]interface{}{"total_updated": totalUpdated})
		}
	}
}

// logMigrationProgress logs to both stdout and the log_entries table,
// matching the hash worker's progress reporting pattern.
func logMigrationProgress(db *gorm.DB, message string, details map[string]interface{}) {
	log.Print(message)

	entry := models.LogEntry{
		CreatedAt:  time.Now(),
		Level:      models.LogLevelInfo,
		Action:     models.LogActionProgress,
		EntityType: "migration",
		Message:    message,
	}

	if details != nil {
		if jsonBytes, err := json.Marshal(details); err == nil {
			entry.Details = types.JSON(jsonBytes)
		}
	}

	db.Create(&entry)
}
