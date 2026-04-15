---
exitCodes: 0 on success; 1 on any error
relatedCmds: resource-category get, resources list, categories list
---

# Long

Discover ResourceCategories. The `resource-categories` subcommand group
currently exposes `list` for filtered queries against the full set of
resource categories, with pagination via the global `--page` flag.

Resource categories are the per-Resource taxonomy (compare `categories`
for per-Group). Use `resource-categories list --json | jq` to derive
IDs for scripting, and `resource-category` for single-category CRUD.
