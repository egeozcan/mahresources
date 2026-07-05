package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/mrql"
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

	"/templatePartial/new":  {template_context_providers.TemplatePartialCreateContextProvider, "createTemplatePartial.tpl", http.MethodGet},
	"/templatePartials":     {template_context_providers.TemplatePartialListContextProvider, "listTemplatePartials.tpl", http.MethodGet},
	"/templatePartial":      {template_context_providers.TemplatePartialContextProvider, "displayTemplatePartial.tpl", http.MethodGet},
	"/templatePartial/edit": {template_context_providers.TemplatePartialCreateContextProvider, "createTemplatePartial.tpl", http.MethodGet},

	"/resource/new":      {template_context_providers.ResourceCreateContextProvider, "createResource.tpl", http.MethodGet},
	"/resources":         {template_context_providers.ResourceListContextProvider, "listResources.tpl", http.MethodGet},
	"/resources/details": {template_context_providers.ResourceListContextProvider, "listResourcesDetails.tpl", http.MethodGet},
	"/resources/simple":  {template_context_providers.ResourceListContextProvider, "listResourcesSimple.tpl", http.MethodGet},
	"/resource":          {template_context_providers.ResourceContextProvider, "displayResource.tpl", http.MethodGet},
	"/resource/edit":     {template_context_providers.ResourceCreateContextProvider, "createResource.tpl", http.MethodGet},
	"/resource/compare":  {template_context_providers.CompareContextProvider, "compare.tpl", http.MethodGet},

	"/series": {template_context_providers.SeriesContextProvider, "displaySeries.tpl", http.MethodGet},

	"/group/new":     {template_context_providers.GroupCreateContextProvider, "createGroup.tpl", http.MethodGet},
	"/groups":        {template_context_providers.GroupsListContextProvider, "listGroups.tpl", http.MethodGet},
	"/groups/text":   {template_context_providers.GroupsListContextProvider, "listGroupsText.tpl", http.MethodGet},
	"/group":         {template_context_providers.GroupContextProvider, "displayGroup.tpl", http.MethodGet},
	"/group/compare": {template_context_providers.GroupCompareContextProvider, "groupCompare.tpl", http.MethodGet},
	"/group/edit":    {template_context_providers.GroupCreateContextProvider, "createGroup.tpl", http.MethodGet},
	"/group/tree":    {template_context_providers.GroupTreeContextProvider, "displayGroupTree.tpl", http.MethodGet},

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
	"/admin/users":    {template_context_providers.AdminUsersContextProvider, "adminUsers.tpl", http.MethodGet},
	"/account":        {template_context_providers.AccountContextProvider, "account.tpl", http.MethodGet},
	"/admin/export":   {template_context_providers.AdminExportContextProvider, "adminExport.tpl", http.MethodGet},
	"/admin/import":   {template_context_providers.AdminImportContextProvider, "adminImport.tpl", http.MethodGet},
	"/admin/shares":   {template_context_providers.AdminSharesContextProvider, "adminShares.tpl", http.MethodGet}, // BH-035
	"/admin/settings": {template_context_providers.AdminSettingsContextProvider, "adminSettings.tpl", http.MethodGet},

	"/mrql": {template_context_providers.MRQLContextProvider, "mrql.tpl", http.MethodGet},
}

