<!-- Shortcode idea that did not make the first implementation round. -->
> **Status:** idea, not implemented. The plan went through 2 plan/evaluate round(s).
> Last evaluation: consistency 8.1/10, usefulness 7.6/10, average 7.85/10. The bar was 8.5.
>
> **Pitch:** Builds an in-page table of contents from headings. It reads the h2-h4 elements in the rendered slot, and for notes it can also read the headings inside text blocks: [toc levels="2-3" source="blocks" title="On this page"]. It emits <nav aria-label> with nested lists, generates stable slug ids where headings have none, and skips collapsed [details] content it can't see. Long notes, wiki-style groups and documentation categories get navigation without hand-written anchor lists.

# Plan: `[toc]`, a table of contents for long content (revision 2)

Slug: `toc`. A built-in Go shortcode with no DB access, no plugin and no new package imports outside `golang.org/x/net/html`, which `shortcodes/lint.go` already uses.

## 0. What changed since revision 1, and why

| Evaluator point | Decision |
|---|---|
| 1. Blocks mode breaks layering (goldmark copied into `shortcodes/`) | **Cut.** `source=` is gone. Slot mode is now the whole feature. §13 records note-blocks navigation as a follow-up and the precondition it would need: one exported goldmark constructor shared by `markdown_filter.go` and any consumer. |
| 2. Heading blocks with no level | Moot, because blocks mode is cut. §13 records the rule for the follow-up: a missing, 0 or out-of-range level counts as 2, matching `blockEditor.tpl:114`. |
| 3. Reflection over `types.JSON` | Moot, because blocks mode is cut. §13 records the follow-up approach: a small interface over `models.NoteBlock.TextContent()`. |
| 4. Cross-slot use does not work | **Accepted and stated up front** (§1, §4 and the docs). Every `process_shortcodes` call is its own unit (`shortcode_tag.go:125`). The primary story is now: put `[toc]` in the Description itself. The second story is a template slot whose own `[partial]`s hold the headings. No cross-slot option is added, because it would need page-level state that the deferred `[reload]`/`[lazy]` re-render cannot reproduce. |
| 5. `<p>[toc]</p>` in Markdown makes `<p><nav>` | **Fixed in the design.** At resolve time, a `<p>` whose only non-whitespace content is the placeholder is replaced as a whole by the nav. A placeholder anywhere else inside a phrasing parent becomes a comment (§5). Covered by a Go test and an e2e DOM check. |
| 6. Wrong sanitizer rationale | **Corrected.** Forgery protection rests on the nonce alone. `sanitize_filter.go:179` does drop comments, but only under `-auth`, and the design does not rely on that. |
| 7. One toc per unit vs. CustomMRQLResult | **Verified and redesigned** (§4). `[mrql]` renders each item's CustomMRQLResult with `processWithDepth` inside the parent unit (`mrql_handler.go:360`). The `/mrql` page renders each item through its own `process_shortcodes` (`templates/partials/mrqlList{group,resource,note}.tpl:7`). Each item is now resolved as its own unit on both paths, so the result is the same on both surfaces: a per-item toc that lists only that item's headings. It is documented as not meant for repeated slots. |
| 8. Blocks staleness | Moot, because blocks mode is cut. §4 states what a note gets: its Description, and each text block as its own unit. |
| 9a. Charge before splicing | **Done.** Pass 1 computes the exact added bytes, and the handler calls `m.chargeOut` before any output is built. |
| 9b. Duplicate existing ids in a unit | **Decided.** The first heading with a given existing id is listed. A later heading with the same existing id is skipped, because its link would go to the first heading. Generated ids never collide (dedupe, §3). |
| 9c. Placeholder in an attribute or raw text | **Decided.** The placeholder is resolved only as an HTML comment token in text context. Any other occurrence is deleted. Lint reports a `[toc]` in tag, attribute or raw-text context as an error. |

## 1. Motivation and user value

Long Markdown Descriptions on wiki-style groups, documentation categories and long resource or note Descriptions have no in-page navigation. Authors write anchor lists by hand, and those lists drift when a heading is renamed. `[toc]` builds the list from the headings the same render actually produced.

