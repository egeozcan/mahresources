package api_handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

type EntityPickerAPIContext interface {
	contracts.EntityPickerReader
	templateRenderContext
	PluginAllowsScopedPrincipals(string) bool
	LoadMRQLRenderData(context.Context, []uint, []uint, []uint, []uint) (*application_context.MRQLRenderData, error)
	Logger() *application_context.Logger
}

type EntityPickerResult struct {
	Value map[string]any `json:"value"`
	HTML  string         `json:"html"`
}
type EntityPickerStyle struct {
	Key string `json:"key"`
	CSS string `json:"css"`
}
type EntityPickerResponse struct {
	Items    []EntityPickerResult `json:"items"`
	Page     int                  `json:"page"`
	HasNext  bool                 `json:"hasNext"`
	Styles   []EntityPickerStyle  `json:"styles"`
	Warnings []string             `json:"warnings"`
}
type EntityPickerResolveResponse struct {
	Items []map[string]any `json:"items"`
}

// Values are copied and projected before transport. Carrier template sources,
// note bodies and unloaded model associations are not selector state.
func pickerValue(row contracts.EntityPickerEntity) map[string]any {
	// Description is the note's body, potentially megabytes. Keep it available
	// to custom rendering, but do not marshal/copy it into selector state.
	note, isNote := row.Raw.(models.Note)
	if isNote {
		note.Description = ""
		row.Raw = note
	}
	bytes, _ := json.Marshal(row.Raw)
	var raw map[string]any
	_ = json.Unmarshal(bytes, &raw)
	value := map[string]any{"ID": row.ID, "Name": row.Name}
	for _, key := range []string{"Description", "Meta", "MetaSchema", "OwnerId", "CategoryId", "NoteTypeId", "ResourceCategoryId", "SeriesID", "ContentType", "Width", "Height", "FileSize", "CreatedAt", "UpdatedAt"} {
		if v, ok := raw[key]; ok {
			value[key] = v
		}
	}
	if isNote {
		delete(value, "Description")
	}
	for _, key := range []string{"Owner", "Category", "NoteType", "ResourceCategory", "Series"} {
		if association, ok := raw[key].(map[string]any); ok {
			value[key] = map[string]any{"ID": association["ID"], "Name": association["Name"]}
		}
	}
	if tags, ok := raw["Tags"].([]any); ok {
		projected := make([]map[string]any, 0, len(tags))
		for _, tag := range tags {
			if v, ok := tag.(map[string]any); ok {
				projected = append(projected, map[string]any{"ID": v["ID"], "Name": v["Name"]})
			}
		}
		value["Tags"] = projected
	}
	return value
}

func pickerHTTPError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, query_models.ErrEntityPickerInput) {
		status = http.StatusBadRequest
	}
	http.Error(w, http.StatusText(status), status)
}

