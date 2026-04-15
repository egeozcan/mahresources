---
exitCodes: 0 on success; 1 on any error
relatedCmds: relation-type create, relation create, categories list
---

# Long

Discover RelationTypes. The `relation-types` group currently exposes
only `list` for paginated, filterable reads. Use `relation-type`
(singular) for create/edit/delete operations on a specific type.

List results power downstream workflows: pipe `relation-types list
--json` into jq to pick an ID by name, then pass it to `mr relation
create --relation-type-id <id>` when linking two groups.