**What it can list (stated plainly).** A `[toc]` lists only headings in the content rendered by the same `process_shortcodes` call it sits in. That call is one slot, one Description or one text block. So:

- **Primary use: a Description.** Typing `[toc]` on its own line at the top of a group, resource, note, category or relation Description lists that Description's `##`–`####` headings. goldmark already gives those headings ids (`markdown_filter.go:31`, `WithAutoHeadingID`), so the links just work. This is the wiki-group layout.
- **Secondary use: a documentation template.** A CustomHeader, CustomSidebar or CustomDetailFooter whose body is built from `[partial]`s with headings, for example a category that renders a standard "About this kind of item" guide.
- **Not supported:** a `[toc]` in a Category CustomHeader cannot list the group Description's headings, because they render in a different call. The docs say this and recommend putting `[toc]` in the Description instead.

Nothing in `docs/ideas/category-templates-and-shortcodes.md` covers this.

## 2. Syntax and attributes

```
[toc]
[toc levels="2-3" title="On this page"]
```

The shortcode is inline only (`IsBlock: BlockNo`). A closing `[/toc]` is reported by the existing generic "inline shortcode … cannot have a closing tag" lint. At runtime the body is ignored, as `[meta]` and `[partial]` already do.

| Attr | Type | Default | Rules |
|---|---|---|---|
| `levels` | string | `2-4` | `N` or `N-M` with digits 1–6 and N ≤ M. Surrounding whitespace is trimmed. Regex `^[1-6](-[1-6])?$`. A single `N` means `N-N`. The level is the tag number of `<hN>`. |
| `title` | string | `On this page` | The visible caption and the landmark's accessible name. An empty or whitespace-only value falls back to the default. The value is HTML-escaped. |

There are no other attributes: no numbering style, depth cap, collapsible form or `source`.

## 3. Rendering output

When there is at least one entry:

```html
<nav class="toc-shortcode" aria-label="On this page">
  <p class="toc-shortcode-title" aria-hidden="true">On this page</p>
  <ol>
    <li><a href="#toc-introduction">Introduction</a>
      <ol>
        <li><a href="#toc-scope">Scope</a></li>
      </ol>
    </li>
    <li><a href="#usage">Usage</a></li>
  </ol>
</nav>
```

### Accessibility

- The output is a `<nav>` landmark. Its `aria-label` equals the visible title, which meets WCAG 2.5.3 (label in name).
- The title is a `<p>`, never a heading. A heading could list itself and would add a level to the outline. The title is `aria-hidden`, so the name is not announced twice.
- Nested `<ol>` elements convey the hierarchy. Links are plain `<a href="#id">`. Native fragment navigation moves the sequential-focus start point, so no JavaScript is involved.
- The docs advise giving two tocs on one page different titles.

### Styling

Add `.toc-shortcode` rules to `public/index.css`, next to `details.details-shortcode`:

- list indentation;
- the existing amber link colours;
- a visible `a:focus-visible` outline.

Do not use `outline-none` anywhere (`internal/arch/forced_colors_focus_test.go`).

### Nesting algorithm

The algorithm is deterministic and always produces balanced HTML. It keeps a stack of open list levels. For each entry at level L:

1. If the stack is empty, open `<ol>` and push L.
2. If L is greater than the top of the stack, open a nested `<ol>` inside the still-open `<li>` and push L. A jump of several levels nests only one deeper.
3. Otherwise, close the current `<li>`. While the stack has more than one entry and L is less than the top, emit `</ol></li>` and pop. Then emit a sibling `<li>`. An entry below the root level becomes a root sibling.
4. At the end, close everything that is open.

### Entry text and cap

- **Entry text** is the heading's text content: the text tokens inside it, with nested markup dropped (mention links, `<code>`, emphasis), whitespace collapsed and trimmed, then re-escaped. A heading with empty text is skipped.
- **Cap.** `tocMaxEntries = 200`. Once 200 entries are collected, scanning for further headings stops, but the scan still collects ids for dedupe. The nav ends with `<!-- mr:toc truncated at 200 entries -->`.

### Ids and links

