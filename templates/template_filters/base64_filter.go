package template_filters

import (
	"encoding/base64"
	"github.com/flosch/pongo2/v4"
)

func base64Filter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface().([]byte)
	return pongo2.AsValue(base64.StdEncoding.EncodeToString(input)), nil
}
