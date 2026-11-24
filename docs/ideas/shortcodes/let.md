<!-- Shortcode idea that did not make the first implementation round. -->
> **Status:** idea, not implemented. The plan went through 3 plan/evaluate round(s).
> Last evaluation: consistency 7.8/10, usefulness 7.8/10, average 7.80/10. The bar was 8.5.
>
> **Pitch:** An authoring-ergonomics pair: [let name="open" mrql="type = note AND meta.status = 'open'" value="count"] or [let name="price" path="meta.price" default="0"] computes a value once per render, and [var name="open"] prints it anywhere later in the same slot, including inside [conditional var="open" gt="0"]. Today a number shown in the header and tested in a conditional costs two identical queries, and copy-paste drifts between them. [let] stores the value per render with a cap on the number of variables, and lint flags uses of undefined variables.

# Plan: `[let]` / `[var]`, variables scoped to one template render (v4)

## 0. Changes from v3 (one entry per evaluator point)

1. **Usefulness, and the per-item exclusion. Accepted; the design changes.**
   - **`[mrql]` block bodies now read the outer scope (read-only).** When an `[mrql]` block has a per-item body (`slots.Item`), every item card it renders gets a fresh scope whose **parent** is the scope of the template that contains the block. `[var]`, `[conditional var=]` and `-var` operands look a name up in their own scope first and then in the parent. `[let]` writes only to the item's own scope.
   - Category-level `CustomMRQLResult` and `CustomCSS` stay isolated. They are another category's template, so the author of this template cannot know their variable names.
   - This is safe because the outer values were computed in the same request, under the same principal and executor, before the block ran. The item card is shown on the same page to the same user.
   - Motivation (§1) now names the overlap with `QueryBudget` honestly. Identical repeats are already free, so `[let]` does not save queries for careful authors. It adds three things:
     1. one place to edit a repeated value;
     2. per-entity thresholds (`gt-var`);
     3. comparing each `[mrql]` result item against a value computed outside the block, which no existing shortcode can express.
2. **Hoisting over-inclusion: fixed.** `hoist` no longer matches byte substrings. It tokenizes the body with `matchTokensM` and scans `[elseif]` dividers to collect the names the body **references** (§4.3). Only those definitions are prepended. Nested deferred bodies are part of the same token scan, so transitive references are collected too. A definition that is not referenced never reaches a token, so it costs no query on the deferred fetch.
3. **Prose that contains `=`: fixed.** A `[let …]`/`[var …]` token counts as a shortcode **only when it has a `name` attribute**:
   - Without one (`[let it be]`, `[let me know if x=1]`, `[var foo=bar]`), it renders `sc.Raw` and lint ignores it entirely, `Required` included.
   - With a `name` that is invalid, it renders `sc.Raw` and lint reports an **error**. Writing `name=` is a strong sign that a shortcode was meant.
   - §5 adds a compatibility note for existing text that contains `[let name=…]`/`[var name=…]`.
4. **`ProcessWithDiagnostics`: now specified.**
   - It records only what it records today: executor errors (through its executor wrapper) and renderer errors.
   - The marker cases are not recorded: nested `[let]`, over the cap, no source, invalid name. This matches every other `shortcodeErrorMarker` today. These cases are lint's job and are caught before save.
   - The test asserts both halves: an executor error from `[let mrql]` appears in `Errors`, and a nested `[let]` does not.
5. **`internal/templateeval/score.go` and `render_budget.go`: now stated (§7).**
   - `score.go`'s `shortcodeShaped` already accepts any identifier, so it needs no change. Its comment ("mirrors shortcodePattern…") stays true.
   - `CheckGeneratedSource` (`render_budget.go:366`) and `matchTokensM`'s `budgetedTokenCap` now count `[let `/`[var ` tokens, the nameless prose ones included. That is the intended, conservative direction: each token is charged once, as every other token is.
6. **`[using]` with a deferred body inside it. Pushed back on the design; now documented, linted and e2e-tested.**
   - The suggested fix (the inline path inherits `ctx.vars` read-only) would make the inline output differ from the deferred output. The deferred endpoint cannot see the outer scope: it only has the sealed text, and hoisting a `path`/`field`/`scope="entity"` definition into a body that renders for another entity would rebind it to that entity.
   - Inline and deferred output staying identical is the stronger property, so both paths see nothing there.
   - Docs show the case with an example. Lint warns on it with a specific hint. The e2e test asserts that it renders empty.
7. **Minor points.**
   - The `-var` operators are limited to eq/neq/gt/gte/lt/lte. `in-var`, `contains-var` and `matches-var` are not in the `conditional` DocAttrs and none of those attrs is a wildcard, so the existing "unknown attribute" warning fires on them (`lint.go:381-386`). A lint test asserts this.
   - New test: a `[let]` inside a `[lazy]` body that redefines a hoisted name. The body's definition wins for the uses after it, because hoisted text comes first.
8. **Caching of hoisted queries: now specified (§4.3, §6).**
   - Inline, the hoisted text is the outer `sc.Raw` byte for byte.
   - The entity check guarantees the same `ctx`, so `ScopeGroupID`, `ParentGroupID` and `RootGroupID` are equal and `resolveScopeKeyword` gives the same scope ID.
   - The `QueryBudget` key (query, options, limit, buckets, scope, params) is therefore identical and the lookup hits.
   - The existing "0 extra executor calls" test proves this.

