package jobview

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"mahresources/jobs"
)

// job_vocabulary.json is how every Job surface names a Job's Kind, where it was
// started from, what class of failure ended it and why it is blocked. The
// identifiers are the system's; the table holds the words a person reads. The
// server-rendered list reads it here and the browser components import the same
// file, so a Kind cannot read "remote-download" on one page and "Download" on
// the next.
//
//go:embed job_vocabulary.json
var jobVocabularyJSON []byte

type vocabularyTable struct {
	Kinds          map[string]string `json:"kinds"`
	Origins        map[string]string `json:"origins"`
	FailureClasses map[string]string `json:"failureClasses"`
	BlockedReasons map[string]string `json:"blockedReasons"`
}

var vocabulary = mustLoadVocabulary()

func mustLoadVocabulary() vocabularyTable {
	var table vocabularyTable
	if err := json.Unmarshal(jobVocabularyJSON, &table); err != nil {
		panic(fmt.Sprintf("jobview: job_vocabulary.json: %v", err))
	}
	return table
}

// KindLabel names a Kind as a person reads it. A Kind the table does not know,
// a retired one or a plugin's, keeps its identifier.
func KindLabel(kind string) string {
	if label, ok := vocabulary.Kinds[kind]; ok {
		return label
	}
	return kind
}

// OriginLabel names where a Job was started from.
func OriginLabel(origin string) string {
	if label, ok := vocabulary.Origins[origin]; ok {
		return label
	}
	return origin
}

// FailureClassLabel names the class of a Job's failure.
func FailureClassLabel(class string) string {
	if label, ok := vocabulary.FailureClasses[class]; ok {
		return label
	}
	return class
}

// BlockedReasonText says why a Job is blocked, from the reason its blocked event
// recorded. A reason the table does not know is read from its code: "source row
// missing" says more than nothing, and a new reason is never shown as blank.
func BlockedReasonText(reason string) string {
	if text, ok := vocabulary.BlockedReasons[reason]; ok {
		return text
	}
	words := strings.TrimSpace(strings.ReplaceAll(reason, "-", " "))
	if words == "" {
		return ""
	}
	return strings.ToUpper(words[:1]) + words[1:] + "."
}

// PhaseText is the phase a surface shows beside a Job's state, or nothing when
// the state already says it: a partial success's label is its phase, and
// "running" under Running, or "cancelling" under Cancelling, only repeats it.
// phaseText in src/components/jobCenter.js is the same rule.
func PhaseText(snapshot jobs.Snapshot) string {
	phase := strings.TrimSpace(snapshot.Phase)
	if snapshot.State == jobs.StateSucceeded && phase == jobs.PhasePartial {
		return ""
	}
	if strings.EqualFold(phase, string(snapshot.State)) || strings.EqualFold(phase, PresentJob(snapshot).Label) {
		return ""
	}
	return phase
}
