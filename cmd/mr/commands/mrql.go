package commands

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

//go:embed mrql_help/*.md
var mrqlHelpFS embed.FS

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

// mrqlGroupedResponse matches the MRQLGroupedResult struct.
type mrqlGroupedResponse struct {
	EntityType  string           `json:"entityType"`
	Mode        string           `json:"mode"`
	Rows        []map[string]any `json:"rows,omitempty"`
	Groups      []mrqlBucket     `json:"groups,omitempty"`
	Warnings    []string         `json:"warnings,omitempty"`
	NextOffset  *int             `json:"nextOffset,omitempty"`
	TotalGroups int              `json:"totalGroups,omitempty"`
}

type mrqlBucket struct {
	Key   map[string]any  `json:"key"`
	Items json.RawMessage `json:"items"`
}

// NewMRQLCmd returns the "mrql" command with subcommands for managing and executing MRQL queries.
func NewMRQLCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var (
		fileFlag string
		limit    int
		buckets  int
		offset   int
		render   bool
		params   []string
	)

	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql.md")
	mrqlCmd := &cobra.Command{
		Use:         "mrql [query]",
		Short:       "Execute and manage MRQL queries",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
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
			if cmd.Flags().Changed("buckets") {
				body["buckets"] = buckets
			}
			if cmd.Flags().Changed("offset") {
				body["offset"] = offset
			}
			if cmd.Flags().Changed("page") {
				body["page"] = *page
			}

			q := url.Values{}
			if err := addParamFlags(q, params); err != nil {
				return err
			}
			if render {
				q.Set("render", "1")
			}

			var raw json.RawMessage
			if err := c.Post("/v1/mrql", q, body, &raw); err != nil {
				return err
			}

			printMRQLResponse(*opts, raw)
			return nil
		},
	}

	mrqlCmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Read query from file")
	mrqlCmd.Flags().IntVar(&limit, "limit", 0, "Items per bucket for GROUP BY, or total items for regular queries")
	mrqlCmd.Flags().IntVar(&buckets, "buckets", 0, "Groups per page for bucketed GROUP BY queries")
	mrqlCmd.Flags().IntVar(&offset, "offset", 0, "Bucket offset for cursor-based GROUP BY pagination")
	mrqlCmd.Flags().BoolVar(&render, "render", false, "Request server-side template rendering via CustomMRQLResult")
	mrqlCmd.Flags().StringArrayVar(&params, "param", nil, "Bind a query parameter placeholder, repeatable: --param name=value")

	mrqlCmd.AddCommand(newMRQLSaveCmd(c, opts))
	mrqlCmd.AddCommand(newMRQLListCmd(c, opts, page))
	mrqlCmd.AddCommand(newMRQLRunCmd(c, opts, page))
	mrqlCmd.AddCommand(newMRQLExplainCmd(c, opts))
	mrqlCmd.AddCommand(newMRQLExportCmd(c, opts, page))
	mrqlCmd.AddCommand(newMRQLDeleteCmd(c, opts))

	return mrqlCmd
}

// addParamFlags parses repeatable --param name=value flags into `param.<name>`
// query parameters (the server accepts these for execute/run/explain/export).
func addParamFlags(q url.Values, params []string) error {
	for _, p := range params {
		name, value, ok := strings.Cut(p, "=")
		if !ok || name == "" {
			return fmt.Errorf("invalid --param %q: expected name=value", p)
		}
		q.Set("param."+name, value)
	}
	return nil
}

func newMRQLSaveCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var description string

	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql_save.md")
	cmd := &cobra.Command{
		Use:         "save <name> <query>",
		Short:       "Save a MRQL query",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(2),
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
	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql_list.md")
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List saved MRQL queries",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
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
	var (
		limit   int
		buckets int
		offset  int
		render  bool
		params  []string
	)

	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql_run.md")
	cmd := &cobra.Command{
		Use:         "run <name-or-id>",
		Short:       "Run a saved MRQL query by name or ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			// Always send both id and name — the server tries id first,
			// then falls back to name. This avoids the ambiguity where
			// numeric-only names (e.g., "42") can't be looked up by name.
			q.Set("id", args[0])
			q.Set("name", args[0])
			if cmd.Flags().Changed("limit") {
				q.Set("limit", strconv.Itoa(limit))
			}
			if cmd.Flags().Changed("buckets") {
				q.Set("buckets", strconv.Itoa(buckets))
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", strconv.Itoa(offset))
			}
			if cmd.Flags().Changed("page") {
				q.Set("page", strconv.Itoa(*page))
			}
			if err := addParamFlags(q, params); err != nil {
				return err
			}
			if render {
				q.Set("render", "1")
			}

			var raw json.RawMessage
			if err := c.Post("/v1/mrql/saved/run", q, nil, &raw); err != nil {
				return err
			}

			printMRQLResponse(*opts, raw)
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Items per bucket for GROUP BY, or total items for regular queries")
	cmd.Flags().IntVar(&buckets, "buckets", 0, "Groups per page for bucketed GROUP BY queries")
	cmd.Flags().IntVar(&offset, "offset", 0, "Bucket offset for cursor-based GROUP BY pagination")
	cmd.Flags().BoolVar(&render, "render", false, "Request server-side template rendering via CustomMRQLResult")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Bind a query parameter placeholder, repeatable: --param name=value")

	return cmd
}

// mrqlExplainStatement mirrors one statement in the /v1/mrql/explain response.
type mrqlExplainStatement struct {
	Label        string `json:"label"`
	SQL          string `json:"sql"`
	Vars         []any  `json:"vars"`
	Interpolated string `json:"interpolated"`
}

type mrqlExplainResponse struct {
	EntityType          string                 `json:"entityType"`
	Statements          []mrqlExplainStatement `json:"statements"`
	Warnings            []string               `json:"warnings,omitempty"`
	DefaultLimitApplied bool                   `json:"default_limit_applied"`
	AppliedLimit        int                    `json:"applied_limit,omitempty"`
}

// newMRQLExplainCmd returns the "mrql explain" subcommand.
func newMRQLExplainCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var (
		saved    string
		fileFlag string
		params   []string
		asJSON   bool
	)

	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql_explain.md")
	cmd := &cobra.Command{
		Use:         "explain [query]",
		Short:       "Show the SQL an MRQL query would run, without executing it",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]interface{}{}
			q := url.Values{}
			if err := addParamFlags(q, params); err != nil {
				return err
			}

			if saved != "" {
				// The server expects a numeric id; send it only when --saved is
				// numeric, and always send the name so numeric-only names still
				// resolve via the server's name fallback.
				if id, err := strconv.ParseUint(saved, 10, 64); err == nil {
					body["id"] = id
				}
				body["name"] = saved
			} else {
				queryText, err := readQueryText(args, fileFlag)
				if err != nil {
					return err
				}
				body["query"] = queryText
			}

			var raw json.RawMessage
			if err := c.Post("/v1/mrql/explain", q, body, &raw); err != nil {
				return err
			}

			if asJSON || opts.JSON {
				fmt.Println(string(raw))
				return nil
			}

			var resp mrqlExplainResponse
			if err := json.Unmarshal(raw, &resp); err != nil {
				fmt.Println(string(raw))
				return nil
			}
			printExplain(resp)
			return nil
		},
	}

	cmd.Flags().StringVar(&saved, "saved", "", "Explain a saved query by name or ID instead of an inline query")
	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Read query from file")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Bind a query parameter placeholder, repeatable: --param name=value")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the raw explain response as JSON")

	return cmd
}

// printExplain renders the explain response as label headers plus interpolated SQL.
func printExplain(resp mrqlExplainResponse) {
	for _, w := range resp.Warnings {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}
	if resp.DefaultLimitApplied {
		fmt.Fprintf(os.Stderr, "Default LIMIT %d applied.\n", resp.AppliedLimit)
	}
	for i, st := range resp.Statements {
		if i > 0 {
			fmt.Println()
		}
		fmt.Printf("-- %s --\n", st.Label)
		fmt.Println(st.Interpolated)
	}
}

