---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-blocks rebalance, note-blocks list, note-block update
---

# Long

Move specific blocks to new positions on their parent note. `--note-id`
and `--positions` are both required. `--positions` takes a JSON object
mapping block ID (as a string key) to its new fractional `position`
string. Only the listed blocks are moved; every other block on the
note keeps its current position. Fractional positions sort
lexicographically, so `"a" < "m" < "z"` — pick new values that slot
into the desired order.

After many reorders, positions can grow long; run `note-blocks
rebalance` to normalize them.

# Example

  # Move block 10 to the top and block 11 to the bottom of note 42
  mr note-blocks reorder --note-id 42 --positions '{"10":"a","11":"z"}'

  # Move one block between two siblings using a midpoint string
  mr note-blocks reorder --note-id 42 --positions '{"10":"m"}'

  # mr-doctest: create two blocks, reorder, verify the new first block
  NID=$(mr note create --name "doctest-nbs-reord-$$-$RANDOM" --json | jq -r '.ID')
  B1=$(mr note-block create --note-id=$NID --type=text --content '{"text":"first"}' --json | jq -r '.id')
  B2=$(mr note-block create --note-id=$NID --type=text --content '{"text":"second"}' --json | jq -r '.id')
  mr note-blocks reorder --note-id=$NID --positions "{\"$B1\":\"z\",\"$B2\":\"a\"}" > /dev/null
  mr note-blocks list --note-id=$NID --json | jq -e --argjson id "$B2" 'sort_by(.position)[0].id == $id'
