package template_filters

import (
	"encoding/json"

	"github.com/flosch/pongo2/v4"
)

func jsonFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	input := in.Interface()

	if str, ok := input.(string); ok {
		// Strings must be properly JSON-encoded (wrapped in quotes, special chars escaped)
		// rather than passed through raw. Raw pass-through breaks when the string
		// contains characters meaningful in the embedding context (e.g., single quotes
		// in HTML attributes, backtick interpolation in JS template literals).
		encoded, err := json.Marshal(str)
		if err != nil {
			return nil, &pongo2.Error{
				Sender:    "filter:json",
				OrigError: err,
			}
		}
		return pongo2.AsValue(string(encoded)), nil
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
