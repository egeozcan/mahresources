package commands

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed queries_help/*.md
var queriesHelpFS embed.FS

// queryResponse is a lightweight struct matching the API's Query JSON shape.
type queryResponse struct {
	ID          uint      `json:"ID"`
	Name        string    `json:"Name"`
	Text        string    `json:"Text"`
	Template    string    `json:"Template"`
	Description string    `json:"Description"`
	CreatedAt   time.Time `json:"CreatedAt"`
	UpdatedAt   time.Time `json:"UpdatedAt"`
}

// NewQueryCmd returns the singular "query" command with get/create/delete/edit-name/edit-description/run/run-by-name/schema subcommands.
func NewQueryCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query.md")
	queryCmd := &cobra.Command{
		Use:         "query",
		Short:       "Get, create, run, or delete a saved query",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	queryCmd.AddCommand(newQueryGetCmd(c, opts))
	queryCmd.AddCommand(newQueryCreateCmd(c, opts))
	queryCmd.AddCommand(newQueryDeleteCmd(c, opts))
	queryCmd.AddCommand(newQueryEditNameCmd(c, opts))
	queryCmd.AddCommand(newQueryEditDescriptionCmd(c, opts))
	queryCmd.AddCommand(newQueryRunCmd(c, opts))
	queryCmd.AddCommand(newQueryRunByNameCmd(c, opts))
	queryCmd.AddCommand(newQuerySchemaCmd(c, opts))

	return queryCmd
}

func newQueryGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query_get.md")
	return &cobra.Command{
		Use:         "get <id>",
		Short:       "Get a query by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/query", q, &raw); err != nil {
				return err
			}

			var qr queryResponse
			if err := json.Unmarshal(raw, &qr); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "ID", Value: strconv.FormatUint(uint64(qr.ID), 10)},
				{Key: "Name", Value: qr.Name},
				{Key: "Text", Value: qr.Text},
				{Key: "Template", Value: qr.Template},
				{Key: "Description", Value: qr.Description},
				{Key: "Created", Value: qr.CreatedAt.Format(time.RFC3339)},
				{Key: "Updated", Value: qr.UpdatedAt.Format(time.RFC3339)},
			}, raw)
			return nil
		},
	}
}

func newQueryCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, text, template string

	help := helptext.Load(queriesHelpFS, "queries_help/query_create.md")
	cmd := &cobra.Command{
		Use:         "create",
		Short:       "Create a new query",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]string{
				"Name": name,
				"Text": text,
			}
			if template != "" {
				body["Template"] = template
			}

			var raw json.RawMessage
			if err := c.Post("/v1/query", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				var qr queryResponse
				if err := json.Unmarshal(raw, &qr); err == nil {
					output.PrintMessage(fmt.Sprintf("Created query %d: %s", qr.ID, qr.Name))
				} else {
					output.PrintMessage("Query created successfully.")
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Query name (required)")
	cmd.MarkFlagRequired("name")
	cmd.Flags().StringVar(&text, "text", "", "Query text/SQL (required)")
	cmd.MarkFlagRequired("text")
	cmd.Flags().StringVar(&template, "template", "", "Query template")

	return cmd
}

func newQueryDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query_delete.md")
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a query by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/query/delete", q, nil, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Query deleted successfully.")
			}
			return nil
		},
	}
}

func newQueryEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query_edit_name.md")
	return &cobra.Command{
		Use:         "edit-name <id> <value>",
		Short:       "Edit a query's name",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Name", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/query/editName", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Query name updated successfully.")
			}
			return nil
		},
	}
}

func newQueryEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query_edit_description.md")
	return &cobra.Command{
		Use:         "edit-description <id> <value>",
		Short:       "Edit a query's description",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			form := url.Values{}
			form.Set("Description", args[1])

			var raw json.RawMessage
			if err := c.PostForm("/v1/query/editDescription", q, form, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else {
				output.PrintMessage("Query description updated successfully.")
			}
			return nil
		},
	}
}

// sqlResultSet mirrors contracts.SQLResultSet, the body of POST /v1/query/run:
// the column list in the query's own order and one array of values per row.
type sqlResultSet struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// printQueryResult renders a saved query's result set. --json passes the body
// through untouched; otherwise the columns become the table header and each row
// its own line, which is what output.Print has always taken. Before the response
// carried its column list, `run` had nothing to build a header from and fell back
// to printing the raw JSON in both modes.
func printQueryResult(opts output.Options, raw json.RawMessage) {
	if opts.JSON {
		output.PrintSingle(opts, nil, raw)
		return
	}
	columns, rows, ok := queryResultTable(raw)
	if !ok {
		output.PrintSingle(opts, nil, raw)
		return
	}
	output.Print(opts, columns, rows, nil)
}

// queryResultTable turns a /v1/query/run body into the header and cells
// output.Print takes, reporting false for a body that is not a result set so the
// caller can fall back to printing the raw JSON.
//
// UseNumber, because the default decodes every JSON number into a float64 and
// float64 holds integers exactly only up to 2^53. Without it `mr query run`
// printed 9007199254740992 for a column the server sent as 9007199254740993 —
// the digits were correct on the wire, correct under --json (which passes the
// raw bytes through), and rounded on the way to the text table.
func queryResultTable(raw json.RawMessage) (columns []string, rows [][]string, ok bool) {
	var result sqlResultSet
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil {
		return nil, nil, false
	}

	rows = make([][]string, 0, len(result.Rows))
	for _, values := range result.Rows {
		cells := make([]string, 0, len(result.Columns))
		for i := range result.Columns {
			if i < len(values) {
				cells = append(cells, formatCell(values[i]))
			} else {
				cells = append(cells, "")
			}
		}
		rows = append(rows, cells)
	}

	return result.Columns, rows, true
}

// formatCell renders one result cell for the text table. Strings print as
// themselves — quoting every one would make the common case unreadable — and
// anything structured is re-encoded as compact JSON.
func formatCell(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return output.Truncate(v, 80)
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		// The literal the server wrote, digit for digit. This is the whole point
		// of decoding with UseNumber; see queryResultTable.
		return output.Truncate(v.String(), 80)
	case float64:
		// Unreachable through queryResultTable, kept for any caller that decodes
		// without UseNumber.
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return output.Truncate(string(encoded), 80)
	}
}

func newQueryRunCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query_run.md")
	return &cobra.Command{
		Use:         "run <id>",
		Short:       "Run a query by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Post("/v1/query/run", q, nil, &raw); err != nil {
				return err
			}

			printQueryResult(*opts, raw)
			return nil
		},
	}
}

func newQueryRunByNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name string

	help := helptext.Load(queriesHelpFS, "queries_help/query_run_by_name.md")
	cmd := &cobra.Command{
		Use:         "run-by-name",
		Short:       "Run a query by name",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("name", name)

			var raw json.RawMessage
			if err := c.Post("/v1/query/run", q, nil, &raw); err != nil {
				return err
			}

			printQueryResult(*opts, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Query name (required)")
	cmd.MarkFlagRequired("name")

	return cmd
}

func newQuerySchemaCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/query_schema.md")
	return &cobra.Command{
		Use:         "schema",
		Short:       "Show database table and column names for query building",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			var raw json.RawMessage
			if err := c.Get("/v1/query/schema", nil, &raw); err != nil {
				return err
			}

			output.PrintSingle(*opts, nil, raw)
			return nil
		},
	}
}

// NewQueriesCmd returns the plural "queries" command with list subcommand.
func NewQueriesCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	help := helptext.Load(queriesHelpFS, "queries_help/queries.md")
	queriesCmd := &cobra.Command{
		Use:         "queries",
		Short:       "List saved queries",
		Long:        help.Long,
		Annotations: help.Annotations,
	}

	queriesCmd.AddCommand(newQueriesListCmd(c, opts, page))
	queriesCmd.AddCommand(newQueriesTimelineCmd(c, opts))

	return queriesCmd
}

func newQueriesListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name string

	help := helptext.Load(queriesHelpFS, "queries_help/queries_list.md")
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List queries",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if name != "" {
				q.Set("name", name)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/queries", q, &raw); err != nil {
				return err
			}

			var queries []queryResponse
			if err := json.Unmarshal(raw, &queries); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, qr := range queries {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(qr.ID), 10),
					output.Truncate(qr.Name, 40),
					output.Truncate(qr.Description, 50),
					qr.CreatedAt.Format(time.RFC3339),
				})
			}

			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter by name")

	return cmd
}

func newQueriesTimelineCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		tFlags timelineFlags
		name   string
	)

	help := helptext.Load(queriesHelpFS, "queries_help/queries_timeline.md")
	cmd := &cobra.Command{
		Use:         "timeline",
		Short:       "Display a timeline of query activity",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if name != "" {
				q.Set("name", name)
			}

			return fetchAndPrintTimeline(c, *opts, "/v1/queries/timeline", buildTimelineQuery(&tFlags, q))
		},
	}

	addTimelineFlags(cmd, &tFlags)
	cmd.Flags().StringVar(&name, "name", "", "Filter by name")

	return cmd
}
