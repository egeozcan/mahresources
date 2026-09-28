package application_context

import (
	"encoding/json"
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
	offered, shown, err := ctx.openableJobOutputs([]jobOutputOf{{Snapshot: jobs.Snapshot{Kind: jobKind}, Output: output}})
	if err != nil {
		return output, true, err
	}
	return offered[0], shown[0], nil
}

// jobOutputOf is one output together with the Job that published it.
type jobOutputOf struct {
	Snapshot jobs.Snapshot
	Output   jobs.Output
}

// openableJobOutputs is openableJobOutput for many outputs at once. It answers
// each output as offered, and whether it is shown, in order. The entities they
// name are asked about with one read per entity type rather than one per output,
// so a listing of many rows costs what one row costs.
func (ctx *MahresourcesContext) openableJobOutputs(items []jobOutputOf) ([]jobs.Output, []bool, error) {
	type named struct {
		page     string
		id       uint
		redirect bool
	}
	offered := make([]jobs.Output, len(items))
	shown := make([]bool, len(items))
	names := make([]*named, len(items))
	wanted := map[string][]uint{}
	asked := map[string]map[uint]bool{}
	for i, item := range items {
		offered[i], shown[i] = item.Output, true
		if item.Output.Availability != jobs.OutputAvailable {
			continue
		}
		var name named
		if item.Output.Type == jobs.OutputTypeEntity {
			// An import review is named by its parse's handle rather than an id, and
			// is offered while the import's files are there (importReviewOutputOffered).
			if review, isReview, err := ctx.importReviewOutputOffered(item.Snapshot.Kind, item.Output.Reference); isReview {
				if err != nil {
					return nil, nil, err
				}
				shown[i] = review
				continue
			}
			page, id, err := jobEntityTarget(item.Output.Reference)
			if err != nil {
				// A reference the resolver refuses as malformed opens nothing.
				shown[i] = false
				continue
			}
			name = named{page: page, id: id}
		} else {
			page, id, ok := jobSummaryDestinationEntity(item.Snapshot.Kind, item.Output)
			if !ok {
				continue
			}
			name = named{page: page, id: uint(id), redirect: true}
		}
		names[i] = &name
		if asked[name.page] == nil {
			asked[name.page] = map[uint]bool{}
		}
		if !asked[name.page][name.id] {
			asked[name.page][name.id] = true
			wanted[name.page] = append(wanted[name.page], name.id)
		}
	}
	reachable := make(map[string]map[uint]bool, len(wanted))
	for page, ids := range wanted {
		found, err := ctx.jobEntitiesReachable(page, ids)
		if err != nil {
			return nil, nil, err
		}
		reachable[page] = found
	}
	for i, name := range names {
		if name == nil || reachable[name.page][name.id] {
			continue
		}
		if !name.redirect {
			shown[i] = false
			continue
		}
		var reference map[string]json.RawMessage
		if err := json.Unmarshal(items[i].Output.Reference, &reference); err != nil {
			return nil, nil, err
		}
		delete(reference, "redirect")
		stripped, err := json.Marshal(reference)
		if err != nil {
			return nil, nil, err
		}
		offered[i].Reference = stripped
	}
	return offered, shown, nil
}
