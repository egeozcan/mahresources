package application_context

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	_ "github.com/mattn/go-sqlite3"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"log"
	"mahresources/constants"
	"mahresources/lib"
	"mahresources/models"
	"mahresources/storage"
	"os"
	"strconv"
	"strings"
	"time"
)

type MahresourcesConfig struct {
	DbType         string
	AltFileSystems map[string]string
	FfmpegPath     string
	BindAddress    string
}

// MahresourcesInputConfig holds all configuration options that can be passed
// via command-line flags or environment variables
type MahresourcesInputConfig struct {
	FileSavePath   string
	DbType         string
	DbDsn          string
	DbReadOnlyDsn  string
	DbLogFile      string
	BindAddress    string
	FfmpegPath     string
	AltFileSystems map[string]string
	// MemoryDB uses an in-memory SQLite database (ephemeral, no persistence)
	MemoryDB bool
	// MemoryFS uses an in-memory filesystem (ephemeral, no persistence)
	MemoryFS bool
}

type MahresourcesLocks struct {
	ThumbnailGenerationLock      *lib.IDLock[uint]
	VideoThumbnailGenerationLock *lib.IDLock[uint]
}

type MahresourcesContext struct {
	// the main file system
	fs afero.Fs
	// the db connection to the main db with read and write rights
	db *gorm.DB
	// the db readonly connection to the main db
	readOnlyDB *sqlx.DB
	Config     *MahresourcesConfig
	// these are the alternative locations to look at files or import them from
	altFileSystems map[string]afero.Fs
	locks          MahresourcesLocks
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, readOnlyDB *sqlx.DB, config *MahresourcesConfig) *MahresourcesContext {
	altFileSystems := make(map[string]afero.Fs, len(config.AltFileSystems))

	for key, path := range config.AltFileSystems {
		altFileSystems[key] = storage.CreateStorage(path)
	}

	thumbnailGenerationLock := lib.NewIDLock[uint](uint(0), nil)
	videoThumbnailGenerationLock := lib.NewIDLock[uint](uint(1), nil)

	return &MahresourcesContext{
		fs:             filesystem,
		db:             db,
		readOnlyDB:     readOnlyDB,
		Config:         config,
		altFileSystems: altFileSystems,
		locks: MahresourcesLocks{
			ThumbnailGenerationLock:      thumbnailGenerationLock,
			VideoThumbnailGenerationLock: videoThumbnailGenerationLock,
		},
	}
}

// EnsureForeignKeysActive ensures that sqlite connection somehow didn't manage to deactivate foreign keys
// I really don't know why this happens, so @todo please remove this if you can fix the root issue
func (ctx *MahresourcesContext) EnsureForeignKeysActive(db *gorm.DB) {
	if ctx.Config.DbType != constants.DbTypeSqlite {
		return
	}

	query := "PRAGMA foreign_keys = ON;"

	if db == nil {
		ctx.db.Exec(query)
	}

	db.Exec(query)
}

func (ctx *MahresourcesContext) WithTransaction(txFn func(transactionCtx *MahresourcesContext) error) error {
	return ctx.db.Transaction(func(tx *gorm.DB) error {
		altContext := NewMahresourcesContext(ctx.fs, tx, ctx.readOnlyDB, ctx.Config)
		return txFn(altContext)
	})
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

func pageLimitCustom(maxResults int) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Limit(maxResults)
	}
}

type fieldResult struct {
	Key string
}

func metaKeys(ctx *MahresourcesContext, table string) (*[]fieldResult, error) {
	var results []fieldResult

	if ctx.Config.DbType == constants.DbTypePosgres {
		if err := ctx.db.
			Table(table).
			Select("DISTINCT jsonb_object_keys(Meta) as Key").
			Where("Meta IS NOT NULL").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else if ctx.Config.DbType == constants.DbTypeSqlite {
		if err := ctx.db.
			Table(fmt.Sprintf("%v, json_each(%v.meta)", table, table)).
			Select("DISTINCT json_each.key as Key").
			Where("Meta IS NOT NULL").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else {
		results = make([]fieldResult, 0)
	}

	return &results, nil
}

// CreateContextWithConfig creates a context using the provided configuration.
// This is the preferred way to create a context when using command-line flags.
func CreateContextWithConfig(cfg *MahresourcesInputConfig) (*MahresourcesContext, *gorm.DB, afero.Fs) {
	var db *gorm.DB
	var mainFs afero.Fs

	// Determine effective database settings
	dbType := cfg.DbType
	dbDsn := cfg.DbDsn
	readOnlyDsn := cfg.DbReadOnlyDsn

	if cfg.MemoryDB {
		dbType = "SQLITE"
		// Use shared cache so main and read-only connections share the same in-memory database
		dbDsn = "file::memory:?cache=shared"
		readOnlyDsn = "file::memory:?cache=shared&mode=ro"
		log.Println("Using in-memory SQLite database (ephemeral mode)")
	}

	// Determine effective filesystem
	if cfg.MemoryFS {
		mainFs = storage.CreateMemoryStorage()
		log.Println("Using in-memory filesystem (ephemeral mode)")
	} else {
		if cfg.FileSavePath == "" {
			log.Fatal("File save path is empty (use -memory-fs for ephemeral mode)")
		}
		mainFs = storage.CreateStorage(cfg.FileSavePath)
	}

	fmt.Printf("DB_TYPE %v DB_DSN %v FILE_SAVE_PATH %v\n", dbType, dbDsn, cfg.FileSavePath)

	if connectedDB, err := models.CreateDatabaseConnection(dbType, dbDsn, cfg.DbLogFile); err != nil {
		log.Fatal(err)
	} else {
		db = connectedDB
	}

	readOnlyDb, err := models.CreateReadOnlyDatabaseConnection(strings.ToLower(dbType), readOnlyDsn)

	if err != nil {
		log.Fatal(err.Error())
	}

	return NewMahresourcesContext(mainFs, db, readOnlyDb, &MahresourcesConfig{
		DbType:         dbType,
		AltFileSystems: cfg.AltFileSystems,
		FfmpegPath:     cfg.FfmpegPath,
		BindAddress:    cfg.BindAddress,
	}), db, mainFs
}

// CreateContext creates a context using environment variables.
// Deprecated: Use CreateContextWithConfig for new code.
func CreateContext() (*MahresourcesContext, *gorm.DB, afero.Fs) {
	var numAlt int64 = 0

	if fileAltCount, err := strconv.ParseInt(os.Getenv("FILE_ALT_COUNT"), 10, 8); err == nil {
		numAlt = fileAltCount
	}

	altFSystems := make(map[string]string, numAlt)
	for i := int64(0); i < numAlt; i++ {
		altFSystems[os.Getenv(fmt.Sprintf("FILE_ALT_NAME_%v", i+1))] = os.Getenv(fmt.Sprintf("FILE_ALT_PATH_%v", i+1))
	}

	return CreateContextWithConfig(&MahresourcesInputConfig{
		FileSavePath:   os.Getenv("FILE_SAVE_PATH"),
		DbType:         os.Getenv("DB_TYPE"),
		DbDsn:          os.Getenv("DB_DSN"),
		DbReadOnlyDsn:  os.Getenv("DB_READONLY_DSN"),
		DbLogFile:      os.Getenv("DB_LOG_FILE"),
		BindAddress:    os.Getenv("BIND_ADDRESS"),
		FfmpegPath:     os.Getenv("FFMPEG_PATH"),
		AltFileSystems: altFSystems,
	})
}
