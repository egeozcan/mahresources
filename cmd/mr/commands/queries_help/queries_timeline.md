---
outputShape: Object with buckets array (each bucket has label, start, end, created, updated) and hasMore (left, right)
exitCodes: 0 on success; 1 on any error
relatedCmds: queries list, resources timeline, groups timeline
---

# Long

Display a timeline of saved-Query activity as an ASCII bar chart.
Each bar represents a time bucket (yearly, monthly, or weekly,
controlled by `--granularity`), with bar height reflecting the count
of queries created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and
shows `--columns` buckets backward from the anchor (default 15, max
60). Pass `--name` to filter by query name substring. Pass the
global `--json` flag to get the raw bucket data for scripting.

# Example

  # Monthly timeline anchored at today (default)
  mr queries timeline

  # Weekly granularity, last 12 weeks
  mr queries timeline --granularity weekly --columns 12

  # Yearly timeline as JSON
  mr queries timeline --granularity yearly --json

  # mr-doctest: create a query, verify timeline has at least one non-zero created bucket
  NAME="doctest-timeline-$$-$RANDOM"
  mr query create --name "$NAME" --text "select 1 as x" --json >/dev/null
  mr queries timeline --granularity weekly --columns 4 --json | jq -e '[.buckets[].created] | add >= 1'
