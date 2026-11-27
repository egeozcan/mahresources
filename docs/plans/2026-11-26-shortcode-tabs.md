# Plan: `[tabs]` / `[tab]`, an accessible tabbed layout shortcode (revision 3)

Slug: `tabs`. Implemented on branch `feat/shortcode-tabs`; the implementation record is the Review section at the end.

A layout block, [tabs][tab label="Overview"]...[/tab][tab label="Files" lazy="true"]...[/tab][/tabs], that renders the WAI-ARIA tabs pattern: role=tablist/tab/tabpanel, arrow-key and Home/End navigation, and aria-selected and aria-controls wiring. A panel renders with the page unless it says lazy="true"; then it reuses the [lazy] deferred-fragment path, so heavy [mrql] or gallery content costs nothing until it's opened. Category authors can fit rich headers and sidebars into a compact space instead of one long scroll.

## 0. Changes since revision 2

Each evaluator point and how this version answers it:

1. **Nested `[tabs]` listener leakage (blocking): fixed.**
   - The delegated `keydown`, `click` and `focusout` handlers act only when the event's tab button is a direct child of this host's own tab list: `tab.parentElement === this._list()`. An event from an inner tab set bubbles to the outer host, fails that check, and is ignored. `stopPropagation` is not used, so page-level listeners still see the event.
   - `_apply()` now touches only this host's own panels and their direct-child `.tabs-fallback-label` (`:scope > .tabs-fallback-label`). An outer set never hides an inner set's labels.
   - The FOCUSABLE check deliberately counts an inner set's tab buttons. A panel that holds a tab list contains focusable content, so it gets no `tabindex`.
   - A new Vitest case dispatches keys and clicks on inner tabs and asserts that only the inner set changes (§9.3).
2. **Morph re-fetching hidden lazy panels that were already loaded: fixed, with a small opt-in change to `deferredShortcodes.js`.** The evaluator is right. `LazyShortcode.refreshFromMorph` re-reveals any block that was revealed, loaded or loading, whether or not it is visible. I chose the fix over documenting the cost, because each re-fetch can be a heavy `[mrql]` on a large deployment.
   - `LazyShortcode` gets one public field, `suspended` (default `false`).
   - When a morph refreshes a block whose `suspended` is true, the block resets and goes back to being observed instead of fetching. It loads again only when it next becomes visible.
   - `TabsShortcode` sets `suspended` on every `<lazy-shortcode>` it contains, from `!!el.closest('.tabs-panel[hidden]')`, after every `_apply()`.
   - This works across the morph gap because the flag is a JS property. It survives morph, and the morph reads the value from the last `_apply()`, which reflects the visibility the panel is about to get back.
   - Plain `[lazy]` behaviour does not change, because nothing else sets the flag.
   - Tests are in §9.3. §3.4 and §6 are corrected.
3. **Roving tabindex: one rule, now stated.** While focus is inside this host's tab list, the focused tab has `tabindex="0"`. Otherwise the selected tab has it. Every other tab has `-1`.
   - The keyboard handler and `_apply()` both follow this rule.
   - A `focusout` handler resets the rule to the selected tab when focus leaves the list.
   - Tab-out and Shift+Tab sequences, and the morph case, are spelled out in §3.3.
4. **templateeval: covered.** `internal/templateeval/score.go` `consumedSlotTags` marks the start offsets of the spans `shortcodes.SlotTagSpans` returns. It uses only `span[0]`, so opener spans that carry attributes work without a code change there. New cases go in `TestUnknownShortcodeScanReadsSlotTagsWhereTheRuntimeDoes` (§9.1).
5. **Lint/runtime contract: simplified and spelled out.**
   - The opener regex is stated exactly, including that a newline after `[tab` counts.
   - The `SlotTagSpans` doc comment is updated for pair-aware `[tabs]` spans with attributed openers. `[elseif …]` is the existing precedent.
   - `lintStrayCloseTags`' `consumed` map is filled with the start offsets of both opener and closer spans.
   - **Partial pushback, simplified instead of extended:** the innermost-registered-block lookup is dropped. Every unconsumed `[tab` opener now gets one message that names the nested-block case. That removes a code path while still telling the author what is wrong.
6. **Usefulness and cost:** the stacked fallback markup stays. It is also what readers see before upgrade, and it costs nothing extra: it is just the panels without the tab roles. The fallback axe test is trimmed to a single test with one `checkA11y` plus one role assertion. The lint surface is smaller (point 5 above, and point 7).
7. **Minor issues.**
   - The first-tab-lazy lint info is dropped. The behaviour is documented only in the docs.
   - §3.4 now covers `<tabs-shortcode>` inside content inserted later (lazy responses, hover cards, deferred `[details]` bodies). Custom elements upgrade synchronously when inserted, so the same rendering-step argument applies.

## 1. Motivation and user value

Category, Resource Category and Note Type authors fill `CustomHeader` with several independent views of the same entity. Today the only way to save space is to stack `[details]` disclosures. That has three problems:

- The header grows with every section that is opened.
- Readers can open every section at once, so the header turns into one long scroll.
- Nothing tells the reader that the sections are alternatives.

`[tabs]` gives N mutually exclusive views using the WAI-ARIA Tabs pattern. Only the active panel occupies space, and its content determines the height. Concrete cases where tabs beat `[details]`:

- **Project group, `CustomHeader`:** `Overview` (description and status meta) | `Open tasks` (lazy `[mrql]` list) | `Files` (lazy gallery) | `Activity` (lazy `[mrql]` sorted by updated). Opening "Files" in a details stack adds it to the other open sections. With tabs the reader sees one section at a time.
- **Recipe Note Type, `CustomHeader`:** `Ingredients` | `Steps` | `Nutrition`. A cook flips between them without opening several disclosures at once.
- **Person group, `CustomSidebar`:** `Contact` | `Notes` (lazy). Two short labels.

A `lazy="true"` panel costs nothing on the server until a reader opens it. It reuses the existing `<lazy-shortcode>` / `/v1/shortcodes/deferred` path: no new endpoint, token type or authorization surface.

`[details]` stays the right tool for one optional section. `[tabs]` is for two or more alternative sections. `docs/ideas/category-templates-and-shortcodes.md` does not propose tabs, and no built-in or plugin shortcode provides them.

## 2. Syntax

```
[tabs label="Project views"]
  [tab label="Overview"] [property path="Description"] [/tab]
  [tab label="Open tasks" lazy="true"] [mrql query='type = "note" AND owner = [property path="ID"]' format="list"] [/tab]
  [tab label="Files" lazy="true"] [mrql query='type = "resource" AND owner = [property path="ID"]' format="list"] [/tab]
[/tabs]
```

### 2.1 `[tabs]` (block; requires a closing `[/tabs]`)

| Attr | Type | Default | Meaning |
|---|---|---|---|
| `label` | string | none | The accessible name of the tab list (`aria-label` on `role="tablist"`). When absent, no `aria-label` is emitted. The docs recommend setting it whenever a page has more than one `[tabs]`. |

There are no other attributes. The first tab is always the one selected first. There is no `active=`, no orientation option and no persistence.

### 2.2 `[tab]` (slot tag, read only directly inside `[tabs]`)

