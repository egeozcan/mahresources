package template_filters

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"reflect"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/deferredtoken"
	"mahresources/mrql"
	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

type processShortcodesNode struct {
	contentExpr pongo2.IEvaluator
	entityExpr  pongo2.IEvaluator
}

func (node *processShortcodesNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	contentVal, err := node.contentExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	content := contentVal.String()
	if content == "" {
		return nil
	}

	entityVal, err := node.entityExpr.Evaluate(ctx)
	if err != nil {
		return err
	}
	entity := entityVal.Interface()
	if entity == nil {
		_, _ = writer.WriteString(content)
		return nil
	}

	var appCtx *application_context.MahresourcesContext
	if appCtxVal, ok := ctx.Public["_appContext"]; ok && appCtxVal != nil {
		appCtx, _ = appCtxVal.(*application_context.MahresourcesContext)
	}

	metaCtx := buildMetaContext(entity, appCtx)
	if metaCtx == nil {
		_, _ = writer.WriteString(content)
		return nil
	}

	// Use request context if available, otherwise background.
	// Attach MRQL cache once per page render: the first process_shortcodes tag
	// creates it and stores the wrapped context back into ctx.Public so
	// subsequent tags (header, sidebar, avatar, etc.) reuse the same cache.
	reqCtx := context.Background()
	if rcVal, ok := ctx.Public["_reqCtxWithCache"]; ok && rcVal != nil {
		if rc, ok := rcVal.(context.Context); ok {
			reqCtx = rc
		}
	} else {
		if reqCtxVal, ok := ctx.Public["_requestContext"]; ok && reqCtxVal != nil {
			if rc, ok := reqCtxVal.(context.Context); ok {
				reqCtx = rc
			}
		}
		reqCtx = buildPageRenderContext(reqCtx, appCtx)
		ctx.Public["_reqCtxWithCache"] = reqCtx
	}

	// A group-confined principal must never cause plugin Lua to run: it executes
	// against the unscoped DB handle. Leaving the renderer nil makes [plugin:*]
	// render the same "unavailable" comment the share server already produces.
	var pluginRenderer shortcodes.PluginRenderer
	access := pluginAccessFromContext(ctx, reqCtx)
	if pmVal, ok := ctx.Public["_pluginManager"]; ok && pmVal != nil {
		if pm, ok := pmVal.(*plugin_system.PluginManager); ok && pm != nil {
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				if !access(pluginName) {
					return "", shortcodes.ErrPluginUnavailable
				}
				return pm.RenderShortcodeContext(reqCtx, pluginName, sc.Name, mctx, sc.Attrs, sc.InnerContent, sc.IsBlock)
			}
		}
	}

	var executor shortcodes.QueryExecutor
	if appCtxVal, ok := ctx.Public["_appContext"]; ok && appCtxVal != nil {
		if appCtx, ok := appCtxVal.(*application_context.MahresourcesContext); ok && appCtx != nil {
			executor = BuildQueryExecutor(appCtx)
		}
	}

	if principal := auth.PrincipalFromContext(reqCtx); principal != nil {
		metaCtx.ForceReadOnly = metaCtx.ForceReadOnly || !principal.CanWrite()
	}
	result := shortcodes.Process(reqCtx, content, *metaCtx, pluginRenderer, executor)
	result = wrapReloadableRegion(result, content, *metaCtx, appCtx)
	if _, writeErr := writer.WriteString(result); writeErr != nil {
		return ctx.Error(fmt.Sprintf("process_shortcodes: write error: %s", writeErr), nil)
	}
	return nil
}

