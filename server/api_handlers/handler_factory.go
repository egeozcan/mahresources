package api_handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"mahresources/constants"
	"mahresources/models/query_models"
	"mahresources/server/http_utils"
	"mahresources/server/interfaces"
)

// CRUDHandlerFactory creates HTTP handlers for standard CRUD operations.
// T is the entity type, Q is the query type, C is the creator type.
//
// IMPORTANT: Q must be a pointer type (e.g., *TagQuery, not TagQuery).
// The ListHandler and CountHandler use reflection to instantiate query objects,
// which requires Q to be a pointer so we can create the underlying struct.
type CRUDHandlerFactory[T interfaces.BasicEntityReader, Q, C any] struct {
	entityName       string // Singular form, e.g., "tag"
	entityNamePlural string // Plural form, e.g., "tags"
	reader           interfaces.GenericReader[T, Q]
	writer           interfaces.GenericWriter[T, C]
}

// NewCRUDHandlerFactory creates a new handler factory for an entity.
func NewCRUDHandlerFactory[T interfaces.BasicEntityReader, Q, C any](
	entityName, entityNamePlural string,
	reader interfaces.GenericReader[T, Q],
	writer interfaces.GenericWriter[T, C],
) *CRUDHandlerFactory[T, Q, C] {
	return &CRUDHandlerFactory[T, Q, C]{
		entityName:       entityName,
		entityNamePlural: entityNamePlural,
		reader:           reader,
		writer:           writer,
	}
}

// GetHandler returns a handler for retrieving a single entity by ID.
func (f *CRUDHandlerFactory[T, Q, C]) GetHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		id := uint(http_utils.GetIntQueryParameter(request, "id", 0))
		if id == 0 {
			http_utils.HandleError(errors.New(f.entityName+" id is required"), writer, request, http.StatusBadRequest)
			return
		}

		entity, err := f.reader.Get(id)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(entity)
	}
}

// ListHandler returns a handler for listing entities with pagination and filtering.
// Note: Q must be a pointer type (e.g., *TagQuery) for proper decoding.
func (f *CRUDHandlerFactory[T, Q, C]) ListHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		page := http_utils.GetIntQueryParameter(request, "page", 1)
		offset := (page - 1) * constants.MaxResultsPerPage

		// Create a new instance of the underlying query type
		var query Q
		// Use reflect to create a new instance if Q is a pointer type
		queryVal := reflect.New(reflect.TypeOf(query).Elem())
		queryPtr := queryVal.Interface()

		if err := decoder.Decode(queryPtr, request.URL.Query()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// Cast back to Q
		typedQuery := queryPtr.(Q)
		entities, err := f.reader.List(int(offset), constants.MaxResultsPerPage, typedQuery)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusNotFound)
			return
		}

		// Get total count for pagination metadata
		totalCount, countErr := f.reader.Count(typedQuery)
		if countErr != nil {
			totalCount = -1
		}

		http_utils.SetPaginationHeaders(writer, int(page), constants.MaxResultsPerPage, totalCount)
		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(entities)
	}
}

// CountHandler returns a handler for counting entities matching a query.
// Note: Q must be a pointer type (e.g., *TagQuery) for proper decoding.
func (f *CRUDHandlerFactory[T, Q, C]) CountHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		// Create a new instance of the underlying query type
		var query Q
		queryVal := reflect.New(reflect.TypeOf(query).Elem())
		queryPtr := queryVal.Interface()

		if err := decoder.Decode(queryPtr, request.URL.Query()); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		typedQuery := queryPtr.(Q)
		count, err := f.reader.Count(typedQuery)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]int64{"count": count})
	}
}

// DeleteHandler returns a handler for deleting an entity by ID.
// Accepts ID from form body (field "id" or "ID") or URL query parameter ("Id" or "id").
func (f *CRUDHandlerFactory[T, Q, C]) DeleteHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var query query_models.EntityIdQuery

		if err := tryFillStructValuesFromRequest(&query, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		// Fall back to URL query parameter if ID not found in form body
		// This handles cases where ID is passed as ?Id=123 in the URL
		if query.ID == 0 {
			query.ID = http_utils.GetUIntQueryParameter(request, "Id", 0)
		}
		if query.ID == 0 {
			query.ID = http_utils.GetUIntQueryParameter(request, "id", 0)
		}

		if query.ID == 0 {
			http_utils.HandleError(errors.New(f.entityName+" id is required"), writer, request, http.StatusBadRequest)
			return
		}

		if err := f.writer.Delete(query.ID); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusInternalServerError)
			return
		}

		if http_utils.RedirectIfHTMLAccepted(writer, request, "/"+f.entityNamePlural) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(map[string]uint{"id": query.ID})
	}
}

