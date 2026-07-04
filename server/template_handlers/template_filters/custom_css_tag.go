package template_filters

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v4"
	"mahresources/application_context"
	"mahresources/plugin_system"
	"mahresources/shortcodes"
)

// customCSSNode renders the CustomCSS of the category/type of an entity — or of every
// distinct category among a collection of entities — as raw <style> blocks. It is meant
// to be used inside a template's {% block head %} so the CSS is injected page-wide
// ("globally"), letting the other Custom* slots (header, sidebar, summary, avatar, mrql
// result) be styled without inlining <style> in each.
//
// KAN-6: the CSS is emitted UNESCAPED on purpose. Mahresources is a trusted, private-network
// personal-information tool and CustomCSS is an intentional extension point, so real CSS
// (selectors containing '>', content() with quotes, etc.) must survive verbatim. Shortcodes
// are processed server-side using a representative entity of each category, so [meta ...] and
// friends still resolve. Each distinct category is emitted at most once per page render
// (deduped via ctx.Public), which is what makes the list/MRQL cases produce one block per
// category instead of one per card.
type customCSSNode struct {
	expr pongo2.IEvaluator
}

func (node *customCSSNode) Execute(ctx *pongo2.ExecutionContext, writer pongo2.TemplateWriter) *pongo2.Error {
	val, err := node.expr.Evaluate(ctx)
	if err != nil {
		return err
	}
	if val == nil {
		return nil
	}
	entities := collectCustomCSSEntities(val.Interface())
	if len(entities) == 0 {
		return nil
	}

	// Page-level dedup set, shared across every custom_css tag on the render.
	seen, _ := ctx.Public["_customCSSSeen"].(map[string]bool)
	if seen == nil {
		seen = map[string]bool{}
		ctx.Public["_customCSSSeen"] = seen
	}

	var appCtx *application_context.MahresourcesContext
	if v, ok := ctx.Public["_appContext"]; ok && v != nil {
		appCtx, _ = v.(*application_context.MahresourcesContext)
	}
	reqCtx := customCSSReqCtx(ctx)
	pluginRenderer := customCSSPluginRenderer(ctx, reqCtx)
	var executor shortcodes.QueryExecutor
	if appCtx != nil {
		executor = BuildQueryExecutor(appCtx)
	}

	for _, e := range entities {
		entityType, catID, css, ok := customCSSForEntity(e)
		if !ok || strings.TrimSpace(css) == "" {
			continue
		}
		key := entityType + ":" + strconv.FormatUint(uint64(catID), 10)
		if seen[key] {
			continue
		}
		seen[key] = true

		rendered := css
		if metaCtx := buildMetaContext(e, appCtx); metaCtx != nil {
			rendered = shortcodes.Process(reqCtx, css, *metaCtx, pluginRenderer, executor)
		}
		if _, werr := writer.WriteString("<style data-mr-custom-css=\"" + key + "\">" + rendered + "</style>"); werr != nil {
			return ctx.Error(fmt.Sprintf("custom_css: write error: %s", werr), nil)
		}
	}
	return nil
}

// collectCustomCSSEntities normalises the tag argument into a flat list of entity values.
// Accepts a single entity (*models.Group, etc.), a slice/array of entities, or a pointer to
// such a slice. Nil pointers and unsupported kinds yield an empty list.
func collectCustomCSSEntities(val any) []any {
	v := reflect.ValueOf(val)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() == reflect.Slice || v.Kind() == reflect.Array {
		out := make([]any, 0, v.Len())
		for i := 0; i < v.Len(); i++ {
			out = append(out, v.Index(i).Interface())
		}
		return out
	}
	if !v.IsValid() {
		return nil
	}
	return []any{v.Interface()}
}

// customCSSForEntity extracts the entity type, its category ID, and the category's CustomCSS
// from a Group / Resource / Note model via reflection (mirroring buildMetaContext's dispatch).
func customCSSForEntity(entity any) (entityType string, catID uint, css string, ok bool) {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}

	var catFieldName string
	switch v.Type().Name() {
	case "Group":
		entityType, catFieldName = "group", "Category"
	case "Resource":
		entityType, catFieldName = "resource", "ResourceCategory"
	case "Note":
		entityType, catFieldName = "note", "NoteType"
	default:
		return
	}

	catField := v.FieldByName(catFieldName)
	if !catField.IsValid() || catField.Kind() != reflect.Ptr || catField.IsNil() {
		return
	}
	cat := catField.Elem()

	cssField := cat.FieldByName("CustomCSS")
	idField := cat.FieldByName("ID")
	if !cssField.IsValid() || cssField.Kind() != reflect.String {
		return
	}
	if !idField.IsValid() || idField.Kind() != reflect.Uint {
		return
	}
	return entityType, uint(idField.Uint()), cssField.String(), true
}

// customCSSReqCtx mirrors process_shortcodes: reuse the per-render MRQL-cache-wrapped request
// context if one exists, otherwise create it and stash it for subsequent tags.
func customCSSReqCtx(ctx *pongo2.ExecutionContext) context.Context {
	if rcVal, ok := ctx.Public["_reqCtxWithCache"]; ok && rcVal != nil {
		if rc, ok := rcVal.(context.Context); ok {
			return rc
		}
	}
	reqCtx := context.Background()
	if v, ok := ctx.Public["_requestContext"]; ok && v != nil {
		if rc, ok := v.(context.Context); ok {
			reqCtx = rc
		}
	}
	reqCtx = plugin_system.WithMRQLCache(reqCtx)
	if appCtxVal, ok := ctx.Public["_appContext"]; ok && appCtxVal != nil {
		if appCtx, ok := appCtxVal.(*application_context.MahresourcesContext); ok {
			reqCtx = shortcodes.WithPartialResolver(reqCtx, BuildPartialResolver(appCtx))
		}
	}
	ctx.Public["_reqCtxWithCache"] = reqCtx
	return reqCtx
}

func customCSSPluginRenderer(ctx *pongo2.ExecutionContext, reqCtx context.Context) shortcodes.PluginRenderer {
	pmVal, ok := ctx.Public["_pluginManager"]
	if !ok || pmVal == nil {
		return nil
	}
	pm, ok := pmVal.(*plugin_system.PluginManager)
	if !ok || pm == nil {
		return nil
	}
	return func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
		return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
	}
}

func customCSSTagParser(doc *pongo2.Parser, start *pongo2.Token, arguments *pongo2.Parser) (pongo2.INodeTag, *pongo2.Error) {
	expr, err := arguments.ParseExpression()
	if err != nil {
		return nil, err
	}
	if arguments.Remaining() > 0 {
		return nil, arguments.Error("custom_css tag takes exactly one argument (an entity or a collection of entities)", nil)
	}
	return &customCSSNode{expr: expr}, nil
}

func init() {
	if err := pongo2.RegisterTag("custom_css", customCSSTagParser); err != nil {
		fmt.Println("error when registering custom_css tag:", err)
	}
}