`[tab]` is a handler-local slot tag, like `[header]`, `[footer]`, `[else]` and `[elseif]`. It is **not** registered with the global parser. It is a real pair, `[tab …]…[/tab]`, and its opener carries attributes.

| Attr | Type | Default | Meaning |
|---|---|---|---|
| `label` | string | `Tab N` (N is the 1-based position) | The visible tab text and its accessible name. It is HTML-escaped and never shortcode-expanded, the same as `[details summary=]`. |
| `lazy` | enum `true` / `false` | `false` | `true` renders the body as a `[lazy]` block with exactly `[lazy]` semantics. It is fetched the first time the panel becomes visible. For the first tab, that is right after page load. Any value other than `"true"` counts as false, the same rule as `[details open=]`. |

### 2.3 Recognition rules

These live once, in `splitTabsM`, and are shared by the renderer, `slotTagSpansM` and lint.

- **Opener.** An opener is a match of the regex `\[tab(?:\s(?:"[^"]*"|'[^']*'|[^\]"'])*)?\]` in the `[tabs]` inner text that is **top-level**, meaning it is not inside any `blockInnerSpansM` span of a nested registered block.
  - `\s` matches a space, tab or newline, so `[tab\n  label="A"]` is an opener.
  - `[tabs]`, `[tabx]` and `[tab-x]` are not openers.
  - Attributes are parsed with the existing `parseAttrs`.
- **Body.** A tab's body runs from its opener to the next top-level literal `[/tab]`.
- **Nested blocks.** A `[tab]` inside a nested `[conditional]`, `[each]`, `[mrql]` and so on is not a tab of this block. Tabs cannot be added conditionally; put the `[conditional]` inside a tab instead. This is a documented non-goal.
- **Nested `[tabs]`.** A `[tabs]` inside a `[tab]` body works. It is a registered block, so its inner span shields its own `[tab]` tags from the outer scan.

## 3. Rendering

### 3.1 Server markup

The new handler is `RenderTabsShortcode(reqCtx, sc, ctx, renderer, executor, depth)` in `shortcodes/tabs_handler.go`. `processWithDepth` dispatches it as `case sc.Name == "tabs"`.

Every surface gets the same progressive-enhancement markup:

```html
<tabs-shortcode class="tabs-shortcode">
  <div class="tabs-list" role="tablist" aria-label="Project views" hidden>
    <button type="button" role="tab" class="tabs-tab" aria-selected="true" tabindex="0">Overview</button>
    <button type="button" role="tab" class="tabs-tab" aria-selected="false" tabindex="-1">Open tasks</button>
    <button type="button" role="tab" class="tabs-tab" aria-selected="false" tabindex="-1">Files</button>
  </div>
  <div class="tabs-panel" role="group" aria-label="Overview"><p class="tabs-fallback-label" aria-hidden="true">Overview</p>…rendered body…</div>
  <div class="tabs-panel" role="group" aria-label="Open tasks"><p class="tabs-fallback-label" aria-hidden="true">Open tasks</p><lazy-shortcode data-token="…"><noscript>This content requires JavaScript to load.</noscript></lazy-shortcode></div>
  <div class="tabs-panel" role="group" aria-label="Files"><p class="tabs-fallback-label" aria-hidden="true">Files</p><lazy-shortcode data-token="…">…</lazy-shortcode></div>
</tabs-shortcode>
```

**Eager panels** (`lazy` is not `"true"`) render with `processWithDepth(reqCtx, body, ctx, renderer, executor, depth+1)`. That includes panels that start hidden. `lazy` is the author's switch for heavy content, and the docs say so.

**Lazy panels** call `RenderLazyShortcode(reqCtx, Shortcode{Name: "lazy", IsBlock: true, InnerContent: body}, ctx, renderer, executor, depth)` unchanged:

- With a signer and a member entity, the panel gets a `<lazy-shortcode>` with a sealed token.
- Otherwise (live preview, JSON, carrier list slots) it gets the inline `<div class="lazy-content">…</div>`.
- `UntrustedSource`, nested deferral and the `[item]` caveat all carry over.

**Before upgrade, or without JavaScript:**

- The tab list is `hidden`.
- Each panel is a stacked, named `role="group"` showing its label.
- All content is reachable, and there are no orphan tab roles.

Headings are deliberately not used. The heading level cannot be known from inside a slot, and the axe `best-practice` `heading-order` rule (run by `e2e/helpers/accessibility/axe-helper.ts`) would fail in some slots. A `<section aria-label>` would add a landmark per tab on every page.

The server never emits **IDs**, because one template renders many times per page (cards, hover cards). The client assigns them.

### 3.2 Labels and escaping

Labels go through `html.EscapeString`, both as button text and in `aria-label`. A blank label becomes `Tab N`.

### 3.3 Client upgrade (`src/webcomponents/tabsShortcode.js`)

This file defines the custom element `TabsShortcode`. `src/main.js` imports it after `./webcomponents/deferredShortcodes.js`.

**Helpers, scoped to this host only:**

- `_list()` returns `this.querySelector(':scope > .tabs-list')`.
- `_tabs()` returns `_list()`'s direct-child `[role="tab"]` buttons (`:scope > [role="tab"]`).
- `_panels()` returns `this.querySelectorAll(':scope > .tabs-panel')`.

**State.** All state lives in JS properties on the host. Morph keeps the host node, so the properties survive.

- `_uid` is assigned once, from a module counter, on the first connect.
- `_selected` starts at 0.

**`connectedCallback`.** The first time it runs:

1. Assign `_uid`.
2. Add one delegated listener each for `keydown`, `click` and `focusout` on the host.
3. Call `_apply()` synchronously.

**Event scoping.** Every handler starts with:

```js
const tab = e.target.closest('[role="tab"]');
if (!tab || tab.parentElement !== this._list()) return;
```

So an event from a nested inner set is ignored by the outer host. The `focusout` handler uses `e.target` the same way. Events are not stopped from propagating.

**Roving tabindex rule.** While `document.activeElement` is one of `_tabs()`, the focused tab has `tabindex="0"`. Otherwise the selected tab has it. Every other tab has `-1`.

**Keyboard (APG, manual activation):**

- ArrowRight and ArrowLeft move focus to the next or previous tab, wrapping at the ends.
- Home and End move focus to the first and last tab.
- Each of these calls `preventDefault()`, focuses the target tab, and then applies the roving rule. Selection does not change.
- Enter, Space and click use native `<button>` activation, so they arrive as `click`. The click handler sets `_selected = index` and calls `_apply()`.
- Activation is manual because a lazy panel fetches when it is shown. Automatic activation would fire one request for every tab an arrow-key user passes over.

**Focus leaving the list.** On a `focusout` whose `relatedTarget` is not inside `_list()`, the tabindex resets to the selected tab. The resulting sequences:

- Selected tab 1, focus moved to tab 3, then Tab: the browser moves on in DOM order to the next focusable element, which is the visible panel 1 (or its first focusable child), because the other panels are hidden. On that `focusout`, tab 1 gets `tabindex="0"` back.
- Shift+Tab from the panel then lands on tab 1, the selected tab.
- A click elsewhere on the page also fires `focusout`, with the same reset.

**`refreshFromMorph(toEl)`.** It does **not** touch the DOM.

