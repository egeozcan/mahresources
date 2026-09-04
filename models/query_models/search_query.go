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
	DisplayType string            `json:"displayType,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Score       int               `json:"score"`
	URL         string            `json:"url"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// GlobalSearchResponse represents the search response
type GlobalSearchResponse struct {
	Query string `json:"query"`
	Total int    `json:"total"`
	// TotalCapped reports that Total is a floor rather than an exact count:
	// the search service trims to its own 50-row ceiling before counting, so a
	// corpus with thousands of matches also reports 50. Callers that display
	// the number must render it as "50+" when this is set (WS6, finding 32).
	TotalCapped bool               `json:"totalCapped,omitempty"`
	Results     []SearchResultItem `json:"results"`
}
