# Plan: `[excerpt]`, a safe plain-text preview of long text (revision 3, implemented)

> **Pitch:** Renders a length-bounded plain-text excerpt of an entity's long text, such as a note's Description or a meta text field, with word-boundary truncation, an ellipsis and an optional 'Read more' link. HTML and markdown are stripped server-side, so the excerpt can't break card layouts or inject markup. CustomSummary cards and MRQL result templates can show meaningful previews instead of names only.
>
> Implemented on `feat/shortcode-excerpt`. Where the implementation differs from the text below, the Review section at the end says how and why. §0 records the revision-2 to revision-3 changes and is kept as history; on the share page in particular, §6 and the Review supersede it.

## 0. Changes since the previous version (response to evaluation)

| # | Feedback | Resolution |
|---|---|---|
| 1 (P1) | `ForceReadOnly` is not share-only. It is also set for every principal without write access (`shortcode_tag.go:115`, `description_preview_api_handlers.go:88`), and `[link]` runs normally on share pages. | **Agreed. The share-page suppression is removed.** `[excerpt]` ignores `ForceReadOnly`, the same as `[link]` and `[property]`. The `more` link renders for read-only principals and on share pages alike, using the same `resolveLinkURL("self")` URL that `[link]` emits there. No new context flag is added. New tests check that `ForceReadOnly: true` keeps the link (unit) and that a read-only guest gets the link over HTTP (API). §6, §11 and §12 are updated. |
| 2 | Extending `otherContext` collection is unnecessary. | **Agreed.** `otherContext` is already built exactly when `metaProps` is non-empty, and `metaPathIssue` does nothing without `metaProps`. The `excerpt` lint case reuses the `[meta]` branch pattern unchanged (§10.5). |
| 3a | Mention names were substituted before Markdown, so `*`, `_` and `<` in a name were re-interpreted. | **Fixed by design.** Before the Markdown step, each mention becomes an opaque private-use sentinel. Names are restored only after the HTML has become text, so they come out verbatim. A test pins `@[resource:5:a*b*_c_<d>]` → `a*b*_c_<d>` (escaped). |
| 3b | Shortcodes inside code fences. | **Documented.** Shortcode spans are deleted from the raw source before Markdown runs, so a shortcode inside a fenced or inline code span is deleted too. The surrounding code text is kept. A test pins this. |
| 4 | Comment stripping ran on the full text before the cap. | **Cap first.** The 16 KiB cap is now step 1, and template-comment stripping runs only on the capped prefix, so the work per call is bounded however large the Description is. If the cut lands inside a `[comment]`, the opener is left unclosed. `templatecomment.Strip` then hides everything from the opener to the end of the prefix, which fails closed: comment text never leaks. A test pins this. |
| 5 | `default` escaping differs from `[property]`. | **Stated explicitly.** `default` is always HTML-escaped and never raw. There is no `raw` attribute at all. The docs say this. |
| 6a | The `len*4` scratch multiplier was arbitrary. | **Replaced with a derived charge of `12 × len(capped)`** (§8), broken down per buffer in the same "reserve before allocate" spirit as `mrql_slots.go`. |
| 6b | Is the stray-closer lint driven by `IsBlock`? | **Confirmed.** `lint.go:131` builds `Known` from `BuiltinDocs()` with `Block: d.IsBlock`, and `lint.go:302` reports "[x] is an inline shortcode and cannot have a closing tag" when `known.Block == BlockNo`. No separate list needs editing. |
| 6c | The goldmark config is duplicated. | **Cross-referencing comments in both files, plus a parity test** in `template_filters` that compares `[excerpt]` output with the visible text of `markdown2` output over a fixed corpus (§11). |

## 1. Motivation and user value

`CustomSummary`, `CustomMRQLResult`, `CustomPanelSummary` and `[mrql]`/`[using]` block bodies can show an entity's name. They have no safe way to show a short preview of its long text. Authors have two options today, and both are bad:

- `[property path="Description"]` dumps the whole stored Markdown source, including `**`, `#`, `@[resource:5:x]` markers, raw HTML (escaped into visible `<b>` text) and embedded shortcode source. It has no length bound, so a 40 KB note stretches its card.
- `[property path="Description" raw="true"]` emits raw markup into a card. That breaks the layout and, with auth off, injects HTML.

The built-in default MRQL card (`renderFlatDefault`: escaped Description plus `line-clamp-2`) still ships the whole text, shows the Markdown syntax, and cannot be reused by template authors.

`[excerpt]` is one inline shortcode. It turns an entity's long text into a short, single-line, HTML-escaped plain-text preview, cut at a word boundary, with an ellipsis and an optional accessible "Read more" link. Note cards, resource cards and MRQL result lists can then show what an item says, not only what it is called.

It is not in `docs/ideas/category-templates-and-shortcodes.md`, and no existing shortcode or plugin does it (checked: `shortcodes/` and `plugins/*/plugin.lua`).

## 2. Scope decisions (what the pitch had and this plan drops)

- **`source="blocks"` is dropped.**
  - A note's first text block mirrors `Note.Description` (`models/note_model.go`), so the Description already is the note's text.
  - `Blocks` is preloaded only on the note detail page (`note_context.go`). List cards, panels and MRQL items do not load it.
  - Reading blocks would mean either a DB load per card (N+1 at millions of notes) or a shortcode that renders nothing on the surfaces it exists for.
  - Like `[property]`, `[excerpt]` never triggers a DB load.
- **Search-term highlighting is dropped.** No card or MRQL slot has a search term in scope, so the attribute would have no input.
- **Dot paths in `field` are rejected.** Previewing a related entity goes through `[using]`, which loads the target through the scoped executor. `[excerpt]` therefore reads only the current entity's own columns and never a preloaded association.

## 3. Syntax and attributes

```
[excerpt]                                    → Description, 30 words
[excerpt words="40" more="Read more"]
[excerpt path="summary.text" words="20"]     → a Meta string
[excerpt field="Name"]                       → another top-level string field
```

