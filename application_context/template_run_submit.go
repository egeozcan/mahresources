package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"mahresources/shortcodes"
)

// Submissions of a harness run (plan sections 2 and 3, "Submit"): the one
// validator, the submit Tool's argument contract, and the resolved value of a
// submitted field.

// ValidateTemplateRunField is the harness's single validator. lint, submit and
// the final Draft all call it, so a missing [partial] is reported the same way
// from each. It is validateTemplateContent's options plus a partial-existence
// check under ctx. A lookup that fails or is cancelled counts as existing for
// the linter and adds one warning, so a cancelled check never reports an
// existing partial as missing.
func ValidateTemplateRunField(ctx context.Context, in TemplateGenerationInput, field, content string) []shortcodes.LintIssue {
	if field == "MetaSchema" {
		return validateMetaSchemaJSON(content, shortcodes.MetaSchemaLintOptions{
			DisplayTypes:          in.DisplayTypes,
			DisplayTypesTruncated: in.DisplayTypesTruncated,
		})
	}
	opts := shortcodes.LintOptions{
		Known:        in.Known,
		ValidateMRQL: in.ValidateMRQL,
		CSSMode:      strings.HasSuffix(field, "CSS"),
	}
	unchecked := map[string]bool{}
	if in.PartialExists != nil {
		opts.PartialExists = func(name string) bool {
			exists, err := in.PartialExists(ctx, name)
			if err != nil || ctx.Err() != nil {
				unchecked[name] = true
				return true
			}
			return exists
		}
	}
	issues := shortcodes.Lint(content, opts)
	if issues == nil {
		issues = []shortcodes.LintIssue{}
	}
	names := make([]string, 0, len(unchecked))
	for name := range unchecked {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		issues = append(issues, shortcodes.LintIssue{
			Severity: shortcodes.SeverityWarning,
			Message:  fmt.Sprintf("could not check that partial %s exists", name),
		})
	}
	return issues
}

// templateRunFieldsOf returns the fields a harness request asks for, in
// request order, whether every one must be submitted, and whether CustomCSS is
// additions to the editor's stylesheet.
//
// Required is computed here and not read from SlotsSelected alone: a
// standalone slot, a MetaSchema, a cluster and a legacy pair (the handler
// rewrites clusters into two-slot bundles without setting SlotsSelected) all
// have required fields. Only a whole-template request without a selection
// offers optional candidates, which is classic fan-out's meaning.
func templateRunFieldsOf(in TemplateGenerationInput) (fields []string, required, cssAdditions bool) {
	switch in.Target {
	case TemplateTargetMetaSchema:
		return []string{"MetaSchema"}, true, false
	case TemplateTargetBundle:
		fan, _ := templateFanOutFor(in)
		fields = append(fields, in.BundleSlots...)
		if fan == nil {
			return fields, true, false
		}
		return fields, in.SlotsSelected, fan.css
	default:
		return []string{in.Slot}, true, false
	}
}

// templateRunSubmissions holds what the model has committed. It is not safe for
// concurrent use: the loop commits between Turns, after every call has ended.
type templateRunSubmissions struct {
	in           TemplateGenerationInput
	cssAdditions bool
	seedCSS      string
	// raw is the trimmed submission as validated (for a whole-template
	// CustomCSS, the additions); order is the commit order of first submission.
	raw   map[string]string
	order []string
}

func newTemplateRunSubmissions(in TemplateGenerationInput, cssAdditions bool) *templateRunSubmissions {
	s := &templateRunSubmissions{in: in, cssAdditions: cssAdditions, raw: map[string]string{}}
	if cssAdditions {
		s.seedCSS = currentTemplateSlots(in)[templateFanOutCustomCSS]
	}
	return s
}

func (s *templateRunSubmissions) commit(field, value string) {
	if _, ok := s.raw[field]; !ok {
		s.order = append(s.order, field)
	}
	s.raw[field] = value
}

