package template_filters

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/models/types"
	"net/url"
)

//goland:noinspection GoUnusedParameter
func urlFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	interfaceVal := in.Interface()
	input, ok := interfaceVal.(types.URL)

	if !ok {
		strInput, okStr := interfaceVal.(string)

		if okStr {
			return pongo2.AsValue(strInput), nil
		}

		input2 := interfaceVal.(*types.URL)

		if input2 == nil {
			return pongo2.AsValue(""), nil
		}

		input = *input2
	}

	converted := url.URL(input)

	return pongo2.AsValue(converted.String()), nil
}
