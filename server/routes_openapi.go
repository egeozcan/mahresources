package server

import (
	"net/http"
	"reflect"

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
		Method:       http.MethodDelete,
		Path:         "/v1/resource/version",
		OperationID:  "deleteVersion",
		Summary:      "Delete a version",
		Tags:         []string{"versions"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "resourceId", Type: "integer", Required: true, Description: "Resource ID"},
			{Name: "versionId", Type: "integer", Required: true, Description: "Version ID"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/resource/version/delete",
		OperationID:  "deleteVersionPost",
		Summary:      "Delete a version (POST alternative)",
		Tags:         []string{"versions"},
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
}
