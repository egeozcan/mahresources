# Plan: `[inspect]`, a context explorer that only template authors see

Slug: `inspect`. Saved as `docs/plans/2026-11-24-shortcode-inspect.md` (the index script files it under "Category templates & shortcodes").

## 1. Motivation and user value

Authors write `path=` blind. The live preview shows what a template *renders*, and lint flags *unknown* Meta paths when a schema exists. Neither tells the author what *is there* at a given point in the template:
- which Meta keys this entity carries, with their JSON types;
- which entity fields `[property]` can read;
- inside `[each]`, what the current element looks like and so which `[item path=…]` works;
- inside an `[mrql]` block or a `[using]` block, which entity the context has switched to.

Today an author finds out by guessing a path, reading the preview, and guessing again. `[inspect]` prints that inventory directly into the preview. Each row includes a ready-to-paste shortcode, so the guessing loop goes away.

Not built here: the pitch mentions `[let]` variables, but no `[let]` shortcode exists (`processor.go` dispatch has none), so there is nothing to list. If `[let]` ships later, it adds its own section. Nothing from `docs/ideas/category-templates-and-shortcodes.md` is re-proposed. Phase 1 there covers preview, lint and autocomplete. This plan builds on those features and replaces none of them.

## 2. Syntax and attributes

```
[inspect]
[inspect path="project"]
```

- Inline only (`IsBlock: BlockNo`).
  - The existing generic lint rule already reports a closer: "[inspect] is an inline shortcode and cannot have a closing tag".
  - If a block form slips through anyway, it renders the panel and drops the inner content.

| Attr | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `path` | string | no | `""` (all of Meta) | A Meta dot-path, written exactly as for `[meta path=]`, `[each path=]` and `[conditional path=]`. It limits the **Meta** section to the subtree at that path. The other sections are unaffected. |

The pitch wrote `path="meta.project"`. This plan uses the bare Meta path (`path="project"`) so the attribute means the same thing it means on every other built-in. No other attributes.

## 3. Where it lives: built-in Go handler