1. Add the host to a module-level pending `Set`.
2. If no flush is queued, `queueMicrotask(flush)`. The precedent is `scheduleNameCheck` in `reloadShortcode.js`.
3. `flush` calls `_apply()` on every pending host that is still `isConnected`.

Each host handles only its own direct children, so outer and inner hosts can flush in any order. Each `_apply()` ends by syncing the `suspended` flags under it (see step 6 below). Because those flags read `closest('.tabs-panel[hidden]')`, `flush` runs a final pass after all the `_apply()` calls: `host._syncSuspended()` for every flushed host. That makes nested hosts consistent regardless of flush order.

Why the microtask: `@alpinejs/morph` runs `patchAttributes`, then `updated()`, then `patchChildren`. `<tabs-shortcode>` is not client-owned, so a synchronous `_apply()` would be overwritten by the child pass. The microtask runs after the whole morph call returns and before the next rendering step.

**`_apply()`** is idempotent:

1. Let `tabs = _tabs()` and `panels = _panels()`. If their lengths differ, or either is zero, return and leave the fallback in place.
2. Clamp `_selected` to `[0, n-1]`.
3. For each `i`:
   - Tab: `id = tabs-sc-<uid>-tab-<i>` and `aria-controls = tabs-sc-<uid>-panel-<i>`.
   - Panel: `id = tabs-sc-<uid>-panel-<i>`, `role = tabpanel`, `aria-labelledby` set to the tab's id, and `aria-label` removed.
   - IDs depend only on `_uid` and `i`, so every pass produces the same values.
4. Remove `hidden` from `_list()`. For each panel, set `hidden` on `panel.querySelector(':scope > .tabs-fallback-label')`.
5. Set `aria-selected` on every tab. Apply the roving tabindex rule. After a morph, a focused tab node is kept in place (same tag, same position), so it keeps focus and keeps `tabindex="0"`.
6. Panels:
   - Every panel except the selected one gets `hidden`.
   - The selected panel loses `hidden`.
   - The selected panel gets `tabindex="0"` only if `panel.querySelector(FOCUSABLE)` is null; otherwise `tabindex` is removed.
   - `FOCUSABLE` = `a[href], button, input, select, textarea, summary, [tabindex]:not([tabindex="-1"]), [contenteditable="true"]`.
   - This deliberately counts the tab buttons of a nested set and the controls of nested `[details]`, so such panels get no extra tab stop.
   - The check runs every time a panel is shown. A lazy panel that is still empty gets `tabindex="0"`. If the loaded content brings links, that extra stop stays until the panel is shown again. That is harmless and matches APG's default.
7. Call `this._syncSuspended()`. For each `el` in `this.querySelectorAll('lazy-shortcode')`, it sets `el.suspended = !!el.closest('.tabs-panel[hidden]')`.

**Morph registration.**

- Add `'TABS-SHORTCODE'` to `MORPH_AWARE_TAGS` in `src/utils/shortcodeElementMorph.js`, and **not** to `CLIENT_OWNED_CHILDREN_TAGS`. Panel children are server-owned and must morph.
- Update the comment above `MORPH_AWARE_TAGS` to mention `<tabs-shortcode>` next to `<reload-shortcode>`.

**Flash of stacked content.** Before the bundle runs, readers see the stacked layout. `<details-shortcode>` has the same trade-off. CSS does not hide undefined panels, because that would hide the content from no-JS readers.

### 3.4 Lazy panels, IntersectionObserver and morph

- **Hidden panels do not load.** A `<lazy-shortcode>` in a `hidden` panel (`display: none`) never intersects. It stays observed by the shared IntersectionObserver (`rootMargin: 200px`) without loading. Selecting the tab un-hides the panel, which then intersects and loads.
- **Everything else is `LazyShortcode`'s.** Loading, `aria-busy`, the error with Retry, and `[reload]` come from `LazyShortcode` unchanged.
- **Upgrade ordering on the initial page.**
  - The module script runs after parsing.
  - `deferredShortcodes.js` defines `LazyShortcode` first, which observes.
  - `TabsShortcode` is defined later in the same task and hides the non-selected panels.
  - IntersectionObserver notifications are computed at the next rendering step, after that task, so hidden lazy panels report not-intersecting.
- **Content inserted later.** This covers lazy and deferred responses, hover cards and `[details]` bodies. A `<tabs-shortcode>` in that content upgrades synchronously when it is inserted, in tree order, inside the same task as the `innerHTML` assignment. So the host's `_apply()` and the nested `LazyShortcode.observe()` both happen before the next rendering step, and the same argument applies.
- **Morph gap.**
  - During a region morph the server markup briefly un-hides every panel.
  - A nested `LazyShortcode.refreshFromMorph` runs during `patchChildren`. It reads `suspended` as set by the last `_apply()`, which reflects the visibility the panel is about to regain.
  - **Not yet loaded:** it calls `_observeOrReveal()`, its existing path. The tabs microtask restores `hidden` before the next rendering step, so it does not fetch.
  - **Already loaded, or revealed, or loading, and `suspended` is true:** the new branch (§3.4.1) resets it and re-observes it, so it does not fetch. When the reader next selects that tab, it intersects and loads fresh content. This is correct, because the morph happened because the data may have changed.
  - **Already loaded and `suspended` is false** (the visible panel): the existing behaviour is unchanged. It re-fetches immediately, the same as a plain `[lazy]`.
  - So one morph costs at most one deferred request per visible lazy panel, never one per visited tab.
  - Token change: when the morph brings a new `data-token`, the same `suspended` rule applies to the existing `if (this._revealed) this._reveal()` branch.
- **Below the fold.** A selected lazy panel below the fold loads when it scrolls within 200px of the viewport, the same as `[lazy]`.
- **No IntersectionObserver.** `_observeOrReveal` reveals at once, so every lazy tab fetches on load and on re-arm. This is accepted: the content stays correct and only the deferral is lost. All supported browsers have IntersectionObserver. A Vitest case documents it.

#### 3.4.1 The change to `src/webcomponents/deferredShortcodes.js` (`LazyShortcode` only)

- In `connectedCallback`, inside the `if (this._init) return` guard block, add `this.suspended = this.suspended === true;`. A value set before upgrade is kept.
- In `refreshFromMorph`, replace the two places that re-reveal after `_resetContent()` with a helper:

```js
  // A block inside a hidden [tabs] panel re-arms instead of fetching; it loads
  // again when it next becomes visible.
  _rearmOrReveal(wasActive) {
    if (wasActive && !this.suspended) { this._reveal(); return; }
    this._revealed = false;
    this._observeOrReveal();
  }
```

  The two call sites become:
  - token-change branch: `this._resetContent(); this._rearmOrReveal(this._revealed);`
  - same-token branch: `if (this._revealed || this._loaded || this._loading) { this._resetContent(); this._rearmOrReveal(true); } else { this._observeOrReveal(); }`

  The `wasActive` argument keeps each branch's existing condition. The token-change branch reveals only if the block was revealed. The same-token branch's existing condition is unchanged. With `suspended` false, both behave exactly as today.
- `attributeChangedCallback`, `reload()` and `DetailsShortcode` are not changed. A `[reload]` button can be clicked only in a visible panel. `[details]` already loads only while it is open.

