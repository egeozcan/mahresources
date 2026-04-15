---
exitCodes: 0 on success; 1 on any error
relatedCmds: note-type get, notes list, note create
---

# Long

Discover Note Types, the typed schemas assigned to Notes. The
`note-types` subcommand currently exposes `list` for filtered queries
(with pagination via the global `--page` flag). Pipe `note-types list
--json` through `jq` when you need to derive IDs to feed into
`note create --note-type-id`.

Singular operations (get, create, edit, delete) live under the sibling
`note-type` command.
