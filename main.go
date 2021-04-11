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

	r := mux.NewRouter()

	appContext := context.NewMahresourcesContext(cachedFS, db)

	r.Methods(constants.GET).Path("/album/new").HandlerFunc(
		handlers.RenderTemplate("templates/createAlbum.tpl", contextProviders.AlbumCreateContextProvider(appContext)),
	)
	r.Methods(constants.GET).Path("/albums").HandlerFunc(
		handlers.RenderTemplate("templates/albums.tpl", contextProviders.AlbumListContextProvider(appContext)),
	)

	r.Methods(constants.GET).Path("/resource/new").HandlerFunc(
		handlers.RenderTemplate("templates/createResource.tpl", contextProviders.ResourceCreateContextProvider(appContext)),
	)
	r.Methods(constants.GET).Path("/resources").HandlerFunc(
		handlers.RenderTemplate("templates/resources.tpl", contextProviders.ResourceListContextProvider(appContext)),
	)

	r.Methods(constants.GET).Path("/person/new").HandlerFunc(
		handlers.RenderTemplate("templates/createPerson.tpl", contextProviders.PersonCreateContextProvider(appContext)),
	)
	r.Methods(constants.GET).Path("/people").HandlerFunc(
		handlers.RenderTemplate("templates/people.tpl", contextProviders.PeopleListContextProvider(appContext)),
	)

	r.Methods(constants.GET).Path("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/albums", http.StatusMovedPermanently)
	})

	r.Methods(constants.GET).Path("/v1/albums").HandlerFunc(api_handlers.GetAlbumsHandler(appContext))
	r.Methods(constants.GET).Path("/v1/album").HandlerFunc(api_handlers.GetAlbumHandler(appContext))
	r.Methods(constants.POST).Path("/v1/album").HandlerFunc(api_handlers.GetAddAlbumHandler(appContext))

	r.Methods(constants.GET).Path("/v1/people").HandlerFunc(api_handlers.GetPeopleHandler(appContext))
	r.Methods(constants.GET).Path("/v1/people/autocomplete").HandlerFunc(api_handlers.GetPeopleAutocompleteHandler(appContext))
	r.Methods(constants.GET).Path("/v1/person").HandlerFunc(api_handlers.GetPersonHandler(appContext))
	r.Methods(constants.POST).Path("/v1/person").HandlerFunc(api_handlers.GetAddPersonHandler(appContext))

	r.Methods(constants.GET).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceHandler(appContext))
	r.Methods(constants.POST).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceUploadHandler(appContext))
	r.Methods(constants.POST).Path("/v1/resource/preview").HandlerFunc(api_handlers.GetResourceUploadPreviewHandler(appContext))
	r.Methods(constants.POST).Path("/v1/resource/addToAlbum").HandlerFunc(api_handlers.GetAddResourceToAlbumHandler(appContext))

	r.Methods(constants.GET).Path("/v1/tags").HandlerFunc(api_handlers.GetTagsHandler(appContext))

	filePathPrefix := "/files/"
	r.PathPrefix(filePathPrefix).Handler(http.StripPrefix(filePathPrefix, http.FileServer(httpFs.Dir("/"))))
	r.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		WriteTimeout: 45 * time.Minute,
		ReadTimeout:  45 * time.Minute,
	}

	log.Fatal(srv.ListenAndServe())
}
