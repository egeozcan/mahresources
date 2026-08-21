package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	filepath "path/filepath"
	"strings"

	"github.com/gorilla/mux"
	"mahresources/application_context"
	"mahresources/plugin_system"
)

// pluginAssetsSegment is the first path segment a plugin's static files live
// under, and is therefore reserved: a plugin page registered at "public" would
// be shadowed by this route, which is registered before the page catch-all.
const pluginAssetsSegment = "public"

// pluginAssetsDir is the directory inside a plugin's own folder that is served.
const pluginAssetsDir = "public"

// registerPluginAssetRoute serves <pluginDir>/public at
// /plugins/<name>/public/*, for enabled plugins only.
//
// Item 4.6. A plugin could render HTML into six slots and could not ship a line
// of its own JavaScript, so browser code lived inside Lua long-strings and was
// re-sent on every page render -- one bundled plugin re-emits 5 KB of unchanging
// QR-generator script per render, behind the VM lock, to produce a string that
// never changes.
//
// No new capability, deliberately. A capability names a power the plugin gains,
// and serving a file gives it none: the Lua VM has no filesystem reach at all
// (no io, no os, loadfile/dofile removed), so the plugin cannot read these files
// -- the host does, out of a directory whoever installed the plugin already
// wrote. The power that matters, arbitrary script in the app's origin on every
// page, is already consented under CapInject, whose label reads "Inject HTML and
// scripts into every page" and whose RenderSlot concatenates raw strings, so a
// plugin holding it can already emit <script src="https://evil.example/x.js">.
// A same-origin file the operator can read on disk is strictly narrower. Gating
// it behind a new name would be the first capability granting less than one its
// holder must already have. It is also inert without one: an asset matters only
// if something references it, and every referencing surface (mah.inject,
// mah.page, shortcodes, block and display types) is already capability-gated.
//
// The mount point is the design, not a detail. pluginCodePathName reads the
// plugin name out of the first segment after /plugins/, so the per-plugin
// AllowScopedPrincipals deny governs these files with no second copy of the
// predicate, and requiredCapability classifies a GET here as capRead -- the same
// class as the render seams. So the <script> tag and the file it names are gated
// by one rule: a confined principal on a closed plugin never receives the tag
// and is refused the file; on an opened plugin it gets both.
//
// It must NOT be mounted under /public/, which is auth-exempt (isPublicPath) and
// CORS-wildcarded, and would publish plugin assets unauthenticated and
// cross-origin, escaping the scoped-access toggle entirely.
func registerPluginAssetRoute(router *mux.Router, appContext *application_context.MahresourcesContext) {
	pm := appContext.PluginManager()
	if pm == nil {
		return
	}

	router.Methods(http.MethodGet, http.MethodHead).
		Path("/plugins/{name}/" + pluginAssetsSegment + "/{path:.*}").
		HandlerFunc(servePluginAsset(pm))
}

func servePluginAsset(pm *plugin_system.PluginManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		name := vars["name"]
		rel := vars["path"]

		dir, ok := pluginAssetsRoot(pm, name)
		if !ok {
			http.NotFound(w, r)
			return
		}

		// os.OpenRoot contains both `..` and symlink escape, which matters here
		// in a way it does not for the app's own static files: a plugin folder
		// is third-party content and may contain a symlink pointing anywhere.
		// This is the only filesystem surface a plugin's own directory has ever
		// had, so the containment is the feature's cost, not an afterthought.
		root, err := os.OpenRoot(dir)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer root.Close()

		clean := path.Clean("/" + rel)
		clean = strings.TrimPrefix(clean, "/")
		if clean == "" || clean == "." {
			http.NotFound(w, r)
			return
		}

		file, err := root.Open(clean)
		if err != nil {
			if !errors.Is(err, fs.ErrNotExist) {
				// A traversal attempt or a permission problem is not something
				// to describe to the caller; both are "no such asset".
				http.NotFound(w, r)
				return
			}
			http.NotFound(w, r)
			return
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil || info.IsDir() {
			// No directory listings: a plugin's folder is not a browsable tree,
			// and an index would expose file names the author never published.
			http.NotFound(w, r)
			return
		}

		http.ServeContent(w, r, info.Name(), info.ModTime(), file)
	}
}

// pluginAssetsRoot resolves an enabled plugin's asset directory.
//
// Enablement is checked per request rather than at registration, because routes
// register once at boot and a plugin can be enabled or disabled at any time. A
// disabled plugin serves nothing, which keeps "disabled" meaning the same thing
// here as it does at every other plugin surface.
func pluginAssetsRoot(pm *plugin_system.PluginManager, name string) (string, bool) {
	if name == "" || !pm.IsEnabled(name) {
		return "", false
	}
	for _, p := range pm.DiscoveredPlugins() {
		if p.Name == name {
			if p.Dir == "" {
				return "", false
			}
			return filepath.Join(p.Dir, pluginAssetsDir), true
		}
	}
	return "", false
}
