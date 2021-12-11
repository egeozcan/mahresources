package application_context

import (
	"fmt"
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"io"
	"mahresources/constants"
	"time"
)

type File interface {
	io.Reader
	io.Seeker
	io.Closer
}

type MahresourcesContext struct {
	fs             afero.Fs
	db             *gorm.DB
	dbType         string
	altFileSystems map[string]afero.Fs
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB, dbType string, altFileSystems map[string]afero.Fs) *MahresourcesContext {
	return &MahresourcesContext{fs: filesystem, db: db, dbType: dbType, altFileSystems: altFileSystems}
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

	if ctx.dbType == "POSTGRES" {
		if err := ctx.db.
			Table(table).
			Select("DISTINCT jsonb_object_keys(Meta) as Key").
			Scan(&results).Error; err != nil {
			return nil, err
		}
	} else if ctx.dbType == "SQLITE" {
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
