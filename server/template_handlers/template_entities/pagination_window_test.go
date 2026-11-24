package template_entities

import (
	"fmt"
	"math"
	"net/url"
	"reflect"
	"strconv"
	"testing"
	"time"
)

// legacyPaginationEntries is the loop GeneratePagination ran before it built only
// the displayed window: one pass over every page, preallocated to the page count.
// It is the reference the windowed build must equal.
func legacyPaginationEntries(baseUrl string, numPages, currentPage int) []paginationEntry {
	parsed, _ := url.Parse(baseUrl)
	entries := make([]paginationEntry, 0, numPages)
	lastIsThreeDots := false
	for i := 1; i <= numPages; i++ {
		if i > 2 && math.Abs(float64(i-currentPage)) > 2 && numPages-i > 2 {
			if i > 1 && !lastIsThreeDots {
				entries = append(entries, paginationEntry{Display: "..."})
				lastIsThreeDots = true
			}
			continue
		}
		lastIsThreeDots = false
		q := parsed.Query()
		q.Set("page", strconv.FormatInt(int64(i), 10))
		parsed.RawQuery = q.Encode()
		entries = append(entries, paginationEntry{Display: fmt.Sprintf("%d", i), Selected: i == currentPage, Link: parsed.String()})
	}
	return entries
}

func TestGeneratePaginationWindowMatchesFullLoop(t *testing.T) {
	const base = "http://example.com/list?a=1&page=3"
	for numPages := 0; numPages <= 40; numPages++ {
		for current := -2; current <= numPages+3; current++ {
			res, err := GeneratePagination(base, int64(numPages)*10, 10, current)
			if err != nil {
				t.Fatal(err)
			}
			want := legacyPaginationEntries(base, numPages, current)
			if res.Entries == nil {
				t.Fatalf("pages %d current %d: Entries is nil", numPages, current)
			}
			if !reflect.DeepEqual(*res.Entries, want) && !(len(*res.Entries) == 0 && len(want) == 0) {
				t.Fatalf("pages %d current %d:\n got %v\nwant %v", numPages, current, *res.Entries, want)
			}
		}
	}
}

func TestGeneratePaginationHugePageCountBuildsOnlyTheWindow(t *testing.T) {
	start := time.Now()
	res, err := GeneratePagination("http://example.com/list", 4_000_000_000_000, 1, 2_000_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a huge page count took %v", elapsed)
	}
	if n := len(*res.Entries); n > 14 {
		t.Fatalf("%d entries for a window of at most 14", n)
	}
	if cap(*res.Entries) > 14 {
		t.Fatalf("capacity %d: the entries were preallocated per page", cap(*res.Entries))
	}
	selected := 0
	for _, e := range *res.Entries {
		if e.Selected {
			selected++
			if e.Display != "2000000000000" {
				t.Fatalf("selected %q", e.Display)
			}
		}
	}
	if selected != 1 {
		t.Fatalf("%d selected entries", selected)
	}
}
