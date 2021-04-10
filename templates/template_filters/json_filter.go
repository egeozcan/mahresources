package template_filters

import (
	"encoding/json"
	"github.com/flosch/pongo2/v4"
)

func jsonFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface()
	jsonValue, err := json.Marshal(&input)

	if err != nil {
		return pongo2.AsValue(input), nil
	}

	return pongo2.AsValue(string(jsonValue)), nil
}