### 3.5 CSS (`public/index.css`, next to the `details-shortcode` rules)

- `tabs-shortcode { display: block; }`
- `.tabs-list:not([hidden])`:
  - `display: flex; flex-wrap: wrap; gap: .25rem; border-bottom: 1px solid #d6d3d1;`
  - `:not([hidden])` is there so `display: flex` cannot override the `hidden` attribute.
  - The list wraps onto more rows instead of scrolling sideways.
- `.tabs-tab`:
  - `padding: .375rem .75rem; color: #292524; background: transparent; border: 0; border-bottom: 2px solid transparent; max-width: 100%; overflow-wrap: anywhere; text-align: start;`
  - A long label wraps inside its button.
- `.tabs-tab[aria-selected="true"] { border-bottom-color: #b45309; font-weight: 600; }`. Selection is not shown by colour alone.
- `.tabs-tab:focus-visible, .tabs-panel:focus-visible { outline: 2px solid #b45309; outline-offset: 2px; }`
- `.tabs-fallback-label { font-weight: 600; margin: .75rem 0 .25rem; }`
- `.tabs-panel { margin-top: .5rem; }`
- The colours are the existing `details` values, which already pass axe AA.

## 4. Empty, missing and invalid input

| Case | Runtime | Lint |
|---|---|---|
| `[tabs]` without `[/tabs]` | `shortcodeErrorMarker("tabs", "[tabs] requires a closing [/tabs] tag")` | the existing unmatched-block error |
| No top-level `[tab]…[/tab]` | error marker: `[tabs] needs at least one [tab label="…"]…[/tab]` | error, same text |
| A top-level opener with no top-level `[/tab]` after it | error marker for the whole block: `[tab] is missing its closing [/tab]` | error |
| A second top-level opener before the current `[/tab]` | error marker: `[tab] cannot contain another [tab]; nest a [tabs] block instead` | error |
| Missing or blank `label` | `Tab N` | warning: `[tab] has no label; it is shown as "Tab N"` |
| `lazy` not `true` / `false` | treated as false | warning: `lazy on [tab] must be "true" or "false"` |
| `lazy="true"` on the first tab | loads right after the page does (documented) | none |
| Unknown attribute on `[tab]` | ignored | warning: `unknown attribute "x" on [tab]` |
| Non-whitespace content outside the top-level tabs | dropped | warning: `content outside [tab]…[/tab] inside [tabs] is not rendered` |
| `[tabs]` inside a `[reload]` button face | `shortcodeErrorMarker("tabs", "[tabs] cannot be used inside a [reload] button face")` via `withinReloadFace` | error, by adding `"tabs"` to the `case "lazy", "details":` arm |
| A `[tab` opener that no `[tabs]` consumes (outside any `[tabs]`, or inside a nested block such as `[conditional]` within one) | literal text | warning: `[tab] is only read directly inside [tabs], not inside a nested block such as [conditional]; here it is shown as text` |
| A `[/tab]` that closes no consumed tab | literal text | error: `stray [/tab] closes no tab; [tab]…[/tab] is read only directly inside a [tabs] block` |
| A label containing `<`, `&` or quotes | HTML-escaped | none |

## 5. Scoping and authorization

`[tabs]` does no data access of its own.

- **Eager panels** render with the caller's `ctx`, `executor` and `renderer`, which are already the scoped executor and the plugin access gate (`auth.PluginAccessFor`). A group-subtree-confined user sees exactly what the same content would show outside `[tabs]`.
- **Lazy panels** use the existing `[lazy]` token.
  - `POST /v1/shortcodes/deferred` is wrapped in `scopedAPI`.
  - The endpoint re-loads the entity through `loadPreviewEntity` under the principal's scope.
  - For an out-of-subtree entity it returns 404 even when the token is valid (`TestDeferredRender_ScopedPrincipal`).
  - The token seals only the entity type and id, never data, so it cannot widen access and it fails closed.

There is no new route, signer or token kind. The `suspended` flag is client-only and has no effect on authorization. A regression test (§9.2) proves that a lazy tab's token passes through the same fail-closed gate.

## 6. Performance (millions of resources)

- **No queries of its own.** `[tabs]` issues no queries. `splitTabsM` is linear. It uses `blockInnerSpansM` and `m.alive()` checks under the existing render meter (`render_budget.go`), and the parsed-source and output budgets apply to every panel.
- **Eager panels** cost the sum of their bodies, charged to the existing per-page query budget (`query_budget.go`).
- **Lazy panels** cost zero queries until they are shown. Each one then costs one deferred request under the same budget as `[lazy]`.
- **Manual activation** prevents arrow-key request storms.
- **Morphs** (a `[reload]` or paste-upload of a region) re-fetch only the visible lazy panel. Hidden panels that were visited earlier re-arm instead of fetching (§3.4).
- **Docs guidance:** use `lazy="true"` on every non-first tab that holds `[mrql]`, a gallery or a plugin data view, and do not use `[tabs]` in per-card `CustomSummary`.

## 7. Where it lives: a built-in Go handler

A plugin cannot do this. It needs:

- the deferred signer (`deferredSignerFrom`), which Lua cannot reach;
- parser registration, so `[tabs]` pairing shields nested `[tab]` tags;
- the built-in fail-closed `[reload]`-face check;
- a bundled client custom element, plus a hook in `LazyShortcode`.

The a11y behaviour belongs in core JS, where unit and axe tests cover it.

## 8. Files to change

### Go, in `shortcodes/`

1. [x] **`parser.go`**: add `tabs` to the alternation in both `shortcodePattern` and `closingTagPattern`. Do not add `tab`.
2. [x] **`tabs_handler.go`** (new):
   - `var tabOpenPattern = regexp.MustCompile(` + "`" + `\[tab(?:\s(?:"[^"]*"|'[^']*'|[^\]"'])*)?\]` + "`" + `)`
   - `const tabCloseTag = "[/tab]"`
   - `type tabSlot struct { Attrs map[string]string; Body string; OpenStart, OpenEnd, CloseStart, CloseEnd int }`
   - `splitTabsM(m *renderMeter, inner string) (tabs []tabSlot, outside bool, errMsg string)`:
     - returns the top-level tabs in source order;
     - reports whether non-whitespace text lies outside them;
     - returns the §4 unclosed or nested-opener message.
   - `RenderTabsShortcode` as in §3.1, built with `strings.Builder` and `html.EscapeString`.
3. [x] **`processor.go`**: add `case sc.Name == "tabs":`.
4. [x] **`mrql_slots.go`**:
   - Add `"tabs": {"tab"}` to `slotTags`.
   - Add `case "tabs":` to `slotTagSpansM`. For each `tabSlot` in order, it returns the opener span `{OpenStart, OpenEnd}` followed by the closer span `{CloseStart, CloseEnd}`. On an `errMsg` it returns the pairs completed before the error.
   - Extend the `SlotTagSpans` doc comment with: "for [tabs] each top-level [tab …] opener (attributes included, like [elseif …]) followed by its [/tab] closer".
   - Update the other comments that enumerate the parents.
