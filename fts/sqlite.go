package fts

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// SQLiteFTS implements FTSProvider for SQLite using FTS5 virtual tables
type SQLiteFTS struct{}

// NewSQLiteFTS creates a new SQLite FTS provider
func NewSQLiteFTS() *SQLiteFTS {
	return &SQLiteFTS{}
}

// Setup creates FTS5 tables and triggers for all configured entities
func (s *SQLiteFTS) Setup(db *gorm.DB) error {
	for entityType, config := range EntityConfigs {
		if err := s.setupTable(db, config); err != nil {
			return fmt.Errorf("failed to setup FTS for %s: %w", entityType, err)
		}
	}
	return nil
}

func (s *SQLiteFTS) setupTable(db *gorm.DB, config EntityFTSConfig) error {
	ftsTableName := config.TableName + "_fts"
	columns := strings.Join(config.Columns, ", ")

	// Check if FTS table already exists
	var count int
	db.Raw("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", ftsTableName).Scan(&count)

	if count == 0 {
		// Create FTS5 virtual table with external content
		createSQL := fmt.Sprintf(`
			CREATE VIRTUAL TABLE %s USING fts5(
				%s,
				content='%s',
				content_rowid='id'
			)`,
			ftsTableName, columns, config.TableName,
		)
		if err := db.Exec(createSQL).Error; err != nil {
			return fmt.Errorf("failed to create FTS table %s: %w", ftsTableName, err)
		}

		// Create triggers to keep FTS in sync
		if err := s.createTriggers(db, config); err != nil {
			return err
		}

		// Initial population of FTS table
		if err := s.populateFTS(db, config); err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLiteFTS) createTriggers(db *gorm.DB, config EntityFTSConfig) error {
	ftsTableName := config.TableName + "_fts"
	columns := strings.Join(config.Columns, ", ")
	newColumns := s.buildPrefixedColumns(config.Columns, "new")
	oldColumns := s.buildPrefixedColumns(config.Columns, "old")

	// INSERT trigger
	insertTrigger := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS %s_ai AFTER INSERT ON %s BEGIN
			INSERT INTO %s(rowid, %s) VALUES (new.id, %s);
		END`,
		config.TableName, config.TableName,
		ftsTableName, columns, newColumns,
	)
	if err := db.Exec(insertTrigger).Error; err != nil {
		return fmt.Errorf("failed to create insert trigger for %s: %w", config.TableName, err)
	}

	// DELETE trigger
	deleteTrigger := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS %s_ad AFTER DELETE ON %s BEGIN
			INSERT INTO %s(%s, rowid, %s) VALUES('delete', old.id, %s);
		END`,
		config.TableName, config.TableName,
		ftsTableName, ftsTableName, columns, oldColumns,
	)
	if err := db.Exec(deleteTrigger).Error; err != nil {
		return fmt.Errorf("failed to create delete trigger for %s: %w", config.TableName, err)
	}

	// UPDATE trigger
	updateTrigger := fmt.Sprintf(`
		CREATE TRIGGER IF NOT EXISTS %s_au AFTER UPDATE ON %s BEGIN
			INSERT INTO %s(%s, rowid, %s) VALUES('delete', old.id, %s);
			INSERT INTO %s(rowid, %s) VALUES (new.id, %s);
		END`,
		config.TableName, config.TableName,
		ftsTableName, ftsTableName, columns, oldColumns,
		ftsTableName, columns, newColumns,
	)
	if err := db.Exec(updateTrigger).Error; err != nil {
		return fmt.Errorf("failed to create update trigger for %s: %w", config.TableName, err)
	}

	return nil
}

func (s *SQLiteFTS) populateFTS(db *gorm.DB, config EntityFTSConfig) error {
	ftsTableName := config.TableName + "_fts"
	columns := strings.Join(config.Columns, ", ")

	// Populate FTS table with existing data
	populateSQL := fmt.Sprintf(`
		INSERT INTO %s(rowid, %s)
		SELECT id, %s FROM %s`,
		ftsTableName, columns,
		columns, config.TableName,
	)
	if err := db.Exec(populateSQL).Error; err != nil {
		return fmt.Errorf("failed to populate FTS table %s: %w", ftsTableName, err)
	}

	return nil
}

