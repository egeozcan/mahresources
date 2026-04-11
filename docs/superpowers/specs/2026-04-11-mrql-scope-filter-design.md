# MRQL Scope Filter Design

## Summary

Add a first-class `SCOPE` clause to MRQL that filters query results to entities within a group's ownership subtree. The `[mrql]` shortcode gains a `scope` attribute (entity/parent/root/global) that resolves to a group ID and delegates to this mechanism. The data-views plugin is retrofitted to use the same MRQL-level scope instead of its own flat `owner_id` filter.

## Motivation

Currently, scope filtering only exists in the data-views Lua plugin, where it applies a flat `WHERE owner_id = ?`. This has two problems:

1. **No tree semantics** — scoping to a group only returns its direct children, not the full subtree.
2. **Not reusable** — the CLI, API, saved queries, and `[mrql]` shortcodes have no access to scope filtering. Every caller that wants scope must implement its own resolution.

Making scope a MRQL-native concept solves both: tree-based filtering through a single code path available everywhere MRQL is executed.

## MRQL Language Changes

### Syntax

`SCOPE` is a top-level clause, positioned after `WHERE` and before `ORDER BY`:

```
WHERE type = "resource" AND name ~ "photo" SCOPE 42 ORDER BY created LIMIT 10
WHERE type = "note" SCOPE "My Project"
SCOPE 123
```

Accepts:
- **Number literal** — group ID (e.g., `SCOPE 42`)
- **String literal** — group name (e.g., `SCOPE "My Project"`)

### AST

New `ScopeExpr` field on the `Query` struct, holding either a `NumberLiteral` (group ID) or `StringLiteral` (group name).

### Validation

- Number: no parse-time validation (existence checked at execution).
- String: deferred to execution time (requires DB lookup).

### Translation

When `Query.Scope` is present, the translator:

1. **Resolves string names** to group IDs via `SELECT id, name, category_id FROM groups WHERE name = ?`.
2. **Emits a recursive CTE** to collect the full subtree:

```sql
WITH RECURSIVE scope_tree AS (
    SELECT id FROM groups WHERE id = ?
    UNION ALL
    SELECT g.id FROM groups g
    INNER JOIN scope_tree st ON g.owner_id = st.id
    WHERE depth < 50
)
```

3. **Injects** `WHERE owner_id IN (SELECT id FROM scope_tree)` into the query, composed with existing WHERE conditions.

Works identically on SQLite and Postgres.

## Error Handling

### Ambiguous group name

When `SCOPE "name"` matches multiple groups, return an error listing all matches with context:

```
ambiguous scope "My Project": found 3 groups:
  - id=42, category=Work, parent=Engineering (id=10)
  - id=87, category=Personal, parent=Home (id=5)
  - id=156, category=Archive, parent=Old Projects (id=30)
Use SCOPE <id> to specify which group.
```

### No match

`SCOPE "nonexistent"` returns: `scope group not found: "nonexistent"`.

### Global / no scope

`SCOPE 0` or omitting SCOPE entirely means no scope filter.

### Circular ownership

The recursive CTE depth cap (50 levels) truncates silently. Circular data is a data problem, not a query error.

### Entity type interaction

All three entity types (resources, notes, groups) have `owner_id`, so scope works uniformly across entity types.

## Shortcode `[mrql]` Scope Support

The `[mrql]` shortcode gains a `scope` attribute:

```
[mrql query='WHERE type = "resource" ORDER BY created LIMIT 5' scope="parent"]
```

### Keywords

| Keyword | Resolution |
|---------|------------|
| `entity` (default) | If context is a group: that group's ID. If resource/note: its `owner_id`. |
| `parent` | The owning group's `owner_id` (one level up). |
| `root` | Walk ownership chain to the top. |
| `global` | No scope filter. |

### Precedence

1. Explicit `SCOPE` clause in the MRQL query string wins.
2. Shortcode `scope` attribute, resolved to a group ID and injected into the parsed AST.
3. Default: `entity` (rendering entity's group).

## Data-Views Plugin Retrofit

### What changes

- The Go-side execution path (`MRQLExecOptions.ScopeID`) now feeds into the MRQL AST scope mechanism instead of a flat `WHERE owner_id = ?`.
- `resolveParentScope`, `resolveRootScope`, `lookupOwnerViaQuerier` in `db_api.go` remain — they resolve keywords to a group ID. The resolved ID is injected into the parsed AST as a `SCOPE`, going through the recursive CTE subtree path.
- The Lua plugin code is unchanged — it still passes `scope` and `scope_entity_id` to `mah.db.mrql_query()`.

### Behavioral change

Scope filtering moves from flat (`owner_id = X`) to tree-based (`owner_id IN subtree(X)`). This is the intended new behavior:
- `entity` scope now returns entities from the group and all its descendants.
- `parent` scope returns the parent group's full subtree.
- `root` scope returns the root group's full subtree (effectively everything in that hierarchy).

## CLI and API

No new flags or API parameters needed. `SCOPE` is part of the query string and flows through naturally. The `mr mrql` help text is updated with `SCOPE` syntax and examples.

Saved queries with `SCOPE` work as-is. Existing saved queries without `SCOPE` are unaffected.

## Documentation Updates

1. **MRQL docs** (docs-site) — add SCOPE clause to syntax reference with examples.
2. **Data-views plugin docs** — update scope description to reflect tree semantics.
3. **Shortcode docs** — document the `scope` attribute on `[mrql]` shortcodes.
4. **`mr mrql` CLI help text** — update usage description and examples.

## Performance

Recursive CTEs are lightweight for typical group hierarchies (hundreds to low thousands of groups). The `owner_id` column is indexed, so `IN (subtree IDs)` is fast even against millions of entities. If performance becomes a concern, subtree IDs can be cached (they only change on group reparenting).
