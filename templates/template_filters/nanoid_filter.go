package template_filters

import (
	"github.com/flosch/pongo2/v4"
	"github.com/matoous/go-nanoid/v2"
)

func nanoidFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.String()
	id, err := gonanoid.New()

	if err != nil {
		return pongo2.AsValue(input), nil
	}

	var res string

	if input != "" {
		res = input + "___" + id
	} else {
		res = id
	}

	return pongo2.AsValue(res), nil
}
