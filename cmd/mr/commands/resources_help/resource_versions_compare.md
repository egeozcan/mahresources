---
outputShape: Comparison object with SizeDelta, SameHash, SameType, DimensionsDiff
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

  # Extract SameHash via jq
  mr resource versions-compare 42 --v1 17 --v2 21 --json | jq -r .SameHash

  # mr-doctest: upload same file twice, compare, assert SameHash is true
  ID=$(mr resource upload ./testdata/sample.jpg --name "compare-test" --json | jq -r .id)
  mr resource version-upload $ID ./testdata/sample.jpg
  V1=$(mr resource versions $ID --json | jq -r '.[1].id')
  V2=$(mr resource versions $ID --json | jq -r '.[0].id')
  mr resource versions-compare $ID --v1 $V1 --v2 $V2 --json | jq -e '.SameHash == true'
