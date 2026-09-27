package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"mahresources/jobs"
)

// JobSummaryDestination is the entity page a plugin action's result summary
// names as its redirect: "/resource?id=N", "/note?id=N" or "/group?id=N", or ""
// when the summary names none or names anything looser than exactly that. It is
// the one reading of a summary's destination: the Job API and the Job Center
// link through it, and GetOpenableJobOutputs withholds it when the entity is one
// the viewer cannot open.
func JobSummaryDestination(jobKind string, output jobs.Output) string {
	path, id, ok := jobSummaryDestinationEntity(jobKind, output)
	if !ok {
		return ""
	}
	return path + "?id=" + strconv.FormatUint(id, 10)
}

// jobSummaryDestinationEntity is JobSummaryDestination as the page's path and the
// entity's id.
func jobSummaryDestinationEntity(jobKind string, output jobs.Output) (string, uint64, bool) {
	if jobKind != JobKindPluginAction || output.Key != "result" ||
		output.Type != jobs.OutputTypeSummary || output.Availability != jobs.OutputAvailable || !json.Valid(output.Reference) {
		return "", 0, false
	}
	var reference map[string]json.RawMessage
	if err := json.Unmarshal(output.Reference, &reference); err != nil || reference == nil {
		return "", 0, false
	}
	var redirect string
	if err := json.Unmarshal(reference["redirect"], &redirect); err != nil {
		return "", 0, false
	}
	return safeEntityDestination(redirect)
}

// safeEntityDestination accepts only a same-origin entity page of exactly the
// form "/resource?id=N", "/note?id=N" or "/group?id=N".
func safeEntityDestination(raw string) (string, uint64, bool) {
	if raw == "" || strings.ContainsAny(raw, "\\\r\n") || !strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "//") {
		return "", 0, false
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" ||
		parsed.Fragment != "" || parsed.RawFragment != "" || parsed.RawPath != "" {
		return "", 0, false
	}
	if parsed.Path != "/resource" && parsed.Path != "/note" && parsed.Path != "/group" {
		return "", 0, false
	}
	if raw != parsed.Path+"?"+parsed.RawQuery {
		return "", 0, false
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil || len(values) != 1 {
		return "", 0, false
	}
	ids, ok := values["id"]
	if !ok || len(ids) != 1 {
		return "", 0, false
	}
	id, err := strconv.ParseUint(ids[0], 10, 64)
	if err != nil || id == 0 || parsed.RawQuery != "id="+strconv.FormatUint(id, 10) {
		return "", 0, false
	}
	return parsed.Path, id, true
}

// openableJobOutput applies the rule every output that names an entity follows,
// whichever Kind published it: it is offered only while the viewer can open that
// entity, asked through the resolver opening an entity output uses. An entity
// output whose entity the viewer cannot open is hidden (reported false). A result
// summary whose redirect names such an entity keeps its JSON and loses the
// redirect, so nothing links to the page. Deletion and lost scope are not told
// apart.
func (ctx *MahresourcesContext) openableJobOutput(jobKind string, output jobs.Output) (jobs.Output, bool, error) {
	if output.Availability != jobs.OutputAvailable {
		return output, true, nil
	}
	if output.Type == jobs.OutputTypeEntity {
		reachable, err := ctx.jobEntityReferenceReachable(output.Reference)
		return output, reachable, err
	}
	path, id, ok := jobSummaryDestinationEntity(jobKind, output)
	if !ok {
		return output, true, nil
	}
	field := map[string]string{"/resource": "resourceId", "/note": "noteId", "/group": "groupId"}[path]
	reachable, err := ctx.jobEntityReferenceReachable(json.RawMessage(fmt.Sprintf(`{%q:%d}`, field, id)))
	if err != nil || reachable {
		return output, true, err
	}
	var reference map[string]json.RawMessage
	if err := json.Unmarshal(output.Reference, &reference); err != nil {
		return output, true, err
	}
	delete(reference, "redirect")
	stripped, err := json.Marshal(reference)
	if err != nil {
		return output, true, err
	}
	output.Reference = stripped
	return output, true, nil
}

// jobEntityReferenceReachable answers whether an entity reference names an entity
// this context's principal can open. A reference the resolver refuses as
// malformed is not reachable either.
func (ctx *MahresourcesContext) jobEntityReferenceReachable(reference json.RawMessage) (bool, error) {
	if _, err := ctx.resolveJobEntityOutput(reference); err != nil {
		if errors.Is(err, jobs.ErrNotFound) || errors.Is(err, ErrJobOutputInvalid) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
