---
outputShape: Object with success (bool, true) on successful unshare
exitCodes: 0 on success; 1 on any error
relatedCmds: note share, note get
---

# Long

Remove the share token from a note, invalidating any previous share
URL. Calling `unshare` on a note that is not currently shared is a
no-op from the client's perspective but still returns success. After
unsharing, subsequent `get` responses will omit the `shareToken`
field entirely.

# Example

  # Unshare note 42
  mr note unshare 42

  # Unshare and confirm via JSON response
  mr note unshare 42 --json | jq -e '.success == true'

  # mr-doctest: share, unshare, verify shareToken is absent
  ID=$(mr note create --name "doctest-unshare-$$-$RANDOM" --json | jq -r '.ID')
  mr note share $ID >/dev/null
  mr note unshare $ID --json | jq -e '.success == true'
  mr note get $ID --json | jq -e '.shareToken == null'
