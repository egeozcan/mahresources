package template_filters

import (
	"github.com/flosch/pongo2/v4"
)

var entityPaths = map[string]string{
	"resource": "/resource",
	"note":     "/note",
	"group":    "/group",
	"tag":      "/tag",
}

//goland:noinspection GoUnusedParameter
func entityPathFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if path, ok := entityPaths[in.String()]; ok {
		return pongo2.AsValue(path), nil
	}
	return pongo2.AsValue(""), nil
}
