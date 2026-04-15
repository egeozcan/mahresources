---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource edit, series get, resources list
---

# Long

Remove a resource from its series. Takes the resource ID as a single
positional argument and clears the resource's `SeriesId`; the series
itself and the resource's bytes are preserved. To move a resource to a
different series instead of detaching it, use `resource edit
--series-id` on the resource.

# Example

  # Detach resource 123 from whatever series it belongs to
  mr series remove-resource 123

  # Detach and confirm by inspecting the resource's seriesId
  mr series remove-resource 123 && mr resource get 123 --json | jq .seriesId

  # mr-doctest: create series+group+resource, attach via edit, detach via remove-resource
  SID=$(mr series create --name "doctest-srm-$$-$RANDOM" --json | jq -r '.ID')
  GID=$(mr group create --name "doctest-srm-g-$$-$RANDOM" --json | jq -r '.ID')
  RID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GID --name "doctest-srm-r-$$-$RANDOM" --json | jq -r '.[0].ID')
  mr resource edit $RID --series-id=$SID > /dev/null
  mr resource get $RID --json | jq --argjson s "$SID" -e '.seriesId == $s'
  mr series remove-resource $RID > /dev/null
  mr resource get $RID --json | jq -e '.seriesId == null'