// wrapReloadableRegion wraps a rendered custom-content slot in a region element
// carrying a sealed token of the slot's raw body, so a [reload] button in that
// slot can re-render the whole slot through /v1/shortcodes/deferred. The token
// is the same (entityType, entityID, body) seal [lazy] and [details] mint, which
// is what lets one endpoint serve all three: a region is simply a body that was
// rendered eagerly and can be asked for again.
//
// It is emitted only when the rendered slot actually contains a reload button.
// During this page render a [reload] inside a deferred [lazy]/[details] block is
// not expanded at all (that body is captured into a token instead), so a button
// present here is one with no deferred ancestor on the page, and every other slot
// keeps its markup byte-for-byte unchanged. Buttons that arrive later, inside a
// body the deferred endpoint expands, do sit under a deferred host: the frontend
// walks the DOM at click time, so those resolve to that host rather than here.
// Carrier slots (CustomListHeader) are skipped because the endpoint cannot load a
// Category by (type, id); a [reload] there falls back to reloading the page.
func wrapReloadableRegion(rendered, raw string, metaCtx shortcodes.MetaShortcodeContext, appCtx *application_context.MahresourcesContext) string {
	if appCtx == nil || !shortcodes.ContainsReloadButton(rendered) || !shortcodes.IsDeferrableEntity(metaCtx) {
		return rendered
	}
	token := deferredtoken.Seal(appCtx.DeferredSigningKey(), metaCtx.EntityType, metaCtx.EntityID, raw)
	if token == "" {
		// Sealing only fails catastrophically (a broken CSPRNG), but a wrapper
		// with no token is one the frontend would skip anyway. Leave the slot as
		// it was and let the button fall back to reloading the page.
		return rendered
	}
	return `<div class="shortcode-region" data-shortcode-region="` + html.EscapeString(token) + `">` + rendered + `</div>`
}

// buildPageRenderContext wraps reqCtx with the per-page render helpers shared by
// the process_shortcodes and custom_css tags: a per-render MRQL cache, the partial
// resolver, the inline-MRQL query budget, and — when appCtx is available — the
// deferred-render signer that makes [lazy]/[details] emit signed placeholders the
// frontend resolves via /v1/shortcodes/deferred (every other render surface omits
// the signer and renders those blocks inline).
//
// Both tags stash the result in ctx.Public["_reqCtxWithCache"] and reuse it, so
// whichever runs first on a page (the custom_css tag renders in <head>, before the
// body's process_shortcodes tags) must install the full set — otherwise later tags
// reuse a context missing the signer and deferral silently degrades to inline.
func buildPageRenderContext(reqCtx context.Context, appCtx *application_context.MahresourcesContext) context.Context {
	reqCtx = plugin_system.WithMRQLCache(reqCtx)
	reqCtx = application_context.WithMRQLRenderDataCache(reqCtx)
	// partials stays a nil INTERFACE when appCtx is nil. Assigning a nil
	// *MahresourcesContext would make it a non-nil interface holding a nil
	// pointer, defeating BuildPartialResolver's own nil check and returning a
	// resolver that panics on first use. The appCtx != nil guard below shows nil
	// is an expected input here.
	var partials PartialResolverContext
	if appCtx != nil {
		partials = appCtx
	}
	reqCtx = shortcodes.WithPartialResolver(reqCtx, BuildPartialResolver(partials))
	reqCtx = shortcodes.WithQueryBudget(reqCtx, pageQueryBudget(appCtx))
	if appCtx != nil {
		reqCtx = shortcodes.WithDeferredSigner(reqCtx, func(entityType string, entityID uint, body string) string {
			return deferredtoken.Seal(appCtx.DeferredSigningKey(), entityType, entityID, body)
		})
	}
	return reqCtx
}

// BuildMetaContextForEntity builds the shortcode rendering context for an entity
// (Group, Resource, or Note). It is the exported entry point so the template
// preview handler and the process_shortcodes tag share one implementation.
func BuildMetaContextForEntity(entity any, appCtx MetaScopeResolver) *shortcodes.MetaShortcodeContext {
	return buildMetaContext(entity, appCtx)
}

