package template_filters

import (
	"encoding/json"
	"github.com/flosch/pongo2/v4"
)

func jsonFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface()

	if str, ok := input.(string); ok {
		return pongo2.AsValue(str), nil
	}

	jsonValue, err := json.Marshal(input)
	if err != nil {
		return nil, &pongo2.Error{
			Sender:    "filter:json",
			OrigError: err,
		}
	}

	return pongo2.AsValue(string(jsonValue)), nil
}
