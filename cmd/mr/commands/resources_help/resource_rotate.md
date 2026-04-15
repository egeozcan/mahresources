---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource preview, resource edit, resource versions
---

# Long

Rotate an image Resource by the given number of degrees. Only image
Resources are supported; the rotation creates a new version on success
so the original is preserved. The `--degrees` flag is required and
typically takes 90, 180, or 270 (negative values rotate counter-
clockwise).

# Example

  # Rotate 90 degrees clockwise
  mr resource rotate 42 --degrees 90

  # Rotate 180 degrees
  mr resource rotate 42 --degrees 180

  # mr-doctest: small fixtures may fail to decode; tolerate known errors, tolerate=/unexpected EOF|not supported|cannot decode|too small/i
  ID=$(mr resource upload ./testdata/sample.jpg --name "rotate-test" --json | jq -r .id)
  mr resource rotate $ID --degrees 90
