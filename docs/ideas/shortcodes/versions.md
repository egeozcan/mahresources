<!-- Shortcode idea that did not make the first implementation round. -->
> **Status:** idea, not implemented. The plan went through 3 plan/evaluate round(s).
> Last evaluation: consistency 7/10, usefulness 7.4/10, average 7.20/10. The bar was 8.5.
>
> **Pitch:** Lists a resource's ResourceVersion rows (version number, date, size delta, content-type change, comment) as a compact semantic table, with links to the version download and the existing compare view: [versions limit="5" show="size,date,comment"]. A block form exposes [item] per version for custom layouts. Media and document categories can surface 'last revised 3 days ago, v7' and a change trail on the detail page or sidebar without opening the versions tab.

# Plan: `[versions]`, a resource version history shortcode (revision 3)

Slug: `versions`. Plan file to create: `docs/plans/2026-11-24-versions-shortcode.md`. After creating it, run `./docs/plans/generate-index.sh`.

## 0. Changes from revision 2, by evaluator point

1. **Usefulness: the design now starts from the list-card status line.**
   - The headline use is `[versions limit="1"]v[item path="number"] · [item path="created" format="date"] · [item path="comment"][/versions]` in a Resource Category `CustomSummary` or `CustomMRQLResult`. The docs show this first. The inline form is secondary.
   - The inline form is no longer a 4-column table. It is a stacked `<ol>`, one short line per version, so it fits list cards as well as sidebars (point 5).
   - **Pushback on a batched per-page preload.**
     - Cards render one at a time through `process_shortcodes`. At render time a shortcode cannot see the page's set of resource IDs.
     - Batching would need a prefetch in every list surface: the resources list, group owner panels, `seeAll.tpl`, series and similar panels, hovercards, `/mrql` cards and the dashboard. It would also need a template scan to decide when to prefetch. That breaks the "surgical changes" rule for a gain of about 1 ms per card.
     - What I did instead keeps per-card cost bounded and stops `[versions]` from competing with `[mrql]`: a separate sub-budget (point 2) and the per-page memo.
     - I cannot supply usage data showing how often the need arises. The argument is structural. Nothing on any list surface shows that a file has been revised, and the panel that does show it is collapsed by default and only exists on the detail page and in the lightbox. Categories that never use the shortcode pay nothing.
2. **Performance: `[versions]` gets its own sub-budget.**
   - `QueryBudget` gets a second counter for `[versions]`. Its cap is the same `-mrql-page-query-budget` value, so there is no new flag. A page full of `[versions]` therefore cannot starve its `[mrql]` blocks, and the reverse holds too.
   - Hitting the cap logs a `[versions]`-specific `/logs` warning once per page.
   - The configuration doc row and the shortcodes doc say this plainly.
   - **Pushback on skipping the authorizing resource-row read when the caller "already holds the entity".**
     - `MetaShortcodeContext.Entity` is `any` and reaches the handler from about ten producers: display pages, the lightbox, deferred bodies rebuilt from a signed token, `[using]`, `[mrql]` result cards, entity-picker previews and template previews.
     - Proving that every one of them loaded the entity through the request principal's scope would be a fragile invariant held by convention. AGENTS.md says scoping must fail closed.
     - The read is a primary-key lookup of about 1 ms. It stays.
     - It is also the only check that covers `resource_versions`, because that table is not in `scopeColumn`.
3. **`-mrql-page-query-budget=0` is specified.**
   - `WithQueryBudget` is installed on every surface whatever the limit (`shortcode_tag.go:191`, `mrql_api_handlers.go:247`). `0` only removes the cap; the existing doc comment on `QueryBudget` says so.
   - So with `0`, the `[versions]` memo still dedupes, and the versions counter is uncapped too.
   - Section 11 adds a test for this.
4. **`/mrql` limitation removed instead of documented.**
   - A confined user on a `WithMRQLPrincipal` handle used to get a refusal. Now the adapter derives a properly scoped handle (`WithPrincipalContext`), lazily, at most once per page render, the first time `[versions]` runs there.
   - That costs one subtree resolution per `/mrql` page that actually uses `[versions]` for a confined user. Every scoped display page already does the same work on every request.
   - If resolution fails, it fails closed and renders the "unavailable" text.
   - The "unavailable" state is now **visible** muted text (`Version history unavailable`), never a bare HTML comment, so it cannot look like a broken card.
   - The HTML comment remains only where no lister exists by design: the share server and the model preview.
5. **Narrow cards.**
   - The inline form is a stacked list with no columns. Each item wraps as normal inline text.
   - The a11y test now names its containers exactly: the list-page card `article.resource-card` and the detail page sidebar region. In both, it asserts that `scrollWidth <= clientWidth` at 320 px.
