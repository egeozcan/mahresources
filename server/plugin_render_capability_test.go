package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Rendering a plugin block or display is a read, like GET block render, so a
// guest may do it on a plugin an operator opened. Only the exact routes gorilla
// sends to the render handlers qualify: a ".json" variant or an extra segment
// falls through to the plugin JSON API catch-all, where a POST is a write.
func TestPluginRenderPostsAreReads(t *testing.T) {
	reads := []string{
		"/v1/plugins/block/render-batch",
		"/v1/plugins/x/display/render",
	}
	for _, p := range reads {
		if got := requiredCapability(http.MethodPost, p); got != capRead {
			t.Errorf("requiredCapability(POST %s) = %v, want capRead", p, got)
		}
	}

	writes := []string{
		"/v1/plugins/x/some/action",
		"/v1/plugins/block/render-batch.json",
		"/v1/plugins/x/display/render.json",
		"/v1/plugins/x/y/display/render",
		"/v1/plugins//display/render",
		"/v1/plugins/x/display/render/extra",
	}
	for _, p := range writes {
		if got := requiredCapability(http.MethodPost, p); got != capWrite {
			t.Errorf("requiredCapability(POST %s) = %v, want capWrite", p, got)
		}
	}
}

// Render Lua can reach mah.db writers, so the render POSTs keep their CSRF check
// even though they are classified as reads.
func TestPluginRenderPostsAreNotCSRFExempt(t *testing.T) {
	for _, p := range []string{"/v1/plugins/block/render-batch", "/v1/plugins/x/display/render"} {
		if csrfExempt(httptest.NewRequest(http.MethodPost, p, nil)) {
			t.Errorf("POST %s must not be CSRF-exempt", p)
		}
	}
}
