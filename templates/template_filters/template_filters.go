package template_filters

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
)

func init() {
	err := pongo2.RegisterFilter("base64", base64Filter)

	if err != nil {
		fmt.Println("error when registering base64 filter", err)
	}

	humanReadableSizeErr := pongo2.RegisterFilter("humanReadableSize", humanReadableSize)

	if humanReadableSizeErr != nil {
		fmt.Println("error when registering humanReadableSize filter", humanReadableSizeErr)
	}

	nanoidErr := pongo2.RegisterFilter("nanoid", nanoidFilter)

	if nanoidErr != nil {
		fmt.Println("error when registering nanoid filter", nanoidErr)
	}

	jsonErr := pongo2.RegisterFilter("json", jsonFilter)

	if jsonErr != nil {
		fmt.Println("error when registering json filter", jsonErr)
	}
}
