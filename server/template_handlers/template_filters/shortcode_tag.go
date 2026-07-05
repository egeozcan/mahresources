package template_filters

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/lib/deferredtoken"
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

	var pluginRenderer shortcodes.PluginRenderer
	if pmVal, ok := ctx.Public["_pluginManager"]; ok && pmVal != nil {
		if pm, ok := pmVal.(*plugin_system.PluginManager); ok && pm != nil {
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
			}
		}
	}

	var executor shortcodes.QueryExecutor
	if appCtxVal, ok := ctx.Public["_appContext"]; ok && appCtxVal != nil {
		if appCtx, ok := appCtxVal.(*application_context.MahresourcesContext); ok && appCtx != nil {
			executor = BuildQueryExecutor(appCtx)
		}
	}

	result := shortcodes.Process(reqCtx, content, *metaCtx, pluginRenderer, executor)
	if _, writeErr := writer.WriteString(result); writeErr != nil {
		return ctx.Error(fmt.Sprintf("process_shortcodes: write error: %s", writeErr), nil)
	}
	return nil
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
	reqCtx = shortcodes.WithPartialResolver(reqCtx, BuildPartialResolver(appCtx))
	reqCtx = shortcodes.WithQueryBudget(reqCtx, pageQueryBudget(appCtx))
	if appCtx != nil {
		reqCtx = shortcodes.WithDeferredSigner(reqCtx, func(entityType string, entityID uint, body string) string {
			return deferredtoken.Sign(appCtx.DeferredSigningKey(), entityType, entityID, body)
		})
	}
	return reqCtx
}

// BuildMetaContextForEntity builds the shortcode rendering context for an entity
// (Group, Resource, or Note). It is the exported entry point so the template
// preview handler and the process_shortcodes tag share one implementation.
func BuildMetaContextForEntity(entity any, appCtx *application_context.MahresourcesContext) *shortcodes.MetaShortcodeContext {
	return buildMetaContext(entity, appCtx)
}

// buildMetaContext uses reflection to extract entity type, ID, Meta, and MetaSchema
// from Group, Resource, or Note model structs. When appCtx is non-nil, scope fields
// (parent, root) are resolved via DB; otherwise falls back to best-effort sentinels.
func buildMetaContext(entity any, appCtx *application_context.MahresourcesContext) *shortcodes.MetaShortcodeContext {
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

	return &shortcodes.MetaShortcodeContext{
		EntityType:    entityType,
		EntityID:      id,
		Meta:          metaJSON,
		MetaSchema:    metaSchema,
		Entity:        entity,
		ScopeGroupID:  scopeID,
		ParentGroupID: parentID,
		RootGroupID:   rootID,
	}
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
func resolveScopeFromEntity(v reflect.Value, entityType string, entityID uint, appCtx *application_context.MahresourcesContext) (scopeID, parentID, rootID uint) {
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
