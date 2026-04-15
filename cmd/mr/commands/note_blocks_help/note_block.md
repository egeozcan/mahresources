---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-blocks list, note, notes list
---

# Long

Note blocks are ordered, typed content units attached to a single Note
(similar to Notion's blocks). Each block has a type (`text`, `heading`,
`todos`, `gallery`, `references`, `table`, `calendar`, `divider`, plus
any plugin-registered types), a free-form `content` JSON payload whose
shape depends on the type, a free-form `state` JSON payload for runtime
UI/view state, and a fractional `position` string that defines its
order within the parent note.

Use the `note-block` subcommands to operate on a single block by ID:
fetch it, create a new one on a note, update its content or state,
delete it, or list the available block types. Use `note-blocks` (plural)
for per-note listing, reorder, and rebalance operations.
