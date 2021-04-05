package template_filters

import (
	"encoding/base64"
	"fmt"
	"github.com/flosch/pongo2/v4"
)

func init() {
	err := pongo2.RegisterFilter("base64", base64Filter)

	if err != nil {
		fmt.Println("error when registering base64 filter", err)
	}
}

func base64Filter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface().([]byte)
	return pongo2.AsValue(base64.StdEncoding.EncodeToString(input)), nil
}
