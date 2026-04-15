---
exitCodes: 0 on success; 1 on any error
relatedCmds: notes remove-tags, notes add-groups, tags list
---

# Long

Add tag IDs to every Note listed in `--ids`. Idempotent: adding a tag
that's already attached is a no-op. Both `--ids` and `--tags` accept
comma-separated unsigned integer lists and are required.

# Example

  # Add tag 5 to notes 1, 2, 3
  mr notes add-tags --ids 1,2,3 --tags 5

  # Add multiple tags at once
  mr notes add-tags --ids 1,2,3 --tags 5,6,7

  # mr-doctest: create tag + two notes, add-tags, list by tag, assert count >= 2
  TAG=$(mr tag create --name "add-tags-notes-$$-$RANDOM" --json | jq -r '.ID')
  ID1=$(mr note create --name "doctest-addtags-a-$$-$RANDOM" --json | jq -r '.ID')
  ID2=$(mr note create --name "doctest-addtags-b-$$-$RANDOM" --json | jq -r '.ID')
  mr notes add-tags --ids $ID1,$ID2 --tags $TAG
  mr notes list --tags $TAG --json | jq -e 'length >= 2'