- **Existing id.** A heading that already has a non-empty `id` with no whitespace is linked as `href="#"+html.EscapeString(id)`. Description headings always take this path, since goldmark provides the ids and the sanitizer keeps `id` on headings.
- **Existing id that cannot be used.** If the existing id is empty or contains whitespace, the heading is skipped, because a second `id` attribute cannot be added.
- **Duplicate existing id.** If an existing id was already used by an earlier listed heading in this unit, the later heading is skipped (point 9b).
- **Heading without an id.** It gets a generated id, written into the output:
  - **Slug.** Apply `strings.ToLower`. Replace every rune that is not a Unicode letter or digit with `-`. Collapse runs of `-` and trim `-` at both ends. Truncate to 64 runes and trim again. If nothing is left, use `section`.
  - **Id.** `"toc-" + slug`. The prefix keeps generated ids clear of app ids such as `content` and of goldmark ids.
  - **Dedupe.** Pass 1 collects every `id` attribute in the unit. A generated id that collides with any of them, or with an id generated earlier, gets `-2`, `-3`, … appended.
  - **Stability.** Ids are stable for unchanged content. Inserting an earlier heading with the same text shifts the suffixes, and the docs say so.
  - **Scope of uniqueness.** Ids are unique within the unit. Two template slots on one page that both use `[toc]` and contain identically titled headings would generate the same id. The docs say to use one templated `[toc]` per page. Description ids come from goldmark and are not affected.
- **Splicing.** The new id is inserted right after the tag name, at byte offset 3 of the start tag's raw bytes: `<h2 class="x">` becomes `<h2 id="toc-intro" class="x">`. Every other byte is copied unchanged from `Tokenizer.Raw()`. The output is never re-serialised.

## 4. Render units and which headings are read

The processor renders in one pass. The handler therefore emits a placeholder, and the list is built after the unit finishes, which lets `[toc]` sit above the headings it lists.

### Placeholder

The placeholder is `<!--mr-toc:NONCE:IDX-->`.

- NONCE is 16 random bytes from `crypto/rand`, in hex. It is created lazily on the `renderMeter`.
- IDX indexes `m.tocs []tocSpec` (levels and title), guarded by `m.mu`.
- Content cannot forge a placeholder because it cannot know the per-render nonce. That is the whole argument. The `-auth` sanitizer's comment dropping is not relied on.

### Where `resolveTOCs(m, html)` runs

`resolveTOCs` resolves only the placeholders present in the string it is given. Every call site below makes its own string a unit.

| Call site | Unit |
|---|---|
| End of `ProcessChecked` (`render_budget.go:295`), after `processWithDepth`. Skipped if `m.Err()` is set. | One slot, one Description, one text block, or one `/v1/shortcodes/deferred` body. |
| `RenderLazyShortcode` inline fallback (`lazy_handler.go:46`) | The `[lazy]` body. |
| `RenderDetailsShortcode` inline fallback (`details_handler.go:38`) | The `[details]` body. |
| `RenderAutorefreshShortcode` (`autorefresh_handler.go:39`) | The `[autorefresh]` body. |
| `renderCustom` per item (`mrql_handler.go:360`) | One CustomMRQLResult item. |

**Why these sites.** The deferred endpoint renders `[lazy]` and `[details]` bodies through `ProcessChecked`, and the inline fallbacks resolve the same body string. A `[toc]` inside those blocks therefore lists that body's headings on every surface: page, preview, lightbox JSON and share page. The `/mrql` page renders each item with its own `process_shortcodes`, which is one `ProcessChecked` per item. Resolving each item at `mrql_handler.go:360` gives the `[mrql]`-in-a-slot path the same per-item answer. Without it, an item's `[toc]` would become the toc of the whole enclosing slot. Inner units resolve first, so their placeholders are gone before the outer unit scans.

**One toc per unit.** The first placeholder in a unit is filled. Any later one becomes `<!-- mr:only one [toc] per render -->`. This covers a second `[toc]` in a slot and a `[toc]` inside an `[each]` body, which stays in the parent unit.

**Repeated slots.** In a slot rendered once per card or row, each card is its own unit, and each card with headings gets its own nav. Such slots are CustomSummary, CustomAvatar, CustomHoverCard, CustomCell, CustomMRQLResult, and Descriptions on full-text list cards (`group.tpl:80`). The docs say `[toc]` is meant for detail-page content and that a toc in a repeated slot repeats per card. Most card slots contain no headings and so produce only the "found no headings" comment.

