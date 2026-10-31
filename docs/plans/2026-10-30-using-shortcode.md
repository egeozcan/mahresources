# `[using]`: render a block in another entity's context

Goal: a block shortcode that loads one group, resource or note and renders its
body with that entity as the shortcode context, so `[property]`, `[meta]`,
`[conditional]`, `[each]`, `[link]`, `[mrql scope=…]` and nested `[using]` all
refer to the target.

```
[using rel="owner"]
  <h3>[link][property path=Name][/link]</h3>
  Status: [meta path="status"]
[else]
  <p>No project.</p>
[/using]
```

## Status

Implemented (`shortcodes/using_handler.go`). Where the build differs from the
plan below:

- A `query=` that names no entity type is a lint **warning**, not an error: the
  runtime accepts it and takes the first result of any type.
- The three "verify" items checked out. Query results do carry resolved scope
  IDs (`convertResultItemsWithData` fills them from `data.Scopes`), so
  `[mrql scope=…]` and `[link to=root]` inside the block work. The deferred
  endpoint's subtree check is already covered by `TestDeferredRender_ScopedPrincipal`
  (a validly signed token for an out-of-subtree entity is a 404), and a `[lazy]`
  inside `[using]` can only seal a target the inline render could see; the unit
  test asserts it seals the target's type and ID. Editing through a `[meta]` in
  the block is covered by the E2E spec.
- The confinement API test runs on SQLite only; the Postgres suite needs Docker.
- `[item]` inside a `[using]` inside an `[each]` gets its own lint warning
  rather than relying on the generic one, which did not fire (the `[item]` is
  inside an `[each]`).
- The template generator's partial summary walks only a `[using]` block's
  `[else]` branch and records `id-path` as a path the partial reads.

## Decisions

1. **One entity per block.** `[using]` never iterates. `query=` runs with
   `LIMIT 1`; an `id-path` that resolves to an array is "no target" and renders
   `[else]`. Lists of entities stay with `[mrql]`; a list of IDs stored in Meta
   is the follow-up `[each … using=]` (below).
2. **Targets: group, resource, note.** These are the only entities with Meta,
   a MetaSchema and an owner chain, and the only types MRQL knows
   (`mrql/ast.go`, `EntityType`). Tags and categories are out.
3. **No outer access in v1.** Inside the block every shortcode sees only the
   target. The child context does carry an `outer` pointer from day one (unexported,
   unread) so `ctx="outer"` can be added later without reshaping the struct.
4. **Load through the `QueryExecutor`, never the DB directly.** This is what
   gives `[using]` the request principal's subtree scope, the per-page query
   budget and its cache, and the same "unavailable" behaviour as `[mrql]` on
   surfaces with no executor (share pages). Missing and forbidden both render
   `[else]`, so the block is not an existence oracle.
5. **IDs never reach query text.** The lookup is the fixed query
   `type = <type> AND id = $id` with `$id` bound through `QueryOptions.Params`.
   `<type>` is whitelisted to the three names above. A resolved ID must be a
   positive integer (a Meta number with no fractional part, or a numeric string);
   anything else is "no target".
6. **`rel` mirrors `[link to=]`, not `[mrql scope=]`.** `scope=parent` means the
   owner for a group but the owner's owner for a resource, which is the wrong
   thing to copy. `rel` accepts `owner` and `root` and resolves exactly as
   `resolveLinkURL` does (group → `ParentGroupID`, resource/note →
   `ScopeGroupID`; `0` and `UnresolvedScopeSentinel` are "no target").

## Syntax

`[using <one id source> …]…[else]…[/using]`, block required, `[else]` optional.

| Attribute | Meaning |
|---|---|
| `type` | `group`, `resource` or `note`. Required with `id`, `id-path`, `id-prop`. Not allowed with `rel` (always group) or `query` (the query names its type). |
| `id` | Literal positive integer. |
| `id-path` | Dot-path into the current entity's Meta (`ctx.rawValueAtPath`). |
| `id-prop` | Field on the current entity holding an ID, e.g. `OwnerId`. Read with the `readUintField` helper from `link_handler.go`. Category IDs are not targets, so lint warns when the field name ends in `CategoryId` or `TypeId`. |
| `rel` | `owner` or `root`. |
| `query` | MRQL query; first result is the target. Must name its entity type and must not use `GROUP BY` or aggregates. `param-<name>` and `scope=` behave as on `[mrql]`. Lint warns when there is no `ORDER BY`. |

Exactly one of `id`, `id-path`, `id-prop`, `rel`, `query` is required.

Lookups by ID run with `ScopeGroupID: 0` (global): the `[mrql]` default scope is
a convenience filter, not authorization, and an explicit reference must not be
silently dropped because the target lives outside the carrier's group.
Authorization comes from the principal on `reqCtx`, as for every other query.

## Implementation

### 1. Shared child-context builder (`shortcodes/`)

