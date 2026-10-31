package commands

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/helptext"
	"mahresources/cmd/mr/output"
)

// templateRevisionsSpec names one Template Carrier's revision endpoints and the
// help files its command group loads. The three carriers share every
// subcommand; only the wording and the endpoint prefix differ.
type templateRevisionsSpec struct {
	noun     string // "category", "resource category" or "note type"
	endpoint string // the carrier's /v1 prefix; revision routes hang off it
	helpFS   embed.FS
	helpDir  string // directory of the help files inside helpFS
	helpBase string // file name stem, e.g. "categories_revisions"
}

func (s templateRevisionsSpec) help(suffix string) helptext.Help {
	return helptext.Load(s.helpFS, s.helpDir+"/"+s.helpBase+suffix+".md")
}

// templateRevisionSummary mirrors the API's revision summary.
type templateRevisionSummary struct {
	ID              uint      `json:"id"`
	Number          uint      `json:"number"`
	CarrierID       uint      `json:"carrierId"`
	CreatedAt       time.Time `json:"createdAt"`
	CreatedByUserID *uint     `json:"createdByUserId"`
	Label           string    `json:"label"`
	Source          string    `json:"source"`
	Applied         []string  `json:"applied"`
	ChangedSlots    []string  `json:"changedSlots"`
	RestoredFrom    *uint     `json:"restoredFrom"`
}

type templateRevisionDetail struct {
	templateRevisionSummary
	Snapshot map[string]any `json:"snapshot"`
}

type templateRevisionDiffResponse struct {
	CarrierID uint  `json:"carrierId"`
	From      uint  `json:"from"`
	To        *uint `json:"to"`
	Slots     []struct {
		Slot   string `json:"slot"`
		Fields []struct {
			Field string `json:"field"`
			Hunks []struct {
				Op    string   `json:"op"`
				Lines []string `json:"lines"`
			} `json:"hunks"`
		} `json:"fields"`
	} `json:"slots"`
}

// newTemplateRevisionsCmd builds the "revisions" group for one carrier.
func newTemplateRevisionsCmd(c *client.Client, opts *output.Options, spec templateRevisionsSpec) *cobra.Command {
	help := spec.help("")
	cmd := &cobra.Command{
		Use:         "revisions",
		Short:       "List, diff, label or restore Template Revisions",
		Long:        help.Long,
		Annotations: help.Annotations,
	}
	cmd.AddCommand(
		newTemplateRevisionsListCmd(c, opts, spec),
		newTemplateRevisionsShowCmd(c, opts, spec),
		newTemplateRevisionsDiffCmd(c, opts, spec),
		newTemplateRevisionsRestoreCmd(c, opts, spec),
		newTemplateRevisionsLabelCmd(c, opts, spec),
	)
	return cmd
}

func revisionQuery(id uint) (url.Values, error) {
	if id == 0 {
		return nil, fmt.Errorf("--id must be positive")
	}
	q := url.Values{}
	q.Set("id", strconv.FormatUint(uint64(id), 10))
	return q, nil
}

func parseRevisionNumber(arg string) (uint, error) {
	n, err := strconv.ParseUint(arg, 10, 32)
	if err != nil || n == 0 {
		return 0, fmt.Errorf("invalid revision number %q: must be a positive integer", arg)
	}
	return uint(n), nil
}

func formatRevisionUser(id *uint) string {
	if id == nil {
		return "-"
	}
	return strconv.FormatUint(uint64(*id), 10)
}

func newTemplateRevisionsListCmd(c *client.Client, opts *output.Options, spec templateRevisionsSpec) *cobra.Command {
	var id uint
	help := spec.help("_list")
	cmd := &cobra.Command{
		Use:         "list",
		Short:       "List the revisions of a " + spec.noun,
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := revisionQuery(id)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := c.Get(spec.endpoint+"/revisions", q, &raw); err != nil {
				return err
			}
			var revisions []templateRevisionSummary
			if err := json.Unmarshal(raw, &revisions); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			columns := []string{"NUMBER", "SOURCE", "CHANGED", "LABEL", "USER", "CREATED"}
			var rows [][]string
			for _, r := range revisions {
				rows = append(rows, []string{
					strconv.FormatUint(uint64(r.Number), 10),
					r.Source,
					output.Truncate(strings.Join(r.ChangedSlots, ","), 50),
					output.Truncate(r.Label, 40),
					formatRevisionUser(r.CreatedByUserID),
					r.CreatedAt.Format(time.RFC3339),
				})
			}
			output.Print(*opts, columns, rows, raw)
			return nil
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "ID of the "+spec.noun)
	cmd.MarkFlagRequired("id")
	return cmd
}

