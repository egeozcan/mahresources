package server

import (
	"net/http"
	"reflect"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/openapi"
)

// RegisterAPIRoutesWithOpenAPI registers all API routes with the OpenAPI registry.
// This function is called by the openapi-gen CLI tool to generate the spec.
func RegisterAPIRoutesWithOpenAPI(registry *openapi.Registry) {
	// Notes
	registerNoteRoutes(registry)

	// NoteTypes
	registerNoteTypeRoutes(registry)

	// Note Sharing
	registerNoteShareRoutes(registry)

	// Note Blocks
	registerBlockRoutes(registry)

	// Groups
	registerGroupRoutes(registry)

	// Relations
	registerRelationRoutes(registry)

	// Resources
	registerResourceRoutes(registry)

	// Resource Versions
	registerVersionRoutes(registry)

	// Series
	registerSeriesRoutes(registry)

	// Tags
	registerTagRoutes(registry)

	// Categories
	registerCategoryRoutes(registry)

	// Resource Categories
	registerResourceCategoryRoutes(registry)

	// Queries
	registerQueryRoutes(registry)

	// Search
	registerSearchRoutes(registry)

	// Logs
	registerLogRoutes(registry)

	// Downloads
	registerDownloadRoutes(registry)

	// Exports
	registerExportRoutes(registry)

	// Plugins
	registerPluginRoutes(registry)

	// Admin
	registerAdminRoutes(registry)

	// Timeline
	registerTimelineRoutes(registry)
}

