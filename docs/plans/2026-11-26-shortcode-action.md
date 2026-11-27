# Plan: `[action]`, a button that runs a plugin action from a template (revision 3)

Slug: `action`. Saved as `docs/plans/2026-11-26-shortcode-action.md` (moved from `docs/ideas/shortcodes/action-button.md`). This is a built-in Go shortcode plus one small web component. It adds no endpoint, no migration, no plugin API and no new pongo context key.

## 0. What changed since revision 2 (a reply to each review point)

1. **`.json` break (P1). Fixed by removing the cause.** Revision 2 put a new func value, `_pluginActionAccess`, on every page context, and `render_template.go`'s deny-list `discardFields` did not list it. This revision adds **no context key**. The `process_shortcodes` tag already receives `_pluginManager`, `_requestContext` and the `_pluginAccess` predicate (through `pluginAccessFromContext`). It builds the action predicate from those (§8.2), so the `discardFields` list and the JSON encoding stay as they are. A regression test still asserts that a detail `.json` encodes and that its lightbox slot contains the button (§10).
2. **Missing render surfaces. Fixed by the same change.** `list_card_renderer.go`, `text_block_renderer.go` and `errorPageEnricher` (server.go) all set `_pluginManager`, `_requestContext` and `_pluginAccess`. Every render that goes through the tag (detail pages, list cards, MRQL result cards and note text blocks) therefore gets the resolver without per-file wiring. Two surfaces call `shortcodes.Process` directly and need explicit wiring: the lightbox `processShortcodesForJSON` and the deferred handler. Tests cover a list card and an `[mrql]` result (§10).
3. **The deferred handler's context type. Verified.** `DeferredRenderContext` already exposes `PluginAllowsScopedPrincipals(string) bool` (handler_interfaces.go:285). It embeds `TemplatePreviewContext`, which provides `PluginManager()`, and the handler already calls both at line 98–99. No interface needs widening.
4. **E2E a11y placement. Fixed.** The axe check moves to `e2e/tests/accessibility/action-shortcode-a11y.spec.ts`, which `test:with-server:a11y` (`tests/accessibility/`) runs. The functional spec is `e2e/tests/109-action-shortcode.spec.ts` (108 is taken by `[tabs]`).
5. **Ignoring placement. Rationale and doc warning added (§2.1).** A dedicated error message covers selection-only params (§4.2), and it is in the tests.
6. **Filter data on projections. Addressed in §7.1.** Every entity that reaches a template is a full row: `identitiesOnly` MRQL projections never feed item rendering. Tests cover a filtered action inside an `[mrql]` result and on a list card.
7. **Minor points:**
   - Focus: the e2e test now puts the button *inside* the `[autorefresh]` block and asserts that focus lands on the re-rendered button. §5.4 states the n-th-focusable caveat.
   - Lost status text: documented in §5.4.
   - `meta-shortcode-saving`: it is now dispatched only on success, right before `meta-shortcode-updated`, so a failed run never leaves a `savingIn` entry. Capturing at completion time is equivalent because the button is still in place until the refresh it triggers (§5.4).

## 1. Motivation and user value

Plugin actions (`mah.action`) can only be reached today through the generic surfaces: the detail-page action menu, the card menu and the bulk bar. A Category, Resource Category or Note Type author who knows the workflow ("mark reviewed", "generate thumbnail", "archive project", "upscale") cannot put that one step next to the data it belongs to. Examples are a "Mark reviewed" button beside a status pill in `CustomSidebar`, a per-row button in an `[mrql]` result template, or a button on a list card's `CustomSummary`. The menu needs two clicks and shows every action. A template button shows only the step this kind of entity needs, where it is needed, with a label that fits the domain.

`[action]` renders a real `<button>` that runs one registered action on the entity the template is rendering. It posts through the existing CSRF-protected `POST /v1/jobs/action/run`, with the clicking user's own permissions. It reports progress and outcome through the page's aria-live region. When it finishes, it asks the enclosing and same-entity `[autorefresh]` blocks to re-render. Nothing in `docs/ideas/category-templates-and-shortcodes.md` covers this. `[reload]` and `[autorefresh]` re-render content but never run anything.

## 2. Syntax and attributes

```
[action plugin="fal-ai" name="upscale" label="Upscale image" confirm="Run upscale?" param-scale="2"]
```

The shortcode is inline only (`BlockNo`). `[action]…[/action]` renders an error marker, and lint reports it through the existing block-capability check.

| Attribute | Type | Required | Default | Meaning |
|---|---|---|---|---|
| `plugin` | string | yes | (none) | Plugin name, as in `[plugin:<plugin>:…]`. Trimmed. |
| `name` | string | yes | (none) | The action's `id` from `mah.action({ id = … })`. Trimmed. |
| `label` | string | no | the registration's `label` | Visible text, which is also the accessible name. If it is empty after trimming, the default is used. |
| `confirm` | string | no | the registration's `confirm` | Replaces the registration's confirmation text. `confirm=""` counts as unset, so a template can never remove a confirmation the plugin declared. With neither set, there is no confirmation. |
| `param-<name>` | string (wildcard) | no | the registration's `default` | Sets action param `<name>` (§4.2). |

There are no other attributes. The only post-run effect is the refresh signal in §5.4.

### 2.1 Placement is ignored (policy and rationale)

`placement` (`detail`, `card`, `bulk`) decides where the *generic UI* lists an action. It is not an authorization boundary: the run handler (`action_handlers.go:202`) looks the action up with `FindAction` and never checks placement, so a hand-made POST can already run any registered action. Requiring a placement would add no security. It would also force plugin authors to add `detail` to actions they want only in templates, which would then appear in the menu too. So `[action]` accepts any registered action whose `entity` matches.

The one real meaning a placement can carry is that a `bulk`-only action needs a selection (an `entity_ref` param with `default = "selection"`). A template has no selection, so §4.2 gives that case its own error message. The docs warn plugin authors: "an action that only makes sense with a selection cannot be run from `[action]`; declare its selection param `required` so template authors see a clear error".

