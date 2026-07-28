# `mr` CLI Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a Go CLI (`mr`) using Cobra that wraps the mahresources JSON API, supporting all CRUD operations, bulk actions, file upload/download, and search.

**Architecture:** Entity-first Cobra subcommand tree (`mr <entity> <action>`). An HTTP client package handles all server communication. An output package handles table/JSON formatting. Commands import `mahresources/models` and `mahresources/models/query_models` directly for type-safe serialization.

**Tech Stack:** Go, Cobra (`github.com/spf13/cobra`), `text/tabwriter` for table output, standard `net/http` + `mime/multipart` for API communication.

---

### Task 1: Add Cobra dependency and scaffold root command

**Files:**
- Create: `cmd/mr/main.go`
- Modify: `go.mod` (via `go get`)

**Step 1: Add Cobra dependency**

Run: `go get github.com/spf13/cobra@latest`

**Step 2: Create `cmd/mr/main.go`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	serverURL string
	jsonOut   bool
	noHeader  bool
	quiet     bool
	page      int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mr",
		Short: "CLI for mahresources",
		Long:  "A command-line interface for interacting with a mahresources server.",
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", envOrDefault("MAHRESOURCES_URL", "http://localhost:8181"), "Server URL (env: MAHRESOURCES_URL)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&noHeader, "no-header", false, "Omit table header")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Only output IDs")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "Page number for list commands")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

**Step 3: Verify it builds**

Run: `go build -o /dev/null ./cmd/mr/`
Expected: builds successfully

**Step 4: Verify help output**

Run: `go run ./cmd/mr/ --help`
Expected: shows "CLI for mahresources" with global flags

**Step 5: Commit**

```bash
git add cmd/mr/main.go go.mod go.sum
git commit -m "feat(cli): scaffold mr root command with global flags"
```

---

### Task 2: HTTP client package

**Files:**
- Create: `cmd/mr/client/client.go`

**Step 1: Create the client package**

This package provides a thin HTTP wrapper used by all commands. It handles:
- Building URLs from base + path + query params
- GET/POST with JSON body
- POST with form data (for endpoints like editName that take `id` query param + form body)
- Multipart file upload
- Streaming file download
- Error parsing (server returns `{"error": "..."}` on failure)

```go
package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

func New(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{},
	}
}

// Get performs a GET request and decodes JSON response into result.
func (c *Client) Get(path string, query url.Values, result any) error {
	u := c.buildURL(path, query)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	return c.doJSON(req, result)
}

// Post performs a POST with JSON body and decodes JSON response into result.
func (c *Client) Post(path string, query url.Values, body any, result any) error {
	u := c.buildURL(path, query)

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPost, u, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doJSON(req, result)
}

// PostForm performs a POST with form-encoded body.
func (c *Client) PostForm(path string, query url.Values, formData url.Values, result any) error {
	u := c.buildURL(path, query)

	req, err := http.NewRequest(http.MethodPost, u, strings.NewReader(formData.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	return c.doJSON(req, result)
}

// Delete performs a DELETE request.
func (c *Client) Delete(path string, query url.Values, result any) error {
	u := c.buildURL(path, query)

	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")

	return c.doJSON(req, result)
}

// Put performs a PUT with JSON body.
func (c *Client) Put(path string, query url.Values, body any, result any) error {
	u := c.buildURL(path, query)

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPut, u, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doJSON(req, result)
}

// Patch performs a PATCH with JSON body.
func (c *Client) Patch(path string, query url.Values, body any, result any) error {
	u := c.buildURL(path, query)

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshaling request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(http.MethodPatch, u, reqBody)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return c.doJSON(req, result)
}

// UploadFile performs a multipart file upload.
// fieldName is the form field for the file (e.g., "resource").
// extraFields are additional form fields to include.
func (c *Client) UploadFile(path string, query url.Values, fieldName string, filePath string, extraFields map[string]string, result any) error {
	u := c.buildURL(path, query)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("opening file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		return fmt.Errorf("creating form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("copying file: %w", err)
	}

	for k, v := range extraFields {
		if err := writer.WriteField(k, v); err != nil {
			return fmt.Errorf("writing field %s: %w", k, err)
		}
	}

	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, u, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return c.doJSON(req, result)
}

// DownloadFile streams a GET response body to a local file.
// Returns the number of bytes written.
func (c *Client) DownloadFile(path string, query url.Values, destPath string) (int64, error) {
	u := c.buildURL(path, query)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	out, err := os.Create(destPath)
	if err != nil {
		return 0, fmt.Errorf("creating file: %w", err)
	}
	defer out.Close()

	n, err := io.Copy(out, resp.Body)
	if err != nil {
		return n, fmt.Errorf("writing file: %w", err)
	}

	return n, nil
}

// GetRaw performs a GET and returns the raw response (for streaming/binary).
func (c *Client) GetRaw(path string, query url.Values) (*http.Response, error) {
	u := c.buildURL(path, query)

	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	return resp, nil
}

func (c *Client) buildURL(path string, query url.Values) string {
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	return u
}

func (c *Client) doJSON(req *http.Request, result any) error {
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to parse error JSON
		var errResp struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error != "" {
			return fmt.Errorf("server error %d: %s", resp.StatusCode, errResp.Error)
		}
		return fmt.Errorf("server error %d: %s", resp.StatusCode, string(body))
	}

	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("decoding response: %w", err)
		}
	}

	return nil
}
```

