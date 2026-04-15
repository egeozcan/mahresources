---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions, resource version, resource version-upload
---

# Long

Restore a previous version to be the current version of a Resource.
Creates a new version that is a copy of the target (the original target
version is preserved). Both `--resource-id` and `--version-id` are
required. The optional `--comment` annotates the restore for the audit
trail.

# Example

  # Restore with a comment
  mr resource version-restore --resource-id 42 --version-id 17 --comment "revert bad edit"

  # Silent restore
  mr resource version-restore --resource-id 42 --version-id 17

  # mr-doctest: upload, version-upload, restore to v1, assert versions count grew
  ID=$(mr resource upload ./testdata/sample.jpg --name "restore-test" --json | jq -r .id)
  mr resource version-upload $ID ./testdata/sample.png
  V1=$(mr resource versions $ID --json | jq -r '.[1].id')
  mr resource version-restore --resource-id $ID --version-id $V1
  mr resource versions $ID --json | jq -e 'length == 3'
