package context_providers

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/http_utils"
	"mahresources/templates/menu"
	"net/http"
)

func AlbumContextProvider(context *context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResults
		albums, err := context.GetAlbums(int(offset), constants.MaxResults)

		if err != nil {
			return BaseTemplateContext
		}

		hasNextPage := len(*albums) > constants.MaxResults
		hasPrevPage := offset > 0
		limitedAlbums := (*albums)[:constants.MaxResults]

		return pongo2.Context{
			"subtitle":    "Albums",
			"albums":      limitedAlbums,
			"hasNextPage": hasNextPage,
			"hasPrevPage": hasPrevPage,
			"page":        page,
			"action": menu.Entry{
				Name: "Create",
				Url:  "/album/new",
			},
		}.Update(BaseTemplateContext)
	}
}
