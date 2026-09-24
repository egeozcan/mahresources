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
	paginationEntries := make([]paginationEntry, 0, numPages)
	lastIsThreeDots := false

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

	for i := 1; i <= numPages; i++ {
		if i > 2 && math.Abs(float64(i-currentPage)) > 2 && numPages-i > 2 {
			if i > 1 && !lastIsThreeDots {
				paginationEntries = append(paginationEntries, paginationEntry{
					Display:  "...",
					Selected: false,
					Link:     "",
				})

				lastIsThreeDots = true
			}

			continue
		}
		lastIsThreeDots = false
		q := parsedBaseUrl.Query()
		q.Set("page", strconv.FormatInt(int64(i), 10))
		parsedBaseUrl.RawQuery = q.Encode()

		paginationEntries = append(paginationEntries, paginationEntry{
			Display:  fmt.Sprintf("%d", i),
			Selected: i == currentPage,
			Link:     parsedBaseUrl.String(),
		})
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