func newTemplateRevisionsShowCmd(c *client.Client, opts *output.Options, spec templateRevisionsSpec) *cobra.Command {
	var id uint
	help := spec.help("_show")
	cmd := &cobra.Command{
		Use:         "show <number>",
		Short:       "Show one revision with its stored content",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseRevisionNumber(args[0])
			if err != nil {
				return err
			}
			q, err := revisionQuery(id)
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := c.Get(fmt.Sprintf("%s/revisions/%d", spec.endpoint, n), q, &raw); err != nil {
				return err
			}
			var d templateRevisionDetail
			if err := json.Unmarshal(raw, &d); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			restoredFrom := ""
			if d.RestoredFrom != nil {
				restoredFrom = "#" + strconv.FormatUint(uint64(*d.RestoredFrom), 10)
			}
			output.PrintSingle(*opts, []output.KeyValue{
				{Key: "Number", Value: strconv.FormatUint(uint64(d.Number), 10)},
				{Key: "ID", Value: strconv.FormatUint(uint64(d.ID), 10)},
				{Key: "Source", Value: d.Source},
				{Key: "Label", Value: d.Label},
				{Key: "User", Value: formatRevisionUser(d.CreatedByUserID)},
				{Key: "Created", Value: d.CreatedAt.Format(time.RFC3339)},
				{Key: "Applied", Value: strings.Join(d.Applied, ", ")},
				{Key: "ChangedSlots", Value: strings.Join(d.ChangedSlots, ", ")},
				{Key: "RestoredFrom", Value: restoredFrom},
			}, raw)
			if !opts.JSON {
				printRevisionSnapshot(d.Snapshot)
			}
			return nil
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "ID of the "+spec.noun)
	cmd.MarkFlagRequired("id")
	return cmd
}

