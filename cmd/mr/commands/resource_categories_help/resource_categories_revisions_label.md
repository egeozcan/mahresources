---
outputShape: Revision summary with the new label under --json; confirmation otherwise
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-categories revisions list, resource-categories revisions show, resource-categories revisions restore
---

# Long

Set, change or clear the label of a revision of a resource category. Pass the new
text as the second argument, at most 200 characters, or pass `--clear` to
remove the label; giving both or neither is an error. The label is the
only part of a revision that can change after it is recorded.

# Example

  # Label revision 2
  mr resource-categories revisions label 2 "before redesign" --id 3

  # Remove the label again
  mr resource-categories revisions label 2 --clear --id 3

  # mr-doctest: set a label, read it back, then clear it
  ID=$(mr resource-category create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr resource-categories revisions label 1 "first draft" --id "$ID" --json | jq -e '.label == "first draft"'
  mr resource-categories revisions list --id "$ID" --json | jq -e '.[0].label == "first draft"'
  mr resource-categories revisions label 1 --clear --id "$ID"
  mr resource-categories revisions list --id "$ID" --json | jq -e '.[0].label == ""'
  mr resource-category delete "$ID"