**Notes.** A note's Description works like any Description. A `[toc]` in a text block lists only that block's headings, because each text block is its own `process_shortcodes` call (`renderedTextBlocks.tpl:3`). Heading blocks are not listed (§13).

### Skipped subtrees

The scanner does not list headings inside the elements below. Each one is tracked by element name and a nesting counter until its matching close tag. Unbalanced markup skips the rest of the unit, which is the safe failure.

| Element | Reason for skipping |
|---|---|
| `details` (`[details]` fallback or hand-written) | Its content is collapsible and may be hidden. |
| `details-shortcode`, `lazy-shortcode` | Deferred; the content is not on the page yet. |
| `div` whose class attribute is exactly `lazy-content` | The `[lazy]` fallback, skipped so the preview matches the page. |
| `autorefresh-shortcode` | A refresh replaces the body and loses any generated ids. |
| `template`, `noscript` | Inert, or not shown to readers. |

**`[autorefresh]` without a refresh signer.** On the preview and share surfaces, `[autorefresh]` returns its body unwrapped (`autorefresh_handler.go:42`), so those headings are listed there. Nothing refreshes on those surfaces, so the links stay valid. The docs note this one difference.

**`[reload]`.** It re-renders the whole slot through the deferred endpoint, which calls `ProcessChecked` again, so the toc is rebuilt with identical ids.

### Scanner

The scanner uses the `golang.org/x/net/html` Tokenizer and runs two passes.

- **Pass 1** collects:
  - heading start offsets, levels, text and existing ids;
  - every `id` attribute;
  - each comment token whose data is exactly `mr-toc:NONCE:IDX`, with its context: the `<p>` start offset when the comment is the sole non-whitespace child of an open `<p>`, and whether any phrasing parent is open (`p`, `span`, `a`, `button`, `label`, `h1`–`h6`).

  Raw-text elements are handled by the tokenizer. A placeholder inside `<script>`, `<style>`, `<textarea>` or `<title>` is a text token, not a comment.
- **Pass 2** splices the generated ids and the nav. It then deletes every remaining literal occurrence of this meter's placeholder strings, which can only sit in attribute values or raw text.

## 5. Empty, missing and invalid inputs

| Case | Output |
|---|---|
| Invalid `levels` (regex miss, or N > M) | `shortcodeErrorMarker("toc", "[toc] levels= must be N or N-M with 1 ≤ N ≤ M ≤ 6")` |
| Inside a `[reload]` face (`withinReloadFace`) | Error marker `[toc] cannot be used inside a [reload] button face` |
| Placeholder is the sole content of a `<p>` (Markdown `[toc]` on its own line, or a hand-written `<p>[toc]</p>`) | The whole `<p>…</p>` is replaced by the nav (or by the comment below). No empty `<p>` is left. |
| Placeholder inside any other phrasing parent (`See [toc] here`, `<span>[toc]</span>`, a Markdown line followed by a hard-wrapped line) | `<!-- mr:toc must stand in its own paragraph -->` |
| Placeholder in an attribute value or raw text | Deleted (lint reports it, §9) |
| No headings match | `<!-- mr:toc found no headings -->`. An empty `<nav>` would be an empty landmark, so none is emitted. |
| Second and later `[toc]` in a unit | `<!-- mr:only one [toc] per render -->` |

## 6. Scoping and authorization

`[toc]` performs no query and loads nothing. It reads only the HTML its own unit produced. That HTML already passed through every scoped executor (`[mrql]`, `[using]`, `[conditional mrql]`) and, for Descriptions and text blocks, through `sanitize_under_auth`.

A group-subtree-confined principal therefore cannot see any entry text that is not already a heading on the page they are allowed to render. The only data source is output that already failed closed upstream.

Entry text and hrefs are always escaped. `UntrustedSource` needs no special case because `[toc]` has no raw option. Generated ids contain only letters, digits and `-`.

## 7. Performance

