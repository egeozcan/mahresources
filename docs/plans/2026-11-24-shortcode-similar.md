# Plan: `[similar]`, a strip of perceptual-hash lookalikes (revision 2)

Slug: `similar`. Saved as `docs/plans/2026-11-24-shortcode-similar.md` (the index script files it under "Category templates & shortcodes").

## 0. Changes since the last version

| # | Feedback | Response |
|---|---|---|
| 1 | Usefulness is modest | Partly accepted. §1 now gives the concrete surfaces where it adds value, and the design was cut back (no E2E spec, one doc file fewer). The pushback is also in §1: the lightbox, which has no similarity information today, is where photo libraries triage. |
| 2 | Drops the duplicate/similar tier | Fixed. The tier rule moves into one shared constant that both the panel (`models.Resource.SimilarityTier`) and `[similar]` use. Each thumbnail shows a visible tier badge, and the alt text includes the tier. |
| 3 | Lightbox not settled | Settled and tested. The lightbox loads `/resource.json`. `processShortcodesForJSON` (`server/routes.go`) builds an executor with `BuildQueryExecutor` and runs `CustomLightbox` through `processSlot`, so `[similar]` renders inline there. Links navigate to the resource page, the same as the panel's existing "Open resource page" link, and §4 says so. The markup is now readable on the dark lightbox ground. A test covers `/resource.json`. |
| 4 | `[using]` composition untested | Added: a unit test that `[using]` hands `[similar]` the resolved resource, and an integration test that a scoped principal's out-of-scope cover renders nothing. |
| 5 | Cache-key format | Stated in §3: every key string changes. The cache is per page render (`QueryBudget.cache` on the request context), so no stored key goes stale. The size-comment update is spelled out. |
| 6 | Second query instead of the existing `ORDER BY distance` expression | Pushed back, with the reason in §3. The flat MRQL select scans into `[]models.Resource` (`executeSingleEntity` → `executeMRQLFind`). `SimilarityDistance` is `gorm:"-"`, so a projected column cannot land there without changing the shared executor and `MRQLResult` for every caller. The second query is two seeks on a unique index for at most 24 IDs. |
| 7 | "with this resource" is ambiguous under `[using]` | Fixed. The sr-only text names the target resource. |
| 8 | Literal `[similar]` text | No info hint (pushback in §9). Any valid use would trigger it, so it would be noise. An existing literal is not silently swallowed: it renders either a visible error marker or a strip. The release note stays. |

## 1. Motivation and user value

Photo and asset libraries collect duplicates, re-exports, crops and edits. The hash worker already stores those pairs in `resource_similarities`. Today they reach the user in only two ways:

- **The resource page's "Similar Resources" panel** (`templates/displayResource.tpl`, gated by `sc.SimilarResources`). It shows a tier badge and a Compare link, but only in its fixed position in the main column.
- **MRQL `SIMILAR TO resource(N)`**, which needs a literal ID.

Where it matters:

- **The lightbox.** This is where photo libraries actually triage, with keyboard quick-tagging and paging through a group. The lightbox details panel has no similarity information at all. A Resource Category can now put "Near-certain duplicate · 1 · Compare" next to the image being reviewed.
- **The sidebar and detail footer**, when a category turns off the built-in panel (`SectionConfig`) to get a custom layout but still wants lookalikes in a compact strip.
- **Group pages**, through `[using type="resource" id-path="cover"][similar][/using]`, to show whether a group's cover image has duplicates elsewhere.

The `[mrql]` workaround cannot do any of this:

1. `parseSimilarTo` accepts only an integer literal, and a shortcode cannot put the current ID into query text.
2. MRQL results carry no distance or tier.
3. `[mrql]` has no accessible thumbnail layout.

The value is concentrated in image-heavy libraries, and for non-images the strip renders nothing. That is an honest limit, so the design stays small: one built-in, one opt-in executor flag, one helper method.

