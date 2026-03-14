package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// Options controls output formatting.
type Options struct {
	JSON     bool
	NoHeader bool
	Quiet    bool
}

// KeyValue represents a single key-value pair for single-entity display.
type KeyValue struct {
	Key   string
	Value string
}

// Print outputs tabular data. In JSON mode it pretty-prints rawJSON.
// In Quiet mode it prints only the first column (assumed to be ID).
func Print(opts Options, columns []string, rows [][]string, rawJSON json.RawMessage) {
	if opts.JSON && rawJSON != nil {
		printJSON(rawJSON)
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

	if !opts.NoHeader && len(columns) > 0 {
		fmt.Fprintln(w, strings.Join(columns, "\t"))
	}

	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}

	w.Flush()
}

// PrintSingle outputs a single entity as key-value pairs.
// In JSON mode it pretty-prints rawJSON.
// If no fields are provided but rawJSON is present, prints the JSON regardless of mode.
func PrintSingle(opts Options, fields []KeyValue, rawJSON json.RawMessage) {
	if opts.JSON && rawJSON != nil {
		printJSON(rawJSON)
		return
	}

	if len(fields) == 0 && rawJSON != nil {
		// No structured fields — fall back to printing JSON
		printJSON(rawJSON)
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, f := range fields {
		fmt.Fprintf(w, "%s:\t%s\n", f.Key, f.Value)
	}
	w.Flush()
}

// PrintRawJSON always prints raw JSON, regardless of output mode.
// Used for endpoints with variable/unknown response shapes.
func PrintRawJSON(raw json.RawMessage) {
	printJSON(raw)
}

// PrintMessage prints a simple message to stdout.
func PrintMessage(msg string) {
	fmt.Println(msg)
}

// Truncate shortens a string to maxLen characters, appending "..." if truncated.
// Newlines are replaced with spaces.
func Truncate(s string, maxLen int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > maxLen {
		if maxLen <= 3 {
			return s[:maxLen]
		}
		return s[:maxLen-3] + "..."
	}
	return s
}

func printJSON(raw json.RawMessage) {
	var indented bytes.Buffer
	if err := json.Indent(&indented, raw, "", "  "); err != nil {
		// Fallback: print as-is
		fmt.Println(string(raw))
		return
	}
	fmt.Println(indented.String())
}
