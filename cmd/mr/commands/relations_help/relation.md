---
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type, relation-types list, group get, group children, group parents
---

# Long

A Relation is a typed, directional link between two Groups. It has a
`FromGroupId`, a `ToGroupId`, and a `RelationTypeId` pointing at a
`relation-type` that defines the allowed category pairing and the
relationship's semantics. Relations may also carry an optional `Name`
and `Description`.

Use the `relation` subcommands to operate on a single relation by ID:
`create` links two groups, `edit-name` and `edit-description` update
its labels, and `delete` removes the link. There is no `relation list`
or `relation get`: to read a relation back, fetch a participating group
with `mr group get <id> --json` and inspect its `Relationships` array,
or query via `mr mrql`.
