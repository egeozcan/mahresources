package contracts

// ImportApplyReportOutcome is the verified, viewer-authorized outcome paired
// with an import report. Known is false when its producing Apply cannot be
// verified for the current reader.
type ImportApplyReportOutcome struct {
	State          string
	FailureMessage string
	Known          bool
}