The shortcode is inline only (`IsBlock: BlockNo`). `[/excerpt]` is reported by the existing inline-closer lint (`lint.go:302`). The `path` attribute (Meta) and `field` attribute (entity field) mirror `[conditional]`.

| Attribute | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `path` | string | no | — | Dot-notation path into the entity's Meta (`cooking.notes`). The value must be a JSON string. Cannot be combined with `field`. |
| `field` | string | no | `Description` (when `path` is also absent) | One exported **string** field of the current entity (`Description`, `Name`, `OriginalName`, …), with no dots. Credential fields (`credentialFields` in `shortcodes/credential_fields.go`: `ShareToken`, `TokenHash`, `CsrfToken`, `PasswordHash`) follow `[property]`'s rule: they are empty unless credentials are visible. Cannot be combined with `path`. |
| `words` | integer | no | `30` | Maximum number of words, from 1 to 200. A value that is not an integer, or is below 1, falls back to 30. A value above 200 is clamped to 200. |
| `more` | string | no | — (no link) | Visible label of a link to the current entity's detail page. The link appears only when the text was actually truncated. |
| `default` | string | no | — | Text shown when the resolved text is empty after stripping. It is **always HTML-escaped, never raw.** |

There are no other attributes and there is no `raw`.

## 4. Text pipeline (server-side, `shortcodes/excerpt_handler.go`)

`excerptText(m *renderMeter, src string) (text string, truncated bool, ok bool)` is a pure function of `src`:

1. **Cap the source** at `excerptSourceCap = 16 << 10` bytes, cut back to a valid UTF-8 boundary (`utf8.RuneStart`). If the source was cut, `capped = true`.
   - The largest possible output is 200 words × 12 runes (step 8), which is far below 16 KiB. The cap therefore never shortens a normal excerpt. It only bounds the work done on a pathological 1 MB Description.
2. **Charge the render meter**: `m.chargeScratch(12 * int64(len(capped)))` (§8). If the charge fails, return `ok = false`; the handler then outputs `""`.
3. **Remove template comments** with `stripTemplateComments(capped)` (`shortcodes/comment.go`).
   - If the cap cut a `[comment]` open, the unclosed opener hides the rest of the prefix (`templatecomment.Strip` semantics), so comment text cannot leak.
4. **Remove shortcodes without expanding them.** Run `parseWithBlocksM(nil, …)` on the text, delete every top-level span `[sc.Start, sc.End)`, then delete any closer left with no opener (`closingTagPattern`).
   - A block span includes its body, so `[details]secret[/details]` disappears entirely.
   - Nothing is executed. An `[excerpt]` in a Description cannot recurse, and an `[mrql]` inside it runs no query.
   - This runs on raw source before Markdown, so shortcodes inside fenced or inline code are deleted too. The rest of the code text is kept.
   - If the cap cuts a block opener off from its closer, only the opener tag is removed and the body text stays. That text is the same entity's own Description, which the viewer can already read.
5. **Turn mentions into sentinels.**
   - First delete any U+E000/U+E001 already in the text, so no collision is possible.
   - Then replace the i-th marker `mentions.ParseAll` would report with `"\uE000" + strconv.Itoa(i) + "\uE001"` and keep `names[i] = Name`. The replacement walks match positions (`mentions.ReplaceAll`, added for this), not `OriginalMatch` text, so a valid marker's text inside a skipped one (`@[group:0:@[note:1:x]`) is not substituted.
   - Private-use runes and ASCII digits have no meaning in Markdown or HTML, so the sentinel passes through unchanged.
6. **Convert Markdown to HTML** with a package-level `excerptMarkdown = goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithRendererOptions(html.WithUnsafe()))`.
   - This is the same parser shape as the `markdown2` filter (`server/template_handlers/template_filters/markdown_filter.go`). Its two other options, `WithAutoHeadingID` and `WithHardWraps`, change only attributes and `<br>`, and step 7 turns `<br>` into a space either way.
   - Unsafe mode is fine because none of this HTML is ever emitted, only its text.
   - Both files get a comment pointing at each other, and the parity test in §11 ties them together.
7. **Convert HTML to text** with the `golang.org/x/net/html` tokenizer:
   - Keep `TextToken` data, with entities decoded.
   - Drop comments and the text of raw-text elements (`script style textarea title xmp iframe noembed noframes noscript plaintext`).
   - The start and end tags of `p div br li tr td th h1–h6 blockquote pre hr ul ol table dd dt section article` insert a space.
   - `<img>` produces no text, so alt text is not included.
8. **Restore names and truncate.**
   - Replace each sentinel `\uE000<i>\uE001` with `names[i]`. An opener or closer that is not part of a well-formed sentinel with an in-range index is stripped; the digits after an unpaired opener stay as text.
   - Collapse every Unicode whitespace run to one space and trim. The output is always one line.
   - Split with `strings.Fields`. If there are more than `words` words, keep the first `words`.
   - If the kept text is longer than `words × 12` runes (CJK text, long URLs), cut it at that rune count.
   - If either rule cut the text, or `capped` is set, then `truncated = true`: trim trailing `` ,;:.-–—`` and append `…` (U+2026).

## 5. Source resolution

- **`path` set:** read the value with `ctx.rawValueAtPath(path)`, which uses the shared `decodedMeta` memo.
  - A `string` goes through the pipeline.
  - Any other JSON type (number, bool, object, array, null), or a missing key, gives the empty state.
- **`field` set, or neither set (in which case `field = "Description"`):**
  - A field that contains `.` gives the empty state. There is no separate validator: `traversePropertyPath(ctx.Entity, field, ctx.credentialsVisible)` on the single segment already refuses a missing field, an unexported one (`CanInterface`) and a withheld credential field (`credentialFields`, `shortcodes/credential_fields.go`), which is what `[conditional field=]` checks inline (`conditional_handler.go`).
  - Only a value whose kind is `reflect.String`, after dereferencing a pointer, goes through the pipeline. Anything else gives the empty state.