Built in, not a plugin, for three reasons:
1. It needs data the plugin API does not expose: the `[each]` element (only `each_handler.go`'s splice pass has it), `ctx.outer` (the `[using]` marker), and the render meter.
2. Fail-closed gating must be a request-context flag that only one Go handler sets. A plugin would have to trust a value the host hands it.
3. Lint, docs and autocomplete are all driven by `BuiltinDocs()`.

New file: `shortcodes/inspect_handler.go`.

## 4. Authorization: fail closed by construction

**Gate.** A new request-context flag, following the pattern of `WithCredentialFieldsVisible` / `WithDeferredSigner`:

```go
// inspect_handler.go
type inspectKey struct{}
type inspectState struct{ panels int }
func WithInspect(ctx context.Context) context.Context { return context.WithValue(ctx, inspectKey{}, &inspectState{}) }
func inspectStateFrom(ctx context.Context) *inspectState // nil when absent or ctx nil
```

- Exactly one production caller sets it: `GetPreviewTemplateHandler` in `server/api_handlers/template_preview_api_handlers.go`. It does so right after `buildMRQLAPIRenderContext`: `reqCtx = shortcodes.WithInspect(reqCtx)`.
- That route already carries the right authority:
  - `/v1/category/previewTemplate` and `/v1/resourceCategory/previewTemplate` require `capTaxonomy` (`isTaxonomyPath`).
  - `/v1/noteType/previewTemplate` requires `capEditor` (`isEditorPath`).
  - So "can preview" equals "can save this template".
  - With `-auth` off there is no role model and every user is an author, which matches today's preview.
- **Every other render surface renders `[inspect]` as the empty string**, because none of them carries the flag:
  - display pages (`shortcode_tag.go`)
  - the deferred endpoint (`[lazy]` / `[details]` / `[reload]` / `[autorefresh]` bodies)
  - the share server
  - MRQL API renders
  - hover cards and the lightbox
  - and especially the AI template run tools (`template_run_tools.go`). If that path set the flag, the panel would send raw Meta values to a model past the privacy mode's masking. It must not set it, and a test pins that.
- The MCP preview tools (`previewCategoryTemplate`, `previewNoteTypeTemplate`, `previewResourceCategoryTemplate`) dispatch to this same route as the same caller, so they render the panel too. That is intended: the caller already holds the save capability and can read every value through `[meta]` in the same preview, and MCP preview applies no privacy masking either way. The AI template run tools are different: they render under privacy mode, which the panel would bypass, so they never set the flag.
- The empty string is deliberate: it is not a comment and not a marker, so a page reveals nothing about whether `[inspect]` exists.
- There is deliberately **no** "user can edit this category" check on live pages. A live-page path would have to be correct across sealed deferred bodies rendered by a later request, shared pages, and per-request render caches. One opt-in point is the fail-closed design. A template saved with `[inspect]` still in it is harmless.
- `ctx.UntrustedSource == true` also renders `""`, as defence in depth. It cannot occur in a slot preview today.

**Subtree scoping.** `[inspect]` issues **zero queries**. It reads only `ctx.Meta`, `ctx.Entity`, `ctx.EntityType`/`EntityID`, `ctx.outer`, and the `[each]` element. The preview handler already loaded all of these through `scopedAPI` (`loadPreviewEntity` returns 404 outside the subtree). An `[mrql]` or `[using]` context is built from the scoped executor's results.

It also reveals strictly less than `[property]`, because it **never reads through an association**:
- Fields whose kind is struct, pointer-to-struct, slice, array or map are listed by name and kind only (`relation` / `list`), with no sample value, no count and no nested fields.
- This matters because the root group of a confined user's subtree preloads its `Owner`, and that owner lies outside the subtree. The panel must not print its name.
- Credential fields (`credentialFields`: `ShareToken`, `TokenHash`, `CsrfToken`, `PasswordHash`) are **always omitted**, even when `ctx.credentialsVisible` is true. An inspector is the wrong place for a bearer token, and authors take screenshots of it.

## 5. Rendering

### 5.1 Dispatch

- **`processor.go` switch:** `case sc.Name == "inspect": replacement = RenderInspectShortcode(reqCtx, sc, ctx)`.
- **`RenderInspectShortcode`, in order:**
  1. Flag absent, or `ctx.UntrustedSource` → `""`. This comes first, so non-authors never see even an error marker.
  2. `withinReloadFace(reqCtx)` → `shortcodeErrorMarker("inspect", "[inspect] cannot be used inside a [reload] button face")`, the same as `[details]`.
  3. Panel cap: `state.panels++`. If the count is 11, emit `<p class="inspect-shortcode__note">Further [inspect] panels omitted (limit 10 per preview).</p>`. Above 11, emit `""`. Constant: `maxInspectPanels = 10`. This bounds an `[inspect]` nested inside `[each]`/`[mrql]` bodies that run per element or per item. The preview already caps `[mrql]` at 5 rows.
  4. Otherwise `renderInspectPanel(ctx, sc, nil, false)`.
- **`[each]` element.** In `each_handler.go`, `markItems` also turns top-level `inspect` tokens into sentinels; the token slice keeps their order. The splice callback in `RenderEachShortcode` dispatches on `items[idx].Name`:
  - `item` → `renderItemValueObserved` (unchanged);
  - `inspect` → for the first element (`i == 0`) only, the gated `renderInspectPanel(ctx, sc, elem, true)`, through the same gate and cap as above; otherwise `""`.
  - This uses the existing rule that "top-level of the [each] body belongs to this loop", so the element shown is exactly the one `[item]` would bind to there.
  - An `[inspect]` nested deeper (for example inside a `[conditional]` in the loop) shows the entity context only, as `[item]` would render empty there. The docs say so.
- **Inside an `[mrql]` block body, a `CustomMRQLResult`, `[using]`, `[partial]`, `[lazy]` or `[details]`** in preview: no special code. `ctx` is already the item or target entity. No deferred signer is wired in preview, so `[lazy]`/`[details]` bodies render inline.

### 5.2 Markup (accessible, no JavaScript)

The panel is a native disclosure, so it opens with Enter and Space and screen readers announce its state. It works inside the preview's `sandbox="allow-scripts"` iframe, where the Clipboard API is unavailable (opaque origin). That is why each snippet is a **read-only text input**: it is reachable with Tab, its text is selectable with Ctrl/Cmd+A, and it copies with Ctrl/Cmd+C.

```html
<details class="inspect-shortcode">
  <summary>Inspect: group #42</summary>            <!-- "· Meta at “project”" appended when path= is set; "· inside [using]" when ctx.outer != nil -->
  <div class="inspect-shortcode__body">
    <p class="inspect-shortcode__note">Author aid: shown only in the template preview; renders nothing on pages.</p>
    <div class="inspect-shortcode__scroll" role="region" aria-label="Meta paths" tabindex="0">
      <table>
        <caption>Meta</caption>
        <thead><tr><th scope="col">Path</th><th scope="col">Type</th><th scope="col">Sample</th><th scope="col">Shortcode</th></tr></thead>
        <tbody>
          <tr><th scope="row"><code>project.name</code></th><td>string</td><td>"Apollo"</td>
              <td><input type="text" readonly class="inspect-shortcode__snippet" value="&#91;meta path=&quot;project.name&quot;]" aria-label="Shortcode for project.name"></td></tr>
        </tbody>
      </table>
    </div>
    <!-- same structure: caption "Properties" (region "Property fields"),
         caption "Current [each] element" (region "Each element paths", only when hasElem) -->
  </div>
</details>
```

- No headings, so the panel cannot cause a heading-level skip in whatever slot hosts it. Tables use `<caption>`, `th scope`, and a focusable scroll region (the axe `scrollable-region-focusable` rule).
- **CSS** goes in `public/index.css`, next to `details.details-shortcode`:
  - `summary` uses the same cursor, weight and `:focus-visible` outline (`#b45309`);
  - stone-700-on-white text (contrast ≥ 4.5:1);
  - monospace path and snippet;
  - `overflow-x:auto` on the region;
  - the snippet input at full cell width.
  - Plain class names, no Tailwind utilities emitted from Go, so the CSS source scan is unaffected.
- **Escaping.** Every emitted text and attribute value is `html.EscapeString`'d, and `[` is additionally written as `&#91;`. Two reasons:
  - Meta values may contain shortcode text (`"[mrql …]"`), and the snippets *are* shortcode text.
  - Neither must ever be parsed again if the output were re-processed. The browser decodes `&#91;` for display and copying.
  - The output also passes through no other path: it is a `replacement`, which `processWithDepth` never rescans.

### 5.3 Section contents

**Meta.** Source is `ctx.decodeMeta()`, narrowed by `navigateJSONValue(root, path)` when `path` is set.
- Walk objects depth-first, keys sorted (deterministic), down to 6 path segments.
- Arrays are leaves (Meta paths do not index arrays).
- One row per key:
  - Type: `string` | `number` | `boolean` | `null` | `object (n keys)` | `array (n items)`.
  - Sample: strings quoted and truncated to 80 runes with `…`; numbers and booleans via `formatJSONScalar(v, "", "")`; objects and arrays show no sample.
- Snippet by type:
  - scalar or null → `[meta path="p"]`;
  - object → `[meta path="p"]`;
  - array → `[each path="p"][item][/each]`.
- A key that cannot be written as a path (empty, or containing `.`, `"`, `'`, `[` or `]`) gets type plus `not addressable by path=` in the Shortcode cell and no input.
- Empty states (`<p>`): "This entity has no Meta." / `No Meta value at path "x".` A `path` resolving to a scalar shows one row for it.

**Properties.** Omitted when `ctx.Entity` is nil.
- Exported fields of the dereferenced struct, in declaration order, one level only, using the same `CanInterface` guard as `traversePropertyPath`.
- Skipped: `Meta` and `OwnMeta` (the Meta section covers them), and credential fields.
- Scalars, `time.Time` and pointers to them: Sample via `formatPropertyValue(f, "", "")`, truncated to 80 runes. `Description` goes through `stripTemplateComments`, as in `[property]`.
- Snippets:
  - `[property path="Name"]`;
  - for `time.Time`: `[property path="CreatedAt" format="date"]`;
  - for an integer field named `FileSize`: `format="filesize"`.
- `json.RawMessage` / `[]byte` / `map` fields: type `json`, no sample, no snippet.
- Structs and slices: type `relation` / `list`, no sample, no snippet, because guessing a nested field name is the speculation this plan avoids. Name and kind only.

**Current [each] element.** Shown only when `hasElem`.
- Same walk as Meta, rooted at `elem`.
- Snippets `[item path="k"]`, and `[item]` for a scalar element.
- First row: `[item index="true"]` (type `position`).

**Row budget.** `maxInspectRows = 200` per panel across all sections. When it is reached, one `<p>` note is added: "N more paths not shown. Narrow with path=." Output size is charged by the existing `processWithDepth` / `spliceLazy` meter calls. Each row is ~300 bytes, so a full panel is ~60 KB, well inside the render budget.

## 6. Performance (millions of resources)

- Zero DB queries; the query budget is untouched.
- Work is O(min(rows, 200) + struct fields) per panel, at most 10 panels per preview, and only in a preview request.
- On live pages the cost is one context lookup returning `""`.
- Meta decoding reuses the shared `decodedMeta` memo.

## 7. Lint

`shortcodes/lint.go`:
- Add `"inspect"` to `builtinBaseNames` (near-miss detection).
- Add `inspect` to the existing `case "lazy", "details":` branch, so `[inspect]` inside a `[reload]` face is an error ("[inspect] cannot be used inside a [reload] button face").
- Unknown-attribute and closing-tag checks come for free from `BuiltinDocs`.
- No new informational lint.

## 8. Files to change

- [x] 1. `shortcodes/inspect_handler.go` (new): `WithInspect`, `inspectStateFrom`, `RenderInspectShortcode`, `renderInspectPanel`, constants `maxInspectPanels=10`, `maxInspectRows=200`, `inspectSampleRunes=80`, `inspectMaxDepth=6`.
- [x] 2. `shortcodes/processor.go`: one `case` in the dispatch switch.
- [x] 3. `shortcodes/each_handler.go`: `markItems` also marks top-level `inspect`; the splice callback dispatches on name. Update the doc comment.
- [x] 4. `shortcodes/builtin_docs.go`: new `BuiltinDoc{Name:"inspect", …}` with the three examples; its description states the preview-only gate. Update the `BuiltinDoc` comment's list of names.
- [x] 5. `shortcodes/lint.go`: as in section 7.
- [x] 6. `server/api_handlers/template_preview_api_handlers.go`: `WithInspect`, with a comment saying this is the only setter and why.
- [x] 7. `public/index.css`: `.inspect-shortcode` rules.
- [x] 8. `templates/createCategory.tpl`, `createResourceCategory.tpl`, `createNoteType.tpl`: one reference-panel entry each (`TestReferencePanelsCoverAllBuiltins` requires it).
- [x] 9. Docs (section 10).

No route, OpenAPI or MCP safety changes: no new endpoint.

## 9. Tests

- [x] Unit, `shortcodes/inspect_handler_test.go`: gate off on every surface shape (inline, `[conditional]`, `[each]` top level, `[mrql]` block, sealed `[details]` body); `UntrustedSource`; Meta rows, types, samples, truncation, unaddressable keys, `path=` narrowing and empty states; escaping; properties (credential omitted even when visible, `Owner` relation without its name, `CreatedAt` date snippet, no `Meta` row); row and panel caps; `[reload]` face marker; accessibility structure.
- [x] Unit, `each_handler_test.go`-style: one panel per loop with the element table, titles still spliced; without the flag the output equals the loop without `[inspect]`.
- [x] Unit, lint: closer error, unknown attribute, near-miss, `[reload]` face error.
- [x] Consistency: existing `builtin_docs_consistency_test.go` tests pass.
- [x] API, `server/api_tests`: preview returns the panel; subtree-confined principal sees `Owner` as a relation without the owner's name; out-of-subtree preview 404; `user` role 403.
- [x] API, fail closed on other surfaces: group display page, deferred endpoint, and the AI template run tools render `[inspect]` as nothing.
- [x] E2E: `e2e/tests/template-preview.spec.ts` keyboard flow; a11y scan of an expanded panel.

## 10. Docs

- `docs-site/docs/features/shortcodes.md`: table row and a `## [inspect] -- Context Explorer (preview only)` section.
- `docs-site/docs/features/custom-templates.md`, "Live preview" subsection: a short paragraph and example.
- `.claude/skills/mahresources-category-designer/references/shortcodes.md`: an entry stating that `[inspect]` is for authoring only.
- The in-app reference panels (section 8, item 8).

## 11. Success criterion

- An author with preview rights adds `[inspect]` anywhere in a slot. The live preview shows a collapsed, keyboard-operable panel listing every reachable Meta path (up to 200) with type, sample and a copyable snippet; the entity's scalar fields; at the top level of an `[each]` body, the element's `[item]` paths; inside `[mrql]` or `[using]`, the switched entity.
- The same template on every non-preview surface renders byte-for-byte as if `[inspect]` were absent.
- A subtree-confined author's panel contains nothing an out-of-subtree `[mrql]` could not already show, and in particular no value read through an association.
- The panel issues zero queries.

## 12. Review (implementation record)

Built as planned, with these deviations, each deliberate:

- **Parser.** `inspect` had to be added to both name alternations in `shortcodes/parser.go`; the plan's file list missed it.
- **Stylesheets get no panel.** `GetPreviewTemplateHandler` sets `WithInspect` only on the markup render, and not when the slot is a `*CSS` slot. The css buffer is a stylesheet, where a `<details>` panel is garbage.
- **No subtree-confined author exists.** `Role.AllowsScopeGroup` admits only `guest` and `user`, and neither can reach any preview route. The API test therefore asserts the 403 for a plain and a confined `user` on all three routes, and an editor's 200 on the note-type route. "Never reads through an association" is pinned against a note, whose preview preloads `Owner` and `Owner.Owner`: neither group's name appears, and neither does the share token.
- **Struct kinds.** A struct field with an `ID` field is a `relation`. Any other struct (for example `types.URL`) is a `struct`. Both are name and kind only, as planned.
- **Near-miss test.** It uses `[inpect]`. The existing near-miss check counts single edits, not transpositions, so `[inspcet]` is reported as literal text. Widening that check is out of scope.
- **a11y scan.** axe scans the preview frame with `landmark-one-main`, `page-has-heading-one` and `region` disabled. Those rules judge the frame as a whole page, but it holds one slot fragment. Every rule about the panel itself runs.

Tests: `shortcodes/inspect_handler_test.go` and `shortcodes/inspect_lint_test.go` (gate off on every render shape, `UntrustedSource`, rows, types, samples, truncation, unaddressable keys, `path=`, escaping, association and credential omission, row and panel caps, `[reload]` face, `[using]`/`[mrql]` contexts, `[each]` first element only, a11y structure). `server/api_tests/inspect_shortcode_test.go` covers the preview panel, CSS slots, the association leak, the role gate, the display page, the deferred endpoint, the share page and the AI run-tool preview. E2E: `template-preview.spec.ts` (keyboard open, row, Tab to snippet, nothing on the saved page) and an axe scan in `category-template-authoring-a11y.spec.ts`.
