package context

import (
	"github.com/spf13/afero"
	"gorm.io/gorm"
	"io"
)

type File interface {
	io.Reader
	io.Seeker
	io.Closer
}

type MahresourcesContext struct {
	fs afero.Fs
	db *gorm.DB
}

func NewMahresourcesContext(filesystem afero.Fs, db *gorm.DB) *MahresourcesContext {
	return &MahresourcesContext{fs: filesystem, db: db}
}

func standardSort(sortField string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Order(sortField)
	}
}

func standardLimit(db *gorm.DB) *gorm.DB {
	return db.Limit(10)
}

func standardSortAndLimit(sortField string) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return standardLimit(standardSort(sortField)(db))
	}
}
