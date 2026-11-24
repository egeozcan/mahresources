<!-- Shortcode idea that did not make the first implementation round. -->
> **Status:** idea, not implemented. The plan went through 2 plan/evaluate round(s).
> Last evaluation: consistency 7.6/10, usefulness 8/10, average 7.80/10. The bar was 8.5.
>
> **Pitch:** Renders an accessible button that copies a value to the clipboard: [copy path="meta.sku" label="Copy SKU"], [copy value="[link]" label="Copy link"], or a block form that copies its rendered inner text. It announces 'Copied' through an aria-live status, falls back to a selectable read-only input when the Clipboard API is unavailable, and never copies raw HTML. Contact cards, credentials metadata, file paths and IDs become one click to reuse, which is a common need with no safe built-in answer today.

# Plan: `[copy]`, a copy-to-clipboard control (revision 3)

The plan is saved as `docs/plans/2026-11-24-copy-shortcode.md`. Run `./docs/plans/generate-index.sh` after writing it.

## 0. Changes from revision 2 (one per evaluator point)

1. **`[each]` per-item copy works now, and it is tested.** `markItems` in `shortcodes/each_handler.go` gains one narrow change. It also marks the top-level `[item]` tokens of a top-level `[copy]` block body, and does the same for `[copy]` blocks nested inside that body. This is safe because a `[copy]` body is rendered in the same pass with `processWithDepth` and is never sealed into a deferred token. That is exactly the classification the existing `markItems` comment asks for. Every other block keeps the conservative rule (section 6).
2. **A `[copy]` inside `[link]` is refused at render time, not only by lint.** There are two checks, mirroring how `[reload]` protects its face:
   - A context key set by the `[link]` block catches anything rendered in the same pass, such as `[partial]`.
   - A source scan in `[link]` catches a `[copy]` in a sealed `[lazy]`/`[details]` body.
   - Section 5 covers app-rendered slots (none of them is wrapped in an `<a>`) and hand-written anchors.
3. **"Copy link" copies an absolute URL.** A new inline `to=` attribute has the same enum as `[link to=]`. The client resolves it against `document.baseURI`. The docs say that `[copy][link][/copy]` copies the relative path as displayed.
4. **The inline `path=`/`field=` form also displays the value.** It now means "show this value with a copy button". With JavaScript off it degrades to readable text. It copies the exact stored value, not the whitespace-collapsed visible text. The inline `to=` form shows only the button (section 3).
5. **The pairing of the optional closing tag is specified.** The render error and the lint error both point at the stray `[/copy]` as well as the opener (section 4).
6. **E2E assumptions are stated.** The clipboard permissions are Chromium-only, and the project config is Desktop Chrome only. The confined-guest test also checks that B's id and name are absent from every `data-copy-value`.
7. No change. The verified helpers are reused as before.
8. Per-item lists now work in `[each]` through the block form plus `[item]`, in `[mrql]` results through `CustomMRQLResult` with the inline form, and in list cards through `CustomSummary`/`CustomCell`. The inline form now adds visible value, exact copy and absolute URLs over the block form.

## 1. Motivation and user value

Templates often show values a reader wants to paste somewhere else, such as an SKU, an account number, a file path, a resource ID, a contact's e-mail or a page URL. Selecting long tokens by hand is error-prone. Inside cards it is worse, because a drag-select tends to start a link click.

Nothing built in covers this today:
- `updateClipboard` in `src/index.js` cannot be reached from templates without inline `onclick` JavaScript.
- It reports neither success nor failure.
- Its last fallback is `prompt()`.

`[copy]` is one declarative control with these properties:
- **Accessible.** It is a real `<button>`. Its visible label is also its accessible name. Success is announced politely, and the fallback field has a label.
- **Plain text only.** It copies a resolved stored value or rendered visible text, never HTML.
- **Works over HTTP.** It works on the private-network HTTP deployments this app targets, where `navigator.clipboard` is missing because it needs a secure context.

## 2. Syntax and attributes

```
[copy path="sku" label="Copy SKU"]          shows the Meta value and a button that copies it exactly
[copy field="ID" label="Copy ID"]           same, for an entity property dot-path ([property path=] semantics)
[copy to="self" label="Copy link"]          a button only; copies the absolute URL of the page [link to=] would target
[copy label="Copy address"][meta path="street" inline="true"], [meta path="city" inline="true"][/copy]
[each path="emails"][copy label="Copy e-mail"][item][/copy][/each]
```

