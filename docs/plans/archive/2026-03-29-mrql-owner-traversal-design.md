# MRQL Owner Traversal & Multi-Level Chaining

**Date:** 2026-03-29
**Status:** Approved

## Problem

MRQL cannot filter resources or notes by properties of their owner group. For example, "resources tagged 'x' whose owner has tag 'y'" is not expressible. Additionally, traversal is limited to one level (e.g., `parent.name`) with no support for chaining (e.g., `owner.parent.name`).

## Solution

Add `owner` as a traversal field on resources and notes. Generalize the existing parent/children FK traversal into a reusable helper that supports multi-level chaining up to 5 parts deep.

## Design

### New Field: `owner`

Add `owner` (FieldRelation, column `owner_id`) to `resourceFields` and `noteFields` in `fields.go`. This enables:

- Direct comparison: `owner = "Group Name"`, `owner ~ "Project*"`
- Traversal: `owner.name`, `owner.tags`, `owner.category`, etc.
- Chaining: `owner.parent.name`, `owner.parent.tags`, etc.

### Multi-Level Traversal

Dotted fields support up to 5 parts. Each part in the chain is classified:

| Position | Role | Valid values |
|----------|------|-------------|
| Root (first) | Entry point | `owner` (resources/notes), `parent`/`children` (groups) |
| Intermediate | FK step | `parent`, `children` only (always targeting groups) |
| Leaf (last) | Queried field | Scalar group fields (`name`, `description`, `category`, `id`, `created`, `updated`) or `tags` |

Rules:
- `owner` is only valid as root, not as an intermediate (it's not a group field).
- After the first step you're always in "groups" context, so only `parent`/`children` are valid intermediates.
- `meta.*` is not supported as a leaf (would require an extra dot level).
- Maximum depth: 5 parts (4 traversal steps + 1 leaf).

### Generalized FK Helper

Extract a reusable traversal helper from the existing parent/children code. The helper models each step as:

```go
type fkStep struct {
    fkExpr    string // source FK, e.g. "resources.owner_id"
    selectCol string // subquery SELECT col: "id" (forward) or "owner_id" (reverse)
    alias     string // subquery alias, e.g. "t0"
}
```

Each traversal field maps to a step direction:

| Field | Direction | fkExpr pattern | selectCol |
|-------|-----------|----------------|-----------|
| `owner` | forward | `<table>.owner_id` | `<alias>.id` |
| `parent` | forward | `<table>.owner_id` | `<alias>.id` |
| `children` | reverse | `<table>.id` | `<alias>.owner_id` |

The translator walks the chain outside-in, building nested subqueries. Example for `owner.parent.tags = "active"`:

```sql
resources.owner_id IN (
  SELECT t0.id FROM groups t0 WHERE t0.owner_id IN (
    SELECT t1.id FROM groups t1 WHERE t1.id IN (
      SELECT gt.group_id FROM group_tags gt
      JOIN tags t ON t.id = gt.tag_id
      WHERE LOWER(t.name) = LOWER(?)
    )
  )
)
```

### Negation / NULL Handling

Only the outermost step gets `OR fkColumn IS NULL` for negated operators (`!=`, `!~`). This matches existing behavior where `parent != "X"` includes root groups (no parent), and `owner != "X"` includes entities with no owner. Inner steps don't add NULL handling.

### Refactoring Scope

The existing `translateParentComparison`, `translateChildrenComparison`, `translateTraversalFieldComparison`, and `translateTraversalTagComparison` are refactored to use the chain builder. Single-step chains produce identical SQL to the current implementation.

## Changes Per File

| File | Change |
|------|--------|
| `mrql/fields.go` | Add `owner` (FieldRelation, `owner_id`) to resourceFields and noteFields |
| `mrql/parser.go` | Remove 2-part dotted field cap, allow up to 5 parts |
| `mrql/validator.go` | Chain validation: root type check, intermediate check, leaf check; allow `owner` on resources/notes |
| `mrql/translator.go` | Extract `fkStep`/chain builder; refactor parent/children to use it; add owner routing |
| `mrql/completer.go` | Suggest `owner` + `owner.*` for resources/notes; suggest `parent`/`children` after intermediate steps |
| `mrql/translator_test.go` | Unit tests for single/multi-level traversal, negation, NULL handling |
| `mrql/validator_test.go` | Validation tests for valid/invalid chains |
| `mrql/completer_test.go` | Completer tests for owner suggestions |
| `e2e/tests/cli/cli-mrql.spec.ts` | CLI E2E tests for owner traversal queries |
| `e2e/tests/` | Browser E2E test for the core use case |
| `docs-site/docs/features/mrql.md` | Document owner field, multi-level traversal, updated field tables |
| `templates/mrql.tpl` | Update inline docs with owner field and traversal examples |

## Example Queries

```
-- Original use case: resources tagged "x" whose owner has tag "y"
type = resource AND tags = "x" AND owner.tags = "y"

-- Direct owner match
type = resource AND owner = "Project Alpha"
type = note AND owner ~ "Sprint*"

-- Owner field traversal
type = resource AND owner.category = "3"
type = resource AND owner.name = "Archive"

-- Multi-level chaining
type = resource AND owner.parent.name = "Acme Corp"
type = resource AND owner.parent.tags = "active"
type = note AND owner.children.name ~ "Q*"

-- Existing parent/children now support chaining too
type = group AND parent.parent.name = "Root"
type = group AND parent.parent.tags = "org-level"

-- Negation includes NULL (entities with no owner)
type = resource AND owner != "Drafts"
```

## Validation Error Examples

```
owner.owner.name        → "owner is not valid as an intermediate traversal field (only parent/children)"
parent.groups.name      → "groups is not a traversal field"
owner.parent.meta.key   → "meta fields require dotted key access; not supported in traversal chains"
a.b.c.d.e.f             → "traversal chain too deep (max 5 parts)"
```