// printRevisionSnapshot prints each non-empty stored field under a header line.
func printRevisionSnapshot(snapshot map[string]any) {
	names := make([]string, 0, len(snapshot))
	for name := range snapshot {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		var text string
		switch v := snapshot[name].(type) {
		case string:
			text = v
		case bool:
			if !v {
				continue
			}
			text = "true"
		default:
			text = fmt.Sprint(v)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		fmt.Printf("\n--- %s ---\n%s\n", name, strings.TrimRight(text, "\n"))
	}
}

func newTemplateRevisionsDiffCmd(c *client.Client, opts *output.Options, spec templateRevisionsSpec) *cobra.Command {
	var id, from, to uint
	help := spec.help("_diff")
	cmd := &cobra.Command{
		Use:         "diff",
		Short:       "Diff two revisions, or a revision against now",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, err := revisionQuery(id)
			if err != nil {
				return err
			}
			if from == 0 {
				return fmt.Errorf("--from must be a positive revision number")
			}
			q.Set("from", strconv.FormatUint(uint64(from), 10))
			if cmd.Flags().Changed("to") {
				if to == 0 {
					return fmt.Errorf("--to must be a positive revision number")
				}
				q.Set("to", strconv.FormatUint(uint64(to), 10))
			}
			var raw json.RawMessage
			if err := c.Get(spec.endpoint+"/revisions/diff", q, &raw); err != nil {
				return err
			}
			if opts.JSON {
				output.PrintRawJSON(raw)
				return nil
			}
			var d templateRevisionDiffResponse
			if err := json.Unmarshal(raw, &d); err != nil {
				return fmt.Errorf("parsing response: %w", err)
			}
			right := "current"
			if d.To != nil {
				right = "#" + strconv.FormatUint(uint64(*d.To), 10)
			}
			left := "#" + strconv.FormatUint(uint64(d.From), 10)
			if len(d.Slots) == 0 {
				output.PrintMessage("No differences between " + left + " and " + right + ".")
				return nil
			}
			for _, slot := range d.Slots {
				for _, f := range slot.Fields {
					fmt.Printf("--- %s (%s)\n+++ %s (%s)\n", f.Field, left, f.Field, right)
					hunks := make([]diffHunk, len(f.Hunks))
					for i, h := range f.Hunks {
						hunks[i] = diffHunk{op: h.Op, lines: h.Lines}
					}
					fmt.Print(renderUnifiedDiff(hunks, 3))
				}
			}
			return nil
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "ID of the "+spec.noun)
	cmd.MarkFlagRequired("id")
	cmd.Flags().UintVar(&from, "from", 0, "Revision number for the left side")
	cmd.MarkFlagRequired("from")
	cmd.Flags().UintVar(&to, "to", 0, "Revision number for the right side (default: the current state)")
	return cmd
}

type diffHunk struct {
	op    string
	lines []string
}

// renderUnifiedDiff turns the server's hunks into unified-diff text with
// `context` equal lines kept around each change and "@@ -a,b +c,d @@" headers.
func renderUnifiedDiff(hunks []diffHunk, context int) string {
	type line struct {
		op   byte
		text string
	}
	var lines []line
	for _, h := range hunks {
		var op byte
		switch h.op {
		case "insert":
			op = '+'
		case "delete":
			op = '-'
		default:
			op = ' '
		}
		for _, t := range h.lines {
			lines = append(lines, line{op, t})
		}
	}
	keep := make([]bool, len(lines))
	for i, l := range lines {
		if l.op == ' ' {
			continue
		}
		for j := max(0, i-context); j <= min(len(lines)-1, i+context); j++ {
			keep[j] = true
		}
	}
	var b strings.Builder
	oldNo, newNo := 1, 1
	for i := 0; i < len(lines); {
		if !keep[i] {
			if lines[i].op != '+' {
				oldNo++
			}
			if lines[i].op != '-' {
				newNo++
			}
			i++
			continue
		}
		j, oldN, newN := i, 0, 0
		for ; j < len(lines) && keep[j]; j++ {
			if lines[j].op != '+' {
				oldN++
			}
			if lines[j].op != '-' {
				newN++
			}
		}
		// Unified diff numbers an empty range by the line before it.
		oldStart, newStart := oldNo, newNo
		if oldN == 0 {
			oldStart--
		}
		if newN == 0 {
			newStart--
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, oldN, newStart, newN)
		for k := i; k < j; k++ {
			b.WriteByte(lines[k].op)
			b.WriteString(lines[k].text)
			b.WriteByte('\n')
		}
		oldNo += oldN
		newNo += newN
		i = j
	}
	return b.String()
}

func newTemplateRevisionsRestoreCmd(c *client.Client, opts *output.Options, spec templateRevisionsSpec) *cobra.Command {
	var id uint
	var slot string
	var noCSS bool
	help := spec.help("_restore")
	cmd := &cobra.Command{
		Use:         "restore <number>",
		Short:       "Restore a revision, or one slot of it",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseRevisionNumber(args[0])
			if err != nil {
				return err
			}
			q, err := revisionQuery(id)
			if err != nil {
				return err
			}
			if noCSS && slot == "" {
				return fmt.Errorf("--no-css requires --slot")
			}
			body := map[string]any{}
			if slot != "" {
				body["slot"] = slot
				body["includeCss"] = !noCSS
			}
			var raw json.RawMessage
			if err := c.Post(fmt.Sprintf("%s/revisions/%d/restore", spec.endpoint, n), q, body, &raw); err != nil {
				return err
			}
			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
				return nil
			}
			var s templateRevisionSummary
			if err := json.Unmarshal(raw, &s); err != nil {
				output.PrintMessage("Restored revision " + args[0] + ".")
				return nil
			}
			what := "revision"
			if slot != "" {
				what = "slot " + slot + " of revision"
			}
			output.PrintMessage(fmt.Sprintf("Restored %s #%d as new revision #%d.", what, n, s.Number))
			return nil
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "ID of the "+spec.noun)
	cmd.MarkFlagRequired("id")
	cmd.Flags().StringVar(&slot, "slot", "", "Restore only this slot, for example CustomHeader (default: the whole revision)")
	cmd.Flags().BoolVar(&noCSS, "no-css", false, "With --slot, leave the slot's CSS as it is now")
	return cmd
}

func newTemplateRevisionsLabelCmd(c *client.Client, opts *output.Options, spec templateRevisionsSpec) *cobra.Command {
	var id uint
	var clear bool
	help := spec.help("_label")
	cmd := &cobra.Command{
		Use:         "label <number> [text]",
		Short:       "Set, change or clear a revision's label",
		Long:        help.Long,
		Example:     help.Example,
		Annotations: help.Annotations,
		Args:        cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			n, err := parseRevisionNumber(args[0])
			if err != nil {
				return err
			}
			q, err := revisionQuery(id)
			if err != nil {
				return err
			}
			if clear == (len(args) == 2) {
				return fmt.Errorf("pass either a label text or --clear")
			}
			label := ""
			if !clear {
				label = args[1]
			}
			var raw json.RawMessage
			if err := c.Post(fmt.Sprintf("%s/revisions/%d/label", spec.endpoint, n), q, map[string]string{"label": label}, &raw); err != nil {
				return err
			}
			if opts.JSON {
				output.PrintSingle(*opts, nil, raw)
			} else if clear {
				output.PrintMessage(fmt.Sprintf("Cleared the label of revision #%d.", n))
			} else {
				output.PrintMessage(fmt.Sprintf("Labelled revision #%d.", n))
			}
			return nil
		},
	}
	cmd.Flags().UintVar(&id, "id", 0, "ID of the "+spec.noun)
	cmd.MarkFlagRequired("id")
	cmd.Flags().BoolVar(&clear, "clear", false, "Remove the label instead of setting one")
	return cmd
}
