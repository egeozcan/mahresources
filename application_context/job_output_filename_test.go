package application_context

import (
	"testing"
	"time"

	"mahresources/jobs"
)

// A file output is saved under a name that says what it is, when it was made and
// what type of file it is: the label alone gave every export of every group the
// same name and no extension, so the saved file opened with nothing.
func TestAJobOutputFileIsNamedByItsLabelTimeAndStoredExtension(t *testing.T) {
	published := time.Date(2026, 9, 26, 12, 42, 7, 0, time.FixedZone("CEST", 2*60*60))
	for _, testCase := range []struct {
		name  string
		label string
		path  string
		at    time.Time
		want  string
	}{
		{"a gzip group export", "Exported archive", "_exports/01a0.tar.gz", published, "exported-archive-20260926-104207.tar.gz"},
		{"a plain group export", "Exported archive", "_exports/01a0.tar", published, "exported-archive-20260926-104207.tar"},
		{"a CSV summary export", "Job summary export", "_exports/job-summaries/01a0.csv", published, "job-summary-export-20260926-104207.csv"},
		{"a JSON summary export", "Job summary export", "_exports/job-summaries/01a0.json", published, "job-summary-export-20260926-104207.json"},
		{"a label that already names the extension", "Import plan.json", "_imports/plan.json", published, "import-plan-20260926-104207.json"},
		{"no label", "", "_exports/archive.tar", published, "archive-20260926-104207.tar"},
		{"a label that tries to leave its directory", "../../etc/pass\x00wd", "_exports/a.tar", published, "etc-pass-wd-20260926-104207.tar"},
		{"letters outside ASCII", "Dışa aktarım", "_exports/a.tar", published, "dışa-aktarım-20260926-104207.tar"},
		{"no publication time", "Exported archive", "_exports/a.tar", time.Time{}, "exported-archive.tar"},
		{"no extension", "Command log", "_logs/run", published, "command-log-20260926-104207"},
		{"nothing to name it by", "", "", time.Time{}, "job-output"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			output := jobs.Output{Label: testCase.label, CreatedAt: testCase.at}
			if got := jobOutputFilename(output, testCase.path); got != testCase.want {
				t.Fatalf("jobOutputFilename(%q, %q) = %q, want %q", testCase.label, testCase.path, got, testCase.want)
			}
		})
	}
}
