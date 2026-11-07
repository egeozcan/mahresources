# Template generation harness: class-rule audit (lane B1)

Plan: `2026-11-07-template-generation-harness.md`, section 2 "Preview and lint work is bounded", Appendix B items 4 to 58 and Appendix C items 1 to 4.

The class rule: every loop reachable from `shortcodes.Process`, `shortcodes.Lint`, `LintMetaSchema`, `ValidateMetaSchema` and the schema expansion helpers checks the context, and every loop that builds output or copies data charges a budget before allocating, or says why it needs neither.

## Mechanisms

- **Render meter** (`shortcodes/render_budget.go`). `ProcessChecked` installs one `renderMeter` per top-level render (a meter already on the context is shared). It holds the output and parsed-source byte counts, a step counter, the compiled-regex cache and a sticky error. Once a limit is crossed or the context is done, every later check fails, so handlers that return plain strings still end the render, and `ProcessChecked` returns `ErrRenderBudgetExceeded` (wrapping the context error on cancellation). `Process` and `ProcessWithDiagnostics` keep their signatures; with no `WithRenderBudget` on the context nothing is limited, and the output is byte-identical (the whole existing shortcode corpus passes unchanged).
- **Overcounting.** Output is charged at every append at every nesting level, so bytes returned from a nested render are counted again when the parent appends them. The budget is therefore conservative by up to the nesting depth. This is deliberate: an undercount in a handler I missed would be unbounded; an overcount only makes the preview stricter.
- **schemaWork** (`shortcodes/schema_work.go`). One work object for the schema paths: 100,000 work units, 1 MiB of expanded bytes, depth 32, a map of `$ref` pointers on the current path, and the context. `newLintWork` uses larger allowances (5,000,000 units, 64 MiB) for template lint.
- **Admission before compile** (`application_context/meta_schema_admission.go`). The jsonschema compiler takes no context and cannot be interrupted, so `ValidateMetaSchema` hands it only what a streaming token pass admitted.

## Appendix B: shortcode processing and lint

