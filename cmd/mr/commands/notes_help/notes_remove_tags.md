---
exitCodes: 0 on success; 1 on any error
relatedCmds: notes add-tags, notes list, tags list
---

# Long

Remove tag IDs from every Note listed in `--ids`. Idempotent: removing
a tag that isn't attached is a no-op. Both `--ids` and `--tags` accept
comma-separated unsigned integer lists and are required.

# Example

  # Remove tag 5 from notes 1, 2
  mr notes remove-tags --ids 1,2 --tags 5

  # Remove multiple tags at once
  mr notes remove-tags --ids 1,2,3 --tags 5,6

  # mr-doctest: add then remove, assert count drops to 0
  TAG=$(mr tag create --name "remove-tags-notes-$$-$RANDOM" --json | jq -r '.ID')
  ID=$(mr note create --name "doctest-remtags-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-tags --ids $ID --tags $TAG
  mr notes remove-tags --ids $ID --tags $TAG
  mr notes list --tags $TAG --json | jq -e 'length == 0'