// newMRQLExportCmd returns the "mrql export" subcommand.
func newMRQLExportCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var (
		saved    string
		fileFlag string
		format   string
		outFile  string
		params   []string
		limit    int
		buckets  int
		offset   int
	)

	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql_export.md")
	cmd := &cobra.Command{
		Use:         "export [query]",
		Short:       "Export MRQL query results as CSV or JSON",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if err := addParamFlags(q, params); err != nil {
				return err
			}
			if format != "" {
				q.Set("format", format)
			}
			if saved != "" {
				// id must be numeric for the server's query decoder; always send
				// the name so numeric-only names still resolve via the fallback.
				if _, err := strconv.ParseUint(saved, 10, 64); err == nil {
					q.Set("id", saved)
				}
				q.Set("name", saved)
			} else {
				queryText, err := readQueryText(args, fileFlag)
				if err != nil {
					return err
				}
				q.Set("query", queryText)
			}
			if cmd.Flags().Changed("limit") {
				q.Set("limit", strconv.Itoa(limit))
			}
			if cmd.Flags().Changed("buckets") {
				q.Set("buckets", strconv.Itoa(buckets))
			}
			if cmd.Flags().Changed("offset") {
				q.Set("offset", strconv.Itoa(offset))
			}
			if cmd.Flags().Changed("page") {
				q.Set("page", strconv.Itoa(*page))
			}

			resp, err := c.GetRaw("/v1/mrql/export", q)
			if err != nil {
				return err
			}
			defer resp.Body.Close()

			if resp.StatusCode != 200 {
				data, _ := io.ReadAll(resp.Body)
				return fmt.Errorf("export failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
			}

			out := os.Stdout
			if outFile != "" {
				f, err := os.Create(outFile)
				if err != nil {
					return fmt.Errorf("creating %q: %w", outFile, err)
				}
				defer f.Close()
				out = f
			}
			if _, err := io.Copy(out, resp.Body); err != nil {
				return fmt.Errorf("writing export: %w", err)
			}
			if outFile != "" && !opts.Quiet {
				fmt.Fprintf(os.Stderr, "Wrote %s\n", outFile)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&saved, "saved", "", "Export a saved query by name or ID instead of an inline query")
	cmd.Flags().StringVarP(&fileFlag, "file", "f", "", "Read query from file")
	cmd.Flags().StringVar(&format, "format", "csv", "Export format: csv or json")
	cmd.Flags().StringVarP(&outFile, "output", "o", "", "Write to a file instead of stdout")
	cmd.Flags().StringArrayVar(&params, "param", nil, "Bind a query parameter placeholder, repeatable: --param name=value")
	cmd.Flags().IntVar(&limit, "limit", 0, "Items per bucket for GROUP BY, or total items for regular queries")
	cmd.Flags().IntVar(&buckets, "buckets", 0, "Groups per page for bucketed GROUP BY queries")
	cmd.Flags().IntVar(&offset, "offset", 0, "Bucket offset for cursor-based GROUP BY pagination")

	return cmd
}

// readQueryText resolves query text from a positional arg, a file, or stdin ('-').
func readQueryText(args []string, fileFlag string) (string, error) {
	if len(args) == 1 && args[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("reading stdin: %w", err)
		}
		return string(data), nil
	}
	if fileFlag != "" {
		data, err := os.ReadFile(fileFlag)
		if err != nil {
			return "", fmt.Errorf("reading file %q: %w", fileFlag, err)
		}
		return string(data), nil
	}
	if len(args) == 1 {
		return args[0], nil
	}
	return "", fmt.Errorf("provide a query as an argument, use -f <file>, pipe to stdin with '-', or use --saved <name-or-id>")
}

