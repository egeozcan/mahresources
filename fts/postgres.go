package fts

import (
	"fmt"
	"log"
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
	// Built in the background: on a large existing table the build takes a
	// while, and prefix search is correct without the index, only slower.
	go func() {
		for _, config := range EntityConfigs {
			if err := p.setupSimpleIndex(db, config); err != nil {
				// The next startup retries. Prefix search works meanwhile.
				log.Printf("FTS: %v", err)
				return
			}
		}
	}()
	return nil
}

// simpleVectorExpr is the unstemmed tsvector of a table's search columns.
// Prefix search matches it as well as the stemmed search_vector, because a
// typed prefix longer than a word's English stem ("Mentionab" for 'mention',
// "runni" for 'run') matches no stem. It is an expression index rather than a
// stored column so that adding it rewrites no table; the query must use this
// exact expression for the index to apply.
//
// qualifier, when not empty, prefixes each column ("resources."); the index
// still applies, as Postgres matches the columns, not the text.
func simpleVectorExpr(config EntityFTSConfig, qualifier string) string {
	cols := make([]string, 0, len(config.Columns))
	for _, col := range config.Columns {
		cols = append(cols, fmt.Sprintf("coalesce(%s%s,'')", qualifier, col))
	}
	return fmt.Sprintf("to_tsvector('simple', %s)", strings.Join(cols, " || ' ' || "))
}

// setupSimpleIndex builds the GIN index on simpleVectorExpr with CREATE INDEX
// CONCURRENTLY, which blocks neither reads nor writes. A build that failed
// part way leaves an invalid index that IF NOT EXISTS would keep skipping, so
// that one is dropped and rebuilt.
func (p *PostgresFTS) setupSimpleIndex(db *gorm.DB, config EntityFTSConfig) error {
	name := fmt.Sprintf("idx_%s_fts_simple", config.TableName)
	var invalid int64
	db.Raw(`
		SELECT COUNT(*) FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid
		WHERE c.relname = ? AND NOT i.indisvalid
	`, name).Scan(&invalid)
	if invalid > 0 {
		if err := db.Exec(fmt.Sprintf("DROP INDEX CONCURRENTLY IF EXISTS %s", name)).Error; err != nil {
			return fmt.Errorf("failed to drop invalid index %s: %w", name, err)
		}
	}
	sql := fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s USING GIN((%s))",
		name, config.TableName, simpleVectorExpr(config, ""))
	if err := db.Exec(sql).Error; err != nil {
		return fmt.Errorf("failed to create index %s: %w", name, err)
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
			// Each word must match, stemmed or unstemmed (simpleVectorExpr):
			// "runs mentionab" finds "Running Mentionable" though only the
			// stemmed side matches "runs" and only the unstemmed side
			// "mentionab". A word English ignores (a stopword like "the")
			// constrains nothing unless every word is one.
			config := configForTable(tableName)
			if config == nil {
				return db.Where("1 = 0")
			}
			simple := simpleVectorExpr(*config, tableName+".")
			for _, word := range strings.Fields(rawTerm) {
				db = db.Where(
					fmt.Sprintf(`(%s.search_vector @@ to_tsquery('simple', (
						SELECT string_agg(lexeme || ':*', ' & ')
						FROM unnest(to_tsvector('english', ?))
					)) OR %s @@ to_tsquery('simple', (
						SELECT string_agg(lexeme || ':*', ' & ')
						FROM unnest(to_tsvector('simple', ?))
					)) OR (to_tsvector('english', ?) = ''::tsvector AND to_tsvector('english', ?) <> ''::tsvector))`,
						tableName, simple),
					word, word, word, rawTerm,
				)
			}
			return db

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

// configForTable returns the FTS config whose table is tableName.
func configForTable(tableName string) *EntityFTSConfig {
	for _, config := range EntityConfigs {
		if config.TableName == tableName {
			return &config
		}
	}
	return nil
}
