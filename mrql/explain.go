package mrql

import "gorm.io/gorm"

// ExplainStatement is one SQL statement that would run to satisfy a query. sql
// is parameterized (bind placeholders); interpolated inlines the vars for
// display only, using the same interpolation the GORM logger uses.
type ExplainStatement struct {
	Label        string `json:"label"`
	SQL          string `json:"sql"`
	Vars         []any  `json:"vars"`
	Interpolated string `json:"interpolated"`
}

// ExplainDB extracts the SQL, bind vars, and display-interpolated SQL for a
// built query WITHOUT executing it, via a DryRun session. dest fixes the SELECT
// column shape: pass a pointer to the model slice for flat/bucket queries, or
// &[]map[string]any{} for aggregated rows.
func ExplainDB(db *gorm.DB, label string, dest any) ExplainStatement {
	dry := db.Session(&gorm.Session{DryRun: true}).Find(dest)
	sql := dry.Statement.SQL.String()
	vars := append([]any{}, dry.Statement.Vars...)
	return ExplainStatement{
		Label:        label,
		SQL:          sql,
		Vars:         vars,
		Interpolated: dry.Dialector.Explain(sql, vars...),
	}
}