- **`path` and `field` both set:** render `shortcodeErrorMarker("excerpt", "set path or field, not both")`. Lint reports this at edit time.
- **`ctx.Entity == nil`:** `field` gives the empty state. `path` still reads `ctx.Meta`.
- **Empty state:** output `html.EscapeString(default)` when `default` is set, otherwise `""`. Text that is empty after stripping (only a shortcode, a comment or an image) also gives the empty state.

## 6. Rendered output and accessibility

- **Not truncated:** `html.EscapeString(text)`. This is a bare value with no wrapper element, so it is valid inside a quoted attribute (`title="[excerpt words='20']"`), exactly like `[property]`. That holds only without `more` (a cut text with `more` carries a link), which is why `emitsBareValue` returns true for `[excerpt]` only when `more` is blank.
- **Truncated, without `more`:** `html.EscapeString(text)` followed by `…`.
- **Truncated, with `more` set, and `resolveLinkURL("self", ctx)` returns ok:**
  ```html
  Escaped text… <a href="/note?id=42" class="underline" aria-label="Read more: Escaped Name">Read more</a>
  ```
  - If `resolveLinkURL` does not return ok (unknown entity type, ID 0), the link is omitted and the ellipsis is kept.
  - The visible label is `html.EscapeString(more)`.
  - The `aria-label` is the label, `: `, and the escaped `Name` (`traversePropertyPath(ctx.Entity, "Name", …)`), or `<entityType> <id>` when there is no name. Repeated "Read more" links on a list therefore have distinct accessible names (WCAG 2.4.4), and each name starts with the visible label (WCAG 2.5.3). (An `aria-label`, not an `sr-only` span: `docs/lessons.md` records that `.sr-only` is `position: absolute` and escapes a card's scroll container.)
  - The `underline` class means the link does not depend on colour alone (WCAG 1.4.1).
  - The literal `…` character is announced naturally by screen readers, so no ARIA is needed.
- **`ForceReadOnly` does not affect the output.** Read-only principals get the link, because it is navigation, not an edit affordance.
- **The public share page gets no link.** `processShareTemplates` (`share_server.go`) renders only a shared note's `NoteType.CustomHeader`/CSS, never `CustomSummary`, and the share server serves no `/note?id=` route, so a link there would be dead and would expose the internal note ID. The share server marks its render with `shortcodes.WithPublicSharePage`, and `[excerpt]` drops `more` there (the ellipsis stays). `[link]` keeps its existing behaviour; this plan does not change it.
- **No link inside a `[reload]` button face** (`withinReloadFace`): a button cannot contain a link.
- **`UntrustedSource` does not change the output**, because the output is always escaped. A test pins this.

## 7. Scoping and authorization

`[excerpt]` issues no query and opens no new data path:

- It reads only `ctx.Meta` and a top-level column of `ctx.Entity`. The surface that rendered it has already loaded that entity in scope, through scoped `Find`, the scoped `QueryExecutor` (`[mrql]` items get their context from `contextFromItem` in `shortcodes/mrql_handler.go`), or `[using]`'s scoped target load.
- Dot paths are refused, so preloaded associations are never read.
- Credential fields go through `traversePropertyPath`'s `credentialFields` guard.
- The link targets only the current entity.
- Mentions contribute only name text already stored in the source.
- Embedded shortcodes are deleted, not run.

A group-subtree-confined principal therefore sees an excerpt only of entities it was already shown. The API test in §11 proves this for both `[mrql]` and `[using]`.

## 8. Performance (millions of resources)

- There are **zero DB queries** per call, so `[excerpt]` adds nothing to the MRQL query budget (`query_budget.go`).
- **Bounded work.** Every step runs on at most 16 KiB, so a 50-card list parses at most about 800 KiB however large its Descriptions are. Neither comment stripping nor anything else touches the bytes past the cap.
- **Scratch charge `12 × len(capped)`** is reserved before allocation (`chargeScratch`, which caps only limited meters, for example budgeted template-generation previews). It is a per-buffer **heuristic, not an upper bound**: the goldmark AST share is the soft part. A node is about a hundred bytes, and pathological input (`*a*`, `- a\n`, `|a|` repeated) makes one every two to four bytes, far more than two bytes per source byte. The 16 KiB source cap is what bounds the work either way:

  | Buffer | Bytes per source byte |
  |---|---|
  | comment-stripped copy | 1 |
  | shortcode-deleted copy | 1 |
  | sentinel copy | 1 |
  | goldmark AST nodes (segments point into the source; heuristic, see above) | ≈2 |
  | rendered HTML buffer (worst case `"` → `&quot;`) | up to 6 |
  | tokenizer buffer and text builder | 1 |

  That totals 12. At the 16 KiB cap the charge is 192 KiB per call, which leaves room for more than 300 maximum-size excerpts inside the 64 MiB scratch limit. Typical Descriptions are charged far less, because the charge is proportional to their length.
- Output is at most about 2.4 KB plus the link, and the processor loop charges it as usual.

## 9. Where it lives, and why

It is a **built-in Go handler** in `shortcodes/`, not a plugin:

- It must run on every surface: share server, previews, and contexts with no plugin renderer or executor.
- It reuses unexported helpers: `traversePropertyPath`, `credentialsVisible`, `rawValueAtPath`, the memoised Meta, `resolveLinkURL`, `stripTemplateComments`, `parseWithBlocksM` and the meter.
- A Lua plugin would see only serialized data, would have to reimplement Markdown and HTML stripping, and would render as "plugin unavailable" for scoped accounts without plugin access.
- It is stateless pure text processing, the same kind of handler as `[property]`.

## 10. Files to change

**Go**

1. **`shortcodes/excerpt_handler.go`** (new):
   - `RenderExcerptShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext) string`
   - `excerptText`, `parseExcerptWords`
   - constants `excerptSourceCap = 16 << 10`, `excerptMaxWords = 200`, `excerptDefaultWords = 30`, `excerptRunesPerWord = 12`, `excerptScratchPerByte = 12`
   - `excerptMarkdown`, with a comment that names `markdown_filter.go` and the parity test
2. **`shortcodes/processor.go`**: add `case sc.Name == "excerpt": replacement = RenderExcerptShortcode(reqCtx, sc, ctx)` next to `property`.
3. **`shortcodes/parser.go`**: add `excerpt` to the opener alternation (`shortcodePattern`, line 29) and the closer alternation (line 72).
4. **`shortcodes/builtin_docs.go`**: a new `BuiltinDoc` after `property`.
   - Name `excerpt`, Syntax `[excerpt words="30"]`, `IsBlock: BlockNo`.
   - Description: "Renders a short plain-text preview of the entity's Description (or another string field, or a Meta string). Markdown, HTML, mentions and shortcodes are stripped, the text is cut at a word boundary with an ellipsis, and the result (including default=) is always HTML-escaped. **Without more=** it emits a bare value usable inside a quoted attribute; with more= a cut text is followed by a link, so more= does not belong in an attribute. A source over 16 KiB always ends with an ellipsis. It never loads data: use it inside [using] or an [mrql] block to preview a related entity."
   - Attrs as in the §3 table: `words` is Type `number` (the type every other numeric attribute uses) with Default `30`, `field` has Default `Description`, no Enums.
   - Examples:
     - `[excerpt]`
     - `[excerpt words="20" more="Read more"]`
     - `[excerpt path="recipe.intro" words="15" default="No intro"]`
     - `[mrql query="type = note ORDER BY updated DESC" limit="5"]<p><b>[property path="Name"]</b> [excerpt words="12"]</p>[/mrql]`
     - `<span title="[excerpt words='25']">[property path="Name"]</span>`
5. **`shortcodes/lint.go`**:
   - Add `"excerpt"` to `builtinBaseNames`.
   - Add a `case "excerpt":` to the semantic switch:
     - Error when both `path` and `field` are non-empty: `[excerpt] takes path or field, not both`.
     - Warning when `field` contains `.`: `[excerpt] field= names one field of this entity; use [using] to preview a related entity`.
     - Warning when `words` is set and is not an integer from 1 to 200: `[excerpt] words= must be a whole number from 1 to 200; it falls back to 30 (or 200 above it)`.
     - When `path` is set, apply `metaPathIssue(path, opts.metaProps)` with exactly the same `otherContext` guard as the `[meta]` case. That collection is already built whenever `metaProps` is non-empty, which is the only time `metaPathIssue` reports anything, so nothing else changes.
   - The inline-closer error for `[/excerpt]` comes from `IsBlock` through `Known` (`lint.go:131`/`302`), with no extra code.
6. **`shortcodes/lint_attr_context.go`**: add `case "excerpt": return attrs["more"] == ""` to `emitsBareValue`.
7. **`server/template_handlers/template_filters/markdown_filter.go`**: comment only. If the converter options change, keep `shortcodes/excerpt_handler.go`'s `excerptMarkdown` and the parity test in step with them.
8. **`server/share_server.go`**: add `[excerpt]` to the list of pure shortcodes in the `processShareTemplates` doc comment, and mark the render with `shortcodes.WithPublicSharePage` (defined in `excerpt_handler.go`) so the `more` link is left out there (§6).

**Frontend**

9. **`src/components/shortcodeCompletion.js`** line 206: add `'excerpt'` to the shortcodes whose `path` attribute gets MetaSchema completion. Attribute completion and hover docs come from `BuiltinDocs`.

**Docs and reference panels** (the consistency tests enforce the first three)

10. **`templates/createCategory.tpl`, `templates/createResourceCategory.tpl`, `templates/createNoteType.tpl`**: in each Shortcodes reference panel, add one line `<code …>[excerpt words="30" more="Read more"]</code>` with a short gloss.
11. **`docs-site/docs/features/shortcodes.md`**:
    - A table row: ``| `[excerpt]` | Inline | Plain-text, length-bounded preview of a Description, string field or Meta string |``.
    - A section `## [excerpt] -- Text Preview` after `[property]`, covering:
      - the attributes table
      - the stripping steps, including that shortcodes inside code are removed
      - the truncation rules (word limit, 12 runes per word, the 16 KiB source cap, and the ellipsis on a capped source)
      - the empty state, with `default` always escaped
      - the bare value versus the `more` link
      - that the link also renders for read-only viewers and on share pages, like `[link]`
      - that it never loads data, so use `[using]` or `[mrql]` for a related entity
      - a note-type `CustomSummary` example
    - A row in the "What works inside the block body" table under `[mrql]`.
12. **`docs-site/docs/features/custom-templates.md`**: one `CustomSummary` example with `[excerpt words="25" more="Read more"]`.
13. **`.claude/skills/mahresources-category-designer/references/shortcodes.md`**:
    - A `### [excerpt]` subsection.
    - A heuristic line: prefer `[excerpt]` over `[property path="Description"]` on cards.

The template-generation prompt and `get_shortcode_docs` pick up the doc from `BuiltinDocs()`. If a golden or prompt-size test (`e2e/fixtures/template-run-golden.json`, the prompt tests under `server/api_handlers`) pins those bytes, update it in the same change. `docs/ideas/category-templates-and-shortcodes.md` does not change.

## 11. Tests

**Unit: `shortcodes/excerpt_handler_test.go`** (new). Tests are table-driven through `Process`, so dispatch is covered.

1. Markdown is stripped: `"# Title\n\n**Bold** and [a link](http://x) and `code`"` → `Title Bold and a link and code`.
2. HTML is stripped and the output escaped. For `<script>alert(1)</script><b>hi</b> <img src=x onerror=y> Tom & <Jerry>`, assert the exact output: no `alert`, and `&amp;`/`&lt;` appear only as escaped text.
3. Shortcodes are removed and not run. `intro [mrql query='type = note'] [details]secret[/details] end`, with an executor that calls `t.Fatal`, gives `intro end`.
4. Shortcodes inside code are deleted. ``"`[property path=\"Name\"]` x"`` and a fenced block containing `[meta path="a"]` keep only the surrounding text.
5. Template comments are removed, and an `[excerpt]` inside its own Description does not recurse.
6. Mentions:
   - `see @[resource:5:Sunset photo] now` → `see Sunset photo now`.
   - `@[resource:5:a*b*_c_<d>]` → exactly `a*b*_c_&lt;d&gt;`. The name is not re-interpreted.
   - A source that already contains U+E000 gives no stray or substituted names.
7. Truncation:
   - `words="3"` on `one two three four` → `one two three…`.
   - Exactly 3 words → no ellipsis.
   - `a, b, c, d` → `a, b, c…`.
8. Rune cap: 500 CJK runes with `words="10"` → 120 runes plus `…`.
9. `words` values: `abc`, `0` and `-1` behave as 30; `500` behaves as 200.
10. Source cap:
    - A 1 MiB Description finishes, its output is at most 2400 runes plus `…`, and it ends with `…`.
    - A Description of 16 KiB minus 10 bytes of text followed by `[comment]` + 100 bytes + `[/comment]` + `tail` (the cut lands inside the comment) never contains the comment body or `tail`.
11. Scratch: with a limited meter whose scratch is nearly exhausted, the output is `""` and the meter reports `ErrRenderBudgetExceeded`.
12. `path`:
    - A string value is excerpted.
    - Number, object and missing values give `""`, or the escaped `default` when set.
    - `default="<b>x</b>"` gives `&lt;b&gt;x&lt;/b&gt;`.
13. `field`:
    - `Name` works.
    - `Owner.Name` → empty.
    - `ID` → empty.
    - `ShareToken` → empty without credential visibility, and the token with it.
14. `path` and `field` together → the output contains `shortcode-error` and `⚠ excerpt`.
15. `more`:
    - There is no link when the text is not truncated.
    - When truncated, `href` is `/note?id=N`, `/resource?id=N` or `/group?id=N`, and the `sr-only` suffix holds the escaped Name (including `<`).
    - The link is omitted for an unknown entity type or ID 0.
    - **With `ForceReadOnly: true` the link is still present and byte-identical.**
16. `UntrustedSource: true` gives byte-identical output.
17. Inside `[mrql]`: with a fake executor returning two notes, `[mrql query="type = note"][excerpt words="2"][/mrql]` shows each note's own excerpt.
18. Inside `[using]`: the excerpt reads the target entity.

**Parity: `server/template_handlers/template_filters/markdown_excerpt_parity_test.go`** (new)

- For a fixed corpus (GFM table, strikethrough, autolink, task list, nested list, raw inline HTML, hard line breaks, entities), compare two strings:
  - `shortcodes.Process` of `[excerpt words="200"]` against an entity whose Description is the input.
  - The visible text of `markDownFilter`'s output, extracted by a small tokenizer helper in the test with the same block-tag spacing and whitespace collapsing, then escaped.
- They must be equal. A change to either goldmark configuration that alters visible text fails this test.
- **The corpus holds no mentions and no shortcodes.** On a page, `render_mentions` and the shortcode pass run after `markdown2`, so neither is part of the comparison; adding one would fail for a reason unrelated to goldmark.

**Lint** (`shortcodes/lint_test.go`)

- `[excerpt words="20" more="x"]` → no issues.
- `path` and `field` together → error.
- `words="0"`, `"x"` and `"201"` → warning.
- `field="Owner.Name"` → warning.
- `<a href="[excerpt]">` → unsafe-context warning.
- `[excerpt more="x"]` in `href` → no bare-value warning.
- A `path` that is not in MetaSchema → unknown-path warning; with no MetaSchema → no warning.
- `[excerpt][/excerpt]` → the inline-closer error.
- `[exerpt]` → the near-miss suggestion.

**Parser** (`shortcodes/parser_test.go`): `[excerpt words="5"]` parses, with its attributes.

**Existing consistency tests** cover the new doc: `TestReferencePanelsCoverAllBuiltins`, `TestDocsSiteShortcodesPageCoversAllBuiltins` and `TestBuiltinDocMRQLExamplesParse`. They must fail before the docs and template changes (§10 steps 10–11) and pass after.

**API: `server/api_tests/excerpt_shortcode_scope_test.go`** (new). It reuses `setupAuthEnv`, `buildScopingFixture`, `previewDescription`/`previewHTML`, `scopedRoleBearer`, `roleBearer`, `getHTML`, `signToken` and `renderShare`. Every query is `scope="global"`, so only the viewer's own subtree confines it, and each confinement check has an admin control that does see the outside note.

- Descriptions on `nIn` (`in-subtree-text alpha beta`) and `nOut` (`out-subtree-text gamma delta`).
- Description preview, confined `RoleUser`: `[mrql …][excerpt][/mrql]` shows `nIn` only; `[using query='type = note AND name = "sf-nOut"' …][excerpt][else]none[/using]` renders `none`.
- **Read-only link:** a `RoleGuest` confined to the fixture root opens `/group?id=<child>` (category `CustomHeader` with `[mrql …][excerpt words="2" more="Read more"][/mrql]`) and `/notes` (note type `CustomSummary` with the same excerpt). Both contain `in-subtree-text alpha… <a href="/note?id=<nIn>" class="underline" aria-label="Read more: sf-nIn">Read more</a>` and no `out-subtree-text`. The guest lacks `CanWrite()`, so the page sets `ForceReadOnly`; the link survives it. (The plan's `previewDescription` route is gated at the edit capability, so a guest cannot call it.)
- `/v1/shortcodes/deferred` and `/v1/mrql?render=1` (`CustomMRQLResult` with `[excerpt]`) for the guest show `nIn` only.
- The share page renders `first…` with no link, and the same note's own page on the main server keeps the link.

**E2E: `e2e/tests/110-excerpt-shortcode.spec.ts`** and **`e2e/tests/accessibility/excerpt-shortcode-a11y.spec.ts`** (new), sharing `e2e/helpers/excerpt-fixture.ts`

- A note type with `CustomSummary: '<p data-probe="ex">[excerpt words="4" more="Read more"]</p>'`.
- A note of that type with the Description `**Alpha** beta <em>gamma</em> delta epsilon <script>window.__exc=1</script> zeta`.
- In that note's card on `/notes?noteTypeId=…`:
  - The probe has no `strong`, `em` or `script` descendants.
  - The text contains `Alpha beta gamma delta…`.
  - `window.__exc` is undefined.
  - The link has the accessible name `Read more: <note name>`, text `Read more` and `href` `/note?id=<id>`, and following it opens the note.
- A second note with a two-word Description shows its text and no link.
- A group category `CustomHeader` with `[mrql query="type = note AND id = <id>" scope="global"]<p>[excerpt words="2"]</p>[/mrql]` renders `Alpha beta…` on the group page.
- `checkA11y()` passes on the notes list and on the group page.

## 12. Success criterion

- In a note type's `CustomSummary`, `[excerpt words="25" more="Read more"]` shows a single-line, escaped, word-truncated preview of each note's text on every card surface: lists, panels and MRQL cards. (The share page renders only `CustomHeader`; there the excerpt renders without its link.)
- No Markdown syntax, HTML, mention markers, shortcode source or comment text leaks into it. Mention names appear verbatim.
- The "Read more" link appears for every viewer who can see the card, read-only principals included. It has a distinct accessible name and passes axe.
- It issues zero DB queries and does bounded work per card: at most 16 KiB processed, and a scratch charge reserved before allocation.
- A subtree-confined principal never sees an excerpt of an out-of-subtree entity.
- Lint, autocomplete, hover docs, the reference panels, docs-site and the skill reference all document it, and the goldmark parity test ties it to `markdown2`.
- All the tests above pass.


## Implementation checklist

- [x] Plan moved to `docs/plans/`, banner removed.
- [x] `mentions.ReplaceAll` with tests (`mentions/mentions_test.go`).
- [x] `shortcodes/excerpt_handler.go`: handler, pipeline, `lintExcerpt`, `WithPublicSharePage`.
- [x] `shortcodes/processor.go` dispatch, `shortcodes/parser.go` opener/closer alternations.
- [x] `shortcodes/builtin_docs.go` entry; `shortcodes/lint.go` near-miss name and `excerpt` case; `shortcodes/lint_attr_context.go` `emitsBareValue`.
- [x] `server/share_server.go` (comment and `WithPublicSharePage`); `markdown_filter.go` cross-reference comment.
- [x] `src/components/shortcodeCompletion.js` Meta-path completion for `path=`, with a vitest (`shortcodeCompletion-excerpt.test.ts`).
- [x] Reference panels in `createCategory.tpl`, `createResourceCategory.tpl`, `createNoteType.tpl`.
- [x] `docs-site/docs/features/shortcodes.md` (table row, section, `[mrql]` body row) and `custom-templates.md`.
- [x] `.claude/skills/mahresources-category-designer/references/shortcodes.md` subsection and heuristic.
- [x] Unit tests (`excerpt_handler_test.go`), lint tests (`excerpt_lint_test.go`), parity test (`markdown_excerpt_parity_test.go`).
- [x] HTTP scope tests (`server/api_tests/excerpt_shortcode_scope_test.go`).
- [x] E2E and axe specs (`e2e/tests/110-excerpt-shortcode.spec.ts`, `e2e/tests/accessibility/excerpt-shortcode-a11y.spec.ts`).

---

## Last evaluator feedback

Most API claims check out against the code: traversePropertyPath (property_handler.go:61) with the credentialFields guard, ctx.credentialsVisible (meta_handler.go:74, set in processor.go:259), ctx.rawValueAtPath (meta_handler.go:234), resolveLinkURL("self") (link_handler.go:42), stripTemplateComments to templatecomment.Strip (comment.go:13), parseWithBlocksM (parser.go:197), renderMeter.chargeScratch (render_budget.go:84, limited meters only, 64 MiB cap), shortcodeErrorMarker (markers.go:24), mentions.ParseAll with OriginalMatch/Name, the Known and IsBlock-driven inline-closer lint (lint.go:131/302), the otherContext guard around the meta case (lint.go:342-470), emitsBareValue (lint_attr_context.go:1566), builtinBaseNames (lint.go:143), the parser opener and closer alternations (parser.go:29/72), the shortcodeCompletion.js path list at line 206, and the markdown2 goldmark options. Nothing similar exists in the roadmap or the plugins; today's templates only use `truncatechars` with strip_template_comments on raw Description. The remaining problems are small, but they should be fixed before implementation.

1. Wrong helper name: `isValidFieldName` does not exist anywhere in the repo. [conditional]'s field= path (conditional_handler.go:56-71) does FieldByName plus CanInterface plus the credentialFields check inline. Either name the real check (no '.', non-empty, then traversePropertyPath on a single segment, which already rejects unexported fields and credential fields) or say a new tiny validator gets added. Also, "`[mrql]` items via `itemContext`" names no real function. Items get their context in mrql_handler.go (~lines 271/380, EntityType: item.EntityType), so cite that instead.

2. Share-page claims overreach. processShareTemplates (share_server.go:590) renders only NoteType.CustomHeader/CSS; CustomSummary never renders on a share page. So §12's "every card surface ... and the share page" is wrong for the CustomSummary example. On the share server, a `/note?id=N` href points at a route the share server does not serve: it is a dead link and exposes the internal note ID. Matching [link] is a fair consistency argument, but the plan should say so honestly. Either suppress `more` when the context is the share server (for example, document that the link is dead there, as [link]'s is), or drop the share-page promise and the matching e2e/doc wording. Do not claim it is useful there.

3. Scratch derivation is presented as precise, but its goldmark part is hand-waved. Goldmark AST nodes are ~100+ byte structs. Pathological inputs like repeated "*a*", "- a\n" or "|a|" create one node per 2-4 bytes, which is far more than 2 bytes per byte. Either call 12x a heuristic and not an upper bound, or raise the AST factor or cap the node-heavy cases. The 16 KiB cap keeps this bounded either way, so this is a wording fix. Do not add more machinery.

4. Sentinel spec: in the plan text the U+E000/U+E001 delimiters around the index appear as empty strings (`"" + strconv.Itoa(i) + ""`). Write them as explicit escapes (""+i+"") so the implementer does not guess. Also state what restore does with an unpaired opener or closer: strip it.

5. `capped` forces an ellipsis even when the visible text of the first 16 KiB is shorter than `words`. That is defensible, but say it explicitly in the docs ("a very long source always ends with …"). Test 10 covers this only implicitly.

6. Parity test caveat: the markdown2 pipeline runs render_mentions after goldmark, so keep mentions out of the parity corpus. Say so, so nobody adds them and gets a spurious failure.

7. Minor: "Credential fields (`ShareToken`, …)" should cite credential_fields.go. §6 says the output is "valid inside a quoted attribute", which is true only without `more`. The lint change to emitsBareValue covers this; mirror that condition in the BuiltinDoc description too.

Usefulness is high. Every note, resource and MRQL card author wants a clean, bounded preview. [property path="Description"] leaks Markdown, mention markers and shortcode source, and has no length bound. Dropping blocks and search highlighting was the right call. Usefulness is held just below 9 by a few edge-only gaps: no "start at heading" or "skip first paragraph" control, and no CJK-aware word counting beyond the rune cap. These are acceptable omissions.

## Review

### Round 1 review fixes

- **Attribute placement (P0): existing template-author policy is lint-only.** A runtime probe passed `[property]`, `[meta inline="true"]` and `[excerpt]` through `Process` in `onclick`, a `javascript:` URL and `srcdoc`. The existing value shortcodes emitted ordinary HTML-escaped text in all three, and HTML parsing recovered the original attribute value; there is no sink-specific runtime protection to inherit. `[excerpt]` now runs the same placement checks whether or not `more` is present; a nonblank `more` in an attribute is also a lint error because it can emit link markup. Docs limit the safe attribute use to inert quoted text. `TestLintExcerptAttributePlacementWithAndWithoutMore` pins both diagnostics.
- **Mention sentinel forgery and amplification (P1).** Escape/decoding is not enough to reserve a delimiter: HTML entities can synthesize private-use characters. A fresh cryptographic nonce per conversion scopes the identity of each marker, and restoration consumes each exact emitted marker at most once. The expansion reserved before restoration cannot exceed the total name bytes in the capped source. `TestExcerptMentionSentinelsCannotBeForgedByEntities` and `TestExcerptRestoreMentionsOnlyRestoresEmittedSentinelsOnce` cover this. This supersedes the fixed sentinels in §4.
- **Markdown labels (P1).** Bare shortcode tokens followed immediately by `(` or used as Markdown reference link/image labels are left for Markdown, including reference definitions. The distinction happens before shortcode block pairing, while real shortcode tokens in code are still removed. `TestExcerptPreservesMarkdownLabelsNamedLikeShortcodes` covers inline links and images, full/collapsed/shortcut references and a shortcode block name.
- **Inert HTML (P1).** The text tokenizer skips template content including nested templates. `TestExcerptDropsNestedHTMLTemplateContents` asserts an independently specified expected result rather than copying the production skip list.
- **Hover cards (P1).** Tooltip mode keeps the ellipsis and leaves out the `more` link. `TestExcerptShortcode_HoverCardOmitsMoreLinkFromCustomSummary` compares the real hover fragment with the normal list card for the same note.
- **Rune cut (P2).** The rune ceiling backs up over combining marks (including variation selectors), emoji modifiers and ZWJ sequences before adding the ellipsis. This is not full Unicode grapheme segmentation. `TestExcerptRuneCapBacksUpOverPartialCharacters` covers both sides of the boundary.
- **Known pre-existing limit (P1, out of scope):** Meta paths decode the whole Meta blob on first lookup for the entity context, like `[meta]`, `[property]` and `[conditional]`. The 16 KiB cap and scratch estimate apply to excerpt conversion after source lookup, not to that shared decode. The CSS selector collision is handled separately by the shared CSS-rendering fix.

Round 1 validation: focused excerpt Go tests, the full shortcode/internal/template-handler package set, and the excerpt HTTP tests passed with the `json1 fts5` tags. `npm run build` and both excerpt Playwright specs passed (5 tests). Vet reported only the pre-existing lock copy in `application_context/job_download_progress_test.go:99`; no dialect-specific, frontend-source, or CSS/template changes were made.

### Round 2 review fixes

- **Markdown label whitespace.** The masking pass trims its attribute capture like the shortcode parser. `TestExcerptPreservesMarkdownLabelsNamedLikeShortcodes` now includes inline links, images and reference definitions with trailing whitespace.
- **`more=` in attribute names.** Name positions get the same link-markup error as attribute values, in addition to the placement warning. `TestLintExcerptAttributePlacementWithAndWithoutMore` covers nonblank, blank and absent `more=` in an attribute name. This remains a lint-only rule.
- **Explicitly hidden HTML.** The tokenizer drops subtrees marked `hidden`, including nested elements, and does not treat a hidden void tag as a subtree opener. `TestExcerptDropsHiddenHTMLSubtrees` pins exact output, with CSS and `aria-hidden` as controls. The author references describe the elements and attributes dropped rather than promising browser-visible text.

Round 2 validation: all three regressions reproduced on the integration base and passed after the fixes. The shortcode, internal and template-handler suites passed with `json1 fts5`, as did the excerpt HTTP tests, shortcode docs endpoint and built-in generation prompt checks. The application built and both excerpt Playwright specs passed (5 tests). Vet reported only the pre-existing lock copy in `application_context/job_download_progress_test.go:99`.

#### Known limits

- **Deferred:** A mention name that ends in shortcode-shaped bracket text can be damaged before mention protection. For example, shortcode stripping can remove `[link]` from `@[note:5:Notes [link]`, including the mention's closing bracket.
- **Deferred (round 4):** an inline Markdown link whose label starts with a shortcode name and contains bracketed text (``[link to the `items[0]` example](/x)``) still loses the start of its label: the shortcode regex ends at the inner `]`. Multiword labels without inner brackets are kept (round 3).
- The HTML scan matches explicit tags. It does not reproduce browser tree repair (including implied end tags), CSS, or other visibility rules. Use explicit closing tags around content marked `hidden`.

### Deviations from the plan

- **`aria-label`, not an `sr-only` span**, on the `more` link. `docs/lessons.md` ("Tailwind's `.sr-only` is `position: absolute`…") records that a visually hidden span escapes a scroll container and widens the page; cards and MRQL results sit in such containers. The accessible name is the same (`Read more: <Name>`), it still starts with the visible label (WCAG 2.5.3), and axe passes on the notes list and the group page.
- **No link on the public share page** (`WithPublicSharePage`), resolving evaluator point 2 by suppression rather than by documenting a dead link. `[link]` is unchanged.
- **No link inside a `[reload]` button face** (`withinReloadFace`), and lint warns about `more=` there. A button cannot contain a link; without this the renderer would emit invalid interactive nesting. Not in the plan.
- **`mentions.ReplaceAll`** (new, in `mentions/`) instead of replacing `ParseAll` results by `OriginalMatch` text. Text replacement could substitute a valid marker's text found inside a marker `ParseAll` skipped (`@[group:0:@[note:1:x]`); `TestReplaceAll_MarkerInsideAnInvalidOne` pins it.
- **The shortcode deletion runs `parseWithBlocksM(nil, …)`, not under the render's meter.** The meter's token and attribute caps apply to template source; under a budgeted meter a Description's own text (one token with over 1000 attributes fits in 16 KiB) could otherwise end a whole budgeted render. The work is bounded by the 16 KiB cap and charged through `chargeScratch` first.
- **Stray closers are deleted too** (`[/mrql]` with no opener), not only top-level spans, so no closing tag text reaches the excerpt.
- **`ctx.EntityID == 0` omits the link explicitly**: `resolveLinkURL("self")` does not check the ID, and returned `/note?id=0`.
- **A blank `more=` (whitespace only) is no label**: no link, and lint treats the excerpt as a bare value.
- **The punctuation trim never empties the text**: `[excerpt words="1"]` on `— dash` renders `—…`, not the default.
- **`words` is documented as Type `number`**, the type every other numeric attribute in `BuiltinDocs` uses; the plan said `integer`.
- **The read-only HTTP test uses display pages, not `previewDescription`.** That route is mounted at the edit capability, so a guest gets 403. The guest test reads `/group?id=` (header) and `/notes` (cards) instead, which set `ForceReadOnly` for a principal without `CanWrite()`; deferred bodies and the MRQL render API are covered as well.
- **E2E spec location**: `e2e/tests/110-excerpt-shortcode.spec.ts`, as the lane assignment required, with a shared `e2e/helpers/excerpt-fixture.ts`.
- **The `BuiltinDoc` type comment's list of names** ("meta, datetime, … comment") was not extended, to keep this lane's edit of `builtin_docs.go` to one contiguous block while four other lanes edit the same file; add `excerpt` there when merging.

### Evaluator points

1. *`isValidFieldName` does not exist; `itemContext` names nothing.* Resolved: §5 now says there is no validator, only "no `.`" plus `traversePropertyPath` on the single segment (missing, unexported and withheld credential fields fail there); §7 cites `contextFromItem` in `mrql_handler.go`.
2. *Share-page claims overreach.* Resolved by suppression: `processShareTemplates` sets `WithPublicSharePage` and `[excerpt]` drops `more` there. §6 and §12 no longer promise the share page; docs say the link is left out there. `TestExcerptShortcode_SharePageHasNoMoreLink` checks the share page has no link while the same note's main-server page does; a unit test covers the context flag.
3. *Scratch derivation hand-waved.* Resolved: §8 and the code comment call 12× a heuristic, not a bound, name the node-heavy inputs, and say the 16 KiB cap is what bounds the work. No machinery added.
4. *Sentinel delimiters invisible; unpaired restore unspecified.* Resolved in §4 by spelling out the delimiters. Round 1 superseded the fixed marker; `TestExcerptRestoreMentionsOnlyRestoresEmittedSentinelsOnce` pins how unknown or malformed markers are handled now.
5. *`capped` forces an ellipsis silently.* Resolved: the BuiltinDoc, the docs-site Truncation section and the skill reference say a source over 16 KiB always ends with `…`. `TestExcerptSourceCap` asserts it directly, including a capped source whose visible text is two words.
6. *Parity corpus must exclude mentions.* Resolved: §11 and the test's comment say the corpus holds no mentions and no shortcodes, and why.
7. *Cite `credential_fields.go`; the attribute claim needs "without more".* §3 cites `credentialFields` in `shortcodes/credential_fields.go`. Round 1 further narrows the attribute guidance and checks placement even when `more` is present.

### Not verified

- **Postgres**: no Postgres-specific test was added. `[excerpt]` issues no query; the entities it reads come from the existing scoped executors, whose Postgres behaviour other suites cover. `server/api_tests` under `--tags 'json1 fts5 postgres'` still runs these tests on SQLite (`SetupTestEnv`), so running them with the tag (done, passing) shows only that they compile and pass there.
- **The full E2E suite** was not run (shared machine); the new specs plus the nearest existing shortcode, authoring-a11y and note-sharing specs were.
- **`go vet ./...`** reports one pre-existing finding in `application_context/job_download_progress_test.go` (a lock copy), a file this branch does not touch; every package this branch changes vets clean.
