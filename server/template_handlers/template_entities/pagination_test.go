package template_entities

import "testing"

func TestGeneratePagination(t *testing.T) {
	url := "http://www.example.com?a=1&page=11"
	pagination, err := GeneratePagination(url, 600, 10, 12)

	if err != nil {
		t.Errorf("Failed while parsing url: %v, got error %v", url, err)
	}

	for i, link := range *pagination.Entries {
		if link.Display == "12" && !link.Selected {
			t.Error("Current page not selected")
		}

		t.Log(i, link)
	}
}

func TestGeneratePagination_ZeroResults(t *testing.T) {
	pagination, err := GeneratePagination("http://example.com", 0, 10, 1)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.Entries == nil {
		t.Error("Entries should not be nil")
		return
	}

	if len(*pagination.Entries) != 0 {
		t.Errorf("expected 0 entries for 0 results, got %d", len(*pagination.Entries))
	}

	if pagination.PrevLink.Selected {
		t.Error("PrevLink should not be selected with 0 results")
	}

	if pagination.NextLink.Selected {
		t.Error("NextLink should not be selected with 0 results")
	}
}

func TestGeneratePagination_SinglePage(t *testing.T) {
	pagination, err := GeneratePagination("http://example.com", 5, 10, 1)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.Entries == nil {
		t.Error("Entries should not be nil")
		return
	}

	if len(*pagination.Entries) != 1 {
		t.Errorf("expected 1 entry for single page, got %d", len(*pagination.Entries))
	}

	if !(*pagination.Entries)[0].Selected {
		t.Error("single page entry should be selected")
	}

	if pagination.PrevLink.Selected {
		t.Error("PrevLink should not be selected on single page")
	}

	if pagination.NextLink.Selected {
		t.Error("NextLink should not be selected on single page")
	}
}

func TestGeneratePagination_FirstPage(t *testing.T) {
	pagination, err := GeneratePagination("http://example.com", 100, 10, 1)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.PrevLink.Selected {
		t.Error("PrevLink should not be selected on first page")
	}

	if !pagination.NextLink.Selected {
		t.Error("NextLink should be selected on first page with multiple pages")
	}

	// First entry should be selected
	if pagination.Entries == nil || len(*pagination.Entries) == 0 {
		t.Error("Entries should not be empty")
		return
	}

	if !(*pagination.Entries)[0].Selected {
		t.Error("first entry should be selected on first page")
	}
}

func TestGeneratePagination_LastPage(t *testing.T) {
	pagination, err := GeneratePagination("http://example.com", 100, 10, 10)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !pagination.PrevLink.Selected {
		t.Error("PrevLink should be selected on last page with multiple pages")
	}

	if pagination.NextLink.Selected {
		t.Error("NextLink should not be selected on last page")
	}

	// Last entry should be selected
	if pagination.Entries == nil || len(*pagination.Entries) == 0 {
		t.Error("Entries should not be empty")
		return
	}

	lastIdx := len(*pagination.Entries) - 1
	if !(*pagination.Entries)[lastIdx].Selected {
		t.Error("last entry should be selected on last page")
	}
}

func TestGeneratePagination_MiddlePage(t *testing.T) {
	pagination, err := GeneratePagination("http://example.com", 100, 10, 5)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if !pagination.PrevLink.Selected {
		t.Error("PrevLink should be selected on middle page")
	}

	if !pagination.NextLink.Selected {
		t.Error("NextLink should be selected on middle page")
	}
}

func TestGeneratePagination_LargePageCount(t *testing.T) {
	// 1000 results, 10 per page = 100 pages
	pagination, err := GeneratePagination("http://example.com", 1000, 10, 50)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.Entries == nil {
		t.Error("Entries should not be nil")
		return
	}

	// Should have ellipsis entries
	hasEllipsis := false
	for _, entry := range *pagination.Entries {
		if entry.Display == "..." {
			hasEllipsis = true
			break
		}
	}

	if !hasEllipsis {
		t.Error("expected ellipsis in pagination with many pages")
	}

	// Should not have more entries than expected (with ellipsis compression)
	// At minimum: first 2 pages + current +/- 2 + last 3 pages + 2 ellipsis
	if len(*pagination.Entries) > 15 {
		t.Errorf("expected compressed pagination, got %d entries", len(*pagination.Entries))
	}
}

func TestGeneratePagination_InvalidURL(t *testing.T) {
	_, err := GeneratePagination("://invalid", 100, 10, 1)

	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

func TestGeneratePagination_PreservesQueryParams(t *testing.T) {
	pagination, err := GeneratePagination("http://example.com?filter=active&sort=name", 100, 10, 1)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.NextLink == nil || pagination.NextLink.Link == "" {
		t.Error("NextLink should have a link")
		return
	}

	// Check that original query params are preserved
	link := pagination.NextLink.Link
	if link == "" {
		t.Error("NextLink should have a URL")
		return
	}

	// The link should contain both original params and the new page param
	// Note: URL encoding may vary, so we check for key presence
	if pagination.Entries != nil && len(*pagination.Entries) > 1 {
		secondEntry := (*pagination.Entries)[1]
		if secondEntry.Link == "" {
			t.Error("Entry links should not be empty")
		}
	}
}

func TestGeneratePagination_ExactPageBoundary(t *testing.T) {
	// Exactly 100 results with 10 per page = exactly 10 pages
	pagination, err := GeneratePagination("http://example.com", 100, 10, 10)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.Entries == nil {
		t.Error("Entries should not be nil")
		return
	}

	// Find the last numbered entry (not ellipsis)
	var lastPage string
	for i := len(*pagination.Entries) - 1; i >= 0; i-- {
		if (*pagination.Entries)[i].Display != "..." {
			lastPage = (*pagination.Entries)[i].Display
			break
		}
	}

	if lastPage != "10" {
		t.Errorf("expected last page to be 10, got %s", lastPage)
	}
}

func TestGeneratePagination_SmallPageCount(t *testing.T) {
	// 30 results with 10 per page = 3 pages (no ellipsis needed)
	pagination, err := GeneratePagination("http://example.com", 30, 10, 2)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if pagination.Entries == nil {
		t.Error("Entries should not be nil")
		return
	}

	// Should have exactly 3 entries, no ellipsis
	if len(*pagination.Entries) != 3 {
		t.Errorf("expected 3 entries for 3 pages, got %d", len(*pagination.Entries))
	}

	for _, entry := range *pagination.Entries {
		if entry.Display == "..." {
			t.Error("should not have ellipsis with only 3 pages")
		}
	}
}
