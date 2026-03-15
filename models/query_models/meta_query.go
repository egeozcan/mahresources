package query_models

import (
	"strconv"
	"strings"
)

type ColumnMeta struct {
	Key       string `json:"name"`
	Value     any    `json:"value"`
	Operation string `json:"operation"`
}

// validOperations is the set of recognized meta query operations.
// Used to distinguish "key:OP:value" from "key:value-with-colon".
var validOperations = map[string]bool{
	"EQ": true, "LI": true, "NE": true, "NL": true,
	"GT": true, "GE": true, "LT": true, "LE": true,
}

func ParseMeta(input string) ColumnMeta {
	// Split into at most 3 parts so colons inside the value are preserved.
	parts := strings.SplitN(input, ":", 3)

	var key, value, operation string

	switch {
	case len(parts) == 3 && validOperations[parts[1]]:
		// key:OP:value (value may contain colons)
		key = parts[0]
		operation = parts[1]
		value = parts[2]
	case len(parts) >= 2:
		// key:value (value may contain colons)
		key = parts[0]
		operation = "LI"
		value = strings.Join(parts[1:], ":")
	default:
		return ColumnMeta{}
	}

	var parsedValue any
	if value == "true" || value == "false" {
		parsedValue = value == "true"
	} else if value == "null" {
		parsedValue = nil
	} else if strings.HasPrefix(value, "\"") && strings.HasSuffix(value, "\"") && strings.Count(value, "\"") == 2 {
		parsedValue = strings.Trim(value, "\"")
	} else {
		float, err := strconv.ParseFloat(value, 64)
		if err != nil {
			parsedValue = value
		} else {
			parsedValue = float
		}
	}

	return ColumnMeta{
		Key:       key,
		Value:     parsedValue,
		Operation: operation,
	}
}
