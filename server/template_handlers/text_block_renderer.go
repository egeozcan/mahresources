package template_handlers

import (
	"net/http"
	"sync"

	"github.com/flosch/pongo2/v4"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/server/template_handlers/loaders"
	"mahresources/server/template_handlers/template_context_providers"
	"mahresources/shortcodes"
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
//
// authEnabled is the page's own "authEnabled", which the partial needs to
// sanitize the blocks under auth (#92); StaticTemplateCtx does not carry it.
func RenderTextBlocks(app ListCardContext, request *http.Request, note *models.Note, authEnabled bool) (string, error) {
	textBlockTemplates.Do(func() {
		textBlockTemplates.set = pongo2.NewSet("text-blocks", loaders.MustNewLocalFileSystemLoader("./templates", nil))
	})
	c := template_context_providers.StaticTemplateCtx(request)
	seen := map[string]bool{}
	c["_appContext"], c["_requestContext"], c["_pluginManager"] = app, shortcodes.WithCSSSeen(request.Context(), seen), app.PluginManager()
	c["_customCSSSeen"] = seen
	c["_pluginAccess"] = auth.PluginAccessFor(request.Context(), app.PluginAllowsScopedPrincipals)
	c["note"], c["blocks"] = note, note.Blocks
	c["authEnabled"] = authEnabled
	tpl, err := textBlockTemplates.set.FromCache("/partials/renderedTextBlocks.tpl")
	if err != nil {
		return "", err
	}
	return tpl.Execute(c)
}
