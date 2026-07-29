package template_context_providers

import (
	"net/http"
	"strconv"

	"github.com/flosch/pongo2/v4"
	"mahresources/constants"
	"mahresources/contracts"
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
	{Link: models.LogActionReset, Title: "Reset"},
	{Link: models.LogActionSystem, Title: "System"},
	{Link: models.LogActionProgress, Title: "Progress"},
	{Link: models.LogActionPlugin, Title: "Plugin"},
}

// entityTypes is the curated list, which exists to give nice labels and a
// sensible order. Entity types are written as free strings from many call
// sites, so this list drifts — makeFilterOptions therefore also surfaces
// whatever value is actually applied, even when it is not listed here.
var entityTypes = []SelectOption{
	{Link: "", Title: "All Types", Active: true},
	{Link: "tag", Title: "Tag"},
	{Link: "category", Title: "Category"},
	{Link: "note", Title: "Note"},
	{Link: "noteType", Title: "Note Type"},
	{Link: "note_type", Title: "Note Type (legacy key)"},
	{Link: "resource", Title: "Resource"},
	{Link: "resourceCategory", Title: "Resource Category"},
	{Link: "resource_category", Title: "Resource Category (legacy key)"},
	{Link: "resource_version", Title: "Resource Version"},
	{Link: "group", Title: "Group"},
	{Link: "series", Title: "Series"},
	{Link: "query", Title: "Query"},
	{Link: "relation", Title: "Relation"},
	{Link: "relationType", Title: "Relation Type"},
	{Link: "templatePartial", Title: "Template Partial"},
	{Link: "mrql_query", Title: "MRQL Query"},
	{Link: "runtime_setting", Title: "Runtime Setting"},
	{Link: "plugin", Title: "Plugin"},
	{Link: "migration", Title: "Migration"},
	{Link: "sql", Title: "Slow Query"},
}

// LogListContextProvider provides context for the log list template.
func LogListContextProvider(context contracts.LogEntryReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		page := http_utils.GetPageParameter(request)
		offset := (page - 1) * constants.MaxResultsPerPage
		var query query_models.LogEntryQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		logs, err := context.GetLogEntries(int(offset), constants.MaxResultsPerPage, &query)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		logsCount, err := context.GetLogEntriesCount(&query)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		pagination, err := template_entities.GeneratePagination(request.URL.String(), logsCount, constants.MaxResultsPerPage, int(page))
		if err != nil {
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
func LogContextProvider(context contracts.LogEntryReader) func(request *http.Request) pongo2.Context {
	return func(request *http.Request) pongo2.Context {
		var query query_models.EntityIdQuery
		err := decoder.Decode(&query, request.URL.Query())
		baseContext := StaticTemplateCtx(request)

		if err != nil {
			return addErrContext(err, baseContext)
		}

		log, err := context.GetLogEntry(query.ID)
		if err != nil {
			return addErrContext(err, baseContext)
		}

		return pongo2.Context{
			"pageTitle":  "Log Entry #" + strconv.Itoa(int(log.ID)),
			"log":        log,
			"hasDetails": len(log.Details) > 0 && string(log.Details) != "null",
		}.Update(baseContext)
	}
}

// makeFilterOptions creates a copy of the options with the correct active state.
func makeFilterOptions(options []SelectOption, currentValue string) []SelectOption {
	result := make([]SelectOption, len(options))
	matched := false
	for i, opt := range options {
		active := opt.Link == currentValue
		matched = matched || active
		result[i] = SelectOption{
			Link:   opt.Link,
			Title:  opt.Title,
			Active: active,
		}
	}
	// If no value is selected, mark the first option (All) as active
	if currentValue == "" && len(result) > 0 {
		result[0].Active = true
		return result
	}
	// An applied value that is not in the curated list must still appear and be
	// selected. Otherwise the control falls back to "All", and submitting the
	// filter form silently discards a filter the user can see working in the
	// table right next to it.
	if !matched && currentValue != "" {
		result = append(result, SelectOption{
			Link:   currentValue,
			Title:  currentValue,
			Active: true,
		})
	}
	return result
}
