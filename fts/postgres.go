package fts

import (
	"fmt"
	"strings"
	"time"
	"unicode"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
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

// simpleIndexLockKey is the advisory lock held while the unstemmed prefix
// indexes are checked and built ("MRFTSSMP").
const simpleIndexLockKey = int64(0x4D52465453534D50)

// EnsureSimpleIndexes builds the GIN indexes on simpleVectorExpr that are
// missing, with CREATE INDEX CONCURRENTLY, which blocks neither reads nor
// writes; prefix search is correct without them, only slower. Like
// models.EnsureJobFilterIndexes, a build that failed leaves an invalid index
// that IF NOT EXISTS would skip forever, so an invalid one is dropped and
// rebuilt; and since a build in progress also reads as invalid, the pass runs
// under an advisory lock on one reserved connection, so a server that finds
// another one building waits for it instead of dropping its work.
func EnsureSimpleIndexes(db *gorm.DB) error {
	return db.Connection(func(conn *gorm.DB) error {
		conn = conn.Session(&gorm.Session{})
		if err := pollAdvisoryLock(conn, simpleIndexLockKey); err != nil {
			return fmt.Errorf("lock FTS prefix indexes: %w", err)
		}
		defer conn.Exec("SELECT pg_advisory_unlock(?)", simpleIndexLockKey)
		for _, config := range EntityConfigs {
			if err := ensureSimpleIndex(conn, config); err != nil {
				return err
			}
		}
		return nil
	})
}

// pollAdvisoryLock takes a session advisory lock by polling pg_try_advisory_lock
// rather than blocking in pg_advisory_lock. CREATE INDEX CONCURRENTLY waits for
// every transaction older than its own, and a session blocked in
// pg_advisory_lock is one: a second server waiting for the lock and the first
// server's build then wait on each other until Postgres aborts one as a
// deadlock. Between polls the waiter has no statement running.
func pollAdvisoryLock(conn *gorm.DB, key int64) error {
	for {
		var locked bool
		if err := conn.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&locked).Error; err != nil {
			return err
		}
		if locked {
			return nil
		}
		time.Sleep(advisoryLockPoll)
	}
}

// advisoryLockPoll is how often a server waiting for another one's index
// build tries the lock again.
const advisoryLockPoll = 250 * time.Millisecond

// EnsureSimpleIndexesOnOwnConnection is EnsureSimpleIndexes through a
// one-connection handle of its own, opened from dsn and closed when the pass
// ends, so a build that takes minutes holds none of the application's pool
// (with -max-db-connections=1, all of it).
func EnsureSimpleIndexesOnOwnConnection(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open the FTS prefix index connection: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open the FTS prefix index connection: %w", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	return EnsureSimpleIndexes(db)
}

func ensureSimpleIndex(conn *gorm.DB, config EntityFTSConfig) error {
	name := fmt.Sprintf("idx_%s_fts_simple", config.TableName)
	var found []struct{ Valid bool }
	if err := conn.Raw(`SELECT i.indisvalid AS valid FROM pg_class c
		JOIN pg_index i ON i.indexrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = ? AND n.nspname = current_schema()`, name).Scan(&found).Error; err != nil {
		return fmt.Errorf("inspect %s: %w", name, err)
	}
	if len(found) == 1 && found[0].Valid {
		return nil
	}
	if len(found) == 1 {
		// Invalid, and this server holds the lock: a failed build's leftover.
		if err := conn.Exec("DROP INDEX CONCURRENTLY IF EXISTS " + name).Error; err != nil {
			return fmt.Errorf("drop %s: %w", name, err)
		}
	}
	statement := fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s USING GIN((%s))",
		name, config.TableName, simpleVectorExpr(config, ""))
	if err := conn.Exec(statement).Error; err != nil {
		return fmt.Errorf("create %s: %w", name, err)
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
			words := prefixWords(rawTerm)
			if len(words) == 0 {
				return db.Where("1 = 0")
			}
			all := strings.Join(words, " ")
			simple := simpleVectorExpr(*config, tableName+".")
			for _, word := range words {
				db = db.Where(
					fmt.Sprintf(`(%s.search_vector @@ to_tsquery('simple', (
						SELECT string_agg(lexeme || ':*', ' & ')
						FROM unnest(to_tsvector('english', ?))
					)) OR %s @@ to_tsquery('simple', (
						SELECT string_agg(lexeme || ':*', ' & ')
						FROM unnest(to_tsvector('simple', ?))
					)) OR (to_tsvector('english', ?) = ''::tsvector AND to_tsvector('english', ?) <> ''::tsvector))`,
						tableName, simple),
					word, word, word, all,
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

// Bounds on a prefix query, which repeats each word in its SQL: past them a
// query is no more selective, only larger.
const (
	maxPrefixWords   = 16
	maxPrefixWordLen = 100
)

// prefixWords splits a prefix query into the words it matches on: distinct,
// each holding a letter or digit (a word of punctuation has no lexemes and
// would only make the query match nothing), at most maxPrefixWords of them,
// each cut to maxPrefixWordLen runes (a shorter prefix still matches).
func prefixWords(term string) []string {
	seen := make(map[string]bool)
	var words []string
	for _, word := range strings.Fields(term) {
		if !strings.ContainsFunc(word, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
			continue
		}
		if runes := []rune(word); len(runes) > maxPrefixWordLen {
			word = string(runes[:maxPrefixWordLen])
		}
		if seen[word] {
			continue
		}
		seen[word] = true
		words = append(words, word)
		if len(words) == maxPrefixWords {
			break
		}
	}
	return words
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
