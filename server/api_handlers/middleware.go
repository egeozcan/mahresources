// Package api_handlers provides HTTP handlers for the API endpoints.
//
// This file contains middleware utilities for request parsing and response handling.
// Some functions (WithParsing, WithJSONResponse, WithRedirectOrJSON, WithDeleteResponse)
// are provided for future use when migrating additional handlers to the generic pattern.
// They enable a more functional style of handler composition.
package api_handlers

import (
	"encoding/json"
	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"net/http"
)

// getEntityID extracts an entity ID from a request, checking form body first,
// then falling back to URL query parameters with both "Id" and "id" casing.
func getEntityID(request *http.Request) uint {
	var query query_models.EntityIdQuery
	_ = tryFillStructValuesFromRequest(&query, request)
	if query.ID == 0 {
		query.ID = http_utils.GetUIntQueryParameter(request, "Id", 0)
	}
	if query.ID == 0 {
		query.ID = http_utils.GetUIntQueryParameter(request, "id", 0)
	}
	return query.ID
}

// ParsedHandler is a handler function that receives a parsed request struct.
type ParsedHandler[T any] func(writer http.ResponseWriter, request *http.Request, parsed *T)

// WithParsing returns middleware that parses the request into the given struct type
// before calling the handler. This consolidates inconsistent parsing patterns.
func WithParsing[T any](handler ParsedHandler[T]) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var parsed T
		if err := tryFillStructValuesFromRequest(&parsed, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}
		handler(writer, request, &parsed)
	}
}

// DataHandler is a handler function that returns data and an optional error.
type DataHandler[T any] func(request *http.Request) (*T, error)

// WithJSONResponse wraps a function that returns data into a JSON handler.
// It handles error responses and sets the appropriate content type.
func WithJSONResponse[T any](handler DataHandler[T]) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		data, err := handler(request)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(data)
	}
}

// RedirectOrJSONHandler is a handler that returns data which may be rendered as
// a redirect (for HTML) or JSON (for API requests).
type RedirectOrJSONHandler[T any] func(request *http.Request) (*T, string, error)

// WithRedirectOrJSON wraps a handler that returns data and a redirect URL.
// For HTML requests, it redirects to the URL. For JSON requests, it returns the data.
func WithRedirectOrJSON[T any](handler RedirectOrJSONHandler[T]) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		data, redirectURL, err := handler(request)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, redirectURL) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(data)
	}
}

// DeleteHandler is a handler function for delete operations.
type DeleteHandler func(request *http.Request) (uint, error)

// WithDeleteResponse wraps a delete handler, returning JSON with the deleted ID
// or redirecting to a list page for HTML requests.
func WithDeleteResponse[T any](handler DeleteHandler, redirectURL string) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		id, err := handler(request)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, redirectURL) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		// Return a minimal response with just the ID
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": id})
	}
}

