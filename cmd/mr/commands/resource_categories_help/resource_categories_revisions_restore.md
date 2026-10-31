---
outputShape: Summary of the new restore revision with number, source, restoredFrom and changedSlots under --json; confirmation otherwise
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-categories revisions list, resource-categories revisions diff, resource-categories revisions label
---

# Long

Restore a revision of a resource category. Without `--slot` the whole revision becomes
the resource category's current templates; with `--slot` only that slot is replaced and
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
  mr resource-categories revisions restore 1 --id 3

  # Restore only the header and keep the current header CSS
  mr resource-categories revisions restore 2 --id 3 --slot CustomHeader --no-css

  # mr-doctest: restore one slot with and without its CSS, then a whole revision
  ID=$(mr resource-category create --name "rev-$$-$RANDOM" --json | jq -r .ID)
  mr resource-category edit --id "$ID" --custom-header '<p>A</p>' --custom-header-css '.a{}'
  mr resource-category edit --id "$ID" --custom-header '<p>B</p>' --custom-header-css '.b{}' --custom-sidebar '<p>S</p>'
  mr resource-categories revisions restore 2 --id "$ID" --slot CustomHeader --no-css --json | jq -e '.number == 4 and .source == "restore" and .restoredFrom == 2'
  mr resource-category get "$ID" --json | jq -e '.CustomHeader == "<p>A</p>" and .CustomHeaderCSS == ".b{}" and .CustomSidebar == "<p>S</p>"'
  mr resource-categories revisions restore 2 --id "$ID" --slot CustomHeader --json | jq -e '.number == 5'
  mr resource-category get "$ID" --json | jq -e '.CustomHeaderCSS == ".a{}" and .CustomSidebar == "<p>S</p>"'
  mr resource-categories revisions restore 1 --id "$ID" --json | jq -e '.number == 6 and .restoredFrom == 1'
  mr resource-category get "$ID" --json | jq -e '.CustomHeader == "" and .CustomSidebar == ""'
  mr resource-category delete "$ID"