// printMRQLResponse handles rendering of both grouped and standard MRQL responses,
// including warnings from the server (e.g., truncated bucketed results).
func printMRQLResponse(opts output.Options, raw json.RawMessage) {
	// Try grouped response first (has "mode" field)
	var grouped mrqlGroupedResponse
	if err := json.Unmarshal(raw, &grouped); err == nil && grouped.Mode != "" {
		for _, w := range grouped.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
		}
		if grouped.Mode == "aggregated" {
			columns, rows := aggregatedToTable(grouped.Rows)
			if len(rows) == 0 && !opts.JSON && !opts.Quiet {
				output.PrintMessage("No results found.")
			} else {
				output.Print(opts, columns, rows, raw)
			}
		} else {
			if len(grouped.Groups) == 0 && !opts.JSON && !opts.Quiet {
				output.PrintMessage("No results found.")
			} else {
				printBucketedOutput(opts, grouped, raw)
				if grouped.NextOffset != nil && !opts.JSON && !opts.Quiet {
					fmt.Fprintf(os.Stderr, "Showing %d of %d groups. Use --offset %d for next page.\n",
						len(grouped.Groups), grouped.TotalGroups, *grouped.NextOffset)
				}
			}
		}
		return
	}

	// Fall back to standard response
	var resp mrqlResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		output.PrintSingle(opts, nil, raw)
		return
	}

	columns := []string{"ID", "TYPE", "NAME", "CREATED"}
	rows := mrqlResponseToRows(resp)
	output.Print(opts, columns, rows, raw)
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

// aggregatedToTable converts aggregated rows to table columns/rows.
func aggregatedToTable(rows []map[string]any) ([]string, [][]string) {
	if len(rows) == 0 {
		return nil, nil
	}

	// Collect column names from the first row and sort for stable order
	var columns []string
	for k := range rows[0] {
		columns = append(columns, k)
	}
	sort.Strings(columns)

	var tableRows [][]string
	for _, row := range rows {
		var cells []string
		for _, col := range columns {
			cells = append(cells, fmt.Sprintf("%v", row[col]))
		}
		tableRows = append(tableRows, cells)
	}

	// Uppercase column headers
	var headers []string
	for _, c := range columns {
		headers = append(headers, strings.ToUpper(c))
	}

	return headers, tableRows
}

// printBucketedOutput renders bucketed results with headers per group.
// In quiet mode, only entity IDs are printed (no headers, no bucket separators).
func printBucketedOutput(opts output.Options, grouped mrqlGroupedResponse, raw json.RawMessage) {
	if opts.JSON {
		output.PrintSingle(opts, nil, raw)
		return
	}

	for _, bucket := range grouped.Groups {
		// Parse items as entities
		var entities []mrqlEntity
		if err := json.Unmarshal(bucket.Items, &entities); err != nil {
			continue
		}

		if opts.Quiet {
			// Quiet mode: just IDs, no headers
			for _, e := range entities {
				fmt.Println(strconv.FormatUint(uint64(e.ID), 10))
			}
			continue
		}

		// Print bucket header
		var keyParts []string
		for k, v := range bucket.Key {
			keyParts = append(keyParts, fmt.Sprintf("%s=%v", k, v))
		}
		sort.Strings(keyParts) // stable order
		output.PrintMessage(fmt.Sprintf("--- %s ---", strings.Join(keyParts, ", ")))

		columns := []string{"ID", "NAME", "CREATED"}
		var rows [][]string
		for _, e := range entities {
			rows = append(rows, []string{
				strconv.FormatUint(uint64(e.ID), 10),
				output.Truncate(e.Name, 40),
				e.CreatedAt.Format(time.RFC3339),
			})
		}
		output.Print(opts, columns, rows, nil)
	}
}

func newMRQLDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	help := helptext.Load(mrqlHelpFS, "mrql_help/mrql_delete.md")
	return &cobra.Command{
		Use:         "delete <id>",
		Short:       "Delete a saved MRQL query by ID",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
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
