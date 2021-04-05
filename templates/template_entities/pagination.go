package template_entities

import (
	"fmt"
	"math"
	"net/url"
	"strconv"
)

type PaginationEntry struct {
	Display  string
	Selected bool
	Link     string
}

type PaginationResult struct {
	Entries  *[]PaginationEntry
	PrevLink *PaginationEntry
	NextLink *PaginationEntry
}

func GeneratePagination(baseUrl string, numResults int64, pageSize int, currentPage int) (*PaginationResult, error) {
	parsedBaseUrl, err := url.Parse(baseUrl)

	if err != nil {
		return nil, err
	}

	res := &PaginationResult{
		Entries: nil,
		PrevLink: &PaginationEntry{
			Display:  "",
			Selected: false,
			Link:     "",
		},
		NextLink: &PaginationEntry{
			Display:  "",
			Selected: false,
			Link:     "",
		},
	}
	numPages := int(math.Ceil(float64(numResults) / float64(pageSize)))
	paginationEntries := make([]PaginationEntry, 0, numPages)
	lastIsThreeDots := false

	if currentPage > 1 && numPages > 1 {
		q := parsedBaseUrl.Query()
		q.Set("page", strconv.FormatInt(int64(currentPage-1), 10))
		parsedBaseUrl.RawQuery = q.Encode()

		res.PrevLink = &PaginationEntry{
			Display:  "Previous",
			Selected: true,
			Link:     parsedBaseUrl.String(),
		}
	}

	if currentPage < numPages {
		q := parsedBaseUrl.Query()
		q.Set("page", strconv.FormatInt(int64(currentPage+1), 10))
		parsedBaseUrl.RawQuery = q.Encode()

		res.NextLink = &PaginationEntry{
			Display:  "Next",
			Selected: true,
			Link:     parsedBaseUrl.String(),
		}
	}

	for i := 1; i <= numPages; i++ {
		if i > 2 && math.Abs(float64(i-currentPage)) > 2 && numPages-i > 2 {
			if i > 1 && !lastIsThreeDots {
				paginationEntries = append(paginationEntries, PaginationEntry{
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

		paginationEntries = append(paginationEntries, PaginationEntry{
			Display:  fmt.Sprintf("%d", i),
			Selected: i == currentPage,
			Link:     parsedBaseUrl.String(),
		})
	}

	res.Entries = &paginationEntries

	return res, nil
}