**Step 2: Verify it builds**

Run: `go build -o /dev/null ./cmd/mr/...`
Expected: builds successfully

**Step 3: Commit**

```bash
git add cmd/mr/client/
git commit -m "feat(cli): add HTTP client package"
```

---

### Task 3: Output formatting package

**Files:**
- Create: `cmd/mr/output/output.go`

**Step 1: Create the output package**

This package handles table vs JSON output. Commands call `output.Print(opts, columns, rows)` where `columns` is `[]string` and `rows` is `[][]string`. If `--json` is set, it outputs the raw JSON instead. If `--quiet` is set, it outputs only the first column (assumed to be ID).

```go
package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

type Options struct {
	JSON     bool
	NoHeader bool
	Quiet    bool
}

// Print outputs data as a table, JSON, or quiet mode (IDs only).
// For JSON mode, rawJSON is printed directly if non-nil; otherwise columns/rows are converted.
func Print(opts Options, columns []string, rows [][]string, rawJSON json.RawMessage) {
	if opts.JSON && rawJSON != nil {
		var indented bytes.Buffer
		if json.Indent(&indented, rawJSON, "", "  ") == nil {
			fmt.Println(indented.String())
		} else {
			fmt.Println(string(rawJSON))
		}
		return
	}

	if opts.Quiet {
		for _, row := range rows {
			if len(row) > 0 {
				fmt.Println(row[0])
			}
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	if !opts.NoHeader {
		fmt.Fprintln(w, strings.Join(columns, "\t"))
		dashes := make([]string, len(columns))
		for i, col := range columns {
			dashes[i] = strings.Repeat("-", len(col))
		}
		fmt.Fprintln(w, strings.Join(dashes, "\t"))
	}

	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
}

// PrintSingle outputs a single entity as key-value pairs or JSON.
func PrintSingle(opts Options, fields []KeyValue, rawJSON json.RawMessage) {
	if opts.JSON && rawJSON != nil {
		var indented bytes.Buffer
		if json.Indent(&indented, rawJSON, "", "  ") == nil {
			fmt.Println(indented.String())
		} else {
			fmt.Println(string(rawJSON))
		}
		return
	}

	if opts.Quiet {
		// Print first field value (assumed to be ID)
		if len(fields) > 0 {
			fmt.Println(fields[0].Value)
		}
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, f := range fields {
		fmt.Fprintf(w, "%s:\t%s\n", f.Key, f.Value)
	}
	w.Flush()
}

type KeyValue struct {
	Key   string
	Value string
}

// PrintMessage prints a simple message (for delete confirmations, etc.)
func PrintMessage(msg string) {
	fmt.Println(msg)
}

// Truncate shortens a string to maxLen, appending "..." if truncated.
func Truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		return s[:maxLen-3] + "..."
	}
	return s
}
```

Note: add `"bytes"` to the imports.

**Step 2: Verify it builds**

Run: `go build -o /dev/null ./cmd/mr/...`
Expected: builds successfully

**Step 3: Commit**

```bash
git add cmd/mr/output/
git commit -m "feat(cli): add output formatting package (table/JSON/quiet)"
```

---

### Task 4: Tags commands (first entity — establishes the pattern)

Tags are the simplest entity, so implement them first to establish the command pattern for all others.

**Files:**
- Create: `cmd/mr/commands/tags.go`
- Modify: `cmd/mr/main.go` (register commands)

**Step 1: Create `cmd/mr/commands/tags.go`**

This file establishes the pattern: each entity file exports a function `NewXxxCmd(...)` that returns a `*cobra.Command` with subcommands. The function receives the shared state (serverURL, output opts) via a closure or a shared config struct.