| # | Loop or allocation | Check or reason |
|---|---|---|
| 4 | `processWithDepth` strip, `Grow`, dispatch, literal copies | `chargeParsed(len(input))` before stripping or parsing; `tick` per dispatch; `chargeOut` before every literal span, replacement and suffix; failure returns "" and the meter stays failed, so nesting unwinds. `Grow(len*2)` runs only on admitted input. |
| 5 | `matchTokens` and `parseAttrs` collections | A budgeted parse caps `FindAll` at 50,001 tokens and 1,000 attributes per token (`budgetedTokenCap`, `budgetedAttrCap`) before retaining slices; `tick` per token. `CheckGeneratedSource` counts tokens with the same regexes capped at 2,001 before anything parses. Lint collects its tokens through the same capped path (a meter forced into limited mode, so the match collection is bounded before it is retained and the loops consult the context) and caps the loose-bracket matches the same way; a source past a cap is "too complex to check". Unbudgeted parses (pages) are unchanged. |
| 6 | Closer matching and block reconstruction | Pairing uses a per-name stack of unmatched openers (equivalent to the nearest-earlier-unmatched backward scan, linear). The forward closer search in `parseWithBlocksM` ticks per step and visits each top-level block once. |
| 7 | `[each]` sentinel pipeline | Item values are formatted lazily at their sentinel (`spliceLazy`); `alive` per iteration, `chargeOut` before every append and before each formatted value. The branch is re-processed per element, and `chargeParsed` counts that repetition. `markItems` runs once per `[each]` over admitted source. "Substituted entity text is never rescanned" is pinned by `TestEachLazySpliceNeverRescans`. |
| 8 | Span membership in `SplitElse`, `SplitBranches`, divider scans, lint placement | `blockInnerSpans` spans are sorted and disjoint, so membership is `insideSortedSpans` (binary search), no longer spans x bytes. The lint `eachSpans`/`reloadSpans`/`usingSpans` are nested, so they keep a linear scan per `[item]`/`[reload]` token; those are bounded by the lint token count and each lint token loop calls `w.step()`. |
| 9 | MRQL slot extraction | `firstSlotSpan`, `firstElseIndex` are one scan over the slot body with the binary-search membership. `mrqlSlotTagSpans` allocates `len(inner)+1` ints once per body; lint reaches it through `lintStrayCloseTags`, which now charges `1+len/8` units per `[mrql]` body, so nested bodies share one allowance. |
| 10 | Conditional helpers | `condFold` folds incrementally (no results slice); `alive()` before each numbered condition and operator; `in` walks its operand with `strings.Cut` and ticks; the tested value's text is built once per condition. The renders stop when the context does (`TestConditionalStopsAfterCancellation`). |
| 11 | `collectShortcodeParams` repeated | Lint collects an opener's params once and builds every numbered query's options from that map (`mrqlCheckOptionsWith`); each `[elseif]` arm collects its own once. At runtime `evaluateConditionSet` collects once per attribute set and passes the map to every numbered source. `[mrql]` and `[using]` collect once per call. |
| 12 | MRQL assembly (CSS wrappers, cards, buckets) | `renderResultCSS` and `renderFlatWithCustom` tick per item and charge each piece before appending; `renderBucketed` ticks per group and charges each bucket; `RenderMRQLShortcode` writes every part through a charging writer. Nested budget errors propagate through the sticky meter instead of becoming empty successes. |
| 13 | Built-in MRQL layouts | The renderers take the meter, tick per item, row, cell and key, and `renderFlatCompact` streams into one builder instead of retaining a parts slice. Every layout reserves each row (each cell for aggregates) with `charged.flush` right after formatting it, so a render holds at most the budget plus one row (`TestBuiltinMRQLLayoutsChargeRows`). |
| 14 | `{count}` `{total}` `{link-all}` multiplication | `substitutePlaceholdersM` counts occurrences, computes the expanded size with overflow checks and calls `admitParsed` (the expanded slot is parsed next) before `Replacer.Replace`. Applies to header, footer and else. `TestMRQLSlotPlaceholderExpansionIsAdmitted`. |
| 15 | `containsReloadShortcode` | `containsReloadShortcodeM` uses `matchTokensM` with the render's meter. |
| 16 | `templatecomment` discovery | Linear in its input per pass with a constant 16-pass cap. `processWithDepth` admits the source before stripping and `Lint` charges `1+len/64` before `StripMapped`, so the input is already admitted. Not changed (outside this lane's files). |
| 17 | Comment stripping and offset mapping | Same: linear in admitted input, constant pass cap, the mapping runs are bounded by the number of removed regions. |
| 18 | `htmlCommentSpans` | One pass of the HTML tokenizer over the admitted lint input, with spans bounded by the number of comments in it; the loop that consumes it ticks per token. |
| 19 | Dropped-value probes (`rawBreakoutReachesLive`, `unquotedSiblingInjectsOne`) | `attributeContextsForW` charges `len(substituted)` bytes before each probe, `2*len` before the two parses and `len` before the lexical pass, against the 64 MiB lint allowance. |
| 20 | URL-context prefix copies | `sentinelsInTag` reserves each unescaped prefix (`chargeBytes(idx.at)`) before allocating it and stops collecting once the 64 MiB lint allowance is spent; `lexicalContexts` steps per token. `TestLintURLPrefixCopiesAreBounded` (10,000 sentinels in one `href`). `urlSchemeBefore` and `stripSentinels` work on one already-collected prefix. |
| 21 | Nested conditional reparsing | Each conditional's `SlotTagSpans` and `SplitBranches` is charged `1+2*len(inner)/8` units, and the `countTopLevelElse` call is charged `1+len(inner)/8` whether or not MRQL validation is on; `countTopLevelElse` no longer rescans every block per divider (binary search). |
| 22 | `attrOffset` rescans | `attrOffsetW` charges `1+len(raw)/8` per call. |
| 23 | Lint ordinary passes, diagnostics | Token, attribute, loose-bracket and structural loops call `w.step()`/`w.charge`; `add` charges each message's bytes before keeping it; exhaustion or cancellation returns one "too complex" issue (`lintTooComplex`). `namedBlockSpans` is a stack over the token slice (linear in tokens). |
| 24 | Attribute classification (`lexicalContexts`, `classifyByParse`) | Admitted by the source charge and the per-parse byte charge above; the passes are linear in the substituted document, run twice, and their fact slices are sized by the span count that `attributeContextsForW` charged first. |

### Schema rendering

| # | Loop | Check |
|---|---|---|
| 25 | `extractSchemaSlice` | One `schemaWork` per slice; `charge(1)` per path part; the final serialization is admitted with `valueBytes` before `json.Marshal`. `valueBytes` charges an upper bound of the JSON plus HTML-attribute escaping (`escapedSize`: control characters and `<`, `>`, `&` at six bytes, quotes and backslashes at seven, structural quotes at five), keys included. Cancellation during expansion keeps its typed cause (`failErr`). |
| 26 | `resolveLeafSchema` direct `$ref` | `enterRef` at the direct-reference branch; a repeated ref stops the expansion (the recursive-schema stack overflow). `TestMetaSelfReferencingSchemaDoesNotCrash`, including a direct a-to-b-to-a cycle. |
| 27 | Leaf conditional copy and re-resolve | `charge(len(node))` before the base copy; `mergeConditionBranches` merges through the charged `shallowMergeSchema`; depth is threaded (`depth+1`) and the work object is shared, never reset. |
| 28 | `inlineRefsDeep` | `deep(depth)` and `charge(1+len(node))` on entry; a charge before each composition slice, property map and loop; `enterRef` for the path. |
| 29 | `resolveSchemaNodeImpl` | Charge on entry and before the base-map copy; every branch visit and merge is charged; exhaustion returns nil and the caller reads `w.Err()` rather than treating it as unresolved. |
| 30 | `tryEvaluateCondition` | `charge(1+len(ifSchema))`, `charge(len(props))`, `charge(len(enum))` before each enum scan; the single `reflect.DeepEqual` of one stored value stays linear. |
| 31 | `normalizeTypeList` | `charge(len(arr))` before the allocation. |
| 32 | `followRef` | `charge(1+segments)` and `chargeBytes(len(ref))` before splitting. |
| 33 | `shallowMergeSchema` | `deep(depth)`, `charge(len(dst)+len(src))` before each map copy, the property merge charged again, recursion carries depth. |

### Schema lint

| # | Loop | Check |
|---|---|---|
| 34 | `schemaLinter.walk` | Work object on the linter; `charge(1+len(obj))` and `chargeBytes(len(path))` per node; depth cap 128; exhaustion yields one "too complex" error issue. |
| 35 | `sortedKeys` | Now `l.sortedKeys`, which charges the count before allocating. |
| 36 | `warn` and child paths | Reserves the message bytes (4x for string arguments, since `%q` lengthens them) before formatting. Child paths are built by `childPath`, which reserves their bytes before the concatenation; absent children get no path and every child loop stops as soon as the work is spent. |
| 37 | `lintLabeledEnum` | `charge(len(entries))` first. |
| 38 | Plugin display types | A set built once per lint (`hasDisplayType`), its construction charged. |
| 39 | `lintBuiltinDisplay` | Consumes deduplicated effective lists; the warning joins are bounded by the reserved `warn` bytes. |
| 40 | `fieldTypeFits` | Shares the linter's work object across every `effective` call and charges per definition. |
| 41 | Effective merges | `addTypes`/`addProp` charge each entry, and deduplicate type lists by text and property subschemas by map identity. |
| 42 | Effective traversal | `charge` on entry and before the `allOf`, type, properties and required loops. |
| 43 | `onlyModelledKeywords` | Now takes the work object and charges the keys it inspects. |
| 44 | `localRef` | Charges segments and bytes before splitting. |

### `ValidateMetaSchema` and the compiler

Items 45 to 58 are loops inside `github.com/santhosh-tekuri/jsonschema/v6`, which takes no context and has no work parameter. **They are not implemented as a cancellable or budgeted compilation boundary, and I could not do that without patching or forking the library; a goroutine timeout would not stop its allocation.** What exists instead is admission of the input before the compiler runs, and `ValidateMetaSchemaContext`, which checks the context between the phases (before admission, before compile, after compile) but not inside `Compile`:

- numbers (`admitMetaSchema`, a streaming `json.Decoder` pass that builds no tree): exponent outside +/-400, more than 100 significant digits, or more than 10,000 digits in total are refused (the `big.Rat` retention of 500 `1e-1000000` constraints was 5.6 GB across six parallel validations before);
- nesting deeper than 64 levels and more than 500,000 tokens are refused;
- regular expressions (54): `pattern` and `patternProperties` keys are admitted only at schema positions (a walk over `properties`, `$defs`, `allOf`, `items` and the other schema-valued keywords, plus the targets of local `$ref`s), never inside `const`, `enum`, `default` or `examples`. Each is bounded by `shortcodes.MaxRegexpProgramSize` and the sum over the schema by 200,000 estimated instructions.

Items 45 to 53 and 55 to 58 remain **open as library-internal work**: queue and resource scans, pointer helpers, metaschema validation depth, `uniqueItems` equality and error-tree formatting are functions of an admitted document (depth <= 64, <= 500,000 tokens, bounded numbers and regexes), and the harness also caps the schema text with `MaxTemplateGeneratedContentLength`. They are bounded by those admissions, not by a work allowance inside the compiler.

## Appendix C

| # | Item | Done |
|---|---|---|
| 1 | MRQL parameter work | `mrql.BindParamsContext`: admits count (1,000) and bytes (4 MiB) before copying, decodes each distinct parameter once (`literalWithToken` gives each occurrence its own node and position), lists at most 10 names in missing/unknown diagnostics, checks the context. LIKE and IN translation memoize the escaped and lowered operand per translation (`likePattern`, `lowerText`; strings are no longer copied through `fmt.Sprint`). `CheckTemplateMRQLContext` is the one bounded path for lint's callback and the `check_mrql` Tool. Tests: `mrql/params_budget_test.go`, `TestCheckTemplateMRQLContextBounds`. |
| 2 | `budgetCacheKey` | `budgetCacheKeyMetered` reserves the key size on the meter before building it and ticks per parameter; the key is built once per call and reused for lookup and store. A refusal returns the meter's budget error, never a miss. A budgeted render may spend 8 MiB on key construction. `TestBudgetedExecutorAdmitsCacheKeyWork`. |
| 3 | Attribute-context substitution | `attributeContextsForW` charges the span count (maps, sort) and the substituted document bytes before allocating, steps per span, and returns before classification when the allowance is spent. `TestAttributeContextsForStopsWhenWorkIsSpent`. |
| 4 | Conditional regexes | `compileBoundedRegexp`: the parse tree is walked with saturating arithmetic to estimate the compiled program, refused above 20,000 instructions, then `regexp.Compile` (a long literal pattern is cheap and still works). Runtime caches compiled patterns per render. A refusal ends a budgeted render with the budget error; on an unbudgeted page it is local to that conditional, which never matches (the `[else]` branch renders), like an invalid pattern, so one pattern cannot blank a page. Lint reports "too complex" as an error issue. |

## Not covered, with reasons

- Plugin shortcode output is trusted code outside the bound (plan section 2); only its returned source is charged to the parsed-source budget.
- The unbudgeted `Lint` entry (classic generation, the lint endpoint) keeps its source-size behavior and gains the work allowance and optional context.
