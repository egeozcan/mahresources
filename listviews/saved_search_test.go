package listviews

import (
	"net/url"
	"reflect"
	"testing"
)

func TestSavedSearchAllViews(t *testing.T) {
	for _, view := range views {
		t.Run(view.Path, func(t *testing.T) {
			got, registered, err := NormalizeSavedURL(view.Path + "?Name=needle&page=7&Page=8&redirect=%2Fnote&SortBy=Name&SortBy=CreatedAt+desc")
			if err != nil || registered == nil || *registered != view {
				t.Fatalf("registry round trip: %q %+v %v", got, registered, err)
			}
			u, _ := url.Parse(got)
			q := u.Query()
			if q.Has("page") || q.Has("Page") || q.Has("redirect") || q.Get("Name") != "needle" ||
				!reflect.DeepEqual(q["SortBy"], []string{"Name", "CreatedAt desc"}) {
				t.Fatalf("lost search state: %s", got)
			}
		})
	}
}

func TestSavedSearchFilters(t *testing.T) {
	q := url.Values{
		"Tags": {"7", "3"}, "SortBy": {"created_at desc", "name"},
		"MetaQuery.key": {"rating", "author"}, "MetaQuery.value": {"5", "A&B"},
		"mrql":         {`created > -7d AND (name ~ "*雪*" OR name = "quoted")`},
		"timelineMode": {"updated"}, "timelineGranularity": {"week"}, "timelineAnchor": {"2026-09-01"},
	}
	got, _, err := NormalizeSavedURL("/resources/timeline?" + q.Encode())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(got)
	if !reflect.DeepEqual(u.Query(), q) {
		t.Fatalf("filters changed: %s", got)
	}
	for _, path := range []string{"/downloads", "/resources"} {
		got, _, err := NormalizeSavedURL(path + "?Error=timeout&error=network")
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(got)
		if u.Query().Has("Error") != (path == "/downloads") || u.Query().Has("error") != (path == "/downloads") {
			t.Fatalf("incorrect Error handling: %s", got)
		}
	}
}

func TestSavedSearchRejectsInvalidURLs(t *testing.T) {
	for _, raw := range []string{"", "resources", "https://example.com/resources", "//evil.test/resources", "/resources#fragment", "/resources.json", "/resources.body", "/v1/resource/delete?id=1", "/resource?id=1", "/resources/../notes", "/%72esources", "/resources?Name=%zz", "/resources?Name=a;b", "javascript:alert(1)", "/resources\n"} {
		if _, _, err := NormalizeSavedURL(raw); err == nil {
			t.Errorf("accepted invalid URL %q", raw)
		}
	}
}
