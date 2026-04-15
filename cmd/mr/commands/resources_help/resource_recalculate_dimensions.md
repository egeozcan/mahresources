---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource get, resource rotate, resources set-dimensions
---

# Long

Re-read an image Resource's bytes and update its stored width and
height. Useful after external file edits or when the original ingest
path failed to decode dimensions. Does not modify the file content
itself; only updates the database record.

# Example

  # Recalculate dimensions for a single resource
  mr resource recalculate-dimensions 42

  # Pipe from a list query to bulk-recalculate
  mr resources list --content-type image/jpeg --json | jq -r '.[].id' | xargs -I {} mr resource recalculate-dimensions {}

  # mr-doctest: upload a known-dimension fixture and verify dimensions populate
  GRP=$(mr group create --name "doctest-recalc-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr resource upload ./testdata/sample.png --owner-id=$GRP --name "recalc-test-$$" --json | jq -r '.[0].ID')
  mr resource recalculate-dimensions $ID
  mr resource get $ID --json | jq -e '.Width > 0'