func (s *SQLiteFTS) buildPrefixedColumns(columns []string, prefix string) string {
	var result []string
	for _, col := range columns {
		result = append(result, prefix+"."+col)
	}
	return strings.Join(result, ", ")
}

// BuildSearchScope returns a GORM scope for FTS search
func (s *SQLiteFTS) BuildSearchScope(tableName string, columns []string, query ParsedQuery) func(*gorm.DB) *gorm.DB {
	ftsTableName := tableName + "_fts"

	return func(db *gorm.DB) *gorm.DB {
		if query.Term == "" {
			return db
		}

		escapedTerm := EscapeForFTS(query.Term)

		switch query.Mode {
		case ModePrefix:
			// FTS5 uses * for prefix matching
			// Split terms and add * to each
			terms := strings.Fields(escapedTerm)
			var matchParts []string
			for _, term := range terms {
				matchParts = append(matchParts, term+"*")
			}
			matchExpr := strings.Join(matchParts, " ")

			return db.Where(
				fmt.Sprintf("%s.id IN (SELECT rowid FROM %s WHERE %s MATCH ?)",
					tableName, ftsTableName, ftsTableName),
				matchExpr,
			)

		case ModeFuzzy:
			// SQLite FTS5 doesn't have built-in fuzzy search
			// Fallback to LIKE with single-character wildcards for basic typo tolerance
			return s.fuzzyFallback(db, tableName, escapedTerm)

		default: // ModeExact
			return db.Where(
				fmt.Sprintf("%s.id IN (SELECT rowid FROM %s WHERE %s MATCH ?)",
					tableName, ftsTableName, ftsTableName),
				escapedTerm,
			)
		}
	}
}

// fuzzyFallback provides basic fuzzy matching for SQLite using LIKE patterns
func (s *SQLiteFTS) fuzzyFallback(db *gorm.DB, tableName, term string) *gorm.DB {
	if len(term) <= 2 {
		// For very short terms, just use contains
		return db.Where(tableName+".name LIKE ?", "%"+term+"%")
	}

	// Build OR conditions for single-char wildcards at each position
	// For "test" -> match "t_st", "_est", "te_t", "tes_"
	var conditions []string
	var args []interface{}

	for i := range term {
		pattern := term[:i] + "_" + term[i+1:]
		conditions = append(conditions, tableName+".name LIKE ?")
		args = append(args, "%"+pattern+"%")
	}

	// Also include exact substring match
	conditions = append(conditions, tableName+".name LIKE ?")
	args = append(args, "%"+term+"%")

	return db.Where(strings.Join(conditions, " OR "), args...)
}

// GetRankExpr returns a SQL expression for relevance ranking
func (s *SQLiteFTS) GetRankExpr(tableName string, columns []string, query ParsedQuery) (string, []interface{}) {
	if query.Term == "" {
		return "0", nil
	}

	ftsTableName := tableName + "_fts"
	escapedTerm := EscapeForFTS(query.Term)

	switch query.Mode {
	case ModeFuzzy:
		// For fuzzy search fallback, we can't get proper ranking
		// Return a simple expression that prefers shorter names (more likely exact matches)
		return fmt.Sprintf("(1.0 / (1 + length(%s.name)))", tableName), nil

	case ModePrefix:
		// FTS5 bm25() returns negative values (lower = better match)
		// We negate it for consistent "higher is better" semantics
		terms := strings.Fields(escapedTerm)
		var matchParts []string
		for _, term := range terms {
			matchParts = append(matchParts, term+"*")
		}
		matchExpr := strings.Join(matchParts, " ")

		return fmt.Sprintf(
			"(SELECT -bm25(%s) FROM %s WHERE rowid = %s.id AND %s MATCH ?)",
			ftsTableName, ftsTableName, tableName, ftsTableName,
		), []interface{}{matchExpr}

	default: // ModeExact
		return fmt.Sprintf(
			"(SELECT -bm25(%s) FROM %s WHERE rowid = %s.id AND %s MATCH ?)",
			ftsTableName, ftsTableName, tableName, ftsTableName,
		), []interface{}{escapedTerm}
	}
}

// SupportsFeature checks if a feature is supported
func (s *SQLiteFTS) SupportsFeature(feature string) bool {
	switch feature {
	case "prefix", "ranking":
		return true
	case "fuzzy":
		return false // Limited fallback only
	default:
		return false
	}
}
