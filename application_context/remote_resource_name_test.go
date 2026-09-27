package application_context

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"mahresources/models/query_models"
)

// A resource fetched with no name of its own is named after what the server
// delivered, the same way a background download is: the Content-Disposition
// filename, or the decoded last path segment of the URL the response came from.
func TestAddRemoteResource_NamesTheResourceAfterWhatTheServerDelivered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/redirect":
			http.Redirect(w, r, "/img/landed-here.png?size=large", http.StatusFound)
			return
		case "/report":
			w.Header().Set("Content-Disposition", `attachment; filename="Report Final.pdf"`)
		}
		_, _ = w.Write([]byte("foreground body of " + r.URL.String()))
	}))
	defer server.Close()

	cases := []struct {
		path string
		want string
	}{
		{"/img/Caf%C3%A9%20terrace.png?w=96&h=64", "Café terrace.png"},
		{"/report?id=7", "Report Final.pdf"},
		{"/redirect", "landed-here.png"},
	}
	for i, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			ctx := createCoverageTestContext(t, "remote_name_"+strconv.Itoa(i))
			ctx.Config.RemoteResourceConnectTimeout = 5 * time.Second
			ctx.Config.RemoteResourceIdleTimeout = 5 * time.Second
			ctx.Config.RemoteResourceOverallTimeout = 10 * time.Second

			res, err := ctx.AddRemoteResource(context.Background(), &query_models.ResourceFromRemoteCreator{URL: server.URL + tc.path})
			if err != nil {
				t.Fatalf("AddRemoteResource: %v", err)
			}
			if res.Name != tc.want {
				t.Fatalf("named the resource %q, want %q", res.Name, tc.want)
			}
		})
	}
}
