---
outputShape: Object with buckets (array of {label, start, end, created, updated}) and hasMore (bool)
exitCodes: 0 on success; 1 on any error
relatedCmds: groups list, resources timeline
---

# Long

Display a timeline of Group creation and update activity as an ASCII
bar chart. Each bar represents a time bucket (yearly, monthly, or
weekly, controlled by `--granularity`), and the bar height reflects
the count of Groups created in that bucket.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor. All group-list filter
flags (`--name`, `--tags`, `--groups`, `--owner-id`, etc.) apply the
same way to the timeline aggregation. Pass the global `--json` flag to
get the raw bucket data for scripting — the top-level response has a
`buckets` array and a `hasMore` flag.

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
