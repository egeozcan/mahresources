---
outputShape: Version object with id, number, size, type, comment, created
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions, resource version-download, resource version-restore
---

# Long

Fetch metadata for a single version by its version ID. Returns the same
fields as `versions` but as a single key/value record. Useful when you
know the version ID and need its size or comment without a list call.

# Example

  # Fetch a version by ID
  mr resource version 17

  # Extract size via jq
  mr resource version 17 --json | jq -r .size

  # mr-doctest: upload, version-upload, fetch second version, assert it exists
  GRP=$(mr group create --name "doctest-version-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "version-test-$$" --json | jq -r '.[0].ID')
  mr resource version-upload $ID ./testdata/sample.png
  VID=$(mr resource versions $ID --json | jq -r '.[0].id')
  mr resource version $VID --json | jq -e '.id > 0'
