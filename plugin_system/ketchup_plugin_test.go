package plugin_system

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// ketchupQuerier serves the resources and groups the ketchup plugin's page and
// sidebar look up. Like the real adapter, a missing entity is (nil, nil); id
// 500 stands for a read that failed.
type ketchupQuerier struct {
	mockQuerier
}

func (q *ketchupQuerier) GetResourceData(id uint) (map[string]any, error) {
	switch id {
	case 7:
		return map[string]any{"id": float64(7), "name": "</script><img src=x onerror=alert(1)>", "content_type": "image/png", "hash": "abc123", "owner_id": float64(3)}, nil
	case 8:
		return map[string]any{"id": float64(8), "name": "anim.gif", "content_type": "image/gif"}, nil
	case 9:
		return map[string]any{"id": float64(9), "name": "photo.bmp", "content_type": "image/bmp", "hash": "def"}, nil
	case 500:
		return nil, fmt.Errorf("database is down")
	}
	return nil, nil
}

func (q *ketchupQuerier) GetGroupData(id uint) (map[string]any, error) {
	switch id {
	case 3:
		return map[string]any{"id": float64(3), "name": "Sketches"}, nil
	case 500:
		return nil, fmt.Errorf("database is down")
	}
	return nil, nil
}

func ketchupPlugin(t *testing.T) *PluginManager {
	t.Helper()
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	t.Cleanup(pm.Close)
	pm.SetEntityQuerier(&ketchupQuerier{})
	if err := pm.EnablePlugin("ketchup"); err != nil {
		t.Fatalf("EnablePlugin(ketchup): %v", err)
	}
	return pm
}

func ketchupPage(t *testing.T, pm *PluginManager, query map[string]any) string {
	t.Helper()
	html, err := pm.HandlePage(context.Background(), "ketchup", "edit", PageContext{Path: "edit", Method: "GET", Query: query})
	if err != nil {
		t.Fatalf("HandlePage(edit, %v): %v", query, err)
	}
	return html
}

// ketchupConfig decodes the JSON the page hands editor.js.
func ketchupConfig(t *testing.T, html string) map[string]any {
	t.Helper()
	const open = `<script type="application/json" id="ketchup-config">`
	start := strings.Index(html, open)
	if start < 0 {
		t.Fatalf("page has no editor config:\n%s", html)
	}
	rest := html[start+len(open):]
	end := strings.Index(rest, "</script>")
	if end < 0 {
		t.Fatal("editor config is not closed")
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(rest[:end]), &config); err != nil {
		t.Fatalf("editor config is not JSON: %v", err)
	}
	return config
}

func TestKetchupEditPageForAnImage(t *testing.T) {
	pm := ketchupPlugin(t)
	html := ketchupPage(t, pm, map[string]any{"id": "7"})

	config := ketchupConfig(t, html)
	if config["mode"] != "edit" || config["saveType"] != "image/png" {
		t.Errorf("config = %v, want edit mode saving PNG", config)
	}
	resource := config["resource"].(map[string]any)
	// The name reaches the editor intact, through JSON, and never as markup.
	if resource["name"] != "</script><img src=x onerror=alert(1)>" || resource["ownerId"] != float64(3) {
		t.Errorf("resource = %v", resource)
	}
	if strings.Contains(html, "<img src=x") {
		t.Error("the resource name escaped the config script as markup")
	}
	if config["fileUrl"] != "/v1/resource/view?id=7&v=abc123" {
		t.Errorf("fileUrl = %v", config["fileUrl"])
	}
	if !strings.Contains(html, `<drawing-app class="ketchup-app" embedded`) {
		t.Error("the page does not embed the editor")
	}
}

func TestKetchupSavesBMPAsPNG(t *testing.T) {
	config := ketchupConfig(t, ketchupPage(t, ketchupPlugin(t), map[string]any{"id": "9"}))
	if config["saveType"] != "image/png" {
		t.Errorf("saveType = %v, want image/png (browsers cannot encode BMP)", config["saveType"])
	}
}

func TestKetchupRefusesWhatItCannotEdit(t *testing.T) {
	pm := ketchupPlugin(t)
	for _, tc := range []struct {
		query map[string]any
		want  string
	}{
		{map[string]any{"id": "8"}, "Ketchup cannot edit this resource"},
		{map[string]any{"id": "404"}, "Resource not found"},
		{map[string]any{"id": "abc"}, "Invalid resource id"},
		{map[string]any{"id": "-1"}, "Invalid resource id"},
		{map[string]any{"id": "0x10"}, "Invalid resource id"},
		{map[string]any{"id": "1e3"}, "Invalid resource id"},
		{map[string]any{"id": "99999999999999999999"}, "Invalid resource id"},
		// A failed read is not reported as a missing resource.
		{map[string]any{"id": "500"}, "Resource could not be read"},
		{map[string]any{"owner": "404"}, "Group not found"},
		{map[string]any{"owner": "x"}, "Invalid group id"},
		{map[string]any{"owner": "500"}, "Group could not be read"},
	} {
		html := ketchupPage(t, pm, tc.query)
		if !strings.Contains(html, tc.want) || strings.Contains(html, "<drawing-app") {
			t.Errorf("%v: want only a %q notice, got:\n%s", tc.query, tc.want, html)
		}
	}
}

