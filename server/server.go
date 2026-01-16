package server

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/spf13/afero"
	"mahresources/application_context"
)

func CreateServer(appContext *application_context.MahresourcesContext, fs afero.Fs, altFs map[string]string) *http.Server {
	router := mux.NewRouter()

	registerRoutes(router, appContext)

	filePathPrefix := "/files/"
	router.PathPrefix(filePathPrefix).Handler(http.StripPrefix(filePathPrefix, http.FileServer(afero.NewHttpFs(fs).Dir("/"))))
	router.PathPrefix("/public/").Handler(http.StripPrefix("/public/", mimeTypeHandler(http.FileServer(http.Dir("./public")))))

	for key, systemName := range altFs {
		system := createCachedStorage(systemName)
		pathKey := fmt.Sprintf("/%v/", key)
		router.PathPrefix(pathKey).Handler(http.StripPrefix(pathKey, http.FileServer(afero.NewHttpFs(system).Dir("/"))))
	}

	return &http.Server{
		Addr:         appContext.Config.BindAddress,
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

// mimeTypeHandler wraps a handler to set correct Content-Type headers
func mimeTypeHandler(next http.Handler) http.Handler {
	mimeTypes := map[string]string{
		".css":   "text/css; charset=utf-8",
		".js":    "application/javascript; charset=utf-8",
		".mjs":   "application/javascript; charset=utf-8",
		".json":  "application/json; charset=utf-8",
		".svg":   "image/svg+xml",
		".png":   "image/png",
		".jpg":   "image/jpeg",
		".jpeg":  "image/jpeg",
		".gif":   "image/gif",
		".ico":   "image/x-icon",
		".woff":  "font/woff",
		".woff2": "font/woff2",
		".ttf":   "font/ttf",
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ext := strings.ToLower(filepath.Ext(r.URL.Path))
		if mimeType, ok := mimeTypes[ext]; ok {
			next.ServeHTTP(&mimeTypeResponseWriter{ResponseWriter: w, mimeType: mimeType}, r)
		} else {
			next.ServeHTTP(w, r)
		}
	})
}

// mimeTypeResponseWriter wraps ResponseWriter to force a specific Content-Type
type mimeTypeResponseWriter struct {
	http.ResponseWriter
	mimeType    string
	wroteHeader bool
}

func (w *mimeTypeResponseWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.ResponseWriter.Header().Set("Content-Type", w.mimeType)
		w.wroteHeader = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *mimeTypeResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}
