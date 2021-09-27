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
	err = db.AutoMigrate(&models.Album{})
	if err != nil {
		panic("failed to migrate Album")
	}
	err = db.AutoMigrate(&models.Tag{})
	if err != nil {
		panic("failed to migrate Tag")
	}
	err = db.AutoMigrate(&models.Person{})
	if err != nil {
		panic("failed to migrate Person")
	}

	base := afero.NewBasePathFs(afero.NewOsFs(), "./filezz")
	layer := afero.NewMemMapFs()
	cachedFS := afero.NewCacheOnReadFs(base, layer, 10*time.Minute)

	httpFs := afero.NewHttpFs(cachedFS)

	router := mux.NewRouter()

	appContext := context.NewMahresourcesContext(cachedFS, db)

	router.Methods(constants.GET).Path("/album/new").HandlerFunc(
		handlers.RenderTemplate("templates/createAlbum.tpl", contextProviders.AlbumCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/albums").HandlerFunc(
		handlers.RenderTemplate("templates/listAlbums.tpl", contextProviders.AlbumListContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/album").HandlerFunc(
		handlers.RenderTemplate("templates/displayAlbum.tpl", contextProviders.AlbumContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/album/edit").HandlerFunc(
		handlers.RenderTemplate("templates/createAlbum.tpl", contextProviders.AlbumCreateContextProvider(appContext)),
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

	router.Methods(constants.GET).Path("/person/new").HandlerFunc(
		handlers.RenderTemplate("templates/createPerson.tpl", contextProviders.PersonCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/person/edit").HandlerFunc(
		handlers.RenderTemplate("templates/createPerson.tpl", contextProviders.PersonCreateContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/people").HandlerFunc(
		handlers.RenderTemplate("templates/listPeople.tpl", contextProviders.PeopleListContextProvider(appContext)),
	)
	router.Methods(constants.GET).Path("/person").HandlerFunc(
		handlers.RenderTemplate("templates/displayPerson.tpl", contextProviders.PersonContextProvider(appContext)),
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
		http.Redirect(writer, request, "/albums", http.StatusMovedPermanently)
	})

	router.Methods(constants.GET).Path("/v1/albums").HandlerFunc(api_handlers.GetAlbumsHandler(appContext))
	router.Methods(constants.GET).Path("/v1/album").HandlerFunc(api_handlers.GetAlbumHandler(appContext))
	router.Methods(constants.POST).Path("/v1/album").HandlerFunc(api_handlers.GetAddAlbumHandler(appContext))

	router.Methods(constants.GET).Path("/v1/people").HandlerFunc(api_handlers.GetPeopleHandler(appContext))
	router.Methods(constants.GET).Path("/v1/people/autocomplete").HandlerFunc(api_handlers.GetPeopleAutocompleteHandler(appContext))
	router.Methods(constants.GET).Path("/v1/person").HandlerFunc(api_handlers.GetPersonHandler(appContext))
	router.Methods(constants.POST).Path("/v1/person").HandlerFunc(api_handlers.GetAddPersonHandler(appContext))

	router.Methods(constants.GET).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceUploadHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource/edit").HandlerFunc(api_handlers.GetResourceEditHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource/preview").HandlerFunc(api_handlers.GetResourceUploadPreviewHandler(appContext))
	router.Methods(constants.POST).Path("/v1/resource/addToAlbum").HandlerFunc(api_handlers.GetAddResourceToAlbumHandler(appContext))

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