func TestKetchupNewImagePage(t *testing.T) {
	pm := ketchupPlugin(t)

	config := ketchupConfig(t, ketchupPage(t, pm, nil))
	if config["mode"] != "new" || config["width"] != float64(1200) || config["height"] != float64(800) || config["owner"] != nil {
		t.Errorf("default new-image config = %v", config)
	}

	config = ketchupConfig(t, ketchupPage(t, pm, map[string]any{"owner": "3"}))
	owner, _ := config["owner"].(map[string]any)
	if owner["id"] != float64(3) || owner["name"] != "Sketches" {
		t.Errorf("owner = %v", config["owner"])
	}

	pm.SetPluginSettings("ketchup", map[string]any{"new_width": float64(640), "new_height": float64(99999)})
	config = ketchupConfig(t, ketchupPage(t, pm, nil))
	if config["width"] != float64(640) || config["height"] != float64(800) {
		t.Errorf("sized config = %v, want the width setting and the default for an out-of-range height", config)
	}
}

func TestKetchupSidebarLinks(t *testing.T) {
	pm := ketchupPlugin(t)
	render := func(slot string, id int) string {
		return pm.RenderSlot(context.Background(), slot, map[string]any{"entity_id": float64(id)}, nil)
	}
	if got := render("resource_detail_sidebar", 7); !strings.Contains(got, `href="/plugins/ketchup/edit?id=7"`) {
		t.Errorf("image resource sidebar = %q", got)
	}
	if got := render("resource_detail_sidebar", 8); strings.Contains(got, "ketchup") {
		t.Errorf("a GIF offers the editor: %q", got)
	}
	if got := render("group_detail_sidebar", 3); !strings.Contains(got, `href="/plugins/ketchup/edit?owner=3"`) {
		t.Errorf("group sidebar = %q", got)
	}
}

func TestKetchupCardActionOffersEditableImagesOnly(t *testing.T) {
	pm := ketchupPlugin(t)
	var edit *ActionRegistration
	for _, a := range pm.GetActions("resource", nil) {
		if a.PluginName == "ketchup" && a.ID == "edit" {
			edit = &a
		}
	}
	if edit == nil {
		t.Fatal("no edit action registered")
	}
	types := slices.Clone(edit.Filters.ContentTypes)
	slices.Sort(types)
	if want := []string{"image/bmp", "image/jpeg", "image/png", "image/webp"}; !slices.Equal(types, want) {
		t.Errorf("content types = %v, want %v", types, want)
	}
	if !slices.Equal(edit.Placement, []string{"card"}) {
		t.Errorf("placement = %v; the detail page links from its sidebar instead", edit.Placement)
	}
}

// The e2e server loads plugins from e2e/test-plugins, so the suite there tests
// a copy; this keeps the copy the plugin that ships.
func TestKetchupE2EFixtureMatchesTheBundledPlugin(t *testing.T) {
	bundledRoot := filepath.Join("..", "plugins", "ketchup")
	fixtureRoot := filepath.Join("..", "e2e", "test-plugins", "ketchup")
	seen := map[string]bool{}
	err := filepath.WalkDir(bundledRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(bundledRoot, path)
		seen[relative] = true
		bundled, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		fixture, err := os.ReadFile(filepath.Join(fixtureRoot, relative))
		if err != nil {
			t.Errorf("e2e fixture is missing %s", relative)
			return nil
		}
		if !bytes.Equal(bundled, fixture) {
			t.Errorf("e2e fixture differs from the bundled plugin: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(fixtureRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, _ := filepath.Rel(fixtureRoot, path)
		if !seen[relative] {
			t.Errorf("e2e fixture has a file the bundled plugin does not: %s", relative)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The bundle is vendored, so its first line has to say which ketchup it is.
// Only scripts/update-ketchup.sh writes that line; a hand-copied build lacks it.
func TestKetchupBundleNamesTheCommitItWasBuiltFrom(t *testing.T) {
	bundle, err := os.ReadFile(filepath.Join("..", "plugins", "ketchup", "public", "ketchup.js"))
	if err != nil {
		t.Fatal(err)
	}
	first, _, _ := bytes.Cut(bundle, []byte("\n"))
	if !regexp.MustCompile(`^/\*! ketchup [0-9a-f]{40} \(`).Match(first) {
		t.Errorf("ketchup.js does not start with the commit stamp; rebuild it with scripts/update-ketchup.sh, got %q", first)
	}
}
