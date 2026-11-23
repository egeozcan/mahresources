package template_context_providers

import (
	"net/url"
	"testing"

	"github.com/flosch/pongo2/v4"
)

func navItems(t *testing.T, nav pongo2.Context, key string) []pongo2.Context {
	t.Helper()
	items, ok := nav[key].([]pongo2.Context)
	if !ok {
		t.Fatalf("%s is not a list of items: %#v", key, nav[key])
	}
	return items
}

func TestReviewNavChipsToggleOneParamAndDropThePage(t *testing.T) {
	values := url.Values{"id": {"7"}, "status": {"open"}, "tier": {"identical"}, "sort": {"size"}, "page": {"3"}}
	nav := reviewNav(values, 7, "open", map[string]int{"open": 4})

	tiers := navItems(t, nav, "tiers")
	if !tiers[0]["active"].(bool) || tiers[1]["active"].(bool) {
		t.Fatalf("identical should be the only active tier chip: %#v", tiers)
	}
	// Clicking the active chip removes it; clicking the other adds it. Neither
	// carries `page`, and both keep the sort and the status.
	if got, want := tiers[0]["href"], "/reduction?id=7&sort=size&status=open"; got != want {
		t.Errorf("active tier chip href = %v, want %v", got, want)
	}
	if got, want := tiers[1]["href"], "/reduction?id=7&sort=size&status=open&tier=identical&tier=near"; got != want {
		t.Errorf("inactive tier chip href = %v, want %v", got, want)
	}
}

func TestReviewNavTabsKeepSortAndDropFilters(t *testing.T) {
	values := url.Values{"id": {"7"}, "tier": {"near"}, "approval": {"approved"}, "sort": {"computed"}, "pageSize": {"50"}}
	nav := reviewNav(values, 7, "open", map[string]int{"open": 4, "skipped": 2})

	tabs := navItems(t, nav, "tabs")
	if len(tabs) != 4 {
		t.Fatalf("want four tabs, got %d", len(tabs))
	}
	if tabs[1]["href"] != "/reduction?id=7&pageSize=50&sort=computed&status=skipped" {
		t.Errorf("skipped tab href = %v", tabs[1]["href"])
	}
	if tabs[1]["count"] != 2 || tabs[0]["current"] != true || tabs[1]["current"] != false {
		t.Errorf("tab counts or current wrong: %#v", tabs)
	}
}

func TestReviewNavHiddenFieldsCarryFiltersButNotSortOrPageSize(t *testing.T) {
	values := url.Values{"id": {"7"}, "Status": {"skipped"}, "attention": {"true"}, "sort": {"size"}, "pageSize": {"100"}}
	nav := reviewNav(values, 7, "skipped", map[string]int{})

	got := map[string]string{}
	for _, h := range navItems(t, nav, "hidden") {
		got[h["name"].(string)] = h["value"].(string)
	}
	if got["id"] != "7" || got["status"] != "skipped" || got["attention"] != "true" {
		t.Errorf("hidden fields lost a filter: %#v", got)
	}
	if _, ok := got["sort"]; ok {
		t.Errorf("the sort select sets sort itself: %#v", got)
	}
	if _, ok := got["pageSize"]; ok {
		t.Errorf("the page size select sets pageSize itself: %#v", got)
	}
}
