package main

import (
	"encoding/json"
	"fmt"
	"github.com/Nr90/imgsim"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"image/jpeg"
	"log"
	"mahresources/application_context"
	"mahresources/models"
	"sync"
	"time"
)

type DuplicateResult struct {
	AHash     string
	Resources json.RawMessage
	Owners    json.RawMessage
	Hashes    json.RawMessage
	Sizes     json.RawMessage
	Count     int
}

func main() {
	_ = godotenv.Load(".env")

	context, db, _ := application_context.CreateContext()

	println(context, db)

	var count int64
	var processed int64 = 0
	var resources []models.Resource

	query := db.
		Table("resources").
		Joins("left join image_hashes ih ON ih.resource_id = resources.id").
		Where("resources.content_type = 'image/jpeg'").
		Where("ih.ID IS NULL")

	if err := query.Count(&count).Error; err != nil {
		log.Fatalln(err)
	}

	batchErr := query.FindInBatches(&resources, 512, func(tx *gorm.DB, batch int) error {
		sem := make(chan struct{}, 4)
		var wg sync.WaitGroup
		startTime := time.Now()

		for _, resource := range resources {
			storage, err := context.GetFsForStorageLocation(resource.StorageLocation)
			println(resource.ID)

			if err != nil {
				return err
			}

			wg.Add(1)

			go func(resource models.Resource, storage afero.Fs) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				imgFile, err := storage.Open(resource.GetCleanLocation())

				if err != nil {
					println(err.Error())
					return
				}

				defer imgFile.Close()

				img, err := jpeg.Decode(imgFile)

				if err != nil {
					println(err.Error())
					return
				}

				ahash := imgsim.AverageHash(img)
				dhash := imgsim.DifferenceHash(img)

				imgHash := models.ImageHash{
					AHash:      ahash.String(),
					DHash:      dhash.String(),
					ResourceId: &resource.ID,
				}

				db.Save(&imgHash)

				processed++
			}(resource, storage)
		}

		fmt.Printf("%v \\ %v \n\n", count, processed)

		wg.Wait()

		currentTime := time.Now()

		diff := currentTime.Sub(startTime)

		fmt.Printf("finished batch in %v seconds\n\n", diff.Seconds())

		return nil
	}).Error

	if batchErr != nil {
		log.Fatalln("batch error", batchErr)
	}
}
