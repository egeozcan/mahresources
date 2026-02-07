package fts

import (
	"gorm.io/gorm"
)

// SearchMode defines the type of search
type SearchMode int

const (
	ModeExact  SearchMode = iota // Standard word matching
	ModePrefix                   // Prefix matching (typ* -> type, typing)
	ModeFuzzy                    // Typo-tolerant matching (~test)
)

// ParsedQuery represents a parsed search term
type ParsedQuery struct {
	Term      string
	Mode      SearchMode
	FuzzyDist int // For fuzzy: max edit distance (default 1)
}

// FTSProvider interface for database-specific FTS implementations
type FTSProvider interface {
	// Setup creates FTS indexes/tables for all entities
	Setup(db *gorm.DB) error

	// BuildSearchScope returns a GORM scope that filters results using FTS
	BuildSearchScope(tableName string, columns []string, query ParsedQuery) func(*gorm.DB) *gorm.DB

	// GetRankExpr returns a SQL expression for relevance ranking
	// Returns empty string if ranking is not supported
	GetRankExpr(tableName string, columns []string, query ParsedQuery) string

	// SupportsFeature checks if a feature is supported by this provider
	SupportsFeature(feature string) bool
}

// EntityFTSConfig defines searchable fields per entity
type EntityFTSConfig struct {
	TableName string
	Columns   []string
	// WeightedCols maps column name to weight (A=highest, D=lowest for PostgreSQL)
	WeightedCols map[string]string
}

// EntityConfigs defines FTS configuration for each searchable entity
var EntityConfigs = map[string]EntityFTSConfig{
	"resource": {
		TableName: "resources",
		Columns:   []string{"name", "description", "original_name"},
		WeightedCols: map[string]string{
			"name":          "A",
			"original_name": "B",
			"description":   "C",
		},
	},
	"note": {
		TableName: "notes",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"group": {
		TableName: "groups",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"tag": {
		TableName: "tags",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"category": {
		TableName: "categories",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"query": {
		TableName: "queries",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"relationType": {
		TableName: "group_relation_types",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"noteType": {
		TableName: "note_types",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
	"resourceCategory": {
		TableName: "resource_categories",
		Columns:   []string{"name", "description"},
		WeightedCols: map[string]string{
			"name":        "A",
			"description": "B",
		},
	},
}

// GetEntityConfig returns the FTS config for an entity type, or nil if not found
func GetEntityConfig(entityType string) *EntityFTSConfig {
	if config, ok := EntityConfigs[entityType]; ok {
		return &config
	}
	return nil
}
