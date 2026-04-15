---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource rotate, resource recalculate-dimensions
---

# Long

Force the stored `width` and `height` on every Resource listed in
`--ids`. Useful when `recalculate-dimensions` cannot decode the file
format (e.g., proprietary formats) or when the stored dimensions are
known to be stale. Does not transform the file bytes; only updates the
database record. All three flags (`--ids`, `--width`, `--height`) are
required.

# Example

  # Set dimensions on a single resource
  mr resources set-dimensions --ids 7 --width 1920 --height 1080

  # Batch update from a tag filter
  IDS=$(mr resources list --tags 5 --json | jq -r 'map(.id) | join(",")')
  mr resources set-dimensions --ids $IDS --width 800 --height 600

  # mr-doctest: upload, force known dimensions, assert via get
  ID=$(mr resource upload ./testdata/sample.jpg --name "setdim-$$" --json | jq -r .id)
  mr resources set-dimensions --ids $ID --width 1024 --height 768
  mr resource get $ID --json | jq -e '(.width // .Width) == 1024'