| Attr | Type | Form | Default | Meaning |
|---|---|---|---|---|
| `path` | string | inline; exactly one of `path`/`field`/`to` | none | Meta dot-path, read with the `[meta]` walker (`navigateJSONValue` over `ctx.decodeMeta()`, memoized per render). |
| `field` | string | inline; exactly one of `path`/`field`/`to` | none | Entity property dot-path, read with `traversePropertyPath(ctx.Entity, path, ctx.credentialsVisible)` and formatted with `formatPropertyValue(v, "", "")`, as `[property path=]` does. Credential fields fail closed. |
| `to` | enum `self` \| `owner` \| `root` \| `category` | inline; exactly one of `path`/`field`/`to` | none | Link target, resolved with the existing `resolveLinkURL(to, ctx)`. |
| `label` | string | both | `Copy` | Button text, which is also the accessible name. It is trimmed, and an empty value falls back to `Copy`. It is HTML-escaped. |

`IsBlock: BlockOptional`. There are no other attributes.

The pitch's `value="[link]"` is dropped for three reasons:
- The parser does not expand shortcodes inside attribute values, so it would copy the literal text `[link]`.
- `to=` covers the link case.
- The block form covers composed text.

`format=` is left out. For a formatted value, use the block form: `[copy][meta path="d" inline="true" format="date"][/copy]`. The naming follows `[conditional]`, which uses `path=` for Meta and `field=` for entity properties.

**Choosing a form:**
- Use inline `path`/`field` to show a stored value and copy it byte for byte. Leading spaces, newlines and tabs survive, which `innerText` would collapse.
- Use inline `to` for a shareable absolute URL.
- Use the block form to copy composed or formatted text exactly as displayed, and for `[each]` items.

## 3. Rendering

### Inline `path=` / `field=`
```html
<copy-shortcode data-copy-value="ESC"><span class="copy-shortcode-text">ESC</span> <button type="button" class="copy-shortcode-button">Copy SKU</button></copy-shortcode>
```
`ESC` is the same `html.EscapeString` value in both places.

Value text:
- A string is copied as stored.
- A number goes through `formatJSONScalar(v, "", "")`, so `1234567` stays `1234567`.
- A boolean is `true` or `false`.
- `field=` produces the text `[property]` prints without `format`.

There is no `raw` option, so `UntrustedSource` changes nothing.

### Inline `to=`
```html
<copy-shortcode data-copy-value="/group?id=5" data-copy-url=""><button type="button" class="copy-shortcode-button">Copy link</button></copy-shortcode>
```
Only the button is shown. A relative path printed next to "Copy link" would be noise, and visible link text is already what `[link]` is for. The server does not know the public origin (reverse proxies, private-network host names), so the client resolves the URL (section 7).

### Block form
The body is rendered with `processWithDepth(markCopyBody(reqCtx), sc.InnerContent, ctx, renderer, executor, depth+1)`:
```html
<copy-shortcode><span class="copy-shortcode-text">RENDERED BODY</span> <button type="button" class="copy-shortcode-button">Copy address</button></copy-shortcode>
```
`markCopyBody` exists only to let tests assert pass-through. It is not otherwise needed; drop it if unused (see section 9).

### Rules for every form
- **Accessible name.** It comes from the visible text. There is no `aria-label`, so the name matches the label (WCAG 2.5.3). There is no icon.
- **Host element.** `<copy-shortcode>` is not a registered custom element. It is a styling hook and the target of a delegated event listener. CSS: `copy-shortcode { display: inline; }`.
- **Button styling** follows `.reload-shortcode-button`:
  - Inherited font, stone/amber colours, a 1px border and uniformly rounded corners.
  - `:focus-visible { outline: 2px solid #b45309; outline-offset: 2px }`. Never `outline-none` (`internal/arch/forced_colors_focus_test.go`).
  - Contrast of at least 4.5:1 for text and 3:1 for the ring.
