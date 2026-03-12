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

	err = pongo2.RegisterFilter("datetime", filterDateTime)

	if err != nil {
		fmt.Println("error when registering datetime filter", err)
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

	urlErr := pongo2.RegisterFilter("printUrl", urlFilter)

	if urlErr != nil {
		fmt.Println("error when registering url print filter", urlErr)
	}

	markdownErr := pongo2.RegisterFilter("markdown2", markDownFilter)

	if markdownErr != nil {
		fmt.Println("error when registering url markdown2 filter", markdownErr)
	}

	lookupErr := pongo2.RegisterFilter("lookup", lookupFilter)

	if lookupErr != nil {
		fmt.Println("error when registering lookup filter", lookupErr)
	}

	timeagoErr := pongo2.RegisterFilter("timeago", timeagoFilter)

	if timeagoErr != nil {
		fmt.Println("error when registering timeago filter", timeagoErr)
	}

	entityPathErr := pongo2.RegisterFilter("entityPath", entityPathFilter)

	if entityPathErr != nil {
		fmt.Println("error when registering entityPath filter", entityPathErr)
	}

	mentionsErr := pongo2.RegisterFilter("render_mentions", renderMentionsFilter)

	if mentionsErr != nil {
		fmt.Println("error when registering render_mentions filter", mentionsErr)
	}
}
