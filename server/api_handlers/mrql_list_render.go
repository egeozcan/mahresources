package api_handlers

import (
	"fmt"
	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers/loaders"
	"mahresources/server/template_handlers/template_context_providers"
	"net/http"
)

type mrqlListPage struct {
	NextOffset *int `json:"nextOffset,omitempty"`
	Page       int  `json:"page"`
	Size       int  `json:"size"`
	Total      int  `json:"total"`
	HasNext    bool `json:"hasNext"`
}

func relativeBucketOffset(offset *int, base int) *int {
	if offset == nil {
		return nil
	}
	relative := max(0, *offset-base)
	return &relative
}

func listPageRequest(page, size, fallback int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = fallback
	}
	if size > 100 {
		size = 100
	}
	return page, size
}

// Page within the already bounded query result. The query's LIMIT/OFFSET are
// never overwritten by display controls. Order preserves cross-entity ordering.
func pageMRQLResult(result *application_context.MRQLResult, page, size int) mrqlListPage {
	total := len(result.Resources) + len(result.Notes) + len(result.Groups)
	last := max(1, (total+size-1)/size)
	page = min(page, last)
	start, end := (page-1)*size, min(page*size, total)
	order := result.Order
	if len(order) == 0 {
		for _, r := range result.Resources {
			order = append(order, application_context.MRQLEntityIdentity{EntityType: "resource", ID: r.ID})
		}
		for _, n := range result.Notes {
			order = append(order, application_context.MRQLEntityIdentity{EntityType: "note", ID: n.ID})
		}
		for _, g := range result.Groups {
			order = append(order, application_context.MRQLEntityIdentity{EntityType: "group", ID: g.ID})
		}
	}
	selected := map[application_context.MRQLEntityIdentity]bool{}
	for _, identity := range order[start:end] {
		selected[identity] = true
	}
	resources, notes, groups := result.Resources, result.Notes, result.Groups
	result.Resources, result.Notes, result.Groups = nil, nil, nil
	for _, r := range resources {
		if selected[application_context.MRQLEntityIdentity{EntityType: "resource", ID: r.ID}] {
			result.Resources = append(result.Resources, r)
		}
	}
	for _, n := range notes {
		if selected[application_context.MRQLEntityIdentity{EntityType: "note", ID: n.ID}] {
			result.Notes = append(result.Notes, n)
		}
	}
	for _, g := range groups {
		if selected[application_context.MRQLEntityIdentity{EntityType: "group", ID: g.ID}] {
			result.Groups = append(result.Groups, g)
		}
	}
	result.Order = nil
	return mrqlListPage{Page: page, Size: size, Total: total, HasNext: end < total}
}

// Render the very same entity partials as ordinary lists, with the same hydrated
// models. Only server-authored templates are evaluated here.
func renderMRQLListCards(ctx MRQLAPIContext, request *http.Request, items any) error {
	app, ok := withRequestContext(ctx, request).(mrqlListRenderContext)
	if !ok {
		return fmt.Errorf("request context does not support MRQL list rendering")
	}
	set := pongo2.NewSet("mrql-list", loaders.MustNewLocalFileSystemLoader("./templates", nil))
	c := template_context_providers.StaticTemplateCtx(request)
	c["_appContext"], c["_requestContext"], c["_pluginManager"] = app, request.Context(), app.PluginManager()
	c["_reqCtxWithCache"] = request.Context()
	c["_pluginAccess"] = auth.PluginAccessFor(request.Context(), app.PluginAllowsScopedPrincipals)
	c["selectable"] = true
	nextID := c["getNextId"].(func(string) string)
	render := func(entityType string, entity any) (string, error) {
		c["entity"], c["tagBaseUrl"] = entity, "/"+entityType+"s"
		c["getNextId"] = func(name string) string { return "mrql_" + entityType + "_" + nextID(name) }
		if pm := app.PluginManager(); pm != nil {
			actions := pm.GetListActionsForPlacement(entityType, "card", nil)
			allowed := actions[:0]
			access := auth.PluginActionAccessFor(request.Context(), app.PluginAllowsScopedPrincipals)
			for _, a := range actions {
				if access(a.PluginName) {
					allowed = append(allowed, a)
				}
			}
			c["pluginCardActions"] = allowed
		}
		c["entityCardTemplate"] = "/partials/" + entityType + ".tpl"
		tpl, err := set.FromFile("/partials/mrqlListCard.tpl")
		if err != nil {
			return "", err
		}
		return tpl.Execute(c)
	}
	switch rows := items.(type) {
	case []models.Resource:
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		loaded, err := app.GetResources(0, len(ids), &query_models.ResourceSearchQuery{Ids: ids})
		if err != nil {
			return err
		}
		byID := map[uint]models.Resource{}
		for _, r := range loaded {
			byID[r.ID] = r
		}
		for i := range rows {
			r, exists := byID[rows[i].ID]
			if !exists {
				return fmt.Errorf("resource no longer available")
			}
			rows[i] = r
			html, err := render("resource", &rows[i])
			if err != nil {
				return err
			}
			rows[i].RenderedHTML = html
		}
	case []models.Note:
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		loaded, err := app.GetNotes(0, len(ids), &query_models.NoteQuery{Ids: ids})
		if err != nil {
			return err
		}
		byID := map[uint]models.Note{}
		for _, n := range loaded {
			byID[n.ID] = *sanitizeDeferredEntity(&n).(*models.Note)
		}
		for i := range rows {
			n, exists := byID[rows[i].ID]
			if !exists {
				return fmt.Errorf("note no longer available")
			}
			rows[i] = n
			html, err := render("note", &rows[i])
			if err != nil {
				return err
			}
			rows[i].RenderedHTML = html
		}
	case []models.Group:
		if len(rows) == 0 {
			return nil
		}
		ids := make([]uint, len(rows))
		for i := range rows {
			ids[i] = rows[i].ID
		}
		loaded, err := app.GetGroups(0, len(ids), &query_models.GroupQuery{Ids: ids})
		if err != nil {
			return err
		}
		byID := map[uint]models.Group{}
		for _, g := range loaded {
			byID[g.ID] = g
		}
		for i := range rows {
			g, exists := byID[rows[i].ID]
			if !exists {
				return fmt.Errorf("group no longer available")
			}
			rows[i] = g
			html, err := render("group", &rows[i])
			if err != nil {
				return err
			}
			rows[i].RenderedHTML = html
		}
	}
	return nil
}

// Hydrate and render the complete bucket page as one batch, sharing the render
// budget, CSS deduplication and generated-ID sequence across all its buckets.
func renderMRQLListBuckets(ctx MRQLAPIContext, request *http.Request, result *application_context.MRQLGroupedResult) error {
	switch result.EntityType {
	case "resource":
		return renderListBuckets[models.Resource](ctx, request, result)
	case "note":
		return renderListBuckets[models.Note](ctx, request, result)
	case "group":
		return renderListBuckets[models.Group](ctx, request, result)
	}
	return fmt.Errorf("unsupported MRQL entity type")
}

func renderListBuckets[T any](ctx MRQLAPIContext, request *http.Request, result *application_context.MRQLGroupedResult) error {
	var rows []T
	for _, bucket := range result.Groups {
		rows = append(rows, bucket.Items.([]T)...)
	}
	if err := renderMRQLListCards(ctx, request, rows); err != nil {
		return err
	}
	offset := 0
	for i := range result.Groups {
		n := len(result.Groups[i].Items.([]T))
		result.Groups[i].Items = rows[offset : offset+n]
		offset += n
	}
	return nil
}