- **Feedback** is added by the client:
  - **On success,** one `<span class="copy-shortcode-feedback" aria-hidden="true">Copied</span>` is inserted after the button and removed after 2000 ms. A click again restarts the timer. `window.mahAnnounce('Copied to clipboard')` makes the polite announcement. The button's text never changes, which avoids a double announcement.
  - **When both copy mechanisms fail,** a field is inserted after the button: `<input type="text" readonly class="copy-shortcode-fallback" aria-label="<label>: press Ctrl+C or Command+C to copy">`.
    - The client sets `.value` as a property, never as HTML.
    - The field is focused and its text selected, and `mahAnnounce('Copy unavailable. The text is selected; press Control+C or Command+C to copy.')` is announced.
    - The field is removed on `blur`. A second click re-focuses and re-selects the existing field.
- **Without JavaScript,** the button is inert, as `[reload]` is. The `path`/`field` and block forms still show their text.

## 4. Empty, missing, invalid and nesting cases

| Case | Output |
|---|---|
| Inline with none of `path`/`field`/`to` | `shortcodeErrorMarker("copy", "[copy] needs path=, field= or to=, or a body to copy")` |
| Inline with two or more of them | `shortcodeErrorMarker("copy", "[copy] takes one of path=, field= or to=")` |
| Block form carrying `path`/`field`/`to` | `shortcodeErrorMarker("copy", "[copy path=…] is inline and was paired with a later [/copy]; remove that [/copy] or the attribute")`. The body is not rendered. |
| `to=` outside the enum | `shortcodeErrorMarker("copy", "[copy] to= must be self, owner, root or category")` |
| `to=` that resolves to nothing (`resolveLinkURL` not ok: an unset category, or the unresolved scope sentinel) | `""` |
| Value missing, null or whitespace-only | `""`. A button that copies nothing would be a trap. |
| Meta object or array | `shortcodeComment("copy: value is not a scalar")`, with no button |
| `field=` names a credential field (`ShareToken`, `TokenHash`, `CsrfToken`, `PasswordHash`) without visibility | `""`: `traversePropertyPath` returns not-ok. |
| `ctx.Entity == nil` with `field=` | `""` |
| Block body renders nothing (`!rendersAnything(inner)`) | The inner output is passed through unchanged (comments only), with no button. |
| Inside a `[reload]` face (`withinReloadFace(reqCtx)`) | `shortcodeErrorMarker("copy", "[copy] cannot be used inside a [reload] button face")` |
| Inside a `[link]` body (`withinLinkBody(reqCtx)`) | `shortcodeErrorMarker("copy", "[copy] cannot be used inside a [link] block: a link may not contain a button")` |
| A `[copy]` nested in a `[copy]` body | Allowed. The inner button sits inside the outer text span, not inside the outer button, so the markup is valid. |

**How the optional closer pairs (`parser.go`):**
- A closer takes the nearest earlier unmatched opener of the same name.
- `[copy path="a"] … [copy label="b"]x[/copy]` pairs the closer with `b`, so `a` stays inline and is correct.
- A stray `[/copy]` whose nearest unmatched opener is an inline `[copy path=…]` turns that opener into a block. That gives the third row of the table above, and its message names the closer as the cause.
- The linter reports the same problem at both ends (section 8, lint). The author sees the error on the `[/copy]` that caused it, not only far away on the opener.

## 5. Interactive nesting (`<a>` and `<button>`)

**`[reload]`:**
- In the render, this is refused through the existing `withinReloadFace`.
- `[reload]` already refuses `[lazy]`/`[details]` in its face, so there is no deferred path around the check.

**`[link]` has two checks, both in `link_handler.go`:**
1. **Context key.** Add `type linkBodyKey struct{}`, `markLinkBody(ctx)` and `withinLinkBody(ctx)`, modelled on `markReloadFace`/`withinReloadFace`. The `[link]` block renders its body with `markLinkBody(reqCtx)` whether or not the target resolves. Lint cannot know whether a target resolves, and this keeps lint and render in agreement. Anything rendered in the same pass, including `[partial]` output, `[each]` bodies and `[mrql]` result templates, sees the key.
2. **Source scan.** A `[copy]` inside a `[lazy]`/`[details]` body within `[link]` is sealed and rendered by a later request that has no link context. So before it expands its body, the `[link]` block checks `containsShortcodeOpenerM(meterFrom(reqCtx), sc.InnerContent, "copy")`. When that finds an opener, the link returns `shortcodeErrorMarker("link", "[link] cannot contain [copy]: a link may not contain a button")`.
   - This is the same "check the source before expanding" pattern `[reload]` uses.
   - `containsReloadShortcodeM` is generalized into `containsShortcodeOpenerM(m, source, name)`: the same `strings.Contains` fast path, then the token scan. `containsReloadShortcodeM` keeps its name as a one-line wrapper, so callers do not change.
   - **Residual case:** a `[partial]` inside a deferred `[lazy]`/`[details]` inside `[link]` whose partial contains `[copy]`. No source is visible to scan there. The worst outcome is invalid markup (a button inside a link), not exposed data. This is documented.

