package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// mrqlEntity represents a single entity with common fields for display.
type mrqlEntity struct {
	ID        uint      `json:"ID"`
	Name      string    `json:"Name"`
	CreatedAt time.Time `json:"CreatedAt"`
}

// mrqlResponse matches the MRQLResult struct returned by the API.
type mrqlResponse struct {
	EntityType string       `json:"entityType"`
	Resources  []mrqlEntity `json:"resources,omitempty"`
	Notes      []mrqlEntity `json:"notes,omitempty"`
	Groups     []mrqlEntity `json:"groups,omitempty"`
}

// mrqlSavedQuery represents a saved MRQL query.
type mrqlSavedQuery struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	Query       string    `json:"query"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// NewMRQLCmd returns the "mrql" command with subcommands for managing and executing MRQL queries.
func NewMRQLCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var (
		fileFlag string
		limit    int
	)

	mrqlCmd := &cobra.Command{
		Use:   "mrql [query]",
		Short: "Execute and manage MRQL queries",
		Long: `Execute MRQL (Mahresources Query Language) queries and manage saved queries.

Examples:
  mr mrql 'type = resource AND tags = "photo"'
  mr mrql -f query.mrql
  echo 'tags = "photo"' | mr mrql -
  mr mrql --limit 10 --page 2 'type = note'`,
		RunE: func(cmd *cobra.Command, args []string) error {
			var queryText string

			// Determine query source: stdin, file, or positional arg
			if len(args) == 1 && args[0] == "-" {
				// Read from stdin
				data, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}
				queryText = string(data)
			} else if fileFlag != "" {
				// Read from file
				data, err := os.ReadFile(fileFlag)
				if err != nil {
					return fmt.Errorf("reading file %q: %w", fileFlag, err)
				}
				queryText = string(data)
			} else if len(args) == 1 {
				queryText = args[0]
			} else {
				return fmt.Errorf("provide a query as an argument, use -f <file>, or pipe to stdin with '-'")
			}

			body := map[string]interface{}{
				"query": queryText,
			}
			if cmd.Flags().Changed("limit") {
				body["limit"] = limit
			}
			if cmd.Flags().Changed("page") {
				body["page"] = *page
			}

			var raw json.RawMessage
			if err := c.Post("/v1/mrql", nil, body, &raw); err != nil {
				return err
			}

			var resp mrqlResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				// Response may not match the expected shape; fall back to raw output
				output.PrintSingle(*opts, nil, raw)
				return nil
			}

			columns := []string{"ID", "TYPE", "NAME", "CREATED"}
			rows := mrqlResponseToRows(resp)

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	mrqlCmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Read query from file")
	mrqlCmd.Flags().IntVar(&limit, "limit", 0, "Override result limit (0 = use query's LIMIT or server default)")

	mrqlCmd.AddCommand(newMRQLSaveCmd(c, opts))
	mrqlCmd.AddCommand(newMRQLListCmd(c, opts, page))
	mrqlCmd.AddCommand(newMRQLRunCmd(c, opts, page))
	mrqlCmd.AddCommand(newMRQLDeleteCmd(c, opts))

	return mrqlCmd
}

func newMRQLSaveCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var description string

	cmd := &cobra.Command{
		Use:   "save <name> <query>",
		Short: "Save a MRQL query",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{
				"name":  args[0],
				"query": args[1],
			}
			if description != "" {
				body["description"] = description
			}

			var raw json.RawMessage
			if err := c.Post("/v1/mrql/saved", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var sq mrqlSavedQuery
				if err := json.Unmarshal(raw, &sq); err == nil && sq.ID != 0 {
					output.PrintMessage(fmt.Sprintf("Saved MRQL query %d: %s", sq.ID, sq.Name))
				} else {
					output.PrintMessage("MRQL query saved successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&description, "description", "", "Description for the saved query")

	return cmd
}

func newMRQLListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List saved MRQL queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))

			var raw json.RawMessage
			if err := c.Get("/v1/mrql/saved", q, &raw); err != nil {
				return err
			}

			var queries []mrqlSavedQuery
			if err := json.Unmarshal(raw, &queries); err != nil {
				// Fall back to raw output if shape doesn't match
				output.PrintSingle(*opts, nil, raw)
				return nil
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, sq := range queries {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(sq.ID), 10),
					output.Truncate(sq.Name, 40),
					output.Truncate(sq.Description, 50),
					sq.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	return cmd
}

func newMRQLRunCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var limit int

	cmd := &cobra.Command{
		Use:   "run <name-or-id>",
		Short: "Run a saved MRQL query by name or ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			// Try to detect if it's a numeric ID or a name
			if _, err := strconv.ParseUint(args[0], 10, 64); err == nil {
				q.Set("id", args[0])
			} else {
				q.Set("name", args[0])
			}
			q.Set("limit", strconv.Itoa(limit))
			q.Set("page", strconv.Itoa(*page))

			var raw json.RawMessage
			if err := c.Post("/v1/mrql/saved/run", q, nil, &raw); err != nil {
				return err
			}

			var resp mrqlResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				output.PrintSingle(*opts, nil, raw)
				return nil
			}

			columns := []string{"ID", "TYPE", "NAME", "CREATED"}
			rows := mrqlResponseToRows(resp)

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 50, "Maximum number of results")

	return cmd
}

// mrqlResponseToRows converts the API response into unified table rows.
func mrqlResponseToRows(resp mrqlResponse) [][]string {
	var rows [][]string
	for _, r := range resp.Resources {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(r.ID), 10),
			"resource",
			output.Truncate(r.Name, 40),
			r.CreatedAt.Format(time.RFC3339),
		})
	}
	for _, n := range resp.Notes {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(n.ID), 10),
			"note",
			output.Truncate(n.Name, 40),
			n.CreatedAt.Format(time.RFC3339),
		})
	}
	for _, g := range resp.Groups {
		rows = append(rows, []string{
			strconv.FormatUint(uint64(g.ID), 10),
			"group",
			output.Truncate(g.Name, 40),
			g.CreatedAt.Format(time.RFC3339),
		})
	}
	return rows
}

func newMRQLDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a saved MRQL query by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/mrql/saved/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("MRQL query deleted successfully.")
			}
			return nil
		},
	}
}
