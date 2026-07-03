# MRQL v3 Package 2: Hierarchy Traversal — Design

Implements `ancestors.` / `descendants.` recursive traversal roots (see
`v3-packages.md` §2a). `parent.parent.name` requires knowing the depth;
`ancestors.category = 3` does not.

## Surface

Two new traversal roots, valid on **all** entity types (resource, note, group):

```
ancestors.<groupfield>    descendants.<groupfield>
```

Where `<groupfield>` is a single group field:

- scalar: `ancestors.name = "Archive"`, `descendants.category = 3`,
  `ancestors.id = 42`, `descendants.created > -30d`
- tags: `ancestors.tags = "wip"`, `descendants.tags ~ "arch*"`
- meta subpath: `ancestors.meta.region = "eu"`, `descendants.meta.a.b > 5`

Operators: `=`, `!=`, `~`, `!~`, `>`, `>=`, `<`, `<=` (whatever the leaf field
type supports, same as the existing FK-chain leaves). No further chaining
(`ancestors.parent.name` is rejected), no `IN`, no `IS EMPTY` / `IS NULL`, not
sortable in `ORDER BY`, not usable in `GROUP BY`.

## Semantics (decisions)

The predicate operates on the **group hierarchy** (`groups.owner_id` is the
parent link). For any entity `E`, its *base group* is:

- a **group**: itself
- a **resource / note**: its owner group (`owner_id`)

`ancestors.X` matches `E` iff **some strict ancestor** of `E`'s base group
satisfies `X`. `descendants.X` matches `E` iff **some strict descendant** of
`E`'s base group satisfies `X`. "Strict" = excludes the base group itself.

Consequences:

- A resource directly in "Archive" does **not** match
  `ancestors.name = "Archive"`. Write `owner.name = "Archive" OR
  ancestors.name = "Archive"` for "in Archive or anywhere below it". This keeps
  `ancestors`/`descendants` composable with `owner`/`parent` and matches the
  plain-English reading of "ancestor".

**Negation is relation-style** (like `tags != "x"`), not scalar-style:
`ancestors.category != 3` means *"no strict ancestor has category 3"*. Owner-less
resources/notes (no base group) trivially satisfy a negated predicate.

## Translation

Reduce both directions to: compute the set of matching groups `M` (from the leaf
comparison), expand it transitively through the hierarchy, then test the
entity's base-group column against the expanded set.

Let `M = SELECT id FROM groups WHERE <positive leaf clause>` (for tags, the
group_tags junction subquery; for meta, the JSON expr). The leaf clause is
always built **positive** — negation is applied at the outer `IN`/`NOT IN`.

- `ancestors.X` → matched groups = **strict descendants of `M`**. Seed the
  recursion with the direct children of `M`, walk down via `owner_id`.
- `descendants.X` → matched groups = **strict ancestors of `M`**. Seed with the
  direct parents of `M`, walk up via `owner_id`.

The expansion is a `WITH RECURSIVE` CTE nested inside the WHERE subquery (both
SQLite and Postgres allow `WITH` at the head of a subquery), mirroring
`scope.go`'s depth-bounded pattern (`depth < 50`, guards against cyclic
`owner_id` data):

```sql
-- ancestors.X : strict descendants of M
<outer> [NOT] IN (
  WITH RECURSIVE _mrql_anc(id, depth) AS (
    SELECT g.id, 0 FROM groups g WHERE g.owner_id IN (<M>)
    UNION ALL
    SELECT c.id, a.depth + 1 FROM groups c
      JOIN _mrql_anc a ON c.owner_id = a.id WHERE a.depth < 50
  ) SELECT id FROM _mrql_anc
)

-- descendants.X : strict ancestors of M
<outer> [NOT] IN (
  WITH RECURSIVE _mrql_desc(id, depth) AS (
    SELECT g.owner_id, 0 FROM groups g
      WHERE g.id IN (<M>) AND g.owner_id IS NOT NULL
    UNION ALL
    SELECT p.owner_id, d.depth + 1 FROM groups p
      JOIN _mrql_desc d ON p.id = d.id WHERE p.owner_id IS NOT NULL AND d.depth < 50
  ) SELECT id FROM _mrql_desc
)
```

`<outer>` is `groups.id` for a group entity, else `<table>.owner_id`. When
negated (`NOT IN`) and the entity is a resource/note, append
`OR <table>.owner_id IS NULL` so owner-less rows match. Sibling subqueries each
get their own CTE scope, so multiple ancestors/descendants predicates in one
query never collide on the `_mrql_anc`/`_mrql_desc` names.

## Touch points

- `mrql/translator.go`: `recursiveRoots` set + `translateRecursiveComparison`,
  routed from `translateComparisonExpr` before the FK-chain routing. Reuses
  `buildScalarClause`, the tag junction clause, and the `meta*On(alias, …)`
  helpers.
- `mrql/validator.go`: `recursiveRoots` recognized as roots on all entities;
  `validateRecursiveChain` for the leaf; recursive roots rejected for `IN`,
  `IS`, `ORDER BY`, and skipped in `validateValueType`.
- `mrql/completer.go`: suggest `ancestors.`/`descendants.` roots and their
  leaf group fields.
- `application_context/mrql_generation.go`: prompt rule + example mappings.
- Docs: `.claude/skills/mahresources-cli/references/mrql.md` and any
  generated CLI docs.

## Explicitly out of scope (v1)

- Chaining recursive with FK steps (`ancestors.parent.x`, `owner.ancestors.x`).
- `ancestors`/`descendants` as GROUP BY keys or ORDER BY keys.
- `IN` / `IS EMPTY` on recursive roots.
