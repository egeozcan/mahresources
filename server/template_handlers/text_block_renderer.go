package template_handlers

import (
	"net/http"
	"sync"

	"github.com/flosch/pongo2/v4"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/server/template_handlers/loaders"
	"mahresources/server/template_handlers/template_context_providers"
)

var textBlockTemplates struct {
	sync.Once
	set *pongo2.TemplateSet
}

// RenderTextBlocks renders every text block of note through
// partials/renderedTextBlocks.tpl, the partial the note page renders them with,
// so a refresh shows exactly what a page load would. Unlike the list-card
// renderer it leaves _reqCtxWithCache unset: the first process_shortcodes tag
// then builds the page render context (MRQL cache, query budget, partial
// resolver, deadline) once and every block shares it, as on the page.
func RenderTextBlocks(app ListCardContext, request *http.Request, note *models.Note) (string, error) {
	textBlockTemplates.Do(func() {
		textBlockTemplates.set = pongo2.NewSet("text-blocks", loaders.MustNewLocalFileSystemLoader("./templates", nil))
	})
	c := template_context_providers.StaticTemplateCtx(request)
	c["_appContext"], c["_requestContext"], c["_pluginManager"] = app, request.Context(), app.PluginManager()
	c["_customCSSSeen"] = map[string]bool{}
	c["_pluginAccess"] = auth.PluginAccessFor(request.Context(), app.PluginAllowsScopedPrincipals)
	c["note"], c["blocks"] = note, note.Blocks
	tpl, err := textBlockTemplates.set.FromCache("/partials/renderedTextBlocks.tpl")
	if err != nil {
		return "", err
	}
	return tpl.Execute(c)
}
