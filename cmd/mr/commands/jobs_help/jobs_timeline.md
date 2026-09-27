---
outputShape: A page of ordered Job events, oldest first, with nextSequence when more events follow
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs get, jobs list
---

# Long

Read the durable event timeline for a visible Job. Events have a sequence
number scoped to the Job, and a page lists them oldest first, starting
after `--after-sequence` (from the first event when it is omitted).
`--limit` bounds a page: 200 events by default, at most 1000. When more
events follow a page, the answer carries `nextSequence`; pass it as
`--after-sequence` to read the next page. A page without `nextSequence`
is the last one.

# Example

  # Read the first 100 events
  mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --limit 100 --json

  # Continue after sequence 100
  mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --after-sequence 100

  # mr-doctest: the timeline command documents its sequence cursor
  mr jobs timeline --help | grep -- '--after-sequence' >/dev/null