// CreateHandler returns a handler for creating a new entity.
// This is a basic create handler - entities with update logic should use CreateOrUpdateHandler.
func (f *CRUDHandlerFactory[T, Q, C]) CreateHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		var creator C

		if err := tryFillStructValuesFromRequest(&creator, request); err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		entity, err := f.writer.Create(creator)
		if err != nil {
			http_utils.HandleError(err, writer, request, http.StatusBadRequest)
			return
		}

		redirectURL := "/" + f.entityName + "?id=" + strconv.Itoa(int((*entity).GetId()))
		if http_utils.RedirectIfHTMLAccepted(writer, request, redirectURL) {
			return
		}

		writer.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(writer).Encode(entity)
	}
}

// createOrUpdateHandler creates an HTTP handler for create-or-update operations.
// getID extracts the ID from the decoded request struct.
// create and update are the respective operations.
// entityName is used for redirect URL construction (e.g., "tag", "category").
func createOrUpdateHandler[T any](
	entityName string,
	getID func(*T) uint,
	create func(*T) (any, error),
	update func(*T) (any, error),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var editor T

		if err := tryFillStructValuesFromRequest(&editor, r); err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		var result any
		var err error

		if getID(&editor) != 0 {
			result, err = update(&editor)
		} else {
			result, err = create(&editor)
		}

		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		type hasID interface{ GetId() uint }
		if entity, ok := result.(hasID); ok {
			redirectURL := "/" + entityName + "?id=" + strconv.Itoa(int(entity.GetId()))
			if http_utils.RedirectIfHTMLAccepted(w, r, redirectURL) {
				return
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// CreateTagHandler returns a handler that creates or updates tags based on ID presence.
// For JSON update requests, it pre-fills unset fields from the existing tag
// so that partial JSON updates don't clear fields, while still allowing explicit clearing.
func CreateTagHandler(writer interfaces.TagsWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var creator query_models.TagCreator
		var sentFields map[string]bool

		if strings.HasPrefix(r.Header.Get("Content-type"), constants.JSON) {
			bodyBytes, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				http_utils.HandleError(readErr, w, r, http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(bodyBytes, &creator); err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(bodyBytes, &raw)
			sentFields = make(map[string]bool, len(raw))
			for k := range raw {
				sentFields[k] = true
			}
		} else {
			if err := tryFillStructValuesFromRequest(&creator, r); err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
		}

		var result any
		var err error

		if creator.ID != 0 {
			if sentFields != nil {
				existing, getErr := writer.GetTagByID(creator.ID)
				if getErr == nil {
					if !sentFields["Description"] {
						creator.Description = existing.Description
					}
				}
			}
			result, err = writer.UpdateTag(&creator)
		} else {
			result, err = writer.CreateTag(&creator)
		}

		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		type hasID interface{ GetId() uint }
		if entity, ok := result.(hasID); ok {
			if http_utils.RedirectIfHTMLAccepted(w, r, "/tag?id="+strconv.Itoa(int(entity.GetId()))) {
				return
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// CreateCategoryHandler returns a handler that creates or updates categories.
// For JSON update requests, it pre-fills unset fields from the existing entity
// so that partial JSON updates don't clear fields that were not included in
// the request body, while still allowing explicit clearing (sending "").
func CreateCategoryHandler(ctx interfaces.CategoryCRUDReader) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var editor query_models.CategoryEditor
		var sentFields map[string]bool

		// For JSON requests, buffer the body so we can detect which fields
		// were explicitly included (distinguishing absent vs. empty).
		if strings.HasPrefix(r.Header.Get("Content-type"), constants.JSON) {
			bodyBytes, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				http_utils.HandleError(readErr, w, r, http.StatusBadRequest)
				return
			}

			// Decode into the struct
			if err := json.Unmarshal(bodyBytes, &editor); err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}

			// Decode into a raw map to discover which keys were sent
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(bodyBytes, &raw)
			sentFields = make(map[string]bool, len(raw))
			for k := range raw {
				sentFields[k] = true
			}
		} else {
			if err := tryFillStructValuesFromRequest(&editor, r); err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
		}

		var result any
		var err error

		if editor.ID != 0 {
			// For JSON requests, pre-populate unset string fields from the
			// existing category so partial updates don't clear them.
			if sentFields != nil {
				existing, getErr := ctx.GetCategory(editor.ID)
				if getErr == nil {
					if !sentFields["Name"] && existing.Name != "" {
						editor.Name = existing.Name
					}
					if !sentFields["Description"] {
						editor.Description = existing.Description
					}
					if !sentFields["CustomHeader"] {
						editor.CustomHeader = existing.CustomHeader
					}
					if !sentFields["CustomSidebar"] {
						editor.CustomSidebar = existing.CustomSidebar
					}
					if !sentFields["CustomSummary"] {
						editor.CustomSummary = existing.CustomSummary
					}
					if !sentFields["CustomAvatar"] {
						editor.CustomAvatar = existing.CustomAvatar
					}
					if !sentFields["MetaSchema"] {
						editor.MetaSchema = existing.MetaSchema
					}
				}
			}
			result, err = ctx.UpdateCategory(&editor)
		} else {
			result, err = ctx.CreateCategory(&editor.CategoryCreator)
		}

		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		type hasID interface{ GetId() uint }
		if entity, ok := result.(hasID); ok {
			redirectURL := "/category?id=" + strconv.Itoa(int(entity.GetId()))
			if http_utils.RedirectIfHTMLAccepted(w, r, redirectURL) {
				return
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// CreateResourceCategoryHandler returns a handler that creates or updates resource categories.
// For JSON update requests, pre-fills unset fields from the existing entity.
func CreateResourceCategoryHandler(writer interfaces.ResourceCategoryWriter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var editor query_models.ResourceCategoryEditor
		var sentFields map[string]bool

		if strings.HasPrefix(r.Header.Get("Content-type"), constants.JSON) {
			bodyBytes, readErr := io.ReadAll(r.Body)
			if readErr != nil {
				http_utils.HandleError(readErr, w, r, http.StatusBadRequest)
				return
			}
			if err := json.Unmarshal(bodyBytes, &editor); err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
			var raw map[string]json.RawMessage
			_ = json.Unmarshal(bodyBytes, &raw)
			sentFields = make(map[string]bool, len(raw))
			for k := range raw {
				sentFields[k] = true
			}
		} else {
			if err := tryFillStructValuesFromRequest(&editor, r); err != nil {
				http_utils.HandleError(err, w, r, http.StatusBadRequest)
				return
			}
		}

		var result any
		var err error

		if editor.ID != 0 {
			if sentFields != nil {
				existing, getErr := writer.GetResourceCategory(editor.ID)
				if getErr == nil {
					if !sentFields["Name"] && existing.Name != "" {
						editor.Name = existing.Name
					}
					if !sentFields["Description"] {
						editor.Description = existing.Description
					}
					if !sentFields["CustomHeader"] {
						editor.CustomHeader = existing.CustomHeader
					}
					if !sentFields["CustomSidebar"] {
						editor.CustomSidebar = existing.CustomSidebar
					}
					if !sentFields["CustomSummary"] {
						editor.CustomSummary = existing.CustomSummary
					}
					if !sentFields["CustomAvatar"] {
						editor.CustomAvatar = existing.CustomAvatar
					}
					if !sentFields["MetaSchema"] {
						editor.MetaSchema = existing.MetaSchema
					}
				}
			}
			result, err = writer.UpdateResourceCategory(&editor)
		} else {
			result, err = writer.CreateResourceCategory(&editor.ResourceCategoryCreator)
		}

		if err != nil {
			http_utils.HandleError(err, w, r, http.StatusBadRequest)
			return
		}

		type hasID interface{ GetId() uint }
		if entity, ok := result.(hasID); ok {
			if http_utils.RedirectIfHTMLAccepted(w, r, "/resourceCategory?id="+strconv.Itoa(int(entity.GetId()))) {
				return
			}
		}

		w.Header().Set("Content-Type", constants.JSON)
		_ = json.NewEncoder(w).Encode(result)
	}
}

// CreateQueryHandler returns a handler that creates or updates queries.
// Queries use QueryCreator for create and QueryEditor (with ID) for update.
func CreateQueryHandler(writer interfaces.QueryWriter) http.HandlerFunc {
	return createOrUpdateHandler(
		"query",
		func(e *query_models.QueryEditor) uint { return e.ID },
		func(e *query_models.QueryEditor) (any, error) { return writer.CreateQuery(&e.QueryCreator) },
		func(e *query_models.QueryEditor) (any, error) { return writer.UpdateQuery(e) },
	)
}