5. [x] **`lint.go`**:
   - Add `"tabs"` to `builtinBaseNames`.
   - In the loose-bracket near-miss scan, skip `name == "tab"` next to `else` and `elseif` (around line 655). Otherwise `[tab]` would get "did you mean [tabs]?".
   - Add `lintTabs(w, lm, input, tokens, add)`. For each `namedBlockSpans(tokens, "tabs")` it reports the §4 rows from `splitTabsM`, charging `w` the way the `[mrql]` slot path does (`w.charge(1 + len/8)`).
   - In `lintStrayCloseTags`:
     - Add a second loop over `namedBlockSpans(tokens, "tabs")` that fills `consumed` with `span[0]+s[0]` for every span `slotTagSpansM(lm, "tabs", …)` returns. Those are the start offsets of both openers and closers.
     - Add `"tab"` to the closer loop, with the §4 stray `[/tab]` error.
     - Add an opener scan with `tabOpenPattern` over `input`. Every match whose start is not in `consumed` gets the single §4 warning.
     - Update the function comment.
   - Add `"tabs"` to the reload-face `case "lazy", "details":` arm.
6. [x] **`builtin_docs.go`**: add a `tabs` entry after `details`.
   - `IsBlock: BlockRequired`, with the `label` attr.
   - The description covers:
     - `[tab label= lazy=]`;
     - the stacked no-JS fallback;
     - eager vs lazy, and that a lazy first tab loads after page load;
     - the `[item]` caveat for lazy panels;
     - the keyboard model;
     - the conditional-tabs non-goal.
   - Two examples: a basic one, and one with lazy `[mrql]` tabs. The query must pass `TestBuiltinDocMRQLExamplesParse`.
   - `[tab]` gets no entry of its own, like `[header]`.

### `internal/templateeval/`

7. [x] **No code change.** `consumedSlotTags` (`score.go`) iterates `shortcodes.SlotTagSpans` generically and records only `span[0]`, so `[tab …]` openers and `[/tab]` closers become known through the new spans. This dependency is pinned by a test (§9.1).

### Frontend

8. [x] **`src/webcomponents/tabsShortcode.js`** (new): §3.3.
9. [x] **`src/webcomponents/deferredShortcodes.js`**: the `suspended` field and `_rearmOrReveal` (§3.4.1). Nothing else changes.
10. [x] **`src/main.js`**: import `./webcomponents/tabsShortcode.js` after `./webcomponents/deferredShortcodes.js`.
11. [x] **`src/utils/shortcodeElementMorph.js`**: add `'TABS-SHORTCODE'` to `MORPH_AWARE_TAGS` only, and update its comment.
12. [x] **`public/index.css`**: the §3.5 rules.

### Templates and docs

13. [x] **`templates/createCategory.tpl`, `createResourceCategory.tpl`, `createNoteType.tpl`**: add a `[tabs]` line to the Shortcodes reference panel (`TestReferencePanelsCoverAllBuiltins`).
14. [x] **`docs-site/docs/features/shortcodes.md`**:
    - Add `[tabs]…[/tabs]` to the built-in block list and table.
    - Add a new section ``## `[tabs]` -- Tabbed sections`` covering:
      - attributes and the `[tab]` rules;
      - eager vs lazy, including the first-tab note;
      - the no-JS fallback;
      - the keyboard model (manual activation);
      - wrapping;
      - when to prefer `[details]`;
      - slot guidance: use it in `CustomHeader`, use at most 3 short labels in `CustomSidebar`, do not use it in `CustomSummary`;
      - the conditional-tabs non-goal;
      - the Project and Recipe examples.
    - Under "Where deferral applies", mention that hidden lazy tabs re-load on next view after a `[reload]`.
15. [x] **`docs-site/docs/features/custom-templates.md`**: add `[tabs]` to the shortcode list, and note the carrier-slot inline fallback next to the existing `[lazy]` / `[details]` note.
16. [x] **`.claude/skills/mahresources-category-designer/references/shortcodes.md`**: add a `[tabs]` entry with the same slot guidance.

## 9. Tests

### 9.1 Go unit tests

**`shortcodes/tabs_handler_test.go`:**

- **Exact inline output (no signer), two tabs.**
  - The tab list is `hidden`.
  - `aria-selected` is true then false; `tabindex` is 0 then -1.
  - The tab list's `aria-label` is present only when `label` is set.
  - Each panel has `role="group"` and an escaped `aria-label`, plus the `aria-hidden` fallback `<p>`.
  - The eager bodies are rendered.
- **Lazy tab by surface.**
  - With no signer, the panel is an inline `<div class="lazy-content">`.
  - With `WithDeferredSigner(fakeSigner)` and a note entity, it is a `<lazy-shortcode data-token=…>`, and the signer receives exactly the tab body.
  - On a carrier (`EntityType: "category"`), it is inline.
- **Every §4 runtime row:**
  - no tabs;
  - unclosed `[tab]`;
  - nested top-level opener;
  - blank label becomes `Tab 2`;
  - invalid `lazy`;
  - outside content is dropped;
  - reload-face error;
  - `[tab\nlabel="A"]` is an opener.
- **Nesting.** A nested `[tabs]` renders as an inner set. A `[tab]` inside a `[conditional]` within `[tabs]` is not a tab.
- **Executor counts.** An eager `[mrql]` tab calls the executor once. A lazy `[mrql]` tab under a signer calls it zero times.
- **Render budget.** A `[tabs]` body over the meter returns `""` without panicking.

**`parser_test.go`:** `[tabs]…[/tabs]` pairs as a block, and `[tab …]` is not tokenized.

**`mrql_slots_test.go`:** extend `TestSlotTagSpansAreWhatTheHandlersConsume` with a pair-aware `tabs` loop over these inputs:

- `[tab label="A"]x[/tab][tab]y[/tab]`
- `[tab]a[tabs][tab]b[/tab][/tabs][/tab]`
- `[tab][conditional path="x"][tab]z[/tab][/conditional][/tab]`

For each input, assert:

1. The span count is even. Span `2k` matches `^\[tab(\s|\])` and span `2k+1` equals `[/tab]`. The spans are sorted and do not overlap.
2. `cut(inner, spans)` equals the concatenation of `splitTabsM`'s `Body` values.
3. The inner `[tab]` tags are not among the spans.

**`lint_test.go`:**

- Each §4 lint row.
- The single unconsumed-opener warning, both outside any `[tabs]` and inside a nested `[conditional]`.
- A stray `[/tab]` error.
- No near-miss info for `[tab]`.
- A lazy first tab yields no issue.
- A clean example yields no issues.

**`internal/templateeval/score_test.go`:** add cases to `TestUnknownShortcodeScanReadsSlotTagsWhereTheRuntimeDoes`:

- `[tabs][tab label="A"]x[/tab][tab lazy="true"]y[/tab][/tabs]` gives `nil`.
- `<p>[tab label="A"]x[/tab]</p>` gives `{"tab"}`.
- `[tabs][tab][conditional mrql="type = note"][tab]z[/tab][/conditional][/tab][/tabs]` gives `{"tab"}`.

**Consistency:** `builtin_docs_consistency_test.go` runs unchanged and must pass.

### 9.2 API test (`server/api_tests/deferred_render_api_test.go`)

`TestDeferredRender_TabsLazyPanelScoped`:

