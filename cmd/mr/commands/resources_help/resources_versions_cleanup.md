---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions-cleanup, resource versions, resources list
---

# Long

Bulk-clean old Resource versions across the entire corpus. Applies the
same retention rules as the singular `resource versions-cleanup`:
`--keep N` retains the N most recent versions per resource;
`--older-than-days N` removes versions older than N days. Both filters
may be combined. Scope the operation to a single owner group with
`--owner-id`. Pass `--dry-run` to preview the count of versions that
would be removed without committing any deletes.

# Example

  # Keep last 3 versions across all resources
  mr resources versions-cleanup --keep 3

  # Preview cleanup of versions older than 90 days, scoped to owner group 5
  mr resources versions-cleanup --older-than-days 90 --owner-id 5 --dry-run

  # Remove all but the latest version across the entire corpus
  mr resources versions-cleanup --keep 1

  # mr-doctest: upload 2 resources, push extra versions, cleanup keep=1, assert each has 1 version
  GRP=$(mr group create --name "doctest-vcsbulk-$$-$RANDOM" --json | jq -r '.ID')
  ID1=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "vcu-a-$$" --json | jq -r '.[0].ID')
  ID2=$(mr resource upload ./testdata/sample.png --owner-id=$GRP --name "vcu-b-$$" --json | jq -r '.[0].ID')
  mr resource version-upload $ID1 ./testdata/sample.png
  mr resource version-upload $ID2 ./testdata/sample.jpg
  mr resources versions-cleanup --keep 1
  mr resource versions $ID1 --json | jq -e 'length == 1'
  mr resource versions $ID2 --json | jq -e 'length == 1'
