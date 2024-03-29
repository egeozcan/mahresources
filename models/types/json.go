package types

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

type JsonOperation string

const (
	OperatorEquals              JsonOperation = "="
	OperatorLike                              = "LIKE"
	OperatorNotEquals                         = "<>"
	OperatorNotLike                           = "NOT LIKE"
	OperatorGreaterThan                       = ">"
	OperatorGreaterThanOrEquals               = ">="
	OperatorLessThan                          = "<"
	OperatorLessThanOrEquals                  = "<="
	OperatorHasKeys                           = "HAS_KEYS"
)

// JSON defined JSON data type, need to implements driver.Valuer, sql.Scanner interface
// taken from https://github.com/go-gorm/datatypes/blob/master/json.go and was modified a bit to add operator support
type JSON json.RawMessage

// Value return json value, implement driver.Valuer interface
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	bytes, err := json.RawMessage(j).MarshalJSON()
	return string(bytes), err
}

// Scan scan value into Jsonb, implements sql.Scanner interface
func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	result := json.RawMessage{}
	err := json.Unmarshal(bytes, &result)
	*j = JSON(result)
	return err
}

// MarshalJSON to output non base64 encoded []byte
func (j JSON) MarshalJSON() ([]byte, error) {
	return json.RawMessage(j).MarshalJSON()
}

// UnmarshalJSON to deserialize []byte
func (j *JSON) UnmarshalJSON(b []byte) error {
	result := json.RawMessage{}
	err := result.UnmarshalJSON(b)
	*j = JSON(result)
	return err
}

func (j JSON) String() string {
	return string(j)
}

// GormDataType gorm common data type
func (j JSON) GormDataType() string {
	return "json"
}

// GormDBDataType gorm db data type
func (j JSON) GormDBDataType(db *gorm.DB, _ *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "JSON"
	case "mysql":
		return "JSON"
	case "postgres":
		return "JSONB"
	}
	return ""
}

func (j JSON) GormValue(_ context.Context, _ *gorm.DB) clause.Expr {
	data, _ := j.MarshalJSON()
	return gorm.Expr("?", string(data))
}

// JSONQueryExpression json query expression, implements clause.Expression interface to use as querier
type JSONQueryExpression struct {
	column    string
	keys      []string
	operation JsonOperation
	value     any
}

// JSONQuery query column as json
func JSONQuery(column string) *JSONQueryExpression {
	return &JSONQueryExpression{column: column}
}

// HasKey returns clause.Expression
//goland:noinspection GoUnnecessarilyExportedIdentifiers
func (jsonQuery *JSONQueryExpression) HasKey(keys ...string) *JSONQueryExpression {
	jsonQuery.keys = keys
	jsonQuery.operation = OperatorHasKeys
	return jsonQuery
}

// Operation returns clause.Expression
func (jsonQuery *JSONQueryExpression) Operation(operation JsonOperation, value any, keys ...string) *JSONQueryExpression {
	if len(keys) == 1 && strings.Contains(keys[0], ".") {
		keys = strings.Split(keys[0], ".")
	}

	jsonQuery.keys = keys
	jsonQuery.operation = operation
	jsonQuery.value = value
	return jsonQuery
}

// Build implements clause.Expression
//goland:noinspection GoUnhandledErrorResult
func (jsonQuery *JSONQueryExpression) Build(builder clause.Builder) {
	if stmt, ok := builder.(*gorm.Statement); ok {
		switch stmt.Dialector.Name() {
		case "mysql", "sqlite":
			switch jsonQuery.operation {
			case OperatorHasKeys:
				if len(jsonQuery.keys) > 0 {
					builder.WriteString("JSON_EXTRACT(" + stmt.Quote(jsonQuery.column) + ",")
					builder.AddVar(stmt, "$."+strings.Join(jsonQuery.keys, "."))
					builder.WriteString(") IS NOT NULL")
				}
			case OperatorEquals, OperatorNotEquals, OperatorLike, OperatorNotLike, OperatorGreaterThan, OperatorGreaterThanOrEquals, OperatorLessThan, OperatorLessThanOrEquals:
				if len(jsonQuery.keys) > 0 {
					builder.WriteString("JSON_EXTRACT(" + stmt.Quote(jsonQuery.column) + ",")
					builder.AddVar(stmt, "$."+strings.Join(jsonQuery.keys, "."))
					str := fmt.Sprintf(") %v ", jsonQuery.operation)
					builder.WriteString(str)
					if _, ok := jsonQuery.value.(bool); ok {
						builder.WriteString(fmt.Sprint(jsonQuery.value))
					} else {
						if jsonQuery.operation == OperatorLike || jsonQuery.operation == OperatorNotLike {
							jsonQuery.value = fmt.Sprintf("%%%v%%", jsonQuery.value)
						}

						stmt.AddVar(builder, jsonQuery.value)
					}
				}
			}
		case "postgres":
			switch jsonQuery.operation {
			case OperatorHasKeys:
				if len(jsonQuery.keys) > 0 {
					stmt.WriteQuoted(jsonQuery.column)
					stmt.WriteString("::jsonb")
					for _, key := range jsonQuery.keys[0 : len(jsonQuery.keys)-1] {
						stmt.WriteString(" -> ")
						stmt.AddVar(builder, key)
					}

					stmt.WriteString(" ? ")
					stmt.AddVar(builder, jsonQuery.keys[len(jsonQuery.keys)-1])
				}
			case OperatorEquals, OperatorNotEquals, OperatorLike, OperatorNotLike, OperatorGreaterThan, OperatorGreaterThanOrEquals, OperatorLessThan, OperatorLessThanOrEquals:
				if len(jsonQuery.keys) > 0 {

					isTextBased := false

					if jsonQuery.operation == OperatorLike || jsonQuery.operation == OperatorNotLike {
						isTextBased = true
					}

					builder.WriteString(fmt.Sprintf("(%v::jsonb #> ", stmt.Quote(jsonQuery.column)))

					stmt.AddVar(builder, fmt.Sprintf("{%v}", strings.Join(jsonQuery.keys, ",")))
					builder.WriteString(")")

					if isTextBased {
						stmt.WriteString("::text")
					}

					if jsonQuery.value == nil && jsonQuery.operation == OperatorNotEquals {
						builder.WriteString(" IS NOT ")
					} else if jsonQuery.value == nil && jsonQuery.operation == OperatorEquals {
						builder.WriteString(" IS ")
					} else {
						builder.WriteString(fmt.Sprintf(" %v ", jsonQuery.operation))
					}

					if jsonQuery.operation == OperatorLike || jsonQuery.operation == OperatorNotLike {
						jsonQuery.value = fmt.Sprintf("%%%v%%", jsonQuery.value)
					}

					switch jsonQuery.value.(type) {
					case string:
						if !isTextBased {
							stmt.WriteString("to_jsonb")
						}
						stmt.WriteString("(")
						stmt.AddVar(builder, jsonQuery.value)
						stmt.WriteString("::text)")
					case bool, float64:
						if !isTextBased {
							stmt.WriteString("to_jsonb")
						}
						stmt.WriteString(fmt.Sprintf("(%v)", jsonQuery.value))
					case nil:
						stmt.WriteString("NULL")
					default:
						stmt.AddVar(builder, fmt.Sprint(jsonQuery.value))
					}
				}
			}
		}
	}
}
