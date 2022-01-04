package application_context

import (
	"fmt"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"io"
	"log"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/storage"
	"os"
	"strconv"
	"time"
)

type File interface {
	io.Reader
	io.Closer
}

type MahresourcesConfig struct {
	DbType         string
	AltFileSystems map[string]string
	FfmpegPath     string
}

type MahresourcesContext struct {
	fs             afero.Fs
	db             *gorm.DB
	Config         *MahresourcesConfig
	altFileSystems map[string]afero.Fs
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, config *MahresourcesConfig) *MahresourcesContext {
	altFileSystems := make(map[string]afero.Fs, len(config.AltFileSystems))

	for key, path := range config.AltFileSystems {
		altFileSystems[key] = storage.CreateStorage(path)
	}

	return &MahresourcesContext{fs: filesystem, db: db, Config: config, altFileSystems: altFileSystems}
}

// EnsureForeignKeysActive ensures that sqlite connection somehow didn't manage to deactivate foreign keys
// I really don't know why this happens, so @todo please remove this if you can fix the root issue
func (ctx *MahresourcesContext) EnsureForeignKeysActive(db *gorm.DB) {
	if ctx.Config.DbType != "SQLITE" {
		return
	}

	query := "PRAGMA foreign_keys = ON;"

	if db == nil {
		ctx.db.Exec(query)
	}

	db.Exec(query)
}

func parseHTMLTime(timeStr string) *time.Time {
	return timeOrNil(time.Parse(constants.TimeFormat, timeStr))
}

func timeOrNil(time time.Time, err error) *time.Time {
	if err != nil {
		fmt.Println("couldn't parse date", err.Error())

		return nil
	}

	return &time
}

func pageLimit(db *gorm.DB) *gorm.DB {
	return db.Limit(constants.MaxResultsPerPage)
}

type fieldResult struct {
	Key string
}

func metaKeys(ctx *MahresourcesContext, table string) (*[]fieldResult, error) {
	var results []fieldResult

	if ctx.Config.DbType == "POSTGRES" {
		if err := ctx.db.
			Table(table).
			Select("DISTINCT jsonb_object_keys(Meta) as Key").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else if ctx.Config.DbType == "SQLITE" {
		if err := ctx.db.
			Table(fmt.Sprintf("%v, json_each(%v.meta)", table, table)).
			Select("DISTINCT json_each.key as Key").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else {
		results = make([]fieldResult, 0)
	}

	return &results, nil
}

func CreateContext() (*MahresourcesContext, *gorm.DB, afero.Fs) {
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

	return NewMahresourcesContext(mainFs, db, &MahresourcesConfig{
		DbType:         dbType,
		AltFileSystems: altFSystems,
		FfmpegPath:     ffMpegPath,
	}), db, mainFs
}
