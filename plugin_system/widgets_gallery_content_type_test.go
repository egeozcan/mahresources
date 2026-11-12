package plugin_system

import (
	"context"
	"encoding/json"
	"html"
	"strings"
	"testing"
)

type evilContentTypeQuerier struct{ mockQuerier }

func (evilContentTypeQuerier) QueryResources(filter map[string]any) ([]map[string]any, error) {
	return []map[string]any{{
		"id": float64(7), "name": "x.png", "hash": "abc",
		"content_type": "image/png');window.__xss=1;('",
	}}, nil
}

// A content type is data. Lua has no JS-string encoder, so the gallery must
// hand it to the lightbox through the data attribute, never inside a JS literal.
func TestWidgetsGalleryContentTypeNotInJSString(t *testing.T) {
	pm, err := NewPluginManager(bundledPluginDir(t))
	if err != nil {
		t.Fatalf("NewPluginManager: %v", err)
	}
	defer pm.Close()
	pm.SetEntityQuerier(&evilContentTypeQuerier{})
	if err := pm.EnablePlugin("widgets"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	rendered, err := pm.RenderShortcode(context.Background(), "widgets", "plugin:widgets:gallery",
		"group", 1, json.RawMessage(`{}`), map[string]string{}, nil, "", false)
	if err != nil {
		t.Fatalf("RenderShortcode: %v", err)
	}
	attr := func(name string) string {
		i := strings.Index(rendered, name+`="`)
		if i < 0 {
			t.Fatalf("no %s attribute in %q", name, rendered)
		}
		rest := rendered[i+len(name)+2:]
		return html.UnescapeString(rest[:strings.Index(rest, `"`)])
	}
	if got, want := attr("@click.prevent"), "$store.lightbox.openFromClick($event, 7, $el.dataset.contentType)"; got != want {
		t.Errorf("click handler = %q, want %q", got, want)
	}
	if got, want := attr("data-content-type"), "image/png');window.__xss=1;('"; got != want {
		t.Errorf("data-content-type = %q, want %q", got, want)
	}
	if got := attr("data-resource-hash"); got != "abc" {
		t.Errorf("data-resource-hash = %q, want abc", got)
	}
}
