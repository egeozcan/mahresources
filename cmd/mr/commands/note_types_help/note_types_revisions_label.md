---
outputShape: Revision summary with the new label under --json; confirmation otherwise
exitCodes: 0 on success; 1 on any error
relatedCmds: mr note-types revisions list, mr note-types revisions show, mr note-types revisions restore
---

# Long

Set, change or clear the label of a revision of a note type. Pass the new
text as the second argument, at most 200 characters, or pass `--clear` to
remove the label; giving both or neither is an error. The label is the
only part of a revision that can change after it is recorded.

# Example

  # Label revision 2
  mr note-types revisions label 2 "before redesign" --id 3

  # Remove the label again
  mr note-types revisions label 2 --clear --id 3

  # mr-doctest: set a label, read it back, then clear it
  ID=$(mr note-type create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr note-types revisions label 1 "first draft" --id "$ID" --json | jq -e '.label == "first draft"'
  mr note-types revisions list --id "$ID" --json | jq -e '.[0].label == "first draft"'
  mr note-types revisions label 1 --clear --id "$ID"
  mr note-types revisions list --id "$ID" --json | jq -e '.[0].label == ""'
  mr note-type delete "$ID"
