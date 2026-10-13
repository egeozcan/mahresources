package template_filters

import (
	"fmt"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
	"mahresources/models/types"
)

// sectionConfigFilter resolves a card's own carrier configuration instead of
// inheriting the surrounding detail page's sc value.
func sectionConfigFilter(in, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	data, ok := in.Interface().(types.JSON)
	if !ok {
		data = types.JSON(in.String())
	}
	switch param.String() {
	case "group":
		return pongo2.AsValue(models.ResolveGroupSectionConfig(&data)), nil
	case "resource":
		return pongo2.AsValue(models.ResolveResourceSectionConfig(&data)), nil
	case "note":
		return pongo2.AsValue(models.ResolveNoteSectionConfig(&data)), nil
	default:
		return nil, &pongo2.Error{Sender: "filter:sectionConfig", OrigError: fmt.Errorf("unknown entity type %q", param.String())}
	}
}

func init() {
	if err := pongo2.RegisterFilter("sectionConfig", sectionConfigFilter); err != nil {
		fmt.Println("error when registering sectionConfig filter", err)
	}
}