6. **Minor clarifications.**
   - **`v=<hash>`.** `GetResourceContentHandler` accepts it. At `resource_api_handlers.go:186`, `v == resource.Hash` switches on immutable caching, and `versionPanel.tpl` already builds the same URL.
   - **`Hash`.** It is loaded only to build the synthesized v1 `downloadUrl`. It is not an `[item]` field.
   - **`[else]` on a non-resource entity.** The rule is that `[else]` means "this resource has no history to show". A non-resource context is an authoring error, not missing data, so it gets the error marker in both forms. That keeps the mistake visible in template previews instead of being hidden behind fallback text. `[each]`'s `[else]` covers a missing or empty meta value, which is data, so the two shortcodes are consistent.
   - **`delta`, `typeChanged`, `compareUrl` and "first version".** These are computed from the next row in the fetched slice, and the slice holds up to `limit+1` rows. So every rendered row has a predecessor whenever an older version exists. These fields are nil, false or `""` only on the resource's oldest version.
   - **Other edits kept.** The skill-reference edit, the reference-panel edits and the per-row `charged{m}` tick stay as they were.
   - **Message helper reverted.** I dropped revision 2's shared `budgetExceededError` refactor of `BudgetedExecutor`. `[versions]` has its own budget and its own message, so `[mrql]` code stays untouched.

## 1. Motivation and user value

Resources carry a version history (`models.ResourceVersion`: number, date, size, content type, comment). Today the only place to see it is the collapsible version panel on the resource page (`versionPanel.tpl`) and in the lightbox (`lightboxVersionPanel.tpl`).

A template cannot reach it:
- No MRQL field covers `resource_versions`.
- `[property]` walks only the loaded struct, and `Versions` and `CurrentVersion` are never preloaded on render surfaces.

What users get:
- **Status line on cards (block form, the primary use).** A Resource Category for documents, design files or renders puts this in `CustomSummary`:

  ```
  [versions limit="1"]v[item path="number"] · [item path="created" format="date"] · [item path="comment"][/versions]
  ```

  Every card on the resources list, on group owner panels and in `[mrql]` result lists then reads "v7 · 2026-11-21 · colour pass". A reader can see which files changed and why without opening each one. No surface offers this today.
- **Short change trail (inline form).** `[versions limit="3"]` renders a compact stacked list in a card or sidebar, for example on a project group page that lists its documents through `[mrql query="type = resource AND ownerId = [property path=ID]"]`.
- On the resource page itself the existing panel stays the full tool, with thumbnails, restore, multi-select compare and upload. The docs say so.

Dates are absolute. Relative "3 days ago" text would go stale in cached or deferred bodies, and the existing `[item]` formats are absolute.

Nothing in `docs/ideas/category-templates-and-shortcodes.md` overlaps this.

## 2. Where it lives: a built-in Go handler, not a plugin

- **Scoping.** Plugin Lua runs on the unscoped DB handle, and every render surface refuses `[plugin:*]` for group-confined principals. A plugin version of this shortcode would be missing for exactly the users whose scoping matters. A built-in reads through the request principal's `MahresourcesContext`.
- **Shared machinery.** It needs the render meter, the page `QueryBudget`, `[item]` splicing (`markItems`, `spliceLazy`, `renderItemValueObserved`) and `splitElseM`. All of these are package-private in `shortcodes/`.
- **No DB in `shortcodes/`.** Data arrives through a per-render callback, `VersionLister`, which is put on the request context the way `PartialResolver` is (`WithPartialResolver` / `BuildPartialResolver`).

## 3. Syntax and attributes

```
[versions]                 inline: stacked list, latest 5
[versions limit="3"]       inline: latest 3
[versions limit="1"]v[item path="number"] · [item path="comment"][else]No history[/versions]   block
```

`IsBlock: BlockOptional`. With a body it is the block form; without one it is the inline form.

| Attr | Type | Default | Rules |
|---|---|---|---|
| `limit` | number | `5` | Number of versions rendered, newest first (`version_number DESC`). Empty, non-integer or `<= 0` gives `5` (`parseIntAttr`). Values above `50` are clamped to `50` (`maxVersionsLimit`). |

There are no other attributes. An unknown attribute is ignored at runtime, and lint flags it through the existing unknown-attribute rule, which runs off `BuiltinDocs()`.

### `[item]` fields in the block form

Each version is a `map[string]any` rendered through `renderItemValueObserved`, the same path `[each]` uses. So `format`, `layout`, `default`, `raw` and `index="true"` behave exactly as documented for `[item]`.

| path | Go type | Meaning |
|---|---|---|
| `number` | int | Version number |
| `id` | uint | Version row ID. `0` for a synthesized v1 (section 5). |
| `created` | time.Time | Creation time. Use `format="date"`, `format="datetime"` or `layout=`. |
| `size` | int64 | Bytes. `format="filesize"` humanizes it. |
| `delta` | int64 or nil | `size` minus the predecessor row's size. The predecessor is the next row in the fetched `limit+1` slice. nil only on the resource's oldest version. |
| `contentType` | string | MIME type |
| `typeChanged` | bool | The content type differs from the predecessor's. false on the oldest version. |
| `comment` | string | Version comment |
| `current` | bool | `id == *CurrentVersionID`, or the synthesized v1 |
| `downloadUrl` | string | `/v1/resource/version/file?versionId=<id>`. For a synthesized v1 it is `/v1/resource/view?id=<resourceID>&v=<hash>`, the same URL `versionPanel.tpl` builds, because `versionId=0` returns 404. |
| `compareUrl` | string | `/resource/compare?r1=R&r2=R&v1=<predecessor number>&v2=<this number>`, the format `versionPanel.tpl` uses. It uses the predecessor's number because numbers can have gaps. `""` on the oldest version. |

