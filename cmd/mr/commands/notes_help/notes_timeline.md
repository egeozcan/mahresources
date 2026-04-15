---
outputShape: Object with buckets (array of {label, start, end, created, updated}) and hasMore ({left, right})
exitCodes: 0 on success; 1 on any error
relatedCmds: notes list, resources timeline, groups timeline
---

# Long

Display a timeline of Note activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Notes
created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15, max
60). All note-list filter flags (`--name`, `--tags`, `--groups`,
`--owner-id`, `--note-type-id`) apply the same way to the timeline
aggregation. Pass the global `--json` flag to get the raw bucket data
for scripting.

# Example

  # Monthly timeline anchored at today (default)
  mr notes timeline

  # Weekly granularity, last 12 weeks
  mr notes timeline --granularity weekly --columns 12

  # Yearly timeline filtered by tag, JSON output
  mr notes timeline --granularity yearly --tags 5 --json

  # mr-doctest: create a note, verify timeline buckets array is returned
  ID=$(mr note create --name "doctest-timeline-$$-$RANDOM" --json | jq -r '.ID')
  mr notes timeline --granularity weekly --columns 4 --json | jq -e '.buckets | type == "array" and length > 0'
