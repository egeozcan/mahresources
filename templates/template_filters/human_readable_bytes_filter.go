package template_filters

import (
	"github.com/c2h5oh/datasize"
	"github.com/flosch/pongo2/v4"
)

func humanReadableSize(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface().(int64)
	return pongo2.AsValue(datasize.ByteSize(input).HumanReadable()), nil
}
