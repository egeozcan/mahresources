---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions, resource versions-cleanup
---

# Long

Delete a specific version by ID. The parent Resource is untouched. Both
`--resource-id` and `--version-id` are required. Fails if deleting would
leave the Resource with zero versions.

# Example

  # Delete an old version
  mr resource version-delete --resource-id 42 --version-id 17

  # Pipe a list of old version IDs
  mr resource versions 42 --json | jq -r '.[1:][].id' | xargs -I {} mr resource version-delete --resource-id 42 --version-id {}

  # mr-doctest: upload, push version, delete the old version, assert count decreased
  GRP=$(mr group create --name "doctest-vdel-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "vdel-test-$$" --json | jq -r '.[0].ID')
  VOLD=$(mr resource versions $ID --json | jq -r '.[-1].id')
  mr resource version-upload $ID ./testdata/sample.png
  BEFORE=$(mr resource versions $ID --json | jq -r 'length')
  mr resource version-delete --resource-id $ID --version-id $VOLD
  AFTER=$(mr resource versions $ID --json | jq -r 'length')
  test "$AFTER" -lt "$BEFORE"
