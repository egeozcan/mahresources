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

func ParseMeta(input string) ColumnMeta {
	var ret ColumnMeta
	parts := strings.Split(input, ":")
	switch len(parts) {
	case 2, 3:
		var parsedValue any
		value := parts[1]
		operation := "LI"

		if len(parts) == 3 {
			value = parts[2]
			operation = parts[1]
		}

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

		ret = ColumnMeta{
			Key:       parts[0],
			Value:     parsedValue,
			Operation: operation,
		}
	}
	return ret
}