- **Queries:** zero, so there is no `QueryBudget` charge.
- **CPU:** one extra tokenizer pass over a unit's output, run only if that unit emitted a placeholder. A `strings.Contains` check for `"<!--mr-toc:"+nonce` comes first, so units without `[toc]` pay nothing beyond that substring check, and only when the meter has a nonce.
- **Output bytes:** already bounded by `RenderBudget.OutputBytes`. Pass 1 computes the exact added bytes (the nav plus `len(` id="…"`)` per spliced heading, minus the removed placeholder and `<p>` wrapper bytes when positive) and calls `m.chargeOut(n)` before pass 2 allocates. On failure, `resolveTOCs` returns `""` and the meter error makes `ProcessChecked` return `""` and the error, like any budget breach.
- **Scale:** nothing grows with the number of resources. List cards pay one small pass only when their slot contains `[toc]`.

## 8. Where it lives, and why

It is a built-in Go handler in `shortcodes/`. A plugin shortcode cannot work here for three reasons:

1. `[toc]` must post-process the whole unit's output and splice ids into it. A plugin sees only its own call.
2. It must hook the meter and the `[lazy]`, `[details]`, `[autorefresh]` and `[mrql]` unit boundaries.
3. `[plugin:*]` is disabled for confined principals, so a plugin version would silently disappear for them.

## 9. Files to change

**`shortcodes/toc_handler.go` (new).**
- `RenderTOCShortcode(reqCtx, sc, ctx)`: validates `levels`, handles the reload face, registers a `tocSpec` on the meter and returns the placeholder.
- `resolveTOCs(m *renderMeter, html string) string`.
- The scanner and splicer, the slug and dedupe helpers, and the nav builder.
- Constants `tocMaxEntries = 200`, `tocDefaultLevels = "2-4"`, `tocDefaultTitle = "On this page"`.

**`shortcodes/processor.go`.** Add `case sc.Name == "toc": replacement = RenderTOCShortcode(reqCtx, sc, ctx)`.

**`shortcodes/render_budget.go`.**
- Add `tocNonce string` and `tocs []tocSpec` fields to `renderMeter`.
- In `ProcessChecked`, add `if m.Err() == nil { out = resolveTOCs(m, out) }`. Then check `m.Err()` as today.

**One-line additions** in `shortcodes/lazy_handler.go`, `details_handler.go` and `autorefresh_handler.go`: `inner = resolveTOCs(meterFrom(reqCtx), inner)` after the existing body `processWithDepth`. In `shortcodes/mrql_handler.go:360`: `piece = resolveTOCs(m, piece)`.

**`shortcodes/builtin_docs.go`.**
- New `BuiltinDoc{Name: "toc", IsBlock: BlockNo, …}` with `levels` (default `2-4`) and `title` (default `On this page`).
- Examples: `[toc]` and `[toc levels="2-3" title="On this page"]`.
- Add `toc` to the list in the `BuiltinDoc` doc comment.

**`shortcodes/lint.go`.**
- Add `"toc"` to `builtinBaseNames`.
- Add `case "toc":` reporting these as errors:
  - an invalid `levels`;
  - a `[toc]` inside `reloadSpans` (same message as runtime);
  - `opts.CSSMode`: `[toc] has no effect in a stylesheet`.
- Add `[toc]` opener spans to the spans passed to the existing `attributeContextsForW` call (`lint.go:335`). If the context has `inValue`, `inName` or a non-empty `rawTextElement`, report the error `[toc] must be placed in text, not inside a tag, attribute or <script>/<style>`.

**`public/index.css`.** Add the `.toc-shortcode` styles.

**Reference panels.** One `[toc]` entry each in `templates/createCategory.tpl`, `createResourceCategory.tpl` and `createNoteType.tpl` (required by `TestReferencePanelsCoverAllBuiltins`).

**`docs-site/docs/features/shortcodes.md`.**
- Table row ``| `[toc]` | Inline | Table of contents from the headings in the same rendered content |`` (required by `TestDocsSiteShortcodesPageCoversAllBuiltins`).
- New section `## [toc] -- Table of Contents` covering:
  - attributes;
  - "put it in the Description" as the main example;
  - the same-content-only rule, with the CustomHeader vs Description example;
  - own-paragraph placement;
  - skipped blocks and the `[autorefresh]` preview note;
  - one per render;
  - per-card repetition in repeated slots;
  - id rules (prefix, suffix shifts, uniqueness within one slot);
  - accessibility advice.