Body semantics are identical to `[each]`:
- The body is processed once per version against the parent resource's context, so `[meta]`, `[property]`, `[conditional]` and `[link]` read the resource.
- `[item]` binds only at the body's top level. An `[item]` nested in `[conditional]`, `[lazy]`, `[details]`, `[autorefresh]` or `[using]` renders nothing. Lint flags the `[using]` case, as it does for `[each]`.
- `[else]` is a top-level divider with no closing tag. It renders against the resource only when the history is nil (section 5).

## 4. Rendering output

### Inline form

```html
<ol class="versions-shortcode text-sm space-y-1" aria-label="Version history, latest 3">
  <li class="break-words">
    <span class="font-mono font-medium">v7</span>
    <span class="px-1.5 py-0.5 text-xs bg-amber-100 text-amber-800 rounded">current</span>
    <span aria-hidden="true" class="text-stone-400">·</span>
    <time class="font-mono" datetime="2026-11-21T10:04:00Z">2026-11-21</time>
    <span aria-hidden="true" class="text-stone-400">·</span>
    <span class="font-mono"><span class="sr-only">size change </span>+40 kB</span>
    <span aria-hidden="true" class="text-stone-400">·</span>
    <span class="text-stone-600">colour pass</span>
  </li>
  ...
</ol>
```

