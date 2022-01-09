package main

import (
	"encoding/json"
	"fmt"
	"github.com/Nr90/imgsim"
	"github.com/jmoiron/sqlx"
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
		sem := make(chan struct{}, 32)
		var wg sync.WaitGroup
		startTime := time.Now()

		for _, resource := range resources {
			storage, err := context.GetFsForStorageLocation(resource.StorageLocation)
			fmt.Println(resource.ID)

			if err != nil {
				return err
			}

			wg.Add(1)

			go func(resource models.Resource, storage afero.Fs) {
				defer wg.Done()
				sem <- struct{}{}

				imgFile, err := storage.Open(resource.GetCleanLocation())

				if err != nil {
					<-sem
					return
				}

				defer imgFile.Close()

				img, err := jpeg.Decode(imgFile)

				if err != nil {
					<-sem
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

				<-sem

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

	var results []DuplicateResult

	sqlDb, _ := db.DB()

	sqlXDb := sqlx.NewDb(sqlDb, "postgres")

	err := sqlXDb.Select(&results, `
		select
			   a_hash AHash
			 , json_agg(r.id)::jsonb Resources
			 , jsonb_agg(r.owner_id)::jsonb Owners
			 , jsonb_agg(r.hash)::jsonb Hashes
			 , jsonb_agg(r.file_size) Sizes
			 , count(*)
		from
			 image_hashes
		join
				 resources r on r.id = image_hashes.resource_id
		join groups g on g.id = r.owner_id
		group by a_hash
		having count(*) > 1
	`)

	fmt.Println(len(results), "results found")

	if err != nil {
		log.Fatalln(err)
	}

	deleteOpCount := 0

out:
	for _, result := range results {
		var ownerIds []uint
		var resIds []uint
		var sizes []uint
		var hashes []string

		ownersDifferent := false

		err := json.Unmarshal(result.Owners, &ownerIds)
		if err != nil {
			log.Fatalln(err)
		}

		err = json.Unmarshal(result.Resources, &resIds)
		if err != nil {
			log.Fatalln(err)
		}

		err = json.Unmarshal(result.Hashes, &hashes)
		if err != nil {
			log.Fatalln(err)
		}

		err = json.Unmarshal(result.Sizes, &sizes)
		if err != nil {
			log.Fatalln(err)
		}

		for i := 0; i < result.Count; i++ {
			println(ownerIds[i], resIds[i], hashes[i])

			if i == 0 {
				continue
			}

			if hashes[i] != hashes[i-1] {
				println("Skipping because hashes are different")
				continue out
			}

			if sizes[i] != sizes[i-1] {
				println("Different sizes?")
				break out
			}

			if ownerIds[i] != ownerIds[i-1] {
				ownersDifferent = true
			}

			if i == result.Count-1 {
				fmt.Printf("will delete %v %v %v \n\n", ownerIds, resIds, hashes)

				if !ownersDifferent {
					err := context.MergeResources(resIds[i], resIds[:i])
					if err != nil {
						log.Fatalln(err)
					}
				} else {
					err := context.MergeResources(resIds[0], resIds[1:])
					if err != nil {
						log.Fatalln(err)
					}
				}
			}
		}

	}

	fmt.Printf("delete count %v \n", deleteOpCount)

}