**App-rendered slots.**
- No app slot is rendered inside an `<a>`. I checked `templates/partials/group.tpl`, `resource.tpl`, `note.tpl`, `mrqlList{group,resource,note}.tpl` (which wrap `CustomMRQLResult` in `<article><div>`), `hovercard.tpl`, and `listResourcesDetails.tpl` (`CustomCell` goes in a `<td>`).
- The card selection toggle is bound only to the checkbox (`x-bind="events"`), so clicking a copy button in a selectable card does not toggle selection.

**Hand-written `<a>` or `<button>` in the author's own markup** is not detected by the renderer or by lint. The lint markup scan reasons about attribute and value placement, not interactive ancestry, and adding that is out of scope. The docs say not to do it.

## 6. The `markItems` change (`each_handler.go`)

**In the marking loop,** when `sc.Name == "copy" && sc.IsBlock`:
1. Compute `innerStart := sc.End - len("[/copy]") - len(sc.InnerContent)` and `innerEnd := innerStart + len(sc.InnerContent)`. The closer is always exactly `[/copy]` (`closingTagPattern`).
2. Write `branch[lastEnd:innerStart]`.
3. Recursively mark `sc.InnerContent` with the same item counter and slice. This only reaches the top level of that body and, again, nested `[copy]` blocks.
4. Write the marked inner, set `lastEnd = innerEnd`, and continue. The `[/copy]` is copied as part of the following text.

**Budget.** The size pre-count walks the same recursion, so `admitParsed` and `chargeScratch` still reserve before anything is built, and each token costs one `m.tick()`.

**What is not marked:**
- An `[item]` inside a `[conditional]`, `[lazy]`, `[details]`, `[reload]`, `[using]`, `[mrql]`, `[link]` or a nested `[each]` inside the `[copy]` body. The conservative rule stands for every block other than `[copy]`.
- The comment on `markItems` is updated to name `[copy]` as the one block classified as "rendered in this pass, never sealed". `RenderCopyShortcode` carries a comment saying that it must never defer its body, and why.

**Splicing.** `[copy]`'s body passes the sentinel through as plain text inside `.copy-shortcode-text`. The existing `spliceLazy` replaces it with the element's escaped value, and the client reads it with `innerText`.

**An empty element.** A sentinel counts as content for `rendersAnything`, so the button renders even when an element is the empty string. In that case the client announces "Nothing to copy" (section 7). Detecting this on the server would need post-splice knowledge, which the design does not add.

## 7. Client (`src/webcomponents/copyShortcode.js`, imported in `src/main.js` next to `reloadShortcode.js`)

**Listener.** One `document.addEventListener('click', …)` matches `event.target.closest('copy-shortcode > .copy-shortcode-button')`. Being delegated, it covers content swapped in by `[lazy]`, `[details]`, `[autorefresh]` and `[reload]`, by Alpine morph, by the lightbox and by hover cards, with nothing to initialize per element.

**Text to copy:**
1. If the host has `data-copy-value` and `data-copy-url`, use `new URL(value, document.baseURI).href`.
2. Otherwise, if it has `data-copy-value`, use that string unchanged.
3. Otherwise, set `hidden` on every `.copy-shortcode-button`, `.copy-shortcode-feedback` and `.copy-shortcode-fallback` inside `:scope > .copy-shortcode-text`. Read `innerText.trim()`, then restore those elements in a `finally`. `innerText` is the visible text without markup. Nested inline `[copy]` values stay in, because they are displayed.
4. If the result is empty, call `mahAnnounce('Nothing to copy')` and stop.

