package main

import (
	"fmt"
	"github.com/joho/godotenv"
	"gorm.io/gorm"
	"log"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
	"mahresources/storage"
	"os"
	"strconv"
)

func main() {
	// you may have no .env, it's okay
	_ = godotenv.Load(".env")

	var numAlt int64 = 0
	var db *gorm.DB

	ffMpegPath := os.Getenv("FFMPEG_PATH")
	dbType := os.Getenv("DB_TYPE")
	dsn := os.Getenv("DB_DSN")
	logType := os.Getenv("DB_LOG_FILE")
	fileSavePath := os.Getenv("FILE_SAVE_PATH")
	if fileAltCount, err := strconv.ParseInt(os.Getenv("FILE_ALT_COUNT"), 10, 8); err == nil {
		numAlt = fileAltCount
	}

	fmt.Printf("DB_TYPE %v DB_DSN %v FILE_SAVE_PATH %v", dbType, dsn, fileSavePath)

	if fileSavePath == "" {
		log.Fatal("File save path is empty")
	}

	if connectedDB, err := models.CreateDatabaseConnection(dbType, dsn, logType); err != nil {
		log.Fatal(err)
	} else {
		db = connectedDB
	}

	mainFs := storage.CreateStorage(fileSavePath)
	altFSystems := make(map[string]string, numAlt)

	for i := int64(0); i < numAlt; i++ {
		altFSystems[os.Getenv(fmt.Sprintf("FILE_ALT_NAME_%v", i+1))] = os.Getenv(fmt.Sprintf("FILE_ALT_PATH_%v", i+1))
	}

	appContext := application_context.NewMahresourcesContext(mainFs, db, &application_context.MahresourcesConfig{
		DbType:         dbType,
		AltFileSystems: altFSystems,
		FfmpegPath:     ffMpegPath,
	})
	util.AddInitialData(db)

	log.Fatal(server.CreateServer(appContext, mainFs, altFSystems).ListenAndServe())
}