func GetEntityPickerResolveHandler(app EntityPickerAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		app := withRequestContext(app, r).(EntityPickerAPIContext)
		var query query_models.EntityPickerResolveQuery
		if err := decoder.Decode(&query, r.URL.Query()); err != nil {
			http.Error(w, "invalid entity picker request", 400)
			return
		}
		rows, err := app.ResolvePickerEntities(&query)
		if err != nil {
			pickerHTTPError(w, err)
			return
		}
		response := EntityPickerResolveResponse{Items: make([]map[string]any, 0, len(rows))}
		for _, row := range rows {
			response.Items = append(response.Items, pickerValue(row))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func GetEntityPickerHandler(app EntityPickerAPIContext) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		reqCtx, cancel := buildMRQLAPIRenderContext(r.Context(), app, false)
		defer cancel()
		app := withRequestContext(app, r.WithContext(reqCtx)).(EntityPickerAPIContext)
		var query query_models.EntityPickerQuery
		if err := decoder.Decode(&query, r.URL.Query()); err != nil {
			http.Error(w, "invalid entity picker request", 400)
			return
		}
		page, err := app.BrowseEntities(&query)
		if err != nil {
			pickerHTTPError(w, err)
			return
		}
		// Batch the scope walk. Building each meta context with a resolver would
		// turn a 50-result page into 50 hierarchy queries.
		var ids apiRenderIDs
		for _, row := range page.Items {
			switch v := row.Raw.(type) {
			case models.Group:
				collectAPIRenderIDs(&application_context.MRQLResult{Groups: []models.Group{v}}, &ids)
			case models.Resource:
				collectAPIRenderIDs(&application_context.MRQLResult{Resources: []models.Resource{v}}, &ids)
			case models.Note:
				collectAPIRenderIDs(&application_context.MRQLResult{Notes: []models.Note{v}}, &ids)
			}
		}
		data, err := app.LoadMRQLRenderData(reqCtx, ids.resourceCategories, ids.noteTypes, ids.categories, ids.scopes)
		if err != nil {
			pickerHTTPError(w, err)
			return
		}
		response := EntityPickerResponse{Items: []EntityPickerResult{}, Page: page.Page, HasNext: page.HasNext, Styles: []EntityPickerStyle{}, Warnings: []string{}}
		renderer, executor := buildPluginRenderer(app, reqCtx), template_filters.BuildQueryExecutor(app)
		stylesSeen, warned := map[string]bool{}, map[string]bool{}
		warn := func(key string) {
			if warned[key] {
				return
			}
			warned[key] = true
			message := "A custom picker result could not be rendered; default content was used."
			response.Warnings = append(response.Warnings, message)
			app.Logger().Warning("entity_picker_render", "template", nil, "", message, map[string]interface{}{"carrier": key})
		}
		for _, row := range page.Items {
			if reqCtx.Err() != nil {
				pickerHTTPError(w, reqCtx.Err())
				return
			}
			key, body, css, owner := pickerCarrier(query.Entity, row.Raw)
			content := ""
			if meta := template_filters.BuildMetaContextForEntity(row.Raw, nil); meta != nil {
				meta.ScopeGroupID, meta.ParentGroupID, meta.RootGroupID = resolveAPIScopeFields(data, query.Entity, owner, row.ID)
				applyAPIPresentationContext(meta, data, query.Entity, owner, row.ID)
				if strings.TrimSpace(body) != "" {
					result := shortcodes.ProcessWithDiagnostics(reqCtx, body, *meta, renderer, executor)
					if len(result.Errors) > 0 {
						warn(key)
					} else {
						content = result.HTML
					}
				}
				if key != "" && !stylesSeen[key] {
					stylesSeen[key] = true
					result := shortcodes.ProcessWithDiagnostics(reqCtx, css, *meta, renderer, executor)
					if len(result.Errors) > 0 {
						warn(key)
					} else if strings.TrimSpace(result.HTML) != "" {
						response.Styles = append(response.Styles, EntityPickerStyle{Key: key, CSS: result.HTML})
					}
				}
			}
			if strings.TrimSpace(content) == "" {
				content, err = template_handlers.RenderEntityPickerDefault(query.Entity, row)
				if err != nil {
					pickerHTTPError(w, err)
					return
				}
			}
			response.Items = append(response.Items, EntityPickerResult{Value: pickerValue(row), HTML: content})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}
}

func pickerCarrier(entity string, raw any) (key, body, css string, owner *uint) {
	switch v := raw.(type) {
	case models.Group:
		owner = v.OwnerId
		if c := v.Category; c != nil {
			key = fmt.Sprintf("%s:%d", entity, c.ID)
			body = c.CustomEntityPickerResult
			css = c.CustomCSS + "\n" + c.CustomEntityPickerResultCSS
		}
	case models.Resource:
		owner = v.OwnerId
		if c := v.ResourceCategory; c != nil {
			key = fmt.Sprintf("%s:%d", entity, c.ID)
			body = c.CustomEntityPickerResult
			css = c.CustomCSS + "\n" + c.CustomEntityPickerResultCSS
		}
	case models.Note:
		owner = v.OwnerId
		if c := v.NoteType; c != nil {
			key = fmt.Sprintf("%s:%d", entity, c.ID)
			body = c.CustomEntityPickerResult
			css = c.CustomCSS + "\n" + c.CustomEntityPickerResultCSS
		}
	}
	return
}
