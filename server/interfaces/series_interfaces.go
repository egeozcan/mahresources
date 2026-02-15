package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type SeriesReader interface {
	GetSeries(id uint) (*models.Series, error)
	GetSeriesBySlug(slug string) (*models.Series, error)
}

type SeriesWriter interface {
	UpdateSeries(editor *query_models.SeriesEditor) (*models.Series, error)
}

type SeriesDeleter interface {
	DeleteSeries(id uint) error
}

type ResourceSeriesRemover interface {
	RemoveResourceFromSeries(resourceID uint) error
}

type SeriesSiblingReader interface {
	GetSeriesSiblings(resourceID uint, seriesID uint) ([]*models.Resource, error)
}
