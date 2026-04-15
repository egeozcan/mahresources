---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource download, resource recalculate-dimensions
---

# Long

Download a server-rendered thumbnail preview of a Resource. Width and
height can be capped via `-w, --width` and `--height`; without caps the
server returns its default preview size. Not every content type supports
previews (e.g., some binary formats or failed decodes).

# Example

  # Default preview
  mr resource preview 42 -o preview.jpg

  # Constrained to 256x256 max
  mr resource preview 42 -o preview.jpg -w 256 --height 256

  # mr-doctest: tolerate preview-not-available for formats without thumbnail, tolerate=/preview|no preview|not available|cannot/i
  ID=$(mr resource upload ./testdata/sample.jpg --name "preview-test" --json | jq -r .id)
  OUT=$(mktemp)
  mr resource preview $ID -o $OUT
  test -s $OUT
  rm -f $OUT
