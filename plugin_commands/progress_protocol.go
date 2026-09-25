package plugin_commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"mahresources/models/jobmetrics"
)

// ProgressLinePrefix starts a structured progress report on a command's
// stdout. The rest of the line is one JSON object in the shape the Lua
// mah.job_progress table takes:
//
//	::mah-progress {"completed":12,"total":40,"unit":"items","message":"Encoding",
//	                "metrics":[{"key":"fps","label":"Frames/s","value":48,"graph":true}]}
//
// A recognized line is consumed: it never reaches the retained output tail,
// where a stream of them would push out the output a person debugging the run
// actually wants. A line that starts with the prefix but does not parse is left
// in the tail untouched, which is where its author will look for it.
const ProgressLinePrefix = "::mah-progress "

// maxProgressLineBytes bounds one buffered line. A longer line is passed
// through as ordinary output without being inspected, so a command that never
// prints a newline cannot make the runner hold an unbounded buffer.
const maxProgressLineBytes = 8 << 10

// ProgressReport is one report parsed from a command's stdout. Fields the line
// omits keep what the previous report said, exactly as in the Lua table form;
// Metrics, when present, replaces the whole set.
type ProgressReport struct {
	Percent   *float64             `json:"percent,omitempty"`
	Completed *int64               `json:"completed,omitempty"`
	Total     *int64               `json:"total,omitempty"`
	Unit      *string              `json:"unit,omitempty"`
	Message   *string              `json:"message,omitempty"`
	Metrics   *[]jobmetrics.Metric `json:"metrics,omitempty"`
}

// parseProgressLine decodes and validates one report. Unknown fields are
// refused so a misspelled key is visible in the tail rather than silently
// ignored.
func parseProgressLine(payload []byte) (ProgressReport, error) {
	var report ProgressReport
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&report); err != nil {
		return ProgressReport{}, err
	}
	if decoder.More() {
		return ProgressReport{}, fmt.Errorf("more than one JSON value")
	}
	if report.Percent != nil && (math.IsNaN(*report.Percent) || *report.Percent < 0 || *report.Percent > 100) {
		return ProgressReport{}, fmt.Errorf("percent must be between 0 and 100")
	}
	if report.Completed != nil && *report.Completed < 0 {
		return ProgressReport{}, fmt.Errorf("completed must be at least zero")
	}
	if report.Total != nil && *report.Total < 0 {
		return ProgressReport{}, fmt.Errorf("total must be at least zero")
	}
	if report.Unit != nil && len(*report.Unit) > jobmetrics.MaxUnitBytes {
		return ProgressReport{}, fmt.Errorf("unit is over the %d-byte ceiling", jobmetrics.MaxUnitBytes)
	}
	if report.Metrics != nil {
		for i := range *report.Metrics {
			if (*report.Metrics)[i].Label == "" {
				(*report.Metrics)[i].Label = (*report.Metrics)[i].Key
			}
		}
		if err := jobmetrics.Validate(*report.Metrics); err != nil {
			return ProgressReport{}, err
		}
	}
	return report, nil
}

// progressLineFilter sits between a command's stdout and its output tail. It
// forwards everything except recognized progress lines, which it parses,
// redacts and hands to report.
//
// It is written to by exactly one goroutine, the stdout drain, and flushed by
// that goroutine once the pipe closes.
type progressLineFilter struct {
	out      io.Writer
	report   func(ProgressReport)
	redact   func(string) string
	line     []byte
	overflow bool
}

func newProgressLineFilter(out io.Writer, report func(ProgressReport), redact func(string) string) *progressLineFilter {
	return &progressLineFilter{out: out, report: report, redact: redact, line: make([]byte, 0, 256)}
}

