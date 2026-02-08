package util

import (
	"gorm.io/gorm"
	"mahresources/models"
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
			// Batch update to avoid a single massive transaction on large databases
			for {
				result := db.Exec(
					"UPDATE resources SET resource_category_id = ? WHERE id IN (SELECT id FROM resources WHERE resource_category_id IS NULL LIMIT 10000)",
					defaultResourceCategory.ID,
				)
				if result.Error != nil || result.RowsAffected == 0 {
					break
				}
			}
		}
	}
}