func registerNoteShareRoutes(r *openapi.Registry) {
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/note/share",
		OperationID:          "shareNote",
		Summary:              "Share a note via public link",
		Tags:                 []string{"notes"},
		IDQueryParam:         "noteId",
		IDRequired:           true,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodDelete,
		Path:                 "/v1/note/share",
		OperationID:          "unshareNote",
		Summary:              "Remove public sharing for a note",
		Tags:                 []string{"notes"},
		IDQueryParam:         "noteId",
		IDRequired:           true,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

func registerBlockRoutes(r *openapi.Registry) {
	noteBlockType := reflect.TypeOf(models.NoteBlock{})
	noteBlockEditorType := reflect.TypeOf(query_models.NoteBlockEditor{})
	noteBlockReorderType := reflect.TypeOf(query_models.NoteBlockReorderEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/note/blocks",
		OperationID:          "getBlocksForNote",
		Summary:              "Get all blocks for a note",
		Tags:                 []string{"blocks"},
		IDQueryParam:         "noteId",
		IDRequired:           true,
		ResponseType:         reflect.SliceOf(noteBlockType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/note/block",
		OperationID:          "getBlock",
		Summary:              "Get a specific block",
		Tags:                 []string{"blocks"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         noteBlockType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/note/block/types",
		OperationID:          "getBlockTypes",
		Summary:              "Get all available block types",
		Tags:                 []string{"blocks"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/note/block",
		OperationID:          "createBlock",
		Summary:              "Create a new block",
		Tags:                 []string{"blocks"},
		RequestType:          noteBlockEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseType:         noteBlockType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPut,
		Path:                 "/v1/note/block",
		OperationID:          "updateBlockContent",
		Summary:              "Update a block's content",
		Tags:                 []string{"blocks"},
		IDQueryParam:         "id",
		IDRequired:           true,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseType:         noteBlockType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPatch,
		Path:                 "/v1/note/block/state",
		OperationID:          "updateBlockState",
		Summary:              "Update a block's state",
		Tags:                 []string{"blocks"},
		IDQueryParam:         "id",
		IDRequired:           true,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseType:         noteBlockType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodDelete,
		Path:         "/v1/note/block",
		OperationID:  "deleteBlock",
		Summary:      "Delete a block",
		Tags:         []string{"blocks"},
		IDQueryParam: "id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/note/block/delete",
		OperationID:  "deleteBlockPost",
		Summary:      "Delete a block (POST alternative)",
		Tags:         []string{"blocks"},
		IDQueryParam: "id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/note/blocks/reorder",
		OperationID:         "reorderBlocks",
		Summary:             "Reorder blocks within a note",
		Tags:                []string{"blocks"},
		RequestType:         noteBlockReorderType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/note/blocks/rebalance",
		OperationID:  "rebalanceBlocks",
		Summary:      "Rebalance block positions for a note",
		Tags:         []string{"blocks"},
		IDQueryParam: "noteId",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/note/block/table/query",
		OperationID:          "getTableBlockQueryData",
		Summary:              "Get query data for a table block",
		Tags:                 []string{"blocks"},
		IDQueryParam:         "blockId",
		IDRequired:           true,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/note/block/calendar/events",
		OperationID:  "getCalendarBlockEvents",
		Summary:      "Get events for a calendar block",
		Tags:         []string{"blocks"},
		IDQueryParam: "blockId",
		IDRequired:   true,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "start", Type: "string", Required: true, Description: "Start date (YYYY-MM-DD)"},
			{Name: "end", Type: "string", Required: true, Description: "End date (YYYY-MM-DD)"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

func registerVersionRoutes(r *openapi.Registry) {
	versionType := reflect.TypeOf(models.ResourceVersion{})
	versionRestoreType := reflect.TypeOf(query_models.VersionRestoreQuery{})
	versionCleanupType := reflect.TypeOf(query_models.VersionCleanupQuery{})
	bulkCleanupType := reflect.TypeOf(query_models.BulkVersionCleanupQuery{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resource/versions",
		OperationID:          "listVersions",
		Summary:              "List versions for a resource",
		Tags:                 []string{"versions"},
		IDQueryParam:         "resourceId",
		IDRequired:           true,
		ResponseType:         reflect.SliceOf(versionType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resource/version",
		OperationID:          "getVersion",
		Summary:              "Get a specific version",
		Tags:                 []string{"versions"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         versionType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource/versions",
		OperationID:          "uploadVersion",
		Summary:              "Upload a new version of a resource",
		Tags:                 []string{"versions"},
		HasFileUpload:        true,
		FileFieldName:        "file",
		ResponseType:         versionType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "resourceId", Type: "integer", Required: true, Description: "Resource ID"},
		},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource/version/restore",
		OperationID:          "restoreVersion",
		Summary:              "Restore a previous version",
		Tags:                 []string{"versions"},
		RequestType:          versionRestoreType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         versionType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodDelete,
		Path:        "/v1/resource/version",
		OperationID: "deleteVersion",
		Summary:     "Delete a version",
		Tags:        []string{"versions"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "resourceId", Type: "integer", Required: true, Description: "Resource ID"},
			{Name: "versionId", Type: "integer", Required: true, Description: "Version ID"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/resource/version/delete",
		OperationID: "deleteVersionPost",
		Summary:     "Delete a version (POST alternative)",
		Tags:        []string{"versions"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "resourceId", Type: "integer", Required: true, Description: "Resource ID"},
			{Name: "versionId", Type: "integer", Required: true, Description: "Version ID"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/resource/version/file",
		OperationID:  "getVersionFile",
		Summary:      "Download a version's file",
		Tags:         []string{"versions"},
		IDQueryParam: "versionId",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource/versions/cleanup",
		OperationID:          "cleanupVersions",
		Summary:              "Clean up old versions for a resource",
		Tags:                 []string{"versions"},
		RequestType:          versionCleanupType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resources/versions/cleanup",
		OperationID:          "bulkCleanupVersions",
		Summary:              "Bulk clean up old versions",
		Tags:                 []string{"versions"},
		RequestType:          bulkCleanupType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/resource/versions/compare",
		OperationID: "compareVersions",
		Summary:     "Compare two versions of a resource",
		Tags:        []string{"versions"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "resourceId", Type: "integer", Required: true, Description: "Resource ID"},
			{Name: "v1", Type: "integer", Required: true, Description: "First version ID"},
			{Name: "v2", Type: "integer", Required: true, Description: "Second version ID"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

func registerSeriesRoutes(r *openapi.Registry) {
	seriesType := reflect.TypeOf(models.Series{})
	seriesQueryType := reflect.TypeOf(query_models.SeriesQuery{})
	seriesEditorType := reflect.TypeOf(query_models.SeriesEditor{})
	seriesCreatorType := reflect.TypeOf(query_models.SeriesCreator{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/seriesList",
		OperationID:          "listSeries",
		Summary:              "List series",
		Tags:                 []string{"series"},
		QueryType:            seriesQueryType,
		ResponseType:         reflect.SliceOf(seriesType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/series/create",
		OperationID:          "createSeries",
		Summary:              "Create a new series",
		Tags:                 []string{"series"},
		RequestType:          seriesCreatorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         seriesType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/series",
		OperationID:          "getSeries",
		Summary:              "Get a specific series",
		Tags:                 []string{"series"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         seriesType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/series",
		OperationID:          "updateSeries",
		Summary:              "Update a series",
		Tags:                 []string{"series"},
		RequestType:          seriesEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         seriesType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/series/delete",
		OperationID:  "deleteSeries",
		Summary:      "Delete a series",
		Tags:         []string{"series"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/series/editName",
		OperationID: "editSeriesName",
		Summary:     "Edit a series name inline",
		Tags:        []string{"series"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "id", Type: "integer", Required: true, Description: "Series ID"},
		},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/resource/removeSeries",
		OperationID:  "removeResourceFromSeries",
		Summary:      "Remove a resource from its series",
		Tags:         []string{"series"},
		IDQueryParam: "id",
		IDRequired:   true,
	})
}

func registerNoteRoutes(r *openapi.Registry) {
	noteType := reflect.TypeOf(models.Note{})
	noteQueryType := reflect.TypeOf(query_models.NoteQuery{})
	noteEditorType := reflect.TypeOf(query_models.NoteEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/notes",
		OperationID:          "listNotes",
		Summary:              "List notes",
		Description:          "Get all notes, paginated, with optional filters.",
		Tags:                 []string{"notes"},
		QueryType:            noteQueryType,
		ResponseType:         reflect.SliceOf(noteType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/notes/meta/keys",
		OperationID:          "getNoteMetaKeys",
		Summary:              "Get all unique meta keys used in notes",
		Tags:                 []string{"notes"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/note",
		OperationID:          "getNote",
		Summary:              "Get a specific note",
		Tags:                 []string{"notes"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         noteType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/note",
		OperationID:          "createOrUpdateNote",
		Summary:              "Create or update a note",
		Tags:                 []string{"notes"},
		RequestType:          noteEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         noteType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/note/delete",
		OperationID:         "deleteNote",
		Summary:             "Delete a note",
		Tags:                []string{"notes"},
		IDQueryParam:        "Id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/note/editName", "editNoteName", "Edit a note's name", "notes").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/note/editDescription", "editNoteDescription", "Edit a note's description", "notes").
		WithIDParam("id", true))

	// Bulk note operations
	bulkQueryType := reflect.TypeOf(query_models.BulkQuery{})
	bulkEditQueryType := reflect.TypeOf(query_models.BulkEditQuery{})
	bulkEditMetaQueryType := reflect.TypeOf(query_models.BulkEditMetaQuery{})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/addTags",
		OperationID:         "bulkAddTagsToNotes",
		Summary:             "Bulk add tags to notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/removeTags",
		OperationID:         "bulkRemoveTagsFromNotes",
		Summary:             "Bulk remove tags from notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/addGroups",
		OperationID:         "bulkAddGroupsToNotes",
		Summary:             "Bulk add groups to notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/addMeta",
		OperationID:         "bulkAddMetaToNotes",
		Summary:             "Bulk add/merge meta to notes",
		Tags:                []string{"notes"},
		RequestType:         bulkEditMetaQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/notes/delete",
		OperationID:         "bulkDeleteNotes",
		Summary:             "Bulk delete notes",
		Tags:                []string{"notes"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})
}

func registerNoteTypeRoutes(r *openapi.Registry) {
	noteTypeType := reflect.TypeOf(models.NoteType{})
	noteTypeQueryType := reflect.TypeOf(query_models.NoteTypeQuery{})
	noteTypeEditorType := reflect.TypeOf(query_models.NoteTypeEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/note/noteTypes",
		OperationID:          "getNoteTypes",
		Summary:              "Get all note types",
		Tags:                 []string{"notes"},
		QueryType:            noteTypeQueryType,
		ResponseType:         reflect.SliceOf(noteTypeType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/note/noteType",
		OperationID:          "createNoteType",
		Summary:              "Create a new note type",
		Tags:                 []string{"notes"},
		RequestType:          noteTypeEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         noteTypeType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/note/noteType/edit",
		OperationID:          "editNoteType",
		Summary:              "Edit a note type",
		Tags:                 []string{"notes"},
		RequestType:          noteTypeEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         noteTypeType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/note/noteType/delete",
		OperationID:  "deleteNoteType",
		Summary:      "Delete a note type",
		Tags:         []string{"notes"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/noteType/editName", "editNoteTypeName", "Edit a note type's name", "notes").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/noteType/editDescription", "editNoteTypeDescription", "Edit a note type's description", "notes").
		WithIDParam("id", true))
}

func registerGroupRoutes(r *openapi.Registry) {
	groupType := reflect.TypeOf(models.Group{})
	groupQueryType := reflect.TypeOf(query_models.GroupQuery{})
	groupEditorType := reflect.TypeOf(query_models.GroupEditor{})
	bulkQueryType := reflect.TypeOf(query_models.BulkQuery{})
	bulkEditQueryType := reflect.TypeOf(query_models.BulkEditQuery{})
	bulkEditMetaQueryType := reflect.TypeOf(query_models.BulkEditMetaQuery{})
	mergeQueryType := reflect.TypeOf(query_models.MergeQuery{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/groups",
		OperationID:          "listGroups",
		Summary:              "List groups",
		Tags:                 []string{"groups"},
		QueryType:            groupQueryType,
		ResponseType:         reflect.SliceOf(groupType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/groups/meta/keys",
		OperationID:          "getGroupMetaKeys",
		Summary:              "Get all unique meta keys used in groups",
		Tags:                 []string{"groups"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/group",
		OperationID:          "getGroup",
		Summary:              "Get a specific group",
		Tags:                 []string{"groups"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         groupType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/group/parents",
		OperationID:          "getGroupParents",
		Summary:              "Get parents of a group",
		Tags:                 []string{"groups"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         reflect.SliceOf(groupType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/group/tree/children",
		OperationID: "getGroupTreeChildren",
		Summary:     "Get tree children of a group (or root groups if no parentId)",
		Tags:        []string{"groups"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "parentId", Type: "integer", Description: "Parent group ID (omit for root groups)"},
			{Name: "limit", Type: "integer", Description: "Maximum number of children to return (default: 50, max: 100)"},
		},
		ResponseType:         reflect.SliceOf(reflect.TypeOf(query_models.GroupTreeNode{})),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/group/clone",
		OperationID:          "cloneGroup",
		Summary:              "Clone a group",
		Tags:                 []string{"groups"},
		RequestType:          reflect.TypeOf(query_models.EntityIdQuery{}),
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         groupType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/group",
		OperationID:          "createOrUpdateGroup",
		Summary:              "Create or update a group",
		Tags:                 []string{"groups"},
		RequestType:          groupEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         groupType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/group/delete",
		OperationID:  "deleteGroup",
		Summary:      "Delete a group",
		Tags:         []string{"groups"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/groups/addTags",
		OperationID:         "bulkAddTagsToGroups",
		Summary:             "Bulk add tags to groups",
		Tags:                []string{"groups"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/groups/removeTags",
		OperationID:         "bulkRemoveTagsFromGroups",
		Summary:             "Bulk remove tags from groups",
		Tags:                []string{"groups"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/groups/addMeta",
		OperationID:         "bulkAddMetaToGroups",
		Summary:             "Bulk add/merge meta to groups",
		Tags:                []string{"groups"},
		RequestType:         bulkEditMetaQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/groups/delete",
		OperationID:         "bulkDeleteGroups",
		Summary:             "Bulk delete groups",
		Tags:                []string{"groups"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/groups/merge",
		OperationID:         "mergeGroups",
		Summary:             "Merge groups",
		Tags:                []string{"groups"},
		RequestType:         mergeQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/group/editName", "editGroupName", "Edit a group's name", "groups").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/group/editDescription", "editGroupDescription", "Edit a group's description", "groups").
		WithIDParam("id", true))
}

func registerRelationRoutes(r *openapi.Registry) {
	relationType := reflect.TypeOf(models.GroupRelation{})
	relationTypeType := reflect.TypeOf(models.GroupRelationType{})
	relationQueryType := reflect.TypeOf(query_models.GroupRelationshipQuery{})
	relationTypeQueryType := reflect.TypeOf(query_models.RelationshipTypeQuery{})
	relationTypeEditorType := reflect.TypeOf(query_models.RelationshipTypeEditorQuery{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/relation",
		OperationID:          "createOrUpdateRelation",
		Summary:              "Create or edit a group relation instance",
		Tags:                 []string{"relations"},
		RequestType:          relationQueryType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         relationType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/relation/delete",
		OperationID:  "deleteRelation",
		Summary:      "Delete a group relation instance",
		Tags:         []string{"relations"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/relationType",
		OperationID:          "createRelationType",
		Summary:              "Create a new relation type",
		Tags:                 []string{"relations"},
		RequestType:          relationTypeEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         relationTypeType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/relationType/delete",
		OperationID:  "deleteRelationType",
		Summary:      "Delete a relation type",
		Tags:         []string{"relations"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/relationType/edit",
		OperationID:          "editRelationType",
		Summary:              "Edit an existing relation type",
		Tags:                 []string{"relations"},
		RequestType:          relationTypeEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         relationTypeType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/relationTypes",
		OperationID:          "listRelationTypes",
		Summary:              "List relation types",
		Tags:                 []string{"relations"},
		QueryType:            relationTypeQueryType,
		ResponseType:         reflect.SliceOf(relationTypeType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/relation/editName", "editRelationName", "Edit a relation instance's name", "relations").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/relation/editDescription", "editRelationDescription", "Edit a relation instance's description", "relations").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/relationType/editName", "editRelationTypeName", "Edit a relation type's name", "relations").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/relationType/editDescription", "editRelationTypeDescription", "Edit a relation type's description", "relations").
		WithIDParam("id", true))
}

func registerResourceRoutes(r *openapi.Registry) {
	resourceType := reflect.TypeOf(models.Resource{})
	resourceQueryType := reflect.TypeOf(query_models.ResourceSearchQuery{})
	resourceEditorType := reflect.TypeOf(query_models.ResourceEditor{})
	resourceCreatorType := reflect.TypeOf(query_models.ResourceFromRemoteCreator{})
	resourceLocalCreatorType := reflect.TypeOf(query_models.ResourceFromLocalCreator{})
	bulkQueryType := reflect.TypeOf(query_models.BulkQuery{})
	bulkEditQueryType := reflect.TypeOf(query_models.BulkEditQuery{})
	bulkEditMetaQueryType := reflect.TypeOf(query_models.BulkEditMetaQuery{})
	mergeQueryType := reflect.TypeOf(query_models.MergeQuery{})
	rotateQueryType := reflect.TypeOf(query_models.RotateResourceQuery{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resource",
		OperationID:          "getResource",
		Summary:              "Get a specific resource",
		Tags:                 []string{"resources"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         resourceType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resources",
		OperationID:          "listResources",
		Summary:              "List resources",
		Tags:                 []string{"resources"},
		QueryType:            resourceQueryType,
		ResponseType:         reflect.SliceOf(resourceType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resources/meta/keys",
		OperationID:          "getResourceMetaKeys",
		Summary:              "Get all unique meta keys used in resources",
		Tags:                 []string{"resources"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource",
		OperationID:          "createResource",
		Summary:              "Create a resource (upload file or from URL)",
		Tags:                 []string{"resources"},
		HasFileUpload:        true,
		FileFieldName:        "resource",
		MultipleFiles:        true,
		RequestType:          resourceCreatorType,
		ResponseType:         resourceType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource/local",
		OperationID:          "addLocalResource",
		Summary:              "Add a resource from a local server path",
		Tags:                 []string{"resources"},
		RequestType:          resourceLocalCreatorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         resourceType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource/remote",
		OperationID:          "addRemoteResource",
		Summary:              "Add a resource from a remote URL",
		Tags:                 []string{"resources"},
		RequestType:          resourceCreatorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         resourceType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/resource/delete",
		OperationID:  "deleteResource",
		Summary:      "Delete a resource",
		Tags:         []string{"resources"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resource/edit",
		OperationID:          "editResource",
		Summary:              "Edit a resource",
		Tags:                 []string{"resources"},
		RequestType:          resourceEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         resourceType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/resource/view",
		OperationID:  "viewResource",
		Summary:      "View a resource's content",
		Tags:         []string{"resources"},
		IDQueryParam: "id",
		IDRequired:   false,
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodGet,
		Path:         "/v1/resource/preview",
		OperationID:  "getResourcePreview",
		Summary:      "Get a preview image for a resource",
		Tags:         []string{"resources"},
		IDQueryParam: "ID",
		IDRequired:   true,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "Width", Type: "integer"},
			{Name: "Height", Type: "integer"},
		},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resource/recalculateDimensions",
		OperationID:         "bulkRecalculateDimensions",
		Summary:             "Recalculate dimensions for resources (bulk)",
		Tags:                []string{"resources"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/setDimensions",
		OperationID:         "setResourceDimensions",
		Summary:             "Set dimensions for a resource",
		Tags:                []string{"resources"},
		RequestType:         resourceEditorType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/addTags",
		OperationID:         "bulkAddTagsToResources",
		Summary:             "Bulk add tags to resources",
		Tags:                []string{"resources"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/addGroups",
		OperationID:         "bulkAddGroupsToResources",
		Summary:             "Bulk add groups to resources",
		Tags:                []string{"resources"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/removeTags",
		OperationID:         "bulkRemoveTagsFromResources",
		Summary:             "Bulk remove tags from resources",
		Tags:                []string{"resources"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/replaceTags",
		OperationID:         "bulkReplaceTagsOfResources",
		Summary:             "Bulk replace tags of resources",
		Tags:                []string{"resources"},
		RequestType:         bulkEditQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/addMeta",
		OperationID:         "bulkAddMetaToResources",
		Summary:             "Bulk add/merge meta to resources",
		Tags:                []string{"resources"},
		RequestType:         bulkEditMetaQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/delete",
		OperationID:         "bulkDeleteResources",
		Summary:             "Bulk delete resources",
		Tags:                []string{"resources"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/merge",
		OperationID:         "mergeResources",
		Summary:             "Merge resources",
		Tags:                []string{"resources"},
		RequestType:         mergeQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/rotate",
		OperationID:         "rotateResource",
		Summary:             "Rotate a resource image",
		Tags:                []string{"resources"},
		RequestType:         rotateQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resource/editName", "editResourceName", "Edit a resource's name", "resources").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resource/editDescription", "editResourceDescription", "Edit a resource's description", "resources").
		WithIDParam("id", true))
}

func registerTagRoutes(r *openapi.Registry) {
	tagType := reflect.TypeOf(models.Tag{})
	tagQueryType := reflect.TypeOf(query_models.TagQuery{})
	tagCreatorType := reflect.TypeOf(query_models.TagCreator{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/tags",
		OperationID:          "listTags",
		Summary:              "List tags",
		Tags:                 []string{"tags"},
		QueryType:            tagQueryType,
		ResponseType:         reflect.SliceOf(tagType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/tag",
		OperationID:          "createOrUpdateTag",
		Summary:              "Create or update a tag",
		Tags:                 []string{"tags"},
		RequestType:          tagCreatorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         tagType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/tag/delete",
		OperationID:  "deleteTag",
		Summary:      "Delete a tag",
		Tags:         []string{"tags"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/tag/editName", "editTagName", "Edit a tag's name", "tags").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/tag/editDescription", "editTagDescription", "Edit a tag's description", "tags").
		WithIDParam("id", true))

	mergeQueryType := reflect.TypeOf(query_models.MergeQuery{})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/tags/merge",
		OperationID:         "mergeTags",
		Summary:             "Merge tags",
		Tags:                []string{"tags"},
		RequestType:         mergeQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	bulkQueryType := reflect.TypeOf(query_models.BulkQuery{})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/tags/delete",
		OperationID:         "bulkDeleteTags",
		Summary:             "Bulk delete tags",
		Tags:                []string{"tags"},
		RequestType:         bulkQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})
}

func registerCategoryRoutes(r *openapi.Registry) {
	categoryType := reflect.TypeOf(models.Category{})
	categoryQueryType := reflect.TypeOf(query_models.CategoryQuery{})
	categoryEditorType := reflect.TypeOf(query_models.CategoryEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/categories",
		OperationID:          "listCategories",
		Summary:              "List categories",
		Tags:                 []string{"categories"},
		QueryType:            categoryQueryType,
		ResponseType:         reflect.SliceOf(categoryType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/category",
		OperationID:          "createOrUpdateCategory",
		Summary:              "Create or update a category",
		Tags:                 []string{"categories"},
		RequestType:          categoryEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         categoryType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/category/delete",
		OperationID:  "deleteCategory",
		Summary:      "Delete a category",
		Tags:         []string{"categories"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/category/editName", "editCategoryName", "Edit a category's name", "categories").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/category/editDescription", "editCategoryDescription", "Edit a category's description", "categories").
		WithIDParam("id", true))
}

func registerResourceCategoryRoutes(r *openapi.Registry) {
	resourceCategoryType := reflect.TypeOf(models.ResourceCategory{})
	resourceCategoryQueryType := reflect.TypeOf(query_models.ResourceCategoryQuery{})
	resourceCategoryEditorType := reflect.TypeOf(query_models.ResourceCategoryEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resourceCategories",
		OperationID:          "listResourceCategories",
		Summary:              "List resource categories",
		Tags:                 []string{"resourceCategories"},
		QueryType:            resourceCategoryQueryType,
		ResponseType:         reflect.SliceOf(resourceCategoryType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/resourceCategory",
		OperationID:          "createOrUpdateResourceCategory",
		Summary:              "Create or update a resource category",
		Tags:                 []string{"resourceCategories"},
		RequestType:          resourceCategoryEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         resourceCategoryType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/resourceCategory/delete",
		OperationID:  "deleteResourceCategory",
		Summary:      "Delete a resource category",
		Tags:         []string{"resourceCategories"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resourceCategory/editName", "editResourceCategoryName", "Edit a resource category's name", "resourceCategories").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resourceCategory/editDescription", "editResourceCategoryDescription", "Edit a resource category's description", "resourceCategories").
		WithIDParam("id", true))
}

func registerQueryRoutes(r *openapi.Registry) {
	queryType := reflect.TypeOf(models.Query{})
	queryQueryType := reflect.TypeOf(query_models.QueryQuery{})
	queryEditorType := reflect.TypeOf(query_models.QueryEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/queries",
		OperationID:          "listQueries",
		Summary:              "List queries",
		Tags:                 []string{"queries"},
		QueryType:            queryQueryType,
		ResponseType:         reflect.SliceOf(queryType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/query",
		OperationID:          "getQuery",
		Summary:              "Get a specific query",
		Tags:                 []string{"queries"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         queryType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/query",
		OperationID:          "createOrUpdateQuery",
		Summary:              "Create or update a query",
		Tags:                 []string{"queries"},
		RequestType:          queryEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         queryType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/query/delete",
		OperationID:  "deleteQuery",
		Summary:      "Delete a query",
		Tags:         []string{"queries"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/query/run",
		OperationID:  "runQuery",
		Summary:      "Run a saved query",
		Tags:         []string{"queries"},
		IDQueryParam: "id",
		IDRequired:   false,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Description: "Name of the query to run (alternative to id)"},
		},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/query/schema",
		OperationID:          "getDatabaseSchema",
		Summary:              "Get database table and column names",
		Description:          "Returns a map of table names to their column names for autocompletion.",
		Tags:                 []string{"queries"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/query/editName", "editQueryName", "Edit a query's name", "queries").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/query/editDescription", "editQueryDescription", "Edit a query's description", "queries").
		WithIDParam("id", true))
}

func registerSearchRoutes(r *openapi.Registry) {
	searchQueryType := reflect.TypeOf(query_models.GlobalSearchQuery{})
	searchResponseType := reflect.TypeOf(query_models.GlobalSearchResponse{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/search",
		OperationID:          "globalSearch",
		Summary:              "Global search across all entities",
		Tags:                 []string{"search"},
		QueryType:            searchQueryType,
		ResponseType:         searchResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})
}

func registerLogRoutes(r *openapi.Registry) {
	logEntryType := reflect.TypeOf(models.LogEntry{})
	logQueryType := reflect.TypeOf(query_models.LogEntryQuery{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/logs",
		OperationID:          "listLogs",
		Summary:              "List log entries",
		Description:          "Get all log entries, paginated, with optional filters.",
		Tags:                 []string{"logs"},
		QueryType:            logQueryType,
		ResponseType:         reflect.SliceOf(logEntryType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/log",
		OperationID:          "getLog",
		Summary:              "Get a specific log entry",
		Tags:                 []string{"logs"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         logEntryType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/logs/entity",
		OperationID:          "getEntityHistory",
		Summary:              "Get history of a specific entity",
		Description:          "Get all log entries for a specific entity type and ID.",
		Tags:                 []string{"logs"},
		ResponseType:         reflect.SliceOf(logEntryType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "entityType", Type: "string", Required: true, Description: "Type of entity (e.g., tag, note, resource)"},
			{Name: "entityId", Type: "integer", Required: true, Description: "ID of the entity"},
		},
	})
}

func registerDownloadRoutes(r *openapi.Registry) {
	remoteCreatorType := reflect.TypeOf(query_models.ResourceFromRemoteCreator{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/download/submit",
		OperationID:          "submitDownload",
		Summary:              "Submit a URL for background download",
		Description:          "Adds one or more URLs to the download queue. Multiple URLs can be submitted by separating them with newlines.",
		Tags:                 []string{"downloads"},
		RequestType:          remoteCreatorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/download/queue",
		OperationID:          "getDownloadQueue",
		Summary:              "Get all download jobs",
		Description:          "Returns all download jobs in the queue, including pending, active, and completed jobs.",
		Tags:                 []string{"downloads"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/download/cancel",
		OperationID:         "cancelDownload",
		Summary:             "Cancel an active download",
		Description:         "Cancels a pending or in-progress download job.",
		Tags:                []string{"downloads"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/download/pause",
		OperationID:         "pauseDownload",
		Summary:             "Pause a download",
		Description:         "Pauses a pending or downloading job. The job can be resumed later.",
		Tags:                []string{"downloads"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/download/resume",
		OperationID:         "resumeDownload",
		Summary:             "Resume a paused download",
		Description:         "Resumes a paused download job. The download will restart from the beginning.",
		Tags:                []string{"downloads"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/download/retry",
		OperationID:         "retryDownload",
		Summary:             "Retry a failed or cancelled download",
		Description:         "Retries a download that previously failed or was cancelled.",
		Tags:                []string{"downloads"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/download/events",
		OperationID: "downloadEvents",
		Summary:     "Server-Sent Events stream for download updates",
		Description: "Returns a Server-Sent Events stream with real-time updates about download job status changes.",
		Tags:        []string{"downloads"},
	})

	// Jobs routes (canonical paths — aliases for download routes above, plus action routes)
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/jobs/download/submit",
		OperationID:          "jobsSubmitDownload",
		Summary:              "Submit a URL for background download (canonical path)",
		Tags:                 []string{"jobs"},
		RequestType:          remoteCreatorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/jobs/queue",
		OperationID:          "jobsGetQueue",
		Summary:              "Get all jobs in the queue (canonical path)",
		Tags:                 []string{"jobs"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/jobs/cancel",
		OperationID:         "jobsCancel",
		Summary:             "Cancel a job (canonical path)",
		Tags:                []string{"jobs"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/jobs/pause",
		OperationID:         "jobsPause",
		Summary:             "Pause a job (canonical path)",
		Tags:                []string{"jobs"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/jobs/resume",
		OperationID:         "jobsResume",
		Summary:             "Resume a paused job (canonical path)",
		Tags:                []string{"jobs"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/jobs/retry",
		OperationID:         "jobsRetry",
		Summary:             "Retry a failed job (canonical path)",
		Tags:                []string{"jobs"},
		IDQueryParam:        "id",
		IDRequired:          true,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/jobs/events",
		OperationID: "jobsEvents",
		Summary:     "Server-Sent Events stream for job updates (canonical path)",
		Tags:        []string{"jobs"},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/jobs/get",
		OperationID:          "getJob",
		Summary:              "Get a single background job by ID",
		Description:          "Returns the current status of a job. Used by the CLI client's polling loop.",
		Tags:                 []string{"jobs"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	// Plugin action routes via jobs
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/jobs/action/run",
		OperationID:          "runPluginAction",
		Summary:              "Run a plugin action as a background job",
		Tags:                 []string{"jobs", "plugins"},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/jobs/action/job",
		OperationID: "getActionJob",
		Summary:     "Get the status of a plugin action job",
		Tags:        []string{"jobs", "plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "jobId", Type: "string", Required: true, Description: "Job ID"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

func registerExportRoutes(r *openapi.Registry) {
	exportReqType := reflect.TypeOf(application_context.ExportRequest{})
	exportEstType := reflect.TypeOf(application_context.ExportEstimate{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/groups/export/estimate",
		OperationID:          "estimateGroupExport",
		Summary:              "Estimate the size and shape of a proposed group export",
		Description:          "Walks the requested scope without writing a tar; returns counts, unique blob count, dangling reference summary.",
		Tags:                 []string{"exports"},
		RequestType:          exportReqType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseType:         exportEstType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/groups/export",
		OperationID:          "submitGroupExport",
		Summary:              "Enqueue a group export job",
		Description:          "Schedules a background job that walks the requested scope and writes a tar to the export staging directory. Returns the job ID; poll /v1/jobs/events for progress and download via /v1/exports/{jobId}/download when status=completed.",
		Tags:                 []string{"exports"},
		RequestType:          exportReqType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/exports/{jobId}/download",
		OperationID: "downloadGroupExport",
		Summary:     "Download a completed group export tar",
		Description: "Streams the tar file produced by a completed group-export job. Returns 409 if the job isn't completed yet, 410 if the file has expired off disk, 404 if no such job.",
		Tags:        []string{"exports"},
		PathParams: []openapi.PathParam{
			{Name: "jobId", Type: "string", Description: "The job ID returned by submitGroupExport"},
		},
	})
}

func registerPluginRoutes(r *openapi.Registry) {
	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/plugins/{pluginName}/block/render",
		OperationID: "renderPluginBlock",
		Summary:     "Render a plugin block",
		Description: "Renders a block using the plugin's block renderer, returning HTML content for the specified mode.",
		Tags:        []string{"plugins", "blocks"},
		PathParams: []openapi.PathParam{
			{Name: "pluginName", Type: "string", Description: "The plugin name"},
		},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "blockId", Type: "integer", Required: true, Description: "The ID of the block to render"},
			{Name: "mode", Type: "string", Required: true, Description: "Render mode: 'view' or 'edit'"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeHTML},
	})

	// Plugin management routes
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/plugin/actions",
		OperationID:          "getPluginActions",
		Summary:              "Get available plugin actions",
		Tags:                 []string{"plugins"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/plugins/manage",
		OperationID:          "getPluginsManage",
		Summary:              "Get plugin management information",
		Tags:                 []string{"plugins"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/plugin/enable",
		OperationID: "enablePlugin",
		Summary:     "Enable a plugin",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name to enable"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/plugin/disable",
		OperationID: "disablePlugin",
		Summary:     "Disable a plugin",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name to disable"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/plugin/settings",
		OperationID: "updatePluginSettings",
		Summary:     "Update plugin settings",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name"},
		},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/plugin/purge-data",
		OperationID: "purgePluginData",
		Summary:     "Purge all data for a plugin",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name to purge data for"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

func registerAdminRoutes(r *openapi.Registry) {
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/server-stats",
		OperationID:          "getServerStats",
		Summary:              "Get server statistics",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/data-stats",
		OperationID:          "getDataStats",
		Summary:              "Get data statistics",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/data-stats/expensive",
		OperationID:          "getExpensiveStats",
		Summary:              "Get expensive data statistics",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

func registerTimelineRoutes(r *openapi.Registry) {
	timelineResponseType := reflect.TypeOf(models.TimelineResponse{})

	timelineQueryParams := []openapi.QueryParam{
		{Name: "granularity", Type: "string", Description: "Time granularity: yearly, monthly, or weekly (default: monthly)"},
		{Name: "anchor", Type: "string", Description: "Anchor date in YYYY-MM-DD format (default: today)"},
		{Name: "columns", Type: "integer", Description: "Number of time buckets to return (default: 15, max: 60)"},
	}

	resourceQueryType := reflect.TypeOf(query_models.ResourceSearchQuery{})
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/resources/timeline",
		OperationID:          "getResourceTimeline",
		Summary:              "Get resource creation/update timeline",
		Description:          "Returns bucketed counts of created and updated resources over time, with optional filters.",
		Tags:                 []string{"resources", "timeline"},
		QueryType:            resourceQueryType,
		ExtraQueryParams:     timelineQueryParams,
		ResponseType:         timelineResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	noteQueryType := reflect.TypeOf(query_models.NoteQuery{})
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/notes/timeline",
		OperationID:          "getNoteTimeline",
		Summary:              "Get note creation/update timeline",
		Description:          "Returns bucketed counts of created and updated notes over time, with optional filters.",
		Tags:                 []string{"notes", "timeline"},
		QueryType:            noteQueryType,
		ExtraQueryParams:     timelineQueryParams,
		ResponseType:         timelineResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	groupQueryType := reflect.TypeOf(query_models.GroupQuery{})
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/groups/timeline",
		OperationID:          "getGroupTimeline",
		Summary:              "Get group creation/update timeline",
		Description:          "Returns bucketed counts of created and updated groups over time, with optional filters.",
		Tags:                 []string{"groups", "timeline"},
		QueryType:            groupQueryType,
		ExtraQueryParams:     timelineQueryParams,
		ResponseType:         timelineResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	tagQueryType := reflect.TypeOf(query_models.TagQuery{})
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/tags/timeline",
		OperationID:          "getTagTimeline",
		Summary:              "Get tag creation/update timeline",
		Description:          "Returns bucketed counts of created and updated tags over time, with optional filters.",
		Tags:                 []string{"tags", "timeline"},
		QueryType:            tagQueryType,
		ExtraQueryParams:     timelineQueryParams,
		ResponseType:         timelineResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	categoryQueryType := reflect.TypeOf(query_models.CategoryQuery{})
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/categories/timeline",
		OperationID:          "getCategoryTimeline",
		Summary:              "Get category creation/update timeline",
		Description:          "Returns bucketed counts of created and updated categories over time, with optional filters.",
		Tags:                 []string{"categories", "timeline"},
		QueryType:            categoryQueryType,
		ExtraQueryParams:     timelineQueryParams,
		ResponseType:         timelineResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	queryQueryType := reflect.TypeOf(query_models.QueryQuery{})
	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/queries/timeline",
		OperationID:          "getQueryTimeline",
		Summary:              "Get query creation/update timeline",
		Description:          "Returns bucketed counts of created and updated queries over time, with optional filters.",
		Tags:                 []string{"queries", "timeline"},
		QueryType:            queryQueryType,
		ExtraQueryParams:     timelineQueryParams,
		ResponseType:         timelineResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}