func (f *progressLineFilter) Write(p []byte) (int, error) {
	written := len(p)
	for len(p) > 0 {
		newline := bytes.IndexByte(p, '\n')
		if f.overflow {
			// Inside a line already judged too long: pass it through until it
			// ends, then go back to inspecting.
			if newline < 0 {
				if _, err := f.out.Write(p); err != nil {
					return 0, err
				}
				return written, nil
			}
			if _, err := f.out.Write(p[:newline+1]); err != nil {
				return 0, err
			}
			f.overflow = false
			p = p[newline+1:]
			continue
		}
		if newline < 0 {
			if len(f.line)+len(p) > maxProgressLineBytes {
				if err := f.passThrough(p); err != nil {
					return 0, err
				}
				f.overflow = true
				return written, nil
			}
			f.line = append(f.line, p...)
			return written, nil
		}
		if len(f.line)+newline > maxProgressLineBytes {
			if err := f.passThrough(p[:newline+1]); err != nil {
				return 0, err
			}
		} else {
			f.line = append(f.line, p[:newline+1]...)
			if err := f.endLine(); err != nil {
				return 0, err
			}
		}
		p = p[newline+1:]
	}
	return written, nil
}

// Flush ends a final line that had no newline.
func (f *progressLineFilter) Flush() error {
	if f.overflow || len(f.line) == 0 {
		f.overflow = false
		return nil
	}
	return f.endLine()
}

func (f *progressLineFilter) passThrough(tail []byte) error {
	if len(f.line) > 0 {
		if _, err := f.out.Write(f.line); err != nil {
			return err
		}
		f.line = f.line[:0]
	}
	_, err := f.out.Write(tail)
	return err
}

func (f *progressLineFilter) endLine() error {
	line := f.line
	f.line = f.line[:0]
	content := bytes.TrimRight(line, "\r\n")
	if payload, ok := bytes.CutPrefix(content, []byte(ProgressLinePrefix)); ok && f.report != nil {
		if report, err := parseProgressLine(payload); err == nil {
			f.report(f.redactReport(report))
			return nil
		}
	}
	_, err := f.out.Write(line)
	return err
}

// redactReport applies the run's output redaction to every text field, since a
// progress report is persisted on the Job just as the tail is. Control
// sequences are stripped first, exactly as the tail strips them before it
// redacts: JSON can carry an escape character, and a secret split by one would
// otherwise slip past an exact-value match and still read whole on the page.
func (f *progressLineFilter) redactReport(report ProgressReport) ProgressReport {
	clean := func(value string) string {
		value = stripProgressControls(value)
		if f.redact != nil {
			value = f.redact(value)
		}
		return value
	}
	text := func(value *string) *string {
		if value == nil {
			return nil
		}
		cleaned := clean(*value)
		return &cleaned
	}
	report.Unit = text(report.Unit)
	report.Message = text(report.Message)
	if report.Metrics != nil {
		metrics := jobmetrics.Clone(*report.Metrics)
		for i := range metrics {
			metrics[i].Label = clean(metrics[i].Label)
			metrics[i].Unit = clean(metrics[i].Unit)
			if clean(metrics[i].Key) != metrics[i].Key {
				metrics[i].Key = fmt.Sprintf("metric-%d", i+1)
			}
		}
		report.Metrics = &metrics
	}
	return report
}

// stripProgressControls removes what outputTail removes from captured output
// (ESC-introduced CSI and OSC sequences, and every other C0 control and DEL),
// turns a newline or tab into a space, since a label or message is one line,
// and removes the C1 controls, which a JSON string can spell as single runes.
func stripProgressControls(value string) string {
	var out strings.Builder
	out.Grow(len(value))
	escape, csi, osc, oscEscape := false, false, false, false
	for i := 0; i < len(value); i++ {
		b := value[i]
		switch {
		case osc:
			if b == 0x07 || (oscEscape && b == '\\') {
				osc, oscEscape = false, false
				continue
			}
			oscEscape = b == 0x1b
			continue
		case csi:
			if b >= 0x40 && b <= 0x7e {
				csi = false
			}
			continue
		case escape:
			escape = false
			switch b {
			case '[':
				csi = true
			case ']':
				osc = true
			}
			continue
		case b == 0x1b:
			escape = true
			continue
		case b == '\n' || b == '\t':
			out.WriteByte(' ')
			continue
		case b < 0x20 || b == 0x7f:
			continue
		}
		out.WriteByte(b)
	}
	return strings.Map(func(r rune) rune {
		if r >= 0x80 && r <= 0x9f {
			return -1
		}
		return r
	}, out.String())
}