1. Render `[tabs][tab label="A"]x[/tab][tab label="B" lazy="true"][property path="Name"][/tab][/tabs]` through `shortcodes.Process`, with a capturing signer bound to the real key. Do this once for an in-scope group and once for an out-of-subtree group.
2. POST each token as a scoped user.
3. Expect 200 with the name for the in-scope group, and 404 for the out-of-subtree group.

### 9.3 Vitest

**`src/webcomponents/tabsShortcode.test.ts`** uses `// @vitest-environment happy-dom` and a real `Alpine.morph`, as `src/pm-native.test.ts` does.

- **Upgrade.**
  - IDs, `aria-controls`, `aria-labelledby` and `role="tabpanel"` are set, and the panels' `aria-label` is removed.
  - The tab list is unhidden; the fallback labels and inactive panels are hidden.
  - A panel without focusable content gets `tabindex="0"`; a panel with a link gets none.
- **Keyboard and roving tabindex.**
  - ArrowRight and ArrowLeft wrap. Home and End work.
  - These keys move focus and `tabindex` without changing `aria-selected`.
  - Enter, Space and click select.
  - After ArrowRight from selected tab 1 to tab 2, dispatching `focusout` with an outside `relatedTarget` gives tab 1 `tabindex="0"` and tab 2 `tabindex="-1"`.
- **Nested sets.**
  - An outer set whose tab 1 panel holds an inner `[tabs]`.
  - ArrowRight on an inner tab moves focus only within the inner list, and the outer tabs' `tabindex` and `aria-selected` do not change.
  - A click on inner tab 2 selects inner panel 2. Outer `_selected` stays 0 and the outer panels do not change.
  - Upgrading the outer set does not hide the inner set's fallback labels by its own action.
  - Outer panel 1 has no `tabindex`, because it contains tab buttons.
- **Real morph.**
  1. Mount `<main>`, select tab 3 and focus its button, and record the IDs.
  2. Run `Alpine.morph(main, sameServerMarkup, morphOptionsWithShortcodeElements())`.
  3. Assert that the attributes were reset *before* the microtask.
  4. Await a microtask, then assert:
     - tab 3 is selected;
     - panels 1 and 2 are hidden;
     - the tab list is shown and the fallback labels are hidden;
     - every `id`, `aria-controls` and `aria-labelledby` matches the recorded values;
     - `document.activeElement` is the same node.
- **Morph to fewer tabs.** Selection clamps to the last tab.
- **Mismatched counts.** Nothing changes.
- **IntersectionObserver gating** (stub IntersectionObserver plus stub `fetch`):
  - A hidden lazy tab-2 panel does not fetch.
  - After selection, firing the callback with `isIntersecting: true` fetches once.
  - With `IntersectionObserver` undefined, every lazy panel fetches on connect.
- **Visited-then-hidden lazy panel on morph (regression for evaluator point 2).**
  1. Select lazy tab 3 and let it load (one fetch).
  2. Select tab 1.
  3. Morph the region with the same markup and await a microtask.
  4. Assert zero new fetches, that tab 3's `<lazy-shortcode>` has `suspended === true` and is observed again, and that panel 3 is hidden.
  5. Select tab 3 and fire intersection: exactly one new fetch.
  6. Control: with tab 3 still selected, a morph fetches once immediately (unchanged `[lazy]` behaviour).

**`src/webcomponents/deferredReload.test.ts`** (existing `LazyShortcode` tests): add that with `suspended = false` (the default), `refreshFromMorph` on a loaded block still reveals immediately. With `suspended = true`, it resets, sets `_revealed = false`, and calls `observe` instead of `fetch`.

**`src/utils/shortcodeElementMorph.test.ts`:** add `TABS-SHORTCODE` to the "refreshes after attributes have been patched" list, and assert that its children are not skipped, mirroring `RELOAD-SHORTCODE`.

### 9.4 E2E

**`e2e/tests/shortcodes/tabs-shortcode.spec.ts`**: a category whose `CustomHeader` has three tabs. The third is `lazy="true"` and holds a stamped payload plus a `[reload]`. All checks run on `/group?id=`.

- **Initial state.** The tab list is visible with 3 tabs, panels 2 and 3 are hidden, and no `/v1/shortcodes/deferred` POST has happened.
- **Keyboard selection.** Focus tab 1, press ArrowRight twice, then Enter. Panel 3 is shown, one deferred POST fires, and the payload appears.
- **Wrap.** ArrowRight on the last tab wraps to the first.
- **Reload.** `[reload]` inside the lazy panel refreshes only that panel, and tab 3 stays selected.
- **Hidden visited panel.** Select tab 1, then trigger a region morph (paste-upload or a header `[reload]` on an eager panel). No deferred POST fires. Selecting tab 3 again fires one.
- **Live preview.** Renders the lazy panel inline.
- **320 px viewport, with long labels.**
  - `tabs-shortcode` has `scrollWidth <= clientWidth`.
  - The document has no horizontal scroll.
  - Every tab button is inside the viewport.

**`e2e/tests/accessibility/tabs-shortcode-a11y.spec.ts`**:

- `checkA11y()` in the initial state, and again after loading the lazy tab.
- **One fallback test.** Block `**/public/dist/main.js*`, then:
  - assert `getByRole('group', { name: 'Overview' })` is visible;
  - run `checkA11y()` once.
- A Note Type `[tabs]` on a share page upgrades: `getByRole('tablist')` is visible.

### 9.5 Commands

```
go test --tags 'json1 fts5' ./shortcodes/... ./internal/templateeval/... ./server/...
npm run test:unit
npm run build
cd e2e && npm run test:with-server -- tests/shortcodes/tabs-shortcode.spec.ts
cd e2e && npm run test:with-server:a11y
./scripts/css-scan-test.sh
```

## 10. Success criterion

The feature is done when all of the following hold:

1. **The tab set works for authors and readers.** An author can write `[tabs][tab label="…"]…[/tab]…[/tabs]` in any `Custom*` slot and get a WAI-ARIA tab set that:
   - passes axe, best-practice rules included;
   - is fully operable with Arrow, Home, End, Enter, Space and Tab/Shift+Tab, with one roving tab stop;
   - works when nested, with each set reacting only to its own tabs;
   - keeps the selected tab and IDs for uniquely matching sets across `[reload]`, `[autorefresh]` and region morphs; restores tab focus when the same tab survives the update;
   - wraps without horizontal overflow at 320 px;
   - degrades to stacked, named groups without JS.
2. **Lazy tabs defer and stay scoped.**
   - A `lazy="true"` tab other than the first issues zero server queries until it is selected, then loads through `/v1/shortcodes/deferred`.
   - A region morph re-fetches only the visible lazy panel.
   - A group-subtree-confined user gets 404 for out-of-subtree tokens.
3. **Lint and templateeval are complete and quiet.**
   - Lint reports every malformed case in §4.
   - templateeval treats consumed `[tab]` tags as known and unconsumed ones as unknown.
   - Valid templates, including a lazy first tab, produce no new lint noise.
4. **Docs and tests are in place.** `BuiltinDocs`, the three editor reference panels, the docs-site pages and the category-designer skill reference document `[tabs]`, and the Go, Vitest, e2e and a11y suites above pass.

---

## Last evaluator feedback

