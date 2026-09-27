package application_context

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"mahresources/jobs"
)

// A Job's title names its subject, so that two Jobs of one Kind can be told
// apart in every list: the group an export exports, the Resource Reduction a
// clustering run computes, the archive an import was uploaded as. A title may
// carry such a name, which is a person's own text, because a Job is read only by
// its owner and by administrators, and at acceptance both could already read the
// name: the title publishes nothing its readers could not see. Two things follow
// and are accepted. A title is a snapshot taken at acceptance, so a later rename
// or deletion does not reach it. And an owner who is later re-scoped or demoted
// still reads their own Job's title, as they still read its history.

// jobTitleNameRunes bounds one name in a title, so a title does not grow with
// its input.
const jobTitleNameRunes = 80

// jobTitleEllipsis ends a name or a title that was cut.
const jobTitleEllipsis = "…"

// jobTitleName is one name as a title may carry it: without control characters
// or surrounding space, and at most jobTitleNameRunes characters, ending in an
// ellipsis when it was cut.
func jobTitleName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name))
	if utf8.RuneCountInString(name) <= jobTitleNameRunes {
		return name
	}
	runes := []rune(name)
	return strings.TrimSpace(string(runes[:jobTitleNameRunes-1])) + jobTitleEllipsis
}

// boundedJobTitle bounds a whole title to jobs.MaxTitleBytes without splitting a
// character, ending in an ellipsis when it was cut.
func boundedJobTitle(title string) string {
	if len(title) <= jobs.MaxTitleBytes {
		return title
	}
	return truncateUTF8(title, jobs.MaxTitleBytes-len(jobTitleEllipsis)) + jobTitleEllipsis
}

// truncateUTF8 cuts value to at most limit bytes without splitting a character.
func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
