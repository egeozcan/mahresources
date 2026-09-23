---
outputShape: A page of ordered Job events
exitCodes: 0 on success; 1 on any error
relatedCmds: jobs get, jobs list
---

# Long

Read the durable event timeline for a visible Job. Events have a sequence
number scoped to the Job. Use `--after-sequence` to continue from a saved
cursor and `--limit` to bound a page.

# Example

  # Read the newest events
  mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --limit 100 --json

  # Continue after sequence 100
  mr jobs timeline 018f4db1-9b40-7f54-8f16-37a449bcf01d --after-sequence 100

  # mr-doctest: the timeline command documents its sequence cursor
  mr jobs timeline --help | grep -- '--after-sequence' >/dev/null