func wrapContextWithPlugins(appContext *application_context.MahresourcesContext, ctxFn func(request *http.Request) pongo2.Context) func(request *http.Request) pongo2.Context {
	pm := appContext.PluginManager()
	return func(request *http.Request) pongo2.Context {
		ctx := ctxFn(request)

		// Always set — needed for [mrql] shortcodes even without plugins
		ctx["_appContext"] = appContext
		ctx["_requestContext"] = request.Context()

		// Expose the authenticated user to templates (nav avatar / logout). The
		// implicit super-user used when auth is disabled is intentionally not
		// surfaced, so the no-auth UI is unchanged.
		ctx["authEnabled"] = appContext.AuthEnabled()
		if p := auth.PrincipalFromContext(request.Context()); p != nil && !p.SuperUser {
			ctx["currentUser"] = p
		}
		// CSRF synchronizer token for the page (meta tag + form fields). Empty
		// when auth is off or for Bearer requests, where CSRF is not enforced.
		ctx["csrfToken"] = auth.CSRFTokenFromContext(request.Context())

		// BH-036: expose the export-retention window to every template context so
		// the admin-export helper text and the per-job expiry label in the
		// downloadCockpit can render consistent values without a bespoke provider
		// on each route. The ms value is consumed by downloadCockpit.js; the
		// human-readable string is rendered directly in adminExport.tpl.
		retention := appContext.Settings().ExportRetention()
		ctx["exportRetention"] = retention.String()
		ctx["exportRetentionMs"] = retention.Milliseconds()

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

// resolveEntityScope computes the three scope IDs (scope, parent, root) for an entity.
// For groups, the caller should set ScopeGroupID = group.ID after this returns.
// ownerID is the entity's OwnerId pointer.
func resolveEntityScope(appCtx *application_context.MahresourcesContext, entityType string, ownerID *uint) (scopeID, parentID, rootID uint) {
	if entityType == "group" {
		// For groups, scope is self (set by caller). Parent = owner, root = walk chain.
		if ownerID != nil && *ownerID > 0 {
			parentID = *ownerID
		} else {
			parentID = mrql.UnresolvedScopeSentinel
		}
		// rootID will be resolved from the group's own ID by the caller
		return 0, parentID, mrql.UnresolvedScopeSentinel
	}
	// Resources and notes: scope = owner group
	if ownerID != nil && *ownerID > 0 {
		scopeID = *ownerID
		parentID = appCtx.ResolveParentScopeID(*ownerID)
		rootID = appCtx.ResolveRootScopeID(*ownerID)
	} else {
		scopeID = mrql.UnresolvedScopeSentinel
		parentID = mrql.UnresolvedScopeSentinel
		rootID = mrql.UnresolvedScopeSentinel
	}
	return scopeID, parentID, rootID
}

// processShortcodesForJSON processes shortcode markup in Custom* fields of
// entity categories/types so that JSON API consumers (e.g., the lightbox)
// receive expanded HTML instead of raw [meta ...] shortcode text.
// Only called for JSON responses — HTML responses use the process_shortcodes template tag.
func processShortcodesForJSON(ctx pongo2.Context, pm *plugin_system.PluginManager, appCtx *application_context.MahresourcesContext, reqCtx context.Context) {
	reqCtx = plugin_system.WithMRQLCache(reqCtx)
	reqCtx = shortcodes.WithPartialResolver(reqCtx, template_filters.BuildPartialResolver(appCtx))
	mainEntity := ctx["mainEntity"]
	entityType, _ := ctx["mainEntityType"].(string)
	if mainEntity == nil || entityType == "" {
		return
	}

	var pluginRenderer shortcodes.PluginRenderer
	if pm != nil {
		pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
			return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
		}
	}

	var executor shortcodes.QueryExecutor
	if appCtx != nil {
		executor = template_filters.BuildQueryExecutor(appCtx)
	}

	switch entityType {
	case "resource":
		if r, ok := mainEntity.(*models.Resource); ok && r.ResourceCategory != nil {
			scopeID, parentID, rootID := resolveEntityScope(appCtx, "resource", r.OwnerId)
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType:    "resource",
				EntityID:      r.ID,
				Meta:          json.RawMessage(r.Meta),
				MetaSchema:    r.ResourceCategory.MetaSchema,
				Entity:        r,
				ScopeGroupID:  scopeID,
				ParentGroupID: parentID,
				RootGroupID:   rootID,
			}
			r.ResourceCategory.CustomHeader = shortcodes.Process(reqCtx, r.ResourceCategory.CustomHeader, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomSidebar = shortcodes.Process(reqCtx, r.ResourceCategory.CustomSidebar, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomSummary = shortcodes.Process(reqCtx, r.ResourceCategory.CustomSummary, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomAvatar = shortcodes.Process(reqCtx, r.ResourceCategory.CustomAvatar, metaCtx, pluginRenderer, executor)
			r.ResourceCategory.CustomCSS = shortcodes.Process(reqCtx, r.ResourceCategory.CustomCSS, metaCtx, pluginRenderer, executor)
		}
	case "group":
		if g, ok := mainEntity.(*models.Group); ok && g.Category != nil {
			_, parentID, _ := resolveEntityScope(appCtx, "group", g.OwnerId)
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType:    "group",
				EntityID:      g.ID,
				Meta:          json.RawMessage(g.Meta),
				MetaSchema:    g.Category.MetaSchema,
				Entity:        g,
				ScopeGroupID:  g.ID,
				ParentGroupID: parentID,
				RootGroupID:   appCtx.ResolveRootScopeID(g.ID),
			}
			g.Category.CustomHeader = shortcodes.Process(reqCtx, g.Category.CustomHeader, metaCtx, pluginRenderer, executor)
			g.Category.CustomSidebar = shortcodes.Process(reqCtx, g.Category.CustomSidebar, metaCtx, pluginRenderer, executor)
			g.Category.CustomSummary = shortcodes.Process(reqCtx, g.Category.CustomSummary, metaCtx, pluginRenderer, executor)
			g.Category.CustomAvatar = shortcodes.Process(reqCtx, g.Category.CustomAvatar, metaCtx, pluginRenderer, executor)
			g.Category.CustomCSS = shortcodes.Process(reqCtx, g.Category.CustomCSS, metaCtx, pluginRenderer, executor)
		}
	case "note":
		if n, ok := mainEntity.(*models.Note); ok && n.NoteType != nil {
			scopeID, parentID, rootID := resolveEntityScope(appCtx, "note", n.OwnerId)
			metaCtx := shortcodes.MetaShortcodeContext{
				EntityType:    "note",
				EntityID:      n.ID,
				Meta:          json.RawMessage(n.Meta),
				Entity:        n,
				ScopeGroupID:  scopeID,
				ParentGroupID: parentID,
				RootGroupID:   rootID,
			}
			n.NoteType.CustomHeader = shortcodes.Process(reqCtx, n.NoteType.CustomHeader, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomSidebar = shortcodes.Process(reqCtx, n.NoteType.CustomSidebar, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomSummary = shortcodes.Process(reqCtx, n.NoteType.CustomSummary, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomAvatar = shortcodes.Process(reqCtx, n.NoteType.CustomAvatar, metaCtx, pluginRenderer, executor)
			n.NoteType.CustomCSS = shortcodes.Process(reqCtx, n.NoteType.CustomCSS, metaCtx, pluginRenderer, executor)
		}
	}
}

func registerRoutes(router *mux.Router, appContext *application_context.MahresourcesContext) {
	for path, templateInfo := range templates {
		info := templateInfo
		// Build the pongo2 context per request against a principal-scoped context
		// so template list/detail pages are confined to a group-limited user's
		// subtree. The template set itself is still compiled once (inside
		// RenderTemplate); only the lightweight context builder runs per request.
		scopedCtxFn := func(request *http.Request) pongo2.Context {
			sc := scopedCtx(appContext, request)
			return wrapContextWithPlugins(sc, info.contextFn(sc))(request)
		}

		router.Methods(info.method).Path(path).HandlerFunc(
			template_handlers.RenderTemplate(info.templateName, scopedCtxFn),
		)

		router.Methods(info.method).Path(path + ".json").HandlerFunc(
			template_handlers.RenderTemplate(info.templateName, scopedCtxFn),
		)

		router.Methods(info.method).Path(path + ".body").HandlerFunc(
			template_handlers.RenderTemplate(info.templateName, scopedCtxFn),
		)
	}

	router.Methods(http.MethodGet).
		Path("/partials/autocompleter").
		HandlerFunc(template_handlers.
			RenderTemplate("partials/form/autocompleter.tpl", wrapContextWithPlugins(appContext, template_context_providers.PartialContextProvider(appContext))))

	// Hover-card preview fragment (Phase 6 item 3). Built against a principal-
	// scoped context so a group-limited user cannot preview entities outside its
	// subtree (fail-closed: out-of-scope IDs resolve to a "Preview unavailable"
	// fragment). GET → capRead via the shared authorization middleware.
	router.Methods(http.MethodGet).
		Path("/hovercard").
		HandlerFunc(template_handlers.RenderTemplate("partials/hovercard.tpl", func(request *http.Request) pongo2.Context {
			sc := scopedCtx(appContext, request)
			return wrapContextWithPlugins(sc, template_context_providers.HoverCardContextProvider(sc))(request)
		}))

	router.Methods(http.MethodGet).Path("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/dashboard", http.StatusMovedPermanently)
	})

	// Authentication routes. The login page is a standalone template (it must be
	// reachable when logged out, so it is not wrapped with plugin context).
	// One shared rate limiter throttles failed logins from both the web form and
	// the JSON API (no-op when -login-max-attempts is 0).
	loginLimiter := newLoginRateLimiter(appContext.LoginRateLimit(), appContext.LoginRateWindow())
	router.Methods(http.MethodGet).Path("/login").HandlerFunc(
		template_handlers.RenderTemplate("login.tpl", template_context_providers.LoginContextProvider(appContext)))
	router.Methods(http.MethodPost).Path("/login").HandlerFunc(LoginSubmitHandler(appContext, loginLimiter))
	router.Methods(http.MethodGet, http.MethodPost).Path("/logout").HandlerFunc(LogoutHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/auth/login").HandlerFunc(APILoginHandler(appContext, loginLimiter))
	router.Methods(http.MethodPost).Path("/v1/auth/logout").HandlerFunc(APILogoutHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/auth/me").HandlerFunc(APIMeHandler(appContext))

	// User administration (admin only — enforced by the authz policy).
	router.Methods(http.MethodGet).Path("/v1/users").HandlerFunc(api_handlers.GetUsersHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/users").HandlerFunc(api_handlers.CreateUserHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/user").HandlerFunc(api_handlers.GetUserHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/user").HandlerFunc(api_handlers.UpdateUserHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/user/delete").HandlerFunc(api_handlers.DeleteUserHandler(appContext))

	// Self-service account management (any authenticated user, including guests).
	router.Methods(http.MethodPost).Path("/v1/account/password").HandlerFunc(api_handlers.ChangeOwnPasswordHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/account/tokens").HandlerFunc(api_handlers.ListOwnTokensHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/account/tokens").HandlerFunc(api_handlers.CreateOwnTokenHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/account/tokens/delete").HandlerFunc(api_handlers.RevokeOwnTokenHandler(appContext))
	// Per-user UI settings (server-backed replacement for localStorage prefs).
	router.Methods(http.MethodGet).Path("/v1/account/settings").HandlerFunc(api_handlers.GetUserSettingsHandler(appContext))
	router.Methods(http.MethodPut).Path("/v1/account/settings/{key}").HandlerFunc(api_handlers.SetUserSettingHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/account/settings/{key}").HandlerFunc(api_handlers.DeleteUserSettingHandler(appContext))

	basicTagWriter := application_context.NewEntityWriter[models.Tag](appContext)
	basicCategoryWriter := application_context.NewEntityWriter[models.Category](appContext)
	basicQueryWriter := application_context.NewEntityWriter[models.Query](appContext)
	basicRelationWriter := application_context.NewEntityWriter[models.GroupRelation](appContext)
	basicRelationTypeWriter := application_context.NewEntityWriter[models.GroupRelationType](appContext)
	basicNoteTypeWriter := application_context.NewEntityWriter[models.NoteType](appContext)
	basicSeriesWriter := application_context.NewEntityWriter[models.Series](appContext)

	router.Methods(http.MethodGet).Path("/v1/notes").HandlerFunc(scopedAPI(appContext, api_handlers.GetNotesHandler))
	router.Methods(http.MethodGet).Path("/v1/notes/meta/keys").HandlerFunc(scopedAPI(appContext, api_handlers.GetNoteMetaKeysHandler))
	router.Methods(http.MethodGet).Path("/v1/note").HandlerFunc(scopedAPI(appContext, api_handlers.GetNoteHandler))
	router.Methods(http.MethodPost).Path("/v1/note").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddNoteHandler))
	router.Methods(http.MethodPost).Path("/v1/note/delete").HandlerFunc(scopedAPI(appContext, api_handlers.GetRemoveNoteHandler))
	router.Methods(http.MethodPost).Path("/v1/note/editName").HandlerFunc(scopedEditName[models.Note](appContext, "note"))
	router.Methods(http.MethodPost).Path("/v1/note/editDescription").HandlerFunc(scopedEditDescription[models.Note](appContext, "note"))
	router.Methods(http.MethodPost).Path("/v1/note/editMeta").HandlerFunc(scopedEditMeta[models.Note](appContext, "note"))
	router.Methods(http.MethodGet).Path("/v1/note/noteTypes").HandlerFunc(api_handlers.GetNoteTypesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/noteType").HandlerFunc(api_handlers.GetAddNoteTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/noteType/edit").HandlerFunc(api_handlers.GetAddNoteTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/noteType/delete").HandlerFunc(api_handlers.GetRemoveNoteTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/noteType/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.NoteType](basicNoteTypeWriter, "noteType"))
	router.Methods(http.MethodPost).Path("/v1/noteType/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.NoteType](basicNoteTypeWriter, "noteType"))

	// Note sharing routes
	router.Methods(http.MethodPost).Path("/v1/note/share").HandlerFunc(scopedAPI(appContext, api_handlers.GetShareNoteHandler))
	router.Methods(http.MethodDelete).Path("/v1/note/share").HandlerFunc(scopedAPI(appContext, api_handlers.GetUnshareNoteHandler))
	// BH-035: centralized /admin/shares dashboard bulk-revoke endpoint. Accepts
	// form-encoded ids=<noteId> repeats; redirects browser-form consumers back
	// to /admin/shares, answers JSON for Accept: application/json callers.
	router.Methods(http.MethodPost).Path("/v1/admin/shares/bulk-revoke").HandlerFunc(api_handlers.GetBulkUnshareNotesHandler(appContext))

	// Note bulk operations
	router.Methods(http.MethodPost).Path("/v1/notes/addTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddTagsToNotesHandler))
	router.Methods(http.MethodPost).Path("/v1/notes/removeTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetRemoveTagsFromNotesHandler))
	router.Methods(http.MethodPost).Path("/v1/notes/addGroups").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddGroupsToNotesHandler))
	router.Methods(http.MethodPost).Path("/v1/notes/addMeta").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddMetaToNotesHandler))
	router.Methods(http.MethodPost).Path("/v1/notes/delete").HandlerFunc(scopedAPI(appContext, api_handlers.GetBulkDeleteNotesHandler))

	// Block API routes
	router.Methods(http.MethodGet).Path("/v1/note/blocks").HandlerFunc(scopedAPI(appContext, api_handlers.GetBlocksHandler))
	router.Methods(http.MethodGet).Path("/v1/note/block").HandlerFunc(scopedAPI(appContext, api_handlers.GetBlockHandler))
	router.Methods(http.MethodGet).Path("/v1/note/block/types").HandlerFunc(api_handlers.GetBlockTypesHandler())
	router.Methods(http.MethodPost).Path("/v1/note/block").HandlerFunc(scopedAPI(appContext, api_handlers.CreateBlockHandler))
	router.Methods(http.MethodPut).Path("/v1/note/block").HandlerFunc(scopedAPI(appContext, api_handlers.UpdateBlockContentHandler))
	router.Methods(http.MethodPatch).Path("/v1/note/block/state").HandlerFunc(scopedAPI(appContext, api_handlers.UpdateBlockStateHandler))
	router.Methods(http.MethodDelete).Path("/v1/note/block").HandlerFunc(scopedAPI(appContext, api_handlers.DeleteBlockHandler))
	router.Methods(http.MethodPost).Path("/v1/note/block/delete").HandlerFunc(scopedAPI(appContext, api_handlers.DeleteBlockHandler))
	router.Methods(http.MethodPost).Path("/v1/note/blocks/reorder").HandlerFunc(scopedAPI(appContext, api_handlers.ReorderBlocksHandler))
	router.Methods(http.MethodPost).Path("/v1/note/blocks/rebalance").HandlerFunc(scopedAPI(appContext, api_handlers.RebalanceBlocksHandler))
	router.Methods(http.MethodGet).Path("/v1/note/block/table/query").HandlerFunc(scopedAPI(appContext, api_handlers.GetTableBlockQueryDataHandler))
	router.Methods(http.MethodGet).Path("/v1/note/block/calendar/events").HandlerFunc(scopedAPI(appContext, api_handlers.GetCalendarBlockEventsHandler))

	router.Methods(http.MethodGet).Path("/v1/groups").HandlerFunc(scopedAPI(appContext, api_handlers.GetGroupsHandler))
	router.Methods(http.MethodGet).Path("/v1/groups/meta/keys").HandlerFunc(scopedAPI(appContext, api_handlers.GetGroupMetaKeysHandler))
	router.Methods(http.MethodGet).Path("/v1/group").HandlerFunc(scopedAPI(appContext, api_handlers.GetGroupHandler))
	router.Methods(http.MethodGet).Path("/v1/group/parents").HandlerFunc(scopedAPI(appContext, api_handlers.GetGroupsParentsHandler))
	router.Methods(http.MethodGet).Path("/v1/group/tree/children").HandlerFunc(scopedAPI(appContext, api_handlers.GetGroupTreeChildrenHandler))
	router.Methods(http.MethodPost).Path("/v1/group/clone").HandlerFunc(scopedAPI(appContext, api_handlers.GetDuplicateGroupHandler))
	router.Methods(http.MethodPost).Path("/v1/group").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddGroupHandler))
	router.Methods(http.MethodPost).Path("/v1/group/delete").HandlerFunc(scopedAPI(appContext, api_handlers.GetRemoveGroupHandler))
	router.Methods(http.MethodPost).Path("/v1/groups/addTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddTagsToGroupsHandler))
	router.Methods(http.MethodPost).Path("/v1/groups/removeTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetRemoveTagsFromGroupsHandler))
	router.Methods(http.MethodPost).Path("/v1/groups/addMeta").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddMetaToGroupsHandler))
	router.Methods(http.MethodPost).Path("/v1/groups/delete").HandlerFunc(scopedAPI(appContext, api_handlers.GetBulkDeleteGroupsHandler))
	router.Methods(http.MethodPost).Path("/v1/groups/merge").HandlerFunc(scopedAPI(appContext, api_handlers.GetMergeGroupsHandler))
	router.Methods(http.MethodPost).Path("/v1/group/editName").HandlerFunc(scopedEditName[models.Group](appContext, "group"))
	router.Methods(http.MethodPost).Path("/v1/group/editDescription").HandlerFunc(scopedEditDescription[models.Group](appContext, "group"))
	router.Methods(http.MethodPost).Path("/v1/group/editMeta").HandlerFunc(scopedEditMeta[models.Group](appContext, "group"))

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

	router.Methods(http.MethodGet).Path("/v1/resource").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceHandler))
	router.Methods(http.MethodGet).Path("/v1/resource/suggestedTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetSuggestedTagsHandler))
	router.Methods(http.MethodGet).Path("/v1/resources").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourcesHandler))
	router.Methods(http.MethodGet).Path("/v1/resources/meta/keys").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceMetaKeysHandler))
	uploadSize := func() int64 { return appContext.Settings().MaxUploadSize() }
	router.Methods(http.MethodPost).Path("/v1/resource").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api_handlers.GetResourceUploadHandler(scopedCtx(appContext, r), uploadSize)(w, r)
	})
	router.Methods(http.MethodPost).Path("/v1/resource/local").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceAddLocalHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/remote").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceAddRemoteHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/delete").HandlerFunc(scopedAPI(appContext, api_handlers.GetRemoveResourceHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/edit").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceEditHandler))
	router.Methods(http.MethodGet).Path("/v1/resource/view").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceContentHandler))
	router.Methods(http.MethodGet).Path("/v1/resource/preview").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceThumbnailHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/preview").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api_handlers.PostResourceCustomThumbnailHandler(scopedCtx(appContext, r), uploadSize)(w, r)
	})
	router.Methods(http.MethodDelete).Path("/v1/resource/preview").HandlerFunc(scopedAPI(appContext, api_handlers.DeleteResourceCustomThumbnailHandler))
	// Some browser environments cannot send DELETE from forms; provide a POST alias.
	router.Methods(http.MethodPost).Path("/v1/resource/preview/clear").HandlerFunc(scopedAPI(appContext, api_handlers.DeleteResourceCustomThumbnailHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/recalculateDimensions").HandlerFunc(scopedAPI(appContext, api_handlers.GetBulkCalculateDimensionsHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/setDimensions").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceSetDimensionsHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/addTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddTagsToResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/addGroups").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddGroupsToResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/removeTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetRemoveTagsFromResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/replaceTags").HandlerFunc(scopedAPI(appContext, api_handlers.GetReplaceTagsOfResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/addMeta").HandlerFunc(scopedAPI(appContext, api_handlers.GetAddMetaToResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/delete").HandlerFunc(scopedAPI(appContext, api_handlers.GetBulkDeleteResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/merge").HandlerFunc(scopedAPI(appContext, api_handlers.GetMergeResourcesHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/rotate").HandlerFunc(scopedAPI(appContext, api_handlers.GetRotateResourceHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/crop").HandlerFunc(scopedAPI(appContext, api_handlers.GetCropResourceHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/trim").HandlerFunc(scopedAPI(appContext, api_handlers.GetTrimVideoHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/editName").HandlerFunc(scopedEditName[models.Resource](appContext, "resource"))
	router.Methods(http.MethodPost).Path("/v1/resource/editDescription").HandlerFunc(scopedEditDescription[models.Resource](appContext, "resource"))
	router.Methods(http.MethodPost).Path("/v1/resource/editMeta").HandlerFunc(scopedEditMeta[models.Resource](appContext, "resource"))

	// Version routes
	router.Methods(http.MethodGet).Path("/v1/resource/versions").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetListVersionsHandler))
	router.Methods(http.MethodGet).Path("/v1/resource/version").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetVersionHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/versions").
		HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			api_handlers.GetUploadVersionHandler(scopedCtx(appContext, r), uploadSize)(w, r)
		})
	router.Methods(http.MethodPost).Path("/v1/resource/version/restore").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetRestoreVersionHandler))
	router.Methods(http.MethodDelete).Path("/v1/resource/version").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetDeleteVersionHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/version/delete").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetDeleteVersionHandler))
	router.Methods(http.MethodGet).Path("/v1/resource/version/file").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetVersionFileHandler))
	router.Methods(http.MethodPost).Path("/v1/resource/versions/cleanup").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetCleanupVersionsHandler))
	router.Methods(http.MethodPost).Path("/v1/resources/versions/cleanup").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetBulkCleanupVersionsHandler))
	router.Methods(http.MethodGet).Path("/v1/resource/versions/compare").
		HandlerFunc(scopedAPI(appContext, api_handlers.GetCompareVersionsHandler))

	// Series routes
	seriesReader, seriesWriter := appContext.SeriesCRUD()
	seriesFactory := api_handlers.NewCRUDHandlerFactory("series", "series", seriesReader, seriesWriter)
	router.Methods(http.MethodGet).Path("/v1/seriesList").HandlerFunc(seriesFactory.ListHandler())
	// Series is the only entity whose CREATE routes through the generic
	// CRUDHandlerFactory (whose writer captured ctx.db at startup, so a create
	// would stamp CreatedByUserId NULL). Build a request-scoped writer per request
	// so the create runs on a db carrying the acting-user context. List/Get/Delete
	// keep using the startup factory.
	router.Methods(http.MethodPost).Path("/v1/series/create").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scopedReader, scopedWriter := scopedCtx(appContext, r).SeriesCRUD()
		api_handlers.NewCRUDHandlerFactory("series", "series", scopedReader, scopedWriter).CreateHandler()(w, r)
	})
	// Scoped: the series detail preloads its Resources, which must be confined to
	// the caller's subtree (a group-limited principal must not read another
	// tenant's resources through a shared series).
	router.Methods(http.MethodGet).Path("/v1/series").HandlerFunc(scopedAPI(appContext, api_handlers.GetSeriesHandler))
	router.Methods(http.MethodPost).Path("/v1/series").HandlerFunc(api_handlers.GetUpdateSeriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/series/delete").HandlerFunc(api_handlers.GetDeleteSeriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/series/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Series](basicSeriesWriter, "series"))
	router.Methods(http.MethodPost).Path("/v1/resource/removeSeries").HandlerFunc(api_handlers.GetRemoveResourceFromSeriesHandler(appContext))

	// Tag routes using factory
	tagReader, tagWriter := appContext.TagCRUD()
	tagFactory := api_handlers.NewCRUDHandlerFactory("tag", "tags", tagReader, tagWriter)
	router.Methods(http.MethodGet).Path("/v1/tags").HandlerFunc(tagFactory.ListHandler())
	// Lean typeahead path: same TagQuery scope as /v1/tags but skips the pagination
	// COUNT (a second full-table scan per keystroke). Tags are global labels, so this
	// is intentionally unscoped, like the rest of the tag routes.
	router.Methods(http.MethodGet).Path("/v1/tags/suggest").HandlerFunc(api_handlers.GetTagsHandler(appContext))
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

	// Template Partial routes (admin-only writes via isTaxonomyPath; reads open)
	basicTemplatePartialWriter := application_context.NewEntityWriter[models.TemplatePartial](appContext)
	router.Methods(http.MethodGet).Path("/v1/templatePartials").HandlerFunc(api_handlers.GetTemplatePartialsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/templatePartial").HandlerFunc(api_handlers.GetAddTemplatePartialHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/templatePartial/edit").HandlerFunc(api_handlers.GetAddTemplatePartialHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/templatePartial/delete").HandlerFunc(api_handlers.GetRemoveTemplatePartialHandler(appContext))
	// No /editName route: a partial's Name must stay kebab-case, which the
	// generic EntityWriter.UpdateName (ValidateEntityName only) does not enforce.
	// Names change through the full create/update path, which validates.
	router.Methods(http.MethodPost).Path("/v1/templatePartial/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.TemplatePartial](basicTemplatePartialWriter, "templatePartial"))

	// Starter template presets (static embedded bundles; read-only, open)
	router.Methods(http.MethodGet).Path("/v1/templatePresets").HandlerFunc(api_handlers.GetTemplatePresetsHandler())

	// Query routes using factory
	queryReader, queryWriter := appContext.QueryCRUD()
	queryFactory := api_handlers.NewCRUDHandlerFactory("query", "queries", queryReader, queryWriter)
	router.Methods(http.MethodGet).Path("/v1/queries").HandlerFunc(queryFactory.ListHandler())
	router.Methods(http.MethodGet).Path("/v1/query").HandlerFunc(queryFactory.GetHandler())
	router.Methods(http.MethodPost).Path("/v1/query").HandlerFunc(api_handlers.CreateQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/query/delete").HandlerFunc(queryFactory.DeleteHandler())
	router.Methods(http.MethodGet).Path("/v1/query/schema").HandlerFunc(api_handlers.GetDatabaseSchemaHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/query/run").HandlerFunc(scopedAPI(appContext, api_handlers.GetRunQueryHandler))
	router.Methods(http.MethodPost).Path("/v1/query/editName").HandlerFunc(api_handlers.GetEditEntityNameHandler[models.Query](basicQueryWriter, "query"))
	router.Methods(http.MethodPost).Path("/v1/query/editDescription").HandlerFunc(api_handlers.GetEditEntityDescriptionHandler[models.Query](basicQueryWriter, "query"))

	// MRQL routes
	router.Methods(http.MethodPost).Path("/v1/mrql").HandlerFunc(scopedAPI(appContext, api_handlers.GetExecuteMRQLHandler))
	router.Methods(http.MethodPost).Path("/v1/mrql/explain").HandlerFunc(scopedAPI(appContext, api_handlers.GetExplainMRQLHandler))
	router.Methods(http.MethodGet, http.MethodPost).Path("/v1/mrql/export").HandlerFunc(scopedAPI(appContext, api_handlers.GetExportMRQLHandler))
	router.Methods(http.MethodPost).Path("/v1/mrql/validate").HandlerFunc(api_handlers.GetValidateMRQLHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/complete").HandlerFunc(api_handlers.GetCompleteMRQLHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/generate").HandlerFunc(api_handlers.GetGenerateMRQLHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetSavedMRQLQueriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetCreateSavedMRQLQueryHandler(appContext))
	router.Methods(http.MethodPut).Path("/v1/mrql/saved").HandlerFunc(api_handlers.GetUpdateSavedMRQLQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/saved/delete").HandlerFunc(api_handlers.GetDeleteSavedMRQLQueryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/mrql/saved/run").HandlerFunc(scopedAPI(appContext, api_handlers.GetRunSavedMRQLQueryHandler))

	// Shortcode editor tooling (docs registry powers lint + autocomplete)
	router.Methods(http.MethodGet).Path("/v1/shortcodes/docs").HandlerFunc(api_handlers.GetShortcodeDocsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/shortcodes/lint").HandlerFunc(api_handlers.GetShortcodeLintHandler(appContext))

	// On-demand render for [lazy]/[details] deferred blocks. Semantically a read
	// (listed in isReadViaPost → capRead, CSRF-exempt) and scoped so out-of-subtree
	// entities fail closed; the sealed token authenticates the server-authored body and keeps it opaque on the page.
	router.Methods(http.MethodPost).Path("/v1/shortcodes/deferred").HandlerFunc(scopedAPI(appContext, api_handlers.GetDeferredRenderHandler))

	// Live template preview — mounted per carrier so the existing path-prefix
	// authorization applies (category/resourceCategory → admin, noteType → editor).
	router.Methods(http.MethodPost).Path("/v1/category/previewTemplate").HandlerFunc(api_handlers.GetPreviewTemplateHandler(appContext, "group"))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/previewTemplate").HandlerFunc(api_handlers.GetPreviewTemplateHandler(appContext, "resource"))
	router.Methods(http.MethodPost).Path("/v1/noteType/previewTemplate").HandlerFunc(api_handlers.GetPreviewTemplateHandler(appContext, "note"))

	// Natural-language template generation — mounted per carrier so the existing
	// path-prefix authorization applies (category/resourceCategory → admin,
	// noteType → editor), matching the preview routes above.
	router.Methods(http.MethodPost).Path("/v1/category/generateTemplate").HandlerFunc(api_handlers.GetGenerateTemplateHandler(appContext, "group"))
	router.Methods(http.MethodPost).Path("/v1/resourceCategory/generateTemplate").HandlerFunc(api_handlers.GetGenerateTemplateHandler(appContext, "resource"))
	router.Methods(http.MethodPost).Path("/v1/noteType/generateTemplate").HandlerFunc(api_handlers.GetGenerateTemplateHandler(appContext, "note"))

	// Global Search
	router.Methods(http.MethodGet).Path("/v1/search").HandlerFunc(scopedAPI(appContext, api_handlers.GetGlobalSearchHandler))

	// Download Queue (background remote downloads)
	// Submit runs on a request-scoped context so a group-limited principal can
	// only target groups inside its subtree (the worker itself runs unscoped).
	router.Methods(http.MethodPost).Path("/v1/download/submit").HandlerFunc(scopedAPI(appContext, api_handlers.GetDownloadSubmitHandler))
	router.Methods(http.MethodGet).Path("/v1/download/queue").HandlerFunc(api_handlers.GetDownloadQueueHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/cancel").HandlerFunc(api_handlers.GetDownloadCancelHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/pause").HandlerFunc(api_handlers.GetDownloadPauseHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/resume").HandlerFunc(api_handlers.GetDownloadResumeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/download/retry").HandlerFunc(api_handlers.GetDownloadRetryHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/download/events").HandlerFunc(api_handlers.GetDownloadEventsHandler(appContext))

	// Jobs routes (new canonical paths — download routes above kept as aliases)
	router.Methods(http.MethodPost).Path("/v1/jobs/download/submit").HandlerFunc(scopedAPI(appContext, api_handlers.GetDownloadSubmitHandler))
	router.Methods(http.MethodGet).Path("/v1/jobs/queue").HandlerFunc(api_handlers.GetDownloadQueueHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/cancel").HandlerFunc(api_handlers.GetDownloadCancelHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/pause").HandlerFunc(api_handlers.GetDownloadPauseHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/resume").HandlerFunc(api_handlers.GetDownloadResumeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/retry").HandlerFunc(api_handlers.GetDownloadRetryHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/jobs/events").HandlerFunc(api_handlers.GetDownloadEventsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/jobs/get").HandlerFunc(api_handlers.GetDownloadJobHandler(appContext))

	// Group exports
	router.Methods(http.MethodPost).Path("/v1/groups/export/estimate").HandlerFunc(scopedAPI(appContext, api_handlers.GetExportEstimateHandler))
	router.Methods(http.MethodPost).Path("/v1/groups/export").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Scope the context so a group-limited principal can only export groups in
		// its subtree; the background export job inherits the scoped context.
		api_handlers.GetExportSubmitHandler(scopedCtx(appContext, r), appContext.GetDefaultFs())(w, r)
	})
	router.Methods(http.MethodGet).Path("/v1/exports/{jobId}/download").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Scope the context so the download handler can enforce job ownership.
		api_handlers.GetExportDownloadHandler(scopedCtx(appContext, r), appContext.GetDefaultFs())(w, r)
	})

	// Group imports. Import creates new top-level groups, which a group-limited
	// principal could not place inside its subtree, so the whole import surface
	// is denied to scoped users/guests (fail-closed); unscoped users, editors,
	// and admins import as before.
	router.Methods(http.MethodPost).Path("/v1/groups/import/parse").HandlerFunc(denyScopedPrincipal(api_handlers.GetImportParseHandler(appContext, func() int64 { return appContext.Settings().MaxImportSize() })))
	router.Methods(http.MethodGet).Path("/v1/imports/{jobId}/plan").HandlerFunc(denyScopedPrincipal(api_handlers.GetImportPlanHandler(appContext)))
	router.Methods(http.MethodDelete).Path("/v1/imports/{jobId}").HandlerFunc(denyScopedPrincipal(api_handlers.GetImportDeleteHandler(appContext)))
	router.Methods(http.MethodPost).Path("/v1/imports/{jobId}/apply").HandlerFunc(denyScopedPrincipal(api_handlers.GetImportApplyHandler(appContext)))
	router.Methods(http.MethodGet).Path("/v1/imports/{jobId}/result").HandlerFunc(denyScopedPrincipal(api_handlers.GetImportResultHandler(appContext)))

	// Plugin action routes. The run handler is request-scoped so a group-limited
	// principal can only target entities inside its subtree.
	router.Methods(http.MethodGet).Path("/v1/plugin/actions").HandlerFunc(api_handlers.GetPluginActionsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/jobs/action/run").HandlerFunc(scopedAPI(appContext, api_handlers.GetActionRunHandler))
	router.Methods(http.MethodGet).Path("/v1/jobs/action/job").HandlerFunc(api_handlers.GetActionJobHandler(appContext))

	// Logs (read-only)
	router.Methods(http.MethodGet).Path("/v1/logs").HandlerFunc(api_handlers.GetLogEntriesHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/log").HandlerFunc(api_handlers.GetLogEntryHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/logs/entity").HandlerFunc(api_handlers.GetEntityHistoryHandler(appContext))

	// Admin stats routes
	router.Methods(http.MethodGet).Path("/v1/admin/server-stats").HandlerFunc(api_handlers.GetServerStatsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/admin/data-stats").HandlerFunc(api_handlers.GetDataStatsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/admin/data-stats/expensive").HandlerFunc(api_handlers.GetExpensiveStatsHandler(appContext))

	// Admin similarity maintenance jobs (image similarity v2)
	router.Methods(http.MethodPost).Path("/v1/admin/similarity/recompute").HandlerFunc(api_handlers.GetRecomputeSimilaritiesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/admin/similarity/retry-failed").HandlerFunc(api_handlers.GetRetryFailedHashesHandler(appContext))

	// Admin runtime settings routes
	router.Methods(http.MethodGet).Path("/v1/admin/settings").HandlerFunc(api_handlers.GetListSettingsHandler(appContext))
	router.Methods(http.MethodPut).Path("/v1/admin/settings/{key}").HandlerFunc(api_handlers.GetSetSettingHandler(appContext))
	router.Methods(http.MethodDelete).Path("/v1/admin/settings/{key}").HandlerFunc(api_handlers.GetResetSettingHandler(appContext))

	// Timeline routes
	router.Methods(http.MethodGet).Path("/v1/resources/timeline").HandlerFunc(scopedAPI(appContext, api_handlers.GetResourceTimelineHandler))
	router.Methods(http.MethodGet).Path("/v1/notes/timeline").HandlerFunc(scopedAPI(appContext, api_handlers.GetNoteTimelineHandler))
	router.Methods(http.MethodGet).Path("/v1/groups/timeline").HandlerFunc(scopedAPI(appContext, api_handlers.GetGroupTimelineHandler))
	router.Methods(http.MethodGet).Path("/v1/tags/timeline").HandlerFunc(api_handlers.GetTagTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/categories/timeline").HandlerFunc(api_handlers.GetCategoryTimelineHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/queries/timeline").HandlerFunc(api_handlers.GetQueryTimelineHandler(appContext))

	// Plugin management API
	router.Methods(http.MethodGet).Path("/v1/plugins/manage").HandlerFunc(api_handlers.GetPluginsManageHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/enable").HandlerFunc(api_handlers.GetPluginEnableHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/disable").HandlerFunc(api_handlers.GetPluginDisableHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/settings").HandlerFunc(api_handlers.GetPluginSettingsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/plugin/purge-data").HandlerFunc(api_handlers.GetPluginPurgeDataHandler(appContext))

	// Plugin block render endpoint (must be before the catch-all). Request-scoped
	// so a group-limited principal can only render blocks whose owning note is in
	// its subtree (GetBlock/GetNote enforce visibility on the scoped context).
	router.Methods(http.MethodGet).Path("/v1/plugins/{pluginName}/block/render").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api_handlers.GetPluginBlockRenderHandler(scopedCtx(appContext, r))(w, r)
	})

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
