package jobview

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"mahresources/application_context"
	"mahresources/jobs"
)

// OutputURL is the canonical route that reauthorizes and opens one output.
func OutputURL(jobID, key string) string {
	return "/v1/jobs/" + url.PathEscape(jobID) + "/outputs?key=" + url.QueryEscape(key)
}

// SummaryDestinationURL restores the direct entity navigation that historical
// plugin-action summaries recorded as a result.redirect value. The target route
// performs normal authorization when opened; this only advertises a tightly
// constrained same-origin destination.
func SummaryDestinationURL(jobKind string, output jobs.Output) string {
	if jobKind != application_context.JobKindPluginAction || output.Key != "result" ||
		output.Type != jobs.OutputTypeSummary || output.Availability != jobs.OutputAvailable || !json.Valid(output.Reference) {
		return ""
	}
	var reference map[string]json.RawMessage
	if err := json.Unmarshal(output.Reference, &reference); err != nil || reference == nil {
		return ""
	}
	var redirect string
	if err := json.Unmarshal(reference["redirect"], &redirect); err != nil {
		return ""
	}
	return safeEntityDestinationURL(redirect)
}

func safeEntityDestinationURL(raw string) string {
	if raw == "" || strings.ContainsAny(raw, "\\\r\n") || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" {
		return ""
	}
	if parsed.Path != "/resource" && parsed.Path != "/note" && parsed.Path != "/group" {
		return ""
	}
	if raw != parsed.Path+"?"+parsed.RawQuery {
		return ""
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values) != 1 {
		return ""
	}
	ids, ok := values["id"]
	if !ok || len(ids) != 1 {
		return ""
	}
	id, err := strconv.ParseUint(ids[0], 10, 64)
	if err != nil || id == 0 || parsed.RawQuery != "id="+strconv.FormatUint(id, 10) {
		return ""
	}
	return parsed.Path + "?id=" + strconv.FormatUint(id, 10)
}

// ResultLink is the one link a finished Job offers straight from a list: what
// it made.
type ResultLink struct {
	URL             string
	Label           string
	AccessibleLabel string
}

// ResultLinkFor picks a succeeded Job's result link from the outputs the viewer
// may open. Any available entity output is that — the Resource a download
// created, the entity a plugin action returned — and its route redirects to the
// entity after checking the viewer may open it. Only a plugin action records a
// summary destination instead, so that fallback stays with that Kind. The zero
// value means the Job offers no result link.
func ResultLinkFor(job jobs.Snapshot, outputs []jobs.Output) ResultLink {
	if job.State != jobs.StateSucceeded {
		return ResultLink{}
	}
	context := strings.TrimSpace(job.Title)
	if context == "" {
		context = job.Kind
	}
	withContext := func(label string) string {
		if context == "" {
			return label
		}
		return label + " for " + context
	}
	for _, output := range outputs {
		if output.Type == jobs.OutputTypeEntity && output.Availability == jobs.OutputAvailable {
			name := strings.ToLower(strings.TrimSpace(output.Label))
			if name == "" {
				name = "entity"
			}
			label := "View " + name
			return ResultLink{URL: OutputURL(job.ID, output.Key), Label: label, AccessibleLabel: withContext(label)}
		}
	}
	for _, output := range outputs {
		if destination := SummaryDestinationURL(job.Kind, output); destination != "" {
			return ResultLink{URL: destination, Label: "View result", AccessibleLabel: withContext("View result")}
		}
	}
	return ResultLink{}
}
