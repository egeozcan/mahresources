---
exitCodes: 0 on success; 1 on any error
relatedCmds: resources list, groups timeline
---

# Long

Display a timeline of Resource activity as an ASCII bar chart. Each
bucket prints two bars, created above updated, with the count beside
each and a legend below the chart, and `--granularity` selects yearly,
monthly or weekly buckets. Both bars are scaled against the largest
count in the range.

The chart is anchored at the `--anchor` date (default: today) and shows
`--columns` buckets backward from the anchor (default 15, max 60; a
value outside that range falls back to 15). The resource-list filter
flags (`--name`, `--tags`, `--groups`, etc.) apply the same way to the
timeline aggregation, though `--mrql` and `--sort-by` are not available
here. Pass the global `--json` flag to get the raw bucket data for
scripting.

# Example

  # Monthly timeline anchored at today (default)
  mr resources timeline

  # Weekly granularity, last 12 weeks
  mr resources timeline --granularity weekly --columns 12

  # Yearly timeline filtered by tag, JSON output
  mr resources timeline --granularity yearly --tags 5 --json

  # mr-doctest: upload a fixture and verify timeline has at least one non-zero bucket
  GRP=$(mr group create --name "doctest-timeline-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "timeline-$$" --json | jq -r '.[0].ID')
  mr resources timeline --granularity weekly --columns 4 --json | jq -e '[.buckets[].created] | add >= 1'
