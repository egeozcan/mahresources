package api_handlers

import (
	"fmt"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/server/template_handlers"
	"mahresources/shortcodes"
	"net/http"
)

type mrqlListPage struct {
	NextItemOffset int  `json:"nextItemOffset,omitempty"`
	NextOffset     *int `json:"nextOffset,omitempty"`
	Page           int  `json:"page"`
	Size           int  `json:"size"`
	Total          int  `json:"total"`
	HasNext        bool `json:"hasNext"`
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

// Hydration runs through the request-scoped ORM, then preserves query order.
// Rows deleted or moved out of scope between execution and hydration disappear.
func renderMRQLListCards(ctx MRQLAPIContext, request *http.Request, result *application_context.MRQLResult) error {
	app, ok := withRequestContext(ctx, request).(mrqlListRenderContext)
	if !ok {
		return fmt.Errorf("request context does not support MRQL list rendering")
	}
	headingLevel := 3
	if result.EntityType == "all" || result.EntityType == "bucketed" {
		headingLevel = 4
	}
	render := template_handlers.NewListCardRenderer(app, request, headingLevel)
	var err error
	result.Resources, err = hydrateListCards(result.Resources, func(r models.Resource) uint { return r.ID },
		func(ids []uint) ([]models.Resource, error) {
			return app.GetResources(0, len(ids), &query_models.ResourceSearchQuery{Ids: ids})
		},
		func(r *models.Resource) error { html, err := render("resource", r); r.RenderedHTML = html; return err })
	if err != nil {
		return err
	}
	result.Notes, err = hydrateListCards(result.Notes, func(n models.Note) uint { return n.ID },
		func(ids []uint) ([]models.Note, error) {
			return app.GetNotes(0, len(ids), &query_models.NoteQuery{Ids: ids})
		},
		func(n *models.Note) error {
			*n = *sanitizeDeferredEntity(n).(*models.Note)
			html, err := render("note", n)
			n.RenderedHTML = html
			return err
		})
	if err != nil {
		return err
	}
	result.Groups, err = hydrateListCards(result.Groups, func(g models.Group) uint { return g.ID },
		func(ids []uint) ([]models.Group, error) {
			return app.GetGroups(0, len(ids), &query_models.GroupQuery{Ids: ids})
		},
		func(g *models.Group) error { html, err := render("group", g); g.RenderedHTML = html; return err })
	if budget := shortcodes.QueryBudgetFrom(request.Context()); budget != nil && budget.Stats().Exceeded {
		result.Warnings = append(result.Warnings, "Inline MRQL query budget reached. Choose fewer items per page to render all card content.")
	}
	return err
}

func hydrateListCards[T any](rows []T, identity func(T) uint, load func([]uint) ([]T, error), render func(*T) error) ([]T, error) {
	ids := make([]uint, 0, len(rows))
	seen := map[uint]bool{}
	for _, row := range rows {
		id := identity(row)
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	byID := map[uint]T{}
	for start := 0; start < len(ids); start += 500 {
		loaded, err := load(ids[start:min(start+500, len(ids))])
		if err != nil {
			return nil, err
		}
		for _, row := range loaded {
			byID[identity(row)] = row
		}
	}
	visible := make([]T, 0, len(rows))
	for _, row := range rows {
		entity, exists := byID[identity(row)]
		if !exists {
			continue
		}
		if err := render(&entity); err != nil {
			return nil, err
		}
		visible = append(visible, entity)
	}
	return visible, nil
}

func renderMRQLListBuckets(ctx MRQLAPIContext, request *http.Request, result *application_context.MRQLGroupedResult) error {
	flat := &application_context.MRQLResult{EntityType: "bucketed"}
	for _, bucket := range result.Groups {
		switch rows := bucket.Items.(type) {
		case []models.Resource:
			flat.Resources = append(flat.Resources, rows...)
		case []models.Note:
			flat.Notes = append(flat.Notes, rows...)
		case []models.Group:
			flat.Groups = append(flat.Groups, rows...)
		}
	}
	if err := renderMRQLListCards(ctx, request, flat); err != nil {
		return err
	}
	result.Warnings = append(result.Warnings, flat.Warnings...)
	// Match by identity rather than original lengths: concurrent removals must
	// neither fail the page nor move a surviving row into another bucket.
	for i := range result.Groups {
		switch rows := result.Groups[i].Items.(type) {
		case []models.Resource:
			result.Groups[i].Items = visibleBucketRows(rows, flat.Resources, func(r models.Resource) uint { return r.ID })
		case []models.Note:
			result.Groups[i].Items = visibleBucketRows(rows, flat.Notes, func(n models.Note) uint { return n.ID })
		case []models.Group:
			result.Groups[i].Items = visibleBucketRows(rows, flat.Groups, func(g models.Group) uint { return g.ID })
		}
	}
	return nil
}

func visibleBucketRows[T any](rows, rendered []T, identity func(T) uint) []T {
	// Consume occurrences so each repeated card retains its own generated IDs.
	result := make([]T, 0, len(rows))
	for _, row := range rows {
		for i, candidate := range rendered {
			if identity(candidate) == identity(row) {
				result = append(result, candidate)
				var zero T
				rendered[i] = zero
				break
			}
		}
	}
	return result
}
