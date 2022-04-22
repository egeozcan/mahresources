package main

import (
	"flag"
	"fmt"
	"github.com/joho/godotenv"
	"io/fs"
	"log"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models/query_models"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	_ = godotenv.Load(".env")

	target := flag.String("target", "", "Target directory, must be a sub-folder of an alternative attach point")
	ownerId := flag.Uint("ownerId", 0, "Id of the owner group")

	var fileSystemKey string
	var fileSystemPath string

	flag.Parse()

	if *target == "" || *ownerId == 0 {
		flag.PrintDefaults()
		os.Exit(1)
	}

	context, _, _ := application_context.CreateContext()

	fmt.Println("target", *target)

	for key, val := range context.Config.AltFileSystems {
		fmt.Println(key, val, *target)

		if strings.HasPrefix(*target, val) {
			fileSystemKey = key
			fileSystemPath = val
			fmt.Println("file system key is " + fileSystemKey)
			break
		}
	}

	if fileSystemKey == "" {
		log.Fatalln("could not find an attach point that contains target folder")
	}

	stat, err := os.Stat(*target)

	if err != nil {
		log.Fatalf("error when opening the target: %v", err)
	}

	if !stat.IsDir() {
		log.Fatalf("target is not a dir")
	}

	walkErr := filepath.Walk(*target, func(path string, info fs.FileInfo, err error) error {
		if strings.HasSuffix(path, constants.ThumbFileSuffix) {
			return nil
		}

		relPath := strings.TrimPrefix(path, fileSystemPath)

		fmt.Println(relPath, info, err)

		if info.IsDir() {
			return err
		}

		_, createErr := context.AddLocalResource(path, &query_models.ResourceFromLocalCreator{
			ResourceQueryBase: query_models.ResourceQueryBase{
				Name:             relPath,
				Description:      "",
				OwnerId:          *ownerId,
				Groups:           []uint{},
				Tags:             []uint{},
				Notes:            []uint{},
				Meta:             `{ "imported": true }`,
				ContentCategory:  "",
				Category:         fmt.Sprintf("%v import", *target),
				OriginalName:     "",
				OriginalLocation: relPath,
			},
			LocalPath: relPath,
			PathName:  fileSystemKey,
		})

		return createErr
	})

	if walkErr != nil {
		log.Fatalf("error when scanning: %v", walkErr)
	}
}
