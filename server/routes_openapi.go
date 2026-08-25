package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/api_handlers"
	"mahresources/server/openapi"
	"mahresources/server/template_presets"
)

// RegisterAPIRoutesWithOpenAPI registers all API routes with the OpenAPI registry.
// This function is called by the openapi-gen CLI tool to generate the spec.
func RegisterAPIRoutesWithOpenAPI(registry *openapi.Registry) {
	// Authentication
	registerAuthRoutes(registry)

	// Users & account management
	registerUserAccountRoutes(registry)

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

	// Template Partials
	registerTemplatePartialRoutes(registry)

	// Queries
	registerQueryRoutes(registry)

	// Search
	registerSearchRoutes(registry)

	// MRQL (Mahresources Query Language)
	registerMRQLRoutes(registry)

	// Logs
	registerLogRoutes(registry)

	// Downloads
	registerDownloadRoutes(registry)

	// Exports
	registerExportRoutes(registry)

	// Imports
	registerImportRoutes(registry)

	// Plugins
	registerPluginRoutes(registry)

	// Admin
	registerAdminRoutes(registry)

	// Timeline
	registerTimelineRoutes(registry)
}

// authLoginRequestType documents the JSON body accepted by POST /v1/auth/login.
// The body both /v1/downloads mutations take: a list of history row ids. Declared
// here rather than reused from api_handlers because that type is unexported —
// the shape is the contract, and it is one field.
var downloadIDListRequestType = reflect.TypeOf(struct {
	IDs []uint `json:"ids"`
}{})

var authLoginRequestType = reflect.TypeOf(struct {
	Username string `json:"username"`
	Password string `json:"password"`
}{})