```go
package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
	"mahresources/models"

	"github.com/spf13/cobra"
)

func NewTagCmd(c *client.Client, opts *output.Options) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tag",
		Short: "Manage a single tag",
	}

	cmd.AddCommand(newTagGetCmd(c, opts))
	cmd.AddCommand(newTagCreateCmd(c, opts))
	cmd.AddCommand(newTagDeleteCmd(c, opts))
	cmd.AddCommand(newTagEditNameCmd(c, opts))
	cmd.AddCommand(newTagEditDescriptionCmd(c, opts))

	return cmd
}

func NewTagsCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tags",
		Short: "List and bulk manage tags",
	}

	cmd.AddCommand(newTagsListCmd(c, opts, page))
	cmd.AddCommand(newTagsMergeCmd(c, opts))
	cmd.AddCommand(newTagsBulkDeleteCmd(c, opts))

	return cmd
}

func newTagsListCmd(c *client.Client, opts *output.Options, page *int) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(*page))
			if name != "" {
				q.Set("Name", name)
			}
			if description != "" {
				q.Set("Description", description)
			}

			var raw json.RawMessage
			if err := c.Get("/v1/tags", q, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.Print(*opts, nil, nil, raw)
				return nil
			}

			var tags []models.Tag
			if err := json.Unmarshal(raw, &tags); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			columns := []string{"ID", "NAME", "DESCRIPTION", "CREATED"}
			var rows [][]string
			for _, t := range tags {
				rows = append(rows, []string{
					fmt.Sprint(t.ID),
					output.Truncate(t.Name, 40),
					output.Truncate(t.Description, 50),
					t.CreatedAt.Format("2006-01-02"),
				})
			}

			output.Print(*opts, columns, rows, nil)
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Filter by name")
	cmd.Flags().StringVar(&description, "description", "", "Filter by description")

	return cmd
}

func newTagGetCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "get <id>",
		Short: "Get a tag by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			var raw json.RawMessage
			if err := c.Get("/v1/tags", q, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
				return nil
			}

			var tag models.Tag
			if err := json.Unmarshal(raw, &tag); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			fields := []output.KeyValue{
				{"ID", fmt.Sprint(tag.ID)},
				{"Name", tag.Name},
				{"Description", tag.Description},
				{"Created", tag.CreatedAt.Format("2006-01-02 15:04:05")},
				{"Updated", tag.UpdatedAt.Format("2006-01-02 15:04:05")},
			}

			output.PrintSingle(*opts, fields, nil)
			return nil
		},
	}
}

func newTagCreateCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var name, description string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"Name":        name,
				"Description": description,
			}

			var raw json.RawMessage
			if err := c.Post("/v1/tag", nil, body, &raw); err != nil {
				return err
			}

			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
				return nil
			}

			var tag models.Tag
			if err := json.Unmarshal(raw, &tag); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}

			output.PrintMessage(fmt.Sprintf("Created tag %d: %s", tag.ID, tag.Name))
			return nil
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Tag name (required)")
	cmd.Flags().StringVar(&description, "description", "", "Tag description")
	cmd.MarkFlagRequired("name")

	return cmd
}

func newTagDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete a tag",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("Id", args[0])

			if err := c.Post("/v1/tag/delete", q, nil, nil); err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Deleted tag %s", args[0]))
			return nil
		},
	}
}

func newTagEditNameCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-name <id> <new-name>",
		Short: "Edit a tag's name",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			formData := url.Values{}
			formData.Set("value", args[1])

			if err := c.PostForm("/v1/tag/editName", q, formData, nil); err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Updated tag %s name to: %s", args[0], args[1]))
			return nil
		},
	}
}

func newTagEditDescriptionCmd(c *client.Client, opts *output.Options) *cobra.Command {
	return &cobra.Command{
		Use:   "edit-description <id> <new-description>",
		Short: "Edit a tag's description",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("id", args[0])

			formData := url.Values{}
			formData.Set("value", args[1])

			if err := c.PostForm("/v1/tag/editDescription", q, formData, nil); err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Updated tag %s description", args[0]))
			return nil
		},
	}
}

func newTagsMergeCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var winner uint
	var losers []uint

	cmd := &cobra.Command{
		Use:   "merge",
		Short: "Merge tags into a primary tag",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"Winner": winner,
				"Losers": losers,
			}

			if err := c.Post("/v1/tags/merge", nil, body, nil); err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Merged tags %v into tag %d", losers, winner))
			return nil
		},
	}

	cmd.Flags().UintVar(&winner, "winner", 0, "Primary tag ID (required)")
	cmd.Flags().UintSliceVar(&losers, "losers", nil, "Tag IDs to merge into primary (required)")
	cmd.MarkFlagRequired("winner")
	cmd.MarkFlagRequired("losers")

	return cmd
}

func newTagsBulkDeleteCmd(c *client.Client, opts *output.Options) *cobra.Command {
	var ids []uint

	cmd := &cobra.Command{
		Use:   "delete",
		Short: "Bulk delete tags",
		RunE: func(cmd *cobra.Command, args []string) error {
			body := map[string]any{
				"ID": ids,
			}

			if err := c.Post("/v1/tags/delete", nil, body, nil); err != nil {
				return err
			}

			output.PrintMessage(fmt.Sprintf("Deleted %d tags", len(ids)))
			return nil
		},
	}

	cmd.Flags().UintSliceVar(&ids, "ids", nil, "Tag IDs to delete (required)")
	cmd.MarkFlagRequired("ids")

	return cmd
}
```

