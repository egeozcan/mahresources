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

  # mr-doctest: upload, push version, delete older version, assert count == 1
  ID=$(mr resource upload ./testdata/sample.jpg --name "vdel-test" --json | jq -r .id)
  mr resource version-upload $ID ./testdata/sample.png
  V1=$(mr resource versions $ID --json | jq -r '.[1].id')
  mr resource version-delete --resource-id $ID --version-id $V1
  mr resource versions $ID --json | jq -e 'length == 1'