func (s *templateRunSubmissions) any() bool { return len(s.raw) > 0 }

// resolved is the value Apply would receive for a submitted field.
func (s *templateRunSubmissions) resolved(field string) string {
	if s.cssAdditions && field == templateFanOutCustomCSS {
		return templateRunCombinedCSS(s.seedCSS, s.raw[field])
	}
	return s.raw[field]
}

// snapshot is the Turn-start map of resolved values for the Tools.
func (s *templateRunSubmissions) snapshot() map[string]string {
	out := make(map[string]string, len(s.raw))
	for field := range s.raw {
		out[field] = s.resolved(field)
	}
	return out
}

// templateRunCombinedCSS joins the Seed's stylesheet and the additions once,
// with appendSharedCSS's separator rule. Empty additions leave the stylesheet
// byte for byte as it was.
func templateRunCombinedCSS(seed, additions string) string {
	if additions == "" {
		return seed
	}
	if seed == "" {
		return additions
	}
	separator := "\n\n"
	if strings.HasSuffix(seed, "\n\n") {
		separator = ""
	} else if strings.HasSuffix(seed, "\n") {
		separator = "\n"
	}
	return seed + separator + additions
}

// validateSubmission lints one submitted value and reports the oversize rule.
// For a whole-template CustomCSS the additions are linted and the combined
// size is what is checked; issue positions are shifted into the combined text,
// as appendSharedCSS shifts them.
func (s *templateRunSubmissions) validateSubmission(ctx context.Context, field, value string) []shortcodes.LintIssue {
	var issues []shortcodes.LintIssue
	resolved := value
	offset := 0
	if s.cssAdditions && field == templateFanOutCustomCSS {
		resolved = templateRunCombinedCSS(s.seedCSS, value)
		offset = len(resolved) - len(value)
	}
	if len(resolved) > MaxTemplateGeneratedContentLength {
		issues = append(issues, templateOversizeIssue(field, len(resolved)))
	}
	for _, issue := range ValidateTemplateRunField(ctx, s.in, field, value) {
		if issue.End > issue.Start {
			issue.Start += offset
			issue.End += offset
		}
		issues = append(issues, issue)
	}
	return issues
}

// Source limits of one submitted field (plan section 2, "Preview and lint work
// is bounded"): bytes and shortcode tokens, checked before any parsing.
const maxTemplateRunSourceTokens = 2000

// admitTemplateRunSource is the one place a model-written source is admitted
// before validation. Integration points it at the shared helper of lanes B1
// and B2 (CheckTemplateToolSource). A shortcode token starts with "[", so
// counting those over-counts and never under-counts.
var admitTemplateRunSource = func(content string) error {
	if len(content) > MaxTemplateGeneratedContentLength {
		return fmt.Errorf("source is %d bytes; the limit is %d", len(content), MaxTemplateGeneratedContentLength)
	}
	if n := strings.Count(content, "["); n > maxTemplateRunSourceTokens {
		return fmt.Errorf("source has %d shortcode tokens; the limit is %d", n, maxTemplateRunSourceTokens)
	}
	return nil
}

// submitErrorText formats an argument error for the model: model-chosen names
// are clipped, and the whole text is capped and fenced (which defangs markers).
func submitErrorText(msg string) string {
	return strings.Join(referenceBlock("submit result", capToolText(msg, 1024)), "\n")
}

// clipName shortens a model-chosen field name for an error message.
func clipName(name string) string { return strings.ToValidUTF8(clipBytes(name, 64), "") }

// submitArgs is the decoded argument object of a submit call.
type submitArgs struct {
	fields      map[string]string
	explanation string
	done        bool
}

