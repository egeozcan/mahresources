package server

import (
	"net/http"
	"reflect"

	"mahresources/models"
	"mahresources/server/openapi"
)

func registerSavedSearchRoutes(r *openapi.Registry) {
	r.SetPartialFields("SavedSearch", "ID", "Name", "Family", "URL", "Layout", "CreatedAt", "UpdatedAt")
	errors := userManagementErrors(http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound)
	r.Register(openapi.RouteInfo{
		Method: http.MethodGet, Path: "/v1/account/saved-searches", OperationID: "listSavedSearches",
		Summary: "List personal saved searches", Tags: []string{"account"},
		ExtraQueryParams: []openapi.QueryParam{{Name: "family", Type: "string", Description: "Optional list family, e.g. resources or notes; includes all its layouts."}},
		ResponseType:     reflect.TypeOf([]models.SavedSearch{}), ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses: errors,
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPost, Path: "/v1/account/saved-searches", OperationID: "createSavedSearch",
		Summary: "Save a personal list search", Tags: []string{"account"},
		SuccessStatus: http.StatusCreated,
		Description:   "Returns 201. Saves an applied relative list URL; removes pagination and transient parameters. Name is trimmed and must be 1–200 UTF-8 bytes. URL is limited to 64 KiB. Duplicate names are allowed. Ownership comes from the current account, or root when authentication is disabled.",
		RequestType: reflect.TypeOf(struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		}{}),
		ResponseType:        reflect.TypeOf(models.SavedSearch{}),
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON}, ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses: errors,
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodPatch, Path: "/v1/account/saved-searches/{id}", OperationID: "updateSavedSearch",
		Summary: "Rename or replace a personal saved search", Tags: []string{"account"},
		Description: "Supply name, url, or both. A replacement URL must belong to the same list family. Another user's record returns 404.",
		PathParams:  []openapi.PathParam{{Name: "id", Type: "integer", Description: "Saved search ID"}},
		RequestType: reflect.TypeOf(struct {
			Name *string `json:"name,omitempty"`
			URL  *string `json:"url,omitempty"`
		}{}),
		ResponseType:        reflect.TypeOf(models.SavedSearch{}),
		RequestContentTypes: []openapi.ContentType{openapi.ContentTypeJSON}, ResponseContentTypes: []openapi.ContentType{openapi.ContentTypeJSON},
		ErrorResponses: errors,
	})
	r.Register(openapi.RouteInfo{
		Method: http.MethodDelete, Path: "/v1/account/saved-searches/{id}", OperationID: "deleteSavedSearch",
		Summary: "Delete a personal saved search", Tags: []string{"account"},
		SuccessStatus:  http.StatusNoContent,
		Description:    "Returns 204 with no body. Missing records and another user's records return 404.",
		PathParams:     []openapi.PathParam{{Name: "id", Type: "integer", Description: "Saved search ID"}},
		ErrorResponses: errors,
	})
}
