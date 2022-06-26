package interfaces

import (
	"github.com/jmoiron/sqlx"
	"mahresources/models"
	"mahresources/models/query_models"
)

type QueryReader interface {
	GetQueries(offset, maxResults int, searchQuery *query_models.QueryQuery) ([]models.Query, error)
	GetQuery(id uint) (*models.Query, error)
}

type QueryWriter interface {
	UpdateQuery(categoryEditor *query_models.QueryEditor) (*models.Query, error)
	CreateQuery(categoryCreator *query_models.QueryCreator) (*models.Query, error)
}

type QueryDeleter interface {
	DeleteQuery(categoryId uint) error
}

type QueryRunner interface {
	RunReadOnlyQuery(queryId uint, params map[string]any) (*sqlx.Rows, error)
	RunReadOnlyQueryByName(queryName string, params map[string]any) (*sqlx.Rows, error)
}
