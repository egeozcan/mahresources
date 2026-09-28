package template_context_providers

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"mahresources/jobs"
)

func TestJobRowStatsFollowsTheJobsState(t *testing.T) {
	// The amount leads the line as the drawer's does; the speed follows the state.
	now := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	completed, total := int64(1<<20), int64(3<<20)
	rate := float64(512 << 10)
	start, end := 0.0, float64(1<<20)
	series := jobs.ProgressSeries{
		Unit: "bytes", Rate: &rate,
		Anchor: &jobs.RateAnchor{At: now.UnixMilli(), Completed: end},
		Points: []jobs.SeriesPoint{{At: now.Add(-4 * time.Second).UnixMilli(), Completed: &start}, {At: now.UnixMilli(), Completed: &end}},
	}
	running := jobs.Snapshot{
		State:          jobs.StateRunning,
		Progress:       jobs.Progress{Completed: &completed, Total: &total, Unit: "bytes"},
		ProgressSeries: series,
	}
	if got := jobRowStats(running, now); got != "1.0 MB of 3.0 MB · 512 KB/s · about 4 s left" {
		t.Fatalf("running stats = %q", got)
	}

	done := running
	done.State = jobs.StateSucceeded
	if got := jobRowStats(done, now); got != "1.0 MB of 3.0 MB · average 256 KB/s" {
		t.Fatalf("finished stats = %q", got)
	}

	percent := running
	percent.Progress.Unit = "percent"
	percent.ProgressSeries.Unit = "percent"
	if got := jobRowStats(percent, now); got != "about 4 s left" {
		t.Fatalf("percent stats = %q; want no speed in percent", got)
	}
}

// jobProgressFormatCases is the table jobProgress.test.ts reads too: the /jobs
// card and the browser surfaces format one Job's figures alike, or a reader sees
// "8051532 / 20971520 bytes" on one page and "7.6 MB of 20.0 MB" on the next.
type jobProgressFormatCases struct {
	Amounts []struct {
		Progress struct {
			Completed *int64 `json:"completed"`
			Total     *int64 `json:"total"`
			Unit      string `json:"unit"`
		} `json:"progress"`
		Finished bool   `json:"finished"`
		Text     string `json:"text"`
	} `json:"amounts"`
	Rates []struct {
		Rate float64 `json:"rate"`
		Unit string  `json:"unit"`
		Text string  `json:"text"`
	} `json:"rates"`
	Etas []struct {
		Seconds   float64 `json:"seconds"`
		Estimated bool    `json:"estimated"`
		Text      string  `json:"text"`
	} `json:"etas"`
}

func TestJobProgressFormatsAsTheBrowserDoes(t *testing.T) {
	raw, err := os.ReadFile("testdata/job_progress_format.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases jobProgressFormatCases
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases.Amounts {
		progress := jobs.Progress{Completed: c.Progress.Completed, Total: c.Progress.Total, Unit: c.Progress.Unit}
		if got := formatJobAmount(progress, c.Finished); got != c.Text {
			t.Errorf("amount %+v finished=%v = %q; want %q", c.Progress, c.Finished, got, c.Text)
		}
	}
	for _, c := range cases.Rates {
		rate := c.Rate
		if got := formatJobRate(&rate, c.Unit); got != c.Text {
			t.Errorf("rate %v %q = %q; want %q", c.Rate, c.Unit, got, c.Text)
		}
	}
	for _, c := range cases.Etas {
		left := time.Duration(c.Seconds * float64(time.Second))
		if got := formatJobEta(left, c.Estimated); got != c.Text {
			t.Errorf("eta %vs estimated=%v = %q; want %q", c.Seconds, c.Estimated, got, c.Text)
		}
	}
}
