---
outputShape: Object with buckets (array of {label, start, end, created, updated}) and hasMore ({left, right} booleans)
exitCodes: 0 on success; 1 on any error
relatedCmds: groups list, resources timeline
---

# Long

Display a timeline of Group creation and update activity as an ASCII
bar chart. Each bucket is a time span (yearly, monthly, or weekly,
controlled by `--granularity`), and each bucket prints two bars, `█` for
Groups created in it and `▓` for Groups updated, both scaled against the
largest count on the chart.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor. The `groups list` filter
flags apply the same way to the timeline aggregation, with one exception:
`--mrql` is not available here. Pass the global `--json` flag to get the
raw bucket data for scripting: the top-level response has a `buckets`
array and a `hasMore` object with `left` and `right` booleans.

# Example

  # Monthly timeline anchored at today (default)
  mr groups timeline

  # Weekly granularity, last 20 weeks
  mr groups timeline --granularity weekly --columns 20

  # Yearly timeline anchored at 2020
  mr groups timeline --granularity yearly --anchor 2020-01-01

  # mr-doctest: create a group, verify timeline has at least one non-zero created bucket
  ID=$(mr group create --name "doctest-tl-$$-$RANDOM" --json | jq -r '.ID')
  mr groups timeline --granularity weekly --columns 4 --json | jq -e '[.buckets[].created] | add >= 1'
