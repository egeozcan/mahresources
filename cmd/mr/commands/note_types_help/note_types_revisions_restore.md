---
outputShape: Summary of the new restore revision with number, source, restoredFrom and changedSlots under --json; confirmation otherwise
exitCodes: 0 on success; 1 on any error
relatedCmds: mr note-types revisions list, mr note-types revisions diff, mr note-types revisions label
---

# Long

Restore a revision of a note type. Without `--slot` the whole revision becomes
the note type's current templates; with `--slot` only that slot is replaced and
every other field stays as it is. `--slot` takes the primary field name,
such as `CustomHeader`, never the `...CSS` sibling. The slot's CSS comes
along unless you pass `--no-css`, which requires `--slot`.

Restoring moves forward: it records a new revision with source `restore`
and never removes later ones, even when the content already matches the
current state. A slot the old revision did not have is restored as empty.
A revision whose content no longer passes the ordinary save validation is
refused with an error naming the field, and nothing is recorded.

# Example

  # Restore the whole of revision 1
  mr note-types revisions restore 1 --id 3

  # Restore only the header and keep the current header CSS
  mr note-types revisions restore 2 --id 3 --slot CustomHeader --no-css

  # mr-doctest: restore the whole revision, then one slot of another
  ID=$(mr note-type create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr note-type edit --id "$ID" --custom-header '<p>A</p>'
  mr note-type edit --id "$ID" --custom-header '<p>B</p>' --custom-sidebar '<p>S</p>'
  mr note-types revisions restore 1 --id "$ID" --json | jq -e '.number == 4 and .source == "restore" and .restoredFrom == 1'
  mr note-type get "$ID" --json | jq -e '.CustomHeader == "" and .CustomSidebar == ""'
  mr note-types revisions restore 2 --id "$ID" --slot CustomHeader --json | jq -e '.number == 5 and .changedSlots == ["CustomHeader"]'
  mr note-type get "$ID" --json | jq -e '.CustomHeader == "<p>A</p>" and .CustomSidebar == ""'
  mr note-type delete "$ID"