// buildMetaContext uses reflection to extract entity type, ID, Meta, and MetaSchema
// from Group, Resource, or Note model structs. When appCtx is non-nil, scope fields
// (parent, root) are resolved via DB; otherwise falls back to best-effort sentinels.
func buildMetaContext(entity any, appCtx MetaScopeResolver) *shortcodes.MetaShortcodeContext {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	idField := v.FieldByName("ID")
	if !idField.IsValid() || idField.Kind() != reflect.Uint {
		return nil
	}
	id := uint(idField.Uint())

	var metaJSON json.RawMessage
	metaField := v.FieldByName("Meta")
	if metaField.IsValid() {
		if raw, err := json.Marshal(metaField.Interface()); err == nil {
			metaJSON = raw
		}
	}

	typeName := v.Type().Name()
	var entityType, metaSchema string

	switch typeName {
	case "Group":
		entityType = "group"
		metaSchema = extractCategorySchema(v, "Category")
	case "Resource":
		entityType = "resource"
		metaSchema = extractCategorySchema(v, "ResourceCategory")
	case "Note":
		entityType = "note"
		metaSchema = extractCategorySchema(v, "NoteType")
	case "Category", "ResourceCategory", "NoteType":
		// Carrier types render their own list-header slot (CustomListHeader). The
		// carrier is not a content entity: it has no Meta (so [meta] renders its
		// empty state) and no owning group, so [mrql] must resolve against global
		// scope (0/0/0) — dashboard queries like "count of groups in this category"
		// are the whole point. carrierEntityType maps the struct name to the
		// [meta] data-entity-type attribute (cosmetic here since Meta is empty).
		return &shortcodes.MetaShortcodeContext{
			EntityType:    carrierEntityType(typeName),
			EntityID:      id,
			Meta:          metaJSON, // nil — carriers have no Meta field
			MetaSchema:    "",
			Entity:        entity,
			ScopeGroupID:  0,
			ParentGroupID: 0,
			RootGroupID:   0,
		}
	default:
		return nil
	}

	// Extract scope fields — DB-backed when appCtx is available
	scopeID, parentID, rootID := resolveScopeFromEntity(v, entityType, id, appCtx)

	metaCtx := &shortcodes.MetaShortcodeContext{
		EntityType:    entityType,
		EntityID:      id,
		Meta:          metaJSON,
		MetaSchema:    metaSchema,
		Entity:        entity,
		ScopeGroupID:  scopeID,
		ParentGroupID: parentID,
		RootGroupID:   rootID,
	}
	populatePresentationNames(metaCtx, v)
	return metaCtx
}

// EnrichMetaContextPresentation is used by non-template render paths that
// already assembled a MetaShortcodeContext but still have the preloaded entity.
func EnrichMetaContextPresentation(ctx *shortcodes.MetaShortcodeContext, entity any) {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Struct {
		populatePresentationNames(ctx, v)
	}
}

type presentationGroup struct {
	id       uint
	name     string
	category string
}

// populatePresentationNames reads only associations the page query already
// preloaded. It never performs a lookup; callers that render collections can
// therefore add ownership context without turning each card into another DB
// round trip.
func populatePresentationNames(ctx *shortcodes.MetaShortcodeContext, entity reflect.Value) {
	groups := make([]presentationGroup, 0, 4)
	if ctx.EntityType == "group" {
		if group, ok := presentationGroupFromValue(entity); ok {
			groups = append(groups, group)
		}
	}
	owner := entity.FieldByName("Owner")
	for depth := 0; depth < 8 && owner.IsValid() && owner.Kind() == reflect.Ptr && !owner.IsNil(); depth++ {
		value := owner.Elem()
		group, ok := presentationGroupFromValue(value)
		if !ok {
			break
		}
		groups = append(groups, group)
		owner = value.FieldByName("Owner")
	}

	for _, group := range groups {
		switch group.id {
		case ctx.ScopeGroupID:
			ctx.ScopeGroupName, ctx.ScopeCategoryName = group.name, group.category
		case ctx.ParentGroupID:
			ctx.ParentGroupName, ctx.ParentCategoryName = group.name, group.category
		}
		if group.id == ctx.RootGroupID {
			ctx.RootGroupName, ctx.RootCategoryName = group.name, group.category
		}
	}
}

