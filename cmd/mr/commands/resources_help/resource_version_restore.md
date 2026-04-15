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
  GRP=$(mr group create --name "doctest-vrestore-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "restore-test-$$" --json | jq -r '.[0].ID')
  V1=$(mr resource versions $ID --json | jq -r '.[0].id')
  mr resource version-upload $ID ./testdata/sample.png
  BEFORE=$(mr resource versions $ID --json | jq -r 'length')
  mr resource version-restore --resource-id $ID --version-id $V1
  AFTER=$(mr resource versions $ID --json | jq -r 'length')
  test "$AFTER" -gt "$BEFORE"