`const DEFAULT_LABEL = 'Copy'` mirrors `defaultCopyLabel`, with a comment saying so.

**`copyText(text) → Promise<boolean>`:**
1. If `window.isSecureContext && navigator.clipboard?.writeText` exists, await it and return `true`.
2. Otherwise, or if step 1 rejected, use the fallback:
   - Create an off-screen `<textarea readonly>` with `.value = text`, then `select()` and `document.execCommand('copy')`.
   - Remove the textarea, return focus to the button, and return the result.
3. On `false` or an exception, show the fallback field (section 3).

`updateClipboard` is left unchanged.

## 8. Files to change

- **`shortcodes/copy_handler.go` (new).** It contains `RenderCopyShortcode(reqCtx, sc, ctx, renderer, executor, depth)` (sections 3 and 4), `const defaultCopyLabel = "Copy"`, and the enum check for `to`, which reuses `resolveLinkURL`.
- **`shortcodes/parser.go`.** Add `copy` to `shortcodePattern` and `closingTagPattern`, after `autorefresh`.
- **`shortcodes/processor.go`.** Add `case sc.Name == "copy": replacement = RenderCopyShortcode(...)`.
- **`shortcodes/each_handler.go`.** The `markItems` recursion described in section 6.
- **`shortcodes/link_handler.go`.** `linkBodyKey`, `markLinkBody` and `withinLinkBody`. The block body renders under `markLinkBody`, after the source-scan refusal.
- **`shortcodes/reload_handler.go`.** `containsShortcodeOpenerM(m, source, name)`, with `containsReloadShortcodeM` as a wrapper.
- **`shortcodes/builtin_docs.go`.**
  - A `BuiltinDoc` with Name `copy`, Syntax `[copy path="..." label="..."] or [copy label="..."]inner[/copy]` and `IsBlock: BlockOptional`.
  - Attrs `path`, `field`, `to` (enum `self`/`owner`/`root`/`category`, no default) and `label` (default `Copy`).
  - Examples: inline path, inline field, `to="self"`, block, and `[each]` with `[item]`.
  - Add `copy` to the doc comment's list.
- **`shortcodes/lint.go`:**
  - Add `"copy"` to `builtinBaseNames`.
  - Compute `linkSpans := namedBlockSpans(tokens, "link")`.
  - Build `copyCloser`, an opener-index to closer-index map produced by the same stack pass as `namedBlockSpans`.
  - Add `case "copy":`
    - Error inside `reloadSpans` or `linkSpans`, with the renderer's messages. A `[copy]` inside `[lazy]` inside `[link]` is found by the same span test.
    - Error when an inline token (`!tk.matched`) has none, or more than one, of `path`/`field`/`to`.
    - Error when `to` is outside the enum. This may already come from the `Enum` attribute check; if so, do not add a duplicate.
    - Error when a matched opener carries `path`/`field`/`to`. It is reported twice: on the opener, with "[copy path=…] is inline but is paired with a later [/copy]", and on that closer's token, with "this [/copy] closes the inline [copy path=…] on line N; remove it".
  - Not added to `emitsBareValue`.
- **`src/webcomponents/copyShortcode.js` (new)**, plus the import in `src/main.js`.
- **`public/index.css`.** Styles for `copy-shortcode`, `.copy-shortcode-text`, `.copy-shortcode-button` (base, hover and `:focus-visible`), `.copy-shortcode-feedback` and `.copy-shortcode-fallback`.
- **Reference panels.** A `[copy]` entry in `templates/createCategory.tpl`, `createResourceCategory.tpl` and `createNoteType.tpl`, using the `[reload]` pattern. `TestReferencePanelsCoverAllBuiltins` enforces this.
- **`.claude/skills/mahresources-category-designer/references/shortcodes.md`.** A `[copy]` section.

## 9. Scoping, authorization and performance

**No queries.**
- `[copy]` reads only the context entity that the page, the `[mrql]` result or the `[using]` block already loaded and authorized.
- It never touches `QueryExecutor`.
- `to=` uses `resolveLinkURL`, which reads ids already on the entity and never emits the unresolved-scope sentinel.
- Group-subtree confinement therefore holds by construction. Shortcodes in a block body keep their own scoping.

**Credentials fail closed** through `traversePropertyPath(…, ctx.credentialsVisible)`. `path=` exposes only Meta that `[meta]` already shows.

