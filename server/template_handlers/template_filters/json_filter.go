package template_filters

import (
	"encoding/json"
	"github.com/flosch/pongo2/v4"
)

//goland:noinspection GoUnusedParameter
func jsonFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface()

	switch input.(type) {
	case string:
		return pongo2.AsValue(input.(string)), nil
	}

	jsonValue, err := json.Marshal(&input)

	if err != nil {
		return pongo2.AsValue(input), nil
	}

	return pongo2.AsValue(string(jsonValue)), nil
}