func registerAuthRoutes(r *openapi.Registry) {
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/auth/login",
		OperationID:          "login",
		Summary:              "Authenticate and start a session",
		Description:          "Exchanges a username/password for a session cookie. Only meaningful when auth is enabled.",
		Tags:                 []string{"auth"},
		RequestType:          authLoginRequestType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/auth/logout",
		OperationID:          "logout",
		Summary:              "End the current session",
		Tags:                 []string{"auth"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/auth/me",
		OperationID:          "currentUser",
		Summary:              "Return the authenticated principal and its capabilities",
		Tags:                 []string{"auth"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
}

// Operation-specific user/account schemas keep create requirements, partial
// update semantics, and one-time credential responses distinct in OpenAPI.
type CreateUserRequest struct {
	Username     string `json:"username" openapi:"required"`
	DisplayName  string `json:"displayName"`
	Password     string `json:"password" openapi:"required"`
	Role         string `json:"role" openapi:"required"`
	ScopeGroupId *uint  `json:"scopeGroupId"`
	Disabled     bool   `json:"disabled"`
}

type UpdateUserRequest struct {
	ID           uint   `json:"id" openapi:"required"`
	Username     string `json:"username"`
	DisplayName  string `json:"displayName"`
	Password     string `json:"password"`
	Role         string `json:"role"`
	ScopeGroupId *uint  `json:"scopeGroupId"`
	Disabled     bool   `json:"disabled"`
}

type ChangePasswordRequest struct {
	CurrentPassword string `json:"currentPassword" openapi:"required"`
	NewPassword     string `json:"newPassword" openapi:"required"`
}

type CreateTokenRequest struct {
	Name      string `json:"name"`
	ExpiresIn string `json:"expiresIn"`
}

type OneTimeTokenResponse struct {
	Token  string `json:"token" openapi:"required"`
	ID     uint   `json:"id" openapi:"required"`
	Name   string `json:"name" openapi:"required"`
	Prefix string `json:"prefix" openapi:"required"`
}

type UserManagementResponse struct {
	ID           uint   `json:"ID" openapi:"required"`
	Username     string `json:"username" openapi:"required"`
	DisplayName  string `json:"displayName" openapi:"required"`
	Role         string `json:"role" openapi:"required"`
	ScopeGroupId *uint  `json:"scopeGroupId"`
	Disabled     bool   `json:"disabled" openapi:"required"`
}

type ApiTokenMetadataResponse struct {
	ID         uint       `json:"ID" openapi:"required"`
	CreatedAt  time.Time  `json:"CreatedAt" openapi:"required"`
	UpdatedAt  time.Time  `json:"UpdatedAt" openapi:"required"`
	UserID     uint       `json:"userId" openapi:"required"`
	Name       string     `json:"name" openapi:"required"`
	Prefix     string     `json:"prefix" openapi:"required"`
	ExpiresAt  *time.Time `json:"expiresAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	Disabled   bool       `json:"disabled" openapi:"required"`
}

type AccountOKResponse struct {
	OK bool `json:"ok" openapi:"required"`
}

// CropResourceResponse is the crop endpoint's JSON body. ID names the resource
// the crop was saved as, and is present only when AsNewResource was set — the
// in-place version path rewrites the resource that was already addressed and so
// answers with ok alone.
type CropResourceResponse struct {
	OK bool `json:"ok" openapi:"required"`
	ID uint `json:"id,omitempty"`
}

var (
	createUserRequestType     = reflect.TypeOf(CreateUserRequest{})
	updateUserRequestType     = reflect.TypeOf(UpdateUserRequest{})
	changePasswordRequestType = reflect.TypeOf(ChangePasswordRequest{})
	createTokenRequestType    = reflect.TypeOf(CreateTokenRequest{})
	oneTimeTokenResponseType  = reflect.TypeOf(OneTimeTokenResponse{})
	userManagementType        = reflect.TypeOf(UserManagementResponse{})
	apiTokenMetadataType      = reflect.TypeOf(ApiTokenMetadataResponse{})
	accountOKResponseType     = reflect.TypeOf(AccountOKResponse{})
	cropResponseType          = reflect.TypeOf(CropResourceResponse{})
)

func userManagementErrors(statuses ...int) map[int]string {
	descriptions := map[int]string{
		http.StatusBadRequest:   "Invalid input",
		http.StatusUnauthorized: "Authentication required or credentials invalid",
		http.StatusForbidden:    "Insufficient permissions",
		http.StatusNotFound:     "User or token not found",
		http.StatusConflict:     "Username, administrator invariant, scope dependency, or token limit conflict",
	}
	result := make(map[int]string, len(statuses))
	for _, status := range statuses {
		result[status] = descriptions[status]
	}
	return result
}

// userSettingRequestType is the PUT body for a per-user setting: an opaque JSON value.
var userSettingRequestType = reflect.TypeOf(struct {
	Value json.RawMessage `json:"value"`
}{})

func registerUserAccountRoutes(r *openapi.Registry) {
	// Array schemas normally use deliberately small partials to avoid expanding
	// model associations. These response DTOs have no associations, so their list
	// partials must include the complete stable JSON shape.
	r.SetPartialFields("UserManagementResponse", "ID", "Username", "DisplayName", "Role", "ScopeGroupId", "Disabled")
	r.SetPartialFields("ApiTokenMetadataResponse", "ID", "CreatedAt", "UpdatedAt", "UserID", "Name", "Prefix", "ExpiresAt", "LastUsedAt", "Disabled")

	r.Register(openapi.RouteInfo{
		Method: http.MethodGet, Path: "/v1/users", OperationID: "listUsers",
		Summary: "List user accounts (admin)", Tags: []string{"users"},
		ResponseType:         reflect.SliceOf(userManagementType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusUnauthorized, http.StatusForbidden),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/users", OperationID: "createUser",
		Summary: "Create a user account (admin)",
		Description: fmt.Sprintf("Creates an account. username, password, and role are required. Passwords require at least %d Unicode code points and at most %d UTF-8 bytes. Passwords are accepted but never returned.",
			auth.MinPasswordLength, auth.MaxPasswordBytes),
		Tags:                 []string{"users"},
		RequestType:          createUserRequestType,
		ResponseType:         userManagementType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodGet, Path: "/v1/user", OperationID: "getUser",
		Summary: "Get a user account (admin)", Tags: []string{"users"},
		IDQueryParam: "id", IDRequired: true,
		ResponseType:         userManagementType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/user", OperationID: "updateUser",
		Summary:              "Partially update a user account (admin)",
		Description:          "Only supplied fields are changed; id is required. Omitted fields are preserved. To clear an optional group scope, send JSON null, or send scopeGroupId empty or zero in form/query input. JSON password:null is rejected; an omitted or blank password preserves the credential. Null is rejected for every other non-scope field.",
		Tags:                 []string{"users"},
		RequestType:          updateUserRequestType,
		ResponseType:         userManagementType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/user/delete", OperationID: "deleteUser",
		Summary: "Delete a user account (admin)", Tags: []string{"users"},
		IDQueryParam: "id", IDRequired: true,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusConflict),
	})

	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/account/password", OperationID: "changeOwnPassword",
		Summary: "Change the authenticated user's password",
		Description: fmt.Sprintf("Requires the current password. The new password requires at least %d Unicode code points and at most %d UTF-8 bytes. On success, other browser sessions are invalidated while the current browser session remains active. Existing API tokens remain valid.",
			auth.MinPasswordLength, auth.MaxPasswordBytes),
		Tags:                 []string{"account"},
		RequestType:          changePasswordRequestType,
		ResponseType:         accountOKResponseType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodGet, Path: "/v1/account/tokens", OperationID: "listOwnTokens",
		Summary: "List the authenticated user's API tokens", Tags: []string{"account"},
		ResponseType:         reflect.SliceOf(apiTokenMetadataType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/account/tokens", OperationID: "createOwnToken",
		Summary:              "Mint a new API token (returned once)",
		Description:          "Returns the raw bearer token exactly once. Store it before dismissing the response; later token listings contain metadata only.",
		Tags:                 []string{"account"},
		RequestType:          createTokenRequestType,
		ResponseType:         oneTimeTokenResponseType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusConflict),
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/account/tokens/delete", OperationID: "revokeOwnToken",
		Summary: "Revoke one of the authenticated user's API tokens", Tags: []string{"account"},
		IDQueryParam: "id", IDRequired: true,
		ResponseType:         accountOKResponseType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses:       userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound),
	})

	r.Register(openapi.RouteInfo{
		Method: http.MethodGet, Path: "/v1/account/settings", OperationID: "listOwnSettings",
		Summary:              "List the authenticated user's UI settings",
		Description:          "Returns all per-user UI preferences (e.g. lightbox quick tags) as a key → JSON value object.",
		Tags:                 []string{"account"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPut, Path: "/v1/account/settings/{key}", OperationID: "setOwnSetting",
		Summary:     "Set one of the authenticated user's UI settings",
		Description: "Upserts a single per-user setting. The body is {\"value\": <json>}.",
		Tags:        []string{"account"},
		PathParams: []openapi.PathParam{
			{Name: "key", Type: "string", Description: "Setting key (max 128 chars)."},
		},
		RequestType:          userSettingRequestType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodDelete, Path: "/v1/account/settings/{key}", OperationID: "deleteOwnSetting",
		Summary:     "Delete one of the authenticated user's UI settings",
		Description: "Removes a single per-user setting. Deleting a missing key is a no-op.",
		Tags:        []string{"account"},
		PathParams: []openapi.PathParam{
			{Name: "key", Type: "string", Description: "Setting key (max 128 chars)."},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
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

	// BH-035: /admin/shares dashboard bulk-revoke endpoint. Accepts a form-
	// encoded body with repeated ids=<noteId> entries; non-numeric and
	// non-existent IDs are silently skipped. Responds 303 See Other
	// redirecting back to /admin/shares by default, or JSON summary if the
	// caller sends Accept: application/json.
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/admin/shares/bulk-revoke",
		OperationID:          "bulkRevokeShares",
		Summary:              "Revoke share tokens for a list of note IDs",
		Tags:                 []string{"notes", "admin"},
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
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "disposition", Type: "string", Description: "Set to \"inline\" to render the file in place rather than download it. Honoured for application/pdf only."},
		},
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

	r.Register(registerMetaEditRoute("note", "notes"))

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

	r.Register(registerMetaEditRoute("group", "groups"))
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
	cropQueryType := reflect.TypeOf(query_models.CropResourceQuery{})
	trimQueryType := reflect.TypeOf(query_models.TrimVideoQuery{})

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
		Path:                 "/v1/resource/suggestedTags",
		OperationID:          "getSuggestedTags",
		Summary:              "Get context-aware tag suggestions for a resource",
		Tags:                 []string{"resources"},
		IDQueryParam:         "id",
		IDRequired:           true,
		ResponseType:         reflect.TypeOf(api_handlers.SuggestedTagsResponse{}),
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
		Path:                "/v1/resource/preview",
		OperationID:         "setCustomResourceThumbnail",
		Summary:             "Replace a resource's thumbnail with a user-uploaded image",
		Tags:                []string{"resources"},
		IDQueryParam:        "ID",
		IDRequired:          true,
		HasFileUpload:       true,
		FileFieldName:       "thumbnail",
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeMultipart},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodDelete,
		Path:         "/v1/resource/preview",
		OperationID:  "clearResourceThumbnail",
		Summary:      "Clear stored thumbnails so the next request regenerates them from source",
		Tags:         []string{"resources"},
		IDQueryParam: "ID",
		IDRequired:   true,
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/resource/preview/clear",
		OperationID:  "clearResourceThumbnailViaPost",
		Summary:      "Clear stored thumbnails (POST alias of DELETE /v1/resource/preview)",
		Tags:         []string{"resources"},
		IDQueryParam: "ID",
		IDRequired:   true,
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

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/crop",
		OperationID:         "cropResource",
		Summary:             "Crop a resource image, saving the result as a new version or as a new resource",
		Tags:                []string{"resources"},
		RequestType:         cropQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:        cropResponseType,
		ErrorResponses: map[int]string{
			http.StatusBadRequest:           "Invalid crop rectangle",
			http.StatusNotFound:             "Resource not found",
			http.StatusConflict:             "A resource with identical content already exists (AsNewResource only)",
			http.StatusUnsupportedMediaType: "Resource is not a raster image the pipeline can decode",
			http.StatusInternalServerError:  "Internal server error",
		},
	})

	r.Register(openapi.RouteInfo{
		Method:              http.MethodPost,
		Path:                "/v1/resources/trim",
		OperationID:         "trimResourceVideo",
		Summary:             "Trim a video resource to a time range and save the result as a new version",
		Tags:                []string{"resources"},
		RequestType:         trimQueryType,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
	})

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resource/editName", "editResourceName", "Edit a resource's name", "resources").
		WithIDParam("id", true))

	r.Register(openapi.NewRoute(http.MethodPost, "/v1/resource/editDescription", "editResourceDescription", "Edit a resource's description", "resources").
		WithIDParam("id", true))

	r.Register(registerMetaEditRoute("resource", "resources"))
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
		Method:               http.MethodGet,
		Path:                 "/v1/tags/suggest",
		OperationID:          "suggestTags",
		Summary:              "Tag typeahead (lean list without the total-count header)",
		Tags:                 []string{"tags"},
		QueryType:            tagQueryType,
		ResponseType:         reflect.SliceOf(tagType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
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

func registerTemplatePartialRoutes(r *openapi.Registry) {
	templatePartialType := reflect.TypeOf(models.TemplatePartial{})
	templatePartialQueryType := reflect.TypeOf(query_models.TemplatePartialQuery{})
	templatePartialEditorType := reflect.TypeOf(query_models.TemplatePartialEditor{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/templatePartials",
		OperationID:          "listTemplatePartials",
		Summary:              "List template partials",
		Tags:                 []string{"templatePartials"},
		QueryType:            templatePartialQueryType,
		ResponseType:         reflect.SliceOf(templatePartialType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/templatePartial",
		OperationID:          "createOrUpdateTemplatePartial",
		Summary:              "Create or update a template partial (admin only)",
		Tags:                 []string{"templatePartials"},
		RequestType:          templatePartialEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         templatePartialType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	// /edit is a form-submission alias of the create/update route above.
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/templatePartial/edit",
		OperationID:          "updateTemplatePartial",
		Summary:              "Update a template partial (admin only; form alias)",
		Tags:                 []string{"templatePartials"},
		RequestType:          templatePartialEditorType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         templatePartialType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/templatePartial/delete",
		OperationID:  "deleteTemplatePartial",
		Summary:      "Delete a template partial (admin only)",
		Tags:         []string{"templatePartials"},
		IDQueryParam: "Id",
		IDRequired:   true,
	})

	// No editName route: a partial's Name must stay kebab-case, which the
	// generic name editor does not enforce. Names change via create/update.
	r.Register(openapi.NewRoute(http.MethodPost, "/v1/templatePartial/editDescription", "editTemplatePartialDescription", "Edit a template partial's description (admin only)", "templatePartials").
		WithIDParam("id", true))

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/templatePresets",
		OperationID:          "listTemplatePresets",
		Summary:              "List starter template presets (static bundles)",
		Tags:                 []string{"templatePartials"},
		ResponseType:         reflect.SliceOf(reflect.TypeOf(template_presets.Preset{})),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})
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
		Method:      http.MethodPost,
		Path:        "/v1/query/run",
		OperationID: "runQuery",
		Summary:     "Run a saved query",
		Description: "Returns the result set as a column list plus one array of values per row, " +
			"index-aligned with it. Columns appear in the order the query's SELECT names them, " +
			"repeated column names are preserved, and the column list is populated even when no " +
			"rows matched.",
		Tags:         []string{"queries"},
		IDQueryParam: "id",
		IDRequired:   false,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Description: "Name of the query to run (alternative to id)"},
		},
		ResponseType:         reflect.TypeOf(contracts.SQLResultSet{}),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
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

// registerMetaEditRoute returns a RouteInfo describing the /v1/{entity}/editMeta
// endpoint. This handler is shared by notes, groups, and resources — each takes
// an `id` query param and a form body with `path` (JSON pointer-ish) and
// `value` (raw JSON).
//
// BH-022: registered for notes, groups, and resources. The handler itself lives
// at server/api_handlers/meta_edit_handler.go and is wired up in server/routes.go.
func registerMetaEditRoute(entity, tag string) openapi.RouteInfo {
	return openapi.RouteInfo{
		Method:       http.MethodPost,
		Path:         "/v1/" + entity + "/editMeta",
		OperationID:  "edit" + capitalizeFirst(entity) + "Meta",
		Summary:      "Edit a " + entity + "'s meta JSON at a given path",
		Description:  "Performs a deep-merge-by-path update of the entity's Meta JSON column. `path` is a dotted key path; `value` is raw JSON that replaces the node at that path.",
		Tags:         []string{tag},
		IDQueryParam: "id",
		IDRequired:   true,
		RequestContentTypes: []openapi.ContentType{
			openapi.ContentTypeForm,
			openapi.ContentTypeJSON,
		},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "path", Type: "string", Required: true, Description: "Dotted path inside Meta to update (also accepted as a form/JSON field)"},
			{Name: "value", Type: "string", Required: true, Description: "Raw JSON value to write at the path (also accepted as a form/JSON field)"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	}
}

// capitalizeFirst uppercases the first byte of a non-empty ASCII string.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'a' && s[0] <= 'z' {
		return string(s[0]-'a'+'A') + s[1:]
	}
	return s
}

// registerMRQLRoutes documents the /v1/mrql subsystem. The handlers live in
// server/api_handlers/mrql_api_handlers.go. BH-022: registered MRQL routes
// that were previously invisible to the OpenAPI spec.
func registerMRQLRoutes(r *openapi.Registry) {
	// Request body shapes are defined inline — the real request types in
	// mrql_api_handlers.go are package-private. We describe them as JSON
	// objects using ExtraQueryParams-style documentation in the summary.
	mrqlTag := []string{"mrql"}

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql",
		OperationID: "executeMRQL",
		Summary:     "Execute an MRQL query",
		Description: `Parses, validates, translates, and executes a Mahresources Query Language (MRQL) query.

Request body fields:
  - query   (string, required) — the MRQL source
  - limit   (integer)          — items per bucket (grouped) or total items
  - buckets (integer)          — buckets per page (grouped mode only)
  - page    (integer)          — 1-based page number
  - offset  (integer)          — explicit cursor offset (takes precedence over page)

Query parameter:
  - render (0 or 1) — when 1, populates each result row's RenderedHTML using
    the entity's CustomMRQLResult template.

An aggregated GROUP BY response carries "columns": the result's column names in
the order the query wrote them (group-by fields first, then aggregates), matching
the header /v1/mrql/export?format=csv emits. "rows" is unchanged — each row is
still an object keyed by column name — so "columns" is additive. Read it rather
than the key order of a row: JSON objects carry no order, and a JavaScript
consumer enumerating a row's keys will not see the order the query was written
in.

A bucketed GROUP BY response carries "keyColumns" for the same reason: the
group-by key names in the order the query wrote them, matching the leading
columns of the CSV export's bucketed header. Each bucket's "key" object is
unchanged, and may carry entries "keyColumns" does not name — a bucket keyed on
a relation field also gets "<field>_id", so two same-named groups stay
distinguishable.`,
		Tags:                mrqlTag,
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "render", Type: "integer", Description: "Set to 1 to render CustomMRQLResult templates"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/validate",
		OperationID: "validateMRQL",
		Summary:     "Validate MRQL syntax",
		Description: `Parses and validates an MRQL query without executing it.

Request body fields:
  - query (string, required)

Response: {"valid": bool, "errors": [...]}`,
		Tags:                 mrqlTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/explain",
		OperationID: "explainMRQL",
		Summary:     "Explain the effective SQL and optimizer plans for MRQL",
		Description: `Parses, binds parameters, validates, and translates an MRQL query, then
returns the generated SQL statement(s) without executing the underlying query. The
explanation describes the Effective MRQL Query after default limits, safety bounds,
resolved SCOPE, and any RBAC-forced scope have been applied.

Request body fields (provide one of query / id / name):
  - query       (string) — inline MRQL source
  - id          (integer) — saved query id
  - name        (string) — saved query name
  - params      (object) — $name placeholder bindings
  - nativePlan  (boolean) — admin-only; ask the active database optimizer for a non-executing plan

Query parameters:
  - param.<name>=<value> — alternative to the params object (always strings)

The additive response includes queryFingerprint, executionShape, and statements.
Each statement retains label/sql/vars/interpolated and optionally includes nativePlan:
{"dialect","format","plan"}. SQLite uses EXPLAIN QUERY PLAN rows; PostgreSQL
uses EXPLAIN (FORMAT JSON). EXPLAIN ANALYZE is never used. All native statements
share one MRQL timeout and the request fails atomically if any plan fails.

Bucketed grouping reports key discovery plus data-dependent fan-out bounds rather
than executing discovery. A principal scoped to no groups receives statements: []
and zero-statement bounds. Non-admins retain generated-SQL explain access but receive
403 when nativePlan=true.`,
		Tags:                 mrqlTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/mrql/export",
		OperationID:          "exportMRQLGet",
		Summary:              "Export MRQL query results as CSV or JSON (GET)",
		Description:          "GET form of /v1/mrql/export: all inputs (query, id/name, format, param.<name>, pagination) are supplied as query parameters. See the POST form for the CSV/JSON shape details.",
		Tags:                 mrqlTag,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "query", Type: "string", Description: "Inline MRQL source (or use id/name)"},
			{Name: "id", Type: "integer", Description: "Saved query id"},
			{Name: "name", Type: "string", Description: "Saved query name"},
			{Name: "format", Type: "string", Description: "Export format: csv (default) or json"},
		},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/export",
		OperationID: "exportMRQL",
		Summary:     "Export MRQL query results as CSV or JSON",
		Description: `Executes a query (inline or saved) and streams the result as a file download.

Accepts the same inputs as /v1/mrql (query or id/name, params / param.<name>, limit,
page, buckets, offset) plus:
  - format (csv|json) — default csv

CSV shapes: aggregated → GROUP BY keys + aggregate aliases; flat → fixed scalar
columns per entity (meta as a JSON string); bucketed → bucket-key columns then the
flat item columns. CSV requires a single entity type; use format=json for
cross-entity results. When no explicit LIMIT is present the default is applied and
reported via the X-MRQL-Default-Limit-Applied response header.`,
		Tags:                 mrqlTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "format", Type: "string", Description: "Export format: csv (default) or json"},
		},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/complete",
		OperationID: "completeMRQL",
		Summary:     "Get MRQL autocomplete suggestions",
		Description: `Returns completion suggestions for the MRQL token at the given cursor offset.

Request body fields:
  - query  (string, required)
  - cursor (integer)          — byte offset in the query`,
		Tags:                 mrqlTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/generate",
		OperationID: "generateMRQL",
		Summary:     "Generate an MRQL draft from natural language",
		Description: `Generates, parses, validates, and lints an MRQL draft from a natural-language prompt.

Request body fields:
  - prompt (string, required) — user request to convert into MRQL

The endpoint does not execute MRQL. The server sends the prompt and syntax-only MRQL instructions to the configured DeepSeek provider.`,
		Tags:                 mrqlTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	// Saved MRQL queries
	savedMRQLType := reflect.TypeOf(models.SavedMRQLQuery{})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/mrql/saved",
		OperationID: "listSavedMRQLQueries",
		Summary:     "List or fetch saved MRQL queries",
		Description: "Without `id`: paginated list of saved queries (pass `all=1` for the full set). With `id`: returns a single saved query.",
		Tags:        mrqlTag,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "id", Type: "integer", Description: "Fetch a single saved query by ID"},
			{Name: "all", Type: "integer", Description: "Set to 1 to disable pagination"},
		},
		ResponseType:         reflect.SliceOf(savedMRQLType),
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		Paginated:            true,
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/saved",
		OperationID: "createSavedMRQLQuery",
		Summary:     "Create a saved MRQL query",
		Description: `Request body fields:
  - name        (string, required)
  - query       (string, required) — MRQL source
  - description (string)`,
		Tags:                 mrqlTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         savedMRQLType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPut,
		Path:                 "/v1/mrql/saved",
		OperationID:          "updateSavedMRQLQuery",
		Summary:              "Update a saved MRQL query",
		Tags:                 mrqlTag,
		IDQueryParam:         "id",
		IDRequired:           true,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseType:         savedMRQLType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/mrql/saved/delete",
		OperationID:          "deleteSavedMRQLQuery",
		Summary:              "Delete a saved MRQL query",
		Tags:                 mrqlTag,
		IDQueryParam:         "id",
		IDRequired:           true,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/mrql/saved/run",
		OperationID: "runSavedMRQLQuery",
		Summary:     "Execute a saved MRQL query by id or name",
		Description: "Runs a previously saved MRQL query. Accepts either `id` or `name` to identify the saved query, plus the same pagination params as /v1/mrql.",
		Tags:        mrqlTag,
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "id", Type: "integer", Description: "Saved query ID"},
			{Name: "name", Type: "string", Description: "Saved query name (fallback if id not found)"},
			{Name: "limit", Type: "integer"},
			{Name: "page", Type: "integer"},
			{Name: "buckets", Type: "integer"},
			{Name: "offset", Type: "integer"},
			{Name: "render", Type: "integer", Description: "Set to 1 to render CustomMRQLResult templates"},
		},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	// Shortcode editor tooling
	shortcodesTag := []string{"shortcodes"}

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/shortcodes/docs",
		OperationID: "listShortcodeDocs",
		Summary:     "List shortcode documentation",
		Description: `Returns a machine-readable catalogue of the built-in shortcodes
(meta, property, mrql, conditional, link, each, item) plus every shortcode
registered by an enabled plugin. Powers editor lint, autocomplete, and hover docs.

Each item: {name, syntax, description, isBlock ("no"|"optional"|"required"),
source ("builtin"|"plugin"), attrs: [{name, type, required, default, description,
enum, wildcard}], examples: [{title, code, notes}]}.`,
		Tags:                 shortcodesTag,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/shortcodes/lint",
		OperationID: "lintShortcodes",
		Summary:     "Lint shortcode markup",
		Description: `Parses shortcode markup and returns diagnostics without executing any
shortcode, plugin code, or database query (only the MRQL parser runs, to
syntax-check query attributes).

Request body fields:
  - content (string) — the template text to lint

Response: {"issues": [{"start", "end", "severity" ("error"|"warning"|"info"),
"message"}]}. Offsets are byte positions into content.`,
		Tags:                 shortcodesTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/shortcodes/deferred",
		OperationID: "renderDeferredShortcode",
		Summary:     "Render a deferred [lazy]/[details] block or a [reload] region",
		Description: `Renders a template body on demand — invoked by the frontend when a [lazy] block
scrolls into view, a [details] disclosure is first opened, or a [reload] button is
activated.

Request body fields:
  - token (string, required) — the sealed token emitted during the display-page
    render, either in a block's placeholder or on the region wrapper a slot
    containing a [reload] button is given. It authenticates the exact entity and
    template body the server itself produced; no client-supplied template text is
    trusted.

Response fields:
  - html (string) — the rendered body.
  - entity (object) — the entity the body was rendered against, for refreshing the
    page's Alpine "entity" scope. Custom templates bind to it directly, and those
    bindings resolve against the scope the page built at load, so the frontend
    installs this before hydrating the new markup. Notes are narrowed to the
    list-card projection (no shareToken, no blocks, hasShare set instead): a token
    identifies an entity but not the surface that minted it, so the endpoint
    cannot tell a card's token from a display page's and returns the shape that is
    safe for both. The frontend merges it into the existing scope rather than
    replacing it, so a detail page keeps the wider fields it started with, and it
    discards a response whose snapshot is older than the one already in scope —
    html and entity together, since the markup was expanded from that same
    snapshot.

The entity is reloaded through the request-scoped context, so an out-of-subtree id
returns 404. Semantically a read: gated at capRead (any authenticated principal)
and CSRF-exempt.`,
		Tags:                 shortcodesTag,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	previewDescription := `Renders a Custom* template slot against a real entity without saving, for a
live preview in the editor.

Request body fields:
  - entityId   (integer, required unless carrier) — the entity to render against
  - content    (string) — the template slot body to render. Markup for every
    slot but CustomCSS, whose body is a stylesheet.
  - css        (string) — CustomCSS to process alongside
  - slot       (string) — which Custom* field content came from. It decides how
    content is linted, because a stylesheet carries no <style> wrapper of its
    own to say it is one. "CustomCSS" lints content as a stylesheet, and a css
    that repeats it verbatim is then the same document, linted once. Any other
    slot makes content markup and css a second document, each linted its own
    way even when their text matches. Omitting slot lints content as markup,
    which is what this endpoint always did; if css repeats it verbatim the
    document is ambiguous, so the stylesheet reading is reported as well, minus
    the findings the two readings share.
  - categoryId (integer) — the category being edited (for a mismatch warning;
    required when carrier is set)
  - carrier    (boolean) — render against the category/type itself rather than a
    member entity (used for the CustomListHeader slot). entityId is then ignored
    and categoryId identifies the carrier.

Response: {"html", "css", "entity", "issues": [...], "cssIssues": [...]}.
"issues" holds the content buffer's lint findings plus any whole-request note
(the category mismatch, which indexes nothing); "cssIssues" holds the css
buffer's. They are two lists because a finding's start/end are byte offsets, and
one list of offsets into two buffers cannot be resolved by the reader. "entity" is
the carrier entity marshaled like the display pages' json filter, so the preview
frame can recreate the x-data="{ entity: ... }" Alpine scope those pages provide.
Executes MRQL and plugin shortcodes, so it is gated at the same capability as
saving the template (admin for category/resourceCategory, editor for noteType).
[mrql] result limits are capped for responsiveness.`

	for _, p := range []struct{ path, op, carrier string }{
		{"/v1/category/previewTemplate", "previewCategoryTemplate", "group"},
		{"/v1/resourceCategory/previewTemplate", "previewResourceCategoryTemplate", "resource"},
		{"/v1/noteType/previewTemplate", "previewNoteTypeTemplate", "note"},
	} {
		r.Register(openapi.RouteInfo{
			Method:               http.MethodPost,
			Path:                 p.path,
			OperationID:          p.op,
			Summary:              "Preview a custom template slot",
			Description:          previewDescription,
			Tags:                 shortcodesTag,
			RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
			ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		})
	}

	generateDescription := `Generate a custom template section from a natural-language prompt using the
configured DeepSeek provider. Requires DEEPSEEK_API_KEY (503 when unset).

Request body fields:
  - target     (string) — "slot" (default), "metaschema", or "bundle" (whole template)
  - slot       (string) — the slot field for target=slot. Accepts any Custom* field the
    carrier declares, which is CustomHeader, CustomDetailFooter, CustomSidebar,
    CustomSummary, CustomAvatar, CustomHoverCard, CustomListHeader, CustomListFooter,
    CustomMRQLResult and CustomCSS on all three carriers, plus CustomOwnEntities on a
    category and CustomPreview, CustomLightbox and CustomCell on a resource category.
    A slot the carrier does not declare is rejected with 400 "unknown template slot",
    so the accepted set differs per endpoint.
  - mode       (string) — "html", "css" or "json". Ignored wherever the target
    already settles the format, which is everywhere a public request can reach:
    a slot target takes CSS from slot == "CustomCSS" and HTML otherwise, and the
    metaschema and bundle targets have formats of their own. It is read only for
    a slot target naming no slot, which this endpoint rejects. Both the model's
    instructions and the lint of its reply follow the resolved format, so this
    field cannot put them at odds.
  - content    (string) — current slot content to refine/extend
  - metaSchema (string) — the (possibly unsaved) MetaSchema being authored
  - prompt     (string, required) — the natural-language request
  - categoryId (integer) — the carrier being edited (for schema + sample entity)
  - entityId   (integer) — a sample entity to ground on (else the first category member)

Response: {"target", "content" | "slots", "explanation", "valid", "issues"}.
"content" is set for slot/metaschema; "slots" (a field→markup map) for bundle.
"valid" is false with "issues" when the draft fails lint / JSON-Schema validation;
the draft is still returned for review (HTTP 200). Rate-limited per client IP.
Gated at the same capability as saving the template (admin for
category/resourceCategory, editor for noteType).`

	for _, p := range []struct{ path, op string }{
		{"/v1/category/generateTemplate", "generateCategoryTemplate"},
		{"/v1/resourceCategory/generateTemplate", "generateResourceCategoryTemplate"},
		{"/v1/noteType/generateTemplate", "generateNoteTypeTemplate"},
	} {
		r.Register(openapi.RouteInfo{
			Method:               http.MethodPost,
			Path:                 p.path,
			OperationID:          p.op,
			Summary:              "Generate a custom template section from natural language",
			Description:          generateDescription,
			Tags:                 shortcodesTag,
			RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
			ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		})
	}
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

	// Download history — the durable record of finished downloads, which outlives
	// the in-memory queue.
	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/downloads",
		OperationID: "listDownloadHistory",
		Summary:     "List finished downloads",
		Description: "Returns the persisted history of downloads that reached a terminal state, filtered by status, URL or name, and date. Admins see every user's downloads; every other principal sees only their own.",
		Tags:        []string{"downloads"},
		// Spelled out rather than derived from DownloadHistoryQuery: that struct
		// also carries the two owner fields, which are the visibility decision and
		// are overwritten from the principal after decoding. Deriving the params
		// would document them as inputs a caller can set, which they are not.
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "status", Type: "string", Description: "Filter by terminal status (completed, failed, cancelled). Repeat for several."},
			{Name: "url", Type: "string", Description: "Substring match over the URL and the download name."},
			{Name: "createdAfter", Type: "string", Description: "Only downloads submitted on or after this date (YYYY-MM-DD or RFC 3339)."},
			{Name: "createdBefore", Type: "string", Description: "Only downloads submitted at or before this instant. A bare YYYY-MM-DD is midnight at the start of that day, so it excludes the day itself; pass the next day, or an RFC 3339 instant, to include it."},
			{Name: "sortBy", Type: "string", Description: "Sort column, e.g. `created_at desc`. Repeat for several."},
		},
		Paginated:            true,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/downloads/retry",
		OperationID:          "retryDownloadHistory",
		Summary:              "Retry stored downloads",
		Description:          "Runs one or more failed or cancelled downloads again from their stored submission, whether or not the original job is still in the queue. Refused for a completed download, for one whose retry is still queued or running, and for a URL the queue is already fetching. Reports an outcome per id.",
		Tags:                 []string{"downloads"},
		RequestType:          downloadIDListRequestType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/downloads/delete",
		OperationID:          "deleteDownloadHistory",
		Summary:              "Delete stored downloads",
		Description:          "Removes one or more download history rows, and the matching queue entries. A download that is still running or paused is refused, as is one whose retry is still running; cancel it first. Reports an outcome per id.",
		Tags:                 []string{"downloads"},
		RequestType:          downloadIDListRequestType,
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON, openapi.ContentTypeForm},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
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

	// UI bug hunt finding 40: the jobs panel had no way to dismiss a finished job.
	// Clears completed/failed/cancelled jobs the caller may see; active and paused
	// jobs are kept.
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/jobs/clearCompleted",
		OperationID:          "jobsClearCompleted",
		Summary:              "Dismiss every finished job (completed, failed or cancelled)",
		Tags:                 []string{"jobs"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
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

func registerImportRoutes(r *openapi.Registry) {
	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/groups/import/parse",
		OperationID:          "parseGroupImport",
		Summary:              "Upload an import tar and start parsing",
		Description:          "Accepts a multipart file upload, stages the tar, and enqueues a parse job.",
		Tags:                 []string{"imports"},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeMultipart},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/imports/{jobId}/plan",
		OperationID: "getImportPlan",
		Summary:     "Get the parsed import plan",
		Description: "Returns the ImportPlan JSON for a completed parse job.",
		Tags:        []string{"imports"},
		PathParams: []openapi.PathParam{
			{Name: "jobId", Type: "string", Description: "The job ID returned by parseGroupImport"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodDelete,
		Path:        "/v1/imports/{jobId}",
		OperationID: "deleteGroupImport",
		Summary:     "Cancel and clean up an import",
		Description: "Cancels any running parse/apply job and deletes the staged tar and plan files.",
		Tags:        []string{"imports"},
		PathParams: []openapi.PathParam{
			{Name: "jobId", Type: "string", Description: "The job ID to clean up"},
		},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/imports/{jobId}/apply",
		OperationID: "applyGroupImport",
		Summary:     "Apply an import with user decisions",
		Description: "Validates decisions against the plan, consumes the plan file, and enqueues an apply job. Returns 409 if already applied.",
		Tags:        []string{"imports"},
		PathParams: []openapi.PathParam{
			{Name: "jobId", Type: "string", Description: "The parse job ID whose plan to apply"},
		},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/imports/{jobId}/result",
		OperationID: "getImportResult",
		Summary:     "Get the import apply result",
		Description: "Returns the ImportApplyResult JSON for a completed apply job.",
		Tags:        []string{"imports"},
		PathParams: []openapi.PathParam{
			{Name: "jobId", Type: "string", Description: "The apply job ID whose result to fetch"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
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

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/plugins/{pluginName}/display/render",
		OperationID: "renderPluginDisplay",
		Summary:     "Render a plugin display fragment",
		Description: "Invokes the named plugin's display renderer (e.g. for CustomHeader/CustomSidebar/CustomAvatar extensions). The request body is a plugin-specific JSON object; the response is HTML.",
		Tags:        []string{"plugins"},
		PathParams: []openapi.PathParam{
			{Name: "pluginName", Type: "string", Description: "The plugin name"},
		},
		RequestContentTypes:  []openapi.ContentType{openapi.ContentTypeJSON},
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
		Path:                 "/v1/plugin/displayTypes",
		OperationID:          "getPluginDisplayTypes",
		Summary:              "List plugin display types available for x-display",
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
		Path:        "/v1/plugin/scopedAccess",
		OperationID: "setPluginScopedAccess",
		Summary:     "Allow or refuse group-limited users access to a plugin",
		Description: "Records whether group-limited users and guests may reach this plugin's own surfaces: its pages, endpoints, shortcodes, injected slots and rendered blocks. Off by default. It does not widen what the plugin may do on their behalf — a confined caller's plugin database calls stay bound to that caller's own subtree and role.",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name"},
			{Name: "allowed", Type: "string", Required: false, Description: "1/true/on/yes to allow; anything else, including absence, refuses"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodGet,
		Path:        "/v1/plugin/schedules",
		OperationID: "listPluginSchedules",
		Summary:     "List a plugin's recurring schedules",
		Description: "The stored schedules for one plugin, as recorded when it was enabled. `registered` is false when the row exists but the plugin no longer declares that id -- a disabled plugin, a renamed schedule, or a removed mah.schedule call all look like this, and none of them run. `owned` is false when the operator who enabled the plugin has since been deleted, at which point the schedule has stopped rather than merely lost its attribution.",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name"},
		},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPost,
		Path:        "/v1/plugin/schedule/run",
		OperationID: "runPluginScheduleNow",
		Summary:     "Run a plugin schedule now",
		Description: "Starts one schedule outside its own cadence, and returns as soon as the run has started rather than when it has finished -- a handler may run for the full async job allowance, and progress is reported through the same job events every other plugin job uses. The run executes as the operator who enabled the plugin, exactly as a scheduled run does, and `nextDueAt` is not moved: this is an extra run, not a re-phasing of the cadence. Refused with 404 when no such schedule is stored, and with 409 when the plugin no longer declares it, when the row has no owner and has therefore stopped, or when a run is already in flight -- the last being `overlap = \"skip\"` doing what it promises.",
		Tags:        []string{"plugins"},
		ExtraQueryParams: []openapi.QueryParam{
			{Name: "name", Type: "string", Required: true, Description: "Plugin name"},
			{Name: "scheduleId", Type: "string", Required: true, Description: "The schedule's own id, as the plugin declared it"},
		},
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

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/admin/similarity/recompute",
		OperationID:          "recomputeSimilarities",
		Summary:              "Recompute image similarity pairs",
		Description:          "Submits a background job that deletes all v2-v2 similarity pairs and rebuilds them from stored perceptual hashes (no image decoding). Returns the job ID. 409 if a recompute is already running.",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodPost,
		Path:                 "/v1/admin/similarity/retry-failed",
		OperationID:          "retryFailedHashes",
		Summary:              "Retry failed image hashes",
		Description:          "Resets image_hashes rows marked failed so the background backfill worker re-attempts them. Returns the number of rows reset.",
		Tags:                 []string{"admin"},
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	settingViewType := reflect.TypeOf(application_context.SettingView{})
	settingViewListType := reflect.TypeOf([]application_context.SettingView{})

	r.Register(openapi.RouteInfo{
		Method:               http.MethodGet,
		Path:                 "/v1/admin/settings",
		OperationID:          "listRuntimeSettings",
		Summary:              "List runtime-editable settings",
		Description:          "Returns all runtime-editable settings with current value, boot default, override metadata, and bounds.",
		Tags:                 []string{"admin"},
		ResponseType:         settingViewListType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodPut,
		Path:        "/v1/admin/settings/{key}",
		OperationID: "setRuntimeSetting",
		Summary:     "Override a runtime setting",
		Description: "Persists an override for the named setting. Runtime changes take effect on the next use (no restart).",
		Tags:        []string{"admin"},
		PathParams: []openapi.PathParam{
			{Name: "key", Type: "string", Description: "Stable setting key; see GET /v1/admin/settings for the list."},
		},
		ResponseType:         settingViewType,
		ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
	})

	r.Register(openapi.RouteInfo{
		Method:      http.MethodDelete,
		Path:        "/v1/admin/settings/{key}",
		OperationID: "resetRuntimeSetting",
		Summary:     "Reset a runtime setting to boot default",
		Description: "Removes any persisted override for the named setting and reverts to the boot-time flag/env value.",
		Tags:        []string{"admin"},
		PathParams: []openapi.PathParam{
			{Name: "key", Type: "string", Description: "Stable setting key."},
		},
		ResponseType:         settingViewType,
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