**Share pages.** `templates/shared/base.tpl` loads `main.js`, and copying is read-only, so `ForceReadOnly` is irrelevant.

**Cost:**
- Each occurrence is O(1): one memoized Meta lookup or one reflection walk.
- The inline value is emitted twice (attribute and text). That is about 130 bytes plus 2× the value per card, all charged to the render budget through `chargeOut`, so a huge Meta string cannot exceed the page cap.
- The `markItems` recursion costs one tick per token and has its size reserved up front.
- The client is one document listener with no per-element work at load, whatever the card count.

`markCopyBody` (mentioned in section 3) is not added. The body renders with the plain `reqCtx`, because nothing needs to detect being inside a copy body.

## 10. Tests

### Go (`shortcodes/copy_handler_test.go`, through `Process`)
- **Inline `path=`:**
  - A string, the float `1234567` (expect `1234567`), a boolean and a nested path.
  - Exact markup: an escaped `data-copy-value` identical to the `.copy-shortcode-text` content, `type="button"` and the label.
- **Escaping.** A value with `"<>&'` is escaped in both places, and the output is identical with `UntrustedSource`.
- **Empty values.** Missing, null and whitespace-only values give `""`. An object or array gives the comment and no `<button`.
- **`field=`.** `field="ID"`, `field="Owner.Name"`, and `field="ShareToken"` on a Note (`""` without `WithCredentialFieldsVisible(true)`, present with it).
- **`to=`:**
  - `to="self"` on a group gives `data-copy-value="/group?id=N"`, `data-copy-url` and no `.copy-shortcode-text`.
  - `to="category"` with no category gives `""`.
  - `to="bogus"` gives the marker.
- **Error markers.** Every marker in the section 4 table, including `[copy path="a"] x [/copy]`. `[copy path="a"] [copy]b[/copy]` renders an inline control for `a` and a block for `b`.
- **Labels.** The default, the empty-label fallback and an escaped label.
- **Block form.** `[property path="Name"]` is expanded inside the span with no `data-copy-value`. A body of only comments gives no button.
- **Nesting:**
  - `[reload][copy path="x"][/reload]` gives the marker, and no `<button` appears inside the reload button.
  - `[link][copy path="x"][/link]` gives the `[link]` marker from the source scan.
  - A `[partial]` whose body is `[copy path="x"]`, inside `[link]`, gives the `[copy]` marker from the context key.
  - `[link][lazy][copy path="x"][/lazy][/link]`, rendered with a deferred signer, gives the `[link]` marker and no sealed token.
  - A `[link]` with no copy renders as before. The existing `link_handler_test.go` tests stay green.
- **In `each_handler_test.go`:**
  - `emails: ["a@x", "b@y"]` with `[each path="emails"][copy label="Copy e-mail"][item][/copy][/each]` gives two controls whose text spans contain `a@x` and `b@y`.
  - `[copy][item path="sku"][/copy]` over objects works the same way.
  - `[copy][conditional …][item][/conditional][/copy]` still renders the item empty (the rule is unchanged).
  - `[copy][lazy][item][/lazy][/copy]` puts no sentinel bytes in the output or in the sealed token.
  - An element value containing `[property path="Name"]` is spliced as literal text, not executed. This is the existing injection guard, now exercised through `[copy]`.
  - A budget test: the render-budget stop still precedes construction (extend the existing `markItems` budget test).
- **Parser.** `[copy …]` and `[/copy]` are recognized, and `[copyx]` is not.
- **Lint:**
  - Each new error.
  - The double report on the opener and the stray `[/copy]`, with the closer's offsets asserted.
  - An unknown attribute (`value=`) warns.
  - `[coppy]` suggests `[copy]`.
  - The valid forms, including `[each]…[copy][item][/copy]…`, produce no issues.
- **Existing consistency tests** cover `copy` once it is in `BuiltinDocs`: the docs-site table, the reference panels, prompt completeness and the MRQL examples.

