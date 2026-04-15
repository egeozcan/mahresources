---
outputShape: Comparison object with sizeDelta, sameHash, sameType, dimensionsDiff
exitCodes: 0 on success; 1 on any error
relatedCmds: resource versions, resource version, resource versions-cleanup
---

# Long

Compare two versions of a Resource and report the size delta, whether
the content hashes match, whether the content types match, and the
dimension differences. Both `--v1` and `--v2` are required and must be
version IDs of the same Resource.

# Example

  # Compare two versions (table)
  mr resource versions-compare 42 --v1 17 --v2 21

  # Extract sameHash via jq
  mr resource versions-compare 42 --v1 17 --v2 21 --json | jq -r .sameHash

  # mr-doctest: upload same file twice, compare, assert sameHash is true
  GRP=$(mr group create --name "doctest-vcompare-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "compare-test-$$" --json | jq -r '.[0].ID')
  V1=$(mr resource versions $ID --json | jq -r '.[0].id')
  mr resource version-upload $ID ./testdata/sample.jpg
  V2=$(mr resource versions $ID --json | jq -r '.[0].id')
  mr resource versions-compare $ID --v1 $V1 --v2 $V2 --json | jq -e '.sameHash == true'