## 1. Motivation and user value

Template authors often need one computed value in several places. Today they repeat it:

```
Open: [mrql query='type = note AND name ~ "open"' value="count" limit="100"]
[conditional mrql='type = note AND name ~ "open"' limit="100" gt="0"]…[/conditional]
```

**What is already solved.** `QueryBudget` serves byte-identical repeats from its cache, so a careful author pays for one query. `[let]` does not save queries in that case. Its first benefit is simply one place to edit: if the copies drift apart (one has `limit`, one does not), they disagree and cost two queries.

**What existing shortcodes cannot express at all.**

(a) **A per-entity threshold.** A project category wants a "Over WIP limit" badge when a group's open notes exceed that group's own `wip_limit` meta value:

```
[let name="open" mrql='type = note AND name ~ "open"' limit="500"]
[let name="cap" path="wip_limit" default="10"]
Open: [var name="open"] / [var name="cap"]
[conditional var="open" gt-var="cap"]<strong>Over WIP limit</strong>[/conditional]
```

Today `gt=` takes only a literal, and that literal lives in the category template, so it is the same for every group.

(b) **Comparing each result item against a value from outside the block.** This flags each note whose estimate exceeds the group's own per-item budget:

```
[let name="cap" path="max_estimate" default="8"]
[mrql query='type = note' limit="50"]
  [property path="Name"] [conditional path="estimate" gt-var="cap"]<strong>Over budget</strong>[/conditional]
[/mrql]
```

Inside an `[mrql]` item body, the context is the item, so `path=` reads the item's meta. The outer entity's `max_estimate` cannot be reached from inside the body today.

Not re-proposed: nothing in `docs/ideas/category-templates-and-shortcodes.md` adds variables. `[using]`, `[each]`/`[item]` and `[partial]` are used without changes.

## 2. Where it lives: a built-in Go handler

- Values must be shared across shortcodes in one render (`[let]` → `[var]`/`[conditional]`), and the `[mrql]` and deferred handlers must be able to read the scope. Plugin shortcodes are separate Lua calls with no shared state, and they cannot extend `[conditional]`.
- It reuses `resolveConditionalValue`, `extractScalarFromResult` and `formatJSONScalarObserved`, so the source semantics match `[conditional]` and `[mrql value=]` by construction.
- The lint needs block structure, which only `shortcodes/lint.go` sees.

## 3. Syntax and attributes

### `[let]`: inline only, renders nothing

- `let` must be followed by whitespace.
- There is no closing form: `[/let]` is plain text.
- The token is a shortcode only when a `name` attribute is present (§0.3).

| Attribute | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `name` | string `^[A-Za-z][A-Za-z0-9_]{0,31}$` | yes | — | Variable name, case-sensitive. |
| `mrql` | MRQL string | one of mrql/field/path | — | Same as `[conditional mrql=]`: the flat item count (capped by `limit`), the bucket count, or `Rows[0][aggregate]`. |
| `field` | string | one of | — | A top-level entity field, as in `[conditional field=]`, including the `credentialFields` withholding. |
| `path` | string | one of | — | A meta dot-path, as in `[conditional path=]`. The JSON type is kept. |
| `aggregate` | string | required for aggregated results | — | Column to read. Without it, the stored error is the existing "mrql aggregated results require aggregate=…". |
| `scope` | `entity`/`parent`/`root`/`global` or a numeric group ID | no | `entity` | Via `resolveScopeKeyword`. |
| `limit` | number | no | `20` | MRQL limit. |
| `buckets` | number | no | `5` | MRQL bucket count. |
| `param-*` | string, wildcard | no | — | `$name` bindings (`collectShortcodeParams`). |
| `default` | string | no | — | Stored when the resolved value is `nil` or `""`. It does not apply to errors or to an unavailable source. |