## 3. Rendered output

When the action is available (§4):

```html
<action-shortcode data-plugin="fal-ai" data-action="upscale" data-entity-type="resource" data-entity-id="42" data-async="true" data-label="Upscale image" data-confirm="Run upscale?" data-params="{&#34;scale&#34;:2}"><button type="button" class="action-shortcode-button">Upscale image</button><span class="action-shortcode-status"></span></action-shortcode>
```

- Every attribute value goes through `html.EscapeString`. `data-params` holds `json.Marshal` of the final param map, and `data-confirm` is left out when there is no confirmation. `data-async` is `"true"` or `"false"` and comes from the registration.
- The accessible name is the visible text, with no `aria-label`, which satisfies WCAG 2.5.3. The label is never empty.
- `.action-shortcode-status` holds the latest outcome text for sighted users and is deliberately **not** a live region. Announcements go through the global `window.mahAnnounce(msg, {assertive})`, whose live region sits on `<body>`. A re-render of the button's block therefore cannot cut an announcement off. A local live region would announce everything twice, and the refresh would destroy it.
- The CSS goes in `public/index.css` next to `.reload-shortcode-button`. `action-shortcode` gets `display:inline-flex; align-items:center; gap:.5rem`, the button reuses the existing secondary-button classes, and there is a visible focus ring. The status text gets `aria-disabled` styling for the running state. The a11y spec checks contrast in both themes.

**Unavailable**: `shortcodeComment("action unavailable in this context")`. This is one byte-identical comment for every cause in §4.1, so a page reveals nothing about which plugins or actions exist or which ones the viewer may run. It mirrors `ErrPluginUnavailable`.

**Author errors** use `shortcodeErrorMarker("action", msg)`:
- `[action] requires plugin= and name=`
- `[action] cannot be used inside a [reload] button face` (`withinReloadFace`, which would make a button inside a button)
- `[action] does not take a body`
- `[action] <param>: <message>` (§4.2). This only happens after access has been granted, so it reveals nothing to an account that could not run the action.

## 4. Behaviour

### 4.1 Availability: fail closed, in this order

1. `ctx.ForceReadOnly`: share pages, and principals that cannot write (the tag sets this from `!principal.CanWrite()`). The resolver is not called.
2. `ctx.UntrustedSource`: a Description or note text block under `-auth`. Without this rule, a `user`-role author could plant a button that an admin clicks, which would run with the admin's rights behind a confirm text the author controls. Template slots are written by admins and editors and stay trusted. The resolver is not called.
3. There is no `ActionResolver` on the request context. This applies to the share server, live template preview, description preview, the entity picker, direct MRQL API result rendering (`render=1`), template-run tools and the CustomCSS tag. The browser's `/mrql` cards use `render=list`, which goes through the authorized `process_shortcodes` path and can render buttons.
4. The resolver answers `ErrActionUnavailable` when any of these is true:
   - the action predicate refuses the plugin (§8.2), which covers a guest, a non-writer, and a group-limited principal for a plugin the operator has not opened to scoped accounts;
   - `pm.FindAction(plugin, name)` fails (the plugin is missing or disabled, the action is not registered, or there is no VM);
   - `action.Entity != ctx.EntityType` (carrier contexts never match);
   - `ctx.EntityID == 0`;
   - the action declares `Filters` and `ActionMatchesEntityData(action, ActionEntityData(ctx.Entity, ctx.EntityType))` is false. Entity data that cannot be built counts as a mismatch when filters are declared.

### 4.2 Params, resolved at render time with no DB access

The resolver starts from the registration, as `pluginActionModal.open` does:
- An `info` param is skipped.
- `entity_ref` with default `trigger` or `both` (or no default), where `param.Entity == action.Entity`, gets the current id: `float64(id)`, or `[]any{float64(id)}` when `multi`.
- `entity_ref` with default `selection` gets empty (`nil`, or `[]any{}`). After overrides and `show_when`, a visible, required selection param gives `needs a bulk selection and cannot run from a template`.
- Any other param gets `param.Default` if set, otherwise `false` for a `boolean` and `""` for everything else.

Then each `param-<name>` attribute is overlaid. `<name>` must be declared. Coercion:

| type | rule | error |
|---|---|---|
| `text`, `textarea`, `hidden` | the string as written | (none) |
| `select` | the string as written, checked against `Options` by `ValidateActionParams` | its message |
| `number` | `strconv.ParseFloat`, finite | `must be a number` |
| `boolean` | exactly `true` or `false` | `must be true or false` |
| `entity_ref`, `info` | not allowed | `cannot be set from a template` |
| undeclared | not allowed | `no such parameter` |

Then `plugin_system.StripHiddenParams` and `plugin_system.ValidateActionParams` run, and the first error becomes the marker. Attribute values are literal, because the parser does not expand shortcodes in attributes and that is unchanged. A visible trigger reference whose own filter excludes the loaded entity makes the button unavailable; one without an override has already passed the action's primary filter. The DB-backed `ValidateActionEntityRefs` and `ValidateActionEntityFilters` are not run at render time. The run endpoint runs both on every click. Param values are visible in the page source, which is acceptable because the action modal shows the same defaults to the same user.

### 4.3 Name collision

Existing text that contains `[action …]` will start rendering as the comment or a marker. The release note calls this out. Lint's near-miss check will start suggesting `action` for `[acton]`.

## 5. Client behaviour (`src/webcomponents/actionShortcode.js`)