### Vitest (`src/webcomponents/copyShortcode.test.ts`, with stubbed globals as in `reloadName.test.ts`)
- The Clipboard path writes `data-copy-value` exactly (including a value with leading spaces and a newline) and announces once. The feedback span appears and is removed after the timer.
- `data-copy-url` resolves to an absolute URL against a stubbed `document.baseURI`.
- With no `navigator.clipboard`, the `execCommand` fallback runs and focus returns to the button.
- When both fail, the readonly field appears with its label, focused and selected. A second click does not duplicate it, and blur removes it.
- Block form: nested buttons and feedback are excluded from the text and restored afterwards.
- Empty text announces "Nothing to copy" and makes no write.

### E2E (`e2e/tests/shortcodes/copy-shortcode.spec.ts`)
**Assumption:** `clipboard-read`/`clipboard-write` are Chromium permissions. That works because every project in `e2e/playwright.config.ts` is Desktop Chrome. The spec calls `test.skip(browserName !== 'chromium')` so that adding another engine later fails loudly rather than silently. It runs on localhost, which is a secure context.

- **Setup.** A category has `CustomHeader: [copy path="sku" label="Copy SKU"] [copy to="self" label="Copy link"] [each path="emails"][copy label="Copy e-mail"][item][/copy][/each]`.
- **Copy SKU.** Click `getByRole('button', { name: 'Copy SKU' })`. `navigator.clipboard.readText()` equals the SKU, and the polite live region contains "Copied to clipboard". The SKU is also visible as text.
- **Copy link.** The clipboard holds `${baseURL}/group?id=<id>`, an absolute URL.
- **Copy e-mail.** The second "Copy e-mail" button copies the second address.
- **Keyboard.** Tab to "Copy SKU" and press Enter; the copy succeeds.
- **Confined guest.**
  - A guest confined to subtree A views a group in A whose template is `[using type="group" id="<B id>"][copy field="Name" label="Copy B"][copy to="self" label="Copy B link"][/using]`, where B is outside the subtree.
  - No such buttons exist.
  - B's name does not appear in the page.
  - No `[data-copy-value]` attribute contains B's name or `id=<B id>`.
- **Fallback.**
  - `addInitScript` deletes `navigator.clipboard` and stubs `document.execCommand` to return `false`.
  - The click shows a focused readonly textbox whose `selectionStart`/`selectionEnd` cover the value.

### Accessibility (`e2e/tests/accessibility/copy-shortcode-a11y.spec.ts`)
- An axe scan in the normal, "Copied" and fallback states.
- Every copy button's accessible name equals its visible text.
- No `copy-shortcode button` has an `a` or `button` ancestor.

## 11. Docs

**`docs-site/docs/features/shortcodes.md`:**
- A table row: ``| `[copy]` | Inline or block | Show and copy a value, a link, or the body's text |``.
- A section `## [copy] -- Copy to clipboard` that covers:
  - The attributes, and which form to choose (section 2).
  - What is copied: the exact stored value, the absolute URL for `to=`, or the body's visible text.
  - That `[copy][link][/copy]` copies the relative path as displayed; use `to=` for a shareable URL.
  - Use in `[each]` with `[item]`. `[item]` inside `[copy]` works, but not inside another block within `[copy]`.
  - Behaviour for empty and invalid input.
  - The HTTP fallback, and that the button needs JavaScript.
  - A11y notes:
    - Give each button a distinct `label` when several appear on one page.
    - Never put `[copy]` inside `[link]` or `[reload]` (refused), or inside a hand-written `<a>`/`<button>` (not detected).
    - Buttons in `CustomHoverCard` are not reachable by keyboard.

**`docs-site/docs/features/custom-templates.md`:** add `[copy]` to the shortcode list near line 507.

## 12. Tradeoffs and assumptions

- **Existing text may change meaning.** Existing prose containing `[copy …]` or `[/copy]` becomes a shortcode, and a bare `[copy]` renders the error marker. The linter flags it, the same tradeoff `[using]` and `[item]` made.
- **Duplicated inline value.** The inline value is duplicated in the attribute and in the text. That buys an exact copy plus a readable no-JS fallback, at about 2× the value's bytes, all charged to the budget.
- **"Limit to the user" means the render context.** Copying is a read of data the page already renders, so there is no role check.

## 13. Success criteria

