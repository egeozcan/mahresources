package application_context

import (
	"testing"
	"time"

	"mahresources/jobs"
)

// A file output is saved under a name that says what it is, when it was made,
// which Job made it and what type of file it is: the label alone gave every
// export of every group the same name and no extension, so the saved file opened
// with nothing, and two exports published in one second still need two names.
func TestAJobOutputFileIsNamedByItsLabelTimeAndStoredExtension(t *testing.T) {
	published := time.Date(2026, 9, 26, 12, 42, 7, 0, time.FixedZone("CEST", 2*60*60))
	job := "01a0ddb2-7830-7abc-8def-0123456789ab"
	for _, testCase := range []struct {
		name  string
		jobID string
		label string
		path  string
		at    time.Time
		want  string
	}{
		{"a gzip group export", job, "Exported archive", "_exports/01a0.tar.gz", published, "exported-archive-20260926-104207-456789ab.tar.gz"},
		{"a plain group export", job, "Exported archive", "_exports/01a0.tar", published, "exported-archive-20260926-104207-456789ab.tar"},
		{"a CSV summary export", job, "Job summary export", "_exports/job-summaries/01a0.csv", published, "job-summary-export-20260926-104207-456789ab.csv"},
		{"a JSON summary export", job, "Job summary export", "_exports/job-summaries/01a0.json", published, "job-summary-export-20260926-104207-456789ab.json"},
		{"a label that already names the extension", job, "Import plan.json", "_imports/plan.json", published, "import-plan-20260926-104207-456789ab.json"},
		{"no label", job, "", "_exports/archive.tar", published, "archive-20260926-104207-456789ab.tar"},
		{"a label that tries to leave its directory", job, "../../etc/pass\x00wd", "_exports/a.tar", published, "etc-pass-wd-20260926-104207-456789ab.tar"},
		{"letters outside ASCII", job, "Dışa aktarım", "_exports/a.tar", published, "dışa-aktarım-20260926-104207-456789ab.tar"},
		{"no publication time", job, "Exported archive", "_exports/a.tar", time.Time{}, "exported-archive-456789ab.tar"},
		{"no extension", job, "Command log", "_logs/run", published, "command-log-20260926-104207-456789ab"},
		{"nothing to name it by but its Job", job, "", "", time.Time{}, "job-output-456789ab"},
		{"no Job", "", "Exported archive", "_exports/a.tar", published, "exported-archive-20260926-104207.tar"},
		{"nothing to name it by", "", "", "", time.Time{}, "job-output"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			output := jobs.Output{JobID: testCase.jobID, Label: testCase.label, CreatedAt: testCase.at}
			if got := jobOutputFilename(output, testCase.path); got != testCase.want {
				t.Fatalf("jobOutputFilename(%q, %q) = %q, want %q", testCase.label, testCase.path, got, testCase.want)
			}
		})
	}
}
