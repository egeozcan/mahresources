package util

import (
	"gorm.io/gorm"
	"mahresources/models"
)

func AddInitialData(db *gorm.DB) {
	var count int64
	db.Model(&models.Category{}).Count(&count)

	if count > 0 {
		return
	}

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
