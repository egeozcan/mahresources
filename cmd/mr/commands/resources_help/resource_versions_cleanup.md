---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions, resource version-delete, resources versions-cleanup
---

# Long

Bulk-delete old versions of a single Resource. Retains the N most recent
versions (`--keep`), deletes versions older than N days
(`--older-than-days`), or both together: a version must satisfy every
filter given to be deleted. Pass `--dry-run` to preview without
deleting.

The current version is never a candidate, and the last remaining version
of a resource is never deleted, so a cleanup always leaves at least one
version in place.

# Example

  # Keep only the last 3 versions
  mr resource versions-cleanup 42 --keep 3

  # Delete versions older than 90 days (preview)
  mr resource versions-cleanup 42 --older-than-days 90 --dry-run

  # mr-doctest: create 3 versions, cleanup keep=1, assert only 1 remains
  GRP=$(mr group create --name "doctest-vcleanup-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "cleanup-test-$$" --json | jq -r '.[0].ID')
  mr resource version-upload $ID ./testdata/sample.png
  mr resource version-upload $ID ./testdata/sample.txt
  mr resource versions-cleanup $ID --keep 1
  mr resource versions $ID --json | jq -e 'length == 1'