func presentationGroupFromValue(value reflect.Value) (presentationGroup, bool) {
	if value.Kind() != reflect.Struct {
		return presentationGroup{}, false
	}
	id := value.FieldByName("ID")
	name := value.FieldByName("Name")
	if !id.IsValid() || id.Kind() != reflect.Uint || !name.IsValid() || name.Kind() != reflect.String {
		return presentationGroup{}, false
	}
	group := presentationGroup{id: uint(id.Uint()), name: name.String()}
	category := value.FieldByName("Category")
	if category.IsValid() && category.Kind() == reflect.Ptr && !category.IsNil() {
		categoryName := category.Elem().FieldByName("Name")
		if categoryName.IsValid() && categoryName.Kind() == reflect.String {
			group.category = categoryName.String()
		}
	}
	return group, group.id > 0
}

// carrierEntityType maps a carrier struct name (Category/ResourceCategory/NoteType)
// to the entity-type string used in the [meta] data-entity-type attribute.
func carrierEntityType(typeName string) string {
	switch typeName {
	case "Category":
		return "category"
	case "ResourceCategory":
		return "resource_category"
	case "NoteType":
		return "note_type"
	default:
		return typeName
	}
}

// resolveScopeFromEntity resolves scope group IDs for an entity.
// When appCtx is available, uses DB-backed resolution for parent/root.
// Otherwise falls back to sentinel values.
func resolveScopeFromEntity(v reflect.Value, entityType string, entityID uint, appCtx MetaScopeResolver) (scopeID, parentID, rootID uint) {
	sentinel := mrql.UnresolvedScopeSentinel

	// Extract OwnerId via reflection
	var ownerID *uint
	ownerField := v.FieldByName("OwnerId")
	if ownerField.IsValid() && ownerField.Kind() == reflect.Ptr && !ownerField.IsNil() {
		oid := uint(ownerField.Elem().Uint())
		if oid > 0 {
			ownerID = &oid
		}
	}

	if entityType == "group" {
		scopeID = entityID
		if ownerID != nil {
			parentID = *ownerID
		} else {
			parentID = sentinel
		}
		if appCtx != nil {
			rootID = appCtx.ResolveRootScopeID(entityID)
		} else {
			rootID = sentinel
		}
		return
	}

	// Resources and notes
	if ownerID != nil {
		scopeID = *ownerID
		if appCtx != nil {
			parentID = appCtx.ResolveParentScopeID(*ownerID)
			rootID = appCtx.ResolveRootScopeID(*ownerID)
		} else {
			parentID = sentinel
			rootID = sentinel
		}
	} else {
		scopeID = sentinel
		parentID = sentinel
		rootID = sentinel
	}
	return
}

// extractCategorySchema reads the MetaSchema field from a preloaded category/type relation.
func extractCategorySchema(entityVal reflect.Value, fieldName string) string {
	catField := entityVal.FieldByName(fieldName)
	if !catField.IsValid() || catField.Kind() != reflect.Ptr || catField.IsNil() {
		return ""
	}
	catVal := catField.Elem()
	schemaField := catVal.FieldByName("MetaSchema")
	if !schemaField.IsValid() || schemaField.Kind() != reflect.String {
		return ""
	}
	return schemaField.String()
}

func processShortcodesTagParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	contentExpr, err := arguments.ParseExpression()
	if err != nil {
		return nil, err
	}

	entityExpr, err := arguments.ParseExpression()
	if err != nil {
		return nil, arguments.Error("process_shortcodes tag requires two arguments: content and entity", nil)
	}

	if arguments.Remaining() > 0 {
		return nil, arguments.Error("process_shortcodes tag takes exactly two arguments", nil)
	}

	return &processShortcodesNode{
		contentExpr: contentExpr,
		entityExpr:  entityExpr,
	}, nil
}

func init() {
	if err := pongo2.RegisterTag("process_shortcodes", processShortcodesTagParser); err != nil {
		fmt.Println("error when registering process_shortcodes tag:", err)
	}
}
