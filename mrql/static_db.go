package mrql

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

// staticDialector is a gorm.Dialector that only names a dialect. It opens no
// connection, so a query built on it can be checked but never run.
type staticDialector struct{ name string }

func (d staticDialector) Name() string                                        { return d.name }
func (staticDialector) Initialize(*gorm.DB) error                             { return nil }
func (staticDialector) Migrator(*gorm.DB) gorm.Migrator                       { return nil }
func (staticDialector) DataTypeOf(*schema.Field) string                       { return "" }
func (staticDialector) DefaultValueOf(*schema.Field) clause.Expression        { return nil }
func (staticDialector) BindVarTo(clause.Writer, *gorm.Statement, interface{}) {}
func (staticDialector) QuoteTo(clause.Writer, string)                         {}
func (staticDialector) Explain(string, ...interface{}) string                 { return "" }

var staticDBs = map[bool]*gorm.DB{
	false: openStaticDB("sqlite"),
	true:  openStaticDB("postgres"),
}

func openStaticDB(name string) *gorm.DB {
	db, err := gorm.Open(staticDialector{name: name}, &gorm.Config{DryRun: true, Logger: logger.Discard})
	if err != nil {
		// Initialize cannot fail, so neither can Open.
		panic(err)
	}
	return db
}

// StaticDB is a database handle for the SQLite or PostgreSQL dialect that
// has no connection pool and runs in DryRun: the translator can build a query
// on it, and so report every error that does not depend on stored data, but
// nothing reaches a database. Callers pass TranslateOptions.FTSAvailable, since
// the translator would otherwise probe the schema for the FTS tables.
func StaticDB(postgres bool) *gorm.DB {
	return staticDBs[postgres]
}

// FieldNames lists an entity's own MRQL fields in declaration order; the
// unspecified entity lists the common fields every entity has. Aliases are
// listed too.
func FieldNames(entityType EntityType) []string {
	var fields []FieldDef
	switch entityType {
	case EntityResource:
		fields = resourceFields
	case EntityNote:
		fields = noteFields
	case EntityGroup:
		fields = groupFields
	default:
		fields = commonFields
	}
	names := make([]string, 0, len(fields))
	for _, f := range fields {
		names = append(names, f.Name)
	}
	return names
}
