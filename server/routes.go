package server

import (
	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/server/api_handlers"
	"mahresources/server/template_handlers"
	"mahresources/server/template_handlers/template_context_providers"
	"net/http"
)

type templateInformation struct {
	contextFn    func(context *application_context.MahresourcesContext) func(request *http.Request) pongo2.Context
	templateName string
	method       string
}

var templates = map[string]templateInformation{
	"/note/new":  {template_context_providers.NoteCreateContextProvider, "createNote.tpl", http.MethodGet},
	"/notes":     {template_context_providers.NoteListContextProvider, "listNotes.tpl", http.MethodGet},
	"/note":      {template_context_providers.NoteContextProvider, "displayNote.tpl", http.MethodGet},
	"/note/edit": {template_context_providers.NoteCreateContextProvider, "createNote.tpl", http.MethodGet},

	"/resource/new":     {template_context_providers.ResourceCreateContextProvider, "createResource.tpl", http.MethodGet},
	"/resources":        {template_context_providers.ResourceListContextProvider, "listResources.tpl", http.MethodGet},
	"/resources/simple": {template_context_providers.ResourceListContextProvider, "simpleResources.tpl", http.MethodGet},
	"/resource":         {template_context_providers.ResourceContextProvider, "displayResource.tpl", http.MethodGet},
	"/resource/edit":    {template_context_providers.ResourceCreateContextProvider, "createResource.tpl", http.MethodGet},

	"/group/new":  {template_context_providers.GroupCreateContextProvider, "createGroup.tpl", http.MethodGet},
	"/groups":     {template_context_providers.GroupsListContextProvider, "listGroups.tpl", http.MethodGet},
	"/group":      {template_context_providers.GroupContextProvider, "displayGroup.tpl", http.MethodGet},
	"/group/edit": {template_context_providers.GroupCreateContextProvider, "createGroup.tpl", http.MethodGet},

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
}

func registerRoutes(router *mux.Router, appContext *application_context.MahresourcesContext) {
	for path, templateInfo := range templates {
		router.Methods(templateInfo.method).Path(path).HandlerFunc(
			template_handlers.RenderTemplate(templateInfo.templateName, templateInfo.contextFn(appContext)),
		)

		router.Methods(templateInfo.method).Path(path + ".json").HandlerFunc(
			template_handlers.RenderTemplate(templateInfo.templateName, templateInfo.contextFn(appContext)),
		)
	}

	router.Methods(http.MethodGet).Path("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/notes", http.StatusMovedPermanently)
	})

	router.Methods(http.MethodGet).Path("/v1/notes").HandlerFunc(api_handlers.GetNotesHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/notes/meta/keys").HandlerFunc(api_handlers.GetNoteMetaKeysHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/note").HandlerFunc(api_handlers.GetNoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note").HandlerFunc(api_handlers.GetAddNoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/note/delete").HandlerFunc(api_handlers.GetRemoveNoteHandler(appContext))

	router.Methods(http.MethodGet).Path("/v1/groups").HandlerFunc(api_handlers.GetGroupsHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/groups/meta/keys").HandlerFunc(api_handlers.GetGroupMetaKeysHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/group").HandlerFunc(api_handlers.GetGroupHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/group").HandlerFunc(api_handlers.GetAddGroupHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/group/delete").HandlerFunc(api_handlers.GetRemoveGroupHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/addTags").HandlerFunc(api_handlers.GetAddTagsToGroupsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/groups/removeTags").HandlerFunc(api_handlers.GetRemoveTagsFromGroupsHandler(appContext))

	router.Methods(http.MethodPost).Path("/v1/relation").HandlerFunc(api_handlers.GetAddRelationHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relation/delete").HandlerFunc(api_handlers.GetRemoveRelationHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relationType").HandlerFunc(api_handlers.GetAddGroupRelationTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relationType/delete").HandlerFunc(api_handlers.GetRemoveRelationTypeHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/relationType/edit").HandlerFunc(api_handlers.GetEditGroupRelationTypeHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/relationTypes").HandlerFunc(api_handlers.GetRelationTypesHandler(appContext))

	router.Methods(http.MethodGet).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resources").HandlerFunc(api_handlers.GetResourcesHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resources/meta/keys").HandlerFunc(api_handlers.GetResourceMetaKeysHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceUploadHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/local").HandlerFunc(api_handlers.GetResourceAddLocalHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/remote").HandlerFunc(api_handlers.GetResourceAddRemoteHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/delete").HandlerFunc(api_handlers.GetRemoveResourceHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/resource/edit").HandlerFunc(api_handlers.GetResourceEditHandler(appContext))
	router.Methods(http.MethodGet).Path("/v1/resource/preview").HandlerFunc(api_handlers.GetResourceThumbnailHandler(appContext))

	router.Methods(http.MethodGet).Path("/v1/tags").HandlerFunc(api_handlers.GetTagsHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/tag").HandlerFunc(api_handlers.GetAddTagHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/tag/delete").HandlerFunc(api_handlers.GetRemoveTagHandler(appContext))

	router.Methods(http.MethodGet).Path("/v1/categories").HandlerFunc(api_handlers.GetCategoriesHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/category").HandlerFunc(api_handlers.GetAddCategoryHandler(appContext))
	router.Methods(http.MethodPost).Path("/v1/category/delete").HandlerFunc(api_handlers.GetRemoveCategoryHandler(appContext))
}
