package plugin_commands

import (
	"bytes"
	"strings"
	"testing"
)

type progressFilterHarness struct {
	out     bytes.Buffer
	reports []ProgressReport
	filter  *progressLineFilter
}

func newProgressFilterHarness(secrets ...string) *progressFilterHarness {
	h := &progressFilterHarness{}
	raw := make([][]byte, 0, len(secrets))
	for _, secret := range secrets {
		raw = append(raw, []byte(secret))
	}
	h.filter = newProgressLineFilter(&h.out,
		func(report ProgressReport) { h.reports = append(h.reports, report) },
		func(value string) string { return redactProgressText(value, raw, false) })
	return h
}

func TestProgressFilterConsumesReportsAndKeepsOtherOutput(t *testing.T) {
	h := newProgressFilterHarness()
	input := "starting\n" +
		`::mah-progress {"completed":3,"total":10,"unit":"items","message":"Encoding"}` + "\n" +
		"frame 3 done\n" +
		`::mah-progress {"metrics":[{"key":"fps","label":"Frames/s","value":48,"graph":true}]}` + "\r\n" +
		"no newline at the end"
	// Deliver it a few bytes at a time: a pipe splits lines wherever it likes.
	for i := 0; i < len(input); i += 7 {
		end := min(i+7, len(input))
		if _, err := h.filter.Write([]byte(input[i:end])); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := h.filter.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if got := h.out.String(); got != "starting\nframe 3 done\nno newline at the end" {
		t.Fatalf("tail = %q; want the progress lines removed and everything else kept", got)
	}
	if len(h.reports) != 2 {
		t.Fatalf("reports = %+v; want two", h.reports)
	}
	first := h.reports[0]
	if first.Completed == nil || *first.Completed != 3 || *first.Total != 10 || *first.Unit != "items" || *first.Message != "Encoding" {
		t.Fatalf("first report = %+v", first)
	}
	second := h.reports[1]
	if second.Completed != nil || second.Metrics == nil || len(*second.Metrics) != 1 || !(*second.Metrics)[0].Graph {
		t.Fatalf("second report = %+v; want only its metrics set", second)
	}
}

func TestProgressFilterLeavesAnInvalidReportInTheTail(t *testing.T) {
	for _, line := range []string{
		`::mah-progress {"completed":-1}`,
		`::mah-progress {"complted":1}`,
		`::mah-progress not json`,
		`::mah-progress {"metrics":[{"key":"Bad Key","value":1}]}`,
		`::mah-progress {"percent":101}`,
		`::mah-progress {} {}`,
	} {
		h := newProgressFilterHarness()
		_, _ = h.filter.Write([]byte(line + "\n"))
		if len(h.reports) != 0 {
			t.Errorf("%s: reported %+v", line, h.reports)
		}
		if h.out.String() != line+"\n" {
			t.Errorf("%s: tail = %q; want the line kept for its author to find", line, h.out.String())
		}
	}
}

func TestProgressFilterPassesAnOverlongLineThroughUninspected(t *testing.T) {
	h := newProgressFilterHarness()
	long := ProgressLinePrefix + `{"message":"` + strings.Repeat("x", maxProgressLineBytes) + `"}`
	_, _ = h.filter.Write([]byte(long[:100]))
	_, _ = h.filter.Write([]byte(long[100:] + "\n" + `::mah-progress {"completed":1}` + "\n"))
	_ = h.filter.Flush()
	if len(h.reports) != 1 || *h.reports[0].Completed != 1 {
		t.Fatalf("reports = %+v; want only the short report after the long line", h.reports)
	}
	if h.out.String() != long+"\n" {
		t.Fatalf("tail kept %d bytes; want the long line whole and nothing else", h.out.Len())
	}
}

func TestProgressFilterRedactsReportText(t *testing.T) {
	h := newProgressFilterHarness("hunter2", "line\nbreak")
	_, _ = h.filter.Write([]byte(`::mah-progress {"message":"using hunter2 and line\nbreak","unit":"hunter2","metrics":[{"key":"hunter2","label":"x","value":1},{"key":"rows","label":"Rows for hunter2","value":2}]}` + "\n"))
	if len(h.reports) != 1 {
		t.Fatalf("reports = %+v", h.reports)
	}
	report := h.reports[0]
	// A metric whose key is the secret is left out; renaming it could collide
	// with another key or be the secret itself.
	if len(*report.Metrics) != 1 || (*report.Metrics)[0].Key != "rows" {
		t.Fatalf("metrics = %+v; want only the rows metric", *report.Metrics)
	}
	for _, text := range []string{*report.Message, *report.Unit, (*report.Metrics)[0].Label} {
		if strings.Contains(text, "hunter2") || strings.Contains(text, "line break") || strings.Contains(text, "line\nbreak") {
			t.Fatalf("report carried a secret: %q", text)
		}
		if strings.ContainsAny(text, "\n\t") {
			t.Fatalf("report text %q is not one line", text)
		}
	}
}

func TestProgressFilterRedactsASecretSplitByControlSequences(t *testing.T) {
	h := newProgressFilterHarness("hunter2")
	// JSON escapes put an ESC-introduced colour code, a C1 CSI and a NUL inside
	// the secret; the page would still show it whole.
	_, _ = h.filter.Write([]byte(`::mah-progress {"message":"key hun\u001b[31mter2 and hunt\u009ber2 and h\u0000unter2","metrics":[{"key":"k","label":"hun\u001b]0;x\u0007ter2","value":1}]}` + "\n"))
	if len(h.reports) != 1 {
		t.Fatalf("reports = %+v", h.reports)
	}
	message := *h.reports[0].Message
	label := (*h.reports[0].Metrics)[0].Label
	for _, text := range []string{message, label} {
		if strings.Contains(stripProgressControls(text), "hunter2") || strings.ContainsAny(text, "\x1b\x00\u009b") {
			t.Fatalf("report text %q still carries the secret or a control character", text)
		}
	}
}
