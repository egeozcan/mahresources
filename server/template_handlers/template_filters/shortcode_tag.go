package template_filters

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
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

	metaCtx := buildMetaContext(entity)
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
		reqCtx = plugin_system.WithMRQLCache(reqCtx)
		ctx.Public["_reqCtxWithCache"] = reqCtx
	}

	var pluginRenderer shortcodes.PluginRenderer
	if pmVal, ok := ctx.Public["_pluginManager"]; ok && pmVal != nil {
		if pm, ok := pmVal.(*plugin_system.PluginManager); ok && pm != nil {
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity)
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

// buildMetaContext uses reflection to extract entity type, ID, Meta, and MetaSchema
// from Group, Resource, or Note model structs.
func buildMetaContext(entity any) *shortcodes.MetaShortcodeContext {
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
	default:
		return nil
	}

	return &shortcodes.MetaShortcodeContext{
		EntityType: entityType,
		EntityID:   id,
		Meta:       metaJSON,
		MetaSchema: metaSchema,
		Entity:     entity,
	}
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
