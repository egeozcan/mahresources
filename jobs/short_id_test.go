package jobs

import "testing"

// The same cases as src/components/jobDetail.test.ts shortJobId, since the page
// title the server renders and the one the page keeps current must agree.
func TestShortIDIsTheIdsLastEightLettersAndDigits(t *testing.T) {
	for id, want := range map[string]string{
		"01a0ddb2-7830-7abc-8def-0123456789AB": "456789ab",
		"job-1":                                "job1",
		"a/b":                                  "ab",
		"":                                     "",
		"dışa-ID-12345678":                     "12345678",
	} {
		if got := ShortID(id); got != want {
			t.Errorf("ShortID(%q) = %q, want %q", id, got, want)
		}
	}
}
