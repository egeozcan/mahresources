package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gorilla/mux"
	"mahresources/plugin_system"
)

// writeAssetPlugin lays out a plugin directory with a public/ folder.
func writeAssetPlugin(t *testing.T, root, name string, files map[string]string) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "public", "nested"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := "plugin = { name = \"" + name + "\", version = \"1.0\", description = \"assets\" }\nfunction init() end\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(src), 0o644); err != nil {
		t.Fatalf("write plugin.lua: %v", err)
	}
	for rel, body := range files {
		p := filepath.Join(dir, "public", rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func assetRouter(t *testing.T, pm *plugin_system.PluginManager) *mux.Router {
	t.Helper()
	r := mux.NewRouter()
	r.Methods(http.MethodGet, http.MethodHead).
		Path("/plugins/{name}/" + pluginAssetsSegment + "/{path:.*}").
		HandlerFunc(servePluginAsset(pm))
	return r
}

func getAsset(t *testing.T, r *mux.Router, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr
}

// Item 4.6. A plugin's own public/ directory is served at
// /plugins/<name>/public/*, so browser code stops living inside Lua strings.
func TestPluginAssets_ServedForAnEnabledPlugin(t *testing.T) {
	root := t.TempDir()
	writeAssetPlugin(t, root, "assets-plugin", map[string]string{
		"app.js":         "console.log('hi')",
		"nested/deep.js": "console.log('deep')",
	})

	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("assets-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	r := assetRouter(t, pm)

	rr := getAsset(t, r, "/plugins/assets-plugin/public/app.js")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET app.js = %d, want 200", rr.Code)
	}
	if got := rr.Body.String(); got != "console.log('hi')" {
		t.Errorf("body = %q", got)
	}
	// Served with a real content type, or a browser will refuse to execute it.
	if ct := rr.Header().Get("Content-Type"); ct == "" {
		t.Error("no Content-Type on a served asset")
	}

	if rr := getAsset(t, r, "/plugins/assets-plugin/public/nested/deep.js"); rr.Code != http.StatusOK {
		t.Errorf("GET nested/deep.js = %d, want 200", rr.Code)
	}
}

// Disabled means disabled here too, checked per request: routes register once at
// boot and a plugin can be turned off at any time after that.
func TestPluginAssets_DisabledPluginServesNothing(t *testing.T) {
	root := t.TempDir()
	writeAssetPlugin(t, root, "assets-plugin", map[string]string{"app.js": "x"})

	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	r := assetRouter(t, pm)

	// Never enabled.
	if rr := getAsset(t, r, "/plugins/assets-plugin/public/app.js"); rr.Code != http.StatusNotFound {
		t.Errorf("disabled plugin served its asset: %d", rr.Code)
	}

	if err := pm.EnablePlugin("assets-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	if rr := getAsset(t, r, "/plugins/assets-plugin/public/app.js"); rr.Code != http.StatusOK {
		t.Fatalf("enabled plugin did not serve its asset: %d", rr.Code)
	}

	if err := pm.DisablePlugin("assets-plugin"); err != nil {
		t.Fatalf("DisablePlugin: %v", err)
	}
	if rr := getAsset(t, r, "/plugins/assets-plugin/public/app.js"); rr.Code != http.StatusNotFound {
		t.Errorf("asset still served after disable: %d", rr.Code)
	}
}

// This is the only filesystem surface a plugin directory has ever had, and a
// plugin folder is third-party content: it can contain a symlink pointing
// anywhere. os.OpenRoot contains both traversal and symlink escape.
func TestPluginAssets_RefusesTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	writeAssetPlugin(t, root, "assets-plugin", map[string]string{"app.js": "x"})

	secret := filepath.Join(root, "outside-secret.txt")
	if err := os.WriteFile(secret, []byte("TOP SECRET"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	// A symlink inside public/ pointing out of it.
	link := filepath.Join(root, "assets-plugin", "public", "escape.txt")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("assets-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	r := assetRouter(t, pm)

	for _, p := range []string{
		"/plugins/assets-plugin/public/escape.txt",
		"/plugins/assets-plugin/public/..%2f..%2foutside-secret.txt",
		"/plugins/assets-plugin/public/../../outside-secret.txt",
		"/plugins/assets-plugin/public/../plugin.lua",
	} {
		rr := getAsset(t, r, p)
		if rr.Code == http.StatusOK && rr.Body.String() != "" {
			t.Errorf("%s served %d with a body: %q", p, rr.Code, rr.Body.String())
		}
		if rr.Body.String() == "TOP SECRET" {
			t.Errorf("%s escaped the plugin's public directory", p)
		}
	}
}

// No directory listings: a plugin folder is not a browsable tree, and an index
// would expose file names the author never published.
func TestPluginAssets_NoDirectoryListingAndNoUnknownPlugin(t *testing.T) {
	root := t.TempDir()
	writeAssetPlugin(t, root, "assets-plugin", map[string]string{"nested/deep.js": "x"})

	pm, err := plugin_system.NewPluginManager(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pm.Close)
	if err := pm.EnablePlugin("assets-plugin"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	r := assetRouter(t, pm)

	if rr := getAsset(t, r, "/plugins/assets-plugin/public/nested/"); rr.Code == http.StatusOK {
		t.Errorf("a directory was served: %d body=%q", rr.Code, rr.Body.String())
	}
	if rr := getAsset(t, r, "/plugins/assets-plugin/public/"); rr.Code == http.StatusOK {
		t.Errorf("the asset root was served: %d", rr.Code)
	}
	if rr := getAsset(t, r, "/plugins/no-such-plugin/public/app.js"); rr.Code != http.StatusNotFound {
		t.Errorf("unknown plugin = %d, want 404", rr.Code)
	}
}

// The two properties the mount point rests on, pinned because the whole reason
// for choosing this path rather than a new capability is that they hold.
//
// If either moves, plugin assets stop being governed by the per-plugin
// scoped-access toggle and start needing a second copy of that predicate — which
// is the drift this tree names as the reason to have one rule, not two.
func TestPluginAssetPathIsGovernedByThePerPluginDeny(t *testing.T) {
	name, ok := pluginCodePathName("/plugins/assets-plugin/public/app.js")
	if !ok || name != "assets-plugin" {
		t.Errorf("pluginCodePathName = %q, %v; the per-plugin AllowScopedPrincipals deny reads the name from here", name, ok)
	}

	if got := requiredCapability(http.MethodGet, "/plugins/assets-plugin/public/app.js"); got != capRead {
		t.Errorf("requiredCapability = %v, want capRead: an asset must be classified like the render seams that emit its <script> tag", got)
	}
}
