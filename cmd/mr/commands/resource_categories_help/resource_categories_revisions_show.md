---
outputShape: Revision summary plus snapshot, the stored value of every tracked field the revision recorded
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-categories revisions list, resource-categories revisions diff, resource-categories revisions restore
---

# Long

Show one revision of a resource category. `<number>` is the per-resource category revision
number from `list`, and `--id` is required. The output is the revision's
metadata followed by each field that has stored content, printed verbatim, which
for an old resource category can include a MetaSchema that is not valid JSON.
Under `--json` the `snapshot` object holds every recorded field, and the
`editor` object holds the same fields as the edit form loads them.

# Example

  # Show revision 2 of a resource category
  mr resource-categories revisions show 2 --id 3

  # Print one stored field
  mr resource-categories revisions show 2 --id 3 --json | jq -r '.snapshot.CustomHeader'

  # mr-doctest: show the stored header of the second revision
  ID=$(mr resource-category create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr resource-category edit --id "$ID" --custom-header '<p>one</p>'
  mr resource-categories revisions show 2 --id "$ID" --json | jq -e '.snapshot.CustomHeader == "<p>one</p>" and .source == "cli"'
  mr resource-categories revisions show 2 --id "$ID" | grep -q -F '<p>one</p>'
  mr resource-category delete "$ID"
