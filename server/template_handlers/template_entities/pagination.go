package template_entities

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
)

type paginationEntry struct {
	Display  string
	Selected bool
	Link     string
}

type paginationResult struct {
	Entries  *[]paginationEntry
	PrevLink *paginationEntry
	NextLink *paginationEntry
}

func GeneratePagination(baseUrl string, numResults int64, pageSize int, currentPage int) (*paginationResult, error) {
	parsedBaseUrl, err := url.Parse(baseUrl)

	if err != nil {
		return nil, err
	}

	res := &paginationResult{
		Entries: nil,
		PrevLink: &paginationEntry{
			Display:  "",
			Selected: false,
			Link:     "",
		},
		NextLink: &paginationEntry{
			Display:  "",
			Selected: false,
			Link:     "",
		},
	}
	numPages := int(math.Ceil(float64(numResults) / float64(pageSize)))
	paginationEntries := make([]paginationEntry, 0, 14)

	if currentPage > 1 && numPages > 1 {
		q := parsedBaseUrl.Query()
		q.Set("page", strconv.FormatInt(int64(currentPage-1), 10))
		parsedBaseUrl.RawQuery = q.Encode()

		res.PrevLink = &paginationEntry{
			Display:  "Previous",
			Selected: true,
			Link:     parsedBaseUrl.String(),
		}
	}

	if currentPage < numPages {
		q := parsedBaseUrl.Query()
		q.Set("page", strconv.FormatInt(int64(currentPage+1), 10))
		parsedBaseUrl.RawQuery = q.Encode()

		res.NextLink = &paginationEntry{
			Display:  "Next",
			Selected: true,
			Link:     parsedBaseUrl.String(),
		}
	}

	// Only the displayed window is built: the first two pages, the two either side
	// of the current one and the last two, with "..." for each hidden run between
	// them. A page count is not a loop bound, so nothing here grows with it.
	shown := 0 // the last page already emitted
	windows := [][2]int{
		{1, min(2, numPages)},
		{max(1, currentPage-2), min(numPages, currentPage+2)},
		{max(1, numPages-2), numPages},
	}
	if windows[2][0] < windows[1][0] {
		windows[1], windows[2] = windows[2], windows[1] // a current page past the last pages
	}
	for _, window := range windows {
		lo, hi := max(window[0], shown+1), window[1]
		if lo > hi {
			continue
		}
		if lo > shown+1 {
			paginationEntries = append(paginationEntries, paginationEntry{Display: "..."})
		}
		for i := lo; i <= hi; i++ {
			q := parsedBaseUrl.Query()
			q.Set("page", strconv.FormatInt(int64(i), 10))
			parsedBaseUrl.RawQuery = q.Encode()

			paginationEntries = append(paginationEntries, paginationEntry{
				Display:  fmt.Sprintf("%d", i),
				Selected: i == currentPage,
				Link:     parsedBaseUrl.String(),
			})
		}
		shown = hi
	}

	res.Entries = &paginationEntries

	return res, nil
}

// KeysetPagination is the Previous/Next navigation of a listing paged by keyset
// rather than offset, drawn by the same partial as the numbered one. It has no
// page numbers because a keyset listing has no stable page N: the Job list pages
// this way so its rows do not drift between pages while Jobs keep arriving. An
// empty link leaves that side inert; with neither, there is no navigation.
func KeysetPagination(prevLink, nextLink string) *paginationResult {
	if prevLink == "" && nextLink == "" {
		return nil
	}
	return &paginationResult{
		PrevLink: &paginationEntry{Display: "Previous", Selected: prevLink != "", Link: prevLink},
		NextLink: &paginationEntry{Display: "Next", Selected: nextLink != "", Link: nextLink},
	}
}
