package application_context

import (
	"fmt"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"io"
	"mahresources/constants"
	"mahresources/storage"
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
	config         *MahresourcesConfig
	altFileSystems map[string]afero.Fs
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, config *MahresourcesConfig) *MahresourcesContext {
	altFileSystems := make(map[string]afero.Fs, len(config.AltFileSystems))

	for key, path := range config.AltFileSystems {
		altFileSystems[key] = storage.CreateStorage(path)
	}

	return &MahresourcesContext{fs: filesystem, db: db, config: config, altFileSystems: altFileSystems}
}

// EnsureForeignKeysActive ensures that sqlite connection somehow didn't manage to deactivate foreign keys
// I really don't know why this happens, so @todo please remove this if you can fix the root issue
func (ctx *MahresourcesContext) EnsureForeignKeysActive(db *gorm.DB) {
	if ctx.config.DbType != "SQLITE" {
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

	if ctx.config.DbType == "POSTGRES" {
		if err := ctx.db.
			Table(table).
			Select("DISTINCT jsonb_object_keys(Meta) as Key").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else if ctx.config.DbType == "SQLITE" {
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
