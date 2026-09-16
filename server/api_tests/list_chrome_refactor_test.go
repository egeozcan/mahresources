package api_tests

import (
	"strings"
	"testing"
)

func TestListChrome_MassEditAllIsInTitleBar(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{
		"/resources", "/resources/details", "/resources/simple", "/resources/timeline",
		"/notes", "/notes/timeline",
		"/groups", "/groups/text", "/groups/timeline",
	} {
		_, body := tc.getHTML(t, path)
		titleStart := strings.Index(body, `<section class="title`)
		if titleStart < 0 {
			t.Fatalf("%s: title bar not found", path)
		}
		titleEnd := strings.Index(body[titleStart:], "</section>")
		if titleEnd < 0 {
			t.Fatalf("%s: title bar is not closed", path)
		}
		title := body[titleStart : titleStart+titleEnd]
		if !strings.Contains(title, "Mass edit all") {
			t.Errorf("%s: Mass edit all is not in the title bar", path)
		}
		if strings.Count(body, "Mass edit all") != 1 {
			t.Errorf("%s: Mass edit all rendered %d times, want once", path, strings.Count(body, "Mass edit all"))
		}
	}
}

func TestListChrome_SavedSearchesLeadTheSidebar(t *testing.T) {
	tc := SetupTestEnv(t)

	for _, path := range []string{"/resources", "/notes", "/groups", "/tags", "/downloads", "/logs"} {
		_, body := tc.getHTML(t, path)
		asideStart := strings.Index(body, `<aside class="sidebar"`)
		if asideStart < 0 {
			t.Fatalf("%s: sidebar not found", path)
		}
		asideEnd := strings.Index(body[asideStart:], "</aside>")
		if asideEnd < 0 {
			t.Fatalf("%s: sidebar is not closed", path)
		}
		aside := body[asideStart : asideStart+asideEnd]
		saved := strings.Index(aside, `aria-label="Saved searches"`)
		filter := strings.Index(aside, `aria-label="Filter `)
		if saved < 0 || filter < 0 || saved > filter {
			t.Errorf("%s: saved searches must precede the filter form (saved=%d filter=%d)", path, saved, filter)
		}
	}
}
