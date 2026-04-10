package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/plugin_system"
	"mahresources/server/api_handlers"
	"mahresources/server/template_handlers"
	"mahresources/server/template_handlers/template_context_providers"
	template_filters "mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

type templateInformation struct {
	contextFn    func(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context
	templateName string
	method       string
}

var templates = map[string]templateInformation{
	"/dashboard": {template_context_providers.DashboardContextProvider, "dashboard.tpl", http.MethodGet},

	"/note/new":      {template_context_providers.NoteCreateContextProvider, "createNote.tpl", http.MethodGet},
	"/notes":         {template_context_providers.NoteListContextProvider, "listNotes.tpl", http.MethodGet},
	"/note":          {template_context_providers.NoteContextProvider, "displayNote.tpl", http.MethodGet},
	"/note/text":     {template_context_providers.NoteContextProvider, "displayNoteText.tpl", http.MethodGet},
	"/note/edit":     {template_context_providers.NoteCreateContextProvider, "createNote.tpl", http.MethodGet},
	"/noteType/new":  {template_context_providers.NoteTypeCreateContextProvider, "createNoteType.tpl", http.MethodGet},
	"/noteTypes":     {template_context_providers.NoteTypeListContextProvider, "listNoteTypes.tpl", http.MethodGet},
	"/noteType":      {template_context_providers.NoteTypeContextProvider, "displayNoteType.tpl", http.MethodGet},
	"/noteType/edit": {template_context_providers.NoteTypeCreateContextProvider, "createNoteType.tpl", http.MethodGet},

	"/resource/new":      {template_context_providers.ResourceCreateContextProvider, "createResource.tpl", http.MethodGet},
	"/resources":         {template_context_providers.ResourceListContextProvider, "listResources.tpl", http.MethodGet},
	"/resources/details": {template_context_providers.ResourceListContextProvider, "listResourcesDetails.tpl", http.MethodGet},
	"/resources/simple":  {template_context_providers.ResourceListContextProvider, "listResourcesSimple.tpl", http.MethodGet},
	"/resource":          {template_context_providers.ResourceContextProvider, "displayResource.tpl", http.MethodGet},
	"/resource/edit":     {template_context_providers.ResourceCreateContextProvider, "createResource.tpl", http.MethodGet},
	"/resource/compare":  {template_context_providers.CompareContextProvider, "compare.tpl", http.MethodGet},

	"/series": {template_context_providers.SeriesContextProvider, "displaySeries.tpl", http.MethodGet},

	"/group/new":   {template_context_providers.GroupCreateContextProvider, "createGroup.tpl", http.MethodGet},
	"/groups":      {template_context_providers.GroupsListContextProvider, "listGroups.tpl", http.MethodGet},
	"/groups/text": {template_context_providers.GroupsListContextProvider, "listGroupsText.tpl", http.MethodGet},
	"/group":       {template_context_providers.GroupContextProvider, "displayGroup.tpl", http.MethodGet},
	"/group/edit":  {template_context_providers.GroupCreateContextProvider, "createGroup.tpl", http.MethodGet},
	"/group/tree":  {template_context_providers.GroupTreeContextProvider, "displayGroupTree.tpl", http.MethodGet},

	"/tag/new":  {template_context_providers.TagCreateContextProvider, "createTag.tpl", http.MethodGet},
	"/tags":     {template_context_providers.TagListContextProvider, "listTags.tpl", http.MethodGet},
	"/tag":      {template_context_providers.TagContextProvider, "displayTag.tpl", http.MethodGet},
	"/tag/edit": {template_context_providers.TagCreateContextProvider, "createTag.tpl", http.MethodGet},

	"/relationType/edit": {template_context_providers.RelationTypeEditContextProvider, "createRelationType.tpl", http.MethodGet},
	"/relationType/new":  {template_context_providers.RelationTypeCreateContextProvider, "createRelationType.tpl", http.MethodGet},
	"/relation/new":      {template_context_providers.RelationCreateContextProvider, "createRelation.tpl", http.MethodGet},
	"/relation/edit":     {template_context_providers.RelationEditContextProvider, "createRelation.tpl", http.MethodGet},
	"/relationTypes":     {template_context_providers.RelationTypeListContextProvider, "listRelationTypes.tpl", http.MethodGet},
	"/relations":         {template_context_providers.RelationListContextProvider, "listRelations.tpl", http.MethodGet},
	"/relationType":      {template_context_providers.RelationTypeContextProvider, "displayRelationType.tpl", http.MethodGet},
	"/relation":          {template_context_providers.RelationContextProvider, "displayRelation.tpl", http.MethodGet},

	"/category/new":  {template_context_providers.CategoryCreateContextProvider, "createCategory.tpl", http.MethodGet},
	"/categories":    {template_context_providers.CategoryListContextProvider, "listCategories.tpl", http.MethodGet},
	"/category":      {template_context_providers.CategoryContextProvider, "displayCategory.tpl", http.MethodGet},
	"/category/edit": {template_context_providers.CategoryCreateContextProvider, "createCategory.tpl", http.MethodGet},

	"/resourceCategory/new":  {template_context_providers.ResourceCategoryCreateContextProvider, "createResourceCategory.tpl", http.MethodGet},
	"/resourceCategories":    {template_context_providers.ResourceCategoryListContextProvider, "listResourceCategories.tpl", http.MethodGet},
	"/resourceCategory":      {template_context_providers.ResourceCategoryContextProvider, "displayResourceCategory.tpl", http.MethodGet},
	"/resourceCategory/edit": {template_context_providers.ResourceCategoryCreateContextProvider, "createResourceCategory.tpl", http.MethodGet},

	"/query/new":  {template_context_providers.QueryCreateContextProvider, "createQuery.tpl", http.MethodGet},
	"/queries":    {template_context_providers.QueryListContextProvider, "listQueries.tpl", http.MethodGet},
	"/query":      {template_context_providers.QueryContextProvider, "displayQuery.tpl", http.MethodGet},
	"/query/edit": {template_context_providers.QueryCreateContextProvider, "createQuery.tpl", http.MethodGet},

	"/resources/timeline":  {template_context_providers.ResourceTimelineContextProvider, "listResourcesTimeline.tpl", http.MethodGet},
	"/notes/timeline":      {template_context_providers.NoteTimelineContextProvider, "listNotesTimeline.tpl", http.MethodGet},
	"/groups/timeline":     {template_context_providers.GroupTimelineContextProvider, "listGroupsTimeline.tpl", http.MethodGet},
	"/tags/timeline":       {template_context_providers.TagTimelineContextProvider, "listTagsTimeline.tpl", http.MethodGet},
	"/categories/timeline": {template_context_providers.CategoryTimelineContextProvider, "listCategoriesTimeline.tpl", http.MethodGet},
	"/queries/timeline":    {template_context_providers.QueryTimelineContextProvider, "listQueriesTimeline.tpl", http.MethodGet},

	"/logs": {template_context_providers.LogListContextProvider, "listLogs.tpl", http.MethodGet},
	"/log":  {template_context_providers.LogContextProvider, "displayLog.tpl", http.MethodGet},

	"/admin/overview": {template_context_providers.AdminOverviewContextProvider, "adminOverview.tpl", http.MethodGet},

	"/mrql": {template_context_providers.MRQLContextProvider, "mrql.tpl", http.MethodGet},
}

func wrapContextWithPlugins(appContext *application_context.MahresourcesContext, ctxFn func(request *http.Request) pongo2.Context) func(request *http.Request) pongo2.Context {
	pm := appContext.PluginManager()
	return func(request *http.Request) pongo2.Context {
		ctx := ctxFn(request)

		// Always set — needed for [mrql] shortcodes even without plugins
		ctx["_appContext"] = appContext
		ctx["_requestContext"] = request.Context()

		if pm == nil {
			if strings.HasSuffix(request.URL.Path, ".json") ||
				strings.Contains(request.Header.Get("Accept"), constants.JSON) {
				processShortcodesForJSON(ctx, nil, appContext, request.Context())
			}
			return ctx
		}

		ctx["_pluginManager"] = pm
		ctx["currentPath"] = request.URL.Path
		ctx["pluginMenuItems"] = pm.GetMenuItems()
		ctx["hasPluginManager"] = true

		// Compute plugin actions for detail pages
		if mainEntity := ctx["mainEntity"]; mainEntity != nil {
			if entityType, ok := ctx["mainEntityType"].(string); ok && entityType != "" {
				entityData := buildEntityDataFromEntity(mainEntity, entityType)
				ctx["pluginDetailActions"] = pm.GetActionsForPlacement(entityType, "detail", entityData)
			}
		}

		// Compute plugin card/bulk actions for list pages (unfiltered)
		path := request.URL.Path
		switch {
		case strings.HasPrefix(path, "/resources"):
			ctx["pluginCardActions"] = pm.GetActionsForPlacement("resource", "card", nil)
			ctx["pluginBulkActions"] = pm.GetActionsForPlacement("resource", "bulk", nil)
		case strings.HasPrefix(path, "/notes"):
			ctx["pluginCardActions"] = pm.GetActionsForPlacement("note", "card", nil)
		case strings.HasPrefix(path, "/groups"):
			ctx["pluginCardActions"] = pm.GetActionsForPlacement("group", "card", nil)
			ctx["pluginBulkActions"] = pm.GetActionsForPlacement("group", "bulk", nil)
		}

		// For JSON responses, process shortcodes in Custom* fields since the
		// pongo2 template (and its {% process_shortcodes %} tag) won't execute.
		if strings.HasSuffix(request.URL.Path, ".json") ||
			strings.Contains(request.Header.Get("Accept"), constants.JSON) {
			processShortcodesForJSON(ctx, pm, appContext, request.Context())
		}

		return ctx
	}
}

// buildEntityDataFromEntity extracts filter-relevant fields from an entity for action matching.
func buildEntityDataFromEntity(entity any, entityType string) map[string]any {
	data := map[string]any{}
	switch entityType {
	case "resource":
		if r, ok := entity.(*models.Resource); ok {
			data["content_type"] = r.ContentType
		}
	case "group":
		if g, ok := entity.(*models.Group); ok && g.CategoryId != nil {
			data["category_id"] = *g.CategoryId
		}
	case "note":
		if n, ok := entity.(*models.Note); ok && n.NoteTypeId != nil {
			data["note_type_id"] = *n.NoteTypeId
		}
	}
	return data
}

// processShortcodesForJSON processes shortcode markup in Custom* fields of
// entity categories/types so that JSON API consumers (e.g., the lightbox)
// receive expanded HTML instead of raw [meta ...] shortcode text.
// Only called for JSON responses — HTML responses use the process_shortcodes template tag.
func processShortcodesForJSON(ctx pongo2.Context, pm *plugin_system.PluginManager, appCtx *application_context.MahresourcesContext, reqCtx context.Context) {
	mainEntity := ctx["mainEntity"]
	entityType, _ := ctx["mainEntityType"].(string)
	if mainEntity == nil || entityType == "" {
		return
	}

	var pluginRenderer shortcodes.PluginRenderer
	if pm != nil {
		pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
			return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity)
		}
	}

	var executor shortcodes.QueryExecutor
	if appCtx != nil {
		executor = template_filters.BuildQueryExecutor(appCtx)
	}

	switch entityType {
	case "resource":
		if r, ok := mainEntity.(*models.Resource); ok && r.ResourceCategory != nil {
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType: "resource",
				EntityID:   r.ID,
				Meta:       json.RawMessage(r.Meta),
				MetaSchema: r.ResourceCategory.MetaSchema,
				Entity:     r,
			}
			r.ResourceCategory.CustomHeader = shortcodes.Process(reqCtx, r.ResourceCategory.CustomHeader, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomSidebar = shortcodes.Process(reqCtx, r.ResourceCategory.CustomSidebar, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomSummary = shortcodes.Process(reqCtx, r.ResourceCategory.CustomSummary, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomAvatar = shortcodes.Process(reqCtx, r.ResourceCategory.CustomAvatar, metaCtx, pluginRenderer, executor)
		}
	case "group":
		if g, ok := mainEntity.(*models.Group); ok && g.Category != nil {
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType: "group",
				EntityID:   g.ID,
				Meta:       json.RawMessage(g.Meta),
				MetaSchema: g.Category.MetaSchema,
				Entity:     g,
			}
			g.Category.CustomHeader = shortcodes.Process(reqCtx, g.Category.CustomHeader, metaCtx, pluginRenderer, executor)
			g.Category.CustomSidebar = shortcodes.Process(reqCtx, g.Category.CustomSidebar, metaCtx, pluginRenderer, executor)
			g.Category.CustomSummary = shortcodes.Process(reqCtx, g.Category.CustomSummary, metaCtx, pluginRenderer, executor)
			g.Category.CustomAvatar = shortcodes.Process(reqCtx, g.Category.CustomAvatar, metaCtx, pluginRenderer, executor)
		}
	case "note":
		if n, ok := mainEntity.(*models.Note); ok && n.NoteType != nil {
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType: "note",
				EntityID:   n.ID,
				Meta:       json.RawMessage(n.Meta),
				Entity:     n,
			}
			n.NoteType.CustomHeader = shortcodes.Process(reqCtx, n.NoteType.CustomHeader, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomSidebar = shortcodes.Process(reqCtx, n.NoteType.CustomSidebar, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomSummary = shortcodes.Process(reqCtx, n.NoteType.CustomSummary, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomAvatar = shortcodes.Process(reqCtx, n.NoteType.CustomAvatar, metaCtx, pluginRenderer, executor)
		}
	}
}

func registerRoutes(router *mux.Router, appContext *application_context.MahresourcesContext) {
	for path, templateInfo := range templates {
		wrappedCtxFn := wrapContextWithPlugins(appContext, templateInfo.contextFn(appContext))

		router.Methods(templateInfo.method).Path(path).HandlerFunc(
			template_handlers.RenderTemplate(templateInfo.templateName, wrappedCtxFn),
		)

		router.Methods(templateInfo.method).Path(path + ".json").HandlerFunc(
			template_handlers.RenderTemplate(templateInfo.templateName, wrappedCtxFn),
		)

		router.Methods(templateInfo.method).Path(path + ".body").HandlerFunc(
			template_handlers.RenderTemplate(templateInfo.templateName, wrappedCtxFn),
		)
	}

	router.Methods(http.MethodGet).
		Path("/partials/autocompleter").
		HandlerFunc(template_handlers.
			RenderTemplate("partials/form/autocompleter.tpl", wrapContextWithPlugins(appContext, template_context_providers.PartialContextProvider(appContext))))

	router.Methods(http.MethodGet).Path("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/dashboard", http.StatusMovedPermanently)
	})

	basicGroupWriter := application_context.NewEntityWriter[models.Group](appContext)
	basicNoteWriter := application_context.NewEntityWriter[models.Note](appContext)
	basicResourceWriter := application_context.NewEntityWriter[models.Resource](appContext)
	basicTagWriter := application_context.NewEntityWriter[models.Tag](appContext)
	basicCategoryWriter := application_context.NewEntityWriter[models.Category](appContext)
	basicQueryWriter := application_context.NewEntityWriter[models.Query](appContext)
	basicRelationWriter := application_context.NewEntityWriter[models.GroupRelation](appContext)
	basicRelationTypeWriter := application_context.NewEntityWriter[models.GroupRelationType](appContext)
	basicNoteTypeWriter := application_context.NewEntityWriter[models.NoteType](appContext)
	basicSeriesWriter := application_context.NewEntityWriter[models.Series](appContext)

	router.Methods(http.MethodGet).Path("/v1/notes").HandlerFunc(api_handlers.GetNotesHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/notes/meta/keys").HandlerFunc(api_handlers.GetNoteMetaKeysHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/note").HandlerFunc(api_handlers.GetNoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note").HandlerFunc(api_handlers.GetAddNoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/delete").HandlerFunc(api_handlers.GetRemoveNoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Note](basicNoteWriter, "note"))
	router.Methods(http.MethodPost).Path("/v1/note/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Note](basicNoteWriter, "note"))
	router.Methods(http.MethodPost).Path("/v1/note/editMeta").HandlerFunc(api_handlers.GetEditMetaHandler(basicNoteWriter, "note"))
	router.Methods(http.MethodGet).Path("/v1/note/noteTypes").HandlerFunc(api_handlers.GetNoteTypesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/noteType").HandlerFunc(api_handlers.GetAddNoteTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/noteType/edit").HandlerFunc(api_handlers.GetAddNoteTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/noteType/delete").HandlerFunc(api_handlers.GetRemoveNoteTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/noteType/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.NoteType](basicNoteTypeWriter, "noteType"))
	router.Methods(http.MethodPost).Path("/v1/noteType/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.NoteType](basicNoteTypeWriter, "noteType"))

	// Note sharing routes
	router.Methods(http.MethodPost).Path("/v1/note/share").HandlerFunc(api_handlers.GetShareNoteHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/note/share").HandlerFunc(api_handlers.GetUnshareNoteHandler(appContext))

	// Note bulk operations
	router.Methods(http.MethodPost).Path("/v1/notes/addTags").HandlerFunc(api_handlers.GetAddTagsToNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/removeTags").HandlerFunc(api_handlers.GetRemoveTagsFromNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/addGroups").HandlerFunc(api_handlers.GetAddGroupsToNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/addMeta").HandlerFunc(api_handlers.GetAddMetaToNotesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/notes/delete").HandlerFunc(api_handlers.GetBulkDeleteNotesHandler(appContext))

	// Block API routes
	router.Methods(http.MethodGet).Path("/v1/note/blocks").HandlerFunc(api_handlers.GetBlocksHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/note/block").HandlerFunc(api_handlers.GetBlockHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/note/block/types").HandlerFunc(api_handlers.GetBlockTypesHandler())
	router.Methods(http.MethodPost).Path("/v1/note/block").HandlerFunc(api_handlers.CreateBlockHandler(appContext))
	router.Methods(http.MethodPut).Path("/v1/note/block").HandlerFunc(api_handlers.UpdateBlockContentHandler(appContext))
	router.Methods(http.MethodPatch).Path("/v1/note/block/state").HandlerFunc(api_handlers.UpdateBlockStateHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/note/block").HandlerFunc(api_handlers.DeleteBlockHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/block/delete").HandlerFunc(api_handlers.DeleteBlockHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/blocks/reorder").HandlerFunc(api_handlers.ReorderBlocksHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/blocks/rebalance").HandlerFunc(api_handlers.RebalanceBlocksHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/note/block/table/query").HandlerFunc(api_handlers.GetTableBlockQueryDataHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/note/block/calendar/events").HandlerFunc(api_handlers.GetCalendarBlockEventsHandler(appContext))

	router.Methods(http.MethodGet).Path("/v1/groups").HandlerFunc(api_handlers.GetGroupsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/groups/meta/keys").HandlerFunc(api_handlers.GetGroupMetaKeysHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/group").HandlerFunc(api_handlers.GetGroupHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/group/parents").HandlerFunc(api_handlers.GetGroupsParentsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/group/tree/children").HandlerFunc(api_handlers.GetGroupTreeChildrenHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/group/clone").HandlerFunc(api_handlers.GetDuplicateGroupHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/group").HandlerFunc(api_handlers.GetAddGroupHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/group/delete").HandlerFunc(api_handlers.GetRemoveGroupHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/addTags").HandlerFunc(api_handlers.GetAddTagsToGroupsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/removeTags").HandlerFunc(api_handlers.GetRemoveTagsFromGroupsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/addMeta").HandlerFunc(api_handlers.GetAddMetaToGroupsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/delete").HandlerFunc(api_handlers.GetBulkDeleteGroupsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/merge").HandlerFunc(api_handlers.GetMergeGroupsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/group/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Group](basicGroupWriter, "group"))
	router.Methods(http.MethodPost).Path("/v1/group/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Group](basicGroupWriter, "group"))
	router.Methods(http.MethodPost).Path("/v1/group/editMeta").HandlerFunc(api_handlers.GetEditMetaHandler(basicGroupWriter, "group"))

	router.Methods(http.MethodPost).Path("/v1/relation").HandlerFunc(api_handlers.GetAddRelationHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relation/delete").HandlerFunc(api_handlers.GetRemoveRelationHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relationType").HandlerFunc(api_handlers.GetAddGroupRelationTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relationType/delete").HandlerFunc(api_handlers.GetRemoveRelationTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relationType/edit").HandlerFunc(api_handlers.GetEditGroupRelationTypeHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/relationTypes").HandlerFunc(api_handlers.GetRelationTypesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relation/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.GroupRelation](basicRelationWriter, "relation"))
	router.Methods(http.MethodPost).Path("/v1/relation/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.GroupRelation](basicRelationWriter, "relation"))
	router.Methods(http.MethodPost).Path("/v1/relationType/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.GroupRelationType](basicRelationTypeWriter, "relationType"))
	router.Methods(http.MethodPost).Path("/v1/relationType/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.GroupRelationType](basicRelationTypeWriter, "relationType"))

	router.Methods(http.MethodGet).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resources").HandlerFunc(api_handlers.GetResourcesHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resources/meta/keys").HandlerFunc(api_handlers.GetResourceMetaKeysHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceUploadHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/local").HandlerFunc(api_handlers.GetResourceAddLocalHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/remote").HandlerFunc(api_handlers.GetResourceAddRemoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/delete").HandlerFunc(api_handlers.GetRemoveResourceHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/edit").HandlerFunc(api_handlers.GetResourceEditHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/view").HandlerFunc(api_handlers.GetResourceContentHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/preview").HandlerFunc(api_handlers.GetResourceThumbnailHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/recalculateDimensions").HandlerFunc(api_handlers.GetBulkCalculateDimensionsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/setDimensions").HandlerFunc(api_handlers.GetResourceSetDimensionsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/addTags").HandlerFunc(api_handlers.GetAddTagsToResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/addGroups").HandlerFunc(api_handlers.GetAddGroupsToResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/removeTags").HandlerFunc(api_handlers.GetRemoveTagsFromResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/replaceTags").HandlerFunc(api_handlers.GetReplaceTagsOfResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/addMeta").HandlerFunc(api_handlers.GetAddMetaToResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/delete").HandlerFunc(api_handlers.GetBulkDeleteResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/merge").HandlerFunc(api_handlers.GetMergeResourcesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/rotate").HandlerFunc(api_handlers.GetRotateResourceHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Resource](basicResourceWriter, "resource"))
	router.Methods(http.MethodPost).Path("/v1/resource/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Resource](basicResourceWriter, "resource"))
	router.Methods(http.MethodPost).Path("/v1/resource/editMeta").HandlerFunc(api_handlers.GetEditMetaHandler(basicResourceWriter, "resource"))

	// Version routes
	router.Methods(http.MethodGet).Path("/v1/resource/versions").
		HandlerFunc(api_handlers.GetListVersionsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/version").
		HandlerFunc(api_handlers.GetVersionHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/versions").
		HandlerFunc(api_handlers.GetUploadVersionHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/version/restore").
		HandlerFunc(api_handlers.GetRestoreVersionHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/resource/version").
		HandlerFunc(api_handlers.GetDeleteVersionHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/version/delete").
		HandlerFunc(api_handlers.GetDeleteVersionHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/version/file").
		HandlerFunc(api_handlers.GetVersionFileHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/versions/cleanup").
		HandlerFunc(api_handlers.GetCleanupVersionsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resources/versions/cleanup").
		HandlerFunc(api_handlers.GetBulkCleanupVersionsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/versions/compare").
		HandlerFunc(api_handlers.GetCompareVersionsHandler(appContext))

	// Series routes
	seriesReader, seriesWriter := appContext.SeriesCRUD()
	seriesFactory := api_handlers.NewCRUDHandlerFactory("series", "series", seriesReader, seriesWriter)
	router.Methods(http.MethodGet).Path("/v1/seriesList").HandlerFunc(seriesFactory.ListHandler())
	router.Methods(http.MethodPost).Path("/v1/series/create").HandlerFunc(seriesFactory.CreateHandler())
	router.Methods(http.MethodGet).Path("/v1/series").HandlerFunc(api_handlers.GetSeriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/series").HandlerFunc(api_handlers.GetUpdateSeriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/series/delete").HandlerFunc(api_handlers.GetDeleteSeriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/series/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Series](basicSeriesWriter, "series"))
	router.Methods(http.MethodPost).Path("/v1/resource/removeSeries").HandlerFunc(api_handlers.GetRemoveResourceFromSeriesHandler(appContext))

	// Tag routes using factory
	tagReader, tagWriter := appContext.TagCRUD()
	tagFactory := api_handlers.NewCRUDHandlerFactory("tag", "tags", tagReader, tagWriter)
	router.Methods(http.MethodGet).Path("/v1/tags").HandlerFunc(tagFactory.ListHandler())
	router.Methods(http.MethodPost).Path("/v1/tag").HandlerFunc(api_handlers.CreateTagHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/tag/delete").HandlerFunc(tagFactory.DeleteHandler())
	router.Methods(http.MethodPost).Path("/v1/tag/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Tag](basicTagWriter, "tag"))
	router.Methods(http.MethodPost).Path("/v1/tag/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Tag](basicTagWriter, "tag"))
	router.Methods(http.MethodPost).Path("/v1/tags/merge").HandlerFunc(api_handlers.GetMergeTagsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/tags/delete").HandlerFunc(api_handlers.GetBulkDeleteTagsHandler(appContext))

	// Category routes using factory
	categoryReader, categoryWriter := appContext.CategoryCRUD()
	categoryFactory := api_handlers.NewCRUDHandlerFactory("category", "categories", categoryReader, categoryWriter)
	router.Methods(http.MethodGet).Path("/v1/categories").HandlerFunc(categoryFactory.ListHandler())
	router.Methods(http.MethodPost).Path("/v1/category").HandlerFunc(api_handlers.CreateCategoryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/category/delete").HandlerFunc(api_handlers.GetRemoveCategoryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/category/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Category](basicCategoryWriter, "category"))
	router.Methods(http.MethodPost).Path("/v1/category/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Category](basicCategoryWriter, "category"))

	// Resource Category routes using factory
	resourceCategoryReader, resourceCategoryWriter := appContext.ResourceCategoryCRUD()
	resourceCategoryFactory := api_handlers.NewCRUDHandlerFactory("resourceCategory", "resourceCategories", resourceCategoryReader, resourceCategoryWriter)
	basicResourceCategoryWriter := application_context.NewEntityWriter[models.ResourceCategory](appContext)
	router.Methods(http.MethodGet).Path("/v1/resourceCategories").HandlerFunc(resourceCategoryFactory.ListHandler())
	router.Methods(http.MethodPost).Path("/v1/resourceCategory").HandlerFunc(api_handlers.CreateResourceCategoryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/delete").HandlerFunc(api_handlers.GetRemoveResourceCategoryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.ResourceCategory](basicResourceCategoryWriter, "resourceCategory"))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.ResourceCategory](basicResourceCategoryWriter, "resourceCategory"))

	// Query routes using factory
	queryReader, queryWriter := appContext.QueryCRUD()
	queryFactory := api_handlers.NewCRUDHandlerFactory("query", "queries", queryReader, queryWriter)
	router.Methods(http.MethodGet).Path("/v1/queries").HandlerFunc(queryFactory.ListHandler())
	router.Methods(http.MethodGet).Path("/v1/query").HandlerFunc(queryFactory.GetHandler())
	router.Methods(http.MethodPost).Path("/v1/query").HandlerFunc(api_handlers.CreateQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/query/delete").HandlerFunc(queryFactory.DeleteHandler())
	router.Methods(http.MethodGet).Path("/v1/query/schema").HandlerFunc(api_handlers.GetDatabaseSchemaHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/query/run").HandlerFunc(api_handlers.GetRunQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/query/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Query](basicQueryWriter, "query"))
	router.Methods(http.MethodPost).Path("/v1/query/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Query](basicQueryWriter, "query"))

	// MRQL routes
	router.Methods(http.MethodPost).Path("/v1/mrql").HandlerFunc(api_handlers.GetExecuteMRQLHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/validate").HandlerFunc(api_handlers.GetValidateMRQLHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/complete").HandlerFunc(api_handlers.GetCompleteMRQLHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetSavedMRQLQueriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetCreateSavedMRQLQueryHandler(appContext))
	router.Methods(http.MethodPut).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetUpdateSavedMRQLQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/saved/delete").HandlerFunc(api_handlers.GetDeleteSavedMRQLQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/saved/run").HandlerFunc(api_handlers.GetRunSavedMRQLQueryHandler(appContext))

	// Global Search
	router.Methods(http.MethodGet).Path("/v1/search").HandlerFunc(api_handlers.GetGlobalSearchHandler(appContext))

	// Download Queue (background remote downloads)
	router.Methods(http.MethodPost).Path("/v1/download/submit").HandlerFunc(api_handlers.GetDownloadSubmitHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/download/queue").HandlerFunc(api_handlers.GetDownloadQueueHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/cancel").HandlerFunc(api_handlers.GetDownloadCancelHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/pause").HandlerFunc(api_handlers.GetDownloadPauseHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/resume").HandlerFunc(api_handlers.GetDownloadResumeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/retry").HandlerFunc(api_handlers.GetDownloadRetryHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/download/events").HandlerFunc(api_handlers.GetDownloadEventsHandler(appContext))

	// Jobs routes (new canonical paths — download routes above kept as aliases)
	router.Methods(http.MethodPost).Path("/v1/jobs/download/submit").HandlerFunc(api_handlers.GetDownloadSubmitHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/jobs/queue").HandlerFunc(api_handlers.GetDownloadQueueHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/cancel").HandlerFunc(api_handlers.GetDownloadCancelHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/pause").HandlerFunc(api_handlers.GetDownloadPauseHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/resume").HandlerFunc(api_handlers.GetDownloadResumeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/retry").HandlerFunc(api_handlers.GetDownloadRetryHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/jobs/events").HandlerFunc(api_handlers.GetDownloadEventsHandler(appContext))

	// Plugin action routes
	router.Methods(http.MethodGet).Path("/v1/plugin/actions").HandlerFunc(api_handlers.GetPluginActionsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/action/run").HandlerFunc(api_handlers.GetActionRunHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/jobs/action/job").HandlerFunc(api_handlers.GetActionJobHandler(appContext))

	// Logs (read-only)
	router.Methods(http.MethodGet).Path("/v1/logs").HandlerFunc(api_handlers.GetLogEntriesHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/log").HandlerFunc(api_handlers.GetLogEntryHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/logs/entity").HandlerFunc(api_handlers.GetEntityHistoryHandler(appContext))

	// Admin stats routes
	router.Methods(http.MethodGet).Path("/v1/admin/server-stats").HandlerFunc(api_handlers.GetServerStatsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/admin/data-stats").HandlerFunc(api_handlers.GetDataStatsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/admin/data-stats/expensive").HandlerFunc(api_handlers.GetExpensiveStatsHandler(appContext))

	// Timeline routes
	router.Methods(http.MethodGet).Path("/v1/resources/timeline").HandlerFunc(api_handlers.GetResourceTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/notes/timeline").HandlerFunc(api_handlers.GetNoteTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/groups/timeline").HandlerFunc(api_handlers.GetGroupTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/tags/timeline").HandlerFunc(api_handlers.GetTagTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/categories/timeline").HandlerFunc(api_handlers.GetCategoryTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/queries/timeline").HandlerFunc(api_handlers.GetQueryTimelineHandler(appContext))

	// Plugin management API
	router.Methods(http.MethodGet).Path("/v1/plugins/manage").HandlerFunc(api_handlers.GetPluginsManageHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/enable").HandlerFunc(api_handlers.GetPluginEnableHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/disable").HandlerFunc(api_handlers.GetPluginDisableHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/settings").HandlerFunc(api_handlers.GetPluginSettingsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/purge-data").HandlerFunc(api_handlers.GetPluginPurgeDataHandler(appContext))

	// Plugin block render endpoint (must be before the catch-all)
	router.Methods(http.MethodGet).Path("/v1/plugins/{pluginName}/block/render").HandlerFunc(
		api_handlers.GetPluginBlockRenderHandler(appContext),
	)

	// Plugin display render endpoint (must be before the catch-all)
	router.Methods(http.MethodPost).Path("/v1/plugins/{pluginName}/display/render").HandlerFunc(
		api_handlers.GetPluginDisplayRenderHandler(appContext),
	)

	// Plugin JSON API endpoints (handler validates methods and returns JSON errors)
	router.PathPrefix("/v1/plugins/").HandlerFunc(api_handlers.PluginAPIHandler(appContext))

	// Plugin management page
	manageCtxFn := wrapContextWithPlugins(appContext, template_context_providers.PluginManageContextProvider(appContext))
	router.Methods(http.MethodGet).Path("/plugins/manage").
		HandlerFunc(template_handlers.RenderTemplate("managePlugins.tpl", manageCtxFn))

	// Plugin pages
	pm := appContext.PluginManager()
	if pm != nil {
		pluginCtxFn := wrapContextWithPlugins(appContext, template_context_providers.PluginPageContextProvider(pm))
		router.Methods(http.MethodGet, http.MethodPost).
			PathPrefix("/plugins/").
			HandlerFunc(template_handlers.RenderTemplate("pluginPage.tpl", pluginCtxFn))
	}
}