- When several sources are given, priority is mrql > field > path (the resolver's existing order). Lint warns in that case.
- The attribute is `aggregate`, not the pitch's `value=`, because the resolver already uses `aggregate`.

### `[var]`: inline only, prints an escaped bare value

| Attribute | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `name` | same pattern | yes | — | Variable to print. |
| `format` | `date`/`datetime`/`time`/`filesize` | no | — | As in `[mrql value=]`. |
| `layout` | Go time layout | no | — | Wins over `format`. |

- There is no `default` and no `raw`: the default lives on `[let]`, so the printed value and the tested value never differ.
- Inside `[each]`, every iteration prints the same value. A variable is per scope, not per item.

### `[conditional]` / `[elseif]` additions

- **`var` (and `var2`, `var3`, …): a condition source.**
  - The tested value is the stored value with its type kept. All existing operators apply.
  - An undefined name tests as `nil`: `empty` is true and `gt` is false.
  - `var` wins over mrql/field/path, and lint warns when they are combined.
- **`eq-var`, `neq-var`, `gt-var`, `gte-var`, `lt-var`, `lte-var` (with numbered suffix, e.g. `gt-var2`): operand variables.**
  - The operand is `fmt.Sprint(stored value)`, then compared by the existing `evaluateOperatorM` for that op. Numeric ops use `toFloat` as today.
  - They work with any source (`path`, `mrql`, `var`, …).
  - A literal op and its `-var` twin may both be present. Each counts as one more operator under `combine`.
  - An operand variable that is undefined or `nil` makes that operator **false**, including for `neq`.
  - An operand variable that holds an error makes the whole condition return that error, through the existing error/unavailable rendering.
  - No `in-var`, `contains-var` or `matches-var`. Lint reports them as unknown attributes.

## 4. Scoping rules

### 4.1 Scope kinds

A scope is one template rendered for one entity. `letScope` has an optional read-only `parent`.

1. **A top-level `Process`/`ProcessChecked` call**, i.e. one slot. Header, sidebar and each list card's summary are separate scopes. No parent.
2. **A deferred body** (`[lazy]`, `[details]`, `[autorefresh]`), on both the deferred and the inline path. It starts from the hoisted definitions of §4.3. No parent.
3. **An `[mrql]` block's per-item body**, one scope per item card.
   - The parent is the scope of the template that contains the `[mrql]` token, at the moment the block renders.
   - Lookups fall back to the parent, and through it to further ancestors (nested `[mrql]` blocks), so a chain is at most `maxRecursionDepth` long.
   - Definitions made earlier in the outer scope are visible. Definitions the outer scope makes after the block are not visible, because the block has already rendered.
4. **Category-level `CustomMRQLResult` cards and the `renderResultCSS` `childCtx`.** Own empty scope, no parent. These are templates of other categories.

### 4.2 Rules

- **`[let]` only at the top level of its scope.**
  - Inside any block it is an error: conditional branches, `[each]`, `[link]`, `[reload]`, `[using]`, a plugin block, partial content, plugin output, or an `[mrql]` `[header]`/`[footer]`/`[else]`.
  - It renders `shortcodeErrorMarker("let", "[let] must be at the top level of its template, not inside another block")` and defines nothing.
  - Detection: every block handler recurses at `depth+1`, and a scope records the depth it was created at. A `[let]` is accepted only when `ctx.vars.depth == depth`.
  - The top level of an `[mrql]` item body is its own scope (kind 3), so `[let]` is allowed there.
- **Visibility.**
  - `[var]`, `[conditional var=]` and `-var` operands see definitions made **earlier in document order** in their scope, then in the parent chain.
  - The scope is shared by conditional branches, `[each]` bodies, link/reload faces, partial content and plugin output (through `ctx` copies). It is also shared by `[using]` bodies (`child.vars = ctx.vars`) and the `[mrql]` header/footer/else (rendered with the parent ctx).
- **Redefinition** in the same scope replaces the value for later uses and does not count again toward the cap. A definition in an item scope shadows the parent's name for that card only.
- **Cap.** At most 32 distinct names stored per scope, hoisted names included and parent names excluded. The 33rd renders the marker "too many variables in this template (limit 32)" and defines nothing.

### 4.3 Hoisting into deferred bodies

`letScope` records:
- for each name, the `sc.Raw` of its latest definition, and the definition order;
- the entity (type, id) the scope was created for.

When a deferred handler is about to sign a body, or to render it inline, it computes `prefix := ctx.vars.hoist(reqCtx, ctx, body)`:

1. **Entity check.** It returns `""` unless `ctx.EntityType/EntityID` equals the scope's entity. Inside `[using]` they differ: a `path`/`field`/`scope="entity"` definition would mean another entity there.
2. **Collect references.**
   - Run `matchTokensM(m, body)`. For each opening token, collect:
     - `name` on `var`;
     - `var`+digits and `<op>-var`+digits on `conditional`.
   - Then run `elseifAnywherePattern` (`\[elseif\s+([^\]]*)\]`, unanchored, same attribute grammar) over the body with `m.tick()` per match, and collect the same attributes from each divider through `parseAttrsReserve`.
   - Tokens inside nested deferred bodies and nested blocks are in the same scan, so transitive references are covered.
3. **Pick definitions.** Include each referenced name that has a definition **in this scope's own map**. Parent-chain definitions are never hoisted, because their entity is another entity. Order them by definition order and join them.
   - A referenced name that the body also redefines is still hoisted. It costs nothing inline; the body's later definition wins for the uses after it.
4. All scanning is charged to the meter. When the meter stops, the prefix is `""`.

The handler passes `prefix + body` to the signer and to the inline `processWithDepth`. The inline call uses `ctx.vars = &letScope{depth: depth+1, entityType, entityID}` with no parent, so both paths evaluate the same text in a fresh scope.

Consequences:

- **No value ever enters a token.** The deferred request recomputes each value with its own principal, `ScopeGroupID` and executor, so subtree confinement holds by construction.
- **Inline cost: zero extra queries.** The text is byte-identical and `ctx` is the same, so the `QueryBudget` key is identical and the lookup hits.
- **Deferred cost.** The fetch runs one query per **referenced** `[let mrql]` definition. Unreferenced definitions are never sealed. `[autorefresh]` re-evaluates them on each refresh, which is what a refresh is for.
- **Trust.** Hoisted text is top-level text of the same template, so it renders under the body's `UntrustedSource` flag, which can only be stricter.
- **Attributes are not expanded.** `[details summary="Open ([var name='open'])"]` prints the text literally, as for any attribute today. Docs recommend putting the value in the body's first line, or next to the `[details]`.
- **Inside `[using]`, deferred bodies see no outer variables, on both paths.** Documented, linted (§8 rule 6 hint), e2e-tested.

## 5. Rendering and bad input

| Situation | `[let]` | `[var]` | `[conditional var=]` / `-var` operand |
|---|---|---|---|
| OK | `""` | `html.EscapeString(formatJSONScalarObserved(ctx, v, format, layout))` | stored value |
| Undefined, or used before its `[let]` | — | `""` | source: `nil`; operand: op false |
| `default` and value nil/`""` | stores default | prints it | tests it |
| Executor error | `""`, stores err | `mrqlErrorHTML(err, true)` | existing error `<div>` |
| `mrql` with no executor (share page) | `""`, stores `NewQueryUnavailable("")` | `unavailableComment(err)`, checked with `errors.Is(err, ErrQueryUnavailable)` before `mrqlErrorHTML` | `unavailableComment` (existing branch) |
| No source | marker "[let] needs an mrql, field or path attribute" | — | — |
| No `name` attribute (`[let it be]`, `[var foo=bar]`) | `sc.Raw` | `sc.Raw` | — |
| `name` present but invalid | `sc.Raw` | `sc.Raw` | source/operand undefined |
| Nested / over the cap | marker | — | — |
| Render budget stopped | `""` | `""` | existing |

- Today `[conditional mrql=]` with no executor skips the mrql source. Storing an unavailable error is new `[let]` behaviour, and the docs describe it that way.
- Emitting `sc.Raw` is as safe as leaving the text alone: it is a substring of the already-sanitized input.
- **Compatibility.** Existing Descriptions and note text are also shortcode-expanded. Text that already contains `[let name=…]` or `[var name=…]` changes meaning: a `[var name=…]` renders `""`, and a `[let name=…]` renders nothing or a marker. Text without `name=` is unaffected. Requiring `name=` makes accidental matches unlikely. The docs changelog line states the change.
- **`ProcessWithDiagnostics`** records executor and renderer errors only, as today. Marker cases are not recorded.
- **Accessibility.**
  - `[let]` emits nothing.
  - `[var]` emits plain escaped text with no wrapper, no roles and no focusable elements. That is safe inside `aria-label` and `title`.
  - Failures reuse the existing markers: an inline span with visible text, and HTML comments that assistive technology ignores.
  - The e2e test runs axe.

## 6. Authorization and performance

**Authorization.**
- `path`/`field` read the render's own entity. `field` keeps the credential withholding.
- `mrql` uses the render's `QueryExecutor`, `ScopeGroupID`, principal-bound DB and actor-keyed cache. A subtree-confined user sees only subtree rows, and an out-of-subtree `SCOPE` fails closed, as for every executor call.
- Parent-scope reads in `[mrql]` item bodies expose only values already computed for the same user in the same request.
- Values live only on a per-render struct reached through `MetaShortcodeContext`. They are never persisted, never shared across requests, never sealed.

**Performance (millions of resources).**
- Each `[let mrql]` is one executor call, charged to the page `QueryBudget` and the MRQL result cache. The default limit is 20 and it never sets `WantTotal`.
- `[var]`, `-var` operands and `[conditional var=]` are map lookups along a chain of at most `maxRecursionDepth` scopes.
- `path`/`field` read the shared decoded-meta memo.
- Top-level-only means at most one evaluation per definition per scope. In an `[mrql]` item body that is per card, bounded by the block's `limit`. The common case, a definition outside the block read by every card, runs **once**, where today a per-card `[conditional mrql=]` runs per card.
- Hoisting adds one bounded token scan of the body, metered.
- Memory: at most 32 entries per scope, plus `sc.Raw` strings already in memory.
- A Process call renders on one goroutine (`QueryBudget`'s mutex guards only the cross-render cache), so `letScope` has no lock. A comment says so.

## 7. Files to change

**New `shortcodes/let_handler.go`**
- Types:
  - `letValue{value any; err error; raw string}`;
  - `letScope{depth int; entityType string; entityID uint; parent *letScope; vars map[string]letValue; order []string}`.
- Functions and constants:
  - `lookup(name) (letValue, bool)`: nil-safe, own map first, then parent;
  - `hoist(reqCtx, ctx, body) string`, as in §4.3;
  - `referencedLetNames(m, body) map[string]struct{}`, shared with lint;
  - `elseifAnywherePattern`, `letNamePattern`, `maxLetVariables = 32`.
- `RenderLetShortcode(reqCtx, sc, ctx, executor, depth)` checks, in order:
  1. no `name`, or an invalid name → `sc.Raw`;
  2. `ctx.vars == nil || ctx.vars.depth != depth` → nested marker;
  3. no source → marker;
  4. cap (own map only) → marker;
  5. mrql and nil executor → store unavailable;
  6. otherwise `resolveConditionalValue(reqCtx, sc.Attrs, "", ctx, executor, collectShortcodeParams(sc.Attrs))`;
  7. apply `default`;
  8. `!m.alive()` → `""`;
  9. store with `raw: sc.Raw`, return `""`.
- `RenderVarShortcode(sc, ctx)` follows the table in §5.

**`shortcodes/meta_handler.go`**
- Add unexported fields `vars *letScope` and `inheritVars *letScope`.
- Doc comment:
  - copies share the `vars` pointer, like `decodedMeta`;
  - `inheritVars` is read only when a new scope is created;
  - the four scope kinds.

**`shortcodes/processor.go`**
- In `processWithDepth`, after the `decodedMeta` init:
  `if ctx.vars == nil { ctx.vars = &letScope{depth: depth, entityType: ctx.EntityType, entityID: ctx.EntityID, parent: ctx.inheritVars} }`.
- Add dispatch cases for `let` and `var`.

**`shortcodes/parser.go`**
- In `shortcodePattern`, add `let\s|var\s` to the name group (one required whitespace).
- `Parse` and `matchTokensM` read the name through a new `shortcodeNameAt(input, mm)` helper that trims trailing whitespace.
- `closingTagPattern` is unchanged. Update the comments: no `[/let]`/`[/var]`.

**`shortcodes/conditional_handler.go`**
- In `evaluateConditionSet`:
  - when `var`+suffix is set, use `lookup`'s value, its error, or `nil`, instead of `resolveConditionalValue`;
  - look up each `<op>-var`+suffix. A stored err is returned. The resolved operands go to `evaluateConditionM` as a map.
- In `evaluateConditionM`, after the literal-op loop, loop over `varOperators = {eq,neq,gt,gte,lt,lte}`. A missing or nil operand gives false; otherwise call `evaluateOperatorM(m, value, str, op, fmt.Sprint(operand))`.
- `hasConditionSource` accepts `var`+suffix.

**`shortcodes/mrql_handler.go`**
- In `RenderMRQLShortcode`, when `itemTemplate != ""`: `itemParent := ctx; itemParent.inheritVars = ctx.vars`. Pass `itemParent` only to `renderFlat`/`renderBucketed`. Header, footer, else and CSS keep `ctx`.
- `renderFlatWithCustom`: `ictx := contextFromItem(item, parentCtx); ictx.inheritVars = parentCtx.inheritVars`.
  - This is non-nil only on the block-body path, where `applyBlockTemplate` has set every item's template to the block body.
  - `contextFromItem` itself is unchanged, so `[using]` (which also calls it) and CSS are unaffected.
- Comment: `renderResultCSS` `childCtx` and category `CustomMRQLResult` stay isolated.

**`shortcodes/lazy_handler.go`, `details_handler.go`, `autorefresh_handler.go`**
- Compute `body := ctx.vars.hoist(reqCtx, ctx, sc.InnerContent) + sc.InnerContent`.
- Use `body` for the signer and the inline `processWithDepth`. The inline call gets `ctx.vars = &letScope{depth: depth+1, entityType: ctx.EntityType, entityID: ctx.EntityID}`.
- The deferred endpoints are unchanged.

**`shortcodes/using_handler.go`**
- `child.vars = ctx.vars`.

**`shortcodes/mrql_slots.go`**
- Add `HeaderSpan`, `FooterSpan`, `ElseSpan [2]int`. They are byte ranges in the original `inner`, `{-1,-1}` when absent, and computed inside `parseMRQLSlotsM`:
  - the header span comes from `firstSlotSpanM` on the original string;
  - the footer and else offsets are mapped back across the removed ranges.

**`shortcodes/builtin_docs.go`**
- Add `let` and `var` entries (`BlockNo`) with valid-MRQL examples.
- Add `conditional` DocAttrs `var` and the six `-var` operators, declared like `gt`.
- Add the WIP and per-item examples from §1.

**`shortcodes/lint.go`**
- The `[conditional]` source message becomes "needs a path, field, mrql, or var attribute".
- The operator-presence check accepts `-var` ops.
- Add `case "let"` to the `ValidateMRQL` switch, using `mrqlCheckOptions(tk.attrs, "")`.
- Skip every diagnostic for let/var tokens with no `name` attribute.
- New rules (§8).
- `builtinBaseNames` is unchanged. `nearMissBuiltin` gets the comment "`let`/`var` are deliberately absent: three-letter names would match too much prose at edit distance 1."

**`shortcodes/lint_attr_context.go`**
- `emitsBareValue`: `case "var": return true`.

**No change, verified**
- `internal/templateeval/score.go`: `shortcodeShaped` already accepts any identifier.
- `render_budget.go`: `CheckGeneratedSource` counts the new tokens through `shortcodePattern`, as intended.

**Templates and editor**
- Add `[let …]`/`[var …]` lines to the reference panel in `templates/createCategory.tpl`, `createResourceCategory.tpl` and `createNoteType.tpl`.
- `src/components/shortcodeCompletion.js`: add `'let'` to the path-completion list.

## 8. Lint rules

- All rules are bounded by `w.step()`/`w.charge()`.
- Let/var tokens without a `name` attribute are ignored.

**Scopes.**
- Regions:
  - the inner spans of matched `[lazy]`/`[details]`/`[autorefresh]` blocks (deferred regions);
  - each matched `[mrql]` block's inner span minus `HeaderSpan`/`FooterSpan`/`ElseSpan` (item regions).
- A token's scope key is the start of its innermost region, or `-1`, via `innermostSpanStart`.
- One forward pass builds `map[scopeKey]map[name]firstDefStart`.
- Visibility:
  - **Deferred region**: its own names, plus the names its enclosing scope defined before the region start. It inherits nothing when a matched `[using]` inner span inside that enclosing scope contains the region.
  - **Item region**: its own names, plus everything visible in the enclosing scope before the `[mrql]` token start. Item regions inherit through the chain; deferred regions do not inherit chain names, only the immediately enclosing scope's own names.

**Rules.**
1. A `name` present but invalid → **error** "name must start with a letter and use only letters, digits and _ (max 32)".
2. `[let]` with no source → **error**. With several sources → **warning** "only mrql is used" / "only field is used". `[conditional]` with `var` plus another source → **warning** "only var is used".
3. A `[let]` whose innermost matched enclosing block is not its scope's opening block, or that sits in an mrql header/footer/else span → **error** (top-level message).
4. `opts.PartialName != ""` and a `[let]` is present → **error** "[let] cannot be used in a partial: the partial renders inside the template that includes it".
5. The 33rd distinct name defined in one scope (hoisted names inherited by a deferred region included, item-region parent names excluded) → **error**.
6. A reference with no visible definition → **warning**.
   - References are `[var name]`, and `var`/`varN`/`<op>-var[N]` on `[conditional]` and `[elseif]`. Elseif attrs are read where the existing elseif MRQL check reads `branch.Attrs`.
   - Message: "renders nothing", "tests an empty value", or "makes this comparison false".
   - When a definition exists elsewhere, add a hint:
     - in a deferred region inside `[using]`: "a [lazy]/[details]/[autorefresh] body inside [using] renders for another entity and sees no outer variables; define it inside the body";
     - otherwise: "variables are not visible before their [let], or in category card templates".
   - The rule is skipped for partials.
7. Existing unknown-attribute check: `in-var`, `contains-var` and `matches-var` warn. No new code is needed.

## 9. Tests

**`shortcodes/let_handler_test.go` (new)**
- **Basics**
  - `path` let plus `[var]`. `default` applies to missing and to `""`, but not to `0`. `[var format="date"]`.
  - `mrql` let with a counting stub executor: one `[let]`, two `[var]` and one `[conditional var gt]` give exactly 1 executor call, and all agree.
  - `[conditional var=]` with eq/gt/empty, `var2` and `[elseif var=]`.
- **`-var` operands**
  - The WIP example is true when 12 > 10 and false when 8 > 10.
  - An undefined operand makes `neq-var` false.
  - An error operand renders the error div.
  - `gt` and `gt-var` together fold under `combine="any"`.
- **Undefined and nesting**
  - Undefined and use-before-define render `""`.
  - A nested `[let]` gives the marker and leaves the name undefined in each of: conditional, each, link, reload, using, partial content (`WithPartialResolver`), plugin output (renderer stub), mrql header.
- **`[mrql]` per-item inheritance**
  - The §1(b) example with a stub executor returning items with estimates 5 and 12 and outer `max_estimate` 8: only the second card shows "Over budget", and the executor is called once for the block.
  - A `[let]` at the item-body top level shadows the outer name for that card only. The outer `[var]` after the block still prints the outer value.
  - A category-level `CustomMRQLResult` (no block body) and `CustomCSS` do **not** see outer variables.
  - The header and footer see outer variables.
  - A definition the outer scope makes after the block is not visible in the cards.
  - A nested `[mrql]` block inside an item body reads both ancestors.
- **Hoisting**
  - An outer `[let path]` printed inside `[lazy]`/`[details]`/`[autorefresh]` with no signer.
  - With a stub signer, the sealed body starts with exactly the referenced definitions' raw text in definition order. An unreferenced definition is absent, **including a short name (`a`) whose letter occurs in the body text**.
  - References collected from `[elseif gt-var="cap"]` and from a nested `[details]` inside a `[lazy]` are both hoisted.
  - The inline mrql re-run costs 0 extra executor calls.
  - A body of depth ≤ 3 rendered via `Process` at depth 0 equals the inline output.
  - Inside `[using]` the prefix is empty and the inline `[var]` renders `""`.
  - A body that redefines a hoisted name: `[var]` before the body's `[let]` prints the hoisted value, and after it prints the body's value.
- **Other scoping**
  - A `[using]` body sees outer variables, and a `[let]` in it is rejected.
  - Cap at 33. Redefinition does not consume a slot. Parent names do not count.
- **Prose and parsing**
  - `[let's go]`, `[let it be]`, `[let me know if x=1]`, `[var foo=bar]`, `[var-x]` and `[letter]` round-trip unchanged.
  - `[let name="a" path="x"] … [/let]` leaves `[/let]` as text.
- **Unavailable and errors**
  - No executor plus mrql: `[var]` and `[conditional var]` give the unavailable comment.
  - Executor error: `[var]` gives the error span.
  - `ProcessWithDiagnostics`: an executor error from `[let mrql]` is in `Errors`; a nested `[let]` marker is not.
- **Isolation and safety**
  - Two `Process` calls sharing one ctx value do not share variables.
  - A credential field is nil without visibility.
  - A `<script>` meta value is escaped.

**`shortcodes/parser_test.go`**
- `[let …]`, `[var …]` and multiline `[let\nname=…]` parse.
- `[letter]`, `[variable]`, `[various artists]`, `[let's go]` and `[var-x]` produce no shortcode.
- `[/let]` is never a closer.
- `[metadata]` is unchanged.

**`shortcodes/mrql_slots_test.go`**
- For header, footer and else in every order, `inner[span]` contains the slot text.
- The spans of an absent slot are `{-1,-1}`.

**`shortcodes/lint_test.go`**
- A positive and a negative case for each rule.
- Visibility:
  - hoisted visibility inside `[lazy]` gives no warning;
  - inside `[using]`+`[lazy]` it warns with the `[using]` hint;
  - an outer name read inside an `[mrql]` item body gives no warning;
  - the same name read in the header before its `[let]` warns.
- Noise and prose:
  - no near-miss info for `[val]`, `[set]`, `[lets]`;
  - no diagnostics for `[let it be]`, `[let me know if x=1]` or `[var foo=bar]`;
  - `[var name="9x"]` reports an error.
- Other:
  - `in-var` on `[conditional]` reports an unknown-attribute warning;
  - `emitsBareValue` warnings for `href`/`onclick`;
  - an invalid `[let mrql]` is reported through `ValidateMRQL`.

**`builtin_docs_consistency_test.go`** (existing) covers the new entries: MRQL validity on SQLite and Postgres, reference panels, docs-site rows.

**`server/api_tests/let_shortcode_scope_test.go` (new)**, modelled on `TestRenderedNestedMRQLCannotEscapePrincipalScope`:
- A root-confined user renders, via `/v1/mrql?render=1`, `[let name="n" mrql='type = "resource"' scope="global" limit="50"][var name="n"]`. The result equals the subtree count, and an outside resource is not counted.
- A `SCOPE <outside id>` query gives "scope group not found" and never the outside name.
- Deferred: sign a body hoisting `[let mrql]` as user A, then fetch it as a confined user. The count is the confined user's subtree count, which proves that values are recomputed and not sealed.
- Share page (no executor): `[let mrql]`+`[var]` gives the unavailable comment, and `[let path]` works.

**`e2e/tests/shortcodes/let-var-shortcode.spec.ts` (new)**
- Category header: `[let name="tier" path="tier" default="basic"]Tier: [var name="tier"] [conditional var="tier" eq="gold"]<strong>Gold</strong>[/conditional]`. Assert it for a gold group and for a group without meta.
- The WIP badge with `gt-var`, on two groups with different `wip_limit` values.
- The §1(b) per-item "Over budget" badge appears on the right note only.
- `[details summary="Tier"][var name="tier"][/details]` shows the value after expansion through the deferred fetch.
- `[using …][details summary="x"][var name="tier"][/details][/using]` shows an empty body (documented asymmetry).
- The editor lint warns on `[var name="missing"]`.
- Axe scan with zero violations.

Before finishing, run:
- `go test --tags 'json1 fts5' ./...`;
- the new spec, `shortcode-autocomplete.spec.ts` and `shortcode-lint.spec.ts`;
- `npm run test:with-server:a11y`.

## 10. Docs to update

**`docs-site/docs/features/shortcodes.md`**
- Add `[let ...]`/`[var ...]` to the "Built-in inline" bullet, and table rows for both.
- New section "## `[let]` and `[var]` -- Template variables", covering:
  - the attribute tables;
  - the rule that the token needs `name=` to count;
  - the scope rules, plainly:
    - top-level only;
    - `[mrql]` block bodies read outer variables, but category cards and CustomCSS do not;
    - deferred bodies re-evaluate the referenced definitions;
    - inside `[using]` a deferred body sees none (with the example);
  - "attributes such as `[details summary]` are never expanded; put the value in the body";
  - the failure table;
  - an honest query-budget note: identical repeats were already free, and `[let]` gives one edit point plus comparisons;
  - the §1 examples;
  - the depth-qualified surface-equivalence note;
  - the compatibility note.
- `[conditional]`:
  - rows for `var` and the six `-var` operators;
  - the footnote becomes "One of `path`, `field`, `mrql`, or `var`";
  - Condition Sources gets **Var** and "Operand variables" paragraphs.

**Other docs**
- `docs-site/docs/features/custom-templates.md`: add `[let]`/`[var]` to the server-side shortcode list.
- `.claude/skills/mahresources-category-designer/references/shortcodes.md`: a short `[let]`/`[var]`/`gt-var` entry with the per-item example.

## 11. Success criterion

- `[let name="open" mrql=…][var name="open"][conditional var="open" gt="0"]…` runs one query per render. It prints and tests the same value on:
  - the detail page and live preview;
  - the share page (unavailable comment for mrql);
  - the deferred endpoint, including inside `[details]`/`[lazy]` through reference-based hoisting.
- Unreferenced definitions are never sealed.
- A per-entity threshold (`gt-var`) works, and an `[mrql]` block body compares each item against an outer value with one query for the block. No existing shortcode can express either today.
- Subtree-confined users get subtree-only values on the page and on the deferred fetch (values are recomputed, never sealed). An out-of-subtree `SCOPE` fails closed. The api test proves both.
- Misuse (nested, in a partial, over the cap, undefined reference, `[using]`+deferred) is flagged by lint before save. On the page it renders a diagnosable marker or nothing.
- Prose such as `[let's go]`, `[let me know if x=1]`, `[var foo=bar]`, `[variable]` or a stray `[/let]` is never consumed, and gets no new lint noise.
- All existing shortcode, lint, consistency and a11y suites pass unchanged.

---

## Last evaluator feedback

I would not approve this as-is, though it is close. Most file and API claims check out against the code:
- `resolveConditionalValue` priority is mrql > field > path and uses `aggregate`.
- `contextFromItem` builds a fresh struct.
- `renderResultCSS` `childCtx` is fresh.
- Header and footer render with the parent ctx through `renderMRQLSlot`.
- `shortcodePattern` is used only in `parser.go` and at `render_budget.go:366`.
- The lint unknown-attribute loop at `lint.go:374-386` strips trailing digits, so `gt-var2` resolves and `in-var` warns.
- Every block handler recurses at `depth+1`, and the plugin-output re-process is at `processor.go:340`.

Problems, most important first:

1. **Scope leak contradicts §4.1 kind 4 and §7.** The plan says `ictx.inheritVars = parentCtx.inheritVars` "is non-nil only on the block-body path". That is false for a nested case.
   - The item ctx (`ictx`) keeps `inheritVars` when it is passed to `processWithDepth`, and every ctx copy inside the item body carries it.
   - A non-block `[mrql query=…]` inside an item body (`itemTemplate == ""`) calls `renderFlat(ctx)`. `renderFlatWithCustom` then copies `parentCtx.inheritVars` (the outer page scope) into each category-level `CustomMRQLResult` card. Those cards get a parent chain, which the plan says must never happen.
   - The lint model (item regions only) would not predict this.
   - Fix: pass the parent scope as an explicit argument to `renderFlat`/`renderFlatWithCustom`/`renderBucketed`, or have `RenderMRQLShortcode` always set `itemParent.inheritVars` to `ctx.vars` when `itemTemplate != ""` and to `nil` otherwise. Also clear `inheritVars` once the scope is created, so it is never carried on ctx copies.
   - Add a test: a block `[mrql]` whose item body contains a non-block `[mrql]` that returns a category-card item reading an outer name. It must render empty.

2. **Test claim is wrong.** §0.8 says "the existing '0 extra executor calls' test proves this", but no such test exists; it appears only in the new §9 list. The claim also depends on `QueryBudget` being in the executor chain: a bare counting stub will see 2 calls.
   - Say that the unit test wraps the stub in the real `QueryBudget` executor wrapper, from `query_budget.go`, as production does.
   - Separately assert that 2 calls happen without the budget, so the test proves the cache hit rather than assuming it.

3. **The `[mrql]` item-body / deferred interaction is unspecified for authors.**
   - A `[details]` or `[lazy]` inside an `[mrql]` item body hoists only the item scope's own map. On both paths it gets a fresh scope with no parent, so the outer `cap` that the surrounding card can read is invisible inside the `[details]`.
   - This is a second asymmetry like the `[using]` one. It needs the same treatment: a docs example, a lint hint, and a test. Today §8 mentions it only implicitly ("deferred regions do not inherit chain names").

4. **Too much machinery for the value delivered.** The design adds all of these for what are essentially two capabilities, operand variables and outer-value access in block bodies:
   - parent chains;
   - reference-scanning hoist with a separate `elseifAnywherePattern`;
   - new `HeaderSpan`/`FooterSpan`/`ElseSpan` offsets with remapping across removed ranges in `mrql_slots`;
   - a lint visibility model with two inheritance rules plus a `[using]` exception;
   - a `depth`-equality nesting check.

   Each piece is a drift risk against the runtime. In particular, the lint region model must stay in lockstep with `parseMRQLSlotsM`'s first-slot semantics, top-level `[else]` counting included.

   A smaller v1 would keep most of the usefulness: `[let]`/`[var]`/`-var` with top-level-only scope, plus outer reads in `[mrql]` bodies, with deferred bodies seeing no variables and lint warning on that. Hoisting could come later.

5. **Usefulness is real but modest.**
   - Per-entity thresholds (`gt-var` against meta) and outer-vs-item comparison are genuinely new.
   - Deduplication is already mostly covered by `QueryBudget`.
   - The other examples (`tier`, `default`) duplicate `[conditional path= eq=]` plus `[meta]`/`[property]`.
   - The motivation would be stronger with evidence from existing plugin or category templates in the repo where a field-to-field comparison was needed or worked around.

6. **Minor: the `[mrql]` handler change is incomplete.**
   - `RenderMRQLShortcode` only checks `itemTemplate != ""`. It should also say what happens for `format="table"/"list"/"compact"` combined with a block body, where `renderFlat` never processes item templates and `inheritVars` is moot.
   - For `[var]` inside attribute contexts (an `aria-label`, for example), confirm that `formatJSONScalarObserved` output for objects/arrays (JSON text) is acceptable, or restrict `[var]` to scalars.

7. **Minor: the `shortcodeNameAt` trim is underspecified.** The name group now includes the whitespace (`let\s`). State explicitly that every reader of `mm[2]:mm[3]` trims, and that lint's `tk.name` comparisons and `builtinBaseNames`-based checks see `let`, not `let `. Add a parser test for `[let\tname=…]`.