It is not on the roadmap in `docs/ideas/category-templates-and-shortcodes.md`. `[plugin:widgets:gallery]` is owner-scoped and has no similarity source.

## 2. Syntax and attributes

```
[similar]
[similar max-distance="6" limit="8" size="sm" label="Lookalikes"]
```

- Inline only (`IsBlock: BlockNo`).
- It always targets the current entity, which must be a resource.
- To target another resource, compose it with `[using]`. There is no `id` attribute.

| Attribute | Type | Default | Valid values | Meaning |
|---|---|---|---|---|
| `max-distance` | number | *(unset: runtime `hash_similarity_threshold`, the panel's threshold)* | integer 0–11 (`mrql.MaxSimilarityDistance`) | Largest effective distance (`COALESCE(p_distance, hamming_distance)`). Maps to `WITHIN <d>`. |
| `limit` | number | `8` | integer 1–24 | Maximum thumbnails, nearest first. |
| `size` | enum | `md` | `sm`, `md`, `lg` | Thumbnail edge 48 / 96 / 200 px, matching `[plugin:widgets:gallery]`. |
| `label` | string | `Similar resources` | any text; empty or whitespace means the default | Visible caption, which is also the strip's accessible name. |

`parseSimilarAttrs(attrs) (similarOptions, problem string)` in `shortcodes/similar_handler.go` holds the rules. The renderer and the linter both call it.

On invalid input the page renders `shortcodeErrorMarker("similar", problem)` and lint reports a `SeverityError` with the same text. The exact messages are:

- `[similar] max-distance must be a whole number from 0 to 11`
- `[similar] limit must be a whole number from 1 to 24`
- `[similar] size must be "sm", "md" or "lg"`

An unknown attribute already gets the generic lint warning, because `KnownFromBuiltins` is derived from `BuiltinDocs`.

## 3. Execution

### Shared tier rule (new, one place)

- Add `constants.SimilarityDuplicateMaxDistance = 2` in `constants/constants.go`.
- `models.Resource.SimilarityTier()` uses it in place of its literal `2`. `models` already imports `constants`, and the method's behaviour does not change.
- `shortcodes` imports `constants`. It is a leaf package, so this adds no import cycle.
- The helper `similarTierLabel(d int) (badge, spoken string)` returns:

  | Distance | Badge | Spoken form |
  |---|---|---|
  | `d <= SimilarityDuplicateMaxDistance` | `"Duplicate"` | `"near-certain duplicate"` |
  | otherwise | `"Similar"` | `"similar"` |

  These are the panel's two tiers and its wording.

### Handler: `RenderSimilarShortcode(reqCtx, sc, ctx, executor)`

1. Parse the attributes. If there is a problem, return the error marker.
2. If `ctx.EntityType != "resource"` or `ctx.EntityID == 0`, return `shortcodeErrorMarker("similar", "[similar] works only on a resource; use [using type=\"resource\" …] to target one")`.
3. Build the query `fmt.Sprintf("type = resource AND SIMILAR TO resource(%d)%s ORDER BY distance ASC", ctx.EntityID, within)`.
   - `within` is `" WITHIN <d>"` or `""`.
   - Only validated integers and the context's own `uint` go into the text.
   - Ties break by ascending ID, through `stableMRQLEntityOrder`.
4. Call the executor with `QueryOptions{Limit: limit, ScopeGroupID: 0, WantSimilarityDistance: true}`.
   - Global scope is deliberate, because duplicates cross groups.
   - A confined principal is still forced to its subtree root by `ExecuteMRQLScoped` → `effectiveMRQLRequestedScope`.
5. Handle the result:
   - `ErrQueryUnavailable`: return `unavailableComment(err)`.
   - Any other error, including budget exhaustion: return `mrqlErrorHTML(err, false)`, as `[using]` does.
   - `!meterFrom(reqCtx).alive()`: return `""`.
   - Nil result, `Mode != "flat"`, or zero items: return `""`.
6. Render at most `limit` items and skip any item that is not a resource.

The output is final HTML and is never passed back through `processWithDepth`. A resource named `[meta path="x"]` stays escaped text.

### Dispatch (`shortcodes/processor.go`)

```go
case sc.Name == "similar":
    if executor != nil {
        replacement = RenderSimilarShortcode(reqCtx, sc, ctx, executor)
    } else {
        replacement = shortcodeComment("similar unavailable in this context")
    }
```

### Which surfaces have an executor

- **Resource page slots** (`process_shortcodes` tag).
- **Deferred `[lazy]`/`[details]` bodies** (`/v1/shortcodes/deferred`).
- **Lightbox.** `/resource.json`, which the lightbox fetches in `src/components/lightbox/navigation.js`, runs `processShortcodesForJSON` → `processSlot(r.ResourceCategory.CustomLightbox, metaCtx)` with `BuildQueryExecutor(appCtx)`. So `[similar]` renders inline in `CustomLightbox`, and in `CustomSidebar` when the lightbox falls back to it.
- **Template previews** go through the privacy and capped executors.
- **Share pages** have no executor, so they get the comment.

### Distance enrichment (the only new data path)

The new fields and flag:

- `shortcodes.QueryOptions.WantSimilarityDistance bool`: off by default.
- `shortcodes.QueryResultItem.SimilarityDistance *int`.
- `budgetCacheKeyMetered` (`shortcodes/query_budget.go`) appends `|%t` for the flag.
  - This changes the string format of every key.
  - That is harmless, because the cache lives on the per-render `QueryBudget` on the request context, and nothing persists keys across requests.
  - The size comment becomes "five numbers of at most 20 digits, seven separators, two booleans and four pairs of quotes: well under 160". The real worst case is about 121 bytes, so the reserved `160` stays.

New and changed code:

- **`mrql.SimilarToTarget(q *Query) (int64, bool)`** in `mrql/translator.go`. It wraps `findSimilarTarget(q.Where)` and is ok only for exactly one `SIMILAR TO`. The target comes from the parsed query, not the caller.
- **`executeMRQLForShortcode`** (`shortcode_query_executor.go`), on the flat path after `convertResultItems`, acts only when the flag is set, the entity type is resource, `SimilarToTarget` is ok, and there is at least one item. It collects the item IDs, calls `appCtx.SimilarityDistancesTo(reqCtx, uint(target), ids)`, and assigns the distances.
- **`(*MahresourcesContext) SimilarityDistancesTo(reqCtx, targetID uint, ids []uint) (map[uint]int, error)`** in `application_context/resource_crud_context.go`, next to `getSimilarResourcesLimited`:
  - It runs this query through `ctx.db.WithContext(reqCtx).Raw(...).Rows()` and keeps the minimum per ID:
    ```sql
    SELECT resource_id2 AS id, <mrql.SimilarPairDistance("resource_similarities")> AS d
      FROM resource_similarities WHERE resource_id1 = ? AND resource_id2 IN ?
    UNION ALL
    SELECT resource_id1, <same> FROM resource_similarities WHERE resource_id2 = ? AND resource_id1 IN ?
    ```
  - Empty `ids` returns an empty map without a query. More than 100 IDs returns an error.
  - It is added to `QueryExecutorContext` and `mrqlShortcodeRunner` in `shortcode_interfaces.go`.
- **Why `Raw` is safe here.** The reason is recorded in a code comment and pinned by a test:
  - It reads only pair rows (IDs and a distance).
  - Its `ids` are exactly what the principal-scoped query just returned.
  - For an invisible target, that query returns nothing (`translateSimilarTo` and the `visibleRelated` guard), so the method is never reached.
- **Why not reuse the `ORDER BY distance` expression** (`resolveOrderByColumn`). That expression lives only in ORDER BY. Selecting it as a column would need the flat select to scan into something other than `[]models.Resource`, whose `SimilarityDistance` is `gorm:"-"`. That would change `executeSingleEntity`, `MRQLResult` and every MRQL caller to serve one shortcode. The opt-in second query touches no shared path.
- **Budget and cache.** It costs no extra budget unit (the same rule as `WantTotal`). `cloneQueryResult` shares the `*int` read-only.
- **Privacy.** In `TemplatePrivacy.queryItems` (`application_context/template_privacy_entity.go`) add `if it.SimilarityDistance != nil { p.keepScalar(int64(*it.SimilarityDistance)) }`.

No other caller sets the flag, so `[mrql]` is unchanged.

## 4. Rendered output (accessible markup)

The match name, match hash and target name are read with reflection: a new `readStringField` next to `readUintField` in `link_handler.go`. The target name is `readStringField(ctx.Entity, "Name")`, falling back to `resource <EntityID>`. Under `[using]`, `ctx` is the resolved cover, so the target is always the resource the strip belongs to.

```html
<figure class="similar-strip my-2" data-similar-for="42">
  <figcaption class="text-xs font-semibold mb-1">Similar resources</figcaption>
  <ul class="flex flex-wrap gap-2" role="list">
    <li class="flex flex-col items-center gap-0.5">
      <a href="/resource?id=17" class="block rounded focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-500">
        <img src="/v1/resource/preview?id=17&amp;width=96&amp;height=96&amp;v=HASH" width="96" height="96"
             loading="lazy" decoding="async" alt="Beach sunset (2), near-certain duplicate, distance 1"
             class="rounded object-cover h-24 w-24 bg-stone-100">
      </a>
      <span class="text-[11px] font-medium rounded px-1 text-rose-800 bg-rose-100" aria-hidden="true">Duplicate · 1</span>
      <a href="/resource/compare?r1=42&amp;r2=17" class="text-xs underline inline-flex items-center min-h-6 px-1 rounded focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-500">Compare<span class="sr-only"> Beach sunset (2) with Beach sunset</span></a>
    </li>
  </ul>
</figure>
```

Rules:

- **Strip name.** `<figure>`/`<figcaption>` names the strip with no generated `id`, so the sidebar and lightbox copies never collide. It adds no landmark per card.
- **Alt text.** `"<name>, <spoken tier>, distance <d>"`. An empty name gives `Resource <id>`. A nil distance (a pair deleted between the two queries) gives the name only, with no badge. The link's accessible name is the alt. `role="list"` keeps list semantics under Tailwind's list reset.
- **Badge.** It carries its own background, which works on both the white page and the dark lightbox panel:
  - "Duplicate" uses `text-rose-800 bg-rose-100` and "Similar" uses `text-stone-700 bg-stone-100`, the panel's colours (both above 7:1).
  - It is `aria-hidden` because the alt already says it.
  - Its wording is the short form of the panel's "Near-certain duplicate · distance N", so it fits under a 48 px thumbnail.
- **Colours inherit.** The caption and the Compare link set no text colour, so they take the slot's colour: stone on the page, `text-stone-300` in the lightbox. The focus outline `amber-500` is above 3:1 against both white and `stone-900`.
- **Compare link.** The visible "Compare" starts its accessible name (WCAG 2.5.3). The name then says which two resources are compared, using the target's name, never "this resource". `min-h-6` gives a 24 px target (WCAG 2.5.8). The URL matches the panel's: `r1=<target>&r2=<match>`.
- **Lightbox behaviour.** The links are ordinary navigations. Following one leaves the lightbox for the resource or compare page, the same as the lightbox's own "Open resource page" link. The lightbox's `canShortcut` already ignores Enter on a focused `a[href]`, so keyboard activation is not hijacked.
- **Escaping.** Names, the label and the target name use `html.EscapeString`. The hash uses `url.QueryEscape` then `html.EscapeString` (a Placeholder in Privacy mode). IDs and sizes are integers.
- **Sizes.** `sm` is `h-12 w-12`/48, `md` is `h-24 w-24`/96, `lg` is `h-48 w-48`/200. The `width`/`height` attributes prevent layout shift.
- **CSS scan.** Run `./scripts/css-scan-test.sh` to confirm the Go-source classes survive.

## 5. Empty, missing and invalid input

| Situation | Output |
|---|---|
| No stored pairs, or every match filtered out by distance or scope | `""` |
| Group or note context, or no ID | "works only on a resource" error marker |
| Invalid `max-distance`, `limit` or `size` | error marker with the exact §2 message, and the same lint error |
| Empty `label` | default caption |
| No executor (share pages) | `<!-- mr:similar unavailable in this context -->` |
| `ErrQueryUnavailable` | `unavailableComment(err)` |
| Budget exhausted or another error | `mrqlErrorHTML(err, false)` |
| Meter stopped | `""` |
| Written as a block | the existing `BlockNo` handling and lint structural check |

There is no fallback to exact `image_hashes` matching. It reads precomputed pairs only.

## 6. Scoping and authorization (fail closed)

- **Confined principal.** Global scope requested as `0` becomes the forced subtree root. A deny principal gets `UnresolvedScopeSentinel`, which means no rows.
- **Target outside the subtree.** `translateSimilarTo` adds the visibility predicate, so there are no rows and the distance query never runs.
- **Under `[using]`.** The `[using]` lookup itself is scoped. An out-of-scope cover resolves to "no target" and renders `[using]`'s else branch (or nothing), so `[similar]` never runs.
- **Ownerless resources and resources in other groups** are excluded by the subtree predicate and never reach `SimilarityDistancesTo`.
- **Nothing leaks.** The output holds only the IDs, names and hashes of returned rows, plus the target's own name and ID, which the caller is already viewing. "No pairs" and "pairs you may not see" look the same.
- **Privacy mode** masks names, hashes and the target name. Distances and tiers stay readable.

## 7. Performance at millions of resources

Each `[similar]` costs one budget unit, or nothing on a cache hit, and runs:

- **(a) One MRQL select.** `idx_sim_r1`/`idx_sim_r2` seeks, ordered by the existing correlated distance expression, with `LIMIT ≤ 24`. Its cost scales with the cluster size, not the library size, and it is the shipped `SIMILAR TO … ORDER BY distance` path.
- **(b) The distance query.** At most 24 IDs and two seeks on the unique `idx_sim_pair`.

There is no hashing, no `image_hashes` scan and no N+1 at render time. On list cards, the per-page `-mrql-page-query-budget` caps total work exactly as it does for `[mrql]`. The docs recommend `CustomSidebar`, `CustomLightbox` and `CustomDetailFooter`. The lightbox renders one resource per `/resource.json` fetch, so the cost there is one instance.

## 8. Where it lives: a built-in Go handler

- Lua has no similarity API, and adding one would be a larger surface than one executor flag.
- Built-ins on the principal-scoped executor inherit subtree confinement, Privacy masking and share-page handling. Plugin shortcodes are gated per plugin for scoped accounts.
- The enrichment must be in Go anyway.

## 9. Parser change and existing literal text

Add `similar` to `shortcodePattern` and `closingTagPattern` in `shortcodes/parser.go`, next to `using`.

Any literal `[similar …]` already in a template becomes a shortcode. No lint "info" hint is added, because it would fire on every valid use, and the linter cannot tell intended uses from literal ones. A former literal never disappears silently:

- on a non-resource it renders a visible error marker;
- on a resource it renders a strip or nothing;
- a malformed attribute gets a lint error.

The release note in `CHANGELOG`/docs says that `[similar]` is now reserved.

## 10. Files to change

**Constants and models**

- [x] `constants/constants.go`: `SimilarityDuplicateMaxDistance = 2`.
- [x] `models/resource_model.go`: `SimilarityTier` uses the constant.

**Shortcodes package**

- [x] `shortcodes/similar_handler.go` (new): `parseSimilarAttrs`, `similarTierLabel`, `RenderSimilarShortcode`, `defaultSimilarLimit = 8`, `maxSimilarLimit = 24`, and the size table.
- [x] `shortcodes/processor.go`: the dispatch case, plus the `QueryOptions.WantSimilarityDistance` and `QueryResultItem.SimilarityDistance` fields.
- [x] `shortcodes/parser.go`: both regexes.
- [x] `shortcodes/query_budget.go`: the key and its size comment.
- [x] `shortcodes/lint.go`: `"similar"` in `builtinBaseNames`, and a `case "similar":` that calls `parseSimilarAttrs` and reports `SeverityError`.
- [x] `shortcodes/link_handler.go`: `readStringField`.
- [x] `shortcodes/builtin_docs.go`:
  - a `BuiltinDoc` with syntax, a description that mentions the tiers, `BlockNo`, four attrs (`Enum` for `size`) and three examples: the default form, `[similar max-distance="2" limit="12" size="sm" label="Possible duplicates"]`, and the `[using]` form;
  - the doc comment's list of names.

**MRQL**

- [x] `mrql/translator.go`: `SimilarToTarget`.

**Server and application context**

- [x] `server/template_handlers/template_filters/shortcode_query_executor.go`: the enrichment step.
- [x] `server/template_handlers/template_filters/shortcode_interfaces.go`: the new method on both interfaces.
- [x] `application_context/resource_crud_context.go`: `SimilarityDistancesTo`.
- [x] `application_context/template_privacy_entity.go`: `keepScalar`.

**Templates and docs**

- [x] `templates/createResourceCategory.tpl`, `createCategory.tpl`, `createNoteType.tpl`: reference panel entries. The latter two point to `[using]`. `TestReferencePanelsCoverAllBuiltins` requires all three.
- [x] `docs-site/docs/features/shortcodes.md`:
  - the table row ``| `[similar]` | Inline | Thumbnails of perceptually similar resources, with tier, distance and a compare link |`` (required by `TestDocsSiteShortcodesPageCoversAllBuiltins`);
  - a "`[similar]` -- Lookalike Thumbnails" section covering attributes, tiers, output, empty behaviour, scope, budget, lightbox navigation, the `[using]` example and the reserved-name note.
- [x] `docs-site/docs/features/custom-templates.md`: one line in the resource-category slot guidance about `CustomSidebar`/`CustomLightbox`.
- [x] `.claude/skills/mahresources-category-designer/references/shortcodes.md`: the entry.

## 11. Tests

### `shortcodes/similar_handler_test.go` (new, fake executor)

1. **Query and options.**
   - The default query is exactly `type = resource AND SIMILAR TO resource(42) ORDER BY distance ASC`, with Limit 8, ScopeGroupID 0 and WantSimilarityDistance true.
   - `max-distance="6"` adds ` WITHIN 6`, and `"0"` adds ` WITHIN 0`.
2. **Markup.**
   - Structure: one `figure`/`figcaption`/`ul[role=list]` and one `li` per item, in order.
   - Image: 96 px by default (48 and 200 for `sm` and `lg`); `src` with an escaped hash (seed `&` and `"`).
   - Compare link: `href="/resource/compare?r1=42&amp;r2=17"`; sr-only text "` Beach sunset (2) with Beach sunset`" taken from the context entity's name, falling back to `resource 42` when the entity has no name.
   - Caption: a custom label, and the default for an empty label.
3. **Tiers.**
   - Distance 0 and 2 give the `Duplicate · d` badge (rose) and the alt `…, near-certain duplicate, distance d`. Distance 3 gives `Similar · 3`.
   - `similarTierLabel` and `(&models.Resource{SimilarityDistance: &d}).SimilarityTier()` agree for d = 0–11. This is a table test in the `shortcodes` test package, which already imports `models` in tests.
4. A name with `"<b>[meta path="x"]` is escaped in the alt and the sr-only text and is not expanded (no extra executor calls). An empty name gives `Resource 17`.
5. A nil distance gives the name-only alt and no badge.
6. Zero items gives `""`. More items than `limit` renders `limit`. Non-resource items are skipped.
7. A note or group context, and `EntityID 0`, give the error marker.
8. Invalid attributes give the exact messages, across a table: `size="xl"`; `max-distance` of `12`, `-1`, `abc` and `2.5`; `limit` of `0`, `25` and `x`.
9. Through `Process`: a nil executor gives the comment, `ErrQueryUnavailable` gives `unavailableComment`, and another error gives the MRQL error box.
10. **`[using]` composition.**
    - Template: `Process` on a group context with `[using type="resource" id-path="cover"][similar][/using]`. The fake executor answers the lookup `type = resource AND id = $id` with resource 99 ("Cover") and records a second call.
    - The second call's query is `… SIMILAR TO resource(99) …`.
    - The sr-only text says "with Cover", and the compare link has `r1=99`.
    - When the lookup returns no items (hidden or missing), only one call is recorded and the output is empty.

### `shortcodes/lint_test.go`

- Each invalid attribute gives a `SeverityError` with the runtime's message.
- A valid `[similar]` gives no issues.
- An unknown attribute warns.
- `[simlar]` gets the near-miss suggestion.

### `shortcodes/query_budget_test.go`

- Keys that differ only by `WantSimilarityDistance` are different.
- The worst-case fixed part of a key, with every number at its maximum digits, fits in the reserved 160 bytes.

### `mrql/similar_to_test.go`

- `SimilarToTarget` returns the target for one predicate, false for none, and false for two predicates (OR).

### `server/template_handlers/template_filters/shortcode_query_executor_test.go`

- With the flag, distances are attached per item.
- Without the flag, or without exactly one `SIMILAR TO`, there are no distances and a recording fake shows zero `SimilarityDistancesTo` calls.

### `server/api_tests/similar_shortcode_test.go` (new, real DB, `seedSimPair`)

- **Ordering and limit.**
  - Seed r1 with pairs at p-distance 1, 5 and 9, plus a legacy pair with nil `PDistance` and Hamming 4.
  - The default renders 1, 4, 5, 9 in order, with tiers Duplicate, Similar, Similar, Similar.
  - `max-distance="6"` drops the 9. `limit="1"` keeps only the 1.
- **Full page.** `CustomSidebar = [similar]`, then GET `/resource?id=r1`. The response has the strip and the compare link.
- **Lightbox path.**
  - `CustomLightbox = [similar]`, then GET `/resource.json?id=r1`.
  - Decoded `resourceCategory.CustomLightbox` contains `figure class="similar-strip`, the r2 link and `Duplicate · 1`, with no shortcode text left.
  - Repeat with `CustomLightbox` empty and `CustomSidebar = [similar]`: the `CustomSidebar` field is expanded the same way, which is the lightbox fallback.
- **Scoped principal.**
  - Fixture (`setupAuthEnv`): a root group and an outside group; r1 and r2 in root, r3 in outside, r4 ownerless; pairs r1–r2, r1–r3 and r1–r4.
  - POST `/v1/shortcodes/deferred` with `signToken(tc, "resource", r1.ID, "[similar]")` and `scopedUserBearer(root)`.
  - The output has r2 and none of r3's or r4's IDs or names in any `href`, `src` or alt.
  - An admin sees all three.
- **Scoped `[using]`.**
  - Fixture: a group G in root whose meta `cover` is r3, which is outside the scope.
  - Sign a group token for G with `[using type="resource" id-path="cover"][similar][/using]` and use the scoped bearer.
  - The output contains no `similar-strip` and no r3 or r1 data.
  - The admin sees r3's strip.
- **Hidden target.** The executor called directly with `accountCtx(..., &root.ID)` on a query targeting r3 returns no items, and `SimilarityDistancesTo` is not reached.
- **Privacy mode.** Extend the existing privacy preview tests with `[similar]`. The neighbour and target names do not appear, the distance and tier do, and the guard does not trip.

### `server/api_tests/similar_shortcode_pg_test.go`

- Through the existing PG helper, `SimilarityDistancesTo` and the rendered order and distances match SQLite. This covers the smallint `COALESCE` and `UNION ALL` with IN lists.

### E2E

No new spec. E2E runs with the hash worker off and no API seeds pairs, so a browser spec could only check the empty case. The populated markup, the lightbox JSON path and scoping are proven by the Go tests above. The a11y properties (alt, label-in-name, contrast through self-backgrounded badges and inherited colours) are asserted on the markup in unit tests.

### Commands

```
go test --tags 'json1 fts5' ./...
./scripts/css-scan-test.sh
```

The Postgres tests run under the existing PG setup.

## 12. Success criterion

On a resource whose category's `CustomSidebar` or `CustomLightbox` contains `[similar]`, on both the detail page and in the lightbox:

- precomputed lookalikes within the threshold (or `max-distance`) render nearest first, up to `limit`;
- each one shows a tier badge (Duplicate or Similar, with the same cut-off as the built-in panel) and the distance;
- its alt text says the name, tier and distance;
- a Compare link names both resources and opens `/resource/compare?r1=<target>&r2=<match>`;
- with no pairs, nothing renders.

These must also hold:

- Through `[using]`, the strip belongs to the resolved resource, and an out-of-scope target renders nothing.
- A group-confined user never sees, in any attribute, a resource outside their subtree or an ownerless one.
- The linter and the page give identical messages for invalid attributes.
- Each instance costs at most one budget unit and two index-bounded queries.
- `[mrql]`, the built-in panel's tiers and every other shortcode are unchanged.
- All the tests above pass on SQLite and Postgres.

## 13. Review (implementation record)

Built as planned, with these differences:

- **`[reload]` face.** Not in the plan: `[similar]` emits a `<figure>` of links, which a `<button>` may not contain, so like `[adjacent]` it renders an error marker inside a `[reload]` face and the linter reports it (`TestSimilarRefusedInsideAReloadFace`, `TestLintSimilar`).
- **Lint tests** live in a new `shortcodes/similar_lint_test.go` rather than `lint_test.go`, following `adjacent_lint_test.go`. `TestLintSimilarMatchesRenderer` pins that lint and page give the same message.
- **Privacy mode** is covered by a unit test of the masker (`application_context/template_privacy_similar_test.go`: names and hashes masked, distance and tier kept, `CheckOutbound` passes) rather than by extending the harness-run privacy preview tests. A distance is at most two digits, under the guard's four-character floor, so the new `keepScalar` call keeps the corpus complete but cannot change a guard verdict.
- **`budgetCacheKey` format.** `TestBudgetCacheKeyUnchangedForOrdinaryInputs` pins the literal key and was updated for the extra boolean.
- **Template preview** (`/v1/resourceCategory/previewTemplate`) is asserted in `TestSimilarShortcode_PageAndLightbox`, along with the page, `CustomLightbox` and the `CustomSidebar` fallback in `/resource.json` (read at `resource.resourceCategory`, which the lightbox uses).
- No CHANGELOG exists; the reserved-name note is in `docs-site/docs/features/shortcodes.md`.

Tests run: `go build --tags 'json1 fts5' ./...`; `go test --tags 'json1 fts5'` over `./shortcodes/... ./mrql/... ./models/... ./server/... ./application_context/... ./internal/arch/... ./plugin_system/...` (pass); `TestSimilarShortcode_Postgres` under `-tags postgres` (pass); `./scripts/css-scan-test.sh` (pass). No E2E spec, as planned.
