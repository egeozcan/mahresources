package interfaces

import "mahresources/models/query_models"

type GlobalSearcher interface {
	GlobalSearch(query *query_models.GlobalSearchQuery) (*query_models.GlobalSearchResponse, error)
}
