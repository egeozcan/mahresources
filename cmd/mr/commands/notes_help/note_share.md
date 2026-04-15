---
outputShape: Object with shareToken (string) and shareUrl (string path beginning with /s/)
exitCodes: 0 on success; 1 on any error
relatedCmds: note unshare, note get, note create
---

# Long

Generate a share token for a note, making it readable via the public
`/s/<token>` share URL without authentication. Calling `share` on a
note that is already shared rotates the token, invalidating any
previous share URL. The response contains both the raw token and the
relative share URL for convenience.

# Example

  # Share note 42 and print the share URL
  mr note share 42 --json | jq -r .shareUrl

  # Share and capture just the token for use elsewhere
  TOKEN=$(mr note share 42 --json | jq -r .shareToken)

  # mr-doctest: create, share, verify shareToken appears on get
  ID=$(mr note create --name "doctest-share-$$-$RANDOM" --json | jq -r '.ID')
  TOKEN=$(mr note share $ID --json | jq -r .shareToken)
  mr note get $ID --json | jq -e --arg t "$TOKEN" '.shareToken == $t and (.shareToken | length) > 0'
