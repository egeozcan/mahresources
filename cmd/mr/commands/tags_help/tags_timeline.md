---
outputShape: Object with buckets ([]{label, start, end, created, updated})
exitCodes: 0 on success; 1 on any error
relatedCmds: tags list, resources timeline
---

# Long

Display a timeline of Tag activity as an ASCII bar chart. Each bar
represents a time bucket (yearly, monthly, or weekly, controlled by
`--granularity`), and the bar height reflects the count of Tags created
in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60). The
`--name` and `--description` filter flags apply the same substring
matching as `tags list`. Pass the global `--json` flag to get the raw
bucket data for scripting.

# Example

  # Monthly timeline anchored at today (default)
  mr tags timeline

  # Weekly granularity, last 12 weeks
  mr tags timeline --granularity weekly --columns 12

  # Yearly timeline anchored at a specific date, JSON output
  mr tags timeline --granularity yearly --anchor 2020-01-01 --json

  # mr-doctest: verify timeline returns an array of buckets
  mr tags timeline --granularity weekly --columns 4 --json | jq -e '.buckets | type == "array"'