### 5.1 Activation
- There is one `click` listener on the inner button. Keyboard activation is native.
- While a run is in flight, the button gets `aria-disabled="true"` (not `disabled`, so focus stays on it) and further clicks are ignored.
- Activation snapshots the payload, label and confirmation, before calling `askToConfirm(text)` from `src/components/confirmDialog.js`. The confirmation fails closed: a decline, a disconnect, or a payload change before acceptance sends no request. The confirmation owns Escape, and the lightbox suspends its focus trap and shortcuts while the confirmation is open.
- The status becomes "Running…" and the component calls `mahAnnounce("<label>: running")`.
- It sends `fetch('/v1/jobs/action/run', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({plugin, action, entity_ids:[entityId], params})})`. CSRF comes from the existing `src/csrf.js` fetch wrapper.

### 5.2 Response (same decoding as `pluginActionModal.submit`)
- `!resp.ok`: the message is `data.errors[*].message` joined with `; `, or `data.error`, or `Action failed`. It is shown and announced assertively. No events are dispatched.
- `data.success === false`: show and announce the refusal before considering a redirect or Job handle. Do not poll or refresh.
- `data.redirect` without refusal: `window.location.href = data.redirect`.
- Sync: `data.success === true` shows and announces (politely) `data.message || "Done"`, then signals the refresh (§5.4). Anything else is a failure with `data.message || "Action failed"`, announced assertively, and nothing is dispatched.
- Async (202): the status becomes "Started" (polite). With `data.canonicalJobId`, the component polls (§5.3). Without it, it shows "Started. Follow it in the Jobs panel." and stops.

### 5.3 Async completion
- `GET /v1/jobs/{canonicalJobId}` every 2 s until `state` is in `terminalStates()` (`src/components/jobStates.js`).
- A host that disconnected while the POST was pending never starts its progress watcher. Polling stops on disconnect or after five minutes of wall time, clearing both timers and aborting any in-flight progress request. At the deadline the status reads "Still running. Follow it in the Jobs panel." The Job itself is not cancelled.
- `succeeded`: announce `presentState(job).label` (this covers partial), then refresh. `failed`, `cancelled` and `interrupted`: show the label and announce it assertively. A non-OK poll gives "Could not read the job's progress". The component never opens the Jobs panel, because that would steal focus.

### 5.4 Refresh signal
- Only after a successful outcome, the component dispatches `document` `meta-shortcode-saving` with `{source}` and, immediately after, `meta-shortcode-updated` with `{entityType, entityId, source}` and no `meta`. The identity comes from the submitted payload. If the host is still attached and unchanged, it supplies the source; otherwise, the source is null so the entity match still refreshes same-entity blocks without capturing whatever now encloses the host. `autorefreshShortcode.js` captures and consumes a non-null source's enclosing blocks in the same tick, so a failed or abandoned run never creates an entry.
- Both existing listeners, the two `meta-shortcode.ts` copies and `display-mode.ts`, ignore an event without `meta`.
- The page is never reloaded. Authors wrap dependent content in `[autorefresh]`.
- **Focus.** When the button sits inside the refreshing block, autorefresh's existing restore moves focus to the element at the same index among the block's focusables. If the template places a variable number of focusables *before* the button in that block (for example a link that appears only after the action), focus can land elsewhere. The docs recommend putting the button first in the block, or outside it.
- **Status text.** When the button sits inside a block that refreshes, its local status text is lost with the old markup. The announcement survives, because it lives on `<body>`. This is documented.
- `src/main.js` imports `./webcomponents/actionShortcode.js` next to the reload and autorefresh imports.

## 6. Scoping and authorization

- **Render.** The entity a template renders is always one the viewer can already see: the page entity, subtree-scoped `[mrql]` results, a `[using]` target that falls back to `[else]`, or a card from a scoped list. The action predicate is `CanWrite && pluginAccess(plugin)`, which matches `auth.PluginActionAccessFor` exactly, so a button is never offered where the run would answer 403 for plugin access. With no `_pluginAccess`, the tag falls back to `auth.PluginAccessFor(reqCtx, nil)`: a group-limited principal gets no plugins, the same fail-closed behaviour `[plugin:*]` has today.
- **Run.** The unchanged run handler enforces everything: CSRF, `withAuthorization` (a guest POST is refused), per-plugin access, `entityVisibleForAction` (403 outside the subtree), params, entity_ref and filter validation, and the BulkMax and deployment caps. A forged `data-entity-id` gets exactly the answer a hand-made POST gets.
- **Assumption.** "The user's own permissions" means the clicker's role plus their Group subtree, as enforced by the existing endpoint.
- Async jobs are owner-tagged, and `GET /v1/jobs/{id}` applies the existing visibility predicate.

## 7. Performance (millions of resources)

- Render costs **zero DB queries** per button. `FindAction` is an in-memory lookup under `RLock`, filter data comes from columns already on the loaded row, and param validation is pure. A 100-card list or an `[mrql]` of N rows with one button each adds no queries and does not touch the inline-MRQL query budget. The render meter counts the output bytes, and each `[action]` costs one `m.tick()`.
- A click costs one POST. An async run adds at most one GET every 2 s per running button, for at most 5 minutes.

### 7.1 Filter data per surface
`ActionEntityData` reads `ContentType` and `ResourceCategoryId` for a resource, `CategoryId` for a group, and `NoteTypeId` for a note. Every surface that reaches the tag or `Process` with a content entity carries the full row:
- Detail pages and the lightbox load the full model.
- List cards and `NewListCardRenderer` get `models.Resource`, `Group` and `Note` from the regular list queries, which select all columns.
- `[mrql]` items come from `convertResultItemsWithData` (`&result.Resources[i]` and so on, full rows). The only reduced projection, `identitiesOnly` in `executeCrossEntity`, serves identity listings and never item rendering.
- `[using]` targets and deferred bodies reload the entity by id.

Both pointer and value forms are accepted, because `buildMetaContext` reflects over either.

## 8. Where it lives and why

### 8.1 Built-in Go handler, not a plugin shortcode
A plugin's Lua cannot, and should not, look up or invoke another plugin's registration. The availability decision must use the server's own registry and access predicate. The fail-closed rules (ForceReadOnly, UntrustedSource, no resolver) belong next to `[reload]` and `[autorefresh]`. `shortcodes` cannot import `plugin_system`, because the import runs the other way. The registry is therefore reached through a per-request resolver on the context, following `WithPartialResolver` and `WithRefreshSigner`.

