---
outputShape: Array of version objects with id, number, size, type, comment, created
exitCodes: 0 on success; 1 on any error
relatedCmds: resource version, resource version-upload, resource versions-compare
---

# Long

List every stored version of a Resource, newest first. Columns are the
version ID, version number, size in bytes, content type, an optional
author comment, and the creation timestamp. Pass the global `--json`
flag to get the full records for scripting.

# Example

  # List versions (table)
  mr resource versions 42

  # Get the newest version's ID via jq
  mr resource versions 42 --json | jq -r '.[0].id'

  # mr-doctest: upload a resource, push a new version, assert the list has 2 entries
  GRP=$(mr group create --name "doctest-versions-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "versions-test-$$" --json | jq -r '.[0].ID')
  mr resource version-upload $ID ./testdata/sample.png
  mr resource versions $ID --json | jq -e 'length >= 2'
