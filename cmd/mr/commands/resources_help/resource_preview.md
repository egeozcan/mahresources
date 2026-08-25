---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource download, resource recalculate-dimensions
---

# Long

Download a server-rendered thumbnail preview of a Resource. `-w, --width`
and `--height` set the target size, each capped at 600. Give both and the
thumbnail is exactly that many pixels, so the aspect ratio is not
preserved; give one and the other is derived from the resource's aspect
ratio; give neither and the image is scaled to fit 600x600 and never
upscaled. Not every content type supports previews (e.g., some binary
formats or failed decodes). A Resource with no available preview redirects
to the server's generic placeholder image, which the CLI follows, so the
command still writes a file and exits 0.

# Example

  # Default preview
  mr resource preview 42 -o preview.jpg

  # Exactly 256x256, aspect ratio not preserved
  mr resource preview 42 -o preview.jpg -w 256 --height 256

  # mr-doctest: tolerate preview-not-available for formats without thumbnail, tolerate=/preview|no preview|not available|cannot/i
  GRP=$(mr group create --name "doctest-preview-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "preview-test-$$" --json | jq -r '.[0].ID')
  OUT=$(mktemp)
  mr resource preview $ID -o $OUT
  test -s $OUT
  rm -f $OUT
