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

	base := afero.NewBasePathFs(afero.NewOsFs(), "G:\\mnt\\md0\\enc\\photos")
	layer := afero.NewMemMapFs()
	cachedFS := afero.NewCacheOnReadFs(base, layer, 10 * time.Minute)

	httpFs := afero.NewHttpFs(cachedFS)

	r := mux.NewRouter()

	r.Methods(GET).Path("/uploadform").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var tplExample = pongo2.Must(pongo2.FromFile("templates/upload.tpl"))
		err := tplExample.ExecuteWriter(pongo2.Context{}, writer)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Methods(GET).Path("/addtoalbum").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var tplExample = pongo2.Must(pongo2.FromFile("templates/addtoalbum.tpl"))
		err := tplExample.ExecuteWriter(pongo2.Context{}, writer)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	})

	r.Methods(GET).Path("/restest").HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		var tplExample = pongo2.Must(pongo2.FromFile("templates/show.tpl"))
		err := tplExample.ExecuteWriter(pongo2.Context{}, writer)
		if err != nil {
			http.Error(writer, err.Error(), http.StatusInternalServerError)
		}
	})

	context := newMahresourcesContext(cachedFS, db)

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
		Addr:    ":8080",
		Handler: r,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	log.Fatal(srv.ListenAndServe())
}