- Add the new comments and markers to "Failure markers".

**Other docs.**
- `docs-site/docs/features/custom-templates.md`: one bullet.
- `.claude/skills/mahresources-category-designer/references/shortcodes.md`: a `[toc]` entry.
- `docs/plans/2026-11-24-toc-shortcode.md`: this plan, followed by `./docs/plans/generate-index.sh`.

No template, model or other server files change.

## 10. Tests

### Go unit tests: `shortcodes/toc_handler_test.go`, all through `Process` or `ProcessChecked`

1. **Basic.** `[toc]` above `<h2>A</h2><h3>B</h3><h2>C</h2>`. Assert the exact nav and nesting, the ids `toc-a`, `toc-b` and `toc-c`, and that all other bytes are unchanged.
2. **Existing ids and attributes.** An existing id is reused. `<h2 class="x">` gets its id spliced after the tag name, and case and attributes are preserved. A duplicate existing id is listed once. An empty `id=""` heading is skipped.
3. **Levels.** The default drops h1, h5 and h6. `levels="3"` keeps only h3.
4. **Nesting.** For h2→h4→h2 and for h3 then h2, parse the output with `x/net/html` and compare the tree. The lists must be balanced.
5. **Slugs.** Unicode text, punctuation-only text (`section`), 64-rune truncation, and collisions with an existing `toc-a` and with repeated text (`-2`, `-3`).
6. **Markdown paragraph.** Run the Description chain (`strip_template_comments | markdown2 | sanitize_under_auth`, auth on and off) on `"[toc]\n\n## One\n\n## Two"` and then `Process` it:
   - the output contains no `<p>` wrapping the nav and no empty `<p>`;
   - the hrefs equal goldmark's ids `#one` and `#two`.

   `"See [toc] here"` gives the own-paragraph comment. This test lives in `server/template_handlers/template_filters/toc_description_test.go` because it needs the filters.
7. **Skipped subtrees.** Headings are not listed when they sit inside:
   - an inline `[details]`;
   - a hand-written `<details open>`;
   - a `[lazy]` body, on both the signer path and the `lazy-content` fallback;
   - a signed `[autorefresh]`;
   - `<template>` or `<noscript>`.

   Headings around them are listed. With no refresh signer, `[autorefresh]` headings are listed.
8. **Render units.** A `[toc]` inside `[details]` or `[lazy]` lists only that body, both inline and through `Process` of the body alone (the deferred path), with identical output.
9. **CustomMRQLResult.**
   - Stub the executor to return three items whose CustomMRQLResult is `[toc]<h2>Item N</h2>`, in a slot that also has an `<h2>Outer</h2>` and its own `[toc]`. Each item's nav lists only `Item N`, and the slot's nav lists only `Outer`.
   - The same item template rendered by its own `Process` (the `/mrql`-page path) gives the same per-item nav.
10. **One per render.** A second `[toc]`, and a `[toc]` in an `[each]` over three items, give one nav plus the "only one" comment.
11. **Empty and invalid input.**
    - No headings gives the comment and no `<nav>`.
    - Bad `levels` values (`0`, `7`, `4-2`, `a`) give the error marker.
    - Inside a `[reload]` face gives the error marker.
    - An empty `title` uses the default.
    - `<script>` in the title is escaped.
12. **Escaping.** `&lt;b&gt;` in a heading round-trips escaped. Nested `<a>` or `<code>` in a heading yields plain text.
13. **Context.**
    - `<a title="[toc]">` and `<script>var x="[toc]"</script>` leave no placeholder text and no nav in that position.
    - A literal `<!--mr-toc:0000…:0-->` with a guessed nonce is left untouched.
14. **Cap.** 250 headings give 200 entries plus the truncation comment.
15. **Budget.** A tiny `OutputBytes` budget fails at resolution with `ErrRenderBudgetExceeded`. `ProcessChecked` returns `""`, and no partial splice is observable.
16. **Scoping invariant.** With an executor stub that returns a confined result set rendered through `[mrql]`, every toc entry's text equals the text of a heading in the rendered output.

