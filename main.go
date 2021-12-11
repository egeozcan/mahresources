package main

import (
	"errors"
	"fmt"
	"github.com/joho/godotenv"
	"github.com/spf13/afero"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"io"
	"io/fs"
	"log"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/util"
	"mahresources/server"
	"os"
	"strconv"
	"time"
)

func main() {
	// you may have no .env, it's okay
	_ = godotenv.Load(".env")

	var db *gorm.DB
	var err error
	var numAlt int64

	dbType := os.Getenv("DB_TYPE")
	dsn := os.Getenv("DB_DSN")
	logType := os.Getenv("DB_LOG_FILE")
	fileSavePath := os.Getenv("FILE_SAVE_PATH")
	if numAlt, err = strconv.ParseInt(os.Getenv("FILE_ALT_COUNT"), 10, 8); err != nil {
		numAlt = 0
		err = nil
	}

	var dbLogger logger.Interface

	config := logger.Config{
		SlowThreshold: 0,
		LogLevel:      logger.Info,
		Colorful:      true,
	}

	switch logType {
	case "STDOUT":
		logWriter := log.New(os.Stdout, "\r\n", log.LstdFlags)
		logWriter.Println("Logging to STDOUT")
		dbLogger = logger.New(
			logWriter,
			config,
		)
	case "":
		dbLogger = logger.New(
			log.New(io.Discard, "", 0),
			logger.Config{},
		)
	default:
		open, err := os.OpenFile(logType, os.O_WRONLY, fs.ModeAppend)

		if err != nil {
			log.Fatalf("Error when opening the log file")
		}

		dbLogger = logger.New(
			log.New(open, "\r\n", log.LstdFlags), // io writer
			config,
		)
	}

	fmt.Printf("DB_TYPE %v DB_DSN %v FILE_SAVE_PATH %v", dbType, dsn, fileSavePath)

	switch dbType {
	case "POSTGRES":
		db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: dbLogger,
		})
	case "SQLITE":
		db, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
			Logger: dbLogger,
		})
		db.Exec("PRAGMA foreign_keys = ON;")
	default:
		err = errors.New("please set the DB_TYPE env var to SQLITE or POSTGRES")
	}

	if err != nil {
		log.Fatalf("failed to connect to the database: %v", err)
	}

	err = db.AutoMigrate(
		&models.Resource{},
		&models.Note{},
		&models.Tag{},
		&models.Group{},
		&models.Category{},
		&models.NoteType{},
		&models.Preview{},
		&models.GroupRelation{},
		&models.GroupRelationType{},
	)

	if err != nil {
		log.Fatalf("failed to migrate: %v", err)
	}

	if fileSavePath == "" {
		log.Fatal("File save path is empty")
	}

	mainFs := createStorage(fileSavePath)
	altFSystems := make(map[string]afero.Fs, numAlt)

	for i := int64(0); i < numAlt; i++ {
		altFSystems[os.Getenv(fmt.Sprintf("FILE_ALT_NAME_%v", i+1))] = createStorage(os.Getenv(fmt.Sprintf("FILE_ALT_PATH_%v", i+1)))
	}

	appContext := application_context.NewMahresourcesContext(mainFs, db, dbType, altFSystems)
	util.AddInitialData(db)

	log.Fatal(server.CreateServer(appContext, mainFs, altFSystems).ListenAndServe())
}

func createStorage(path string) afero.Fs {
	base := afero.NewBasePathFs(afero.NewOsFs(), path)
	layer := afero.NewMemMapFs()
	return afero.NewCacheOnReadFs(base, layer, 10*time.Minute)
}
