---
outputShape: Object with carrierType, carrierId, from, optional to, and slots, each slot holding fields with equal, insert and delete hunks of lines; text mode prints a unified diff
exitCodes: 0 on success; 1 on any error
relatedCmds: mr categories revisions list, mr categories revisions show, mr categories revisions restore
---

# Long

Diff the templates of two revisions of a category. `--id` and `--from` are
required; `--to` defaults to the category's current state. Only fields
that differ are shown. Text output is a unified diff per field, with three
lines of context and `-` and `+` marking removed and added lines.
MetaSchema, SectionConfig and AutoDetectRules are compared as
pretty-printed JSON with sorted keys, so key order never shows as a
change. Under `--json` the server's hunks are printed unchanged.

# Example

  # Compare revision 1 with the current state
  mr categories revisions diff --id 3 --from 1

  # Compare two revisions as JSON
  mr categories revisions diff --id 3 --from 1 --to 4 --json | jq '.slots[].slot'

  # mr-doctest: diff two header edits in text and JSON form
  ID=$(mr category create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr category edit --id "$ID" --custom-header '<p>one</p>'
  mr category edit --id "$ID" --custom-header '<p>two</p>'
  mr categories revisions diff --id "$ID" --from 2 --to 3 | grep -q -x -e '-<p>one</p>'
  mr categories revisions diff --id "$ID" --from 2 | grep -q -x -e '+<p>two</p>'
  mr categories revisions diff --id "$ID" --from 3 | grep -q 'No differences'
  mr categories revisions diff --id "$ID" --from 2 --to 3 --json | jq -e '.slots[0].slot == "CustomHeader"'
  mr category delete "$ID"
