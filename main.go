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
	context2 "mahresources/context"
	"mahresources/models"
	"mahresources/templates/template_context_providers"
	_ "mahresources/templates/template_filters"
	"mahresources/templates/template_handlers"
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

	base := afero.NewBasePathFs(afero.NewOsFs(), "M:\\test")
	layer := afero.NewMemMapFs()
	cachedFS := afero.NewCacheOnReadFs(base, layer, 10*time.Minute)

	httpFs := afero.NewHttpFs(cachedFS)

	r := mux.NewRouter()

	context := context2.NewMahresourcesContext(cachedFS, db)

	r.Methods(constants.GET).Path("/uploadform").HandlerFunc(
		template_handlers.RenderTemplate("templates/upload.tpl", template_context_providers.StaticTemplateCtx),
	)
	r.Methods(constants.GET).Path("/addtoalbum").HandlerFunc(
		template_handlers.RenderTemplate("templates/addtoalbum.tpl", template_context_providers.StaticTemplateCtx),
	)
	r.Methods(constants.GET).Path("/restest").HandlerFunc(
		template_handlers.RenderTemplate("templates/show.tpl", template_context_providers.StaticTemplateCtx),
	)
	r.Methods(constants.GET).Path("/album/new").HandlerFunc(
		template_handlers.RenderTemplate("templates/createAlbum.tpl", template_context_providers.CreateAlbumContextProvider(context)),
	)
	r.Methods(constants.GET).Path("/albums").HandlerFunc(
		template_handlers.RenderTemplate("templates/albums.tpl", template_context_providers.AlbumContextProvider(context)),
	)
	r.Methods(constants.GET).Path("/album").HandlerFunc(
		template_handlers.RenderTemplate("templates/albums.tpl", template_context_providers.StaticTemplateCtx),
	)
	r.Methods(constants.GET).Path("/").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "/albums", http.StatusMovedPermanently)
	})

	r.Methods(constants.GET).Path("/v1/albums").HandlerFunc(api_handlers.GetAlbumsHandler(context))
	r.Methods(constants.GET).Path("/v1/album").HandlerFunc(api_handlers.GetAlbumHandler(context))
	r.Methods(constants.POST).Path("/v1/album").HandlerFunc(api_handlers.GetAddAlbumHandler(context))

	r.Methods(constants.GET).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceHandler(context))
	r.Methods(constants.POST).Path("/v1/resource").HandlerFunc(api_handlers.GetResourceUploadHandler(context))
	r.Methods(constants.POST).Path("/v1/resource/preview").HandlerFunc(api_handlers.GetResourceUploadPreviewHandler(context))
	r.Methods(constants.POST).Path("/v1/resource/addToAlbum").HandlerFunc(api_handlers.GetAddResourceToAlbumHandler(context))

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
