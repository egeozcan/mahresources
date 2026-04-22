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
	"mahresources/server/template_handlers"

	"github.com/flosch/pongo2/v4"
)

// BuildPrimaryRouter assembles the primary mux.Router with every route, the
// 404 handler, and the file-system PathPrefix handlers registered. It is the
// shared router-construction path used by both CreateServer (which wraps it
// with security-headers middleware and embeds it in an http.Server) and the
// OpenAPI drift test (which walks the routes directly to detect missing spec
// entries). Exposing the router separately keeps the middleware wrapper
// BH-032 introduced from hiding the routes behind a http.HandlerFunc the
// drift test can't traverse.
func BuildPrimaryRouter(appContext *application_context.MahresourcesContext, fs afero.Fs, altFs map[string]string) *mux.Router {
	router := mux.NewRouter()

	registerRoutes(router, appContext)

	// Build a context enricher that adds plugin info to the 404 page,
	// mirroring what wrapContextWithPlugins does for normal routes.
	var notFoundEnricher func(ctx pongo2.Context) pongo2.Context
	if pm := appContext.PluginManager(); pm != nil {
		notFoundEnricher = func(ctx pongo2.Context) pongo2.Context {
			ctx["_pluginManager"] = pm
			ctx["pluginMenuItems"] = pm.GetMenuItems()
			ctx["hasPluginManager"] = true
			return ctx
		}
	}
	router.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		template_handlers.RenderNotFound(w, r, notFoundEnricher)
	})

	filePathPrefix := "/files/"
	router.PathPrefix(filePathPrefix).Handler(http.StripPrefix(filePathPrefix, http.FileServer(afero.NewHttpFs(fs).Dir("/"))))
	router.PathPrefix("/public/").Handler(http.StripPrefix("/public/", mimeTypeHandler(http.FileServer(http.Dir("./public")))))

	for key, systemName := range altFs {
		system := createCachedStorage(systemName)
		pathKey := fmt.Sprintf("/%v/", key)
		router.PathPrefix(pathKey).Handler(http.StripPrefix(pathKey, http.FileServer(afero.NewHttpFs(system).Dir("/"))))
	}

	return router
}

func CreateServer(appContext *application_context.MahresourcesContext, fs afero.Fs, altFs map[string]string) *http.Server {
	router := BuildPrimaryRouter(appContext, fs, altFs)

	// BH-032: wrap the primary router with a CSP-free subset of the share
	// server's security headers (clickjacking, MIME sniffing, Referer,
	// HSTS). The strict default-src 'self' CSP the share server ships blocks
	// enough primary-server template behaviour (inline scripts emitted by
	// shortcodes, plugin-provided <script>/<style> blocks, the a11y test
	// suite detected Alpine initialization timing regressions) that applying
	// it unmodified here would be net-negative for admin UX. A tighter
	// primary-server CSP is tracked as a separate follow-up so it can be
	// audited and rolled out on its own cadence, without reverting the
	// share-server hardening BH-032 shipped.
	return &http.Server{
		Addr:         appContext.Config.BindAddress,
		Handler:      withPrimarySecurityHeaders(router),
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
