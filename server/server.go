package server

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"mahresources/application_context"
	"net/http"
	"time"
)

func CreateServer(appContext *application_context.MahresourcesContext, fs afero.Fs, altFs map[string]string) *http.Server {
	router := mux.NewRouter()

	registerRoutes(router, appContext)

	filePathPrefix := "/files/"
	router.PathPrefix(filePathPrefix).Handler(http.StripPrefix(filePathPrefix, http.FileServer(afero.NewHttpFs(fs).Dir("/"))))
	router.PathPrefix("/public/").Handler(http.StripPrefix("/public/", http.FileServer(http.Dir("./public"))))

	for key, systemName := range altFs {
		system := createCachedStorage(systemName)
		pathKey := fmt.Sprintf("/%v/", key)
		router.PathPrefix(pathKey).Handler(http.StripPrefix(pathKey, http.FileServer(afero.NewHttpFs(system).Dir("/"))))
	}

	return &http.Server{
		Addr:         ":8080",
		Handler:      router,
		WriteTimeout: 45 * time.Minute,
		ReadTimeout:  45 * time.Minute,
	}
}

func createCachedStorage(path string) afero.Fs {
	base := afero.NewBasePathFs(afero.NewOsFs(), path)
	layer := afero.NewMemMapFs()
	return afero.NewCacheOnReadFs(base, layer, 10*time.Minute)
}
