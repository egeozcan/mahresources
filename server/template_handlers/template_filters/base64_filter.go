package template_filters

import (
	"encoding/base64"
	"github.com/flosch/pongo2/v4"
)

//goland:noinspection GoUnusedParameter
func base64Filter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	i := in.Interface()

	if i == nil {
		return pongo2.AsValue(""), nil
	}

	input := i.([]byte)

	if len(input) == 0 {
		return pongo2.AsValue(""), nil
	}

	return pongo2.AsValue(base64.StdEncoding.EncodeToString(input)), nil
}