### 8.2 Where the predicate comes from
- **The `process_shortcodes` tag.** It already computes `access := pluginAccessFromContext(ctx, reqCtx)` and reads `_pluginManager`. It adds `actionAccess := func(p string) bool { return auth.PrincipalFromContext(reqCtx).CanWrite() && access(p) }`. This is the same rule as `PluginActionAccessFor` ("not a writer, so nothing; otherwise PluginAccessFor"), built from the predicate the context already carries. The tag wraps its **local** `reqCtx` (not `_reqCtxWithCache`) with `WithActionResolver(reqCtx, BuildActionResolver(pm, actionAccess))` just before `shortcodes.Process`, so `custom_css_tag` stays resolver-free. Pages, list cards, text blocks and error pages are all covered through the keys they already set.
- **`processShortcodesForJSON`** (routes.go) installs `BuildActionResolver(pm, auth.PluginActionAccessFor(reqCtx, appCtx.PluginAllowsScopedPrincipals))`.
- **The deferred handler** installs `BuildActionResolver(ctx.PluginManager(), auth.PluginActionAccessFor(reqCtx, ctx.PluginAllowsScopedPrincipals))` inside the existing `pm != nil` branch. Without this, a button inside `[lazy]`, `[details]` or `[autorefresh]` would turn into the comment after its first refresh.

## 9. Files to change

**New: `shortcodes/action_handler.go`**
- `type ActionButton struct { Label, Confirm string; Async bool; Params map[string]any }`
- `type ActionResolver func(plugin, name string, params map[string]string, ctx MetaShortcodeContext) (ActionButton, error)`. `params` holds only the `param-*` attrs, with the prefix stripped.
- `var ErrActionUnavailable = errors.New("action unavailable in this context")`
- `WithActionResolver(ctx, r)` and `actionResolverFrom(ctx)`
- `RenderActionShortcode(reqCtx, sc, ctx) string`, implementing §3 and §4.1 steps 1–3. It applies the `label` and `confirm` overrides. Any non-`ErrActionUnavailable` error becomes `[action] <msg>`.

**`shortcodes/processor.go`**: `case sc.Name == "action":` dispatches to `RenderActionShortcode`, as for every other built-in. It carries a body/`IsBlock` guard that produces the body marker.

**`shortcodes/builtin_docs.go`**: new entry `action` (`BlockNo`). The attrs are `plugin` and `name` (required), `label`, `confirm`, and `param-` (`Wildcard: true`). The examples are "Mark reviewed" inside `[autorefresh]` and a per-row button in an `[mrql]` block. The `autorefresh` description gains "or when an [action] button for this entity, or inside the block, finishes".

**`shortcodes/lint.go`**
- Add `action` to `builtinBaseNames`.
- Add `LintOptions.ActionExists func(plugin, name string) bool`, memoized like `PartialExists`. When it is nil, the check is off.
- Add `case "action"`. Empty `plugin` or `name` is an error. Inside `reloadSpans` is an error. `ActionExists` returning false gives the warning `no action "<name>" of plugin "<plugin>" is available; this renders nothing`, with the same wording for a missing action and an unreachable one.

**`plugin_system/actions.go`**: export `ActionMatchesEntityData(a ActionRegistration, data map[string]any) bool`, a wrapper over `actionMatchesFilters`.

**New: `server/template_handlers/template_filters/action_resolver.go`**
- `ActionEntityData(entity any, entityType string) map[string]any`, moved verbatim from `routes.go` `buildEntityDataFromEntity` and extended to accept value `models.Resource`, `Group` and `Note`.
- `BuildActionResolver(pm *plugin_system.PluginManager, access auth.PluginAccess) shortcodes.ActionResolver`. With a nil `pm` or nil `access`, it always answers `ErrActionUnavailable`. It implements §4.1 step 4 and §4.2.

**`server/template_handlers/template_filters/shortcode_tag.go`**: §8.2. No new context key is added.

**`server/routes.go`**: delete `buildEntityDataFromEntity` and call `template_filters.ActionEntityData` at line 270. Install the resolver in `processShortcodesForJSON`.

**`server/api_handlers/deferred_render_api_handlers.go`**: install the resolver (§8.2). No interface change is needed.

**`server/api_handlers/shortcode_docs_api_handlers.go`** (lint) and **`template_preview_api_handlers.go`** (piggybacked lint): set `ActionExists` to `PluginActionAccessFor(request.Context(), ctx.PluginAllowsScopedPrincipals)(plugin) && pm.FindAction(plugin, name) == nil`. If either context interface lacks `PluginAllowsScopedPrincipals`, widen it by that one method; `MahresourcesContext` already implements it. Every other `LintOptions` builder keeps nil.

**`server/template_handlers/render_template.go`**: unchanged (point 1).

**Frontend**: new `src/webcomponents/actionShortcode.js`, the import in `src/main.js`, and CSS in `public/index.css`.

**Moved test**: `server/entity_data_test.go` moves to `server/template_handlers/template_filters/action_entity_data_test.go`, renamed and with value-form cases added.

## 10. Tests

**Go: `shortcodes/action_handler_test.go`**
- No resolver gives exactly the comment. A resolver answering `ErrActionUnavailable` gives byte-identical output.
- `ForceReadOnly` and `UntrustedSource` each give the comment, and a spy resolver is never called.
- Missing attrs, a reload face and the block form each give their marker.
- Happy path: the exact HTML, with escaping of `<b>"x"</b>` in the label, the confirm text and a param value. `data-params` round-trips, and `data-confirm` is absent when unset.
- The label override works, and a whitespace label falls back. `confirm=""` keeps the registration's text, and a non-empty value replaces it.
- A resolver error becomes a marker. Only `param-*` attrs reach the resolver, with the prefix stripped.