**Step 2: Update `cmd/mr/main.go` to register tag commands and wire up client/output**

Update `main.go` to create the client and output options, then register the tag commands:

```go
package main

import (
	"fmt"
	"os"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/commands"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

var (
	serverURL string
	jsonOut   bool
	noHeader  bool
	quiet     bool
	page      int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mr",
		Short: "CLI for mahresources",
		Long:  "A command-line interface for interacting with a mahresources server.",
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", envOrDefault("MAHRESOURCES_URL", "http://localhost:8181"), "Server URL (env: MAHRESOURCES_URL)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&noHeader, "no-header", false, "Omit table header")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Only output IDs")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "Page number for list commands")

	// These are resolved at command execution time via PersistentPreRun
	var c *client.Client
	opts := &output.Options{}

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		c = client.New(serverURL)
		opts.JSON = jsonOut
		opts.NoHeader = noHeader
		opts.Quiet = quiet
	}

	// Use a lazy wrapper so c and opts are set by PersistentPreRun
	addCommands := func() {
		rootCmd.AddCommand(commands.NewTagCmd(nil, nil))
		rootCmd.AddCommand(commands.NewTagsCmd(nil, nil, nil))
	}

	// Actually, we need client at command construction time for closures.
	// Instead, pass pointers that get populated by PersistentPreRun.
	// Simpler approach: create client eagerly with default, update in PreRun.

	// Simplest: create client and opts now, update serverURL in PreRun.
	c = client.New("http://localhost:8181") // placeholder, overwritten
	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		*c = *client.New(serverURL)
		opts.JSON = jsonOut
		opts.NoHeader = noHeader
		opts.Quiet = quiet
	}

	rootCmd.AddCommand(commands.NewTagCmd(c, opts))
	rootCmd.AddCommand(commands.NewTagsCmd(c, opts, &page))

	_ = addCommands // unused, remove

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

Actually, clean this up — remove the `addCommands` cruft. The pattern is: create a placeholder `client.Client` and `output.Options`, pass pointers to commands, update them in `PersistentPreRun`. Final `main.go`:

```go
package main

import (
	"fmt"
	"os"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/commands"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

var (
	serverURL string
	jsonOut   bool
	noHeader  bool
	quiet     bool
	page      int
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mr",
		Short: "CLI for mahresources",
		Long:  "A command-line interface for interacting with a mahresources server.",
	}

	rootCmd.PersistentFlags().StringVar(&serverURL, "server", envOrDefault("MAHRESOURCES_URL", "http://localhost:8181"), "Server URL (env: MAHRESOURCES_URL)")
	rootCmd.PersistentFlags().BoolVar(&jsonOut, "json", false, "Output as JSON")
	rootCmd.PersistentFlags().BoolVar(&noHeader, "no-header", false, "Omit table header")
	rootCmd.PersistentFlags().BoolVar(&quiet, "quiet", false, "Only output IDs")
	rootCmd.PersistentFlags().IntVar(&page, "page", 1, "Page number for list commands")

	c := client.New("http://localhost:8181")
	opts := &output.Options{}

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, args []string) {
		*c = *client.New(serverURL)
		opts.JSON = jsonOut
		opts.NoHeader = noHeader
		opts.Quiet = quiet
	}

	rootCmd.AddCommand(commands.NewTagCmd(c, opts))
	rootCmd.AddCommand(commands.NewTagsCmd(c, opts, &page))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
```

**Step 3: Verify it builds**

Run: `go build -o /dev/null ./cmd/mr/`
Expected: builds successfully

**Step 4: Verify help output shows tag commands**

Run: `go run ./cmd/mr/ --help`
Expected: shows `tag` and `tags` in available commands

Run: `go run ./cmd/mr/ tag --help`
Expected: shows `get`, `create`, `delete`, `edit-name`, `edit-description`

Run: `go run ./cmd/mr/ tags --help`
Expected: shows `list`, `merge`, `delete`

**Step 5: Integration test against ephemeral server**

Run: `npm run build && ./mahresources -ephemeral -bind-address=:18181 &`
Then:
```bash
go run ./cmd/mr/ --server http://localhost:18181 tag create --name "test-tag" --description "from CLI"
go run ./cmd/mr/ --server http://localhost:18181 tags list
go run ./cmd/mr/ --server http://localhost:18181 tags list --json
go run ./cmd/mr/ --server http://localhost:18181 tag get 1
go run ./cmd/mr/ --server http://localhost:18181 tag edit-name 1 "renamed-tag"
go run ./cmd/mr/ --server http://localhost:18181 tag delete 1
```
Kill the ephemeral server when done.

**Step 6: Commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add tag/tags commands (first entity)"
```

