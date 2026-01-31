package template_context_providers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/template_handlers/template_entities"
)

// Log levels and actions for filter dropdowns
var logLevels = []SelectOption{
	{Link: "", Title: "All Levels", Active: true},
	{Link: models.LogLevelInfo, Title: "Info"},
	{Link: models.LogLevelWarning, Title: "Warning"},
	{Link: models.LogLevelError, Title: "Error"},
}

var logActions = []SelectOption{
	{Link: "", Title: "All Actions", Active: true},
	{Link: models.LogActionCreate, Title: "Create"},
	{Link: models.LogActionUpdate, Title: "Update"},
	{Link: models.LogActionDelete, Title: "Delete"},
	{Link: models.LogActionSystem, Title: "System"},
	{Link: models.LogActionProgress, Title: "Progress"},
}

var entityTypes = []SelectOption{
	{Link: "", Title: "All Types", Active: true},
	{Link: "tag", Title: "Tag"},
	{Link: "category", Title: "Category"},
	{Link: "note", Title: "Note"},
	{Link: "noteType", Title: "Note Type"},
	{Link: "resource", Title: "Resource"},
	{Link: "group", Title: "Group"},
	{Link: "query", Title: "Query"},
	{Link: "relation", Title: "Relation"},
	{Link: "relationType", Title: "Relation Type"},
}

// LogListContextProvider provides context for the log list template.
func LogListContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.LogEntryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		logs, err := context.GetLogEntries(int(offset), constants.MaxResultsPerPage, &query)
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		logsCount, err := context.GetLogEntriesCount(&query)
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), logsCount, constants.MaxResultsPerPage, int(page))
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		// Prepare filter options with active states
		levels := makeFilterOptions(logLevels, query.Level)
		actions := makeFilterOptions(logActions, query.Action)
		types := makeFilterOptions(entityTypes, query.EntityType)

		return pongo2.Context{
			"pageTitle":   "Logs",
			"logs":        logs,
			"pagination":  pagination,
			"logLevels":   levels,
			"logActions":  actions,
			"entityTypes": types,
			"queryValues": request.URL.Query(),
		}.Update(baseContext)
	}
}

// LogContextProvider provides context for displaying a single log entry.
func LogContextProvider(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := staticTemplateCtx(request)

		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		log, err := context.GetLogEntry(query.ID)
		if err != nil {
			fmt.Println(err)
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle": "Log Entry #" + strconv.Itoa(int(log.ID)),
			"log":       log,
		}.Update(baseContext)
	}
}

// makeFilterOptions creates a copy of the options with the correct active state.
func makeFilterOptions(options []SelectOption, currentValue string) []SelectOption {
	result := make([]SelectOption, len(options))
	for i, opt := range options {
		result[i] = SelectOption{
			Link:   opt.Link,
			Title:  opt.Title,
			Active: opt.Link == currentValue,
		}
	}
	// If no value is selected, mark the first option (All) as active
	if currentValue == "" && len(result) > 0 {
		result[0].Active = true
	}
	return result
}