**Go: `template_filters/action_resolver_test.go`** (real `PluginManager` with a small test Lua plugin)
- The predicate answering false, a nil access and a nil pm each give unavailable.
- An unknown plugin, an unknown action, an entity mismatch and `EntityID` 0 each give unavailable.
- `content_types` mismatch gives unavailable and a match gives OK. `category_ids` is checked on pointer and value resources, groups and notes.
- Coercion: `number` OK, `abc` and `Inf` are errors; `boolean` `true` OK, `yes` an error; a `select` outside its options is an error; an undeclared param is an error; an `entity_ref` attr is an error.
- Defaults: the trigger default becomes the id (single and multi); a required param with nothing set is an error; a `show_when`-hidden param is stripped.
- **A bulk-only action whose `selection` param is required gives `needs a bulk selection and cannot run from a template`.**
- `Async` and `Confirm` are copied from the registration.

**Go: `shortcode_tag` test**: with `_pluginAccess` absent and a group-limited principal, the output is the comment. With an admin principal and no `_pluginAccess`, the output is the button. A guest gets the comment.

**Go: `shortcodes/lint_test.go`**: missing attrs give an error; nested in `[reload]` gives an error; `ActionExists` false gives the warning; nil gives no warning; `ActionExists` is called once for duplicate references; an unknown attr gives a warning; `param-x` gives no warning; `[acton]` gets the near-miss suggestion.

**Go: consistency**: `builtin_docs_consistency_test.go` forces the docs-site row, the three reference panels and the example lint.

