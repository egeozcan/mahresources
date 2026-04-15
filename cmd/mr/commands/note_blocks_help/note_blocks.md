---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-block get, note-block create, note get
---

# Long

Discover and reorganize the blocks attached to a Note. The `note-blocks`
subcommands operate on the full set of blocks owned by one parent note:
`list` returns every block in position order, `reorder` moves specific
blocks to new positions via an explicit `blockId -> position` map, and
`rebalance` normalizes every block's position string to clean, evenly
spaced values (useful after many reorders cause position strings to
grow long).

All commands require `--note-id` to scope to a single note. To mutate
an individual block's content, state, or type, use the singular
`note-block` subcommands.
