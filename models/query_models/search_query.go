package query_models

// GlobalSearchQuery represents the search request parameters
type GlobalSearchQuery struct {
	Query string   `json:"q"`
	Limit int      `json:"limit"`
	Types []string `json:"types"`
}

// SearchResultItem represents a single search result
type SearchResultItem struct {
	ID          uint              `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Score       int               `json:"score"`
	URL         string            `json:"url"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// GlobalSearchResponse represents the search response
type GlobalSearchResponse struct {
	Query   string             `json:"query"`
	Total   int                `json:"total"`
	Results []SearchResultItem `json:"results"`
}
