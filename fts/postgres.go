package fts

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// PostgresFTS implements FTSProvider for PostgreSQL using tsvector and pg_trgm
type PostgresFTS struct{}

// NewPostgresFTS creates a new PostgreSQL FTS provider
func NewPostgresFTS() *PostgresFTS {
	return &PostgresFTS{}
}

// Setup creates FTS indexes for all configured entities
func (p *PostgresFTS) Setup(db *gorm.DB) error {
	// Enable pg_trgm extension for fuzzy search
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pg_trgm").Error; err != nil {
		return fmt.Errorf("failed to create pg_trgm extension: %w", err)
	}

	for entityType, config := range EntityConfigs {
		if err := p.setupTable(db, config); err != nil {
			return fmt.Errorf("failed to setup FTS for %s: %w", entityType, err)
		}
	}
	return nil
}

func (p *PostgresFTS) setupTable(db *gorm.DB, config EntityFTSConfig) error {
	// Build tsvector column expression with weights
	// setweight(to_tsvector('english', coalesce(name,'')), 'A') ||
	// setweight(to_tsvector('english', coalesce(description,'')), 'B')
	var parts []string
	for _, col := range config.Columns {
		weight := config.WeightedCols[col]
		if weight == "" {
			weight = "D" // default lowest weight
		}
		parts = append(parts, fmt.Sprintf(
			"setweight(to_tsvector('english', coalesce(%s,'')), '%s')",
			col, weight,
		))
	}
	tsvectorExpr := strings.Join(parts, " || ")

	// Check if search_vector column already exists
	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = ? AND column_name = 'search_vector'
	`, config.TableName).Scan(&count)

	if count == 0 {
		// Add generated tsvector column
		alterSQL := fmt.Sprintf(`
			ALTER TABLE %s
			ADD COLUMN search_vector tsvector
			GENERATED ALWAYS AS (%s) STORED`,
			config.TableName, tsvectorExpr,
		)
		if err := db.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to add search_vector column to %s: %w", config.TableName, err)
		}
	}

	// Create GIN index on search_vector (idempotent with IF NOT EXISTS)
	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_fts
		ON %s USING GIN(search_vector)`,
		config.TableName, config.TableName,
	)
	if err := db.Exec(indexSQL).Error; err != nil {
		return fmt.Errorf("failed to create FTS index on %s: %w", config.TableName, err)
	}

	// Create trigram index for fuzzy search on name column
	trigramSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_trgm
		ON %s USING GIN(name gin_trgm_ops)`,
		config.TableName, config.TableName,
	)
	if err := db.Exec(trigramSQL).Error; err != nil {
		return fmt.Errorf("failed to create trigram index on %s: %w", config.TableName, err)
	}

	return nil
}

// BuildSearchScope returns a GORM scope for FTS search
func (p *PostgresFTS) BuildSearchScope(tableName string, columns []string, query ParsedQuery) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if query.Term == "" {
			return db
		}

		escapedTerm := EscapeForFTS(query.Term)

		switch query.Mode {
		case ModePrefix:
			// Use to_tsquery with :* for prefix matching
			// Split terms and add :* to each
			terms := strings.Fields(escapedTerm)
			var tsqueryParts []string
			for _, term := range terms {
				tsqueryParts = append(tsqueryParts, term+":*")
			}
			tsquery := strings.Join(tsqueryParts, " & ")

			return db.Where(
				fmt.Sprintf("%s.search_vector @@ to_tsquery('english', ?)", tableName),
				tsquery,
			)

		case ModeFuzzy:
			// Use trigram % operator which can use the GIN index
			// The % operator uses pg_trgm.similarity_threshold (default 0.3)
			return db.Where(
				fmt.Sprintf("%s.name %% ?", tableName),
				escapedTerm,
			)

		default: // ModeExact
			// Use plainto_tsquery for natural language search
			return db.Where(
				fmt.Sprintf("%s.search_vector @@ plainto_tsquery('english', ?)", tableName),
				escapedTerm,
			)
		}
	}
}

// GetRankExpr returns a SQL expression for relevance ranking
func (p *PostgresFTS) GetRankExpr(tableName string, columns []string, query ParsedQuery) (string, []interface{}) {
	if query.Term == "" {
		return "0", nil
	}

	escapedTerm := EscapeForFTS(query.Term)

	switch query.Mode {
	case ModeFuzzy:
		// Use similarity score for fuzzy search
		return fmt.Sprintf("similarity(%s.name, ?)", tableName), []interface{}{escapedTerm}

	case ModePrefix:
		// Use ts_rank with prefix query
		terms := strings.Fields(escapedTerm)
		var tsqueryParts []string
		for _, term := range terms {
			tsqueryParts = append(tsqueryParts, term+":*")
		}
		tsquery := strings.Join(tsqueryParts, " & ")
		return fmt.Sprintf(
			"ts_rank(%s.search_vector, to_tsquery('english', ?))",
			tableName,
		), []interface{}{tsquery}

	default: // ModeExact
		return fmt.Sprintf(
			"ts_rank(%s.search_vector, plainto_tsquery('english', ?))",
			tableName,
		), []interface{}{escapedTerm}
	}
}

// SupportsFeature checks if a feature is supported
func (p *PostgresFTS) SupportsFeature(feature string) bool {
	switch feature {
	case "prefix", "fuzzy", "ranking":
		return true
	default:
		return false
	}
}
