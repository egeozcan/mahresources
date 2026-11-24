# Plan: `[calc]`, safe arithmetic over fields

Slug: `calc`. Saved as `docs/plans/2026-11-24-shortcode-calc.md` (the index script files it under "Category templates & shortcodes").

## 1. Motivation and user value

Category, Resource Category and Note Type templates can show stored values (`[meta]`, `[property]`, `[item]`) and compare them (`[conditional gt=… lt=…]`). They cannot show a value derived from stored ones. Some examples:

- an invoice line total (`qty × unit_price`)
- a discounted price
- a recipe scaled per serving
- a budget's remaining amount, or how much of it is used
- an inventory value
- a resource's pixel count or aspect ratio

Today an author has two options, and both are poor:

- **Store a redundant field.** It goes stale the moment someone edits a source field.
- **Write a meta-editor or Lua plugin.** That is heavy, needs plugin work admission for every card, and is unavailable where the plugin renderer is not wired (share pages render `<!-- mr:plugin unavailable -->`).

`[calc]` evaluates a small, closed arithmetic language in Go:

- It reads only data already loaded for the entity being rendered.
- It never issues a query.
- Its cost is fixed, bounded and linear in the length of the expression.

Non-goals, deliberately left out:

- string evaluation
- conditionals inside the expression (`[conditional]` exists)
- loops and aggregation over arrays (no `sum(meta.lines…)`)
- variables (there is no `[let]` shortcode in the codebase, so the pitch's "[let] variables" is dropped)
- locale-aware number formatting
- date arithmetic

For totals across many entities, use the MRQL aggregate path that already exists (`[mrql value=…]` over a `GROUP BY`).

## 2. Syntax

```
[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" default="-"]
```

Inline-only (`BlockNo`). A `[/calc]` closer is a stray closer and follows the existing generic stray-closer lint and render behaviour.

### Attributes

| Attr | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `expr` | string | yes | — | The expression (grammar below). Use single quotes around the attribute when the expression itself needs none. The expression never contains quotes, so either quoting works. |
| `decimals` | number | no | unset | A whole number from 0 to 10. When set, the result is rounded half away from zero to exactly this many fraction digits, with trailing zeros kept (`10` with `decimals="2"` gives `10.00`). When unset, the result is rounded to 10 fraction digits and trailing zeros (and a trailing `.`) are stripped, so `0.1 + 0.2` gives `0.3` and `1 / 3` gives `0.3333333333`. |
| `format` | enum `number`, `grouped`, `percent` | no | `number` | `number` writes a plain decimal (`1234567.89`). `grouped` adds a `,` every three integer digits and uses `.` as the decimal point (`1,234,567.89`), which is fixed and not locale-aware. `percent` multiplies by 100, applies `decimals` and appends `%` (`0.256` gives `25.6%`). |
| `default` | string | no | `""` | Text rendered when the expression has no numeric result (section 5). It is HTML-escaped. |

No other attributes are accepted. Unknown ones get the existing generic "unknown attribute" lint warning. `raw`, `hide-empty`, `editable` and `layout` do not exist on `[calc]`: the output is always a number or the escaped default, so none of them would mean anything.

### Expression grammar (EBNF, whitespace allowed between tokens)

```
expr     = term { ("+" | "-") term } ;
term     = unary { ("*" | "/" | "%") unary } ;
unary    = { "+" | "-" } primary ;
primary  = number | ref | call | "(" expr ")" ;
number   = digit { digit } [ "." digit { digit } ] ;        (max 32 chars, no exponent, no leading ".")
ref      = "meta" "." seg { "." seg }
         | "property" "." pseg { "." pseg }
         | "item" { "." seg } ;
seg      = (letter | "_") { letter | digit | "_" } ;
pseg     = seg | digit { digit } ;                           (numeric segment = slice index, as in [property])
call     = fname "(" expr { "," expr } ")" ;
fname    = "min" | "max" | "abs" | "round" | "coalesce" ;
```

- Operators: `+ - * /` as usual. `%` is `math.Mod`, so the result takes the sign of the dividend (`-7 % 3` is `-1`). Precedence: unary, then `* / %`, then `+ -`, all left-associative.
- Function arity:
  - `min` and `max` take 1 to 8 arguments.
  - `abs` takes 1.
  - `round` takes `round(x)` or `round(x, d)`. `d` must evaluate to a whole number from 0 to 10, otherwise the call has no result. Rounding is half away from zero.
  - `coalesce` takes 1 to 8 arguments and returns the first one that has a numeric result. Arguments are evaluated left to right, and evaluation stops at the first hit.
- Names are case-sensitive. A bare identifier (`qty`), an unknown root (`entity.x`), an unknown function (`sqrt`) or a call with the wrong arity is a syntax error.
- Meta keys containing `-`, spaces or dots cannot be referenced: `meta.unit-price` parses as `meta.unit - price`, and `price` is then an unknown name, so it is a syntax error. This limitation is documented. Quoting keys would collide with attribute quoting, so it is not added.

## 3. Operand resolution

Every reference produces either a finite `float64` or "no value". "No value" propagates upward through every operator and function except `coalesce`.

- **`meta.<path>`**: `navigateJSONValue(ctx.decodeMeta(), path)`, which uses the per-render shared decode cache (`decodedMeta`).
  - A JSON number gives its value.
  - A JSON string gives its value only when, after `strings.TrimSpace`, it fully matches `^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$` and `strconv.ParseFloat` returns a finite value. So `"12.50"` and `" 3 "` count, while `"12,5"`, `"0x10"`, `"NaN"`, `"Inf"`, `"1_000"` and `""` do not.
  - Booleans, null, objects, arrays and a missing key give "no value".
- **`property.<Path>`**: `traversePropertyPath(ctx.Entity, path, ctx.credentialsVisible)`, exactly as `[property]` uses it. Pointers and interfaces are dereferenced; a nil pointer (for example `property.OwnerId` on an ownerless entity) gives "no value".
  - Go integer, unsigned and float kinds give their value.
  - Every other kind gives "no value": string, bool, `time.Time`, struct, slice.
  - Strings are deliberately not parsed. A struct string field is text, and the same rule protects `[property format=…]` from changing meaning.
  - Credential fields are refused by the traversal itself.
  - `ctx.Entity == nil` gives "no value".
- **`item` / `item.<path>`**: the current `[each]` element, through `navigateJSONValue(elem, path)` with the same conversion as `meta`. This works only where `[item]` works (section 6). Everywhere else it gives "no value".
- **Literals**: `strconv.ParseFloat`.

Every intermediate and final value that is NaN or ±Inf becomes "no value". This covers division by zero, `% 0` and overflow. As a result, `coalesce(meta.a / meta.b, 0)` catches a zero divisor.

## 4. Rendering output and accessibility

The output is bare text with no wrapper element, the same as `[meta inline="true"]`, `[property]` and `[item]`:

- When there is a numeric result: the formatted number. ASCII `-` for negatives. `-0` and anything that rounds to zero is normalised to an unsigned `0` (or `0.00`). No exponent notation is ever used: formatting is `'f'` on a decimal string rounded per `decimals`.
- When there is no result: `html.EscapeString(default)`, or `""` when the default is empty.

Why bare text:

- It is usable inside attributes (`style="width: [calc …]%"`, `<meter value="[calc …]">`, `data-total="…"`).
- A screen reader reads it as ordinary text in its surrounding context: a table cell under a `<th scope="col">`, or a sentence.
- A wrapper span adds nothing for assistive technology.

The output cannot carry markup from data: digits, `.`, `,`, `-`, `%`, or the author-written default (escaped). That is why `[calc]` is **not** added to `emitsBareValue` (lint_attr_context.go). The unsafe-attribute-context warnings exist for user-controlled strings, and here they would only be noise.

Accessibility guidance goes in the docs (section 10):

- Present derived numbers in a labelled context: a table with header cells, or text such as "Total due: …". Do not show a bare number alone.
- When a value drives a visual bar, use `<meter>` or `<progress>` with an accessible name, e.g. `<meter min="0" max="1" value="[calc expr='meta.spent / meta.budget' default='0']" aria-label="Budget used">[calc expr='meta.spent / meta.budget' format='percent' decimals='0' default='unknown']</meter>`. The fallback content and `aria-label` both carry meaning. Never use colour alone.
- Give `default` meaningful text (for example `"not set"`) rather than a lone dash where the reader needs to know why the value is absent. A `-` read aloud as "dash" is acceptable inside a table cell.

Author errors render `shortcodeErrorMarker("calc", detail)` (markers.go): a `⚠ calc` inline span with the escaped detail in `title`. This is the existing marker for actionable template mistakes. Author errors are:

- syntax errors
- limit violations (section 7)
- an invalid `decimals` (not a whole number from 0 to 10)
- an invalid `format`

A blank or missing `expr` renders `""`, matching `[meta]` and `[property]` with no `path`. Lint reports it as a missing required attribute.

## 5. Empty, missing and invalid inputs (summary)

| Situation | Output |
|---|---|
| `expr` absent or blank | `""` (lint error) |
| Syntax error, unknown name or function, wrong arity, limit exceeded | error marker |
| `decimals` not 0–10 integer, `format` not in enum | error marker |
| Missing, null, boolean, object, array or non-numeric operand (not caught by `coalesce`) | `default` |
| Divide or modulo by zero, overflow to ±Inf, NaN | `default` (catchable by `coalesce`) |
| `round(x, d)` with `d` not a whole number from 0 to 10 | that call has no value, so `default` unless caught |
| `item…` outside `[each]` or not at the top level of its branch | `default` (lint warning) |
| `property…` with no entity | `default` |

## 6. Integration with `[each]`

`[item]` values are spliced in after the branch is processed, through sentinels (`markItems` / `spliceLazy` in each_handler.go), so the processor's dispatch never sees an element. `[calc]` follows that exact mechanism:

- `markItems` marks top-level `calc` tokens as well as top-level `item` tokens. The size accounting loop counts both.
- The splice callback dispatches on `items[idx].Name`:
  - `calc` calls `renderCalc(ctx, sc, elem, true)`.
  - `item` calls `renderItemValueObserved` as today.
- `stripNUL` still applies.

Consequences, documented the same way as for `[item]`:

- A `[calc]` nested inside another block within the branch (`[conditional]`, `[lazy]`, `[details]`, `[autorefresh]`, `[using]`) does not see the element: its `item…` operands have no value. `meta.` and `property.` operands still read the parent entity.
- A nested `[each]` binds its own `[calc]`.
- Top-level `[calc]` tokens that reference only `meta.` or `property.` are also evaluated at splice time. The result is identical, because the output is text and not template source.

## 7. Limits (cost cannot be driven by input)

These are constants in `calc_handler.go`:

| Limit | Value |
|---|---|
| `expr` length (after the parser's attribute unescape) | 512 bytes |
| Tokens | 128 |
| Nesting depth (parentheses, calls and unary chain combined) | 32 |
| References per expression | 32 |
| Numeric literal length | 32 characters |
| Arguments per call | 8 |

- The tokenizer stops at the first limit hit and reports an author error. It is a single pass with no backtracking and no regular expressions over the expression; the numeric-string check in section 3 is a fixed, anchored RE2 pattern applied to operand values.
- The parser is recursive descent, bounded by the depth limit.
- Evaluation is one walk of an AST of at most 128 nodes.
- Each `meta.` reference is one map walk over the per-render shared decode, so the Meta blob is decoded once per entity whatever the number of references.
- Each `property.` reference is one reflection walk with no DB loads.

## 8. Scoping and authorization

`[calc]` has no query path:

- It reads only `ctx.Meta`, `ctx.Entity` and the `[each]` element.
- It takes no `QueryExecutor` or `PluginRenderer`, so it cannot reach data outside the entity the page is already rendering for this principal.
- Inside `[using]` or `[mrql]` result templates it reads the context those handlers built, and they already enforce the subtree.

Group-subtree confinement therefore holds by construction and fails closed: there is no fallback path that could widen it. `property.` exposes nothing `[property]` does not already expose, because it is the same traversal with the same credential-field refusal.

Untrusted text: `[calc]` in a sanitized Description (`UntrustedSource`) is safe without special handling. Its output is a number or the escaped default, and there is no `raw`.

Privacy mode (template runs): a computed number is reported to `ctx.scalarObserver`, matching `formatJSONScalarObserved`, because it is text derived only from numbers, which privacy mode preserves. A masked string operand is a placeholder, which is non-numeric, so it yields the default. The default is never reported.

## 9. Performance at millions of resources

- No queries, so the per-page query budget (query_budget.go) is untouched.
- The render meter's existing per-shortcode `tick` and `chargeOut` in `processWithDepth` and `spliceLazy` cover output size. There is no new budget.
- Cost per `[calc]` is microseconds: at most 512 bytes parsed and 128 nodes evaluated. A list page of 100 cards with 5 calcs each is 500 tiny parses. ASTs are not cached, because no measurement justifies a cache.
- `[each]` already caps iterations (`limit`, default 100), so a `[calc]` in an `[each]` is bounded by that cap.

## 10. Where it lives and why

It is a **built-in Go handler** in `shortcodes/`, not a plugin, for four reasons:

1. **Reach.** It must work everywhere `[meta inline]` works: share pages, `CustomMRQLResult` cards, live preview and hover cards. Plugin shortcodes are "unavailable in this context" on several of those surfaces.
2. **Cost.** A plugin call per card goes through VM and work admission (plugin-work-admission.md). Pure arithmetic does not justify that.
3. **`[each]` integration.** It needs the sentinel splice, which is internal to the shortcodes package and unavailable to plugins.
4. **Safety and lint.** The safety argument (closed grammar, hard limits) is easiest to state and test in one Go file, and lint can parse the expression with the same parser.

## 11. Files to change

New:

- [x] `shortcodes/calc_handler.go`: tokenizer, parser (`parseCalcExpr(expr string) (*calcProgram, error)`, exposing the reference list for lint), evaluator, decimal-string rounding shared by `round()` and `decimals`, grouped and percent formatting, `RenderCalcShortcode(sc Shortcode, ctx MetaShortcodeContext) string` and the internal `renderCalc(ctx, sc, elem any, hasElem bool) string`.
- [x] `shortcodes/calc_handler_test.go`
- [x] `shortcodes/calc_fuzz_test.go`
- [x] `e2e/tests/shortcodes/calc-shortcode.spec.ts`
- [x] `e2e/tests/accessibility/calc-shortcode-a11y.spec.ts`

Edited (each change minimal):

- [x] `shortcodes/processor.go`: add `case sc.Name == "calc": replacement = RenderCalcShortcode(sc, ctx)`.
- [x] `shortcodes/parser.go`: add `calc` to both name alternations.
- [x] `shortcodes/each_handler.go`: `markItems` marks `calc` alongside `item` (size loop and build loop); the splice callback dispatches by name; comments updated.
- [x] `shortcodes/builtin_docs.go`: the `calc` `BuiltinDoc` (attributes, `format` enum, Examples "Line total", "Optional discount", "Inside [each]", "Percent in a meter") and the type comment's name list.
- [x] `shortcodes/lint.go`: `"calc"` in `builtinBaseNames`; a `case "calc":` that reports parse/limit errors as `[calc] expr: <message>`, errors on bad `decimals` and `format`, warns on undeclared `meta.` references outside `otherContext`, and warns on `item` references outside `[each]` (and inside a `[using]` within one).
- [x] `shortcodes/lint_feedback.go`: `metaPathIssueFor(label, path, props)`, with `metaPathIssue` delegating.
- [x] `templates/createCategory.tpl`, `templates/createResourceCategory.tpl`, `templates/createNoteType.tpl`: a reference-panel entry for `[calc expr="…"]`.
- [x] `docs-site/docs/features/shortcodes.md`: the table row and a `## \`[calc]\` -- Arithmetic over Fields` section (Attributes, Operands, Operators and functions, Missing values and `coalesce`, Formatting and rounding, Inside `[each]`, Limits, Accessibility, Examples, the hyphenated-key limitation).
- [x] `docs-site/docs/features/custom-templates.md`: a "Derived totals" example under Advanced Examples.
- [x] `.claude/skills/mahresources-category-designer/references/shortcodes.md`: a `### [calc]` entry.

Not changed: `emitsBareValue` (section 4), the query executor and the plugin system, OpenAPI (`/v1/shortcodes/docs` already serves `BuiltinDocs`).

## 12. Tests

Go unit tests (table-driven, `shortcodes/`):

- [x] 1. Arithmetic: precedence, parentheses, unary chains, `%` sign, left associativity.
- [x] 2. Pitch example: `33.75`; without `discount`, `37.50`.
- [x] 3. Operand conversion: numeric strings accepted, `"12,5"`, `"abc"`, `"0x10"`, `"NaN"`, `"Inf"`, `"1_000"`, `""`, `true`, `null`, `{}`, `[]`, missing give the default.
- [x] 4. Non-finite: `/ 0`, `% 0`, overflow give the default; `coalesce(meta.a / meta.b, 0)` gives `0`.
- [x] 5. Functions: `min`, `max`, `abs`, `round` (half away from zero, `2.675` to `2.68`), `round(1, 11)` / `round(1, 1.5)` give the default, `coalesce` short-circuits, wrong arity is the marker.
- [x] 6. Formatting: unset `decimals`, fixed `decimals`, no `-0.00`, `grouped`, `percent`, no exponent for `1e20`.
- [x] 7. Property: `Width * Height`, nil `OwnerId`, string field, credential field, nil entity.
- [x] 8. Author errors render the marker (513 bytes, 129 tokens, depth 33, bare identifier, unknown function, trailing operator, `()`, `meta.`, `meta.unit-price`, bad `decimals`, bad `format`), and the `title` escapes `<script>`.
- [x] 9. Escaping of `default`.
- [x] 10. No data path: executor and plugin renderer that `t.Fatal`.
- [x] 11. Privacy observer receives `33.75`, nothing for the default.
- [x] 12. `[each]`: object and scalar elements, outside `[each]`, nested `[each]`, inside `[conditional]`, and a `markItems` size-accounting test with a `[calc]` token.
- [x] 13. Fuzz `FuzzCalcExpr`.
- [x] 14. Lint: syntax error, bad `decimals`/`format`, missing `expr`, undeclared `meta.` with a `[calc]` label, `item` outside `[each]`, `[/calc]` closer, every `BuiltinDocs` `[calc]` example lints clean.
- [x] 15. Existing consistency tests (docs table, panels, generation-prompt docs).

E2E:

- [x] `e2e/tests/shortcodes/calc-shortcode.spec.ts`: pitch value, `not set`, line totals in a captioned table, `[autorefresh]` update after an inline edit.
- [x] `e2e/tests/accessibility/calc-shortcode-a11y.spec.ts`: same fixture plus the labelled `<meter>`, `checkA11y()` with no violations.

## 13. Success criterion

The following holds on a fresh build:

- **Group detail page:** a category template using `[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2" default="-"]` renders `33.75` for `{qty:3, unit_price:12.5, discount:0.1}`, and renders `-` when `qty` is missing or is `"abc"`.
- **Line items:** the same template renders correct per-line totals inside `[each]`.
- **Live update:** the value updates in place inside `[autorefresh]` after an inline edit.
- **Author errors:** every malformed or over-limit expression renders the `⚠ calc` marker and is flagged by lint in the editor.
- **No data access beyond the page:** a test proves `[calc]` never calls the query executor or the plugin renderer.
- **Docs:** the docs-site table, the three reference panels and the skill reference list `[calc]`.
- **Accessibility:** the axe scan of a page using `[calc]` in a captioned table and a labelled `<meter>` reports no violations.
- **Suites:** all Go, browser E2E, CLI E2E and a11y suites are green.

## 14. Review

Reviewed after the work was finished, against the tree that already carries `[inspect]`, `[adjacent]`, `[similar]` and `[calendar]`.

- **Plan vs implementation.** Every item in sections 11 and 12 is present. `[calc]` sits beside the four later shortcodes in both parser alternations, in `isElementToken` with `[item]` and `[inspect]`, and in the splice callback's name switch. No gap needed filling.
- **Outside the plan.** `public/jsonTable.css` darkens `.metaToggler` from stone-500 to stone-600 (hover stone-700). The a11y scan of the calc fixture page found the 11px toggler at 4.39:1, under AA. The fix is one colour pair and affects every page that shows the meta toggler.
- **Verified.** `go build --tags 'json1 fts5' ./...`; `go test` over `./shortcodes/...`, `./internal/arch/...` and `./server/api_tests/...` filtered to `Calc|Shortcode|Docs|Lint`, plus the whole `shortcodes` and `internal/arch` packages; `FuzzCalcExpr` for 20s with no failure; `npm run build`; `./scripts/css-scan-test.sh`; and the two calc E2E specs (4 tests, all passing).
- **Not run here.** The full browser, CLI and Postgres E2E suites from section 13 were not run.
- **Known limitation, kept on purpose.** Meta keys containing `-`, spaces or dots cannot be referenced (section 2). The docs say so.