I checked the plan's claims against the code and they hold up:
- `LazyShortcode.refreshFromMorph` has two re-reveal sites (the token-change branch and the same-token branch with `_revealed || _loaded || _loading`). `_rearmOrReveal` fits them without changing the non-suspended behaviour.
- `MORPH_AWARE_TAGS` / `CLIENT_OWNED_CHILDREN_TAGS` and the `updated()` hook in `shortcodeElementMorph.js` work as described. `updated` fires after the attribute patch and before children, so the microtask deferral is justified.
- `slotTags`, `slotTagSpansM` and the `SlotTagSpans` doc comment are in `mrql_slots.go`. `consumedSlotTags` in templateeval uses only `span[0]`, so no change is needed there.
- `lintStrayCloseTags`' `consumed` map, the near-miss skip at `lint.go:655`, `builtinBaseNames`, the reload-face `case "lazy", "details"` arm, `withinReloadFace`, and the `RenderLazyShortcode` / `RenderDetailsShortcode` signatures all exist as stated.
- All the named tests exist: `TestReferencePanelsCoverAllBuiltins`, `TestUnknownShortcodeScanReadsSlotTagsWhereTheRuntimeDoes`, `TestSlotTagSpansAreWhatTheHandlersConsume`, `TestDeferredRender_ScopedPrincipal` and `TestBuiltinDocMRQLExamplesParse`.
- Share pages load `main.js`, and `docs/ideas` does not propose tabs.

Scoping fails closed by reusing the `[lazy]` token, and the a11y design follows APG with manual activation. I would approve it.

Remaining issues, most important first:

1. **Focus during the morph gap.** The morph re-adds `hidden` to the tab list while one of its tabs has focus. The plan says `document.activeElement` survives, but happy-dom does not run the focus-fixup rule. In Chromium, anything that forces style or layout during the morph (for example another morph-aware element reading geometry, or a focusout handler) can blur the focused tab before the microtask un-hides the list. Either add a Playwright check that the tab keeps focus after a real `[reload]` or paste-upload morph, or say the guarantee is best-effort and re-focus the previously focused tab in `flush` if focus was lost (record it in `refreshFromMorph`).

2. **The e2e "hidden visited panel" step does not name a trigger.** "Paste-upload or a header `[reload]` on an eager panel" leaves it open. Check what a `[reload]` placed in eager content actually morphs (its target region) and name one concrete trigger, or the test will be written by guessing.

3. **Stale panel tab stop.** A lazy panel that is empty when shown gets `tabindex="0"`, and it keeps it after links load until the panel is shown again. That is cheap to fix: re-run the FOCUSABLE check once the nested `<lazy-shortcode>` finishes loading (listen for its load or `aria-busy` change). Otherwise keyboard users get an extra stop on the most common lazy case.

4. **Lint opener scan runs over all of `input`.** The `tabOpenPattern` scan in `lintStrayCloseTags` also runs in CSS mode and inside `<style>`. The loose-bracket scan already skips `cssSpans` / `CSSMode`. Do the same here so a `[tab]` attribute selector in `CustomCSS` does not get a misleading warning. The new `[/tab]` stray-closer error needs the same exemption.

5. **`[tab` counted as top-level inside a nested shortcode's opening-tag attributes.** "Top-level" is defined against `blockInnerSpansM`, so a `[tab` that sits inside a nested shortcode's attribute value (for example in an `[mrql query='…']` opener) still counts as top-level. This is a rare edge, but state it or exclude shortcode opener spans too, so the renderer and lint agree on purpose rather than by accident.

6. **Scope trimming (minor).** The test matrix is very large. The no-IntersectionObserver Vitest case and the separate 320 px "every tab button inside the viewport" check could be folded into existing cases. They do no harm as written.

Usefulness: tabs are a common, real layout need for `CustomHeader`. `[details]` does not express mutually exclusive alternatives, and the lazy-per-tab deferral matters on large deployments. The value is a little lower in sidebars and summaries, which the plan itself discourages.

## Review (implementation record)

Built on `feat/shortcode-tabs`. Every §8 item is done (ticked above) and every §4 row is implemented and tested. The evaluator points below were all taken as required work.

### How each evaluator point was resolved