### Lint: `shortcodes/lint_test.go`

- An invalid `levels` is an error.
- `[toc]` in `[reload]…[/reload]` is an error.
- `[toc]` with `CSSMode` is an error.
- `[toc]` inside an attribute value, a tag name and `<script>` are errors.
- `[toc]…[/toc]` draws the existing closing-tag error.
- `[tco]` suggests `toc`.
- `TestBuiltinDocMRQLExamplesParse` and both docs-coverage tests pass.

### E2E: `e2e/tests/shortcodes/toc-shortcode.spec.ts`, ephemeral server, API client

1. **Group Description.** A group whose Description is `[toc]` followed by `##` and `###` sections:
   - `getByRole('navigation', {name: 'On this page'})` is visible;
   - the links equal the headings;
   - every fragment resolves with `document.getElementById`;
   - `page.locator('.description p:has(nav)')` has count 0 and there is no empty `<p>` before the nav;
   - clicking a link sets `location.hash` and brings the heading into the viewport.
2. **Category template.** A Category whose CustomHeader is `[toc]`, a `[partial]` with h2 and h3 sections, and a `[details]` holding an h2:
   - the toc lists the partial's headings but not the `[details]` heading;
   - the injected `toc-` ids exist.
3. **Reload.** A `[reload]` in that CustomHeader re-renders the slot, and the nav and ids are identical afterwards.

### Accessibility: `e2e/tests/accessibility/toc-shortcode-a11y.spec.ts`

- An axe scan (a11y fixture) of both pages above, expecting no violations: labelled nav, no empty landmark, no duplicate ids introduced.
- A keyboard check: Tab reaches each toc link in order, and the focus outline is visible (computed `outline-style` is not `none`).

Before and after the change, run `go test --tags 'json1 fts5' ./...`, `cd e2e && npm run test:with-server:all`, `./scripts/css-scan-test.sh` and the Postgres suites. The Postgres suites are unaffected because `[toc]` uses no SQL.

## 11. Docs to update

See §9:

- `shortcodes.md`: the section, table row and failure markers;
- `custom-templates.md`: one bullet;
- the three in-app reference panels;
- the category-designer skill reference.

Editor autocomplete, hover docs and lint descriptions come from `BuiltinDocs` automatically.

## 12. Success criterion

1. A Description containing `[toc]` on its own line renders, on the detail page, the lightbox JSON and the share page, a `<nav aria-label="On this page">` that is not wrapped in a `<p>`. Its nested `<ol>` lists exactly that Description's h2–h4 headings in document order, and every link resolves.
2. A template slot containing `[toc]` and headings (directly or via `[partial]`) lists them, skips headings in `<details>`, `[lazy]` and signed `[autorefresh]`, gives id-less headings stable `toc-…` ids, and keeps them across a `[reload]`.
3. A `[toc]` inside `[lazy]`, `[details]` or a CustomMRQLResult item lists only that body's headings, identically on every render path.
4. Invalid input and placement give the documented markers and lint errors. No headings gives a comment and no landmark.
5. The feature adds zero queries, and resolved output stays within the existing output budget.
6. A subtree-confined user never sees a toc entry that is not a heading already rendered for them.
7. axe reports no violations, and keyboard navigation reaches every link with a visible focus ring.
8. All new and existing Go, lint, E2E and a11y suites pass.

## 13. Explicitly out of scope (follow-up, not built here)

**Note heading-block navigation** (the former `source="blocks"`) is out of scope. If it is taken up later, it requires:

1. One exported goldmark constructor in a shared package, used by both `markdown_filter.go` and the toc. No copied configuration.
2. A small interface over `models.NoteBlock` (`ID`, `Type`, `TextContent()` and a heading-level accessor), with no reflection.
3. A missing, 0 or out-of-range heading level counts as 2, matching `blockEditor.tpl:114`.
4. A documented saved-state-only staleness rule for blocks edited client-side.

It is cut now because it is the riskiest part, a niche use, and would roughly double the surface area.

---

## Last evaluator feedback