// parseSubmitArgs decodes a submit call. The error text goes to the model.
func parseSubmitArgs(raw string, requested map[string]bool, order []string) (submitArgs, error) {
	var decoded struct {
		Fields      map[string]json.RawMessage `json:"fields"`
		Explanation *string                    `json:"explanation"`
		Done        *bool                      `json:"done"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return submitArgs{}, fmt.Errorf("submit arguments must be a JSON object with a fields map")
	}
	if len(decoded.Fields) == 0 {
		return submitArgs{}, fmt.Errorf("submit needs a non-empty fields map of field name to content")
	}
	out := submitArgs{fields: make(map[string]string, len(decoded.Fields))}
	var outside, notStrings []string
	for name, value := range decoded.Fields {
		if !requested[name] {
			outside = append(outside, name)
			continue
		}
		// A JSON null is not an empty submission: only a string clears a field.
		var text *string
		if err := json.Unmarshal(value, &text); err != nil || text == nil {
			notStrings = append(notStrings, name)
			continue
		}
		if err := admitTemplateRunSource(*text); err != nil {
			return submitArgs{}, fmt.Errorf("not submitted: %s: %v", clipName(name), err)
		}
		out.fields[name] = strings.TrimSpace(*text)
	}
	sort.Strings(outside)
	sort.Strings(notStrings)
	outside, notStrings = clipNames(outside), clipNames(notStrings)
	if len(outside) > 0 {
		return submitArgs{}, fmt.Errorf("not submitted: %s not requested in this run; the requested fields are %s", strings.Join(outside, ", ")+" "+pluralWas(len(outside)), strings.Join(order, ", "))
	}
	if len(notStrings) > 0 {
		return submitArgs{}, fmt.Errorf("not submitted: the value of %s must be a string", strings.Join(notStrings, ", "))
	}
	if decoded.Explanation != nil {
		out.explanation = strings.TrimSpace(*decoded.Explanation)
	}
	out.done = decoded.Done != nil && *decoded.Done
	return out, nil
}

// clipNames clips at most five names, noting the rest.
func clipNames(names []string) []string {
	if len(names) > 5 {
		names = append(names[:5:5], fmt.Sprintf("and %d more", len(names)-5))
	}
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = clipName(n)
	}
	return out
}

func pluralWas(n int) string {
	if n == 1 {
		return "was"
	}
	return "were"
}

// capToolText cuts text to at most limit bytes at a UTF-8 boundary and says so.
func capToolText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return fmt.Sprintf("%s\n[cut: %d of %d bytes shown]", text[:cut], cut, len(text))
}

// submitResultText renders the lint result of one submit call, fenced and
// capped like every other Tool result.
func submitResultText(fields []string, issuesByField map[string][]shortcodes.LintIssue, explanation string) string {
	type fieldResult struct {
		Errors   int                    `json:"errors"`
		Warnings int                    `json:"warnings"`
		Issues   []shortcodes.LintIssue `json:"issues,omitempty"`
	}
	result := struct {
		Submitted []string               `json:"submitted"`
		Fields    map[string]fieldResult `json:"fields"`
		Note      string                 `json:"note,omitempty"`
	}{Submitted: fields, Fields: map[string]fieldResult{}}
	for _, field := range fields {
		r := fieldResult{Issues: issuesByField[field]}
		for _, issue := range r.Issues {
			switch issue.Severity {
			case shortcodes.SeverityError:
				r.Errors++
			case shortcodes.SeverityWarning:
				r.Warnings++
			}
		}
		result.Fields[field] = r
	}
	if explanation == "" {
		result.Note = "No explanation was sent; add one with the explanation argument."
	}
	body, _ := json.Marshal(result)
	text := strings.Join(referenceBlock("submit result", capToolText(string(body), ToolCapSubmitBytes)), "\n")
	return text
}

func countIssues(issues []shortcodes.LintIssue) (errors, warnings int) {
	for _, issue := range issues {
		switch issue.Severity {
		case shortcodes.SeverityError:
			errors++
		case shortcodes.SeverityWarning:
			warnings++
		}
	}
	return
}
