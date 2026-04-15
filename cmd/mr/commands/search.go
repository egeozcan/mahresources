package commands

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed search_help/*.md
var searchHelpFS embed.FS

// searchResult represents a single result from the global search API.
type searchResult struct {
	ID          uint              `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Score       int               `json:"score"`
	URL         string            `json:"url"`
	Extra       map[string]string `json:"extra"`
}

// globalSearchResponse wraps the search results array.
type globalSearchResponse struct {
	Query   string         `json:"query"`
	Total   int            `json:"total"`
	Results []searchResult `json:"results"`
}

// NewSearchCmd returns the top-level "search" command.
func NewSearchCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var typesStr string
	var limit int

	help := helptext.Load(searchHelpFS, "search_help/search.md")
	cmd := &cobra.Command{
		Use:         "search <query>",
		Short:       "Search across all entities",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("q", args[0])
			q.Set("limit", strconv.Itoa(limit))

			if typesStr != "" {
				parts := strings.Split(typesStr, ",")
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						q.Add("types", p)
					}
				}
			}

			var raw json.RawMessage
			if err := c.Get("/v1/search", q, &raw); err != nil {
				return err
			}

			var resp globalSearchResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "TYPE", "NAME", "SCORE", "DESCRIPTION"}
			var rows [][]string
			for _, r := range resp.Results {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(r.ID), 10),
					r.Type,
					output.Truncate(r.Name, 40),
					strconv.Itoa(r.Score),
					output.Truncate(r.Description, 50),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&typesStr, "types", "", "Comma-separated entity types to search (e.g. resources,notes)")
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of results")

	return cmd
}