- **List, not table.** An ordered list (newest first) needs no column layout, so it wraps naturally in 320 px cards and narrow sidebars. There is no scroll region and no `tabindex`, so axe `scrollable-region-focusable` cannot fire and no tab stop is added.
- **Accessible name.** `aria-label` is `Version history` when every version is shown, and `Version history, latest N` when more versions exist, which is known from the extra row. Authors keep control of visible headings.
- **Current.** The visible text badge "current" carries the state in text, not colour.
  - Contrast: amber-800 (#92400e) on amber-100 (#fef3c7) is about 6.4:1. stone-600 on white is about 7.6:1.
  - The `·` separators are stone-400 and `aria-hidden`. Their contrast is exempt because they are decorative.
- **Date.** `<time datetime=RFC3339 UTC>` containing `2006-01-02` text.
- **Change.** The humanizer from the `formatPropertyValue` filesize branch.
  - Positive delta: `+` followed by the size.
  - Negative delta: `-` followed by the size.
  - Zero: `0 B`.
  - Oldest version: `first version` as visible text, with no sr-only prefix.
- **Comment.** Escaped. When the comment is empty, the comment span and its preceding separator are omitted.
- **Escaping.** Every interpolated value goes through `html.EscapeString`. There is no raw mode.
- **Meter.** Items are written through the `charged{m: m}` pattern from `mrql_renderer.go`, with `m.tick()` called once per item.

### Block form

The processed body for each version, concatenated, with no wrapper, the same as `[each]`. Under `UntrustedSource`, `[item raw="true"]` keeps its contract and is ignored.

### Unavailable state

```html
<span class="versions-shortcode-unavailable text-xs text-stone-600">Version history unavailable</span>
```

It is the same in both forms, and `[else]` is never rendered for it.

## 5. Empty, missing and invalid inputs (inline and block behave the same unless stated)

| Situation | Output |
|---|---|
| Context entity is not a resource | `shortcodeErrorMarker("versions", "[versions] only works on a resource: use it in a Resource Category template, inside [using type=\"resource\"], or in an [mrql] block over resources")` |
| No lister on the request context (share server, model preview) | `shortcodeComment("versions unavailable in this context")`. These surfaces have no lister by design, and `[else]` is not rendered. |
| Lister returns `ErrVersionsUnavailable` (the scoped handle could not be derived, or a scoped principal has no filter; fail closed) | The visible unavailable span (section 4) |
| `[versions]` sub-budget spent | `shortcodeErrorMarker("versions", "[versions] budget exceeded (N per page); refine templates or raise -mrql-page-query-budget")` |
| Lister returns any other error | `shortcodeErrorMarker("versions", err.Error())` |
| History is nil (resource not visible, or deleted) | inline: `""`. block: the `[else]` branch, or `""` if there is none. |
| Resource has no `resource_versions` rows | One synthesized v1 built from the resource row: id 0, number 1, the resource's `created_at`, `file_size`, `content_type` and `hash`, comment `""`, current = true |
| Rows exist and `CurrentVersionID` is nil | No row is current, and the handler does not guess |
| `CurrentVersionID` points at an ID not among the fetched rows (outside the window, or dangling) | No fetched row is current |
| `limit` invalid | 5 |
| `limit` > 50 | 50 |
| Meter stopped mid-render | `""` (the standard unwind) |

A visible resource always yields at least one row, real or synthesized. So `""` or `[else]` happens only for a resource that is not visible, and that renders exactly like a deleted one. The shortcode therefore cannot be used to probe whether a resource exists.

## 6. Data path

### `shortcodes/versions_handler.go` (new)

```go
const maxVersionsLimit = 50

type VersionRow struct {
    ID          uint // 0 = synthesized v1
    Number      int
    CreatedAt   time.Time
    FileSize    int64
    ContentType string
    Comment     string
    Hash        string // only for the synthesized v1 downloadUrl; not an [item] field
    Current     bool
}

// VersionList holds up to limit+1 rows, newest first. Row limit+1 is never
// rendered; it gives the oldest rendered row its predecessor and tells the
// accessible name that more versions exist.
type VersionList struct {
    ResourceID uint
    Rows       []VersionRow
}

type VersionLister func(reqCtx context.Context, resourceID uint, limit int) (*VersionList, error)

var ErrVersionsUnavailable = errors.New("resource versions are unavailable in this render")
var errVersionsBudgetExceeded = errors.New("[versions] budget exceeded")

func WithVersionLister(ctx context.Context, l VersionLister) context.Context
func versionListerFrom(ctx context.Context) VersionLister

// BudgetedVersionLister: with a QueryBudget on reqCtx, serves a repeated
// (resourceID, limit) from the budget's value cache (free), charges each miss
// against the [versions] sub-budget (AllowVersions), and on exhaustion returns
// fmt.Errorf("%w (%d per page); refine templates or raise -mrql-page-query-budget",
// errVersionsBudgetExceeded, limit) without calling base; onExceeded (may be nil)
// fires once per page via MarkVersionsExceeded. With no budget it calls base directly.
func BudgetedVersionLister(base VersionLister, onExceeded func(limit int)) VersionLister

func RenderVersionsShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string
```

Handler order:
1. Check the entity type.
2. Resolve the lister. If there is none, render the comment.
3. Clamp `limit`.
4. Call the lister.
5. Map the error: `ErrVersionsUnavailable` gives the visible span; `errVersionsBudgetExceeded` (`errors.Is`) or any other error gives the marker.
6. Check `meterFrom(reqCtx).alive()`.
7. Render the inline list or the block body.

The block form follows `RenderEachShortcode` exactly: `splitElseM`, `markItems`, `spliceLazy`, `renderItemValueObserved`.

### `shortcodes/query_budget.go`

Add to `QueryBudget`:
- `versionCount int`
- `versionsExceeded bool`
- `values map[string]any`, initialised in `WithQueryBudget`

Add methods:
- `AllowVersions() bool`: the same rule as `Allow`, but against `versionCount`. A limit of `0` always allows.
- `MarkVersionsExceeded() bool`: true exactly once.
- `lookupValue(key string) (any, bool)` and `storeValue(key string, v any)`.

What stays the same:
- `Allow`, `MarkExceeded`, `Stats`, and the MRQL cache and counters are unchanged, so `[mrql]` behaviour and benchmarks stay byte-identical.
- The value cache does not touch MRQL hit and miss stats.

The cache key is `fmt.Sprintf("versions|%d|%d", resourceID, limit)`. It is numeric only. No clone is needed: the handler never mutates a `*VersionList`, and the block form builds fresh item maps for each render.

Because `WithQueryBudget` is installed on every page and API render whatever the limit, a limit of `0` keeps dedupe and removes only the cap.

### `shortcodes/processor.go`

Add `case sc.Name == "versions": replacement = RenderVersionsShortcode(reqCtx, sc, ctx, renderer, executor, depth)` next to `each`.

### `application_context/resource_version_context.go`

```go
const maxVersionHistoryLimit = 50

var ErrVersionHistoryUnscoped = errors.New("version history needs a subtree-scoped handle")

type ResourceVersionHistory struct {
    ResourceID          uint
    CurrentVersionID    *uint                    // as stored; may be nil or dangling
    Versions            []models.ResourceVersion // version_number DESC, at most limit+1; only ID,
                                                 // VersionNumber, CreatedAt, FileSize, ContentType, Comment, Hash loaded
    ResourceCreatedAt   time.Time                // fallback fields for the synthesized v1
    ResourceFileSize    int64
    ResourceContentType string
    ResourceHash        string
}

// ResourceVersionHistory returns nil, nil when the resource is not visible.
func (ctx *MahresourcesContext) ResourceVersionHistory(resourceID uint, limit int) (*ResourceVersionHistory, error)

// VersionHistoryReader returns a context that may call ResourceVersionHistory for
// this principal: ctx itself when it already carries a scope filter or the
// principal is unscoped; otherwise (a scoped principal on a WithMRQLPrincipal
// handle) ctx.WithPrincipalContext(ctx.db.Statement.Context, ctx.principal),
// which materializes the subtree once. A subtree that could not be read
// returns the error (the copy denies everything anyway).
func (ctx *MahresourcesContext) VersionHistoryReader() (*MahresourcesContext, error)
```

`ResourceVersionHistory` body, in order:
1. **Clamp** `limit` to `[1, 50]`.
2. **Fail closed.** If `ctx.isScopedPrincipal()` and `scopeFromContext(ctx.db.Statement.Context) == nil`, return `nil, ErrVersionHistoryUnscoped`. This is defence in depth, because the adapter always goes through `VersionHistoryReader`.
3. **Authorizing read.** `ctx.db.Model(&models.Resource{}).Select("id, current_version_id, hash, file_size, content_type, created_at").Where("id = ?", resourceID).Limit(1).Find(&rows)`.
   - `Find` runs the scope callback, which adds the `owner_id` filter. Do not use `Scan`, which skips the callback.
   - No row returns `nil, nil`.
4. **Versions.** `ctx.db.Select("id, version_number, created_at, file_size, content_type, comment, hash").Where("resource_id = ?", resourceID).Order("version_number DESC").Limit(limit+1).Find(&versions)`.
   - This runs only after step 3 succeeded. `resource_versions` is unscoped, so step 3 is its authorization.
5. **Return** the struct.

It does not reuse `GetVersions`, which is unbounded and preloads through `GetResource`.

### `server/template_handlers/template_filters/shortcode_version_lister.go` (new)

```go
type VersionHistoryContext interface {
    VersionHistoryReader() (*application_context.MahresourcesContext, error)
}
func BuildVersionLister(appCtx VersionHistoryContext, onExceeded func(limit int)) shortcodes.VersionLister
func logVersionsBudgetExceeded(appCtx budgetLogger, limit int)
```

`BuildVersionLister`:
- Returns nil for a nil interface. Use the same nil-interface care as `partials` in `buildPageRenderContext`.
- The base closure holds a `sync.Once` that calls `appCtx.VersionHistoryReader()` on the first `[versions]` call.
  - The closure is built once per page render: `buildPageRenderContext` stashes it in `_reqCtxWithCache`, and the API surfaces build one per request.
  - So the subtree is derived at most once per page, and only on pages that use `[versions]`.
  - A reader error makes every call on that page return `ErrVersionsUnavailable`.
- It wraps the base in `shortcodes.BudgetedVersionLister(base, onExceeded)`.
- Mapping:
  - A nil history gives `nil, nil`.
  - `ErrVersionHistoryUnscoped` gives `ErrVersionsUnavailable`.
  - Each row gets `Current = CurrentVersionID != nil && *CurrentVersionID == v.ID`.
  - Empty `Versions` gives the single synthesized v1 with `Current = true`.

`logVersionsBudgetExceeded`:
- Writes one `/logs` warning, entity type `mrql`, with the message "[versions] budget exceeded (N per page); some [versions] shortcodes were not rendered. Refine templates or raise -mrql-page-query-budget."
- It sits next to `logPageQueryBudgetExceeded`, which is unchanged.

Also add `VersionHistoryContext` to `PageRenderContext` in `shortcode_interfaces.go`, with a compile-time assertion for `*application_context.MahresourcesContext`.

### Wiring by render surface

| Surface | Change |
|---|---|
| Display pages through `buildPageRenderContext` (the `process_shortcodes` and `custom_css` tags) | After `WithQueryBudget`: `reqCtx = shortcodes.WithVersionLister(reqCtx, BuildVersionLister(versionCtx, func(l int){ logVersionsBudgetExceeded(appCtx, l) }))`, nil-guarded like `partials` |
| API renders through `buildMRQLAPIRenderContext` (deferred, template and description preview, entity picker, `/v1/mrql` render, MRQL cards) | Same line. `mrqlRenderContext` (`handler_interfaces.go`) embeds `template_filters.VersionHistoryContext`. |
| Lightbox JSON (`processShortcodesForJSON`, `server/routes.go`) | Same line |
| Share server | Not wired, so it renders the comment. Public pages must not link to primary-server version routes. |
| Model preview (`template_run_tools.go`) | `renderCtx = shortcodes.WithVersionLister(renderCtx, nil)`, which renders the comment. No opt-in covers version comments, and a harness tool never writes a log row. |
| Test fake (`meta_scope_memo_test.go`) | Add the method |

## 7. Scoping and authorization (fail closed)

- **`WithPrincipal` handles** (display pages, the lightbox, `scopedAPI` routes including deferred and previews):
  - `VersionHistoryReader` returns ctx itself, because the handle already carries the subtree filter.
  - Step 3 returns nothing outside the subtree, and the versions query never runs.
- **`WithMRQLPrincipal` handles** (the `/v1/mrql` family):
  - For a confined principal, `VersionHistoryReader` derives a `WithPrincipalContext` copy once per page.
  - Its handle carries the filter, so step 3 is enforced as above.
  - A failed subtree read leaves the copy denying everything and also returns an error. The adapter treats either as unavailable.
  - Admins and unscoped principals use ctx directly.
- **Fail-closed backstop.** Step 2 refuses any scoped principal whose handle lacks a filter, so an unknown future caller cannot read unscoped data.
- **Targets reached through other shortcodes.** `[using type="resource" id=…]` and `[mrql]` reach their targets through the principal-enforced executor, and step 3 checks again regardless.
- **Cache safety.** The memo and the derived handle live on a per-page `QueryBudget` and in a per-page closure. Neither outlives a request or a principal.

## 8. Performance (millions of resources)

- **Per cache miss: 2 indexed queries.**
  - A primary-key lookup on `resources`.
  - `resource_versions WHERE resource_id = ? ORDER BY version_number DESC LIMIT ≤51`, on the existing `resource_id` index.
  - Neither cost grows with the table size.
  - There is no COUNT; the `limit+1` row answers "more exist".
- **Per page.**
  - Cache misses are capped by the `[versions]` sub-budget, which equals `-mrql-page-query-budget` (default 200). Hits are free.
  - Example: a 50-card list with `[versions limit="1"]` in `CustomSummary` costs 50 sub-budget units and 100 small queries. It consumes none of the `[mrql]` budget.
  - Past the cap: the marker renders, and one warning is logged per page.
  - With a value of `0`: uncapped, but still deduped.
- **Confined users on `/mrql`.** Add one subtree resolution per page that uses `[versions]`. This is the same work every scoped display page already does.
- **Output.** At most 50 rows per call. Output and parse size stay bounded by the existing render meter.

## 9. Lint (`shortcodes/lint.go`, `shortcodes/mrql_slots.go`)

- The `BuiltinDocs()` entry registers the name, `limit` and block capability. It feeds `KnownFromBuiltins`, autocomplete, hover docs and, through `plugin_system/shortcode_docs.go`, the docs endpoint.
- Add `"versions"` to `builtinBaseNames`, so `[version]` gets a "did you mean [versions]?" hint.
- Add `"versions": {"else"}` to `slotTags`.
- **`[item]` placement.**
  - The eachSpans set becomes `namedBlockSpans(tokens, "each")` plus `namedBlockSpans(tokens, "versions")`.
  - The innermost-`[using]` check treats `[versions]` like `[each]`.
  - The warning text becomes "[item] is only meaningful inside an [each] or [versions] block".
- **New `case "versions"` warning:** a `limit` that is not a positive integer, or that is above 50 ("clamped to 50").

## 10. Files to change

- **New:**
  - `shortcodes/versions_handler.go` and `shortcodes/versions_handler_test.go`
  - `server/template_handlers/template_filters/shortcode_version_lister.go` and its `_test.go`
  - `server/api_tests/versions_shortcode_scope_test.go`
  - `e2e/tests/108-versions-shortcode.spec.ts`
  - `e2e/tests/accessibility/versions-shortcode-a11y.spec.ts`
- **Edit:**
  - `shortcodes/processor.go`
  - `shortcodes/query_budget.go` and `query_budget_test.go`
  - `shortcodes/builtin_docs.go`: the entry after `each`/`item`, plus the `BuiltinDoc` comment list
  - `shortcodes/lint.go`, `shortcodes/mrql_slots.go`
  - `application_context/resource_version_context.go` and its test
  - `server/template_handlers/template_filters/shortcode_tag.go`, `shortcode_interfaces.go`, `shortcode_query_executor.go` (the log function sits beside the existing one), and the `meta_scope_memo_test.go` fake
  - `server/api_handlers/mrql_api_handlers.go`, `handler_interfaces.go`, `template_run_tools.go`
  - `server/routes.go`
- **Docs:**
  - `docs-site/docs/features/shortcodes.md`:
    - add `[versions]` to the "Built-in inline or block" syntax line;
    - add the table row `| \`[versions]\` | Inline or block | List a resource's version history |`;
    - add a section `## \`[versions]\` -- Resource Version History`. It opens with the card status-line example, then covers the inline trail example, the attribute and item-field tables, the note that the resource-page and lightbox panels remain the full tool, the "unavailable" states, and the budget note: own sub-budget equal to `-mrql-page-query-budget`, cache hits free, `0` = uncapped and still deduped.
  - `docs-site/docs/features/custom-templates.md`: a Resource Category `CustomSummary` status-line example.
  - `docs/architecture/configuration.md`: the `-mrql-page-query-budget` row gets "`[versions]` has a separate counter with the same cap, so it never consumes the `[mrql]` budget; its exhaustion logs its own warning".
  - `templates/createResourceCategory.tpl`, `createCategory.tpl`, `createNoteType.tpl`: a reference-panel entry. The latter two say "resource templates only". `TestReferencePanelsCoverAllBuiltins` requires these.
  - `.claude/skills/mahresources-category-designer/references/shortcodes.md`: one entry.

## 11. Tests

### Unit: `shortcodes/versions_handler_test.go` (fake lister)

- **Inline:**
  - an `<ol>` with one `<li>` per rendered row;
  - `aria-label` with and without "latest N";
  - the badge only on the `Current` row;
  - delta `+`, `-`, `0 B`, and "first version" only on the oldest row;
  - the `limit+1` row is never rendered, but it does supply the last rendered row's delta;
  - an empty comment omits the span and its separator;
  - no `tabindex`, `role="region"` or `<table>` in the output.
- **Escaping:** a `<script>` comment is escaped inline and in the block form under `UntrustedSource`.
- **Limit:** `""`, `"0"`, `"-3"` and `"abc"` reach the lister as 5; `"999"` reaches it as 50.
- **Block:**
  - every `[item]` path;
  - `compareUrl` across a gap (v7 then v5 gives `v1=5&v2=7`);
  - `compareUrl`, `delta` and `typeChanged` are empty, nil and false only on the true oldest row;
  - the synthesized v1 `downloadUrl` is `/v1/resource/view?id=R&v=<hash>`;
  - `hash` as an item path renders nothing;
  - `format="date"` and `format="filesize"`, `index="true"`, `default=` on a nil delta;
  - `raw="true"` is ignored under `UntrustedSource`;
  - `[else]` renders on a nil history;
  - `[meta]` inside the body reads the resource.
- **Inert cases:**
  - a non-resource entity gives the marker in both forms, and `[else]` is not rendered;
  - no lister gives the comment;
  - `ErrVersionsUnavailable` gives the visible span, not `[else]`;
  - a generic error gives the marker.
- **Budget:**
  - `WithQueryBudget(ctx, 1)` plus `BudgetedVersionLister`: two identical calls call base once, and both render;
  - a call with a different limit renders the `[versions]` budget marker;
  - `onExceeded` fires once across two over-budget calls;
  - with the `[versions]` sub-budget spent, `budget.Allow()` still returns true, so `[mrql]` is unaffected;
  - `WithQueryBudget(ctx, 0)`: 300 distinct calls all reach base, an identical repeat is served from cache, and the marker never renders;
  - with no budget on the context, base is called every time.
- **Meter:** a tiny `WithRenderBudget` output budget stops a 50-row list cleanly.

### `shortcodes/query_budget_test.go`

- `AllowVersions` and `Allow` count independently.
- `MarkVersionsExceeded` returns true once.
- The value cache round-trips.
- `Stats()` is unchanged by `[versions]` activity.

### Lint: `shortcodes/lint_test.go`

- `[item]` inside `[versions]` gives no warning.
- `[item]` inside `[using]` inside `[versions]` warns.
- `limit="80"` and `limit="x"` warn.
- `[version]` gets the near-miss hint.
- `[versions]…[else]…[/versions]` is accepted.
- `[versions show="x"]` gets the unknown-attribute warning.
- `builtin_docs_consistency_test.go` covers the docs table, the reference panels and the example parse.

### `application_context/resource_version_context_test.go`

- Rows come back DESC, at most limit+1, and the limit is clamped to [1, 50].
- No rows gives an empty slice plus the fallback fields.
- `CurrentVersionID` is returned as stored.
- `WithPrincipal`, confined guest, out-of-subtree resource: `nil, nil`, and a GORM query-callback counter shows the versions query never ran.
- An in-subtree resource returns its rows.
- On a `WithMRQLPrincipal` confined guest, direct `ResourceVersionHistory` returns `ErrVersionHistoryUnscoped`.
- On the same handle, `VersionHistoryReader()` returns a handle that sees the in-subtree resource and gets `nil, nil` for the out-of-subtree one.
- On a `WithMRQLPrincipal` admin, `VersionHistoryReader()` returns ctx itself, and the rows are visible.
- A confined guest whose scope group is missing: `VersionHistoryReader()` returns an error.

### Adapter: `shortcode_version_lister_test.go`

- `Current` is mapped by exact ID.
- A nil or dangling `CurrentVersionID` gives no current row.
- No rows gives a synthesized v1 with current set.
- `ErrVersionHistoryUnscoped` maps to `ErrVersionsUnavailable`.
- A reader error makes every call `ErrVersionsUnavailable`.
- `VersionHistoryReader` is called once across 10 lister calls.
- A nil appCtx gives a nil lister.

### API scoping: `server/api_tests/versions_shortcode_scope_test.go`

These follow the `using_shortcode_scope_test.go` pattern (`setupAuthEnv`, `buildScopingFixture`, `previewDescription`). Version rows with comments `sf-vIn` and `sf-vOut` are seeded on `rInID` and `rOutID`.

- **Confined guest:**
  - a preview of `rInID` with `[versions]` contains `sf-vIn`;
  - `[using type="resource" id=rOut][versions][else]hidden[/versions][/using]` contains no `sf-vOut`.
- **`/v1/mrql` with render, over a resource category whose `CustomMRQLResult` is `[versions]`:**
  - the confined guest's query matching both resources returns `sf-vIn` and never `sf-vOut`;
  - an admin gets both.
- **Deferred:** `[lazy][versions][/lazy]` on `rInID` renders `sf-vIn` for the guest.
- **Model preview:** renders the comment.
- The Postgres `api_tests` run (`--tags 'json1 fts5 postgres'`) picks this file up.

### E2E: `e2e/tests/108-versions-shortcode.spec.ts`

1. Create a resource category with:
   - `CustomSummary` set to `<p class="vs-line">[versions limit="1"]v[item path="number"] · [item path="comment"][/versions]</p>`;
   - `CustomSidebar` set to `[versions limit="2"]`.
2. Create a resource and upload two versions with comments via `POST /v1/resource/versions?resourceId=`, as `version-compare.spec.ts` does.
3. On the resources list, the card's `.vs-line` reads `v3 · <comment>`.
4. On the detail page:
   - the sidebar `ol.versions-shortcode` has aria-label "Version history, latest 2";
   - it has 2 items;
   - the first item contains "v3" and "current".
5. A second resource with no uploads shows one item containing "v1", "current" and "first version".

### E2E a11y: `e2e/tests/accessibility/versions-shortcode-a11y.spec.ts`

- Same fixture, with `CustomSummary` also holding `[versions limit="3"]`.
- Run axe on the list page and on the detail page.
- At a 320 px viewport, assert `scrollWidth <= clientWidth` for:
  - each `article.resource-card` holding an `ol.versions-shortcode`;
  - the detail page sidebar element that wraps the `CustomSidebar` output.
- Axe must pass at 320 px.

### Suites

- Before and after the change: `go test --tags 'json1 fts5' ./...`, `cd e2e && npm run test:with-server:all`, and `npm run test:with-server:a11y`.
- The Postgres `api_tests`.
- `./scripts/css-scan-test.sh`, because new utility classes come from Go source.

## 12. Success criterion

- A Resource Category whose `CustomSummary` holds `[versions limit="1"]…[/versions]` shows the correct latest version number, date and comment on every list card.
  - This covers the resources list, group panels, `[mrql]` blocks and `/mrql` cards, including for group-confined users.
  - `[versions]` in a sidebar or card renders an accessible stacked list that does not overflow at 320 px.
- Each occurrence costs at most 2 indexed queries and 1 `[versions]` sub-budget unit on a miss, and 0 on a hit.
  - It never consumes the `[mrql]` budget.
  - `-mrql-page-query-budget=0` keeps dedupe.
  - Exhaustion logs once per page.
- A confined guest never sees version data of a resource outside their subtree on any surface: `[using]`, `[mrql]`, deferred renders, previews, `/v1/mrql` (enforced through the derived scoped handle), the share server and the model preview (comment). There is a test for each.
- The docs-site, reference panels, lint, autocomplete and skill reference all list `[versions]`.
- The Go, E2E, a11y and Postgres suites pass.

---

## Last evaluator feedback

1. The wiring misses the /mrql list-card surface, and section 12 claims that surface works. `renderMRQLListCards` (server/api_handlers/mrql_list_render.go:98) calls `template_handlers.NewListCardRenderer`, and list_card_renderer.go:37 sets `c["_reqCtxWithCache"] = shortcodes.WithCSSSeen(request.Context(), seen)` ahead of time. So the `process_shortcodes` tag never calls `buildPageRenderContext` on those cards: no lister, no QueryBudget. The headline use, a `CustomSummary` status line on `/mrql` resource cards, would render the "versions unavailable in this context" comment. The /v1/mrql scope test only covers `CustomMRQLResult`, which goes through `buildMRQLAPIRenderContext`, so it would not catch this. Fix: add list_card_renderer.go to the wiring table. Install a lister there, plus a `WithQueryBudget` if the cap is meant to hold, on the handle `renderMRQLListCards` gets through `withRequestContext` (check whether that is a `WithMRQLPrincipal` handle). Then add an E2E or API test that renders `/mrql` list cards with `[versions]` for both an admin and a confined guest.
2. Point 3 ("`WithQueryBudget` is installed on every surface whatever the limit") is wrong for the lightbox. `processShortcodesForJSON` (server/routes.go:431) installs `WithMRQLCache` and `WithPartialResolver` but no `WithQueryBudget`. There, `BudgetedVersionLister` takes the "no budget, call base directly" branch, so the lightbox gets neither dedupe nor a cap. The existing `[mrql]` executor has the same gap. Either add `WithQueryBudget(reqCtx, appCtx.MRQLPageQueryBudget())` in that function (a behavior change for `[mrql]` too, which should be called out) or document the lightbox as unbudgeted. Either way, fix the claim and the "Lightbox JSON: same line" row.
3. Some of the plan's statements about existing code are wrong:
   - versionPanel.tpl is not collapsed by default. It carries `open` when `versions|length > 1`, which weakens the motivation.
   - "The same URL versionPanel.tpl builds" for the synthesized v1 `downloadUrl` is wrong. The panel's Download link is `/v1/resource/version/file?versionId={{version.ID}}`, which is 404 for the virtual v1, and its thumbnail is `/v1/resource/preview?id=&v=`, not `/v1/resource/view`. The chosen `/v1/resource/view?id=R&v=<hash>` route does exist (routes.go:765, `scopedAPI`), so keep it, but correct the justification.
   - `GetVersions` gives its virtual v1 the comment "Current version"; the plan uses `""`. State that this difference is intentional.
4. Over-engineering versus value: the plan adds a second budget counter, a `map[string]any` value cache on `QueryBudget`, a lazily derived `WithPrincipalContext` handle behind a `sync.Once`, a new interface on `PageRenderContext` and `mrqlRenderContext`, and three "unavailable" output variants (comment, visible span, marker). That is a lot of surface for one read-only list. Consider charging `[versions]` misses to the existing budget through a small, generic keyed memo, and folding `ErrVersionsUnavailable` and the no-lister case into one visible state, except for share and model preview.
5. Edge cases to pin down:
   - `[item path="delta" format="filesize"]` with a negative int64: check that `formatPropertyValue`'s filesize branch handles negatives, or specify the output and test it.
   - `[versions]` inside `CustomCSS`, whose `custom_css` tag also calls `buildPageRenderContext`: it should be inert or linted.
   - Hovercards (routes.go:614) render `CustomSummary` too. List them in the wiring and test coverage, since the status line would appear there.
6. Usefulness: the card status line ("v7 · date · comment") is real and not covered by existing shortcodes, since `[property]` cannot reach unpreloaded `Versions`. But it only matters for deployments that actively version files, a minority workflow. The inline `<ol>` trail adds little over the detail-page panel. The value is narrow and not something most template authors will use often.