1. **Focus during the morph gap.** Both halves. `refreshFromMorph` records the focused tab; the microtask flush, after re-applying every host, gives focus back to that tab when focus has fallen to `<body>` (never when it went somewhere real) and re-syncs the roving tabindex. Vitest simulates the loss with a blur inside the real morph and fails without the restore. The E2E morph test (point 2's trigger) asserts that the focused Overview tab is still focused after a real paste-upload morph in Chromium.
2. **Naming the trigger.** Checked against the code: a `[reload]` in an eager panel does **not** morph. It resolves to the slot's `[data-shortcode-region]` and `reloadInto` swaps it with `innerHTML` (`reloadShortcode.js` → `deferredShortcodes.js`). So the morph trigger in the E2E spec is **paste-upload**, whose `_refreshPage` morphs `.main` with `morphOptionsWithShortcodeElements`. The innerHTML swap found here is a gap the plan had not seen: a fresh `<tabs-shortcode>` started on its first tab, so a `[reload]` in tab 2 closed tab 2 and its focus restore aimed at a button inside a hidden panel. `reloadInto` now carries each host's selection and IDs into the host at the same position in the new markup (`captureTabSelections` / `restoreTabSelections`, which skip hosts inside nested `[lazy]`/`[details]` bodies, since those come back unrevealed). That one change covers region reloads, `[lazy]`/`[details]` reloads and `[autorefresh]`. An E2E test fails without it.
3. **Stale panel tab stop.** A `MutationObserver` on the host watches `aria-busy` in its subtree, which every deferred block clears when content lands (first load, retry, `[reload]`), and re-runs the panel tab-stop check. A focused panel keeps its `tabindex` so focus is not dropped to `<body>`; it is re-checked the next time the panel is shown. Vitest covers the loaded-link case.
4. **Lint scan in stylesheets.** The unconsumed-opener and stray-closer scans skip `CSSMode` entirely and `<style>` elements otherwise. They also skip text inside a shortcode's own tag (an attribute value), and the body of a `[tabs]` whose problem is already reported, so one malformed block is one issue rather than a cascade.
5. **`[tab` inside a nested shortcode's attributes.** Excluded on purpose. "Top-level" now means outside the whole extent (`Start`..`End`) of every shortcode the parser finds in the body: a nested block's opener, body and closer, and an inline shortcode's tag. `splitTabsM` is the one definition the renderer, `SlotTagSpans` and lint share, so they agree by construction. Tested in the handler, lint and slot-span tests.
6. **Scope trimming.** The no-IntersectionObserver Vitest case is dropped: it exercises `LazyShortcode` behaviour this change does not touch, and the shared observer is cached per module, so it cannot be tested beside the IO-stubbed cases. The 320 px button check is folded into the one narrow-viewport test.

### Deviations from the plan

- **`attributeChangedCallback` also re-arms.** §3.4.1 said only `refreshFromMorph` changes. The first E2E run showed one stray request per morph: every server render seals a fresh, nonced token, so a morph changes `data-token`, and `attributeChangedCallback` (which runs first, during the attribute patch) revealed the hidden block. It now calls `_rearmOrReveal(this._revealed)` too. With `suspended` false it behaves exactly as before. A Vitest case morphs with a freshly sealed token and fails without the change.
- **`reloadInto` carries tab state** (point 2 above). `deferredShortcodes.js` therefore imports two helpers from `tabsShortcode.js`, which means `<tabs-shortcode>` is defined just before `<lazy-shortcode>`. That ordering is harmless (and slightly better: hidden panels are hidden before any lazy block observes), and `suspended` set on a not-yet-upgraded `<lazy-shortcode>` is kept, as §3.4.1 already required. The `src/main.js` import stays, after `deferredShortcodes.js`, as the plan said.
- **`splitTabsM` returns a `tabsSplit` struct** (`Tabs`, `Outside` ranges, `Problem`, `ProblemAt`) instead of `(tabs, outside bool, errMsg)`, so lint can anchor each issue on the opener or the dropped text it is about.
- **Lint lives in a new `tabs_lint.go`** (`lintTabs`, one call from `lintText`), not inside `lintStrayCloseTags`, and the `[reload]`-face check is there rather than in the shared `case "lazy", "details", …` arm. Same behaviour, fewer shared lines for the five parallel lanes to conflict on. The only shared `lint.go` edits are `builtinBaseNames`, the near-miss skip for `tab`, and the call.
- **API test** is a new file, `server/api_tests/tabs_shortcode_scope_test.go`, and uses the tokens a real `/group` page mints (admin page for the outside group) rather than a capturing signer: a confined guest gets 200 for its own subtree and 404 for the outside group's token; an admin control gets 200.
- **Vitest layout.** The `LazyShortcode.suspended` cases live in `tabsShortcode.test.ts`, under happy-dom with real custom elements, because `deferredReload.test.ts` stubs `HTMLElement` and cannot construct the element. `deferredReload.test.ts` only gains `querySelectorAll` on its fake container. Native Enter/Space activation is covered by the E2E spec, not Vitest.
- **E2E paths.** `e2e/tests/108-tabs-shortcode.spec.ts` (the lane's assigned prefix) and `e2e/tests/accessibility/tabs-shortcode-a11y.spec.ts`. The shared-note scan is scoped to the tab set with `region` and `landmark-one-main` disabled: the share page template has no `<main>` landmark, which axe reports page-wide whatever the content (pre-existing, not changed here).
- **CSS** selectors are scoped under `tabs-shortcode >` so an author's own `.tabs-tab` class is not restyled, and tabs get `cursor: pointer`.
- **Docs.** The `BuiltinDoc` comment listing the built-ins was not edited in this lane. See the round 1 review resolutions below for the updated selection guarantee.

### Verified

- `go test --tags 'json1 fts5' ./...`; the new `shortcodes` handler, lint and slot-span tests, the templateeval cases, and the API scope test.
- `npm run test:unit` (Vitest), `npm run build`, `./scripts/css-scan-test.sh`.
- E2E: the two new specs, plus the deferred, `[reload]`, `[autorefresh]`, `[calendar]`, shortcode lint/autocomplete/general specs and the category-template authoring a11y spec.

### Not verified

- Postgres: `[tabs]` runs no queries, so no Postgres test was added or run for it.
- Chromium does not lose focus in the morph gap on this page: with the restore disabled, the paste-upload E2E test still passed. The restore is a safeguard for browsers or pages that force a style recalculation mid-morph, proven by simulation in Vitest.
- Windows forced colours and right-to-left arrow direction (arrows are not flipped for RTL) were not checked. Selection is shown by weight as well as the accent underline, so it does not rely on colour.
- Only Chromium runs the E2E suite.

### Round 1 review resolutions

- A complete `[tab …]` slot opener is opaque to the global token scan and to lint's delimiter scans; it is not itself a shortcode token. Quoted `[tabs]`, `[/tabs]`, `[/tab]` and other shortcode text stay literal (`TestTabsQuotedBracketTextIsLiteral`). Outside-content lint trims Unicode whitespace on both sides of the reported span (`TestLintTabsOutsideContentSpanTrimsUnicodeWhitespace`).
- Lazy blocks coalesce a changed-token callback and the morph update into one request after tab visibility settles. The Vitest tests cover same-token and fresh-token morphs of a visible panel, and the negative control for one the reader left.
- Panel tab stops use the same `tabbable` library already included by the app's focus-trap tooling. `focusin` enforces the roving tabindex for keyboard or programmatic focus. Chromium covers Tab with hidden, disabled and negative-tabindex descendants, and a real `[autorefresh]` swap that restores focus on an unselected tab.
- Supersedes positional state matching in the implementation record above: reload swaps and page morphs preserve selection and client IDs only when the same raw `[tabs]` source and entity pair uniquely on both sides. Otherwise selection starts on the first tab. Tests remove a preceding set in `[reload]` and Alpine morphs, and check duplicate identities on either side of a reload swap and across two morphs in one turn.
- Hover cards emit only stacked labelled groups, with no upgradable tab control. Lazy panel bodies still use `[lazy]`; the shared deferred-token change carries the hover-card context through the fetch. `TestTabsShortcode_HoverCardStacksLabelledGroups` checks the real hover-card fragment and the normal detail page. The lightbox Info panel inherits readable tab label colours, pinned by the lightbox axe test.
- Only the selected panel takes up space, not a constant-height area; the authoring references now describe the layout as implemented. The stylesheet selector collision belongs to the shared cross-shortcode lane.

### Round 2 review resolutions

- The stylesheet and HTML-balance scans mask complete shortcode and tab-slot openers without shifting byte offsets. `TestTabsQuotedStyleTextDoesNotChangeStylesheetContext` checks a literal `<style>` label, a real stylesheet and a working `[reload]` in the same template.
- `[tab …]` is opaque only at the top level of a paired `[tabs]` body. `TestTabLikeTextDoesNotHideShortcodesOutsideTabSlots` checks that the property still expands in bracketed prose, after an unpaired `[tabs]`, inside a nested conditional and after a CSS selector.
- An unambiguous tab-set match now carries focus as well as selection and IDs. Morphs and `[autorefresh]` follow the focused tab by index; an activated `[reload]` follows its button by position within the matched panel. If the old focus is lost and its replacement cannot take focus, focus is parked on the refreshed content. Tests remove a preceding tab set in all three paths and verify the fallback when a reload button survives but is hidden.
- Lint shares the parser's literal-selector test. `TestLintTreatsStylesheetBareOpenersAsSelectors` covers the accepted prefixes in both `CustomCSS` and embedded `<style>` and checks that interpolation still receives a diagnostic.
- Validation: tagged Go shortcode, internal, template-handler and targeted API tests passed; `go vet` reported only the pre-existing lock-copy warning in `application_context/job_download_progress_test.go`. All 2,692 Vitest tests and 29 Chromium tests (tabs, tabs accessibility, reload and autorefresh) passed. No SQL changed.

#### Known limits

- A bare `[tab]` attribute selector such as `.x[tab]` inside a `<style>` in a tab body can still be read as a nested `[tab]` and cause a visible error. Write `.x[ tab]` as a workaround.
- A nested tab set in a lazy tab inside a lightbox `[autorefresh]` can reset after Reload: the initial lightbox renders the lazy tab inline, while the refresh response defers it again.