**Go: `server/api_tests/action_shortcode_render_test.go`** (pattern: `plugin_action_render_gate_test.go`). The fixture is a resource category with `CustomSidebar` set to `[action plugin="test-actions" name="sync-greet"]`.
- Admin detail page: the button is present with the right id.
- Guest: the comment, with no button.
- Group-limited user, plugin not opened: no button, and the HTML at that spot is identical to the output for `[action plugin="no-such" name="x"]`.
- Group-limited user, opened plugin, entity inside the subtree: the button is present.
- A Description containing `[action …]` under `-auth`: no button.
- `POST /v1/shortcodes/deferred` for an `[autorefresh]` body: the button is present.
- **The detail `.json` route answers 200 with valid JSON whose lightbox slot contains `<action-shortcode`.** This guards point 1.
- **A list card** (resource list with the category's `CustomSummary` holding the button) and **an `[mrql]` result** (block template with a per-row button): present for the admin. With an action `filters.content_types = {"image/png"}`, the button appears only on the PNG row and card.

**JS: `src/webcomponents/actionShortcode.test.ts`** (vitest, pattern: `reloadActivation.test.ts`)
- The exact POST body. A declined confirm sends no fetch.
- Sync success dispatches `saving` then `updated` in that order, with `source` equal to the host and no `meta`, and announces politely.
- `success:false`, `{errors}` and `{error}` each announce assertively and dispatch **neither** event.
- A redirect navigates. A second click while running is ignored.
- Async: the component polls and dispatches on `succeeded`. `failed` dispatches nothing. Polling stops on disconnect and at the cap. With no `canonicalJobId`, there is no polling.

**E2E: `e2e/tests/109-action-shortcode.spec.ts`**
- New plugin `e2e/test-plugins/test-action-button/plugin.lua`, kept separate so the `test-actions` counts do not change, and enabled through `client.enablePlugin` as in `plugin-actions.spec.ts`. It has a sync action `mark-reviewed` (entity resource, `confirm = "Mark as reviewed?"`) that patches the description to `reviewed`.
- The category template is `[autorefresh][action plugin="test-action-button" name="mark-reviewed"] [property path="Description"][/autorefresh]`, with the button first in the block.
- Tab to the button and press Enter. The confirm dialog opens; confirm it. The description shows `reviewed` with no navigation (the URL is unchanged and a window marker survives). The live region contains the message. **`document.activeElement` is the re-rendered `.action-shortcode-button`**, and it is a new node, not the old one.
- `async-demo` (test-actions) reaches "Succeeded" when the button sits outside any block.
- A guest sees no button.

**E2E a11y: `e2e/tests/accessibility/action-shortcode-a11y.spec.ts`** (a11y fixture): `checkA11y()` runs on that detail page with the button idle and again after a completed run, in light and dark themes.

## 11. Docs

- **`docs-site/docs/features/shortcodes.md`**
  - Add the table row `| \`[action]\` | Inline | Run a plugin action on this entity from a button |`.
  - Add a section "`[action]` -- Run a plugin action" covering: the attributes; the coercion table; the unavailable cases (non-writers, guests, unopened plugins for scoped users, untrusted Descriptions, share pages, live preview); the confirm rule; async behaviour; pairing with `[autorefresh]`; the focus and status-text notes from §5.4; the placement rule (§2.1).
  - Update the trigger text in the `[autorefresh]` section.
- **`docs-site/docs/features/plugin-actions.md`**: add a subsection "Running an action from a template", covering that placement is ignored, the warning about selection-only actions, and that template params are literal.
- **`docs-site/docs/features/custom-templates.md`**: add a one-line mention next to `[reload]` and `[autorefresh]`.
- **`templates/createCategory.tpl`**, **`createResourceCategory.tpl`** and **`createNoteType.tpl`**: add an `[action]` line to the reference panels. The consistency test requires it.
- **`.claude/skills/mahresources-category-designer/references/shortcodes.md`**: add an `[action]` entry.
- At implementation time, per AGENTS.md: this plan is saved as `docs/plans/2026-11-26-shortcode-action.md`. The plans index is regenerated by whoever merges the shortcode lanes.

## 12. Success criterion

On a resource detail page whose category template has `[action plugin="test-action-button" name="mark-reviewed"]` inside an `[autorefresh]` that shows the Description:
- An editor or admin activates the button by keyboard, confirms, and sees the Description change in place without navigation. The result is announced through the aria-live region, focus ends on the re-rendered button, and axe (`tests/accessibility/`) reports no violations.
- A guest, a group-limited user without access to the plugin, a share page, the live preview and an untrusted Description all render the identical "action unavailable" comment, the same one a nonexistent plugin renders.
- The same button works on list cards, in `[mrql]` result rows, in deferred bodies and in the lightbox, and a filtered action appears only on matching rows.
- Every `.json` template route still encodes.
- A forged click outside a group-limited user's subtree gets the run endpoint's existing 403.
- N buttons add zero DB queries.
- `/v1/shortcodes/lint` warns about an unknown action.
- `go test --tags 'json1 fts5' ./...`, vitest, and `cd e2e && npm run test:with-server` plus `npm run test:with-server:a11y` all pass.

---

## Last evaluator feedback

I checked the plan's main claims against the code and they hold. I would approve it as written, with the small fixes below.

Verified:
- shortcode_tag.go already has `pluginAccessFromContext` and `_pluginManager` in hand. It sets ForceReadOnly from `!principal.CanWrite()`, and `CanWrite` is nil-safe.
- `PluginActionAccessFor` is CanWrite plus PluginAccessFor, so the plan's predicate is equivalent.
- The deferred handler already calls `ctx.PluginManager()` and `ctx.PluginAllowsScopedPrincipals` at lines 98-99.
- `processShortcodesForJSON` builds its own context, so explicit wiring there is correct.
- `buildEntityDataFromEntity` is at routes.go:368 and is tested in server/entity_data_test.go.
- The run handler uses FindAction and never checks placement (action_handlers.go:202).
- autorefreshShortcode.js reads `savingIn` and deletes it on update, and `hostsToRefresh` does the entity-wide match.
- Both `meta-shortcode.ts` copies and `display-mode.ts` ignore an event without `meta`.
- `askToConfirm`, `terminalStates` and `presentState` exist, and `window.mahAnnounce` is in main.js.
- list_card_renderer, text_block_renderer and server.go set `_pluginAccess`.

Problems to fix before or during implementation:
1. **Lint snippet won't compile.** `PluginManager.FindAction` returns `(ActionRegistration, *lua.LState, error)`, so `pm.FindAction(plugin, name) == nil` is wrong. Write `_, _, err := pm.FindAction(...); err == nil` in both lint builders. Also state that a nil pm makes `ActionExists` return false, or leave it nil, so it cannot panic.
2. **The §8.2 reqCtx wording is muddled.** The tag's local `reqCtx` *is* the `_reqCtxWithCache` value (lines 79-91). Say "wrap a local copy derived from it and never write it back to ctx.Public". The goal is that the custom_css tag and later tags never inherit the resolver.
3. **FindAction holds `pm.mu.RLock` per call.** On a 100-card list with several buttons per card that is fine. Still, confirm the resolver never calls FindAction while holding another pm lock, to rule out lock-order issues with plugin enable/disable.
4. **Some §3 CSS wording is vague.** "reuses the existing secondary-button classes" and "status text gets aria-disabled styling" should name the actual classes and selectors, e.g. `[aria-disabled=true]` on the button rather than on the status span.
5. **Share server.** State explicitly that it neither installs a resolver nor sets `_pluginManager`, and add one assertion in the share e2e or Go test. The plan lists it as unavailable but has no test that pins it.
6. **Usefulness caveat.** Value depends on plugins exposing writer actions with entity-scoped params. Placement-ignored is well argued, but a short worked example from an existing bundled plugin in docs (an action that already exists) would show immediate value beyond the test plugin.

## 13. Implementation checklist

- [x] `shortcodes/action_handler.go`: `ActionButton`, `ActionResolver`, `ErrActionUnavailable`, `WithActionResolver`, `RenderActionShortcode` (§3, §4.1 steps 1 to 3), `lintAction`.
- [x] `shortcodes/processor.go` dispatch and the parser alternations (`shortcodePattern`, `closingTagPattern`).
- [x] `shortcodes/builtin_docs.go`: the `action` entry (`BlockNo`, `plugin`/`name` required, `label`, `confirm`, `param-` wildcard) and the `[autorefresh]` trigger sentence.
- [x] `shortcodes/lint.go`: `action` in `builtinBaseNames`, memoized `LintOptions.ActionExists`, the `[reload]` face error, the missing-action warning.
- [x] `plugin_system/actions.go`: `ActionMatchesEntityData`.
- [x] `template_filters/action_resolver.go`: `ActionEntityData` (moved from `routes.go`, value forms added), `BuildActionResolver`, the §4.2 params.
- [x] Wiring: the `process_shortcodes` tag (local context only), `processShortcodesForJSON` (slots only), the deferred handler; `ActionExists` in `/v1/shortcodes/lint` and the template preview's piggybacked lint.
- [x] `src/webcomponents/actionShortcode.js`, its import in `src/main.js`, the CSS in `public/index.css`.
- [x] Go tests: `shortcodes/action_handler_test.go`, `shortcodes/action_lint_test.go`, `template_filters/action_resolver_test.go` (resolver and tag), `template_filters/action_entity_data_test.go` (moved from `server/entity_data_test.go`, value forms added), `server/api_tests/action_shortcode_render_test.go`.
- [x] JS tests: `src/webcomponents/actionShortcode.test.ts`.
- [x] E2E: `e2e/tests/109-action-shortcode.spec.ts`, `e2e/tests/accessibility/action-shortcode-a11y.spec.ts`, `e2e/tests/auth/action-shortcode-roles.spec.ts`, fixture plugin `e2e/test-plugins/test-action-button`.
- [x] Docs: `docs-site/docs/features/shortcodes.md` (row, section, `[autorefresh]` trigger), `plugin-actions.md` ("Running an action from a template"), `custom-templates.md`, the three reference panels, the category-designer skill reference.

## Review

### Deviations from the plan

- **Hover cards render the comment.** Not in §4.1. A hover card is a tooltip the keyboard cannot enter, and `[adjacent]` and `[calendar]` already leave their controls out there (`WithinTooltip`). `actionResolverFrom` answers nil inside one, so the button is never drawn where only a mouse could reach it.
- **The JSON render installs the resolver for the slots, not for CustomCSS.** §8.2 put it on the whole `processShortcodesForJSON` context. On the HTML page the CSS goes through the `custom_css` tag, which has no resolver, so the JSON route now matches it: a stylesheet never holds a button on either surface (pinned by `TestActionShortcode_DeferredAndLightbox` and the page check in `TestActionShortcode_DetailPageFollowsTheRunRule`).
- **Missing `plugin=`/`name=` in lint** is reported by the generic required-attribute check (`[action] is missing required attribute "plugin"`), not by a second, dedicated error saying the same thing.
- **`ActionExists` with no plugin manager answers false** rather than being left nil: with no plugin system every `[action]` renders nothing, so the warning is true. It never panics (evaluator point 1).
- **An `entity_ref` with `default = ""`** gets no id, as in `pluginActionModal.resolveEntityRefDefault` (only `trigger` and `both` take the trigger). "No default" in §4.2 still means the trigger, because registration turns a missing default into `"trigger"`.
- **The click listener is delegated from the host**, as `reloadShortcode.js` does, instead of bound to the inner button: a page morph may replace the button, and a listener on it would go with it.
- **Announcements carry the button's label** (`"Mark reviewed: Marked resource 7 reviewed"`), so a screen-reader user hears which button finished. The status text beside the button shows the message alone.
- **A submitted synchronous action can still complete after disconnect.** Its announcement and refresh address the submitted action and entity; it cannot update an old or repurposed control.
- **The guest E2E case is in `e2e/tests/auth/action-shortcode-roles.spec.ts`**, not in the functional spec: the default Playwright project runs with login off, and only the `auth` project has a guest. It also covers the unopened group-limited user and compares both to a nonexistent plugin.
- **"Light and dark themes"**: the app has no dark theme. The dark surface a button can appear on is the lightbox's stone-900 info panel, so the a11y spec scans the button there (`checkComponentA11y`) besides the page, idle and after a run. The button takes `currentColor` for text and border, which is why one style reads on both.
- **CSS** (evaluator point 4): `.btn-secondary` exists only scoped under `.plugin-action-modal-actions`, so there is no global secondary-button class to reuse. The block in `public/index.css` names `action-shortcode` (inline-flex host), `.action-shortcode-button` (currentColor text and border, 28 px minimum height, amber-700 `:focus-visible` outline), `.action-shortcode-button[aria-disabled="true"]` (dashed border, progress cursor; the busy state is on the button, not the status span) and `.action-shortcode-status:empty` (no space until there is text).
- **A label that is blank in the registration** falls back to the action id, so the button is never nameless.
### Round 1 render-context fixes

- **Literal CSS selectors.** Stylesheet processing keeps a bare shared-parser built-in opener literal when it immediately follows an ASCII letter, digit, `-`, `_`, `*`, `]` or `|`. `form[action]` and `.w[tabs]` survive byte-for-byte; `form[ action]` remains an optional spelling. This includes stylesheet slots and embedded `<style>` bodies; the pre-parser `[comment]` is unchanged. Lint recognizes the same rule, and attributed shortcodes still expand. Verified by `TestStylesheetShortcodesPreserveAttributeSelectorsOnPageAndLightbox`, `TestTemplatePreviewPreservesStylesheetAttributeSelectors` and the parser/lint tests in `css_context_test.go`.
- **Inline result CSS never gets an action resolver.** `renderResultCSS` masks it without changing the result card's own context; `TestInlineMRQLStylesheetMasksActionResolverOnlyForCSS` asserts the comment in CSS and the live button on the card.
- **Deferred hover-card renders remain tooltips.** A sealed token carries the tooltip bit alongside source trust. The endpoint restores it before installing the signers for nested bodies. `TestHoverCardDeferredAndRefreshTokensRetainTooltip` follows actual hover-card and detail-page tokens through `[lazy]`, nested `[lazy]` and `[autorefresh]` to prove controls stay suppressed only in the tooltip.

### Evaluator points

1. **Lint snippet.** `actionExistsFn` (`shortcode_docs_api_handlers.go`) does `_, _, err := pm.FindAction(plugin, name); return err == nil`, behind `pm == nil || !access(plugin)` → false. Both lint builders use it with `auth.PluginActionAccessFor`.
2. **§8.2 reqCtx wording.** The tag's `reqCtx` is `_reqCtxWithCache`. It now builds `renderCtx := shortcodes.WithActionResolver(reqCtx, …)`, passes that to `Process`, and never writes it back to `ctx.Public`. `TestActionShortcodeTag_FollowsTheRunEndpointRule` renders a `custom_css` tag after the shortcode tag and asserts the CSS holds the comment.
3. **FindAction lock order.** Checked: the resolver takes no plugin-manager lock of its own; `FindAction` takes and releases `pm.mu.RLock` inside the call. The resolver runs only from `processWithDepth`'s dispatch, and no plugin-manager method calls `shortcodes.Process` (plugin shortcode output is expanded after `RenderShortcodeContext` returns and its VM lock is released), so it is never reached with `pm.mu` held. Written down in `BuildActionResolver`'s comment.
4. **CSS wording.** See the CSS bullet above.
5. **Share server.** `share_server.go` calls `shortcodes.Process(…, nil, nil)` with `ForceReadOnly = true`, sets no `_pluginManager` and installs no resolver. `TestActionShortcode_SurfacesWithoutAResolver` asserts the share page renders the comment, with the note's own page as the control.
6. **Usefulness.** The `BuiltinDocs` examples, the docs-site section, the reference panels and the skill reference use bundled actions: `project-management`'s `pm-set-status` (`param-status="done"`) and `pm-open-board`, and `fal-ai`'s `upscale` (`param-model="topaz"`).

### Verified

- `gofmt -l` on the changed Go files is empty. `go vet --tags 'json1 fts5' ./...` reports one finding, in `application_context/job_download_progress_test.go:99` (a copied lock), which is on the base commit and untouched here.
- `go test --tags 'json1 fts5' ./... -count=1`: 47 packages pass, including `internal/arch` and the docs consistency tests. Under five lanes' load, `application_context` failed two timing tests (`TestAPauseAskedOfAnotherProcessIsDeliveredToTheTransfer`, `TestReductionSpoolChangedMembersLinearSQLite`, neither in code this lane touched) and `server/api_tests` hit the default 10-minute timeout. Rerun, the two tests pass and `server/api_tests` passes in full with `-timeout 45m` (813 s).
- `npm run test:unit` (vitest), `npm run build`, `./scripts/css-scan-test.sh`.
- E2E: the three new specs, plus `106-reload-shortcode`, `107-autorefresh-shortcode`, `tests/shortcodes/`, `template-preview`, `category-template-authoring-a11y`, `plugins/plugin-actions` and `plugins/plugin-manage`.
- Zero queries per button holds by construction: `BuildActionResolver` is given a plugin manager and an access predicate and no database handle.

### Not verified

- No Postgres-specific test was added: `[action]` issues no SQL at render time, and the run endpoint it posts to already has Postgres coverage. The new API tests were run with the `postgres` tag too, which exercises the same SQLite harness.
- The full browser, CLI and Postgres E2E suites were not run (the machine is shared by five lanes).

### Round 1 follow-up

- `actionShortcode.test.ts` exercises confirmation invalidation on target/param changes and detach/reattach, successful completion after a morph, refusal with a Job handle, removal before the POST returns, abort of an in-flight GET, and the five-minute deadline with an unresolved response body.
- The lightbox a11y spec checks keyboard confirmation, focus containment, Escape cancellation, and focus return to the button without closing the Info panel or posting the action.
- `TestActionResolver_VisibleTriggerReferenceMustMatchItsOwnFilter` checks the unavailable result for a visible mismatching trigger ref, and allows a matching or hidden ref. `TestActionResolver_SelectionRequiredOnlyWhenVisible` checks defaults and a `param-*` override that exposes the required selection.
- `TestActionShortcode_MRQLRenderModes` pins the existing surface split: `render=list` can draw authorized buttons; `render=1` has no action resolver. `TestActionShortcode_DeferredDescriptionKeepsSourceTrust` sends real page-minted tokens through the deferred endpoint under auth: the Description yields the unavailable comment and the trusted template yields the button.

### Round 2 follow-up

- **Run ownership through list refreshes.** An unchanged action host reapplies its busy state after Alpine patches its children. Changing its action attributes or disconnecting releases the control and stops its progress watcher, not the submitted Job. A stale synchronous reply still reports and refreshes the submitted entity but cannot update or unlock a newer run. The same-target, retarget-during-poll and stale-POST tests in `actionShortcode.test.ts` all use the production list morph helper.
- **Response handling.** HTTP 202 or `canonicalJobId` indicates async submission; a legacy `job_id` alone does not. The HTTP 200 success regression checks the exact reply shape, status message, announcement and refresh events.
- **Required params.** The resolver applies defaults, overrides and visibility, then checks trigger-reference eligibility before validating final values. `TestActionResolver_TriggerReferenceEligibilityPrecedesRequiredParams` checks the exact PNG unavailable comment and the required-text marker on a JPEG.
- **Validation.** The regressions failed before the fixes. The targeted Go and Vitest tests, broader shortcode/internal/template-handler Go tests, focused action and render-context API tests, CSS scan, and all six action functional, accessibility and auth E2E tests passed. The initial concurrent Vitest run timed out in three schema-editor adversarial tests; the full run with four workers passed all 2,694 tests. Go vet still reports only the unchanged copied-lock finding in `application_context/job_download_progress_test.go:99`. No SQL changed.

### Round 3 follow-up

- **Keyboard focus through list refreshes.** The shared Alpine morph captures the focused `[action]` button's plugin, action, entity type and ID, and resolved parameters before patching. If the node is retargeted or lost, focus returns to a unique button with the original payload in the refreshed list; otherwise it parks on the list. An unchanged focused button keeps focus, and the repair does not override focus that moved elsewhere.
- **Regression.** The two `a download list morph …` browser cases dispatch the production refresh event after a newer resource is added. With the original action still listed, Enter posts only its ID; with it pushed off the page, focus parks and Enter does not run the arrival unless the reader deliberately focuses it. Vitest covers matches that differ by plugin, name, entity type or parameters, and rejects two exact matches as ambiguous. The seven new cases failed before the fix and passed after it.
- **Validation.** All 2,700 Vitest tests, all eight action browser tests (functional, accessibility and auth), the shortcode/internal/template-handler Go suites and the focused action API tests passed. Go vet reports only the existing copied lock in `application_context/job_download_progress_test.go:99`. No SQL changed.

### Known limits

- When a watcher stops because the button was removed or retargeted, follow the submitted Job in the Jobs panel.
- Round 3 deferred: a late redirect from a submitted action can still navigate away after the same button has been retargeted and used for a newer run.
- Round 3 deferred: in the lightbox, holding Escape can close the viewer after the initial keydown cancels the confirmation; repeated keydowns arrive after the dialog has unmounted.
- Round 4 deferred: if a list morph retargets the button while its confirmation is open, the dialog still returns focus to the reused (now retargeted) button on close. The old confirmation never posts; the next Enter opens a fresh confirmation for the new entity.
- Round 4 fixed by the parent: when list-morph focus repair moves focus to the original action elsewhere in the list, it scrolls that button into view (`block: 'nearest'`), so the focus indicator stays visible.

