package template_handlers

import (
	"fmt"
	"net/http"
	"sync"

	"github.com/flosch/pongo2/v4"
	"mahresources/auth"
	"mahresources/plugin_system"
	"mahresources/server/template_handlers/loaders"
	"mahresources/server/template_handlers/template_context_providers"
	"mahresources/server/template_handlers/template_filters"
)

type ListCardContext interface {
	template_filters.PageRenderContext
	PluginManager() *plugin_system.PluginManager
	PluginAllowsScopedPrincipals(string) bool
}

var listCardTemplates struct {
	sync.Once
	set *pongo2.TemplateSet
}

// NewListCardRenderer shares request state across executions and caches parsed
// templates. Literal includes ensure entity partials are parsed once as well.
func NewListCardRenderer(app ListCardContext, request *http.Request, headingLevel int) func(string, any) (string, error) {
	listCardTemplates.Do(func() {
		listCardTemplates.set = pongo2.NewSet("list-cards", loaders.MustNewLocalFileSystemLoader("./templates", nil))
	})
	c := template_context_providers.StaticTemplateCtx(request)
	c["_appContext"], c["_requestContext"], c["_pluginManager"] = app, request.Context(), app.PluginManager()
	c["_reqCtxWithCache"], c["_customCSSSeen"] = request.Context(), map[string]bool{}
	c["_pluginAccess"] = auth.PluginAccessFor(request.Context(), app.PluginAllowsScopedPrincipals)
	c["selectable"], c["cardHeadingLevel"] = true, headingLevel
	nextID := c["getNextId"].(func(string) string)
	cardActions := map[string]any{}
	if pm := app.PluginManager(); pm != nil {
		access := auth.PluginActionAccessFor(request.Context(), app.PluginAllowsScopedPrincipals)
		for _, entityType := range []string{"resource", "note", "group"} {
			registered := pm.GetListActionsForPlacement(entityType, "card", nil)
			allowed := registered[:0]
			for _, action := range registered {
				if access(action.PluginName) {
					allowed = append(allowed, action)
				}
			}
			cardActions[entityType] = allowed
		}
	}
	return func(entityType string, entity any) (string, error) {
		if entityType != "resource" && entityType != "note" && entityType != "group" {
			return "", fmt.Errorf("unsupported list entity %q", entityType)
		}
		c["entity"], c["tagBaseUrl"] = entity, "/"+entityType+"s"
		c["pluginCardActions"] = cardActions[entityType]
		c["getNextId"] = func(name string) string { return "mrql_" + entityType + "_" + nextID(name) }
		tpl, err := listCardTemplates.set.FromCache("/partials/mrqlList" + entityType + ".tpl")
		if err != nil {
			return "", err
		}
		return tpl.Execute(c)
	}
}
