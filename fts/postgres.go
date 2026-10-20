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
	// search_vector holds English stems for exact and stemmed matching.
	// search_vector_simple holds the same words unstemmed: prefix search needs
	// it, because a typed prefix longer than a word's stem ("Mentionab" for
	// 'mention', "runni" for 'run') matches no stem.
	for _, v := range []struct{ column, config, index string }{
		{"search_vector", "english", "fts"},
		{"search_vector_simple", "simple", "fts_simple"},
	} {
		if err := p.setupVector(db, config, v.column, v.config, v.index); err != nil {
			return err
		}
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

// setupVector adds a generated, weighted tsvector column built with the given
// text search configuration, and a GIN index on it. Adding the column
// computes it for every existing row.
func (p *PostgresFTS) setupVector(db *gorm.DB, config EntityFTSConfig, column, tsConfig, indexSuffix string) error {
	// setweight(to_tsvector('<config>', coalesce(name,'')), 'A') ||
	// setweight(to_tsvector('<config>', coalesce(description,'')), 'B')
	var parts []string
	for _, col := range config.Columns {
		weight := config.WeightedCols[col]
		if weight == "" {
			weight = "D" // default lowest weight
		}
		parts = append(parts, fmt.Sprintf(
			"setweight(to_tsvector('%s', coalesce(%s,'')), '%s')",
			tsConfig, col, weight,
		))
	}
	tsvectorExpr := strings.Join(parts, " || ")

	var count int64
	db.Raw(`
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = ? AND column_name = ?
	`, config.TableName, column).Scan(&count)

	if count == 0 {
		alterSQL := fmt.Sprintf(`
			ALTER TABLE %s
			ADD COLUMN %s tsvector
			GENERATED ALWAYS AS (%s) STORED`,
			config.TableName, column, tsvectorExpr,
		)
		if err := db.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("failed to add %s column to %s: %w", column, config.TableName, err)
		}
		// Fresh statistics for the new column, so the planner uses its index
		// rather than estimating from none.
		if err := db.Exec(fmt.Sprintf("ANALYZE %s", config.TableName)).Error; err != nil {
			return fmt.Errorf("failed to analyze %s: %w", config.TableName, err)
		}
	}

	indexSQL := fmt.Sprintf(`
		CREATE INDEX IF NOT EXISTS idx_%s_%s
		ON %s USING GIN(%s)`,
		config.TableName, indexSuffix, config.TableName, column,
	)
	if err := db.Exec(indexSQL).Error; err != nil {
		return fmt.Errorf("failed to create %s index on %s: %w", column, config.TableName, err)
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

		// rawTerm preserves hyphens so a query term tokenizes the same way the
		// stored search_vector did (it is generated from the raw column text).
		// It is only ever passed as a bind parameter, never concatenated into
		// SQL, and its character set is restricted (letters/digits/space/_/.-).
		rawTerm := query.RawTerm
		if rawTerm == "" {
			rawTerm = query.Term
		}

		switch query.Mode {
		case ModePrefix:
			// Derive the prefix tsquery from the raw term's OWN lexemes so it
			// tokenizes IDENTICALLY to search_vector. A naive whitespace-split
			// + ':*' mishandles hyphenated number/word compounds: Postgres
			// parses e.g. "2024-q3" or "1719-7ab" into a signed-int lexeme
			// ("-7") plus the remainder, so the split query never matches its
			// own row (~27% of hyphen+digit tokens, measured). The lexemes are
			// already english-normalized, so the OUTER to_tsquery uses 'simple'
			// to avoid stemming them a second time. The subquery depends only on
			// the bind parameter, so Postgres evaluates it once (InitPlan) and
			// still uses the GIN index on search_vector.
			//
			// The same query over unstemmed words catches a prefix longer than
			// its word's stem ("runni" for "running", stored as 'run'). The
			// stemmed side stays for the opposite case ("runs" for "running").
			return db.Where(
				fmt.Sprintf(`(%[1]s.search_vector @@ to_tsquery('simple', (
					SELECT string_agg(lexeme || ':*', ' & ')
					FROM unnest(to_tsvector('english', ?))
				)) OR %[1]s.search_vector_simple @@ to_tsquery('simple', (
					SELECT string_agg(lexeme || ':*', ' & ')
					FROM unnest(to_tsvector('simple', ?))
				)))`, tableName),
				rawTerm, rawTerm,
			)

		case ModeFuzzy:
			// Use trigram % operator which can use the GIN index
			// The % operator uses pg_trgm.similarity_threshold (default 0.3)
			return db.Where(
				fmt.Sprintf("%s.name %% ?", tableName),
				escapedTerm,
			)

		default: // ModeExact
			// plainto_tsquery tokenizes with the same 'english' parser as the
			// stored vector, so pass the hyphen-preserving raw term; a
			// hyphen-collapsed term would mis-split compounds as above.
			return db.Where(
				fmt.Sprintf("%s.search_vector @@ plainto_tsquery('english', ?)", tableName),
				rawTerm,
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
			"GREATEST(ts_rank(%[1]s.search_vector, to_tsquery('english', ?)), ts_rank(%[1]s.search_vector_simple, to_tsquery('simple', ?)))",
			tableName,
		), []interface{}{tsquery, tsquery}

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
