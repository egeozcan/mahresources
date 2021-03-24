package main

import (
	"github.com/flosch/pongo2/v4"
	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"log"
	"net/http"
	"os"
	"time"
)

const MaxResults = 10
const JSON = "application/json"
const POST = "POST"
const GET = "GET"

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

	err = db.AutoMigrate(&Resource{})
	if err != nil {
		panic("failed to migrate Resource")
	}
	err = db.AutoMigrate(&Album{})
	if err != nil {
		panic("failed to migrate Album")
	}
	err = db.AutoMigrate(&Tag{})
	if err != nil {
		panic("failed to migrate Tag")
	}
	err = db.AutoMigrate(&Person{})
	if err != nil {
		panic("failed to migrate Person")
	}

	base := afero.NewBasePathFs(afero.NewOsFs(), "M:\\test")
	layer := afero.NewMemMapFs()
	cachedFS := afero.NewCacheOnReadFs(base, layer, 10*time.Minute)

	httpFs := afero.NewHttpFs(cachedFS)

	r := mux.NewRouter()

	baseTemplateContext := pongo2.Context{
		"title": "mahresources",
	}
	staticTemplateCtx := func(request *http.Request) pongo2.Context { return baseTemplateContext }

	context := newMahresourcesContext(cachedFS, db)

	r.Methods(GET).Path("/uploadform").HandlerFunc(renderTemplate("templates/upload.tpl", staticTemplateCtx))
	r.Methods(GET).Path("/addtoalbum").HandlerFunc(renderTemplate("templates/addtoalbum.tpl", staticTemplateCtx))
	r.Methods(GET).Path("/restest").HandlerFunc(renderTemplate("templates/show.tpl", staticTemplateCtx))
	r.Methods(GET).Path("/album/new").HandlerFunc(renderTemplate("templates/createAlbum.tpl", staticTemplateCtx))
	r.Methods(GET).Path("/albums").HandlerFunc(renderTemplate("templates/albums.tpl", func(request *http.Request) pongo2.Context {
		offset := (getIntQueryParameter(request, "page", 1) - 1) * MaxResults
		albums, err := context.getAlbums(int(offset), MaxResults)

		if err != nil {
			return baseTemplateContext
		}

		return pongo2.Context{
			"albums": albums,
		}.Update(baseTemplateContext)
	}))

	r.Methods(GET).Path("/v1/albums").HandlerFunc(getAlbumsHandler(context))
	r.Methods(GET).Path("/v1/album").HandlerFunc(getAlbumHandler(context))
	r.Methods(POST).Path("/v1/album").HandlerFunc(getAddAlbumHandler(context))

	r.Methods(GET).Path("/v1/resource").HandlerFunc(getResourceHandler(context))
	r.Methods(POST).Path("/v1/resource").HandlerFunc(getResourceUploadHandler(context))
	r.Methods(POST).Path("/v1/resource/preview").HandlerFunc(getResourceUploadPreviewHandler(context))
	r.Methods(POST).Path("/v1/resource/addToAlbum").HandlerFunc(getAddResourceToAlbumHandler(context))

	filePathPrefix := "/files/"
	r.PathPrefix(filePathPrefix).Handler(http.StripPrefix(filePathPrefix, http.FileServer(httpFs.Dir("/"))))

	srv := &http.Server{
		Addr:         ":8080",
		Handler:      r,
		WriteTimeout: 45 * time.Minute,
		ReadTimeout:  45 * time.Minute,
	}

	log.Fatal(srv.ListenAndServe())
}

func renderTemplate(templateName string, templateContext func(request *http.Request) pongo2.Context) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		var tplExample = pongo2.Must(pongo2.FromFile(templateName))
		err := tplExample.ExecuteWriter(templateContext(request), writer)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	}
}
