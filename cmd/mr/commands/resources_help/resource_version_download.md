---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource download, resource versions, resource version
---

# Long

Stream a specific version's bytes to a local file. Use `resource
download` to fetch the current version; this command exists to retrieve
older versions by their version ID. Output path defaults to
`version_<id>` if `-o` is not given.

# Example

  # Download a version to an explicit path
  mr resource version-download 17 -o old.jpg

  # Default output path
  mr resource version-download 17

  # mr-doctest: upload 2 versions, download both, assert sizes differ, timeout=60s
  GRP=$(mr group create --name "doctest-vdownload-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "vdl-test-$$" --json | jq -r '.[0].ID')
  mr resource version-upload $ID ./testdata/sample.png
  V1=$(mr resource versions $ID --json | jq -r '.[1].id')
  V2=$(mr resource versions $ID --json | jq -r '.[0].id')
  OUT1=$(mktemp); OUT2=$(mktemp)
  mr resource version-download $V1 -o $OUT1
  mr resource version-download $V2 -o $OUT2
  test $(stat -f%z $OUT1 2>/dev/null || stat -c%s $OUT1) -ne $(stat -f%z $OUT2 2>/dev/null || stat -c%s $OUT2)
  rm -f $OUT1 $OUT2