Extract the struct literal in `renderFlatWithCustom` (`mrql_handler.go`) into
`contextFromItem(item QueryResultItem, parent MetaShortcodeContext) MetaShortcodeContext`.
It also copies `ForceReadOnly` from the parent (the current literal drops it;
harmless today because share pages have no executor, but a child of a read-only
context must stay read-only) and sets the new `outer *MetaShortcodeContext`
field. Use it from `renderFlatWithCustom` and from `[using]`.

Extract the owner/root branch of `resolveLinkURL` into
`relGroupID(rel string, ctx MetaShortcodeContext) (uint, bool)` and use it from
both handlers.

### 2. Handler: `shortcodes/using_handler.go`

`RenderUsingShortcode(reqCtx, sc, ctx, renderer, executor, depth)`:

1. Not a block → error marker (lint already flags it).
2. `body, elseBranch := SplitElse(sc.InnerContent)`.
3. Resolve the target: `(entityType, id)` for the ID sources, or the query for
   `query=`. Unresolvable → render `[else]` with the *current* context.
4. Run the executor (`Limit: 1`). Error → `mrqlErrorHTML(err, false)`, so author
   mistakes in `query=` are visible the way `[mrql]` errors are. Empty result →
   `[else]`.
5. `processWithDepth(reqCtx, body, contextFromItem(items[0], ctx), …, depth+1)`.
   No wrapper element, no CSS prefix, no "No results." placeholder.

Wire-up:
- `parser.go`: add `using` to `shortcodePattern`.
- `processor.go`: dispatch case; with a nil executor emit
  `shortcodeComment("using unavailable in this context")` like `[mrql]`.
- `mrql_slots.go`: `"using": {"else"}` in `slotTags`.

### 3. Lint (`shortcodes/lint.go`)

- Add `using` to `builtinBaseNames`.
- Exactly one ID source; `type` required/forbidden per the table; `type` in the
  whitelist; `rel` in `owner|root`; literal `id` a positive integer.
- `query=` goes through the existing `ValidateMRQL` hook with `Limit: 1`, plus
  an error for `GROUP BY` and warnings for a missing entity type or no `ORDER BY`.
- `[item]` inside a `[using]` inside an `[each]` already renders empty (only
  top-level `[item]`s are marked); the existing "[item] outside [each]" check
  should flag it the same way it flags one inside `[conditional]`. Verify, and
  add a test either way.

### 4. Docs and skills

- `builtin_docs.go` entry (drives the in-app reference and the
  `builtin_docs_consistency_test.go` check).
- `docs-site/docs/features/shortcodes.md` and the custom-templates page.
- `.claude/skills/mahresources-category-designer/references/shortcodes.md`.
- Note the cost: one query per block per render, so `[using]` inside a list-card
  slot is N queries per page. The budget cache keys on query + params, so
  repeated identical lookups (every card pointing at the same root) cost one.

## Things to verify, not assume

- **Deferred rendering.** `[lazy]`/`[details]`/`[reload]` inside `[using]` sign
  the *target's* type and ID. The deferred endpoint reloads through the scoped
  context (`deferred_render_api_handlers.go`), so an out-of-subtree target fails
  closed. Add an API test that a lazy block inside `[using]` on a visible carrier,
  targeting an invisible entity, does not render for a group-confined principal.
- **Editable `[meta]` in the block** posts to the target's ID. Writes are
  authorized server-side, so this is correct, but add an E2E test that the
  widget edits the target and a test that a read-only child stays read-only.
- **Scope keywords inside the block** (`[mrql scope=parent]`, `[link to=root]`)
  resolve against the target. The executor fills the item's scope IDs with the
  unresolved sentinel today (`convertResultItemsWithData`); check whether
  they are resolved later for flat results, and if not, resolve them for the
  single `[using]` item (one entity, so the cost is bounded).

## Tests

Unit (`shortcodes/using_handler_test.go`, fake executor):
- each ID source resolves; bad/fractional/negative/array IDs → `[else]`;
- the executor receives the fixed query with `$id` bound and scope 0, never the
  ID in the text;
- missing and forbidden (executor returns empty) render identically;
- nil executor → comment; executor error → error HTML;
- nested `[using]`, `[using]` inside `[mrql]` and `[each]`, depth cap;
- `ForceReadOnly` and `outer` are carried into the child.

Lint: one test per rule above, plus differential-fuzz corpus entries.

API (SQLite and Postgres): group-confined principal cannot read an
out-of-subtree target through `[using id=…]`, `id-path`, `rel=root` or `query=`,
including through a deferred block.

E2E: a category template using `[using rel="owner"]` renders the owner's name
and Meta on a resource page; editing a `[meta editable]` inside it updates the
owner.

Run: `go test --tags 'json1 fts5' ./...`, the Postgres suite, `cd e2e && npm run
test:with-server`.

## Follow-ups (not in v1)

- **`[each path="…" using="group"]`**: each element is an ID; the body renders
  once per element in that entity's context. Batch the lookup as one
  `id IN (…)` query rather than N.
- **`ctx="outer"`** on `[property]`, `[meta]`, `[conditional]`, reading the
  `outer` pointer. Needs a decision on nesting (`outer` = one level up) and on
  deferred blocks, whose tokens would have to sign the context chain.