- A keyboard-only user on a group page can Tab to "Copy SKU" and press Enter. The clipboard then holds the plain SKU, and the live region says "Copied to clipboard" once.
- "Copy link" yields an absolute URL.
- In `[each path="emails"]`, each button copies its own address.
- On an HTTP origin without the Clipboard API, the copy succeeds through `execCommand`. If that also fails, a focused, labelled, selected readonly field appears.
- No form ever puts markup on the clipboard.
- No `<button>` is ever rendered inside a `[link]` or `[reload]`, directly, through `[partial]`, or through a deferred body.
- A subtree-confined guest never gets a copy control, a value or a `data-copy-value` for an entity outside their subtree.
- `field="ShareToken"` renders nothing unless credentials are visible.
- The control issues zero DB queries.
- These pass: `go test --tags 'json1 fts5' ./...`, Vitest, `cd e2e && npm run test:with-server:all`, and the new a11y spec.

---

## Last evaluator feedback

1. Parser prefix collision (blocking). In `shortcodes/parser.go`, `shortcodePattern` is `\[(name|...)\s*((?:...|[^\]])*)\]`. Because `\s*` allows zero whitespace, any bracket text that starts with a shortcode name parses as that shortcode. I checked this with a throwaway test: `Parse("[items] [metadata] [linked here]")` returns item, meta and link. So the planned parser test "`[copyx]` is not recognized" would fail. Worse, ordinary prose like `[copyright 2024]`, `[copyedit]` or `[copy of contract]` would turn into a bare inline `[copy]` and render the error marker. For the word "copy" this is far more likely than for meta/link/item, so the section 12 tradeoff note understates it. Either:
   - add a word-boundary guard just for `copy` (and say whether you deliberately keep the old behaviour for the other names), or
   - make a bare `[copy]` with no attributes and no closer render nothing or a comment instead of a visible error marker, and lint it as a warning.
   Then fix the test claim to match.

2. Lint has no generic Enum check. `grep Enum shortcodes/lint*.go` finds nothing; `Enum` is only doc metadata in `builtin_docs.go`. So "this may already come from the Enum attribute check; if so, do not add a duplicate" is decided: the plan must add the `to=` enum check in `case "copy"`. Say so, and say which message it uses.

3. `markCopyBody` contradicts itself. Section 3 renders the body with `markCopyBody(reqCtx)` and says to "drop it if unused". Section 9 says it is not added and the body uses plain `reqCtx`. Delete it from section 3 so the implementer doesn't have to guess.

4. Make the `markItems` recursion concrete:
   - State that the recursive call works on `sc.InnerContent` with offsets relative to the inner string, not the branch.
   - State that the size pre-count loop recurses the same way (the current loop only counts top-level items).
   - Write `innerStart` from the parser's actual block fields if the parser exposes the closer start, rather than recomputing it from `len("[/copy]")`.
   - Name the existing budget test you will extend.

5. The `[link]` source scan refuses the whole link because of one `[copy]` anywhere in its body. That is stricter than the existing handling: `[reload]` inside `[link]` is a button inside a link too, and nothing refuses it today. Say whether `[reload]`/`[autorefresh]` inside `[link]` should get the same treatment for consistency, or explicitly scope it out. Without that, the general principle "no button inside a link" is only half enforced.

6. `field=` versus `[property path=]`. `[conditional]` uses `field=` for entity properties, so the naming has precedent. But `[copy path=]` means Meta while `[property path=]` means an entity property, so authors moving from `[property]` will make mistakes. Add a lint near-miss hint: `[copy]` with a `path` value that matches a known entity property name (`Name`, `ID`) gets a warning suggesting `field=`.

7. Minor:
   - The inline `to=` form renders a button with no visible context. Several "Copy link" buttons on one page are only told apart by `label`, which the docs mention; fine.
   - The 2000 ms feedback span is `aria-hidden`, which is good, but confirm it does not shift layout inside dense list cards.
   - The duplicated value (attribute plus text) doubles the bytes for large Meta strings in `[each]` over thousands of items. It is charged to the budget, but it is worth documenting as a reason to prefer the block form in big lists.

Usefulness: copy-to-clipboard is a frequent, real need. Today it can only be done with inline `onclick` and `updateClipboard` (no feedback, falls back to `prompt()`), so this is not a duplicate. The `to=` absolute URL and the exact stored-value copy add real value. The design is somewhat heavy (three forms, `markItems` surgery, `[link]` changes) for a small control, and that keeps usefulness at about 8 rather than higher.