---

### Task 5: Categories and Resource Categories commands

These follow the exact same pattern as tags. Simple entities with list, create, delete, edit-name, edit-description.

**Files:**
- Create: `cmd/mr/commands/categories.go`
- Create: `cmd/mr/commands/resource_categories.go`
- Modify: `cmd/mr/main.go` (register new commands)

**Step 1: Create categories.go**

Follow the exact same pattern as `tags.go`:
- `NewCategoryCmd` — `get`, `create`, `delete`, `edit-name`, `edit-description`
- `NewCategoriesCmd` — `list`
- List endpoint: `GET /v1/categories` with `Name`, `Description` filters
- Single: `GET /v1/categories?id=` (note: uses the list endpoint with `id` query param — check actual endpoint; it may be different)
- Create: `POST /v1/category` with `CategoryEditor` fields (`Name`, `Description`, `CustomHeader`, `CustomSidebar`, `CustomSummary`, `CustomAvatar`, `MetaSchema`)
- Delete: `POST /v1/category/delete?Id=`
- EditName: `POST /v1/category/editName?id=`
- EditDescription: `POST /v1/category/editDescription?id=`
- Table columns: `ID`, `NAME`, `DESCRIPTION`, `CREATED`

**Step 2: Create resource_categories.go**

Same pattern with `resource-category` / `resource-categories` as command names:
- List: `GET /v1/resourceCategories`
- Create: `POST /v1/resourceCategory`
- Delete: `POST /v1/resourceCategory/delete?Id=`
- EditName: `POST /v1/resourceCategory/editName?id=`
- EditDescription: `POST /v1/resourceCategory/editDescription?id=`

**Step 3: Register in main.go**

```go
rootCmd.AddCommand(commands.NewCategoryCmd(c, opts))
rootCmd.AddCommand(commands.NewCategoriesCmd(c, opts, &page))
rootCmd.AddCommand(commands.NewResourceCategoryCmd(c, opts))
rootCmd.AddCommand(commands.NewResourceCategoriesCmd(c, opts, &page))
```

**Step 4: Build and verify help**

Run: `go build -o /dev/null ./cmd/mr/`

**Step 5: Commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add category and resource-category commands"
```

---

### Task 6: Notes commands

**Files:**
- Create: `cmd/mr/commands/notes.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create notes.go**

Notes are more complex than tags:

Singular (`note`):
- `get <id>` — `GET /v1/note?id=`
- `create` — `POST /v1/note` with `NoteEditor` fields: `--name`, `--description`, `--tags` (uint slice), `--groups` (uint slice), `--resources` (uint slice), `--meta` (string), `--owner-id`, `--note-type-id`
- `delete <id>` — `POST /v1/note/delete?Id=`
- `edit-name <id> <value>` — `POST /v1/note/editName?id=`
- `edit-description <id> <value>` — `POST /v1/note/editDescription?id=`
- `share <id>` — `POST /v1/note/share?noteId=`
- `unshare <id>` — `DELETE /v1/note/share?noteId=`

Plural (`notes`):
- `list` — `GET /v1/notes` with filter flags: `--name`, `--description`, `--tags`, `--groups`, `--owner-id`, `--note-type-id`, `--created-before`, `--created-after`
- `add-tags` — `POST /v1/notes/addTags` with `--ids` and `--tags`
- `remove-tags` — `POST /v1/notes/removeTags`
- `add-groups` — `POST /v1/notes/addGroups`
- `add-meta` — `POST /v1/notes/addMeta` with `--ids` and `--meta`
- `delete` — `POST /v1/notes/delete` with `--ids`
- `meta-keys` — `GET /v1/notes/meta/keys`

Table columns for list: `ID`, `NAME`, `TYPE`, `OWNER`, `DESCRIPTION`, `CREATED`

For the `get` single display, show key-value pairs including tag names, group names, resource count.

**Step 2: Register in main.go**

**Step 3: Build, verify help**

**Step 4: Commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add note/notes commands"
```

---

### Task 7: Note types commands

**Files:**
- Create: `cmd/mr/commands/note_types.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create note_types.go**

