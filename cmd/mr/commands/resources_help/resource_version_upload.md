---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions, resource version, resource version-restore
---

# Long

Push a new version of an existing Resource. The new bytes replace the
current version pointer; previous versions remain accessible via their
version IDs. The `--comment` flag attaches a free-form note (useful for
"rotated 90°" or "rescanned" audit trails).

# Example

  # Upload a new version
  mr resource version-upload 42 ./photo_v2.jpg

  # With a comment
  mr resource version-upload 42 ./photo_v2.jpg --comment "color corrected"

  # mr-doctest: upload, push second version, assert versions count increased
  GRP=$(mr group create --name "doctest-vupload-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "vup-test-$$" --json | jq -r '.[0].ID')
  BEFORE=$(mr resource versions $ID --json | jq -r 'length')
  mr resource version-upload $ID ./testdata/sample.png
  AFTER=$(mr resource versions $ID --json | jq -r 'length')
  test "$AFTER" -gt "$BEFORE"
