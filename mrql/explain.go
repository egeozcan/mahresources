package mrql

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"
)

var ErrNativeExplainUnsupportedDialect = errors.New("native explain unsupported for database dialect")

// ExplainStatement is one SQL statement that would run to satisfy a query. SQL
// is parameterized (bind placeholders); Interpolated inlines vars for display
// only, using the same interpolation the GORM logger uses.
type ExplainStatement struct {
	Label        string      `json:"label"`
	SQL          string      `json:"sql"`
	Vars         []any       `json:"vars"`
	Interpolated string      `json:"interpolated"`
	NativePlan   *NativePlan `json:"nativePlan,omitempty"`
}

// NativePlan preserves the database's native optimizer representation under a
// stable envelope. Plan is a JSON array for both supported dialects: normalized
// EXPLAIN QUERY PLAN rows on SQLite and PostgreSQL's native FORMAT JSON value.
type NativePlan struct {
	Dialect string          `json:"dialect"`
	Format  string          `json:"format"`
	Plan    json.RawMessage `json:"plan"`
}

// SQLitePlanRow is one row returned by SQLite EXPLAIN QUERY PLAN.
type SQLitePlanRow struct {
	ID      int64  `json:"id"`
	Parent  int64  `json:"parent"`
	NotUsed int64  `json:"notused"`
	Detail  string `json:"detail"`
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

// NativeExplain asks the active database optimizer to plan a generated
// statement without executing the underlying SELECT. It always executes the
// parameterized SQL with its original bind vars; Interpolated is display-only.
func NativeExplain(ctx context.Context, db *gorm.DB, statement ExplainStatement) (*NativePlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session := db.WithContext(ctx)
	if session.Statement == nil || session.Statement.ConnPool == nil {
		return nil, errors.New("native explain has no database connection pool")
	}

	switch dialect := db.Dialector.Name(); dialect {
	case "sqlite":
		rows, err := session.Statement.ConnPool.QueryContext(ctx, "EXPLAIN QUERY PLAN "+statement.SQL, statement.Vars...)
		if err != nil {
			return nil, fmt.Errorf("native explain %q on sqlite: %w", statement.Label, err)
		}
		var planRows []SQLitePlanRow
		for rows.Next() {
			var row SQLitePlanRow
			if err := rows.Scan(&row.ID, &row.Parent, &row.NotUsed, &row.Detail); err != nil {
				_ = rows.Close()
				return nil, fmt.Errorf("scan native explain %q on sqlite: %w", statement.Label, err)
			}
			planRows = append(planRows, row)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read native explain %q on sqlite: %w", statement.Label, err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close native explain %q on sqlite: %w", statement.Label, err)
		}
		if planRows == nil {
			planRows = []SQLitePlanRow{}
		}
		raw, err := json.Marshal(planRows)
		if err != nil {
			return nil, fmt.Errorf("encode native explain %q on sqlite: %w", statement.Label, err)
		}
		return &NativePlan{Dialect: "sqlite", Format: "query-plan", Plan: raw}, nil

	case "postgres":
		rows, err := session.Statement.ConnPool.QueryContext(ctx, "EXPLAIN (FORMAT JSON) "+statement.SQL, statement.Vars...)
		if err != nil {
			return nil, fmt.Errorf("native explain %q on postgres: %w", statement.Label, err)
		}
		if !rows.Next() {
			rowErr := rows.Err()
			_ = rows.Close()
			if rowErr != nil {
				return nil, fmt.Errorf("read native explain %q on postgres: %w", statement.Label, rowErr)
			}
			return nil, fmt.Errorf("native explain %q on postgres returned no plan", statement.Label)
		}
		var value any
		if err := rows.Scan(&value); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan native explain %q on postgres: %w", statement.Label, err)
		}
		if rows.Next() {
			_ = rows.Close()
			return nil, fmt.Errorf("native explain %q on postgres returned multiple plans", statement.Label)
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("read native explain %q on postgres: %w", statement.Label, err)
		}
		if err := rows.Close(); err != nil {
			return nil, fmt.Errorf("close native explain %q on postgres: %w", statement.Label, err)
		}
		raw, err := postgresPlanJSON(value)
		if err != nil {
			return nil, fmt.Errorf("decode native explain %q on postgres: %w", statement.Label, err)
		}
		return &NativePlan{Dialect: "postgres", Format: "json", Plan: raw}, nil

	default:
		return nil, fmt.Errorf("%w %q", ErrNativeExplainUnsupportedDialect, dialect)
	}
}

func postgresPlanJSON(value any) (json.RawMessage, error) {
	var raw []byte
	switch typed := value.(type) {
	case []byte:
		raw = append([]byte(nil), typed...)
	case string:
		raw = []byte(typed)
	case json.RawMessage:
		raw = append([]byte(nil), typed...)
	default:
		return nil, fmt.Errorf("unexpected plan value type %T", value)
	}
	if !json.Valid(raw) {
		return nil, errors.New("database returned invalid JSON plan")
	}
	return json.RawMessage(raw), nil
}
