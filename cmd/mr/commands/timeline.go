package commands

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"strings"
	"time"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// ---------------------------------------------------------------------------
// Response types matching the timeline API JSON shape
// ---------------------------------------------------------------------------

type timelineBucket struct {
	Label   string    `json:"label"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Created int64     `json:"created"`
	Updated int64     `json:"updated"`
}

type timelineHasMore struct {
	Left  bool `json:"left"`
	Right bool `json:"right"`
}

type timelineResponse struct {
	Buckets []timelineBucket `json:"buckets"`
	HasMore timelineHasMore  `json:"hasMore"`
}

// ---------------------------------------------------------------------------
// Shared flag helpers
// ---------------------------------------------------------------------------

type timelineFlags struct {
	granularity string
	anchor      string
	columns     int
}

// addTimelineFlags registers the common --granularity, --anchor, and --columns
// flags on a cobra command.
func addTimelineFlags(cmd *cobra.Command, flags *timelineFlags) {
	cmd.Flags().StringVar(&flags.granularity, "granularity", "monthly", "Bucket granularity: yearly, monthly, or weekly")
	cmd.Flags().StringVar(&flags.anchor, "anchor", "", "Anchor date (YYYY-MM-DD); defaults to today")
	cmd.Flags().IntVar(&flags.columns, "columns", 15, "Number of timeline buckets (max 60)")
}

// buildTimelineQuery merges timeline-specific params with entity-specific
// filter params and returns the combined url.Values.
func buildTimelineQuery(flags *timelineFlags, extraParams url.Values) url.Values {
	q := url.Values{}
	// Copy entity-specific params first
	for k, vs := range extraParams {
		for _, v := range vs {
			q.Add(k, v)
		}
	}
	if flags.granularity != "" {
		q.Set("granularity", flags.granularity)
	}
	if flags.anchor != "" {
		q.Set("anchor", flags.anchor)
	}
	if flags.columns > 0 {
		q.Set("columns", fmt.Sprintf("%d", flags.columns))
	}
	return q
}

// fetchAndPrintTimeline fetches the timeline data from the API and prints it.
func fetchAndPrintTimeline(c *client.Client, opts output.Options, apiPath string, q url.Values) error {
	var raw json.RawMessage
	if err := c.Get(apiPath, q, &raw); err != nil {
		return err
	}

	if opts.JSON {
		output.PrintRawJSON(raw)
		return nil
	}

	var resp timelineResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("parsing response: %w", err)
	}

	if len(resp.Buckets) == 0 {
		output.PrintMessage("No timeline data available.")
		return nil
	}

	printASCIIChart(resp)
	return nil
}

// ---------------------------------------------------------------------------
// ASCII chart rendering
// ---------------------------------------------------------------------------

const maxBarWidth = 40

func printASCIIChart(resp timelineResponse) {
	// Find the maximum count to scale bars
	var maxCount int64
	for _, b := range resp.Buckets {
		if b.Created > maxCount {
			maxCount = b.Created
		}
		if b.Updated > maxCount {
			maxCount = b.Updated
		}
	}

	if maxCount == 0 {
		output.PrintMessage("No activity in the selected time range.")
		return
	}

	// Determine label width for alignment
	labelWidth := 0
	for _, b := range resp.Buckets {
		if len(b.Label) > labelWidth {
			labelWidth = len(b.Label)
		}
	}

	// Print each bucket
	for _, b := range resp.Buckets {
		createdWidth := int(math.Round(float64(b.Created) / float64(maxCount) * maxBarWidth))
		updatedWidth := int(math.Round(float64(b.Updated) / float64(maxCount) * maxBarWidth))

		createdBar := strings.Repeat("\u2588", createdWidth) // █
		updatedBar := strings.Repeat("\u2593", updatedWidth) // ▓

		label := fmt.Sprintf("%-*s", labelWidth, b.Label)
		fmt.Printf("%s  %s %d\n", label, createdBar, b.Created)
		fmt.Printf("%s  %s %d\n", strings.Repeat(" ", labelWidth), updatedBar, b.Updated)
	}

	// Legend
	fmt.Println()
	fmt.Println("\u2588 Created  \u2593 Updated")

	// HasMore indicators
	var indicators []string
	if resp.HasMore.Left {
		indicators = append(indicators, "<< more")
	}
	if resp.HasMore.Right {
		indicators = append(indicators, "more >>")
	}
	if len(indicators) > 0 {
		fmt.Println(strings.Join(indicators, "  "))
	}
}
