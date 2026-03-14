package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// searchResult represents a single result from the global search API.
type searchResult struct {
	ID          uint              `json:"ID"`
	Type        string            `json:"Type"`
	Name        string            `json:"Name"`
	Description string            `json:"Description"`
	Score       int               `json:"Score"`
	URL         string            `json:"URL"`
	Extra       map[string]string `json:"Extra"`
}

// globalSearchResponse wraps the search results array.
type globalSearchResponse struct {
	Results []searchResult `json:"Results"`
}

// NewSearchCmd returns the top-level "search" command.
func NewSearchCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var typesStr string
	var limit int

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search across all entities",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"q":     args[0],
				"limit": limit,
			}

			if typesStr != "" {
				parts := strings.Split(typesStr, ",")
				var types []string
				for _, p := range parts {
					p = strings.TrimSpace(p)
					if p != "" {
						types = append(types, p)
					}
				}
				if len(types) > 0 {
					body["types"] = types
				}
			}

			var raw json.RawMessage
			if err := c.Post("/v1/search", nil, body, &raw); err != nil {
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