I checked the plan's claims against the code. Most of them hold:
- `markdown_filter.go:31` has `WithAutoHeadingID`.
- The sanitizer keeps `id` on h1–h6 (`sanitize_filter.go:83`) and drops comments only under `-auth`.
- `ProcessChecked` is at `render_budget.go:286`.
- `Process` delegates to `ProcessChecked`, which is the path `shortcode_tag.go:125` uses.
- The inline fallbacks for `[lazy]`, `[details]` and `[autorefresh]` and the per-item `processWithDepth` in `mrql_handler.go` (~360) match the plan.
- `renderMeter` has `mu`, `chargeOut` and `Err`.
- The reference-panel and docs-site coverage tests exist.
- `builtinBaseNames` and `attributeContextsForW` exist.
- Descriptions run sanitize before `process_shortcodes`, so the placeholder comment survives `-auth`.

**Factual errors to fix (these cost the most points):**

1. **The share page does not process Description shortcodes.** `templates/shared/displayNote.tpl:16` renders `{{ note.Description|strip_template_comments|markdown2|sanitize_under_auth|safe }}` with no `process_shortcodes`. Shared text blocks go through `sharedBlock.tpl`, not `renderedTextBlocks.tpl`. So success criterion 1 ("detail page, the lightbox JSON and the share page") is false. A `[toc]` in a shared note's Description shows up as the literal text "[toc]".
   - The plan must decide the share-page behaviour. Either strip or ignore the `[toc]` text there (for example with a filter), or document that it shows literally.
   - It must also correct the success criterion and the e2e list to match.
2. **The lightbox JSON path does not render Descriptions through shortcodes either.** `processShortcodesForJSON` in `server/routes.go:431` only processes category slots (CustomHeader, CustomSidebar and so on). Drop "lightbox JSON" from the Description claims, or name the exact code path that does render them. That might be `description_preview_api_handlers.go:92`. If so, list it, because it is another render surface that needs to be covered and tested.
3. **The Description chain in test 6 is incomplete.** The real chain (`templates/partials/description.tpl:17`, `renderedTextBlocks.tpl:3`) is `strip_template_comments|markdown2|render_mentions|sanitize_under_auth`. Add `render_mentions` so the test matches production. It matters because mention links inside headings are an explicit text-extraction case.

**Gaps and ambiguities:**

4. **Note text blocks render inside `<template data-rendered-text-block>`, and JavaScript moves them into the DOM.** The plan should confirm that the nav and fragment links work after that client-side move. It should also add one e2e check on a note text block, since that is a stated use.
5. **Goldmark ids can collide with application ids on the page.** A heading named "Content" or "Description" gets the goldmark id `content` or `description`. The plan protects only generated ids with the `toc-` prefix. Under the "every link resolves" criterion, a Description toc link could jump to an app element instead. Either state this as a known limit, or add a test that rules it out on the group detail page.
6. **Pass 2 removes every leftover placeholder, but the plan does not say how a parent unit treats placeholders left from inner units.** The plan says inner units resolve first. It should also say what happens to a placeholder from an inner unit that failed or was budget-truncated, so that the parent unit never fills an inner unit's placeholder by mistake.

**Scope and over-engineering:**

7. Five resolution call sites, a per-meter nonce, two-pass splicing, a `<p>`-unwrapping context tracker, a skip-subtree counter and lint attribute-context checks add up to a large surface for a single feature.
   - Dropping the CustomMRQLResult per-item resolution would cut a call site and a test group. Documenting `[toc]` as unsupported in repeated slots is enough, since a toc there is not useful anyway.
   - `aria-hidden` on a visible title together with `aria-label` works, but `aria-labelledby` pointing at the title `<p>` is the simpler, more conventional pattern and avoids hidden visible text.

**Usefulness:**

8. The value is real but moderate. Long, wiki-style Descriptions with headings are a minority use in a personal information manager whose main objects are resources, notes and groups.
   - The biggest template use is not supported: a category CustomHeader that lists the Description's headings. Users have to opt in by typing `[toc]` into each Description, so category authors cannot apply it once to every item in a category.
   - Making it work for a category-wide CustomHeader or CustomSidebar, for example with a small client-side progressive-enhancement fallback that reads the rendered `.description` headings, would raise usefulness a lot. That would have to be weighed against the documented page-state concerns.
