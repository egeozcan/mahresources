---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, resource preview, resource version-download
---

# Long

Stream a Resource's bytes to a local file. Writes to the path given by
`-o, --output`, defaulting to `resource_<id>` in the current directory.
The file content is streamed as-is from the server; no conversion is
performed.

# Example

  # Download to an explicit path
  mr resource download 42 -o ./out.jpg

  # Download to the default path (resource_42)
  mr resource download 42

  # mr-doctest: upload, download, assert file exists and is non-empty, timeout=60s
  GRP=$(mr group create --name "doctest-download-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "dl-test-$$" --json | jq -r '.[0].ID')
  OUT=$(mktemp)
  mr resource download $ID -o $OUT
  test -s $OUT
  rm -f $OUT
