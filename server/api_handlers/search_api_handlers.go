package api_handlers

import (
	"encoding/json"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
	"net/http"
	"strings"
)

func GetGlobalSearchHandler(ctx interfaces.GlobalSearcher) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		searchQuery := &query_models.GlobalSearchQuery{
			Query: http_utils.GetQueryParameter(request, "q", ""),
			Limit: int(http_utils.GetIntQueryParameter(request, "limit", 20)),
		}

		typesParam := http_utils.GetQueryParameter(request, "types", "")
		if typesParam != "" {
			searchQuery.Types = strings.Split(typesParam, ",")
		}

		results, err := ctx.GlobalSearch(searchQuery)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(results)
	}
}