Singular (`note-type`):
- `get <id>` — needs to look this up (may need to list and filter, or check if there's a direct endpoint)
- `create` — `POST /v1/note/noteType` with `NoteTypeEditor` fields: `--name`, `--description`
- `edit <id>` — `POST /v1/note/noteType/edit` with `NoteTypeEditor` (includes `ID`)
- `delete <id>` — `POST /v1/note/noteType/delete?Id=`
- `edit-name <id> <value>` — `POST /v1/noteType/editName?id=`
- `edit-description <id> <value>` — `POST /v1/noteType/editDescription?id=`

Plural (`note-types`):
- `list` — `GET /v1/note/noteTypes` with `--name`, `--description`

Table columns: `ID`, `NAME`, `DESCRIPTION`, `CREATED`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add note-type/note-types commands"
```

---

### Task 8: Note blocks commands

**Files:**
- Create: `cmd/mr/commands/note_blocks.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create note_blocks.go**

Singular (`note-block`):
- `get <id>` — `GET /v1/note/block?id=`
- `create` — `POST /v1/note/block` with `NoteBlockEditor`: `--note-id`, `--type`, `--content` (JSON string), `--position`
- `update <id>` — `PUT /v1/note/block?id=` with content JSON body
- `update-state <id>` — `PATCH /v1/note/block/state?id=` with state JSON body
- `delete <id>` — `DELETE /v1/note/block?id=` or `POST /v1/note/block/delete?id=`
- `types` — `GET /v1/note/block/types`

Plural (`note-blocks`):
- `list --note-id <id>` — `GET /v1/note/blocks?noteId=`
- `reorder --note-id <id> --positions '{"1":"a","2":"b"}'` — `POST /v1/note/blocks/reorder`
- `rebalance --note-id <id>` — `POST /v1/note/blocks/rebalance?noteId=`

Table columns for list: `ID`, `TYPE`, `POSITION`, `CREATED`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add note-block/note-blocks commands"
```

---

### Task 9: Groups commands

**Files:**
- Create: `cmd/mr/commands/groups.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create groups.go**

Singular (`group`):
- `get <id>` — `GET /v1/group?id=`
- `create` — `POST /v1/group` with `GroupEditor`: `--name`, `--description`, `--tags`, `--groups`, `--category-id`, `--owner-id`, `--meta`, `--url`
- `delete <id>` — `POST /v1/group/delete?Id=`
- `edit-name <id> <value>` — `POST /v1/group/editName?id=`
- `edit-description <id> <value>` — `POST /v1/group/editDescription?id=`
- `parents <id>` — `GET /v1/group/parents?id=`
- `children <id>` — `GET /v1/group/tree/children?id=`
- `clone <id>` — `POST /v1/group/clone` with `EntityIdQuery`

Plural (`groups`):
- `list` — `GET /v1/groups` with many filter flags
- `add-tags`, `remove-tags`, `add-meta`, `delete`, `merge`
- `meta-keys` — `GET /v1/groups/meta/keys`

Table columns: `ID`, `NAME`, `CATEGORY`, `OWNER`, `DESCRIPTION`, `CREATED`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add group/groups commands"
```

---

### Task 10: Relations and relation types commands

**Files:**
- Create: `cmd/mr/commands/relations.go`
- Create: `cmd/mr/commands/relation_types.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create relations.go**

Singular (`relation`):
- `create` — `POST /v1/relation` with `GroupRelationshipQuery`: `--from-group-id`, `--to-group-id`, `--relation-type-id`, `--name`, `--description`
- `delete <id>` — `POST /v1/relation/delete?Id=`
- `edit-name <id> <value>` — `POST /v1/relation/editName?id=`
- `edit-description <id> <value>` — `POST /v1/relation/editDescription?id=`

**Step 2: Create relation_types.go**

Singular (`relation-type`):
- `create` — `POST /v1/relationType` with `RelationshipTypeEditorQuery`: `--name`, `--description`, `--from-category`, `--to-category`, `--reverse-name`
- `edit` — `POST /v1/relationType/edit`
- `delete <id>` — `POST /v1/relationType/delete?Id=`

Plural (`relation-types`):
- `list` — `GET /v1/relationTypes`

Table columns: `ID`, `NAME`, `DESCRIPTION`, `FROM_CATEGORY`, `TO_CATEGORY`

**Step 3: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add relation and relation-type commands"
```

---

### Task 11: Resources commands (CRUD + file operations)

This is the largest and most complex entity. Split into basic CRUD and file operations.

**Files:**
- Create: `cmd/mr/commands/resources.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create resources.go with basic CRUD**

Singular (`resource`):
- `get <id>` — `GET /v1/resource?id=`
- `create` — see upload below
- `edit <id>` — `POST /v1/resource/edit` with `ResourceEditor`
- `delete <id>` — `POST /v1/resource/delete?Id=`
- `edit-name <id> <value>` — `POST /v1/resource/editName?id=`
- `edit-description <id> <value>` — `POST /v1/resource/editDescription?id=`

**Step 2: Add file operations**

- `upload <file> [flags]` — `POST /v1/resource` multipart. Flags: `--name`, `--description`, `--tags`, `--groups`, `--owner-id`, `--meta`, `--category`, `--resource-category-id`. Uses `c.UploadFile("/v1/resource", ...)` with field name `"resource"`.
- `download <id> [-o file]` — `GET /v1/resource/view?id=`. If no `-o`, derive filename from Content-Disposition header or use `resource_<id>`.
- `preview <id> [-w width] [-h height] [-o file]` — `GET /v1/resource/preview?ID=&Width=&Height=`
- `from-url` — `POST /v1/resource/remote` with `--url`, `--name`, `--tags`, `--groups`, etc.
- `from-local` — `POST /v1/resource/local` with `--path`, `--name`, etc.
- `rotate <id> --degrees N` — `POST /v1/resources/rotate`
- `recalculate-dimensions <id>` — `POST /v1/resource/recalculateDimensions`

**Step 3: Add version subcommands**

- `versions <resource-id>` — `GET /v1/resource/versions?resourceId=`
- `version <version-id>` — `GET /v1/resource/version?id=`
- `version-upload <resource-id> <file>` — `POST /v1/resource/versions?resourceId=` multipart with `--comment`
- `version-download <version-id> [-o file]` — `GET /v1/resource/version/file?versionId=`
- `version-restore` — `POST /v1/resource/version/restore` with `--resource-id`, `--version-id`, `--comment`
- `version-delete` — `DELETE /v1/resource/version?resourceId=&versionId=`
- `versions-cleanup <resource-id>` — `POST /v1/resource/versions/cleanup` with `--keep`, `--older-than-days`, `--dry-run`
- `versions-compare <resource-id> --v1 X --v2 Y` — `GET /v1/resource/versions/compare`

Plural (`resources`):
- `list` — `GET /v1/resources` with many filter flags: `--name`, `--description`, `--content-type`, `--owner-id`, `--tags`, `--groups`, `--notes`, `--resource-category-id`, `--created-before`, `--created-after`, `--min-width`, `--min-height`, `--max-width`, `--max-height`, `--hash`, `--original-name`, `--sort-by`
- `add-tags`, `remove-tags`, `replace-tags`, `add-groups`, `add-meta`, `delete`, `merge`
- `set-dimensions --ids ... --width ... --height ...`
- `versions-cleanup --ids ... [--keep ...]` — `POST /v1/resources/versions/cleanup`
- `meta-keys` — `GET /v1/resources/meta/keys`

Table columns: `ID`, `NAME`, `TYPE`, `SIZE`, `DIMENSIONS`, `OWNER`, `CREATED`

**Step 4: Register, build, verify help**

**Step 5: Integration test file upload/download against ephemeral server**

**Step 6: Commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add resource/resources commands with file operations"
```

---

### Task 12: Series commands

**Files:**
- Create: `cmd/mr/commands/series.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create series.go**

Singular (`series`):
- `get <id>` — `GET /v1/series?id=`
- `create` — `POST /v1/series/create` with `SeriesCreator`: `--name`
- `edit <id>` — `POST /v1/series` with `SeriesEditor`: `--name`, `--meta`
- `delete <id>` — `POST /v1/series/delete?Id=`
- `remove-resource <resource-id>` — `POST /v1/resource/removeSeries?id=`

Plural (`series` is already plural — use `series list`):
- `list` — `GET /v1/seriesList` with `--name`, `--slug`

Table columns: `ID`, `NAME`, `SLUG`, `CREATED`

Note: since `series` is both singular and plural, put all commands under one `series` command.

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add series commands"
```

---

### Task 13: Queries commands

**Files:**
- Create: `cmd/mr/commands/queries.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create queries.go**

Singular (`query`):
- `get <id>` — `GET /v1/query?id=`
- `create` — `POST /v1/query` with `QueryEditor`: `--name`, `--text`, `--template`
- `delete <id>` — `POST /v1/query/delete?Id=`
- `edit-name <id> <value>`
- `edit-description <id> <value>`
- `run <id>` — `POST /v1/query/run?id=` (output raw JSON result)
- `run --name <name>` — `POST /v1/query/run?name=`
- `schema` — `GET /v1/query/schema` (output raw JSON)

Plural (`queries`):
- `list` — `GET /v1/queries`

Table columns: `ID`, `NAME`, `DESCRIPTION`, `CREATED`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add query/queries commands"
```

---

### Task 14: Search command

**Files:**
- Create: `cmd/mr/commands/search.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create search.go**

Single command `search`:
- `search <query> [--types resources,notes --limit 20]`
- `POST /v1/search` with `GlobalSearchQuery` body: `{"q": "...", "limit": 20, "types": ["resources", "notes"]}`

Table columns: `ID`, `TYPE`, `NAME`, `SCORE`, `DESCRIPTION`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add search command"
```

---

### Task 15: Logs commands

**Files:**
- Create: `cmd/mr/commands/logs.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create logs.go**

Singular (`log`):
- `get <id>` — `GET /v1/log?id=`
- `entity --entity-type <type> --entity-id <id>` — `GET /v1/logs/entity?entityType=&entityId=`

Plural (`logs`):
- `list` — `GET /v1/logs` with filters: `--level`, `--action`, `--entity-type`, `--entity-id`, `--message`, `--created-before`, `--created-after`

Table columns: `ID`, `LEVEL`, `ACTION`, `ENTITY_TYPE`, `ENTITY_ID`, `MESSAGE`, `CREATED`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add log/logs commands"
```

---

### Task 16: Jobs commands

**Files:**
- Create: `cmd/mr/commands/jobs.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create jobs.go**

Singular (`job`):
- `submit --urls "url1,url2" [--tags ... --groups ...]` — `POST /v1/jobs/download/submit`
- `cancel <id>` — `POST /v1/jobs/cancel?id=`
- `pause <id>` — `POST /v1/jobs/pause?id=`
- `resume <id>` — `POST /v1/jobs/resume?id=`
- `retry <id>` — `POST /v1/jobs/retry?id=`

Plural (`jobs`):
- `list` — `GET /v1/jobs/queue`

Table columns: `ID`, `URL`, `STATUS`, `PROGRESS`, `CREATED`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add job/jobs commands"
```

---

### Task 17: Plugins commands

**Files:**
- Create: `cmd/mr/commands/plugins.go`
- Modify: `cmd/mr/main.go`

**Step 1: Create plugins.go**

Singular (`plugin`):
- `enable <name>` — `POST /v1/plugin/enable`
- `disable <name>` — `POST /v1/plugin/disable`
- `settings <name> --data '{...}'` — `POST /v1/plugin/settings`
- `purge-data <name>` — `POST /v1/plugin/purge-data`

Plural (`plugins`):
- `list` — `GET /v1/plugins/manage`

Table columns: `NAME`, `ENABLED`, `DESCRIPTION`

**Step 2: Register, build, commit**

```bash
git add cmd/mr/
git commit -m "feat(cli): add plugin/plugins commands"
```

---

### Task 18: End-to-end integration test

**Files:**
- None new (manual verification)

**Step 1: Build the CLI**

Run: `go build --tags 'json1 fts5' -o ./mr ./cmd/mr/`

**Step 2: Start ephemeral server**

Run: `npm run build && ./mahresources -ephemeral -bind-address=:18181 &`

**Step 3: Exercise every entity**

```bash
export MAHRESOURCES_URL=http://localhost:18181

# Tags
./mr tag create --name "test-tag"
./mr tags list
./mr tag get 1
./mr tag edit-name 1 "renamed"
./mr tags list --json
./mr tags list --quiet

# Categories
./mr category create --name "test-cat"
./mr categories list

# Notes
./mr note create --name "test-note" --description "hello"
./mr notes list
./mr note get 1

# Groups
./mr group create --name "test-group"
./mr groups list
./mr group get 1

# Resources (upload)
echo "hello" > /tmp/test.txt
./mr resource upload /tmp/test.txt --name "test-file"
./mr resources list
./mr resource download 1 -o /tmp/downloaded.txt

# Search
./mr search "test"

# Queries
./mr query schema

# Logs
./mr logs list

# Jobs
./mr jobs list
```

**Step 4: Verify all commands work, fix any issues**

**Step 5: Kill ephemeral server and clean up**

**Step 6: Final commit (if any fixes)**

```bash
git commit -m "fix(cli): integration test fixes"
```

---

### Task 19: Add `mr` to build scripts (optional)

**Files:**
- Modify: `package.json` (add `build-cli` script)

**Step 1: Add build script**

Add to `package.json` scripts:
```json
"build-cli": "go build --tags 'json1 fts5' -o mr ./cmd/mr/"
```

**Step 2: Commit**

```bash
git add package.json
git commit -m "feat(cli): add build-cli npm script"
```

---

Plan complete and saved to `docs/plans/2026-03-14-mr-cli-impl.md`. Two execution options:

**1. Subagent-Driven (this session)** — I dispatch fresh subagent per task, review between tasks, fast iteration

**2. Parallel Session (separate)** — Open new session with executing-plans, batch execution with checkpoints

Which approach?