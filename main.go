package main

import (
	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"mahresources/api_handlers"
	"mahresources/constants"
	"mahresources/context"
	"mahresources/models"
	contextProviders "mahresources/templates/template_context_providers"
	_ "mahresources/templates/template_filters"
	handlers "mahresources/templates/template_handlers"
	"net/http"
	"os"
	"time"
)

func main() {
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
		logger.Config{
			SlowThreshold: 0,
			LogLevel:      logger.Info,
			Colorful:      true,
		},
	)

	db, err := gorm.Open(sqlite.Open("test.db"), &gorm.Config{
		Logger: newLogger,
	})
	if err != nil {
		panic("failed to connect to the database")
	}

	err = db.AutoMigrate(&models.Resource{})
	if err != nil {
		panic("failed to migrate Resource")
	}
	err = db.AutoMigrate(&models.Note{})
	if err != nil {
		panic("failed to migrate Note")
	}
	err = db.AutoMigrate(&models.Tag{})
	if err != nil {
		panic("failed to migrate Tag")
	}
	err = db.AutoMigrate(&models.Group{})
	if err != nil {
		panic("failed to migrate Group")
	}

	base := afero.NewBasePathFs(afero.NewOsFs(), "./filezz")
	layer := afero.NewMemMapFs()
	cachedFS := afero.NewCacheOnReadFs(base, layer, 10*time.Minute)

	httpFs := afero.NewHttpFs(cachedFS)

	router := mux.NewRouter()

	appContext := context.NewMahresourcesContext(cachedFS, db)

	router.Methods(constants.GET).Path("/note/new").HandlerFunc(
		handlers.RenderTemplate("templates/createNote.tpl", contextProviders.NoteCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/notes").HandlerFunc(
		handlers.RenderTemplate("templates/listNotes.tpl", contextProviders.NoteListContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/note").HandlerFunc(
		handlers.RenderTemplate("templates/displayNote.tpl", contextProviders.NoteContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/note/edit").HandlerFunc(
		handlers.RenderTemplate("templates/createNote.tpl", contextProviders.NoteCreateContextProvider(appContext)),
	)

	router.Methods(constants.GET).Path("/resource/new").HandlerFunc(
		handlers.RenderTemplate("templates/createResource.tpl", contextProviders.ResourceCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/resources").HandlerFunc(
		handlers.RenderTemplate("templates/listResources.tpl", contextProviders.ResourceListContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/resource").HandlerFunc(
		handlers.RenderTemplate("templates/displayResource.tpl", contextProviders.ResourceContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/resource/edit").HandlerFunc(
		handlers.RenderTemplate("templates/createResource.tpl", contextProviders.ResourceCreateContextProvider(appContext)),
	)

	router.Methods(constants.GET).Path("/group/new").HandlerFunc(
		handlers.RenderTemplate("templates/createGroup.tpl", contextProviders.GroupCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/group/edit").HandlerFunc(
		handlers.RenderTemplate("templates/createGroup.tpl", contextProviders.GroupCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/groups").HandlerFunc(
		handlers.RenderTemplate("templates/listGroups.tpl", contextProviders.GroupsListContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/group").HandlerFunc(
		handlers.RenderTemplate("templates/displayGroup.tpl", contextProviders.GroupContextProvider(appContext)),
	)

	router.Methods(constants.GET).Path("/tag/new").HandlerFunc(
		handlers.RenderTemplate("templates/createTag.tpl", contextProviders.TagCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/tag/edit").HandlerFunc(
		handlers.RenderTemplate("templates/createTag.tpl", contextProviders.TagCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/tags").HandlerFunc(
		handlers.RenderTemplate("templates/listTags.tpl", contextProviders.TagListContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/tag").HandlerFunc(
		handlers.RenderTemplate("templates/displayTag.tpl", contextProviders.TagContextProvider(appContext)),
	)

	router.Methods(constants.GET).Path("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/notes", http.StatusMovedPermanently)
	})

	router.Methods(constants.GET).Path("/v1/notes").HandlerFunc(api_handlers.GetNotesHandler(appContext))
	router.Methods(constants.GET).Path("/v1/note").HandlerFunc(api_handlers.GetNoteHandler(appContext))
	router.Methods(constants.POST).Path("/v1/note").HandlerFunc(api_handlers.GetAddNoteHandler(appContext))

	router.Methods(constants.GET).Path("/v1/groups").HandlerFunc(api_handlers.GetGroupsHandler(appContext))
	router.Methods(constants.GET).Path("/v1/groups/autocomplete").HandlerFunc(api_handlers.GetGroupsAutocompleteHandler(appContext))
	router.Methods(constants.GET).Path("/v1/group").HandlerFunc(api_handlers.GetGroupHandler(appContext))
	router.Methods(constants.POST).Path("/v1/group").HandlerFunc(api_handlers.GetAddGroupHandler(appContext))

	router.Methods(constants.GET).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceUploadHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource/edit").HandlerFunc(api_handlers.GetResourceEditHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource/preview").HandlerFunc(api_handlers.GetResourceUploadPreviewHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource/addToNote").HandlerFunc(api_handlers.GetAddResourceToNoteHandler(appContext))

	router.Methods(constants.GET).Path("/v1/tags").HandlerFunc(api_handlers.GetTagsHandler(appContext))
	router.Methods(constants.POST).Path("/v1/tag").HandlerFunc(api_handlers.GetAddTagHandler(appContext))

	filePathPrefix := "/files/"
	router.PathPrefix(filePathPrefix).Handler(http.StripPrefix(filePathPrefix, http.FileServer(httpFs.Dir("/"))))
	router.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      router,
		WriteTimeout: 45 * time.Minute,
		ReadTimeout:  45 * time.Minute,
	}

	log.Fatal(srv.ListenAndServe())
}
